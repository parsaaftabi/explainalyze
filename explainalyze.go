// Package explainalyze parses PostgreSQL EXPLAIN (ANALYZE, BUFFERS,
// FORMAT JSON) output and flags the problems you actually look for:
// sequential scans over real data, planner misestimates, disk sorts,
// lossy bitmaps, runaway nested loops, and the nodes eating your time.
//
//	plan, err := explainalyze.Parse(reader)
//	if err != nil { ... }
//	for _, f := range explainalyze.Analyze(plan) {
//		fmt.Printf("[%s] %s\n", f.Severity, f.Message)
//	}
//
// The CLI in cmd/explainalyze wraps this for terminal use:
//
//	psql -c "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) SELECT ..." | explainalyze
package explainalyze

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Node is one plan node. Only the fields the analysis needs are decoded;
// anything else in the JSON is ignored.
type Node struct {
	NodeType     string  `json:"Node Type"`
	RelationName string  `json:"Relation Name"`
	Alias        string  `json:"Alias"`
	IndexName    string  `json:"Index Name"`
	JoinType     string  `json:"Join Type"`
	SortMethod   string  `json:"Sort Method"`
	SortSpaceUse float64 `json:"Sort Space Used"`
	SortSpaceTyp string  `json:"Sort Space Type"`

	StartupCost float64 `json:"Startup Cost"`
	TotalCost   float64 `json:"Total Cost"`
	PlanRows    float64 `json:"Plan Rows"`

	ActualStartupTime float64 `json:"Actual Startup Time"`
	ActualTotalTime   float64 `json:"Actual Total Time"`
	ActualRows        float64 `json:"Actual Rows"`
	ActualLoops       float64 `json:"Actual Loops"`

	SharedHitBlocks     float64 `json:"Shared Hit Blocks"`
	SharedReadBlocks    float64 `json:"Shared Read Blocks"`
	SharedDirtiedBlocks float64 `json:"Shared Dirtied Blocks"`
	SharedWrittenBlocks float64 `json:"Shared Written Blocks"`
	TempReadBlocks      float64 `json:"Temp Read Blocks"`
	TempWrittenBlocks   float64 `json:"Temp Written Blocks"`

	ExactHeapBlocks float64 `json:"Exact Heap Blocks"`
	LossyHeapBlocks float64 `json:"Lossy Heap Blocks"`

	Plans []*Node `json:"Plans"`
}

// Result is one statement's EXPLAIN output.
type Result struct {
	Plan          *Node
	PlanningTime  float64 `json:"Planning Time"`
	ExecutionTime float64 `json:"Execution Time"`
}

// Parse decodes EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) output. It accepts
// the standard top-level array as well as a bare single object.
func Parse(r io.Reader) (*Result, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("explainalyze: read: %w", err)
	}
	data = bytes.TrimSpace(data)

	var arr []struct {
		Plan          *Node   `json:"Plan"`
		PlanningTime  float64 `json:"Planning Time"`
		ExecutionTime float64 `json:"Execution Time"`
	}
	if err := json.Unmarshal(data, &arr); err == nil && len(arr) > 0 && arr[0].Plan != nil {
		return &Result{Plan: arr[0].Plan, PlanningTime: arr[0].PlanningTime, ExecutionTime: arr[0].ExecutionTime}, nil
	}
	var single struct {
		Plan          *Node   `json:"Plan"`
		PlanningTime  float64 `json:"Planning Time"`
		ExecutionTime float64 `json:"Execution Time"`
	}
	if err := json.Unmarshal(data, &single); err != nil || single.Plan == nil {
		return nil, fmt.Errorf("explainalyze: not EXPLAIN (ANALYZE, FORMAT JSON) output")
	}
	return &Result{Plan: single.Plan, PlanningTime: single.PlanningTime, ExecutionTime: single.ExecutionTime}, nil
}

// Severity ranks a finding.
type Severity string

