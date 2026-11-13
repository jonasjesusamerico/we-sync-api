// Thin HTTP client for the we-sync API. Every call is tagged with a stable
// `name` (so URLs with IDs aggregate into one metric) and a `kind`:
//   auth — bcrypt-bound endpoints (login/register), slow by design
//   crud — everything else; this is what the latency thresholds watch
import http from 'k6/http';
import { check } from 'k6';

export const BASE_URL = (__ENV.BASE_URL || 'http://localhost:8080').replace(/\/$/, '') + '/api/v1';

function request(method, path, { token, body, name, kind = 'crud', expect = 200 } = {}) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  const res = http.request(method, BASE_URL + path, body === undefined ? null : JSON.stringify(body), {
    headers,
    tags: { name, kind },
  });
  check(res, { [`${name} → ${expect}`]: (r) => r.status === expect });
  return res;
}

function jsonOrNull(res, expect) {
  return res.status === expect ? res.json() : null;
}

// --- auth -------------------------------------------------------------------

export function register(email, password) {
  return request('POST', '/auth/register', {
    body: { name: 'k6 user', email, password }, name: 'POST /auth/register', kind: 'auth', expect: 201,
  });
}

// Returns { access_token, refresh_token, token_type } or null.
export function login(email, password) {
  return jsonOrNull(request('POST', '/auth/login', {
    body: { email, password }, name: 'POST /auth/login', kind: 'auth',
  }), 200);
}

export function refresh(refreshToken) {
  return jsonOrNull(request('POST', '/auth/refresh', {
    body: { refresh_token: refreshToken }, name: 'POST /auth/refresh',
  }), 200);
}

export function logout(refreshToken) {
  return request('POST', '/auth/logout', {
    body: { refresh_token: refreshToken }, name: 'POST /auth/logout', expect: 204,
  });
}

// --- purchases --------------------------------------------------------------

export function listPurchases(token, query = '') {
  return jsonOrNull(request('GET', `/purchases${query}`, { token, name: 'GET /purchases' }), 200);
}

export function monthOverview(token, query) {
  return jsonOrNull(request('GET', `/purchases/overview${query}`, { token, name: 'GET /purchases/overview' }), 200);
}

export function getPurchase(token, id) {
  return request('GET', `/purchases/${id}`, { token, name: 'GET /purchases/{id}' });
}

export function createPurchase(token, body) {
  return jsonOrNull(request('POST', '/purchases', {
    token, body, name: 'POST /purchases', expect: 201,
  }), 201);
}

export function updatePurchase(token, id, body) {
  return request('PUT', `/purchases/${id}`, { token, body, name: 'PUT /purchases/{id}' });
}

export function deletePurchase(token, id) {
  return request('DELETE', `/purchases/${id}`, { token, name: 'DELETE /purchases/{id}', expect: 204 });
}

// --- categories / payment methods ------------------------------------------
// resource: 'categories' | 'payment-methods'
// Categories require icon and color; payment methods ignore both fields, so one body serves the two.
// Own categories and payment methods are archived, not deleted (feature 006).

export function listOwned(token, resource) {
  return jsonOrNull(request('GET', `/${resource}`, { token, name: `GET /${resource}` }), 200);
}

export function createOwned(token, resource, name) {
  return jsonOrNull(request('POST', `/${resource}`, {
    token, body: { name, icon: 'TAG', color: 'FOOD' }, name: `POST /${resource}`, expect: 201,
  }), 201);
}

export function updateOwned(token, resource, id, name) {
  return request('PUT', `/${resource}/${id}`, {
    token, body: { name, icon: 'TAG', color: 'FOOD' }, name: `PUT /${resource}/{id}`,
  });
}

export function archiveOwned(token, resource, id) {
  return request('PUT', `/${resource}/${id}/archive`, { token, name: `PUT /${resource}/{id}/archive`, expect: 204 });
}
