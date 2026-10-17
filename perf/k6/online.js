// Online users test: N VUs behave like N logged-in users with the app open.
// Each VU runs one CRUD action, then "thinks" for a random time, and keeps its
// keep-alive connection open meanwhile — so N VUs hold N client connections,
// which is what exposes connection limits (throughput.js cannot).
//
//   VUS        online users at the plateau          (default 300)
//   RAMP       time to reach VUS                    (default 1m)
//   HOLD       time at the plateau                  (default 3m)
//   THINK_MIN  min seconds between actions          (default 5)
//   THINK_MAX  max seconds between actions          (default 15)
//   USERS      distinct accounts shared by VUs      (default 10)
//   PURCHASES  seeded purchases per account         (default 30)
//
// Keep RAMP + HOLD under 15m: the access tokens issued in setup expire then.
// Expected load ≈ VUS / average think time requests per second.
import { sleep } from 'k6';
import { prepareUsers } from './lib/data.js';
import { crudAction } from './lib/workload.js';

const THINK_MIN = Number(__ENV.THINK_MIN || 5);
const THINK_MAX = Number(__ENV.THINK_MAX || 15);

export const options = {
  setupTimeout: '5m',
  scenarios: {
    online: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: __ENV.RAMP || '1m', target: Number(__ENV.VUS || 300) },
        { duration: __ENV.HOLD || '3m', target: Number(__ENV.VUS || 300) },
      ],
      gracefulRampDown: '0s',
      gracefulStop: '5s',
    },
  },
  thresholds: {
    'http_req_failed{kind:crud}': ['rate<0.01'],
    'http_req_duration{kind:crud}': ['p(95)<500'],
    'http_reqs{kind:crud}': ['count>0'],
  },
};

export function setup() {
  return prepareUsers(Number(__ENV.USERS || 10), Number(__ENV.PURCHASES || 30));
}

export default function (data) {
  crudAction(data);
  sleep(THINK_MIN + Math.random() * (THINK_MAX - THINK_MIN));
}