const (
	// Warning is something you should probably fix.
	Warning Severity = "WARN"
	// Note is worth knowing but often acceptable.
	Note Severity = "NOTE"
)

// Finding is one diagnosed problem.
type Finding struct {
	Severity Severity `json:"severity"`
	NodeType string   `json:"node_type"`
	Relation string   `json:"relation,omitempty"`
	Message  string   `json:"message"`
	Detail   string   `json:"detail,omitempty"`
}

// Options tune the analysis thresholds.
type Options struct {
	// SeqScanRows: warn on sequential scans touching at least this many rows.
	SeqScanRows float64
	// MisestimateRatio: warn when actual/estimated rows differ by this factor.
	MisestimateRatio float64
	// SlowNodeMs: nodes whose exclusive time exceeds this are candidates for
	// the "eating your time" note.
	SlowNodeMs float64
	// SlowNodeFraction: ...when they also consume this fraction of total time.
	SlowNodeFraction float64
	// BufferBlocks: note plans touching at least this many shared buffers.
	BufferBlocks float64
	// NestedLoopRows: warn when a nested loop's inner side produces at least
	// this many total rows (rows x loops).
	NestedLoopRows float64
}

// DefaultOptions are deliberately conservative: they flag clear problems,
// not noise.
func DefaultOptions() Options {
	return Options{
		SeqScanRows:      10000,
		MisestimateRatio: 10,
		SlowNodeMs:       100,
		SlowNodeFraction: 0.5,
		BufferBlocks:     10000,
		NestedLoopRows:   100000,
	}
}

// Analyze walks the plan and returns findings, ordered by discovery
// (parents before children).
func Analyze(res *Result, opts ...func(*Options)) []Finding {
	o := DefaultOptions()
	for _, fn := range opts {
		fn(&o)
	}
	a := &analyzer{opts: o, totalMs: res.ExecutionTime}
	a.walk(res.Plan)
	if a.totalBuffers() >= o.BufferBlocks {
		a.add(Finding{
			Severity: Note,
			NodeType: res.Plan.NodeType,
			Message:  fmt.Sprintf("plan touched %s shared buffers (%s)", commas(int64(a.bufs)), formatBytes(int64(a.bufs)*8192)),
			Detail:   "High buffer counts mean the query reads a lot of data; check whether an index or a tighter filter shrinks it.",
		})
	}
	return a.findings
}

type analyzer struct {
	opts     Options
	totalMs  float64
	bufs     float64
	findings []Finding
}

func (a *analyzer) add(f Finding) { a.findings = append(a.findings, f) }

func (a *analyzer) totalBuffers() float64 { return a.bufs }

