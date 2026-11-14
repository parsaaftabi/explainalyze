# Changelog

## v0.2.0

- Lower the Go requirement in `go.mod` from 1.27.2 to 1.24. CI now runs Go 1.24 and Go 1.27.
- Fix the planner misestimate check for 0 actual rows. Planned 10,000 rows and 0 actual rows now warns. Planned 5 rows and 0 actual rows does not. The check does not divide by zero.
- Fix exclusive time. Postgres reports per-loop averages. The old code subtracted child per-loop time from parent per-loop time and then scaled by the parent loop count. When a child ran 1,000 loops and the parent ran 1 loop, the parent got blamed for child time. The code now multiplies each node time by its own loop count first, then subtracts.
- Add a hash spill check. A hash node with `Hash Batches` above 1 warns when the batch count grew past `Original Hash Batches`, or when `Peak Memory Usage` is set with multiple batches. The node struct now reads `Hash Batches`, `Original Hash Batches`, and `Peak Memory Usage`.
- Add a filter check. A node with `Rows Removed by Filter` at 10,000 or more, and at least 10 times the returned rows, warns. The detail says the filter is doing work an index should do. The node struct now reads `Rows Removed by Filter`.
- `Analyze` returns nil for a nil result or a result with no plan. It does not panic.
- `Parse` rejects empty input with an error that says the input is empty.
- CLI flags now cover all thresholds. New flags are `--slow-node-ms`, `--slow-node-fraction`, `--buffer-blocks`, `--nested-loop-rows`, `--hash-batches`, `--filter-removed-rows`, and `--filter-removed-ratio`.
- Add `--strict` to the CLI. It exits with code 1 when any WARN finding is present.
- Add `--version` to the CLI. It prints `explainalyze 0.2.0`.
- Add tests for the misestimate zero case, loop scaling, hash spill, filter removal, nil input, and empty input. Add plan files under `testdata/` and an `ExampleAnalyze` function.

## v0.1.0

- First release.
- Parse PostgreSQL `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` output, array form and single object form.
- Checks for sequential scans, planner misestimates, disk sorts, lossy bitmap heap scans, runaway nested loops, slow nodes, and heavy buffer use.
- CLI reads a file argument, `-f`, or stdin. It supports `--seq-scan-rows`, `--misestimate-ratio`, and `--json`.
