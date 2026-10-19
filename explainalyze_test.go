package explainalyze

import (
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
