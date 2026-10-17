# 002 — Capacity tuning: results

- **Spec:** [spec.md](spec.md) · **Tasks:** [tasks.md](tasks.md)

## Method

`perf/k6/online.js` with the defaults unless stated: `RAMP 1m`, `HOLD 3m`, think time 5–15 s (≈ VUS/10
req/s at the plateau), `USERS 10`, `PURCHASES 30`. k6 runs from Docker with `--network host` on a 12-core
host, so container CPU quotas, not the host, are the limit.

Collected per run: k6 summary (CRUD requests, error rate, p90/p95/max), `docker stats` every ~2 s (CPU
average and p90 over the plateau only, i.e. after setup + ramp; peak memory over the whole run), open
connections in the nginx container (`netstat`), PostgreSQL client connections (`pg_stat_activity`) and new
sessions (`pg_stat_database.sessions`), nginx error log, `OOMKilled`.

CPU % is of one core: the quotas are nginx 20%, each app 35%, PostgreSQL 60%.

Business rule (spec): error rate < 1%, CRUD p95 < 500 ms, no connection refusals, no OOM.

## Step 0 — baseline (current configuration)

nginx: 1 worker, `worker_connections 1024`, `keepalive_timeout 65s`. PostgreSQL: `max_connections 20`,
0.60 CPU. App pools: 5 open / 2 idle (write and read).

| VUS | CRUD reqs | Errors | p95 | nginx conns (max) | PG CPU avg / p90 | App CPU avg (each) | PG conns (max) |
|----:|----------:|-------:|----:|------------------:|-----------------:|-------------------:|---------------:|
| 300 | 6,470 | 0.00% | 6.9 ms | — | — ¹ | — ¹ | 11 |
| 1000 | 21,594 | 0.005% (1 EOF) | 5.1 ms | — | 9.3% / 16.0% | 7.3% | 12 |
| 1500 | 31,977 | 0.20% (EOF) | 342 ms | **969** | 47.0% / 62.2% | 13.8% | **20** |
| 2000 | 42,247 | 0.47% (EOF) | 470 ms | **986** | **59.2% / 61.8%** | 17.8% | **20** |

¹ First run sampled CPU including setup (bcrypt); the sampler was fixed to the plateau window afterwards.

Memory peaks: apps ≤ 37 MiB (limit 100), nginx ≤ 13 MiB (32), PostgreSQL ≤ 103 MiB (250). No OOM kill,
no restart.

**Findings**

