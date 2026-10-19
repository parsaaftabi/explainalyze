# explainalyze

[![CI](https://github.com/parsaaftabi/explainalyze/actions/workflows/ci.yml/badge.svg)](https://github.com/parsaaftabi/explainalyze/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/parsaaftabi/explainalyze.svg)](https://pkg.go.dev/github.com/parsaaftabi/explainalyze)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Read a PostgreSQL `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plan and get the problems that matter — the same ones from ["PostgreSQL EXPLAIN ANALYZE: What I Actually Look At in Query Plans"](https://dev.to/parsaaftabi). As a library or a CLI.

```bash
go install github.com/parsaaftabi/explainalyze/cmd/explainalyze@latest

psql -c "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) SELECT ..." | explainalyze
```

```
Execution: 2010.0 ms · Planning: 0.6 ms

[WARN] nested loop executed its inner side 1,000 times (1,000,000 total rows)
       Nested loops scale with the outer row count. A hash or merge join is usually cheaper here; check join selectivity and work_mem.
[WARN] sequential scan on a touched 95,000 rows
       ...
```

## What it flags

| Check | Severity | Default threshold |
|---|---|---|
| Sequential scan over real data | WARN | ≥ 10,000 rows |
| Planner row misestimate | WARN | ≥ 10x off either way |
| Sort spilling to disk | WARN | `Sort Method: external ...` |
| Lossy bitmap heap scan | WARN | any lossy pages |
| Runaway nested loop | WARN | inner side ≥ 100,000 total rows |
| Node eating most of the time | NOTE | ≥ 100 ms exclusive and ≥ 50% of total |
| Heavy buffer usage | NOTE | ≥ 10,000 shared buffers |

Every threshold is tunable via `Options` in the library and flags in the CLI (`--seq-scan-rows`, `--misestimate-ratio`, `--json` for machine-readable output).

## Library

```go
res, err := explainalyze.Parse(reader)
for _, f := range explainalyze.Analyze(res) {
    fmt.Printf("[%s] %s\n", f.Severity, f.Message)
}
```

## Notes

- `FORMAT JSON` is required; text-format plans are not parsed.
- `BUFFERS` is optional but recommended — without it, buffer findings are skipped.
- Findings are heuristics over the plan, not a substitute for understanding your schema. A seq scan on a 12-row lookup table will never warn; a misestimate warning always deserves a look at `ANALYZE` / `CREATE STATISTICS`.

## License

MIT — see [LICENSE](LICENSE).
