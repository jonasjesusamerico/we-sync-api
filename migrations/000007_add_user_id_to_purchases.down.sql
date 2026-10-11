DROP INDEX IF EXISTS idx_purchase_entries_user_id;
ALTER TABLE purchase_entries DROP COLUMN IF EXISTS user_id;
