-- purchase_entries references categories(id) through category_id.
-- The categories table must exist before this migration (000003).
-- The former sector field was replaced by category_id.
-- Ownership integrity (a user may only use global categories or their own)
-- is enforced by the service layer; the database guarantees existence via FK.

CREATE TABLE purchase_entries (
    id             BIGSERIAL      PRIMARY KEY,
    user_id        BIGINT         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id    BIGINT         NOT NULL REFERENCES categories(id),
    purchase_date  DATE           NOT NULL,
    description    TEXT           NOT NULL,
    payment_method VARCHAR(50)    NOT NULL,
    amount         NUMERIC(10, 2) NOT NULL CHECK (amount > 0),
    status         VARCHAR(20)    NOT NULL,
    created_at     TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_status CHECK (status IN ('PENDING', 'PAID', 'CANCELED')),
    CONSTRAINT chk_payment_method CHECK (
        payment_method IN ('CASH', 'CREDIT_CARD', 'DEBIT_CARD', 'PIX', 'BANK_TRANSFER', 'BOLETO')
    )
);

CREATE INDEX idx_purchase_entries_user_id       ON purchase_entries (user_id);
CREATE INDEX idx_purchase_entries_purchase_date ON purchase_entries (purchase_date);
CREATE INDEX idx_purchase_entries_status        ON purchase_entries (status);
CREATE INDEX idx_purchase_entries_category_id   ON purchase_entries (category_id);
