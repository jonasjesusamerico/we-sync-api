-- Expense categories.
-- user_id IS NULL  → global category (system-defined, immutable for users).
-- user_id NOT NULL → personal category, owned by the given user.
--
-- Name uniqueness (case-insensitive) is enforced by two partial indexes:
--   1. Globals: unique on lower(name) where user_id IS NULL.
--   2. Personal: unique on (user_id, lower(name)) where user_id IS NOT NULL.
--
-- Additionally, to prevent a user from creating a category with the same name
-- as a global one, validation is applied in the service. The database enforces
-- the invariant within each group; the application enforces it across groups.
--
-- Note: COALESCE(user_id, 0) cannot be used in a purchase_entries FK because
-- user_id = 0 does not exist in users. The chosen approach is a plain FK on
-- category_id with ownership validation in the service layer (see purchase service).

CREATE TABLE categories (
    id         BIGSERIAL    PRIMARY KEY,
    user_id    BIGINT       REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT         NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Global uniqueness: two global names cannot share the same lower(name).
CREATE UNIQUE INDEX uq_categories_global_name
    ON categories (lower(name))
    WHERE user_id IS NULL;

-- Personal uniqueness: the same user cannot have two equal names.
CREATE UNIQUE INDEX uq_categories_user_name
    ON categories (user_id, lower(name))
    WHERE user_id IS NOT NULL;

-- Supporting index for lookups and FK checks by user_id.
CREATE INDEX idx_categories_user_id ON categories (user_id);
