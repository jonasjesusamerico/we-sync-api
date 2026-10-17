# 003 — Lower cost per request: results

- **Spec:** [spec.md](spec.md) · **Plan:** [plan.md](plan.md) · **Tasks:** [tasks.md](tasks.md)

## Method

As described in [plan.md](plan.md#measurement-procedure): `perf/k6/online.js` with the 002 defaults, host-side
collection from cgroup v2 and `/proc`, plateau = last 170 s of the hold. CPU % is of one core: quotas are
nginx 20%, each app 35%, PostgreSQL 60%. "Throttled" = share of CFS periods (100 ms) in which the container
used up its quota and was stopped until the next period. "PSI" = share of wall time in which at least one
task of the container was runnable but waiting for CPU (`cpu.pressure`, `some`).

### Test host

| Item | Value |
|------|-------|
| CPU | Intel Core i5-12450H, **hybrid**: 4 P-cores with HT (CPUs 0–7, up to 4.4 GHz) + 4 E-cores (CPUs 8–11, up to 3.3 GHz); no `cpuset` pinning |
| Governor | `intel_pstate` `powersave`, turbo enabled |
| Power | **On battery** (discharging, 30%) during every run below — see limitations |
| Memory | 15.7 GB, swap in use by the host (1.4 GB); no container swapped during the runs (`memory.swap.current` = 0) |
| Path | k6 → `docker-proxy` (userland, host) → nginx → app → PostgreSQL |

CFS quotas are time, not work: the same request costs more quota on an E-core, on a P-core whose HT
sibling is busy (k6, `docker-proxy`), or at a lower frequency (battery). Absolute "CPU ms per request"
values below are only comparable between runs on the same power state.

## T1 — Baseline (002 configuration, unchanged)

Configuration: as left by 002 (`docker-compose.yml`, `nginx.conf` uncommitted changes of 002). Table
`purchase_entries` before the runs: ~102k rows; each run adds ~15–16k.

| VUS | CRUD reqs (plateau) | req/s | Errors (plateau) | p50 | p90 | p95 | p99 | p95 whole run | Max |
|----:|--------------------:|------:|-----------------:|----:|----:|----:|----:|--------------:|----:|
| 4500 | 76,805 | 451.8 | 0 | 2.5 ms | 10.8 ms | 46.3 ms | 122 ms | 38.5 ms | 699 ms |
| 5000 | 84,999 | 500.0 | 1 (0.001%, EOF) | 2.7 ms | 88.1 ms | **268.5 ms** | **1,109 ms** | 224.2 ms | 4,036 ms |

CPU over the plateau (cgroup; `docker stats` in parentheses) and saturation signals:

| VUS | Container | CPU avg | CPU p90 (2 s) | Throttled periods | PSI some | Mem peak (`docker stats`) |
|----:|-----------|--------:|--------------:|------------------:|---------:|--------------------------:|
| 4500 | nginx | 15.0% (15.5) | 16.0% (16.7) | 3.6% | 0.2% | 31.8 MiB |
| 4500 | app1 / app2 | 24.6% / 24.6% | 26.4% / 26.3% | 3.8% / 3.5% | 0.4% | 38.5 / 38.8 MiB |
| 4500 | PostgreSQL | 39.3% (40.7) | 48.6% (53.0) | **17.0%** | **6.3%** | 104.4 MiB |
| 5000 | nginx | 16.4% (16.9) | 17.1% (18.0) | 5.5% | 0.2% | 40.2 MiB |
| 5000 | app1 / app2 | 27.0% / 27.1% | 28.2% / 28.2% | 5.9% / 6.1% | 0.4% | 52.8 / 53.5 MiB |
| 5000 | PostgreSQL | 44.8% (46.2) | 54.1% (61.3) | **27.7%** | **11.7%** | 112.5 MiB |

| VUS | nginx client conns (max) | nginx → app conns (max) | PG client conns (max) | New PG sessions (− collector) | OOM / restarts | nginx `warn` | app `ERROR` | 5xx |
|----:|-------------------------:|------------------------:|----------------------:|------------------------------:|---------------:|-------------:|------------:|----:|
| 4500 | 4,500 | 87 | 22 | 54 | 0 / 0 | 0 | 0 | 0 |
| 5000 | 5,007 | **516** | 20 | 44 | 0 / 0 | 0 | 0 | 0 |

New sessions (~50) are the pools reopening after `CONN_MAX_IDLE_TIME` (5 min) between runs, not churn.

**Both levels meet the business rule** (5000: p95 268 ms whole-run 224 ms, errors 0.001%). This differs from
002 (5000: p95 717 ms); the host power state and the pre-run `VACUUM`/`CHECKPOINT` are two known differences.

### Cost per request (plateau, CPU ms per CRUD request)

| VUS | nginx | apps (sum) | PostgreSQL |
|----:|------:|-----------:|-----------:|
| 4500 | 0.33 | 1.09 | 0.87 |
| 5000 | 0.33 | 1.08 | 0.90 |

Time series (20 s buckets) show that **only PostgreSQL's cost grows during the run**: at 5000 VUS from
0.78 to 1.03 ms/request, while apps (1.07–1.11) and nginx (0.32–0.33) stay flat. In the last minute PostgreSQL
is throttled in 40–57% of the periods, and nginx opens up to 516 upstream connections (keep-alive pool is 32):
when the app answers slower, more requests are in flight than idle upstream connections, so nginx opens
(and later closes) extra ones.

**Findings**

1. **PostgreSQL is the first bottleneck**, in latency terms: its average CPU (45%) is below its quota (60%),
   but demand comes in bursts (up to 20 backends running at once on 12 host CPUs), so the quota of a 100 ms
   period is often used up early and every backend waits for the next period (throttled 28% of periods at
   5000). This is why the average utilization alone hides the saturation.
2. The app and nginx are not throttled meaningfully (≤ 6% of periods, PSI ≤ 0.4%).
3. The knee is between 4500 (p95 46 ms) and 5000 (p95 268 ms, p99 1.1 s).

## T2 — Cost attribution (diagnosis, no change)

### App (CPU profile)

Two 25 s CPU profiles of app1 at 4500 VUS (separate run; app1 24.7% vs app2 24.6% CPU, so the profiler's
overhead is not measurable). A 60 s profile returns empty: the server `WriteTimeout` (30 s) cuts it.

| Mechanism | Share of app CPU | Notes |
|-----------|-----------------:|-------|
| Response JSON encoding (`httputil.WriteJSON`) | 15.5% | `encoding/json` (v2 backend), `decimal.MarshalJSON`, time formatting |
| `pgx` / `database/sql` (round trips, decode) | ~25% | includes socket `read`/`write` syscalls |
| JWT parse (`jwt.ParseWithClaims`) | 8.9% | 7.0% is `ParseUnverified` (base64 + JSON decode of claims); HMAC verify 1.2% |
| `otelhttp` own work (span, metrics) | 7.2% | metric recording 3.0% |
| Request logging middleware | ~4–5% | `slog` JSON + `With`; `write` to stdout 1.4% |
| Request body JSON decode | ~6% | create/update |
| HTTP server read/write | ~11% | `readRequest` 4.6%, `finishRequest` 5.9% |
| GC background mark | 2.5% | plus assists inside `mallocgc` (7.7% total allocation path) |
| Scheduler / netpoll | ~7% | `GOMAXPROCS=1` |

Runtime metrics over 60 s (app1): 225 req/s, **1.12 ms CPU, 44 KB and 589 allocations per request**,
46 GC cycles/min (one every ~1.3 s), heap in use ~26 MB, `next_gc` ~28 MB, 41 goroutines.

### PostgreSQL

Index statistics over a 4500-VUS run: `idx_purchase_entries_user_date` read **13.0 M index tuples in 89 k
scans (~146 per scan) with 13.0 M heap fetches**, i.e. ~94% of all tuples returned by the database.

`EXPLAIN (ANALYZE, BUFFERS)` and backend CPU measured from the host (`/proc/<pid>/stat`), single session,
idle host, user with ~1,300 rows:

| Statement | CPU per execution | Note |
|-----------|------------------:|------|
| `COUNT(*)` of all user rows, visibility map not set (as during a run) | 0.58 ms | Index Only Scan, **Heap Fetches = rows** (1,289 buffers) |
| same after `VACUUM` (all pages all-visible) | 0.19 ms | 0 heap fetches, 15 buffers |
| `COUNT(*)` of one month | 0.063 → 0.042 ms | before → after `VACUUM` |
| List data query, no filter (`LIMIT $2 OFFSET $3`), prepared, plan cache `auto` | 0.61 ms | **custom plan on every execution** (0 of 10 generic) |
| same, `plan_cache_mode = force_generic_plan` | 0.27 ms | planning ≈ 0.34 ms per execution |
| `SELECT 1` | 0.02 ms | per-statement floor |

The month list, count and lookup statements switch to a generic plan after 5 executions; only the
unfiltered list keeps re-planning its 3-table join.

**Findings**

1. Rows inserted during a run are on pages not marked all-visible (autovacuum's insert threshold is
   `1000 + 0.2 × reltuples` ≈ 27k rows, more than a run inserts), so the "index-only" `COUNT(*)` reads the heap
   for every row; its cost grows with the user's row count, which is what makes PostgreSQL's cost per request
   rise during the run.
2. The unfiltered list re-plans on every execution (~0.34 ms of CPU each time).
3. These two explain only part of PostgreSQL's cost: a model from the per-statement costs above predicts
   ~0.3 ms per request, the load test measures ~0.9 ms. **Unexplained 3× gap** — candidates: per-statement
   protocol/transaction overhead under concurrency, contention between backends, and CPU-time inflation on
   the host under load (E-cores, busy HT siblings, battery power limits). Needs per-statement statistics
   (`pg_stat_statements`) and a per-process CPU breakdown before choosing the PostgreSQL experiment.

## Limitations of the measurements so far

- **Battery power:** all runs on battery; to be repeated on AC power before any comparison with experiments.
- Single run per level; run-to-run variability not yet measured.
- `pg_stat_statements` not available (needs a PostgreSQL restart with `shared_preload_libraries`; not done).
- Latency percentiles of the "whole run" view include the ramp, as in 002; the plateau view is the reference
  for 003.
