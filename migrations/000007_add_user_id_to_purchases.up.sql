ALTER TABLE purchase_entries
    ADD COLUMN user_id BIGINT REFERENCES users(id);

INSERT INTO users (email, password_hash)
VALUES ('system@we-sync-api.local', 'system-user-not-for-login')
ON CONFLICT (email) DO NOTHING;

UPDATE purchase_entries
SET user_id = (SELECT id FROM users WHERE email = 'system@we-sync-api.local')
WHERE user_id IS NULL;

ALTER TABLE purchase_entries
    ALTER COLUMN user_id SET NOT NULL;

CREATE INDEX idx_purchase_entries_user_id ON purchase_entries (user_id);
