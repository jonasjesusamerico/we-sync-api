-- Migration 000007 assigned legacy purchases to a synthetic account because
-- ownership could not be determined. Keep those records, but do not expose
-- them as belonging to an arbitrary user.
ALTER TABLE purchase_entries
    ALTER COLUMN user_id DROP NOT NULL;

UPDATE purchase_entries
SET user_id = NULL
WHERE user_id = (
    SELECT id FROM users WHERE email = 'system@we-sync-api.local'
);

DELETE FROM users
WHERE email = 'system@we-sync-api.local'
  AND NOT EXISTS (
      SELECT 1 FROM purchase_entries WHERE purchase_entries.user_id = users.id
  );
