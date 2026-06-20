-- 000001_init.up.sql
CREATE TABLE health_check_test (
    id SERIAL PRIMARY KEY,
    status TEXT NOT NULL
);
