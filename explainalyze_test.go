package explainalyze

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func parse(t *testing.T, doc string) *Result {
	t.Helper()
	res, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return res
}

func severities(fs []Finding) map[string]int {
	m := map[string]int{}
	for _, f := range fs {
		m[string(f.Severity)]++
	}
	return m
}

func TestParse_Invalid(t *testing.T) {
	if _, err := Parse(strings.NewReader("not json")); err == nil {
		t.Error("expected error for non-JSON")
	}
	if _, err := Parse(strings.NewReader(`{"foo": 1}`)); err == nil {
		t.Error("expected error for JSON without a plan")
	}
}

const seqScanPlan = `[{
  "Plan": {
    "Node Type": "Seq Scan",
    "Relation Name": "orders",
    "Alias": "orders",
    "Startup Cost": 0.00,
    "Total Cost": 1234.56,
    "Plan Rows": 95000,
    "Plan Width": 72,
    "Actual Startup Time": 0.5,
    "Actual Total Time": 120.0,
    "Actual Rows": 95000,
    "Actual Loops": 1,
    "Shared Hit Blocks": 800,
    "Shared Read Blocks": 200
  },
  "Planning Time": 0.5,
  "Execution Time": 125.0
}]`

func TestAnalyze_SeqScan(t *testing.T) {
	fs := Analyze(parse(t, seqScanPlan))
	if len(fs) == 0 {
		t.Fatal("expected findings")
	}
	found := false
	for _, f := range fs {
		if f.Severity == Warning && strings.Contains(f.Message, "sequential scan") {
			found = true
		}
	}
	if !found {
		t.Errorf("no seq-scan warning in %v", fs)
	}
}

const misestimatePlan = `[{
  "Plan": {
    "Node Type": "Index Scan",
    "Relation Name": "users",
    "Index Name": "users_email_idx",
    "Plan Rows": 10,
    "Actual Startup Time": 0.1,
    "Actual Total Time": 45.0,
    "Actual Rows": 50000,
    "Actual Loops": 1
  },
  "Planning Time": 0.3,
  "Execution Time": 46.0
}]`

func TestAnalyze_Misestimate(t *testing.T) {
	fs := Analyze(parse(t, misestimatePlan))
	found := false
	for _, f := range fs {
		if f.Severity == Warning && strings.Contains(f.Message, "misestimated") {
			found = true
		}
	}
	if !found {
		t.Errorf("no misestimate warning in %v", fs)
	}
}

const sortSpillPlan = `[{
  "Plan": {
    "Node Type": "Sort",
    "Sort Method": "external merge",
    "Sort Space Used": 12345,
    "Sort Space Type": "Disk",
    "Plan Rows": 200000,
    "Actual Startup Time": 10.0,
    "Actual Total Time": 900.0,
    "Actual Rows": 200000,
    "Actual Loops": 1,
    "Plans": [{
      "Node Type": "Seq Scan",
      "Relation Name": "events",
      "Plan Rows": 200000,
      "Actual Startup Time": 0.5,
      "Actual Total Time": 50.0,
      "Actual Rows": 200000,
      "Actual Loops": 1
    }]
  },
  "Planning Time": 0.4,
  "Execution Time": 950.0
}]`

func TestAnalyze_SortSpill(t *testing.T) {
	fs := Analyze(parse(t, sortSpillPlan))
	found := false
	for _, f := range fs {
		if f.Severity == Warning && strings.Contains(f.Message, "spilled to disk") {
			found = true
		}
	}
	if !found {
		t.Errorf("no sort-spill warning in %v", fs)
	}
}

const nestedLoopPlan = `[{
  "Plan": {
    "Node Type": "Nested Loop",
    "Join Type": "Inner",
    "Plan Rows": 1000000,
    "Actual Startup Time": 0.5,
    "Actual Total Time": 2000.0,
    "Actual Rows": 1000000,
    "Actual Loops": 1,
    "Plans": [
      {
        "Node Type": "Seq Scan",
        "Relation Name": "a",
        "Plan Rows": 1000,
        "Actual Startup Time": 0.1,
        "Actual Total Time": 5.0,
        "Actual Rows": 1000,
        "Actual Loops": 1
      },
      {
        "Node Type": "Index Scan",
        "Relation Name": "b",
        "Index Name": "b_a_id_idx",
        "Plan Rows": 1000,
        "Actual Startup Time": 0.01,
        "Actual Total Time": 1.5,
        "Actual Rows": 1000,
        "Actual Loops": 1000
      }
    ]
  },
  "Planning Time": 0.6,
  "Execution Time": 2010.0
}]`

