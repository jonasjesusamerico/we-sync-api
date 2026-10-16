// The traffic mix of a logged-in user of a personal finance app: mostly reads
// (current month first), a steady share of new entries, few edits/deletes.
// Login is deliberately not part of it: it is bcrypt-bound (cost 12) and
// would measure password hashing, not the API. Weights are percentages.
import exec from 'k6/execution';
import * as api from './api.js';
import { pick, randomDate, randomPurchase } from './data.js';

const MIX = [
  ['listMonth', 30],
  ['listAll', 15],
  ['getById', 15],
  ['categories', 8],
  ['paymentMethods', 7],
  ['create', 15],
  ['update', 7],
  ['delete', 3],
];
const TOTAL = MIX.reduce((sum, [, w]) => sum + w, 0);

// Module state is per VU: purchases this VU created and may delete, so a
// delete never removes a seeded purchase that other VUs still read.
const created = [];

function chooseAction() {
  let r = Math.random() * TOTAL;
  for (const [action, weight] of MIX) {
    r -= weight;
    if (r < 0) {
      return action;
    }
  }
  return MIX[0][0];
}

// Users are shared round-robin between VUs (setup tokens, read-only here).
export function userForVU(data) {
  return data.users[(exec.vu.idInTest - 1) % data.users.length];
}

export function crudAction(data) {
  const user = userForVU(data);
  const token = user.accessToken;

  switch (chooseAction()) {
    case 'listMonth': {
      const [year, month] = randomDate().split('-');
      api.listPurchases(token, `?year=${year}&month=${Number(month)}&page=1&page_size=20`);
      break;
    }
    case 'listAll':
      api.listPurchases(token, `?page=${1 + Math.floor(Math.random() * 3)}&page_size=20`);
      break;
    case 'getById':
      api.getPurchase(token, pick(user.purchaseIds));
      break;
    case 'categories':
      api.listOwned(token, 'categories');
      break;
    case 'paymentMethods':
      api.listOwned(token, 'payment-methods');
      break;
    case 'create': {
      const purchase = api.createPurchase(token, randomPurchase(data.refs));
      if (purchase) {
        created.push(purchase.id);
      }
      break;
    }
    case 'update':
      api.updatePurchase(token, pick(user.purchaseIds), randomPurchase(data.refs));
      break;
    case 'delete':
      if (created.length > 0) {
        api.deletePurchase(token, created.pop());
      } else {
        api.listPurchases(token, '?page=1&page_size=20');
      }
      break;
  }
}
