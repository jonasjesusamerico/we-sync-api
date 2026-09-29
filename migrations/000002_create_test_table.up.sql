-- Migration 000002: creates the temporary test table removed in 000004.
-- Kept as a no-op stub so the migration sequence is contiguous.
CREATE TABLE IF NOT EXISTS test (
    id BIGSERIAL PRIMARY KEY
);
