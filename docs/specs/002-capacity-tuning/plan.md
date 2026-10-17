# 002 — Capacity tuning: plan

- **Spec:** [spec.md](spec.md)
- **Status:** Approved

## Approach

Measure first, then change one thing at a time and measure again with the same test:

1. **Test:** `perf/k6/online.js` — `ramping-vus` up to `VUS`, each VU runs one CRUD action and sleeps a
   random think time (`THINK_MIN`..`THINK_MAX`). Each k6 VU keeps its own keep-alive connection, so N VUs
   hold N client connections, like N browser tabs. Run at increasing N until the business rule breaks.
2. **nginx:** `worker_processes 1` (explicit; the container has 0.20 CPU, more workers add nothing),
   `worker_connections 4096`, `keepalive_timeout 15s` (idle clients release connections sooner).
3. **PostgreSQL:** `max_connections 25` — 20 for the pools + 5 headroom. Memory cost is small
   (~2–3 MB per backend at `work_mem 2MB`).
4. **Only with approval:** CPU limit changes, if PostgreSQL is pinned at its quota.

## Affected code

| Path | Change |
|------|--------|
| `perf/k6/online.js` | New script (AC-1) |
| `perf/k6/README.md` | Document `online.js` |
| `nginx.conf` | Worker and keep-alive settings (AC-3) |
| `docker-compose.yml` | PostgreSQL `max_connections` (AC-4); app pool sizes in `environment` (AC-7) |
| `docs/specs/002-capacity-tuning/results.md` | Measurements per step (AC-6) |

## Database

No migration.

## API contract changes

None. Swagger regeneration: no.

## Security and data isolation

No change. Security headers, `server_tokens off`, proxy timeouts and app timeouts stay as they are.
A shorter `keepalive_timeout` reduces, not increases, exposure to idle-connection exhaustion.

## Observability

No change. During tests: `docker stats` (CPU/memory per container), nginx error log,
`pg_stat_activity` for connection count, k6 summary.

## Test strategy

| AC | Level | Test |
|----|-------|------|
| AC-1 | k6 | `online.js` runs end to end with small N |
| AC-2 | k6 | Baseline ramp on the current config |
| AC-3 | k6 | Same ramp after the nginx change; check nginx log |
| AC-4 | SQL | `SHOW max_connections` and `count(*) FROM pg_stat_activity` under load |
| AC-5 | docker | `docker stats` / `docker inspect` `OOMKilled` after each run |

Go code is not touched; `go build ./...` and `go test ./...` still run once at the end as a regression check.

## Risks and alternatives considered

- **k6 on the same host** competes for host CPU, not for container quotas; the host has 12 cores, so the
  limits measured are the containers'.
- **Access tokens expire in 15 min:** each run stays under 10 min.
- **`worker_processes auto`** was rejected: it starts one worker per host core (12) inside a 0.20 CPU
  quota, wasting memory of a 32 MB container.
- **HTTP/2/TLS in nginx** would multiplex connections, but it changes the deployment; out of scope.
