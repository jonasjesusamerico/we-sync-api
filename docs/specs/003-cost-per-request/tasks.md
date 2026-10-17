# 003 — Lower cost per request: tasks

- **Spec:** [spec.md](spec.md)
- **Plan:** [plan.md](plan.md)

One change per experiment. After each change: apply it, verify it is active, run the same test, record the
result in [results.md](results.md) and decide (keep / revert) before the next one.

- [ ] T1 — Baseline at 4,500 and 5,000 VUS on the 002 configuration. (AC-1)
- [ ] T2 — Cost attribution: CPU seconds per request per tier; app CPU profile under load; PostgreSQL
      statement statistics. Rank the candidate mechanisms. (AC-2)
- [ ] T3 — Experiments, one per hypothesis, in the order set by T2. (AC-3, AC-4)
- [ ] T4 — Final configuration at the highest approved VUS level and one level above. (AC-5)
- [ ] T5 — Run `gofmt -l internal cmd`, `go vet ./...`, `go build ./...`, `go test ./...` if code changed. (AC-6)

## Notes

Decisions taken during implementation that deviate from the plan.

- T1 was run on battery power (discharging); it is provisional until repeated on AC power.
- T2: CPU profiles must be ≤ 25 s each (`WriteTimeout` 30 s cuts longer ones); two were merged.
- T2: `pg_stat_statements` needs a PostgreSQL restart with `shared_preload_libraries`; pending approval.
