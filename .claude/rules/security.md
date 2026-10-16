---
paths:
  - "internal/auth/**"
  - "internal/middleware/**"
  - "internal/router/**"
  - "internal/config/**"
  - "internal/**/service.go"
  - "internal/**/repository*.go"
  - "migrations/**"
  - "cmd/**"
  - "http/**"
  - "docker-compose.yml"
  - "Dockerfile"
  - "nginx.conf"
---

# Segurança

Nenhuma alteração enfraquece um controle existente para facilitar uma implementação. Se um controle atrapalhar,
explique o conflito e pergunte.

## Autenticação (JWT + refresh token) — preservar

- `middleware.JWTWithConfig`: só `HS256`, `iss`/`aud` obrigatórios (`auth.JWTIssuer`/`JWTAudience`), `iat` e
  `exp` obrigatórios, `exp > iat`, `iat` ≤ 1 min no futuro, TTL ≤ 24h, `sub` numérico positivo.
- `JWT_SECRET`: obrigatório, ≥ 32 bytes, rejeita o segredo de desenvolvimento padrão. TTLs: access ≤ 24h, refresh ≤ 720h.
- Refresh token: 32 bytes de `crypto/rand`, guardado **só como SHA-256**, rotação atômica em transação,
  logout idempotente sem revelar se o token existia.
- Senhas `bcrypt` 8–72 bytes; e-mail normalizado (`trim + lower`).
- Login errado: mesmo erro (`invalid credentials`) **e mesmo custo de tempo** para usuário inexistente e senha
  errada — e-mail desconhecido compara contra `dummyPasswordHash` (derivado de `passwordCost`). Não retorne
  cedo quando `user == nil`.
- Não troque algoritmo, não aceite token por query string/cookie, não remova checagem de claims.
  Aleatoriedade de segurança só com `crypto/rand`.

## Autorização e isolamento por usuário

- `userID` vem **só** de `identity.UserIDFromContext(ctx)` no service; ausente ⇒ `ErrUnauthorized`.
  Requests nunca têm `user_id`; filtros (`ListFilter.UserID`) são preenchidos pelo service.
- Autenticado ≠ autorizado: toda operação por `id` confirma que o recurso é do usuário; o SQL também filtra
  por `user_id` (ver `persistence.md`).
- Recurso de outro usuário ⇒ `ErrNotFound` (404), nunca 403.
- ID de outro domínio no body (ex.: `category_id`) exige checagem de acesso (`validateCategoryAccess` +
  `AccessibleBy`) — FK garante existência, não propriedade.
- Categorias globais (`user_id IS NULL`) são somente leitura (`ErrForbidden`).
- Rota nova de dados de usuário fica no grupo autenticado do router.

## Validação de entrada

- Body limitado a 1 MiB (`middleware.MaxBodySize`) — não remova nem aumente sem justificativa.
- Path/query com `strconv` e checagem de faixa (`id > 0`, `month 1..12`).
- No service: obrigatoriedade, enums via `IsValid()`, `decimal > 0`, tamanho máximo de todo texto livre.
  Validação do cliente nunca substitui a do servidor.

## Segredos

- Só via env vars lidas em `internal/config`; em testes, valores claramente falsos.
- `.env` é ignorado pelo git; nunca copie valores dele para respostas ou arquivos versionados.
- `http/purchase.http` tem tokens de ambiente local já inválidos — aceitável; nunca tokens reais.
- `postgres/postgres` do compose é só para desenvolvimento local.

## Logs e telemetria

Proibido em logs e atributos de span: senha, hash, JWT, refresh token (bruto ou hash), `Authorization`, DSN,
`JWT_SECRET`, corpo de request, e-mail (use `user_id`). Erro interno é logado no servidor; o cliente recebe
mensagem genérica, sem stack, SQL ou detalhe interno.

## Superfície HTTP

- Timeouts do `http.Server` (read header 5s, read 15s, write 30s, idle 120s) e do nginx são coordenados — mude juntos.
- CORS por allowlist exata (`CORS_ALLOWED_ORIGINS`); nunca `*` com `Authorization`.
- `ENABLE_PPROF`/`ENABLE_SWAGGER` `false` por padrão; pprof nunca público. Container distroless `nonroot`.
- Respostas só JSON: não gere HTML nem reflita entrada em `text/html` (XSS). CSRF não se aplica enquanto o
  token for `Bearer`; aceitar token por cookie exige plano com proteção CSRF.

## Lacunas conhecidas (não corrija de passagem)

- **Sem rate limiting** em nenhum ponto; `/auth/login` e `/auth/register` são prioridade (`limit_req` no nginx é
  o caminho mais simples). Só com pedido; aponte o risco ao mexer nesses endpoints.
- **Sem TLS** no nginx (`:80`); produção precisa de TLS à frente.

## Dependências

Biblioteca de segurança nova/atualizada: justifique, use a estável mais recente compatível, não rebaixe.
Ao mexer em dependências, rode `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` e relate o resultado.
