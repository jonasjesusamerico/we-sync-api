-- Restores the fixed payment_method enum column. Fails if rows exist, mirroring the up migration.
DROP INDEX IF EXISTS idx_purchase_entries_payment_method_id;

ALTER TABLE purchase_entries
    DROP COLUMN payment_method_id,
    ADD COLUMN payment_method VARCHAR(50) NOT NULL,
    ADD CONSTRAINT chk_payment_method CHECK (
        payment_method IN ('CASH', 'CREDIT_CARD', 'DEBIT_CARD', 'PIX', 'BANK_TRANSFER', 'BOLETO')
    );

DROP TABLE IF EXISTS payment_methods;
