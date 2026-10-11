# Diagnóstico técnico — we-sync-api

## Resumo executivo do estado atual

O repositório implementa uma API Go para CRUD de lançamentos financeiros (`purchase_entries`), com PostgreSQL, `database/sql` + `pgx`, `chi`, `slog`, migrations Dockerizadas e telemetria OpenTelemetry.

A implementação do domínio de compras está funcional no código e coberta por testes unitários de handler e service. Contudo, não existe autenticação, autorização ou multitenancy: todos os registros pertencem a um espaço global. Isso contraria uma diretriz central e é o principal risco atual.

## Estrutura, responsabilidades e inicialização

```text
cmd/api/main.go                 ponto de entrada e composição das dependências
internal/
  apperrors/                    erros de aplicação compartilhados
  configs/                      ambiente, pools Read/Write do PostgreSQL
  handler/                      handler genérico de health check
  httputil/                     respostas JSON e envelope de erros
  logger/                       logger slog e logger no contexto
  middleware/                   CORS e logging por request
  purchase/                     domínio completo de compras
    model.go                    entidade, tipos, enums, filtros e DTOs
    handler.go                  adaptador HTTP do domínio
    service.go                  casos de uso e validação
    repository.go               porta de persistência
    repository_postgres.go      adaptador PostgreSQL
  router/                       rotas chi e middlewares globais
  telemetry/                    OpenTelemetry e endpoint Prometheus
migrations/                     migrations SQL versionadas
docs/swagger/                   OpenAPI gerado
observability/                  OTel Collector, Prometheus e Grafana
```

O ponto de entrada é `cmd/api/main.go`. O bootstrap carrega o ambiente, inicializa telemetria, abre pools de banco, compõe `repository → service → handler`, registra o router e encerra servidor, telemetria e pools após sinal do sistema.

## O que já funciona

- Health check: `GET /health-check`.
- CRUD de compras em `/api/v1/purchases`.
- Validação de campos obrigatórios, data, valor positivo, status e método de pagamento.
- Paginação e filtros por mês, ano, status, setor e método de pagamento.
- SQL parametrizado, sem ORM.
- Separação efetiva entre pools `db.Read` e `db.Write`.
- Tratamento HTTP padronizado para `400`, `404`, `422` e `500`.
- Logging estruturado por request, com UUID de request e correlação com trace quando disponível.
- Tracing e métricas OpenTelemetry.
- Migrations e stack local de PostgreSQL, Nginx, Jaeger, Prometheus e Grafana.

### Endpoints e fluxos

| Método | Rota | Fluxo |
|---|---|---|
| GET | `/health-check` | handler → resposta `{status:"ok"}`; não consulta o banco |
| POST | `/api/v1/purchases/` | handler decodifica JSON → service valida → repository usa `db.Write` → `INSERT ... RETURNING` |
| GET | `/api/v1/purchases/` | handler cria filtro → service normaliza paginação → repository usa `db.Read` para `COUNT` e `SELECT` |
| GET | `/api/v1/purchases/{id}` | handler valida ID → service → repository usa `db.Read` |
| PUT | `/api/v1/purchases/{id}` | handler decodifica → service valida → repository usa `db.Write` com `UPDATE ... RETURNING` |
| DELETE | `/api/v1/purchases/{id}` | handler → service → repository usa `db.Write` com `DELETE` |

## O que está parcialmente implementado

- Arquitetura hexagonal: há portas `PurchaseService` e `PurchaseRepository`, injeção manual em `main` e adaptador PostgreSQL. Porém, domínio, DTOs HTTP, handler e repositório concreto coexistem no pacote `purchase`.
- Observabilidade: tracing, métricas e logs estão presentes, mas o servidor Prometheus auxiliar não tem shutdown e ignora falha de inicialização.
- Health check: é um liveness check, não readiness check; responde saudável mesmo se o banco ficar indisponível após a inicialização.
- CORS: restringe origens configuradas, mas não normaliza valores da variável de ambiente e responde a preflight sem exigir origem permitida.
- Swagger: existe e é servido condicionalmente, mas não há validação automatizada de sincronização com o código.
- Testes: há testes unitários de handler e service, sem integração HTTP completa, PostgreSQL, migrations, telemetria ou Docker Compose.

## O que ainda não foi implementado

- JWT, login, cadastro de usuário e autorização; as rotas estão somente comentadas.
- Multitenancy lógica por `user_id`.
- Extração de identidade confiável do `context.Context`.
- Coluna `user_id`, chave estrangeira ou tabelas de usuários.
- ULID para identificadores persistidos.
- Testes de integração com PostgreSQL.
- Transações explícitas; os CRUDs atuais possuem uma instrução cada, mas não há infraestrutura para casos de uso multi-etapa.
- Controle de concorrência otimista para atualizações concorrentes.

## Banco de dados

A conexão usa dois `*sql.DB` com driver `pgx`:

- `Write`: inserção, atualização e exclusão.
- `Read`: consultas por ID, contagem e listagem.

