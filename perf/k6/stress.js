// Stress test: N VUs online at once, each signed in as its own account
// (VU i ↔ k6-load-i@example.com), instead of sharing a few accounts like
// online.js. Accounts and their refresh tokens come from seed.sql, which must
// run with the same KEY right before each run (refresh tokens work once).
//
// Each VU "opens the app" on its first iteration (refresh, global categories
// and payment methods, first page of purchases), then runs the same CRUD mix
// and think time as online.js, refreshing its access token every 10 minutes.
// No setup and no login: VU state stays per VU, so memory does not grow with
// N × N, and bcrypt is not what gets measured.
//
//   KEY        key passed to seed.sql                (required)
//   VUS        online users at the plateau           (default 4500, ≤ seeded accounts)
//   RAMP       time to reach VUS                     (default 2m)
//   HOLD       time at the plateau                   (default 5m)
//   THINK_MIN  min seconds between actions           (default 5)
//   THINK_MAX  max seconds between actions           (default 15)
import exec from 'k6/execution';
import { sleep } from 'k6';
import * as api from './lib/api.js';
import { crudAction } from './lib/workload.js';

const KEY = __ENV.KEY;
const VUS = Number(__ENV.VUS || 4500);
const THINK_MIN = Number(__ENV.THINK_MIN || 5);
const THINK_MAX = Number(__ENV.THINK_MAX || 15);
// Access tokens last 15m (JWT_ACCESS_TOKEN_DURATION); refresh well before.
const REFRESH_EVERY_MS = 10 * 60 * 1000;

if (!KEY) {
  throw new Error('KEY is required: use the key passed to seed.sql');
}

export const options = {
  scenarios: {
    stress: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: __ENV.RAMP || '2m', target: VUS },
        { duration: __ENV.HOLD || '5m', target: VUS },
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

// Module state is per VU: this VU's session.
let session = null;

function globalIds(token, resource) {
  return (api.listOwned(token, resource) || []).filter((it) => it.is_global).map((it) => it.id);
}

function openApp() {
  const tokens = api.refresh(`k6-${KEY}-${exec.vu.idInTest}`);
  if (!tokens) {
    // Retried on the next iteration. A 401 on every VU means a wrong KEY,
    // seed.sql not re-run, or fewer seeded accounts than VUs.
    return null;
  }
  const user = { accessToken: tokens.access_token, purchaseIds: [] };
  const refs = {
    categoryIds: globalIds(user.accessToken, 'categories'),
    paymentMethodIds: globalIds(user.accessToken, 'payment-methods'),
  };
  const page = api.listPurchases(user.accessToken, '?page=1&page_size=100');
  user.purchaseIds = page ? page.items.map((p) => p.id) : [];
  return { user, refs, refreshToken: tokens.refresh_token, refreshedAt: Date.now() };
}

function keepSignedIn() {
  if (Date.now() - session.refreshedAt < REFRESH_EVERY_MS) {
    return;
  }
  const tokens = api.refresh(session.refreshToken);
  if (tokens) {
    session.user.accessToken = tokens.access_token;
    session.refreshToken = tokens.refresh_token;
    session.refreshedAt = Date.now();
  }
}

export default function () {
  if (session === null) {
    session = openApp();
  } else {
    keepSignedIn();
    crudAction(session.user, session.refs);
  }
  sleep(THINK_MIN + Math.random() * (THINK_MAX - THINK_MIN));
}
