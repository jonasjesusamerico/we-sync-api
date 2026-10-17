# 003 — Lower cost per request: plan

- **Spec:** [spec.md](spec.md)
- **Status:** Approved

## Approach

Measure, attribute, then change one thing at a time:

1. **Baseline (AC-1):** the 002 configuration, unchanged, at 4,500 and 5,000 VUS.
2. **Attribution (AC-2):** CPU seconds per request per tier (cgroup `usage_usec` / requests in the plateau),
   a CPU profile of one app under load, and PostgreSQL statement statistics. This is diagnosis, not a change:
   profiling runs are not compared with measurement runs.
3. **Experiments (AC-3):** pick the change with the largest expected reduction in cost per request of the
   bottleneck tier, predict which metrics move, apply it alone, repeat the test, keep or revert.

Candidate mechanisms (starting points, ranked only after step 2): per-request logging (JSON encoding, an
unbuffered `write` to stdout per line, Docker `json-file` driver), OpenTelemetry (spans, 10% sampling, HTTP
metrics), JWT verification, JSON encoding, PostgreSQL queries (`COUNT(*)` + `LIMIT/OFFSET` in the list,
two round trips per list), Go GC (`GOGC` with `GOMEMLIMIT`), nginx (gzip, upstream keep-alive), and moving
CPU quota between tiers (same total).

## Measurement procedure

Scripts live outside the repository (session scratch directory); this section is the reproducible recipe.

Before each run: `VACUUM (ANALYZE) purchase_entries` and `CHECKPOINT` (same starting state: fresh statistics,
no dead tuples, no pending checkpoint), then 5 s idle. Data accumulates between runs (each run seeds new
users; the table grows ~16k rows per run); its size is recorded per run.

Load (same as 002, plus a per-request CSV and p99 in the summary — neither changes the load):

```bash
docker run --rm -i --network host -v "$PWD/perf/k6:/scripts:ro" -v <out>:/out grafana/k6:1.0.0 run -q \
  --summary-export /out/summary.json --summary-trend-stats 'avg,min,med,max,p(90),p(95),p(99)' \
  --out csv=/out/k6.csv.gz -e VUS=4500 /scripts/online.js
```

Collection, read from the host so that no collector process runs inside (and is billed to) a container:

| Metric | Source | Interval |
|--------|--------|----------|
| CPU used, throttled periods, throttled time | cgroup v2 `cpu.stat` | 2 s |
| CPU pressure (time with a runnable task waiting for CPU) | cgroup v2 `cpu.pressure` (`some total`) | 2 s |
| Memory, swap | cgroup v2 `memory.current`, `memory.swap.current`; `docker stats` (excludes page cache, as in 002) | 2 s |
| CPU (same tool as 002) | `docker stats --no-stream` | ~2 s |
| Open client connections (nginx), upstream connections, PostgreSQL client connections, accept queue | `/proc/<pid>/net/tcp{,6}` of the container's main process | 2 s |
| Backends by state | `pg_stat_activity` (local socket) | 10 s |
| Sessions, transactions, buffer hits/reads, WAL, checkpoints, table size | `pg_stat_database`, `pg_stat_wal`, `pg_stat_checkpointer`, `pg_stat_user_tables` | before/after |
| OOM kills, restarts | `memory.events`, `docker inspect` | before/after |
| nginx warnings, app `ERROR` lines | `docker logs --since <start>` | after |
| Requests, errors (status / error code), p50/p90/p95/p99 per endpoint | k6 per-request CSV | after |

**Plateau window:** the last 170 s of the hold, ending 5 s before the last request (setup with bcrypt and the
1-minute ramp excluded). CPU average = `Δusage_usec / Δt` over the window; p90 over the 2 s samples. Latency
and error rate are reported for the plateau and for the whole run (the whole-run view is what 002 reported).

**Cost per request** of a tier = CPU seconds used in the window / CRUD requests completed in the window.

New PostgreSQL sessions = `Δ pg_stat_database.sessions` − sessions opened by the collector (counted).

## Affected code

Decided per experiment and recorded in [results.md](results.md) and [tasks.md](tasks.md).

## Database

No migration planned. An index or query change is proposed only with `EXPLAIN (ANALYZE, BUFFERS)` evidence.

## API contract changes

None. Swagger regeneration: no.

## Security and data isolation

- `pprof` is read directly from the app container address on the Docker network (host only), never through
  nginx.
- No change to authentication, authorization, timeouts, body limit or headers.
- PostgreSQL durability settings are out of scope.

## Observability

Any change that reduces logs, spans or metrics is an explicit trade-off, recorded with what diagnosis
capability is lost, and kept only with the author's approval.

## Test strategy

| AC | Level | Test |
|----|-------|------|
| AC-1, AC-3, AC-5 | k6 + host collection | Procedure above, same parameters per run |
| AC-2 | pprof / SQL | CPU profile under load; statement statistics |
| AC-4 | docker | `docker inspect` limits before each run |
| AC-6 | go | `gofmt -l`, `go vet`, `go build`, `go test` |

## Risks and alternatives considered

- **k6, `docker-proxy` and the collectors run on the same 12-core host**, outside the quotas: they compete
  for host CPU, not for container quota. `localhost:8080` goes through the userland `docker-proxy`, which
  adds a hop not present in production; kept for comparability with 002.
- **Run-to-run variability:** decisions near the threshold need repeated runs.
- **Profiling overhead:** profiles are taken in separate runs and are not used for capacity numbers.
- **Data growth:** each run adds rows; recorded per run, and runs being compared are taken close in time.
