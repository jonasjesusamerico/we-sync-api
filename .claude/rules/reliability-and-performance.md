---
paths:
  - "cmd/**/*.go"
  - "internal/**/service.go"
  - "internal/**/repository*.go"
  - "internal/middleware/**"
  - "nginx.conf"
  - "docker-compose.yml"
---

# Concorrência, resiliência e limites

Contexto: **2 instâncias** atrás do nginx, mesmo PostgreSQL. Estado em memória é por instância, requisições
do mesmo usuário caem em instâncias diferentes, e versões distintas coexistem no deploy. Não há filas, cache
nem serviços externos — não introduza sem pedido. Regras de query eficiente estão em `persistence.md`.

## Limites e recursos

- Toda coleção que cresce por usuário tem limite (`MaxPageSize = 100`, body 1 MiB, tamanho máximo de texto).
  Endpoint que lista sem limite precisa de justificativa.
- Loops aninhados sobre dados de entrada/banco: use `map` no lado interno quando `n` não for trivial.
- Goroutine nova tem dono e condição de término (contexto/canal); nada de goroutine por request sem limite.
- Cache em memória só com pedido, tamanho máximo, TTL e invalidação — cada instância tem o seu e pode servir
  dado velho após escrita na outra.
- Não otimize sem medir (pprof local, métricas no Grafana).

## Concorrência e consistência

- **TOCTOU**: "verificar e depois gravar" em duas queries não é atômico. Garanta no banco (`UNIQUE`, `CHECK`,
  `UPDATE ... WHERE <condição> RETURNING`) e trate `23505` → `ErrConflict`.
  Exemplo: `checkGlobalNameCollision` (pessoal vs. global) é check-then-act, tolerado porque globais só mudam
  via migration; se virarem editáveis em runtime, a regra vai para o banco.
- Atualização relativa no SQL (`SET x = x + $1`), nunca ler-calcular-gravar em Go.
- `READ COMMITTED` basta hoje; `FOR UPDATE` ou isolamento maior só com justificativa e transação curta.
- Estado compartilhado em Go: `sync.Mutex`/`atomic` ou imutável após construção; teste com `-race`.
- **Idempotência**: `POST /purchases` não é idempotente — retry do cliente após timeout duplica o lançamento.
  O nginx não reenvia `POST` (padrão de `proxy_next_upstream`) — não habilite. Evitar duplicata exige proposta
  de chave de idempotência (header + `UNIQUE (user_id, idempotency_key)`) antes de implementar.

## Timeouts, cancelamento e retries

- Camadas coordenadas: nginx 30s ≥ `WriteTimeout` 30s > timeout de query. Nada espera indefinidamente.
- Propague `ctx`; não ignore `context.Canceled`/`DeadlineExceeded`. Várias escritas sem transação + cancelamento
  = estado parcial.
- Timeout de escrita **não** prova que falhou: o commit pode ter ocorrido e a resposta se perdido.
- Sem retry automático hoje; se pedido, só para operação idempotente e erro transitório, com limite, backoff e
  jitter. Circuit breaker/fallback só quando houver dependência externa.
- Graceful shutdown (10s em `main.go`) continua drenando requests e encerrando telemetria.

## Armadilhas

- Overflow: IDs/contadores em `int64`; calcule `offset` só após validar `page`/`page_size`.
- Texto: `TrimSpace` antes de validar/gravar; comparação de nomes case-insensitive via `lower(name)` no banco;
  limite em runes (`utf8.RuneCountInString`) para regra de caracteres, em bytes para limite técnico (bcrypt 72).
- Relógio: instâncias podem divergir um pouco (o JWT tolera 1 min no `iat`); para ordenar eventos gravados,
  prefira `NOW()` do banco.
