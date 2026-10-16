# Specs (Spec-Driven Development)

Every non-trivial change starts as a spec. The spec is the source of truth for **what** and **why**;
the code is the source of truth for **how it actually works**. When they diverge, fix one of them in the
same change.

## Flow

| Phase | Artifact | Answers | Gate |
|-------|----------|---------|------|
| 1. Specify | `spec.md` | What and why: behavior, rules, acceptance criteria. No implementation details. | Open questions resolved, spec `Approved` |
| 2. Plan | `plan.md` | How: affected packages, schema, API contract, risks, test strategy. | Plan reviewed against `.claude/rules/` |
| 3. Tasks | `tasks.md` | Small, ordered, verifiable steps traced to acceptance criteria. | Every criterion covered by a task |
| 4. Implement | code + tests | Execute tasks one by one, checking them off. | `gofmt`, `go vet`, `go build`, `go test` pass |

Small changes (bug fix, copy change, internal refactor without contract change) may skip the spec.

## Layout

```
docs/specs/
  README.md
  _templates/            spec.md, plan.md, tasks.md
  NNN-short-name/        one folder per feature, NNN = next sequential number
    spec.md
    plan.md
    tasks.md
```

## Status values

`Draft` → `Approved` → `In progress` → `Done` (or `Superseded by NNN`).

## Conventions

- Written in English (see `CLAUDE.md`).
- Acceptance criteria are numbered (`AC-1`, `AC-2`, ...) and referenced from `plan.md` and `tasks.md`.
- Never include secrets, real tokens or personal data in examples.
- An approved spec is changed only with a dated entry in its `Changelog` section.
