-- System-defined categories (user_id IS NULL = global).
-- Available to every authenticated user; they cannot be changed or deleted
-- by users through the API. Names are user-facing data (pt-BR) on purpose.
INSERT INTO categories (user_id, name) VALUES
    (NULL, 'Alimentação'),
    (NULL, 'Transporte'),
    (NULL, 'Moradia'),
    (NULL, 'Saúde'),
    (NULL, 'Educação'),
    (NULL, 'Lazer');
