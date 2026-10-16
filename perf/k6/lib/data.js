// Test data: throwaway users created per run, plus seeded purchases so that
// list queries scan realistic volumes. Nothing here is a real credential:
// e-mails use the reserved example.com domain and the password is random.
import http from 'k6/http';
import { fail } from 'k6';
import { BASE_URL, listOwned } from './api.js';

const BATCH = 10;
const STATUSES = ['PENDING', 'PAID', 'CANCELED'];

export function pick(items) {
  return items[Math.floor(Math.random() * items.length)];
}

// A date within the last `days` days, as "YYYY-MM-DD".
export function randomDate(days = 365) {
  const d = new Date(Date.now() - Math.floor(Math.random() * days) * 86400000);
  return d.toISOString().slice(0, 10);
}

export function randomPurchase(refs) {
  return {
    purchase_date: randomDate(),
    description: `k6 purchase ${Math.floor(Math.random() * 1e6)}`,
    category_id: pick(refs.categoryIds),
    payment_method_id: pick(refs.paymentMethodIds),
    amount: (1 + Math.random() * 500).toFixed(2),
    status: pick(STATUSES),
  };
}

function postBatch(requests, expect, what) {
  const responses = http.batch(requests);
  responses.forEach((r) => {
    if (r.status !== expect) {
      fail(`setup: ${what} returned ${r.status}: ${r.body}`);
    }
  });
  return responses;
}

function jsonRequest(path, body, name, token) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return ['POST', BASE_URL + path, JSON.stringify(body), { headers, tags: { name, kind: 'setup' } }];
}

// Registers and logs in `count` users and seeds `purchasesPerUser` purchases
// for each. Returns { password, users: [{ email, accessToken, refreshToken,
// purchaseIds }], refs: { categoryIds, paymentMethodIds } }.
//
// Register + login cost two bcrypt hashes per user, and the API serializes
// bcrypt per CPU, so setup takes roughly 0.7 s per user on the default limits.
export function prepareUsers(count, purchasesPerUser) {
  const runId = `${Date.now().toString(36)}${Math.floor(Math.random() * 1e6).toString(36)}`;
  const password = `k6-${runId}-pw`;
  const users = [];

  for (let i = 0; i < count; i += BATCH) {
    const emails = [];
    for (let j = i; j < Math.min(i + BATCH, count); j++) {
      emails.push(`k6-${runId}-${j}@example.com`);
    }
    postBatch(emails.map((email) => jsonRequest('/auth/register', { email, password }, 'setup register')), 201, 'register');
    const logins = postBatch(emails.map((email) => jsonRequest('/auth/login', { email, password }, 'setup login')), 200, 'login');
    logins.forEach((r, k) => {
      const tokens = r.json();
      users.push({ email: emails[k], accessToken: tokens.access_token, refreshToken: tokens.refresh_token, purchaseIds: [] });
    });
  }

  // Global categories/payment methods are shared, so one user is enough to read them.
  const globalIds = (resource) => {
    const items = listOwned(users[0].accessToken, resource) || [];
    const ids = items.filter((it) => it.is_global).map((it) => it.id);
    if (ids.length === 0) {
      fail(`setup: no global ${resource} found (are the seed migrations applied?)`);
    }
    return ids;
  };
  const refs = { categoryIds: globalIds('categories'), paymentMethodIds: globalIds('payment-methods') };

  users.forEach((user) => {
    for (let i = 0; i < purchasesPerUser; i += BATCH) {
      const n = Math.min(BATCH, purchasesPerUser - i);
      const requests = [];
      for (let k = 0; k < n; k++) {
        requests.push(jsonRequest('/purchases', randomPurchase(refs), 'setup purchase', user.accessToken));
      }
      postBatch(requests, 201, 'create purchase').forEach((r) => user.purchaseIds.push(r.json().id));
    }
  });

  return { password, users, refs };
}