func (a *analyzer) walk(n *Node) {
	if n == nil {
		return
	}
	a.bufs += n.SharedHitBlocks + n.SharedReadBlocks + n.SharedDirtiedBlocks + n.SharedWrittenBlocks

	rel := n.RelationName
	if rel == "" {
		rel = n.Alias
	}

	// 1. Sequential scans over real data.
	if n.NodeType == "Seq Scan" && n.ActualRows >= a.opts.SeqScanRows {
		a.add(Finding{
			Severity: Warning,
			NodeType: n.NodeType,
			Relation: rel,
			Message:  fmt.Sprintf("sequential scan on %s touched %s rows", quote(rel), commas(int64(n.ActualRows))),
			Detail:   "A seq scan this large usually means a missing index on the filter/join column. Verify with the WHERE clause before adding one.",
		})
	}

	// 2. Planner misestimates.
	if n.PlanRows >= 1 && n.ActualLoops >= 1 {
		ratio := n.ActualRows / n.PlanRows
		inv := n.PlanRows / n.ActualRows
		if n.ActualRows > 0 && (ratio >= a.opts.MisestimateRatio || inv >= a.opts.MisestimateRatio) {
			a.add(Finding{
				Severity: Warning,
				NodeType: n.NodeType,
				Relation: rel,
				Message: fmt.Sprintf("planner misestimated %s by %.1fx (planned %s rows, got %s)",
					quote(label(n, rel)), max(ratio, inv), commas(int64(n.PlanRows)), commas(int64(n.ActualRows))),
				Detail: "Bad estimates pick bad plans. Run ANALYZE on the table; for skewed columns consider extended statistics (CREATE STATISTICS).",
			})
		}
	}

	// 3. Sorts spilling to disk.
	if n.NodeType == "Sort" && strings.Contains(n.SortMethod, "external") {
		a.add(Finding{
			Severity: Warning,
			NodeType: n.NodeType,
			Relation: rel,
			Message:  fmt.Sprintf("sort spilled to disk (%s, %s %s)", n.SortMethod, commas(int64(n.SortSpaceUse)), n.SortSpaceTyp),
			Detail:   "Disk sorts are slow. Raise work_mem until the sort fits in memory, or reduce the sorted row count earlier in the plan.",
		})
	}

	// 4. Lossy bitmap heap scans.
	if n.NodeType == "Bitmap Heap Scan" && n.LossyHeapBlocks > 0 {
		a.add(Finding{
			Severity: Warning,
			NodeType: n.NodeType,
			Relation: rel,
			Message:  fmt.Sprintf("bitmap heap scan on %s went lossy on %s pages", quote(rel), commas(int64(n.LossyHeapBlocks))),
			Detail:   "Lossy pages recheck every tuple on the page. Raise work_mem so the bitmap stays exact.",
		})
	}

	// 5. Runaway nested loops.
	if n.NodeType == "Nested Loop" && len(n.Plans) == 2 {
		inner := n.Plans[1]
		total := inner.ActualRows * inner.ActualLoops
		if total >= a.opts.NestedLoopRows {
			a.add(Finding{
				Severity: Warning,
				NodeType: n.NodeType,
				Relation: rel,
				Message:  fmt.Sprintf("nested loop executed its inner side %s times (%s total rows)", commas(int64(inner.ActualLoops)), commas(int64(total))),
				Detail:   "Nested loops scale with the outer row count. A hash or merge join is usually cheaper here; check join selectivity and work_mem.",
			})
		}
	}

	// 6. The node eating your time.
	if excl := exclusiveMs(n); excl >= a.opts.SlowNodeMs && a.totalMs > 0 && excl/a.totalMs >= a.opts.SlowNodeFraction {
		a.add(Finding{
			Severity: Note,
			NodeType: n.NodeType,
			Relation: rel,
			Message: fmt.Sprintf("%s consumed %.1f%% of execution time (%.1f ms exclusive)",
				label(n, rel), 100*excl/a.totalMs, excl),
		})
	}

	for _, c := range n.Plans {
		a.walk(c)
	}
}

// exclusiveMs estimates the node's own time: its total minus its children,
// scaled by loop count. Times in EXPLAIN are per-loop averages.
func exclusiveMs(n *Node) float64 {
	child := 0.0
	for _, c := range n.Plans {
		child += c.ActualTotalTime
	}
	excl := n.ActualTotalTime - child
	if excl < 0 {
		excl = 0
	}
	loops := n.ActualLoops
	if loops < 1 {
		loops = 1
	}
	return excl * loops
}

// label prefers "Index Scan on orders using idx", falling back sanely.
func label(n *Node, rel string) string {
	if rel == "" {
		return n.NodeType
	}
	if n.IndexName != "" {
		return n.NodeType + " on " + rel + " using " + n.IndexName
	}
	return n.NodeType + " on " + rel
}

func quote(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func commas(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	for _, u := range []string{"KB", "MB", "GB", "TB"} {
		f /= unit
		if f < unit || u == "TB" {
			return fmt.Sprintf("%.1f %s", f, u)
		}
	}
	return fmt.Sprintf("%d B", n)
}