func TestAnalyze_NestedLoop(t *testing.T) {
	fs := Analyze(parse(t, nestedLoopPlan))
	found := false
	for _, f := range fs {
		if f.Severity == Warning && strings.Contains(f.Message, "nested loop") {
			found = true
		}
	}
	if !found {
		t.Errorf("no nested-loop warning in %v", fs)
	}
}

const lossyBitmapPlan = `[{
  "Plan": {
    "Node Type": "Bitmap Heap Scan",
    "Relation Name": "logs",
    "Plan Rows": 50000,
    "Actual Startup Time": 5.0,
    "Actual Total Time": 300.0,
    "Actual Rows": 50000,
    "Actual Loops": 1,
    "Exact Heap Blocks": 100,
    "Lossy Heap Blocks": 850
  },
  "Planning Time": 0.4,
  "Execution Time": 310.0
}]`

func TestAnalyze_LossyBitmap(t *testing.T) {
	fs := Analyze(parse(t, lossyBitmapPlan))
	found := false
	for _, f := range fs {
		if f.Severity == Warning && strings.Contains(f.Message, "lossy") {
			found = true
		}
	}
	if !found {
		t.Errorf("no lossy-bitmap warning in %v", fs)
	}
}

const cleanPlan = `[{
  "Plan": {
    "Node Type": "Index Scan",
    "Relation Name": "users",
    "Index Name": "users_pkey",
    "Plan Rows": 1,
    "Actual Startup Time": 0.02,
    "Actual Total Time": 0.05,
    "Actual Rows": 1,
    "Actual Loops": 1,
    "Shared Hit Blocks": 4
  },
  "Planning Time": 0.1,
  "Execution Time": 0.08
}]`

func TestAnalyze_CleanPlan(t *testing.T) {
	fs := Analyze(parse(t, cleanPlan))
	for _, f := range fs {
		if f.Severity == Warning {
			t.Errorf("unexpected warning on clean plan: %s", f.Message)
		}
	}
}

func TestAnalyze_SlowNodeNote(t *testing.T) {
	// A single index scan eating ~all of a 500ms query gets the time note.
	doc := `[{
	  "Plan": {
	    "Node Type": "Index Scan",
	    "Relation Name": "big",
	    "Index Name": "big_idx",
	    "Plan Rows": 400000,
	    "Actual Startup Time": 1.0,
	    "Actual Total Time": 480.0,
	    "Actual Rows": 400000,
	    "Actual Loops": 1
	  },
	  "Planning Time": 0.5,
	  "Execution Time": 500.0
	}]`
	fs := Analyze(parse(t, doc))
	found := false
	for _, f := range fs {
		if f.Severity == Note && strings.Contains(f.Message, "execution time") {
			found = true
		}
	}
	if !found {
		t.Errorf("no slow-node note in %v", fs)
	}
}

func hasWarning(fs []Finding, substr string) bool {
	for _, f := range fs {
		if f.Severity == Warning && strings.Contains(f.Message, substr) {
			return true
		}
	}
	return false
}

func hasNoteFor(fs []Finding, nodeType string) bool {
	for _, f := range fs {
		if f.Severity == Note && f.NodeType == nodeType && strings.Contains(f.Message, "execution time") {
			return true
		}
	}
	return false
}

func TestParse_Empty(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "empty string", in: ""},
		{name: "whitespace only", in: "  \n\t "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.in))
			if err == nil {
				t.Fatal("expected error for empty input")
			}
			if !strings.Contains(err.Error(), "empty input") {
				t.Errorf("error = %q, want it to mention empty input", err.Error())
			}
		})
	}
}

func TestAnalyze_Nil(t *testing.T) {
	if got := Analyze(nil); got != nil {
		t.Errorf("Analyze(nil) = %v, want nil", got)
	}
	if got := Analyze(&Result{}); got != nil {
		t.Errorf("Analyze(empty result) = %v, want nil", got)
	}
	if got := Analyze(&Result{Plan: nil}); got != nil {
		t.Errorf("Analyze(nil plan) = %v, want nil", got)
	}
}

