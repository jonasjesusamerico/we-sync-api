-- Categorias predefinidas pelo sistema (user_id IS NULL = global).
-- Disponíveis para todos os usuários autenticados; não podem ser alteradas
-- ou excluídas por usuários via API.
INSERT INTO categories (user_id, name) VALUES
    (NULL, 'Alimentação'),
    (NULL, 'Transporte'),
    (NULL, 'Moradia'),
    (NULL, 'Saúde'),
    (NULL, 'Educação'),
    (NULL, 'Lazer');
