-- Display name of the user, required from registration on (feature 004).
ALTER TABLE users ADD COLUMN name TEXT;

-- Existing accounts get the e-mail local part, cut to 60 characters (RN-18).
UPDATE users SET name = left(split_part(email, '@', 1), 60);

ALTER TABLE users ALTER COLUMN name SET NOT NULL;

-- 1..60 characters: the API requires 2..60 on input; backfilled names may have 1.
ALTER TABLE users ADD CONSTRAINT users_name_check
    CHECK (char_length(name) BETWEEN 1 AND 60 AND name = btrim(name));