1. **nginx connection cap.** Open connections stop at ~970–990, just under `worker_connections 1024`.
   Beyond that nginx closes idle keep-alive connections to accept new ones, and clients that reuse a
   closed connection get `EOF` (all failures were `EOF`, none reached the app). Queueing for a free
   connection raises p95 from ~5 ms to 340–470 ms. The nginx warning ("worker_connections are not enough,
   reusing connections") is not logged because the default `error_log` level is `error`.
2. **PostgreSQL CPU saturates at ~200 req/s** (2000 VUS: 59% of a 60% quota), much earlier than expected
   for indexed queries. Suspected cause: **connection churn**. The pools allow 5 open but keep only 2 idle,
   so under load extra connections are closed and reopened; each new session costs a backend fork plus
   `scram-sha-256` authentication. Cumulative counters showed 10,555 sessions for ~309k transactions.
   Both pools reached 20 connections — the whole of `max_connections`.
3. The apps are far from their quota (≤ 18% of 35%).

**Max online users meeting the business rule cleanly: ~1000** (1 connection per user). At 1500–2000 the
thresholds still pass, but only because errors stay under 1%; latency is 70–90× worse and errors grow.

## Step 1 — nginx connections

Change: `worker_processes 1`, `worker_rlimit_nofile 8192`, `error_log ... warn`, `worker_connections 4096`,
`keepalive_timeout 15s`. Everything else as in step 0.

Applying it needed `docker compose up -d --no-deps --force-recreate nginx`: `nginx.conf` is a single-file
bind mount, and an editor that saves by replacing the file leaves the container on the old inode, so
`nginx -s reload` silently keeps the old config.

| VUS | CRUD reqs | Errors | p95 | nginx conns (max) | PG CPU avg / p90 | App CPU avg (each) | PG conns (max) | New PG sessions |
|----:|----------:|-------:|----:|------------------:|-----------------:|-------------------:|---------------:|----------------:|
| 1500 | 32,104 | 0.10% (500) | 296 ms | 1549 | 49.8% / 61.3% | 13.8% | 20 | 3,988 |
| 2000 | 42,432 | 0.43% (500, 3 EOF) | 367 ms | 2072 | 57.4% / 61.5% | 16.4% | 20 | 3,650 |

Memory peaks: nginx ≤ 16 MiB (32). No OOM kill.

**Findings**

1. **nginx cap removed (AC-3):** connections now follow the number of users (1549, 2072); no more `EOF`
   from connection reuse at 1500.
2. **Errors moved to PostgreSQL:** all remaining failures are HTTP 500 with
   `FATAL: sorry, too many clients already`. The two pools hold all 20 slots of `max_connections`; any
   extra session (here the test's own `psql` sampler, in real life an operator or a backup) makes an app
   connection attempt fail. → T4.
3. **Churn confirmed:** ~3,650–4,000 new PostgreSQL sessions per 4-minute run (~1 per 10 requests).
   PostgreSQL CPU is still at its quota at 2000 VUS, so p95 stays high. → T5.
4. 3 `EOF` at 2000 VUS: the client reused a connection exactly as nginx closed it after 15 s idle (the
   test's max think time is also 15 s). Rare (0.007%); watched in the next steps.

## Step 2 — PostgreSQL connection headroom

Change: PostgreSQL `max_connections 25` (was 20). Applied with `docker-compose up -d --no-deps postgres`.

| VUS | CRUD reqs | Errors | p95 | nginx conns (max) | PG CPU avg / p90 | App CPU avg (each) | PG conns (max) | New PG sessions |
|----:|----------:|-------:|----:|------------------:|-----------------:|-------------------:|---------------:|----------------:|
| 1500 | 32,059 | 0.01% (4 EOF) | 283 ms | 1548 | 51.3% / 61.2% | 13.8% | 18 | 3,959 |
| 2000 | 42,406 | 0.01% (4 EOF) | 477 ms | 2076 | 57.8% / 61.6% | 16.9% | 23 | 3,422 |

Memory peak PostgreSQL ≤ 92 MiB (250). No OOM kill.

**Findings**

1. **No more `too many clients` (AC-4):** zero after the restart (all 75 occurrences in the app logs
   predate it). Peak 23 client connections = 20 from the pools + the test's own sessions, with 2+ slots
   still free.
2. Error rate fell from 0.10–0.43% to 0.01%. The remaining failures are only `EOF` on `POST`/`PUT`/
   `DELETE`: the keep-alive close race. The client transparently retries idempotent `GET`s on a reused
   connection but not the others — which is also correct for the API (`POST /purchases` is not
   idempotent).
3. PostgreSQL CPU still at its quota at 2000 VUS (p95 477 ms) and churn unchanged (~3,400–4,000 new
   sessions per run). → T5.

## Step 3 — app pool idle connections

Change: `DATABASE_{WRITE,READ}_MAX_IDLE_CONNS 5` (was 2, from `.env`), declared in the `environment` of
`app1`/`app2` in `docker-compose.yml` next to `MAX_OPEN_CONNS 5`, so the values tied to
`max_connections` are versioned in one file. Applied with `docker-compose up -d --no-deps app1 app2`.

| VUS | CRUD reqs | Errors | p95 | nginx conns (max) | PG CPU avg / p90 | App CPU avg (each) | PG conns (max) | New PG sessions |
|----:|----------:|-------:|----:|------------------:|-----------------:|-------------------:|---------------:|----------------:|
| 1500 | 32,249 | 0.01% (4 EOF) | 66 ms | 1527 | 14.1% / 22.1% | 10.6% | 23 | 68 |
| 2000 | 43,044 | 0.00% (2 EOF) | 76 ms | 2031 | 17.3% / 25.5% | 12.5% | 23 | 51 |
| 3500 | 75,148 | 0.02% (14 EOF) | 153 ms | 3566 | 30.0% / 39.0% | 20.0% | 23 | 50 |

Memory peaks: nginx 21.8 MiB at 3500 (limit 32), apps ≤ 34 MiB, PostgreSQL ≤ 104 MiB. No OOM kill, no
error in the app logs.

**Findings**

1. **Churn removed (AC-7):** new sessions per run fell from ~3,400–4,000 to ~50 (startup of the pools
   plus the test's own sessions).
2. **PostgreSQL CPU at 2000 VUS: 57.8% → 17.3%**, p95 477 ms → 76 ms. Most of PostgreSQL's CPU was spent
   forking backends and running `scram-sha-256` authentication, not on queries.
3. At 3500 VUS (~350 req/s) every container is below its quota: PostgreSQL 30% of 60%, apps 20% of 35%,
   nginx 12% of 20%.
4. **Next limit is nginx again:** 3566 of 4096 `worker_connections`, and memory grows ~4 KiB per open
   connection (≈ 29 MiB at 4096, against a 32 MiB limit). Raising it means changing a resource limit (T6).
5. The keep-alive `EOF` race stays at ≤ 0.02%.

## Step 4 — nginx capacity, same total budget (T6)

Change: nginx `worker_connections 16384`, `worker_rlimit_nofile 32768`, `mem_limit` 32m → 64m; PostgreSQL
`mem_limit` 250m → 218m (it never used more than ~106 MiB). Total memory and all CPU quotas unchanged.

| VUS | CRUD reqs | Errors | p95 | nginx conns (max) | nginx mem (peak) | PG CPU avg / p90 | App CPU avg / p90 (each) | App mem (peak) |
|----:|----------:|-------:|----:|------------------:|-----------------:|-----------------:|-------------------------:|---------------:|
| 4500 | 96,321 | 0.02% (EOF) | 239 ms | 4710 | 33.5 MiB | 41.5% / 53.4% | 25.2% / 28.1% | 38 MiB |
| 5000 | 106,022 | 0.01% (EOF) | **717 ms** | 5506 | 40.5 MiB | 51.0% / 61.9% | 28.0% / 32.0% | 54 MiB |
| 5500 | 113,405 | 0.03% (EOF + 16× 500) | **1571 ms** | 6621 | 56.5 MiB | 49.4% / 62.1% | 26.5% / 34.5% | 75 MiB |

No OOM kill. The 500s at 5500 are `context deadline exceeded` on queries (saturation, not a config
error).

**Findings**

1. **Max online users meeting the rule: ~4500** (p95 239 ms). At 5000 p95 breaks 500 ms; at 5500 queries
   start timing out.
2. **The stack is now balanced:** at the knee every container is at 80–90% of its CPU quota (nginx 16.5 of
   20, apps 28 of 35, PostgreSQL 51 of 60 with p90 at the quota). There is no idle budget left to move;
   PostgreSQL reaches its quota first (p90), the apps right after.
3. At 4500 nginx used 33.5 MiB: with the old 32 MiB limit it would have been OOM-killed.
4. Past the knee, memory grows with queued work (nginx 56 MiB, apps 75 MiB at 5500), so the new limits
   also give room for overload instead of OOM kills.
5. **Spare PostgreSQL slots in use:** during these runs DBeaver on the host held 5 connections — exactly
   the headroom from step 2. The apps kept their 20 and got no `too many clients`; only the test's own
   `psql` sampler was refused (so the "new sessions" counter is invalid for these runs and is omitted).

## Summary

| Step | Max VUS meeting the rule | p95 at 2000 VUS | Errors at 2000 VUS | PG CPU at 2000 VUS |
|------|-------------------------:|----------------:|-------------------:|-------------------:|
| 0 baseline | ~1000 | 470 ms | 0.47% | 59.2% (quota 60%) |
| 1 nginx connections | ~1000 (errors moved to PG) | 367 ms | 0.43% | 57.4% |
| 2 `max_connections 25` | ~1500 | 477 ms | 0.01% | 57.8% |
| 3 idle pool = open | ≥ 3500 (bounded by nginx) | 76 ms | 0.00% | 17.3% |
| 4 nginx memory/connections | **~4500** (CPU-bound, all containers) | — | — | — |

At ~4500 online users the API serves ~450 req/s with p95 ≈ 240 ms. Beyond this point the configuration
has no spare resource; more capacity needs lower cost per request (code) or a larger budget.

One VU = one connection, acting every 5–15 s. Browsers may open more than one connection per user, so
for browser clients read these as upper bounds.
