-- Accent-insensitive search on purchase descriptions (feature 005, ADR-01).
-- unaccent ships in the contrib of the postgres:17 image and is trusted since PostgreSQL 13.
CREATE EXTENSION IF NOT EXISTS unaccent;
