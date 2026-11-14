# explainalyze

[![CI](https://github.com/parsaaftabi/explainalyze/actions/workflows/ci.yml/badge.svg)](https://github.com/parsaaftabi/explainalyze/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/parsaaftabi/explainalyze.svg)](https://pkg.go.dev/github.com/parsaaftabi/explainalyze)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Read a PostgreSQL `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plan and get the problems you would look for by hand. Use it as a library or as a CLI.

Requires Go 1.24 or later.

```bash
go install github.com/parsaaftabi/explainalyze/cmd/explainalyze@v0.2.0

psql -c "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) SELECT ..." | explainalyze
```

Sample run. The plan is in `testdata/bad_plan.json` in this repo. A hash join reads `customers`, a seq scan on `events` filters out most of its rows, and the hash needs 8 batches.

```bash
explainalyze testdata/bad_plan.json
```

```
Execution: 960.0 ms · Planning: 0.5 ms

[WARN] Seq Scan on events removed 50,000 rows by filter and returned 100 rows
       The filter is doing work an index should do. Add an index on the filtered columns or rewrite the predicate so an existing index can use it.
[WARN] hash used 8 batches (planned 1), peak memory 1,024 KB
       A hash that needs more than one batch spills to disk. Raise work_mem, cut the row count before the join, or check the join key estimate.
[NOTE] Hash consumed 66.7% of execution time (640.0 ms exclusive)
[WARN] sequential scan on customers touched 200,000 rows
       A seq scan this large usually means a missing index on the filter/join column. Verify with the WHERE clause before adding one.
```

## What it flags

| Check | Severity | Default threshold |
|---|---|---|
| Sequential scan over real data | WARN | 10,000 rows or more |
| Planner row misestimate | WARN | 10x off either way, including planned rows with 0 actual rows |
| Sort spilling to disk | WARN | `Sort Method: external ...` |
| Lossy bitmap heap scan | WARN | any lossy pages |
| Runaway nested loop | WARN | inner side 100,000 total rows or more |
| Hash using more than one batch | WARN | more than 1 batch, or batches above the planned count, or peak memory set with multiple batches |
| Filter removing far more rows than it returns | WARN | 10,000 removed rows or more, and at least 10x the returned rows |
| Node eating most of the time | NOTE | 100 ms exclusive or more, and 50% of total time or more |
| Heavy buffer usage | NOTE | 10,000 shared buffers or more |

Exclusive time uses total time per node. Postgres reports per-loop averages, so the code multiplies each node time by its own loop count before it subtracts child time from parent time.

## CLI

```bash
explainalyze plan.json
explainalyze -f plan.json
psql -c "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) SELECT ..." | explainalyze
explainalyze --strict plan.json
explainalyze --json plan.json
explainalyze --version
```

`--strict` exits with code 1 when any WARN finding is present. It exits with code 0 when the plan has no warnings. Use it in a script when you want a bad plan to fail the step.

`--version` prints `explainalyze 0.2.0`.

All thresholds have flags.

| Flag | Default |
|---|---|
| `--seq-scan-rows` | 10000 |
| `--misestimate-ratio` | 10 |
| `--slow-node-ms` | 100 |
| `--slow-node-fraction` | 0.5 |
| `--buffer-blocks` | 10000 |
| `--nested-loop-rows` | 100000 |
| `--hash-batches` | 1 |
| `--filter-removed-rows` | 10000 |
| `--filter-removed-ratio` | 10 |

## Library

```go
res, err := explainalyze.Parse(reader)
if err != nil {
    // empty input and input without a plan both return an error
}
for _, f := range explainalyze.Analyze(res) {
    fmt.Printf("[%s] %s\n", f.Severity, f.Message)
}
```

`Analyze` returns nil for a nil result or a result with no plan. It does not panic.

## Notes

- `FORMAT JSON` is required. Text format plans are not parsed.
- `BUFFERS` is optional but recommended. Without it, buffer findings are skipped.
- Findings are checks over the plan. They do not replace reading the query and the schema. A seq scan on a small lookup table will not warn. A misestimate warning means you should look at `ANALYZE` and at extended statistics for that table.

## License

MIT. See [LICENSE](LICENSE).