func TestAnalyze_MisestimateZeroActual(t *testing.T) {
	tests := []struct {
		name     string
		planRows float64
		wantWarn bool
	}{
		{name: "planned 10000 got 0 warns", planRows: 10000, wantWarn: true},
		{name: "planned 10 got 0 warns at threshold", planRows: 10, wantWarn: true},
		{name: "planned 5 got 0 does not warn", planRows: 5, wantWarn: false},
		{name: "planned 1 got 0 does not warn", planRows: 1, wantWarn: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := fmt.Sprintf(`[{
			  "Plan": {
			    "Node Type": "Index Scan",
			    "Relation Name": "users",
			    "Index Name": "users_email_idx",
			    "Plan Rows": %v,
			    "Actual Startup Time": 0.1,
			    "Actual Total Time": 5.0,
			    "Actual Rows": 0,
			    "Actual Loops": 1
			  },
			  "Planning Time": 0.2,
			  "Execution Time": 6.0
			}]`, tt.planRows)
			fs := Analyze(parse(t, doc))
			if got := hasWarning(fs, "misestimated"); got != tt.wantWarn {
				t.Errorf("misestimate warning = %v, want %v, findings %v", got, tt.wantWarn, fs)
			}
		})
	}
}

func TestAnalyze_MisestimateZeroActualFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/misestimate_zero.json")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Parse(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("Parse fixture: %v", err)
	}
	fs := Analyze(res)
	if !hasWarning(fs, "misestimated") {
		t.Errorf("no misestimate warning in fixture findings %v", fs)
	}
}

func TestExclusiveMs_LoopScaling(t *testing.T) {
	parent := &Node{
		NodeType:        "Hash Join",
		ActualTotalTime: 1000,
		ActualLoops:     1,
		Plans: []*Node{
			{
				NodeType:        "Index Scan",
				RelationName:    "inner_table",
				ActualTotalTime: 0.9,
				ActualLoops:     1000,
			},
		},
	}
	if got := exclusiveMs(parent); got != 100 {
		t.Errorf("exclusiveMs(parent) = %v, want 100", got)
	}
	if got := exclusiveMs(parent.Plans[0]); got != 900 {
		t.Errorf("exclusiveMs(child) = %v, want 900", got)
	}
}

func TestAnalyze_SlowNodeLoopScaling(t *testing.T) {
	// The old math subtracted the child per-loop time (0.9 ms) from the
	// parent per-loop time (1000 ms) and flagged the parent at 999.1 ms.
	// The child ran 1000 loops, so its total is 900 ms. The parent own
	// time is 100 ms. The note must land on the child, not the parent.
	doc := `[{
	  "Plan": {
	    "Node Type": "Hash Join",
	    "Plan Rows": 1000,
	    "Actual Startup Time": 1.0,
	    "Actual Total Time": 1000.0,
	    "Actual Rows": 1000,
	    "Actual Loops": 1,
	    "Plans": [{
	      "Node Type": "Index Scan",
	      "Relation Name": "inner_table",
	      "Index Name": "inner_table_idx",
	      "Plan Rows": 1,
	      "Actual Startup Time": 0.01,
	      "Actual Total Time": 0.9,
	      "Actual Rows": 1,
	      "Actual Loops": 1000
	    }]
	  },
	  "Planning Time": 0.5,
	  "Execution Time": 1000.0
	}]`
	fs := Analyze(parse(t, doc))
	if hasNoteFor(fs, "Hash Join") {
		t.Errorf("parent Hash Join got a slow note, it should not: %v", fs)
	}
	if !hasNoteFor(fs, "Index Scan") {
		t.Errorf("child Index Scan did not get a slow note: %v", fs)
	}
}

func TestAnalyze_HashSpill(t *testing.T) {
	data, err := os.ReadFile("testdata/hash_spill.json")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Parse(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("Parse fixture: %v", err)
	}
	fs := Analyze(res)
	if !hasWarning(fs, "hash used 8 batches") {
		t.Errorf("no hash spill warning in %v", fs)
	}
	for _, f := range fs {
		if strings.Contains(f.Message, "hash used") && !strings.Contains(f.Detail, "work_mem") {
			t.Errorf("hash detail should mention work_mem: %q", f.Detail)
		}
	}
}

func TestAnalyze_HashSpillPeakOnly(t *testing.T) {
	doc := `[{
	  "Plan": {
	    "Node Type": "Hash Join",
	    "Plan Rows": 100,
	    "Actual Total Time": 50.0,
	    "Actual Rows": 100,
	    "Actual Loops": 1,
	    "Plans": [{
	      "Node Type": "Hash",
	      "Plan Rows": 100,
	      "Actual Total Time": 40.0,
	      "Actual Rows": 100,
	      "Actual Loops": 1,
	      "Hash Batches": 4,
	      "Original Hash Batches": 4,
	      "Peak Memory Usage": 512
	    }]
	  },
	  "Execution Time": 55.0
	}]`
	fs := Analyze(parse(t, doc))
	if !hasWarning(fs, "hash used 4 batches") {
		t.Errorf("no hash warning for peak memory with multiple batches in %v", fs)
	}
}

