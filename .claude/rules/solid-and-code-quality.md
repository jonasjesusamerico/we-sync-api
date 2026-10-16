---
paths:
  - "internal/**/*.go"
  - "cmd/**/*.go"
---

# SOLID e qualidade de código (Go, neste projeto)

SOLID aqui serve para manter módulos pequenos, testáveis e desacoplados — não é checklist para criar tipos.
Toda interface, struct, helper ou pacote novo responde: **que problema concreto isso resolve agora?**

## S — Responsabilidade única

Responsabilidades por arquivo: tabela em `architecture.md`.

- Formato de transporte (ID de path não numérico, `month=abc`) → handler, 400.
- Regra de negócio (obrigatório, enum, `amount > 0`, propriedade) → service, `apperrors.NewValidationError` → 422.
- Tradução de erro do Postgres → repository.

Funções extraídas têm propósito nomeável (`validateCategoryAccess`, `checkGlobalNameCollision`). Não fragmente
função linear e legível só para reduzir linhas.

## O — Aberto/fechado

Extensão por **novos valores e casos explícitos**, não por plugins:

- Novo status/forma de pagamento: constante + `case` em `IsValid()` + mensagem de validação + `CHECK` em nova
  migration + enum nas anotações swag.
- Novo erro de domínio: envolva um sentinela (`fmt.Errorf("category %w", apperrors.ErrNotFound)`) para o
  `handleError` continuar funcionando.

Sem strategy/registry/factory para 2–3 variações conhecidas.

## L — Substituição

Postgres e mocks respeitam o contrato documentado em `repository.go` (tabela em `persistence.md`). Mudou o
contrato ⇒ atualize comentário da interface, implementação, mocks e services no mesmo diff.

## I — Segregação de interfaces

**Aceite interfaces, retorne structs.** A interface pertence a quem **consome**, não a quem implementa:

- Construtores retornam o tipo concreto: `NewService(...) *Service`, `NewRepository(...) *PostgresRepository`.
  Não declare interface ao lado da implementação "para o caso de precisar".
- O consumidor declara, sem exportar, só os métodos que chama: `handler.go` tem `type service interface`;
  `purchase/service.go` tem `type categoryLookup interface { GetByID(...) }` em vez de receber
  `category.Repository` inteiro. Dependência nova entre domínios segue esse modelo.
- Exceção deliberada: `Repository` fica em `repository.go`, porque é a porta documentada do domínio
  (contrato da tabela em `persistence.md`) e o `Service` do mesmo pacote é quem a consome.
- A implementação garante o contrato em tempo de compilação: `var _ Repository = (*PostgresRepository)(nil)`.
- Interfaces pequenas; método novo só com caso de uso real. Exportar uma interface só quando outro pacote
  precisar dela.

## D — Inversão de dependência

- Service depende da interface `Repository`; handler da interface local `service`; o wiring em `main.go` passa
  os tipos concretos, que satisfazem as interfaces implicitamente.
- Sem globais mutáveis, `init()` com efeitos ou service locator. Config entra como valor (`jwtSecret`, TTLs).
- Sem interface para implementação única que não precisa de dublê (`httputil`, `apperrors`, validações puras).

## Quando abstrair

Só se: há 2+ implementações ou dublê de teste necessário; a mesma lógica não trivial aparece em **três**
lugares; ou isola dependência externa volátil. Duplicação tolerada hoje: `parseID` (category, purchase),
`dbAttrs`/`dbTimeout` por repositório — consolidar só como tarefa própria ou se já estiver no escopo.

Regra de negócio vive em um lugar só (o service). Invariantes (ex.: `amount > 0`, global imutável, dado só do
dono) são validadas no service **e** garantidas por constraint no banco. Cada tabela tem um dono: só o
repositório do seu domínio escreve nela. Simplificação consciente é registrada (comentário com o porquê ou resumo).

## Erros

- Compare sempre com `errors.Is`/`errors.As` (inclusive `sql.ErrNoRows`), nunca `==` — erros chegam envolvidos.
- Envolva com o método: `fmt.Errorf("purchaseService.Create: %w", err)`. Prefixos de erro e nomes de span
  mantêm o nome histórico em minúsculas (`purchaseService`, `categoryPostgresRepository`) mesmo com os tipos
  agora chamados `Service`/`PostgresRepository` — são contrato de observabilidade (traces, dashboards).
- Validação: `apperrors.NewValidationError("amount must be greater than zero")` — inglês, minúsculas, sem
  ponto final, citando o nome JSON do campo.
- `_` para erro só quando intencional e óbvio (`_ = json.Encode` na resposta, `Close` em caminho de erro);
  na dúvida, comente. Sem `panic` para fluxo de controle.

## Estilo

- Sem gagueira com o nome do pacote: `category.Service`, `category.NewHandler` (não `category.CategoryService`).
- Enums: `type Status string` + constantes + `IsValid()`.
- Comente o **porquê** e contratos não óbvios; doc comment em exportados.
- Constantes nomeadas para limites (`MaxPageSize`, `maxRequestBodyBytes`).
- Código linear e explícito; generics só para duplicação real; nada de parâmetros/flags "para o futuro".
- Antes de concluir: algo no diff pode ser removido sem perder requisito?
