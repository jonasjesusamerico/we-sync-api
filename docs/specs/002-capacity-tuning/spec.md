# 002 — Capacity tuning for concurrent online users

- **Status:** Done
- **Author:** jonasjesusamerico
- **Date:** 2026-10-16

## Context

A configuration review of `docker-compose.yml` and `nginx.conf` estimated that the stack supports roughly
300 users actively using the API at the same time, and that the first limit hit is not CPU but nginx
connections:

- nginx runs a single worker (`worker_processes` is not set) with `worker_connections 1024`. Each
  in-flight request uses two connections (client + upstream), and clients keep idle connections open
  for `keepalive_timeout 65s`. Above ~1000 open connections nginx refuses new clients even with CPU to
  spare.
- PostgreSQL has `max_connections 20`, exactly the sum of both app pools (2 × (5 write + 5 read)). There is
  no free slot for an operator session, a backup, or a third instance.
- PostgreSQL (0.60 CPU) is expected to be the next bottleneck once connections are no longer the limit.

The estimate was not measured: the existing `throughput.js` uses few persistent connections and no think
time, so it cannot reproduce a connection limit.

Login is out of scope: users do not log in at the same time.

## Goals

- Measure how many concurrent online users the stack supports, with a reproducible test.
- Remove the nginx connection limit as the first bottleneck.
- Leave connection headroom in PostgreSQL.
- Record the measured result of each change, so every step is justified by numbers.

## Non-goals

- Application code changes (handlers, queries, pools).
- Load on login/register (bcrypt-bound by design).
- TLS / HTTP/2 termination, horizontal scaling beyond `app1`/`app2`.
- Changing CPU/memory limits without explicit approval (see open questions).

## Functional requirements

### Business rules

- An "online user" holds one keep-alive connection and acts every few seconds (think time), with the
  CRUD mix of `perf/k6/lib/workload.js`.
- A configuration is acceptable for N online users when, during a sustained window: error rate < 1%,
  CRUD p95 < 500 ms, no nginx `worker_connections are not enough`, no container OOM kill.

### Ownership and authorization

Unchanged. No security control (headers, timeouts, body limit, auth) is weakened.

## Acceptance criteria

- **AC-1:** A k6 script simulates N online users with think time and reports error rate, p95 and
  requests per second; N and think time are configurable.
- **AC-2:** A baseline with the current configuration is recorded (max N that meets the business rule and
  the first bottleneck observed).
- **AC-3:** After the nginx change, the stack serves more online users than the baseline under the same
  test, with no nginx connection errors.
- **AC-4:** With both app pools full, PostgreSQL still has at least 5 free connection slots.
- **AC-5:** Memory of every container stays under its `mem_limit` during the tests (no OOM kill).
- **AC-6:** Each step's result (configuration, N, rps, p95, errors, bottleneck) is recorded in
  [results.md](results.md).
- **AC-7:** Under steady load, the apps do not keep opening new PostgreSQL sessions (new sessions per run
  close to the pool size, not proportional to requests).

## Open questions

- [x] May resource limits change? Resolved 2026-10-16: yes, moving budget between services while keeping
      the total; CPU quotas were not changed (they were already balanced at the knee).

## Changelog

- 2026-10-16 — Created.
- 2026-10-16 — Added AC-7 (connection churn) after the baseline showed PostgreSQL CPU saturating at
  ~200 req/s with ~1 new session per 30 transactions.
- 2026-10-16 — Open question resolved: memory moved from PostgreSQL to nginx, same total budget.
