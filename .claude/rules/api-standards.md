---
paths:
  - "internal/**/handler*.go"
  - "internal/**/model.go"
  - "internal/httputil/**"
  - "internal/router/**"
  - "internal/middleware/**"
  - "docs/swagger/**"
  - "http/**"
---

# Padrões da API HTTP

## Rotas

- Prefixo `/api/v1`, recursos no plural em minúsculas (`/purchases`, `/categories`).
- `GET /`, `POST /`, `GET /{id}`, `PUT /{id}` (substituição completa), `DELETE /{id}`. Sem `PATCH` sem necessidade real.
- `/health-check`, `/swagger/*`, `/debug/pprof/*` fora de `/api`; os dois últimos atrás de `ENABLE_SWAGGER`/`ENABLE_PPROF`.

## Handler

```go
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
    id, ok := parseID(w, r)                       // 400 se inválido
    if !ok { return }
    var req UpdateCategoryRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        httputil.WriteBadRequest(w, "invalid request body")
        return
    }
    c, err := h.svc.Update(r.Context(), id, &req)  // contexto carrega userID
    if err != nil { h.handleError(w, r, err); return }
    httputil.WriteJSON(w, http.StatusOK, c)
}
```

- Sempre `r.Context()`; handler não lê `userID`. Toda resposta via `httputil.WriteJSON`/`Write*`.
- `POST` de criação → `201` com o recurso; `DELETE` → `204` sem corpo.

## Status (contrato atual)

| Situação | Status | Helper / origem |
|---|---|---|
| JSON malformado, path/query inválido | 400 `BAD_REQUEST` | `WriteBadRequest` (handler) |
| Token ausente/inválido, credenciais inválidas | 401 `UNAUTHORIZED` | middleware / `ErrUnauthorized` |
| Operação proibida em recurso existente (categoria global) | 403 `FORBIDDEN` | `ErrForbidden` |
| Não existe **ou é de outro usuário** | 404 `NOT_FOUND` | `ErrNotFound` |
| Unicidade, recurso em uso | 409 `CONFLICT` | `ErrConflict` |
| Regra de negócio/validação | 422 `VALIDATION_ERROR` | `ErrValidation` |
| Qualquer outro | 500 `INTERNAL_SERVER_ERROR` | `WriteInternalError` (loga) |

- `handleError` por handler, `switch` com `errors.Is`, específicos antes dos genéricos
  (`category.ErrCategoryNotFound` antes de `apperrors.ErrNotFound` em `purchase/handler.go`).
- Só mensagens de `ErrValidation` (e `invalidCredentialsError`) vão ao cliente via `err.Error()`; o resto usa
  mensagem fixa. `default` loga com `logger.FromContext(r.Context()).Error("internal error in <domínio> handler", "error", err)`.
- Corpo de erro sempre `httputil.APIError` (`{"code": "...", "message": "..."}`); novo código só como constante em `httputil`.

## Contrato JSON

- `snake_case`; datas sem hora `purchase.Date` (`"YYYY-MM-DD"`); instantes `time.Time` (RFC 3339);
  dinheiro `decimal.Decimal` serializado como string (`"174.65"`); enums em MAIÚSCULAS.
- Request e response são tipos separados; cliente nunca envia `id`, `user_id`, `created_at`, `updated_at`.
- Sensíveis com `json:"-"` (`User.PasswordHash`). Lista vazia é `[]` (`items := []T{}`).
- Paginação: `purchase.PaginatedResult` (`items`, `total`, `page`, `page_size`, `total_pages`), defaults
  `page=1`, `page_size=20`, máximo validado no service.

## Swagger e `http/*.http`

- Handler público tem `@Summary`, `@Tags`, `@Accept/@Produce`, `@Param`, `@Success`, **todos** os `@Failure`
  com `httputil.APIError`, `@Router` (relativo a `/api/v1`). Mudou contrato ⇒ atualize e regenere (`CLAUDE.md`).
- `http/*.http`: exemplos em inglês, atualizados com o contrato.

## Compatibilidade

- Mudar nome/tipo de campo, status ou semântica é mudança de contrato: sinalize no plano/resumo.
  Campo opcional novo na resposta é compatível.
- Duas versões atendem ao mesmo cliente durante o deploy: mudança incompatível precisa aceitar formato antigo
  e novo por um período, ou nova versão de rota.
- `GET`/`PUT`/`DELETE` permanecem idempotentes; `POST` de criação não é (ver `reliability-and-performance.md`).
