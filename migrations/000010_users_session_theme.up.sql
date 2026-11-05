-- Session version (feature 006, ADR-01): bumped on every password change. Access tokens carry it as the
-- "sv" claim and the JWT middleware compares it on every request. Tokens issued before this migration
-- have no "sv" and count as 0, the default, so the deploy logs nobody out.
ALTER TABLE users ADD COLUMN session_version BIGINT NOT NULL DEFAULT 0;

-- Theme saved in the account (feature 006, ADR-05). NULL = never chosen: the browser choice is kept.
ALTER TABLE users ADD COLUMN theme TEXT;
ALTER TABLE users ADD CONSTRAINT users_theme_check CHECK (theme IN ('LIGHT', 'DARK'));
