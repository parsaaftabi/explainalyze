// Command explainalyze reads PostgreSQL EXPLAIN (ANALYZE, BUFFERS,
// FORMAT JSON) output and prints what is wrong with the plan.
//
//	explainalyze plan.json
//	psql -c "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) SELECT ..." | explainalyze
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/parsaaftabi/explainalyze"
)

const version = "0.2.0"

func main() {
	defaults := explainalyze.DefaultOptions()
	var (
		file               = flag.String("f", "", "plan JSON file (default: stdin)")
		seqScanRows        = flag.Float64("seq-scan-rows", defaults.SeqScanRows, "warn on seq scans touching at least this many rows")
		misestimate        = flag.Float64("misestimate-ratio", defaults.MisestimateRatio, "warn when actual/estimated rows differ by this factor")
		slowNodeMs         = flag.Float64("slow-node-ms", defaults.SlowNodeMs, "note nodes whose exclusive time is at least this many ms")
		slowNodeFraction   = flag.Float64("slow-node-fraction", defaults.SlowNodeFraction, "note nodes using at least this fraction of total time")
		bufferBlocks       = flag.Float64("buffer-blocks", defaults.BufferBlocks, "note plans touching at least this many shared buffers")
		nestedLoopRows     = flag.Float64("nested-loop-rows", defaults.NestedLoopRows, "warn when a nested loop inner side produces at least this many total rows")
		hashBatches        = flag.Int("hash-batches", defaults.HashBatches, "warn when a hash uses more than this many batches")
		filterRemovedRows  = flag.Float64("filter-removed-rows", defaults.FilterRemovedRows, "warn when a filter removes at least this many rows")
		filterRemovedRatio = flag.Float64("filter-removed-ratio", defaults.FilterRemovedRatio, "warn when removed rows are at least this many times returned rows")
		strict             = flag.Bool("strict", false, "exit 1 if any WARN finding is present")
		showVersion        = flag.Bool("version", false, "print version and exit")
		jsonOut            = flag.Bool("json", false, "emit findings as JSON")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("explainalyze %s\n", version)
		return
	}

	in := os.Stdin
	if *file != "" {
		f, err := os.Open(*file)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		in = f
	} else if len(flag.Args()) > 0 {
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		in = f
	}

	res, err := explainalyze.Parse(in)
	if err != nil {
		fatal(err)
	}

	findings := explainalyze.Analyze(res, func(o *explainalyze.Options) {
		o.SeqScanRows = *seqScanRows
		o.MisestimateRatio = *misestimate
		o.SlowNodeMs = *slowNodeMs
		o.SlowNodeFraction = *slowNodeFraction
		o.BufferBlocks = *bufferBlocks
		o.NestedLoopRows = *nestedLoopRows
		o.HashBatches = *hashBatches
		o.FilterRemovedRows = *filterRemovedRows
		o.FilterRemovedRatio = *filterRemovedRatio
	})

	hasWarn := false
	for _, f := range findings {
		if f.Severity == explainalyze.Warning {
			hasWarn = true
			break
		}
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if findings == nil {
			fmt.Println("[]")
		} else if err := enc.Encode(findings); err != nil {
			fatal(err)
		}
		if *strict && hasWarn {
			os.Exit(1)
		}
		return
	}

	fmt.Printf("Execution: %.1f ms · Planning: %.1f ms\n\n", res.ExecutionTime, res.PlanningTime)
	if len(findings) == 0 {
		fmt.Println("OK: no problems found.")
		return
	}
	for _, f := range findings {
		fmt.Printf("[%s] %s\n", f.Severity, f.Message)
		if f.Detail != "" {
			fmt.Printf("       %s\n", f.Detail)
		}
	}
	if *strict && hasWarn {
		os.Exit(1)
	}
}

func fatal(err error) {
	msg := strings.TrimPrefix(err.Error(), "explainalyze: ")
	fmt.Fprintln(os.Stderr, "explainalyze:", msg)
	os.Exit(1)
}
