# 003 — Lower cost per request (capacity within the same budget)

- **Status:** In progress
- **Author:** jonasjesusamerico
- **Date:** 2026-10-16

## Context

Spec [002](../002-capacity-tuning/results.md) removed the configuration limits (nginx connections, PostgreSQL
`max_connections`, connection churn) and reached **~4,500 online users** (p95 239 ms, ~450 req/s). At that
point every container runs at 80–90% of its CPU quota: PostgreSQL reaches its quota first (p90), the apps
right after, nginx close behind. Its conclusion: more capacity needs a lower cost per request or a larger
budget.

The budget is fixed: nginx 0.20 CPU / 64 MiB, each app 0.35 CPU / 100 MiB (`GOMAXPROCS=1`,
`GOMEMLIMIT=80MiB`), PostgreSQL 0.60 CPU / 218 MiB — 1.50 CPU and 482 MiB in total.

In a closed system with think time, the sustainable request rate is bounded by the scarcest resource:
`X_max = min_i(quota_i / cost_i)`, where `cost_i` is the CPU seconds tier `i` spends per request. With a fixed
budget, capacity only grows by lowering `cost_i` of the bottleneck tier, or by moving budget from a tier
with spare capacity to the bottleneck tier.

## Goals

- Re-measure the 002 configuration (baseline) at 4,500 and 5,000 online users with a richer, host-side
  collection (CPU throttling, CPU pressure, plateau-only latency percentiles).
- Find where each tier spends CPU per request (profiles, query statistics), before changing anything.
- Validate, one change at a time, changes that lower the cost per request or raise usable capacity, and
  keep only those with a measured gain.
- Record every experiment (kept, discarded or inconclusive) with its mechanism and evidence.

## Non-goals

- Raising the total CPU or memory budget (only with explicit approval, per experiment).
- Changing PostgreSQL durability (`fsync`, `synchronous_commit`, `full_page_writes`).
- Load on login/register (bcrypt-bound by design).
- New infrastructure (cache, queues, connection pooler, HTTP/2/TLS).
- Weakening security, isolation or observability without an explicit, recorded decision.

## Functional requirements

### Business rules

- Workload: `perf/k6/online.js` with the defaults of 002 (ramp 1 min, hold 3 min, think time 5–15 s,
  `USERS 10`, `PURCHASES 30`), one keep-alive connection per VU.
- A configuration is **approved** for N online users when, during the plateau: error rate < 1%, CRUD p95
  < 500 ms, no connection refusals, no container OOM-killed.
- API behavior and contract do not change. A change that alters responses, logs or telemetry content must
  say so in [results.md](results.md) and be approved before it is kept.

### Ownership and authorization

Unchanged. No security control is weakened; `pprof` is used only locally and never through nginx.

## Acceptance criteria

- **AC-1:** Baseline at 4,500 and 5,000 VUS recorded with the collection described in [plan.md](plan.md),
  before any change.
- **AC-2:** CPU cost per request is attributed per tier (nginx, app, PostgreSQL) and, inside the app, per
  function/mechanism from a CPU profile taken under load.
- **AC-3:** Each experiment changes one variable, is run with the same test as the baseline, and is recorded
  with: hypothesis, mechanism, prediction, result, decision (kept / discarded / inconclusive).
- **AC-4:** The total CPU and memory budget is unchanged in every kept configuration.
- **AC-5:** The final configuration is approved (business rule) at a VUS level ≥ the re-measured baseline,
  with the highest approved level, the highest tested level and the observed saturation point reported
  separately.
- **AC-6:** Code changes keep `gofmt`, `go vet`, `go build` and `go test` passing.

## Open questions

- [ ] `ENABLE_PPROF=true` in the local `.env` exposes `/debug/pprof` through nginx on `0.0.0.0:8080`.
      Block `/debug/` in nginx, bind the port to `127.0.0.1`, or turn it off between profiling sessions?

## Changelog

- 2026-10-16 — Created from the capacity mission brief (approved by the author as the requirement).
