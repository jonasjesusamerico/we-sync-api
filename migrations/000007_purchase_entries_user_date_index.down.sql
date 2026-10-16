CREATE INDEX idx_purchase_entries_status        ON purchase_entries (status);
CREATE INDEX idx_purchase_entries_purchase_date ON purchase_entries (purchase_date);
CREATE INDEX idx_purchase_entries_user_id       ON purchase_entries (user_id);

DROP INDEX idx_purchase_entries_user_date;
