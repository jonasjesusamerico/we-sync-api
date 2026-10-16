-- Payment methods, modeled after categories (see 000003_create_categories).
-- user_id IS NULL  → global payment method (system-defined, immutable for users).
-- user_id NOT NULL → personal payment method, owned by the given user.
--
-- Name uniqueness (case-insensitive) is enforced within each group by partial
-- unique indexes; the service blocks personal names that shadow a global one.

CREATE TABLE payment_methods (
    id         BIGSERIAL    PRIMARY KEY,
    user_id    BIGINT       REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT         NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_payment_methods_global_name
    ON payment_methods (lower(name))
    WHERE user_id IS NULL;

CREATE UNIQUE INDEX uq_payment_methods_user_name
    ON payment_methods (user_id, lower(name))
    WHERE user_id IS NOT NULL;

CREATE INDEX idx_payment_methods_user_id ON payment_methods (user_id);

-- System-defined payment methods. Names are user-facing data (pt-BR) on purpose.
INSERT INTO payment_methods (user_id, name) VALUES
    (NULL, 'Dinheiro'),
    (NULL, 'Cartão de crédito'),
    (NULL, 'Cartão de débito'),
    (NULL, 'Pix'),
    (NULL, 'Transferência bancária'),
    (NULL, 'Boleto');

-- Replace the fixed payment_method enum with a reference to payment_methods.
-- No backfill: there is no persisted data yet (see docs/specs/001-payment-methods).
-- Adding the NOT NULL column fails if rows exist, which is intended.
-- The constraint name is referenced by the purchase repository to tell this
-- FK violation apart from the category one. ON DELETE defaults to RESTRICT, so a
-- payment method in use cannot be deleted (23503 → 409 in the API).
ALTER TABLE purchase_entries
    DROP CONSTRAINT chk_payment_method,
    DROP COLUMN payment_method,
    ADD COLUMN payment_method_id BIGINT NOT NULL
        CONSTRAINT fk_purchase_entries_payment_method REFERENCES payment_methods(id);

CREATE INDEX idx_purchase_entries_payment_method_id ON purchase_entries (payment_method_id);
