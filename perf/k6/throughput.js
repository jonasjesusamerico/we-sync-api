// Throughput test (JMeter-style thread group): a fixed number of VUs fire
// requests back to back, with no think time, for a fixed duration. The answer
// is in the summary: `http_reqs` = how many requests the API served in that
// time (and per second), with latency and error rate alongside.
//
//   VUS        concurrent clients ("threads")     (default 50)
//   DURATION   measured window                    (default 1m)
//   USERS      distinct accounts shared by VUs    (default 10)
//   PURCHASES  seeded purchases per account       (default 30)
//
// Setup (users + seed data) runs before the window and is not counted in it.
// More VUs than the API can serve does not raise the total, only latency:
// try a few values (e.g. 10, 50, 100) and keep the one with the most requests.
import { prepareUsers } from './lib/data.js';
import { crudAction } from './lib/workload.js';

export const options = {
  setupTimeout: '5m',
  scenarios: {
    throughput: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || 50),
      duration: __ENV.DURATION || '1m',
      gracefulStop: '5s',
    },
  },
  thresholds: {
    'http_req_failed{kind:crud}': ['rate<0.01'],
    // Breaks down the total per endpoint in the summary.
    'http_reqs{kind:crud}': ['count>0'],
  },
};

export function setup() {
  return prepareUsers(Number(__ENV.USERS || 10), Number(__ENV.PURCHASES || 30));
}

export default function (data) {
  crudAction(data);
}
