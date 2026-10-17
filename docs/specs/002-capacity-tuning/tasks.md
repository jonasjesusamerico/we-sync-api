# 002 — Capacity tuning: tasks

- **Spec:** [spec.md](spec.md)
- **Plan:** [plan.md](plan.md)

One change per task. After each change: restart what it affects, run the same test, record the result
in [results.md](results.md) before moving to the next task.

- [x] T1 — `perf/k6/online.js` + README section; validate with a small N. (AC-1)
- [x] T2 — Baseline: ramp N on the current config, find the max N and the bottleneck. (AC-2, AC-5, AC-6)
- [x] T3 — `nginx.conf`: `worker_processes 1`, `worker_connections 4096`, `keepalive_timeout 15s`;
      reload nginx and repeat T2's test. (AC-3, AC-5, AC-6)
- [x] T4 — `docker-compose.yml`: PostgreSQL `max_connections 25`; recreate postgres, check free slots
      under load and repeat the test. (AC-4, AC-5, AC-6)
- [x] T5 — App pools: `DATABASE_{WRITE,READ}_MAX_IDLE_CONNS` = `MAX_OPEN_CONNS` (5) to stop connection
      churn; measure new sessions and PostgreSQL CPU before/after. (AC-7, AC-6)
- [x] T6 — nginx `worker_connections 16384`, `worker_rlimit_nofile 32768`, `mem_limit` 64m, paid by
      PostgreSQL `mem_limit` 250m → 218m (same total budget, approved); find the new max at 4500/5000/5500.
      (AC-3, AC-5, AC-6)
- [x] T7 — Run `gofmt -l internal cmd`, `go vet ./...`, `go build ./...`, `go test ./...`

## Notes

Decisions taken during implementation that deviate from the plan.

- T3 also sets `worker_rlimit_nofile 8192` (the container's soft limit of 1024 descriptors would cap
  connections below 4096) and `error_log ... warn` (the connection-exhaustion warning was invisible at
  the default `error` level).
- T3: `nginx -s reload` does not pick up an edited `nginx.conf` when the editor replaces the file (single-
  file bind mount keeps the old inode); the container must be recreated. README updated.
- T5 was not in the original plan: it came from the baseline (AC-7 added to the spec). The pool sizes moved
  from `.env` (not versioned) to the app `environment` in `docker-compose.yml`, which overrides `.env`.
