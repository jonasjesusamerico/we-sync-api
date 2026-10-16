-- Composite index matching how purchases are always read: scoped by user,
-- filtered by a purchase_date range and ordered by (purchase_date DESC, id DESC).
-- It serves List (filter + ORDER BY + LIMIT without a sort) and the COUNT.
CREATE INDEX idx_purchase_entries_user_date
    ON purchase_entries (user_id, purchase_date DESC, id DESC);

-- user_id alone is a prefix of the index above (it also serves the users FK cascade).
DROP INDEX idx_purchase_entries_user_id;

-- Every query is scoped by user_id, so these single-column indexes are never chosen
-- and only add write cost and memory pressure on shared_buffers.
DROP INDEX idx_purchase_entries_purchase_date;
DROP INDEX idx_purchase_entries_status;
