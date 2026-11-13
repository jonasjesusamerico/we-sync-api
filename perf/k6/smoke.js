// Smoke test: one user walks through every endpoint once. Run it before the
// heavier tests to confirm the stack is up and the scripts match the API.
import { check, fail } from 'k6';
import * as api from './lib/api.js';
import { prepareUsers, randomPurchase } from './lib/data.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1'],
    'http_req_duration{name:GET /purchases/overview}': ['p(95)<500'],
  },
};

export function setup() {
  return prepareUsers(1, 3);
}

export default function (data) {
  const user = data.users[0];

  // Fresh session: login → refresh rotates the token → logout revokes it.
  const session = api.login(user.email, data.password) || fail('login failed');
  const rotated = api.refresh(session.refresh_token) || fail('refresh failed');
  check(rotated, { 'refresh returns a new refresh token': (t) => t.refresh_token !== session.refresh_token });
  api.logout(rotated.refresh_token);

  const token = user.accessToken;

  for (const resource of ['categories', 'payment-methods']) {
    const items = api.listOwned(token, resource);
    check(items, { [`${resource} include globals`]: (list) => list && list.some((it) => it.is_global) });
    const own = api.createOwned(token, resource, `k6 ${resource} ${Date.now()}`);
    if (own) {
      // Names stay within the 40-character limit: "k6 payment-methods edit <ms>" has 37.
      api.updateOwned(token, resource, own.id, `k6 ${resource} edit ${Date.now()}`);
      api.archiveOwned(token, resource, own.id);
    }
  }

  const purchase = api.createPurchase(token, randomPurchase(data.refs)) || fail('create purchase failed');
  api.getPurchase(token, purchase.id);
  const [year, month] = purchase.purchase_date.split('-');
  const page = api.listPurchases(token, `?year=${year}&month=${Number(month)}`);
  check(page, { 'month filter returns the new purchase': (p) => p && p.items.some((it) => it.id === purchase.id) });
  const overview = api.monthOverview(token, `?year=${year}&month=${Number(month)}&today=${purchase.purchase_date}`);
  check(overview, {
    'overview has one bar per day': (o) => o && o.daily.length === o.days_in_month,
    'overview counts the new purchase': (o) => o && (purchase.status === 'CANCELED' || o.active_count > 0),
  });
  api.updatePurchase(token, purchase.id, randomPurchase(data.refs));
  api.deletePurchase(token, purchase.id);
}
