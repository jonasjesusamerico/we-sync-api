# Performance tests (k6)

Tests against the full `docker compose` stack (nginx → app1/app2 → PostgreSQL), with the CPU and memory
limits from `docker-compose.yml`. They are not part of `go test`.

| Script | Question it answers | Duration |
|---|---|---|
| `smoke.js` | Is the stack up, and do the scripts match the API? (every endpoint once) | seconds |
| `throughput.js` | **How many requests fit in 1 minute?** (N VUs, no think time, JMeter-style) | 1 min + setup |
| `online.js` | **How many users can be online at once?** (N VUs with think time, each holding a keep-alive connection) | 4 min + setup |

`lib/` holds the HTTP client (`api.js`), the test data setup (`data.js`) and the traffic mix
(`workload.js`: ~75% reads, 15% creates, 7% updates, 3% deletes).

## Running

k6 runs from its Docker image, so it does not need a local install. `--network host` reaches nginx on
`localhost:8080`. The k6 container has no CPU limit, so it does not take CPU from the API's quota.

```bash
docker compose up --build -d   # or docker-compose, depending on the install

K6="docker run --rm -i --network host -v $PWD/perf/k6:/scripts:ro grafana/k6:1.0.0"

$K6 run /scripts/smoke.js
$K6 run /scripts/throughput.js
$K6 run -e VUS=100 -e DURATION=1m /scripts/throughput.js
```

`throughput.js` variables: `VUS` (default 50), `DURATION` (1m), `USERS` (10), `PURCHASES` per user (30).

`online.js` variables: `VUS` (default 300), `RAMP` (1m), `HOLD` (3m), `THINK_MIN`/`THINK_MAX` seconds
between actions (5/15), `USERS`, `PURCHASES`. Raise `VUS` between runs until the thresholds fail (error
rate ≥ 1% or CRUD p95 ≥ 500 ms). Unlike `throughput.js`, it holds one open connection per VU, so it also
hits connection limits (nginx `worker_connections`), not only CPU. One k6 VU uses one connection; a
browser may open several, so treat the result as an upper bound for browser clients.

```bash
$K6 run -e VUS=1000 /scripts/online.js
```
`BASE_URL` (default `http://localhost:8080`) points both scripts somewhere else.

### Notes

- **Setup runs before the measured window** and is not counted in it. It is slow on purpose: each test
  user costs 2 bcrypt hashes (register + login), and the API runs only one bcrypt at a time per CPU.
- **Logins are not part of the measured mix.** They are bcrypt-bound, so they would measure password
  hashing instead of the API.
- **Data accumulates** (users and purchases from every run). `docker compose down -v` resets the
  database.

## Reading the results

- `http_reqs{kind:crud}` is the **total number of requests served in the window**. The `/s` rate k6
  prints is averaged over the whole run, setup included; divide the count by `DURATION` for the real rate.
- More VUs than the API can serve only raise latency, not the total: try a few values (10, 50, 100) and
  keep the one with the most requests at an acceptable p95.
- **While a test runs**, watch:
  - `docker stats` to see which container is pinned at its CPU limit (the bottleneck);
  - Grafana (`http://localhost:3001`) for request rate, latency and errors per endpoint;
  - CPU throttling (`nr_throttled`, `throttled_usec`). The images are distroless, so read it from the
    host (cgroup v2, systemd driver):
    `cat /sys/fs/cgroup/system.slice/docker-$(docker inspect -f '{{.Id}}' we-sync-api-1).scope/cpu.stat`.

## Comparing before/after a change

Run `throughput.js` with the same variables on both versions, ideally with a fresh database each time
(`docker compose down -v && docker compose up --build -d`), and compare the request count, p95 and
error rate per endpoint (`name` tag) in the end-of-test summary. Repeat each side a few times: a single
run varies by several percent. `nginx.conf` is a single-file bind mount: most editors save by replacing the
file, which leaves the container on the old copy, so `nginx -s reload` keeps the old config. After changing
it, recreate the container: `docker compose up -d --no-deps --force-recreate nginx`, then confirm with
`docker exec we-sync-nginx nginx -T`.
