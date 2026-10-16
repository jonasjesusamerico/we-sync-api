-- purchase_entries referencia categories(id) via category_id.
-- A tabela categories deve existir antes desta migration (000003).
-- O campo sector foi substituído por category_id nesta versão.
-- A integridade de propriedade (usuário só usa categorias globais ou suas próprias)
-- é garantida pela camada de serviço; o banco garante existência via FK.

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
