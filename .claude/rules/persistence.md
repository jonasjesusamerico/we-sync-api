---
paths:
  - "internal/**/repository*.go"
  - "internal/database/**"
  - "migrations/**"
---

# Persistência (PostgreSQL + database/sql)

## Repositórios

- SQL à mão em `const query` no método; sem ORM nem query builder.
- **Só placeholders** (`$1, $2`). Filtros dinâmicos (`purchase` `List`): monte só fragmentos fixos de `WHERE`
  com placeholders numerados e valores em `args`.
- Leitura em `r.db.Read`; escrita e transações em `r.db.Write`. Ler o que acabou de escrever: `Write` ou `RETURNING`
  (prefira `RETURNING` a um `SELECT` depois).
- Por operação: `context.WithTimeout(ctx, dbTimeout)` + `defer cancel()`; span `db.<tabela>.<operação>` + `dbAttrs`.
- `defer rows.Close()` e `rows.Err()` após o loop — conexão não liberada esgota o pool.
- Colunas anuláveis via `sql.Null*` com conversão explícita (`user_id` em `category`).

## Consultas eficientes

- **Sem N+1**: nada de query em loop; use `JOIN` (como `purchase.List` traz a categoria) ou `= ANY($1)`.
- Selecione só as colunas que o model usa; nunca `SELECT *`.
- `ORDER BY` sempre termina em chave única (`purchase_date DESC, id DESC`) — senão páginas repetem/pulam itens.
- Paginação atual `LIMIT/OFFSET` + `COUNT(*)`; se precisar escalar, proponha keyset antes de implementar.
- Ordenação, contagem e soma no banco, não em Go sobre listas completas.
- Índice para coluna nova usada em `WHERE`/`JOIN`/`ORDER BY` frequente; prefira composto que case com o filtro
  (ex.: `(user_id, purchase_date)`). Em dúvida, `EXPLAIN ANALYZE` e informe.

## Contratos de retorno (documentados em `repository.go`)

| Caso | Retorno |
|---|---|
| não encontrado em `GetByID`/`Update` | `(nil, nil)` |
| nada removido em `Delete` | `(false, nil)` |
| `23505` unique / `23503` FK | `apperrors.ErrConflict` |
| demais | `fmt.Errorf("<tipo>.<Metodo>: %w", err)` |

Erro do Postgres via `errors.As(err, &pgErr)` com `*pgconn.PgError`.

## Escopo por usuário

SQL de dado de usuário filtra por `user_id` (`WHERE id = $1 AND user_id = $2`) mesmo que o service já tenha
checado — defesa em profundidade. Exceção documentada: `category.GetByID` sem filtro (globais compartilhadas);
quem chama verifica `AccessibleBy(userID)`. Nova exceção só documentada na interface.

## Transações

Para mais de uma escrita atômica (ex.: `auth.RotateRefreshToken`): `BeginTx` em `r.db.Write`, `defer tx.Rollback()`,
`Commit` explícito, tudo dentro do repositório. Transação curta, sem trabalho pesado dentro; ao travar várias
linhas, mesma ordem (ex.: `id` crescente) para evitar deadlock.

## Integridade e migrations

- `migrations/NNNNNN_descricao.up.sql` + `.down.sql` que reverte de verdade; nunca edite migration aplicada.
- Regra crítica garantida **no banco e no service**: `NOT NULL` por padrão, `CHECK` para enums e valores
  (`amount > 0`), FKs, índices únicos (inclusive parciais, como `000003_create_categories`). Enum novo ⇒ `CHECK` em nova migration.
- `NULL` só quando "ausente" é estado de negócio (`categories.user_id IS NULL` = global); vazio não substitui `NULL`.
- Tipos: `TIMESTAMPTZ` para instantes, `DATE` ↔ `purchase.Date` (sem passar por fuso local),
  `NUMERIC(p,s)` ↔ `decimal.Decimal` (arredondamento explícito e documentado), `BIGINT`/`BIGSERIAL` para IDs.
- **Rolling deploy** (2 instâncias): expand/contract — adicione coluna anulável/com default → código que usa →
  backfill → só então `NOT NULL`/remoção em migration posterior. Não renomeie/remova coluna que a versão em
  execução lê. Índice em tabela grande: considere `CREATE INDEX CONCURRENTLY`.
- Comentários SQL em inglês; dados exibidos ao usuário em pt-BR; seeds globais em migration própria.

## Verificação

Sem testes com banco. Valide SQL/migrations com `docker compose up postgres migrate` + `http/*.http` quando
possível; senão, diga que o SQL não foi validado contra o banco.
