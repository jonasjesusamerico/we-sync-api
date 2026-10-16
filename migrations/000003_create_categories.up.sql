-- Tabela de categorias de despesas.
-- user_id IS NULL  → categoria global (predefinida pelo sistema, imutável por usuários).
-- user_id NOT NULL → categoria personalizada, pertence ao usuário indicado.
--
-- Unicidade de nome (case-insensitive) é garantida por dois índices parciais:
--   1. Globais: unique por lower(name) onde user_id IS NULL.
--   2. Personalizadas: unique por (user_id, lower(name)) onde user_id IS NOT NULL.
--
-- Adicionalmente, para impedir que um usuário crie uma categoria com nome igual
-- ao de uma categoria global, a validação é aplicada no service. O banco garante
-- a invariante dentro de cada grupo; a aplicação garante entre grupos.
--
-- Nota: COALESCE(user_id, 0) não pode ser usado em FK de purchase_entries porque
-- user_id = 0 não existe em users. A solução adotada é FK simples em category_id
-- com validação de propriedade na camada de serviço (ver purchase service).

CREATE TABLE categories (
    id         BIGSERIAL    PRIMARY KEY,
    user_id    BIGINT       REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT         NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Unicidade de globais: dois nomes globais não podem ter o mesmo lower(name).
CREATE UNIQUE INDEX uq_categories_global_name
    ON categories (lower(name))
    WHERE user_id IS NULL;

-- Unicidade de personalizadas: mesmo usuário não pode ter dois nomes iguais.
CREATE UNIQUE INDEX uq_categories_user_name
    ON categories (user_id, lower(name))
    WHERE user_id IS NOT NULL;

-- Índice de suporte para buscas e FK lookups por user_id.
CREATE INDEX idx_categories_user_id ON categories (user_id);
