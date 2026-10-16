---
paths:
  - "**/*_test.go"
  - "internal/**/service.go"
  - "internal/**/handler.go"
  - "internal/middleware/**"
  - "internal/config/**"
---

# Testes

## Estratégia atual

| Alvo | Como | Exemplo |
|---|---|---|
| Service | mock manual do repositório + `identity.WithUserID` no contexto | `internal/category/service_test.go` |
| Handler | `httptest.NewRecorder` + mock da interface local `service` + `withChiID` | `internal/category/handler_test.go` |
| Invariantes | propriedade com `rapid` | `purchase/service_test.go`, `middleware/jwt_test.go` |
| Middleware/config/util | teste direto | `jwt_test.go`, `env_properties_test.go`, `response_test.go` |
| Repositório, integração, contrato, e2e | **não há**; validação manual com `docker compose` + `http/*.http` | — |
| Carga/performance | k6 contra o compose (fora do `go test`) | `perf/k6/README.md` |

Só stdlib + `rapid`. Sem testify, gomock, mockery, testcontainers etc. sem pedido.

## O que testar

Comportamento observável, não implementação. Para cada operação nova/alterada:

- **Sucesso**: retorno; no handler, status + corpo.
- **Sem usuário no contexto** → `ErrUnauthorized`.
- **Validação**: cada regra (vazio, só espaços, enum inválido, `amount <= 0`, limite de página).
- **Isolamento**: recurso de outro usuário → `ErrNotFound`; o `userID` passado ao repositório é o do contexto (asserte no mock).
- **Erros do repositório**: `ErrConflict` propagado; erro genérico envolvido mantém `errors.Is`.
- **Handler**: cada erro → status/código, ID inválido, JSON malformado, `204` sem corpo no delete.
- **Limites**: zero, negativo, máximo, máximo+1.
- **Regressão**: bug corrigido ⇒ teste que falharia antes.
- **Invariante** que a mudança precisa preservar (ex.: "nunca retorna dado de outro usuário") — com `rapid`
  quando o espaço de entradas for amplo; não substitui casos de exemplo legíveis.
- **Concorrência**: estado compartilhado/goroutines/middlewares ⇒ chamadas paralelas + `-race`. Regras que
  dependem do banco sob concorrência (unicidade) não são cobertas — declare no resumo.

## Convenções

- `xxx_test.go` no mesmo pacote; nomes `TestService_<Op>_<Cenário>` / `TestHandler_<Op>_<Cenário>`.
- Mocks `mockXxx` com campos `XxxFn func(...)` e fallback neutro, em `*_mock_test.go`. Método novo na interface ⇒
  atualize **todos** os mocks (ex.: `category/repository_mock_test.go`). Mock de interface do consumidor implementa só os métodos dela
  (ex.: `purchase/category_lookup_mock_test.go` só tem `GetByID`).
- Fixtures como funções pequenas (`personalCategory(...)`, `ctxWithUser(...)`).
- Erros com `errors.Is`; comparar `err.Error()` só quando a mensagem é contrato (validação devolvida ao cliente).
- Table-driven quando os casos têm a mesma forma.
- Determinísticos: sem rede, banco, relógio real (fixe instantes) ou ordem de map.
- bcrypt custo 12 ≈ 200 ms: testes com muitos hashes (especialmente `rapid`) usam `withMinPasswordCost(svc)`.
- Spans: `go.opentelemetry.io/otel/sdk/trace/tracetest` (ver `TestRecordAuthError_RecordsOnlyUnexpectedErrors`).

## Conclusão

Rode a sequência de verificação do `CLAUDE.md` (+ `-race` quando aplicável) e relate o resultado real.
Teste falhando por motivo pré-existente: informe com a saída; nunca desative, use `t.Skip` ou afrouxe a asserção.
SQL/migrations não são cobertos pela suíte — declare no resumo.
