---
paths:
  - "cmd/**/*.go"
  - "internal/router/**"
  - "internal/config/**"
  - "internal/database/**"
  - "internal/logger/**"
  - "internal/telemetry/**"
  - "internal/**/service.go"
  - "internal/**/repository*.go"
---

# Arquitetura

## Estilo: package-by-feature em camadas finas

`internal/<domínio>` com `handler → *Service → Repository (interface) → *PostgresRepository → PostgreSQL`.
Interfaces seguem "aceite interfaces, retorne structs" (ver `solid-and-code-quality.md`, seção I).
Portas e adaptadores **leve**: sem `domain/`, `usecase/`, `infra/` nem mappers. É adequado ao tamanho do
projeto — **mantenha**. Clean Architecture completa, DDD tático, CQRS, event sourcing, mensageria,
microsserviços, container de DI ou geradores de código só com problema concreto e proposta prévia.

## Responsabilidades

| Arquivo | Faz | Não faz |
|---|---|---|
| `handler.go` | interface `service` mínima (só o que o handler chama), decode, parse de path/query, chama service, erro→HTTP, anotações swag | regra de negócio, banco, decidir autorização |
| `service.go` | struct `Service`, interfaces mínimas de outros domínios, `userID` do contexto, regras, propriedade, orquestra repositórios, spans/métricas | `http.Request`, SQL, status HTTP |
| `repository.go` | interface `Repository` + contrato documentado (porta consumida pelo `Service`) | — |
| `repository_postgres.go` | struct `PostgresRepository` + `var _ Repository = (*PostgresRepository)(nil)`, SQL, scan, erro Postgres→`apperrors`, timeout, span de DB | regra de negócio, validação |
| `model.go` | entidades, requests/responses, enums com `IsValid()`, erros do domínio | dependências externas |

Referências: `internal/category` (propriedade global vs. pessoal) e `internal/purchase` (filtros, paginação,
métricas, dependência de outro domínio).

## Novo domínio

1. Copie a forma de `internal/category`.
2. `NewService(repo Repository, ...) *Service`, `NewRepository(db *database.DB) *PostgresRepository`, `NewHandler(svc service) *Handler` — construtores retornam o tipo concreto (sem repetir o nome do pacote: `xxx.Service`, não `xxx.XxxService`).
3. Wiring em `cmd/api/main.go`; registro em `router.Handlers` + `registerV1`, no grupo autenticado.
4. Migration (`persistence.md`) e testes de service/handler (`testing.md`).

Pacote novo só para conceito com ciclo de vida próprio; operação nova sobre conceito existente vai no pacote existente.

## Dependências entre pacotes

- `cmd` → `router` → domínios → (`apperrors`, `httputil`, `logger`, `database`, `identity` para o usuário do contexto); `config` só é importado por `cmd`, `logger` e `database`.
- Domínio → domínio só com relação real (`purchase` → `category`), **sem ciclos**; se dois precisarem um do
  outro, proponha onde a regra mora.
- `httputil`/`apperrors` só recebem código usado por mais de um domínio. Sem `utils`/`common`/`helpers`.
- Env var nova: `config.Config` via `getEnv*`, default seguro, validação em `LoadProperties`; só `config` lê
  `os.Getenv`; valores chegam por construtor.

## Observabilidade

- Service: `tracerName = "we-sync-api/<domínio>"`, span `"<tipo>.<Metodo>"`.
  Repository: `repoTracerName = "we-sync-api/<domínio>/repository"`, span `"db.<tabela>.<operação>"` + `dbAttrs(...)`.
- Erro inesperado: `RecordError` + `SetStatus(codes.Error, ...)`. Erro esperado de negócio: só `SetStatus`
  (em `auth`, `recordAuthError`/`isClientError`).
- Atributos: IDs e enums (dados proibidos em `security.md`; sem descrição livre nem valores monetários).
- Métricas de negócio (`metric.Int64Counter`) só com valor operacional, criadas no construtor do service.
- Logs via `logger.FromContext(ctx)` (traz `request_id`, `trace_id`, `user_id`), nunca `slog` global em request.

## Operação

- `/health-check` é liveness e não toca o banco — mantenha; readiness com dependências seria endpoint separado.
- Alertas, SLI/SLO, backups e DR não existem no repositório; não invente em código, aponte no resumo se relevante.
- Mover pacotes, trocar biblioteca base (chi, pgx, slog, OTel), nova camada ou novo wiring: apresente problema,
  alternativa mínima e impacto antes.
