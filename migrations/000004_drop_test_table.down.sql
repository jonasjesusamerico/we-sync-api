-- 000004_drop_test_table.down.sql
-- Recreates the 'test' table to allow rollback to the 000002 state.
CREATE TABLE IF NOT EXISTS test (
    id     SERIAL PRIMARY KEY,
    status TEXT   NOT NULL
);