As operações usam o pool apropriado. A tabela `purchase_entries` possui `BIGSERIAL`, data, descrição, setor, método, valor decimal, status e timestamps. Não há relacionamentos nem `user_id`.

As queries têm timeout de dez segundos e a listagem fecha `rows` e verifica `rows.Err()`. Não existem transações explícitas. As migrations são aplicadas pelo container `migrate`; a aplicação não as executa internamente.

## Problemas concretos

1. **Crítico — ausência de multitenancy e autorização.** Não há middleware JWT ativo, `user_id` no contexto, coluna de tenant ou filtros por tenant. Todas as compras são globais. Referências: `internal/router/router.go:70`, `migrations/000003_create_purchase_entries.up.sql:1`, `internal/purchase/repository_postgres.go:101` e `:346`.
2. **Alto — rollback da migration inicial falha.** A migration `up` cria `health_check_test`, mas a `down` remove `health_check`. Referências: `migrations/000001_init.up.sql:2` e `migrations/000001_init.down.sql:2`.
3. **Alto — upstreams duplicados no Nginx.** `app1` e `app2` são declarados duas vezes no mesmo bloco. Referências: `nginx.conf:26-32`.
4. **Médio — servidor HTTP sem timeouts de conexão.** Não há `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` ou `IdleTimeout`. Referência: `cmd/api/main.go:82`.
5. **Médio — `os.Exit(1)` em goroutine.** Falha de `ListenAndServe` pula o fechamento de telemetria e pools de banco. Referência: `cmd/api/main.go:99-105`.
6. **Médio — servidor de métricas ignora falhas e não é desligado.** `ListenAndServe` é descartado e não há shutdown. Referência: `internal/telemetry/telemetry.go:174-186`.
7. **Médio — borda HTTP permissiva.** Corpo sem limite, campos JSON desconhecidos e conteúdo adicional não são rejeitados. Referências: `internal/purchase/handler.go:39-43` e `:173-177`.
8. **Médio — filtros inválidos na listagem.** `status` e `payment_method` inválidos não produzem erro de validação. Referências: `internal/purchase/handler.go:74-78`, `internal/purchase/service.go:124-158`.
9. **Médio — paginação inconsistente sob concorrência.** `COUNT` e busca de itens são queries independentes sem snapshot comum. Referências: `internal/purchase/repository_postgres.go:197-203` e `:216-255`.
10. **Baixo — filtros podem degradar em volume.** `EXTRACT(MONTH/YEAR)` e `ILIKE '%texto%'` reduzem o uso dos índices atuais. Referências: `internal/purchase/repository_postgres.go:162-180`.
11. **Baixo — configurações sensíveis de desenvolvimento no Compose.** Devem permanecer exclusivamente locais e ser externalizadas em produção. Referências: `docker-compose.yml:11-15` e `:202-204`.

## Qualidade e testes executados

Comandos executados:

```text
GOCACHE=/tmp/we-sync-api-gocache go test ./...
GOCACHE=/tmp/we-sync-api-gocache go vet ./...
```

Resultados:

- `go test ./...`: sucesso em todos os pacotes; `internal/purchase` passou em `0.007s`.
- `go vet ./...`: sucesso, sem diagnósticos.
- Existem testes unitários de handler e service em `internal/purchase`.
- Não existem testes de integração, banco real, migrations, repositório PostgreSQL, Compose, autenticação, CORS ou concorrência.

## Matriz de aderência às decisões arquiteturais

| Diretriz | Estado | Evidência |
|---|---|---|
| Go stdlib-first | Atendida | `net/http`, `slog`, `database/sql`; dependências externas pontuais |
| Clean Architecture / Hexagonal | Parcialmente atendida | Há ports e DI manual, mas responsabilidades HTTP e infraestrutura permanecem no pacote de domínio |
| PostgreSQL + `database/sql` + `pgx`, sem ORM | Atendida | Driver pgx via stdlib e SQL manual |
| `chi` para rotas e `slog` para logs | Atendida | Router chi e logger JSON com slog |
| ULID para identificadores | Não atendida | Banco usa `BIGSERIAL`; request ID usa UUID |
| Multitenancy por `user_id` do contexto confiável | Não atendida | Não há identidade, middleware, coluna ou filtros de tenant |
| `db.Read` para leitura e `db.Write` para escrita/transações | Atendida | Uso consistente no repositório |
| Separação entre domain, service, repository, handler e middleware | Parcialmente atendida | Há papéis/arquivos distintos, mas o pacote `purchase` concentra camadas |

## Cinco próximos passos recomendados

1. Implementar autenticação JWT e middleware que injete um `user_id` confiável no contexto.
2. Introduzir multitenancy no schema e em todas as consultas, incluindo índices e testes de isolamento entre usuários.
3. Corrigir a migration `000001` e validar migrations de subida/rollback contra PostgreSQL real.
4. Corrigir o upstream duplicado do Nginx e adicionar health checks/readiness para as instâncias da API.
5. Endurecer a borda HTTP: timeouts do servidor, limite de corpo, JSON estrito e validação de filtros de listagem.
