-- Stress test accounts for stress.js. LOCAL TEST DATABASES ONLY.
-- Requires migration 000008 (users.name NOT NULL).
--
-- Creates accounts k6-load-1@example.com .. k6-load-<n>@example.com with
-- <purchases> purchases each, and gives account i the refresh token
-- "k6-<key>-<i>" (stored as its SHA-256, like the API does). The accounts
-- cannot log in with a password: password_hash is not a bcrypt hash.
--
-- Accounts are created through SQL because the API costs one bcrypt per
-- register (~50 min for 4500 accounts on the compose limits).
--
-- Re-run it, with a fresh key, before every stress run: the API rotates refresh
-- tokens, so each one works once. Users and purchases are only created when
-- missing; old refresh tokens of these accounts are replaced.
--
--   KEY=$(openssl rand -hex 16)
--   docker exec -i we-sync-postgres psql -U postgres -d finance -v n=4500 -v key=$KEY < perf/k6/seed.sql

\set ON_ERROR_STOP on

\if :{?purchases}
\else
  \set purchases 30
\endif

\if :{?n}
\else
  DO $$ BEGIN RAISE EXCEPTION 'missing -v n=<accounts>'; END $$;
\endif

\if :{?key}
\else
  DO $$ BEGIN RAISE EXCEPTION 'missing -v key=<random key>, e.g. $(openssl rand -hex 16)'; END $$;
\endif

BEGIN;

INSERT INTO users (name, email, password_hash)
SELECT format('k6 load %s', i), format('k6-load-%s@example.com', i), '!no-password-login'
FROM generate_series(1, :n) AS i
ON CONFLICT (email) DO NOTHING;

-- Only accounts without purchases get them, so re-runs do not grow the data.
WITH refs AS (
    SELECT
        (SELECT array_agg(id) FROM categories WHERE user_id IS NULL)      AS categories,
        (SELECT array_agg(id) FROM payment_methods WHERE user_id IS NULL) AS payment_methods
),
empty_users AS (
    SELECT u.id
    FROM users u
    WHERE u.email LIKE 'k6-load-%@example.com'
      AND NOT EXISTS (SELECT 1 FROM purchase_entries p WHERE p.user_id = u.id)
)
INSERT INTO purchase_entries (user_id, category_id, payment_method_id, purchase_date, description, amount, status)
SELECT
    u.id,
    refs.categories[1 + floor(random() * cardinality(refs.categories))::int],
    refs.payment_methods[1 + floor(random() * cardinality(refs.payment_methods))::int],
    CURRENT_DATE - floor(random() * 365)::int,
    format('k6 purchase %s', s),
    round((1 + random() * 499)::numeric, 2),
    (ARRAY['PENDING', 'PAID', 'CANCELED'])[1 + floor(random() * 3)::int]
FROM empty_users u
CROSS JOIN refs
CROSS JOIN generate_series(1, :purchases) AS s;

DELETE FROM refresh_tokens rt
USING users u
WHERE rt.user_id = u.id
  AND u.email LIKE 'k6-load-%@example.com';

INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
SELECT u.id, encode(sha256(convert_to(format('k6-%s-%s', :'key', i), 'UTF8')), 'hex'), NOW() + INTERVAL '1 day'
FROM generate_series(1, :n) AS i
JOIN users u ON u.email = format('k6-load-%s@example.com', i);

COMMIT;

ANALYZE users, purchase_entries, refresh_tokens;

SELECT
    (SELECT count(*) FROM users WHERE email LIKE 'k6-load-%@example.com') AS accounts,
    (SELECT count(*) FROM purchase_entries p JOIN users u ON u.id = p.user_id
      WHERE u.email LIKE 'k6-load-%@example.com')                         AS purchases,
    (SELECT count(*) FROM refresh_tokens rt JOIN users u ON u.id = rt.user_id
      WHERE u.email LIKE 'k6-load-%@example.com' AND NOT rt.revoked)      AS refresh_tokens;