func TestAnalyze_HashSingleBatchClean(t *testing.T) {
	doc := `[{
	  "Plan": {
	    "Node Type": "Hash Join",
	    "Plan Rows": 100,
	    "Actual Total Time": 5.0,
	    "Actual Rows": 100,
	    "Actual Loops": 1,
	    "Plans": [{
	      "Node Type": "Hash",
	      "Plan Rows": 100,
	      "Actual Total Time": 4.0,
	      "Actual Rows": 100,
	      "Actual Loops": 1,
	      "Hash Batches": 1,
	      "Original Hash Batches": 1,
	      "Peak Memory Usage": 256
	    }]
	  },
	  "Execution Time": 6.0
	}]`
	fs := Analyze(parse(t, doc))
	if hasWarning(fs, "hash used") {
		t.Errorf("unexpected hash warning in %v", fs)
	}
}

func TestAnalyze_FilterRemoved(t *testing.T) {
	data, err := os.ReadFile("testdata/filter_removed.json")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Parse(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("Parse fixture: %v", err)
	}
	fs := Analyze(res)
	if !hasWarning(fs, "removed 50,000 rows by filter") {
		t.Errorf("no filter warning in %v", fs)
	}
	for _, f := range fs {
		if strings.Contains(f.Message, "removed") && !strings.Contains(f.Detail, "index should do") {
			t.Errorf("filter detail should say the filter is doing work an index should do: %q", f.Detail)
		}
	}
}

func TestAnalyze_FilterRemovedThresholds(t *testing.T) {
	tests := []struct {
		name     string
		removed  float64
		returned float64
		wantWarn bool
	}{
		{name: "removed 50000 returned 100 warns", removed: 50000, returned: 100, wantWarn: true},
		{name: "removed 50000 returned 10000 does not warn on ratio", removed: 50000, returned: 10000, wantWarn: false},
		{name: "removed 5000 returned 10 does not warn on count", removed: 5000, returned: 10, wantWarn: false},
		{name: "removed 10000 returned 0 warns", removed: 10000, returned: 0, wantWarn: true},
		{name: "removed 9999 returned 0 does not warn", removed: 9999, returned: 0, wantWarn: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := fmt.Sprintf(`[{
			  "Plan": {
			    "Node Type": "Seq Scan",
			    "Relation Name": "events",
			    "Plan Rows": %v,
			    "Actual Total Time": 5.0,
			    "Actual Rows": %v,
			    "Actual Loops": 1,
			    "Rows Removed by Filter": %v
			  },
			  "Execution Time": 6.0
			}]`, tt.returned, tt.returned, tt.removed)
			// Seq scan rows stay below the seq scan threshold in these cases
			// except the returned 10000 case, which is checked only for the
			// filter warning.
			fs := Analyze(parse(t, doc))
			if got := hasWarning(fs, "rows by filter"); got != tt.wantWarn {
				t.Errorf("filter warning = %v, want %v, findings %v", got, tt.wantWarn, fs)
			}
		})
	}
}

func ExampleAnalyze() {
	doc := `[{
	  "Plan": {
	    "Node Type": "Seq Scan",
	    "Relation Name": "orders",
	    "Plan Rows": 20000,
	    "Actual Total Time": 10.0,
	    "Actual Rows": 20000,
	    "Actual Loops": 1
	  },
	  "Execution Time": 12.0
	}]`
	res, err := Parse(strings.NewReader(doc))
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, f := range Analyze(res) {
		fmt.Printf("[%s] %s\n", f.Severity, f.Message)
	}
	// Output:
	// [WARN] sequential scan on orders touched 20,000 rows
}

func TestAnalyze_BufferNote(t *testing.T) {
	doc := `[{
	  "Plan": {
	    "Node Type": "Index Scan",
	    "Relation Name": "users",
	    "Index Name": "users_pkey",
	    "Plan Rows": 1,
	    "Actual Total Time": 1.0,
	    "Actual Rows": 1,
	    "Actual Loops": 1,
	    "Shared Hit Blocks": 12000
	  },
	  "Execution Time": 2.0
	}]`
	fs := Analyze(parse(t, doc))
	found := false
	for _, f := range fs {
		if f.Severity == Note && strings.Contains(f.Message, "plan touched 12,000 shared buffers") {
			found = true
		}
	}
	if !found {
		t.Errorf("no buffer note in %v", fs)
	}
}
