-- Archived rows become active again: the archive state is lost on rollback.
-- The old lower(name) indexes accept everything the 000011 indexes accepted, so recreating them cannot fail.
DROP INDEX uq_payment_methods_user_name;
DROP INDEX uq_payment_methods_global_name;
CREATE UNIQUE INDEX uq_payment_methods_global_name ON payment_methods (lower(name)) WHERE user_id IS NULL;
CREATE UNIQUE INDEX uq_payment_methods_user_name   ON payment_methods (user_id, lower(name)) WHERE user_id IS NOT NULL;
ALTER TABLE payment_methods
    DROP CONSTRAINT payment_methods_global_not_archived,
    DROP COLUMN archived_at;

DROP INDEX uq_categories_user_name;
DROP INDEX uq_categories_global_name;
CREATE UNIQUE INDEX uq_categories_global_name ON categories (lower(name)) WHERE user_id IS NULL;
CREATE UNIQUE INDEX uq_categories_user_name   ON categories (user_id, lower(name)) WHERE user_id IS NOT NULL;
ALTER TABLE categories
    DROP CONSTRAINT categories_global_not_archived,
    DROP CONSTRAINT categories_color_check,
    DROP CONSTRAINT categories_icon_check,
    DROP COLUMN archived_at,
    DROP COLUMN color,
    DROP COLUMN icon;

DROP FUNCTION catalog_name_key(TEXT);
