CREATE TABLE purchase_entries (
    id             BIGSERIAL PRIMARY KEY,
    purchase_date  DATE           NOT NULL,
    description    TEXT           NOT NULL,
    sector         VARCHAR(255)   NOT NULL,
    payment_method VARCHAR(50)    NOT NULL,
    amount         NUMERIC(10, 2) NOT NULL CHECK (amount > 0),
    status         VARCHAR(20)    NOT NULL,
    created_at     TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ    NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_status
        CHECK (status IN ('PENDING', 'PAID', 'CANCELED')),

    CONSTRAINT chk_payment_method
        CHECK (payment_method IN ('CASH', 'CREDIT_CARD', 'DEBIT_CARD', 'PIX', 'BANK_TRANSFER', 'BOLETO'))
);

CREATE INDEX idx_purchase_entries_purchase_date ON purchase_entries (purchase_date);
CREATE INDEX idx_purchase_entries_status        ON purchase_entries (status);
CREATE INDEX idx_purchase_entries_sector        ON purchase_entries (sector);
