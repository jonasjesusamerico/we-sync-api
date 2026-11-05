-- Accent- and case-insensitive key for category and payment method names (feature 006, ADR-07):
-- "Saude" = "Saúde" = "SAÚDE". unaccent() is STABLE; with the dictionary and the schema fixed it is safe
-- to declare this wrapper IMMUTABLE and use it in indexes.
-- If a PostgreSQL upgrade changes unaccent.rules, REINDEX the four uq_*_name indexes below.
CREATE FUNCTION catalog_name_key(name TEXT) RETURNS TEXT
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    RETURN lower(public.unaccent('public.unaccent'::regdictionary, name));

-- Categories: icon and color (enums of the API, ADR-06) and archiving (ADR-08).
-- The defaults keep the running version inserting during the deploy and give the existing own
-- categories the tag icon and the neutral color (RN-11). NEUTRAL is stored only: the API never accepts it.
ALTER TABLE categories
    ADD COLUMN icon TEXT NOT NULL DEFAULT 'TAG',
    ADD COLUMN color TEXT NOT NULL DEFAULT 'NEUTRAL',
    ADD COLUMN archived_at TIMESTAMPTZ,
    ADD CONSTRAINT categories_icon_check CHECK (icon IN
        ('TAG', 'HEART', 'SMILE', 'BOOK', 'CAR', 'HOUSE', 'FOOD', 'CARD', 'GIFT', 'SPARKLE')),
    ADD CONSTRAINT categories_color_check CHECK (color IN
        ('FOOD', 'TRANSPORT', 'HOME', 'HEALTH', 'EDUCATION', 'LEISURE', 'NEUTRAL')),
    -- archived_at IS NULL = active. Global categories are never archived.
    ADD CONSTRAINT categories_global_not_archived CHECK (user_id IS NOT NULL OR archived_at IS NULL);

-- Icon and color of the global categories, as in the prototype.
UPDATE categories SET icon = 'FOOD',  color = 'FOOD'      WHERE user_id IS NULL AND name = 'Alimentação';
UPDATE categories SET icon = 'CAR',   color = 'TRANSPORT' WHERE user_id IS NULL AND name = 'Transporte';
UPDATE categories SET icon = 'HOUSE', color = 'HOME'      WHERE user_id IS NULL AND name = 'Moradia';
UPDATE categories SET icon = 'HEART', color = 'HEALTH'    WHERE user_id IS NULL AND name = 'Saúde';
UPDATE categories SET icon = 'BOOK',  color = 'EDUCATION' WHERE user_id IS NULL AND name = 'Educação';
UPDATE categories SET icon = 'SMILE', color = 'LEISURE'   WHERE user_id IS NULL AND name = 'Lazer';

-- Name uniqueness ignoring case and accents, also for archived rows (a name of an archived category
-- reactivates it on create and blocks a rename, RN-13). Global × own stays in the service.
DROP INDEX uq_categories_global_name;
DROP INDEX uq_categories_user_name;
CREATE UNIQUE INDEX uq_categories_global_name
    ON categories (catalog_name_key(name))
    WHERE user_id IS NULL;
CREATE UNIQUE INDEX uq_categories_user_name
    ON categories (user_id, catalog_name_key(name))
    WHERE user_id IS NOT NULL;

-- Payment methods: archiving only (no icon or color, RF-13).
ALTER TABLE payment_methods
    ADD COLUMN archived_at TIMESTAMPTZ,
    ADD CONSTRAINT payment_methods_global_not_archived CHECK (user_id IS NOT NULL OR archived_at IS NULL);

DROP INDEX uq_payment_methods_global_name;
DROP INDEX uq_payment_methods_user_name;
CREATE UNIQUE INDEX uq_payment_methods_global_name
    ON payment_methods (catalog_name_key(name))
    WHERE user_id IS NULL;
CREATE UNIQUE INDEX uq_payment_methods_user_name
    ON payment_methods (user_id, catalog_name_key(name))
    WHERE user_id IS NOT NULL;
