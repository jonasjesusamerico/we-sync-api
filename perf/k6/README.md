# Performance tests (k6)

Tests against the full `docker compose` stack (nginx → app1/app2 → PostgreSQL), with the CPU and memory
limits from `docker-compose.yml`. They are not part of `go test`.

| Script | Question it answers | Duration |
|---|---|---|
| `smoke.js` | Is the stack up, and do the scripts match the API? (every endpoint once) | seconds |
| `throughput.js` | **How many requests fit in 1 minute?** (N VUs, no think time, JMeter-style) | 1 min + setup |

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
run varies by several percent. `nginx.conf` is a bind mount read at startup, so after changing it run
`docker exec we-sync-nginx nginx -s reload`.
