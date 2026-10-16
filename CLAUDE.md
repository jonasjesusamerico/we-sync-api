# we-sync API — diretrizes do Claude Code

API REST em Go para gestão financeira pessoal (lançamentos de compras, categorias, autenticação).
Somente backend: **não há frontend neste repositório** — não crie regras, arquivos ou dependências de UI.

Atue como engenheiro sênior: simplicidade, correção, segurança e consistência com o código existente
valem mais do que demonstrar padrões. Qualidade **não** se mede em número de camadas ou abstrações.

## Stack (evidência: `go.mod`, `docker-compose.yml`)

- Go 1.27.2, `net/http` + `go-chi/chi/v5`
- PostgreSQL 17 via `database/sql` + driver `pgx/v5/stdlib` (sem ORM), pools separados `database.DB.Write` / `database.DB.Read`
- Migrations SQL com `golang-migrate` (`migrations/NNNNNN_nome.{up,down}.sql`), executadas pelo serviço `migrate` do compose
- Auth: JWT HS256 (`golang-jwt/jwt/v5`) + refresh token opaco (hash SHA-256 no banco) + `bcrypt`
- Dinheiro: `shopspring/decimal` (nunca `float64`); datas sem hora: `purchase.Date`
- Observabilidade: OpenTelemetry (traces + métricas), `log/slog` JSON, Prometheus/Grafana em `observability/`
- Docs da API: `swaggo` (anotações nos handlers → `docs/swagger/`, arquivos gerados — não editar à mão)
- Testes: `testing`, `httptest`, mocks manuais, `pgregory.net/rapid`; carga com k6 em `perf/k6/`
- Deploy: 2 instâncias (`app1`, `app2`) atrás do nginx, mesmo banco

## Mapa do código

```
cmd/api/main.go          composição manual de dependências + graceful shutdown
internal/router          rotas, middlewares globais, grupo autenticado
internal/<domínio>/      auth, category, purchase — um pacote por feature:
  model.go               entidades, DTOs de request/response, tipos enumerados
  repository.go          interface Repository (porta) + contratos documentados
  repository_postgres.go PostgresRepository: implementação SQL
  service.go             Service (struct): regras de negócio, validação, autorização por dono
  handler.go             interface local `service` + HTTP: decode, parse de path/query, mapeamento erro→status, anotações swag
internal/apperrors       erros sentinela (ErrNotFound, ErrValidation, ErrUnauthorized, ErrForbidden, ErrConflict)
internal/httputil        envelope de erro APIError e helpers Write*
internal/middleware      JWT, CORS, logging, limite de body
internal/config          env vars (Config), validação na carga
internal/database        conexões PostgreSQL (DB com pools Write/Read)
internal/identity        userID autenticado no context (WithUserID / UserIDFromContext)
internal/logger, telemetry
```

Regras detalhadas em `.claude/rules/` (carregadas pelos arquivos em uso): `architecture`, `solid-and-code-quality`,
`api-standards`, `persistence`, `security`, `testing`, `reliability-and-performance`.

## Comandos

```bash
gofmt -l internal cmd          # deve retornar vazio; corrigir com gofmt -w <arquivo>
go vet ./...                   # única análise estática configurada
go build ./...
go test ./...                  # unitários, não exigem banco
go test -race ./internal/...   # ao mexer em concorrência/middlewares
go test ./internal/purchase -run TestService_Create -v
docker compose up --build      # postgres, migrate, nginx (:8080), 2 apps, observabilidade
docker run --rm -i --network host -v $PWD/perf/k6:/scripts:ro grafana/k6:1.0.0 run /scripts/smoke.js   # carga: ver perf/k6/README.md
go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g cmd/api/main.go -o docs/swagger   # após mudar anotações
```

Após regenerar o Swagger, confira `git diff --stat docs/swagger`; se não puder regenerar, diga que ficou desatualizado.
Não existe Makefile nem CI — não crie sem pedido.

## Fluxo de trabalho

1. Entenda o requisito. Ambiguidade que muda contrato, regra de negócio ou segurança → pergunte;
   detalhe menor com convenção clara → siga a convenção e mencione.
2. Leia o código e os testes do trecho, e o módulo mais parecido (`category`, `purchase`).
3. Escolha a solução de menor complexidade que atende ao requisito atual.
4. Implemente e teste (service + handler: sucesso, falha, limites).
5. Verifique com os comandos acima (escopo menor no ciclo, suíte completa antes de concluir).
6. Revise o diff: regressão, vazamento entre usuários, erro engolido, abstração desnecessária, escopo extra.
7. Resuma: o que mudou, comandos executados e resultado real, limitações e pendências.

Tarefa grande (novo domínio, schema com dados, auth, nova dependência, contrato público): plano curto e
confirmação antes de implementar. Tarefa pequena: execute direto.

**Spec-Driven Development:** features não triviais seguem `docs/specs/README.md`
(spec → plan → tasks → implement, templates em `docs/specs/_templates/`). Não avance de fase sem o gate
da anterior; ao implementar, marque as tasks e mantenha spec e código coerentes.

## Regras inegociáveis

- **Escopo:** só o necessário; refatorações e "melhorias de passagem" vão para o resumo como sugestão.
- **Padrões do projeto:** desvie só com justificativa técnica explícita no resumo.
- **Dependência nova:** justifique e avise antes de adicionar.
- **Isolamento por usuário** e **controles de segurança** — ver `security.md`; nunca enfraqueça para "fazer funcionar".
- **Segredos:** nunca em código, testes, `http/*.http`, logs ou docs.
- **Migrations aplicadas** não são editadas; crie nova. (O projeto já normalizou migrations uma vez antes de
  produção — só repita com pedido explícito.)
- **Honestidade:** nunca afirme que build/teste passou sem executar e ler o resultado; diga o que não foi executado.

## Conflitos entre instruções

Prioridade: (1) pedido explícito do usuário, (2) segurança e isolamento de dados, (3) consistência com o
código existente, (4) preferências destas regras. Pedido que conflite com segurança → aponte o risco e pergunte.
Conflito entre regras → explicite a escolha no resumo.

## Idioma

- **Tudo o que é versionado em inglês**: identificadores, comentários, logs, mensagens de erro, anotações
  Swagger, comentários SQL/infra, `http/*.http` e `docs/`.
- Exceção: dados exibidos ao usuário final (ex.: nomes das categorias globais em `000004_seed_categories`) em pt-BR.
- Chat com o usuário: português.
