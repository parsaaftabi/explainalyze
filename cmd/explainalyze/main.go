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

	"github.com/parsaaftabi/explainalyze"
)

func main() {
	var (
		file        = flag.String("f", "", "plan JSON file (default: stdin)")
		seqScanRows = flag.Float64("seq-scan-rows", 10000, "warn on seq scans touching at least this many rows")
		misestimate = flag.Float64("misestimate-ratio", 10, "warn when actual/estimated rows differ by this factor")
		jsonOut     = flag.Bool("json", false, "emit findings as JSON")
	)
	flag.Parse()

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
	})

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(findings); err != nil {
			fatal(err)
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
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "explainalyze:", err)
	os.Exit(1)
}
