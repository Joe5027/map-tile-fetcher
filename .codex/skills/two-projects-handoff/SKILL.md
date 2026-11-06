---
name: two-projects-handoff
description: Use for repository-specific maintenance, release preparation, validation, AI control-surface updates, and post-merge cleanup in this single Go map tile downloader workspace.
---

# Map Tile Fetcher Handoff

Use this local skill when work is about this repository's Go application, release
readiness, validation path, or post-merge cleanup.

## Trigger Conditions

Apply this skill when the task mentions:

- `apps/admin-region-tiler`
- this repository's GitHub handoff, release, license, validation, or merge plan
- range download vs administrative-region download unification
- repository-local AI docs, project map, knowledge graph, validation chain,
  automation guardrails, done definition, or handoff memory

Do not use this skill for unrelated global `.codex` changes unless the user
explicitly asks to connect this repository to global operating policy.

## Required Context Order

1. `AGENTS.md`
2. `README.md`
3. `PROJECT_MANIFEST.md`
4. `docs/project-map.md`
5. `docs/done-definition.md`
6. `docs/ai-operating-handbook.md` for repository-local AI control-surface work
7. `docs/validation-chain.md`
8. the README and source files for the affected app

Avoid broad reads of `apps/admin-region-tiler/geojson/` unless the task is
about region resources.

## Repository Rules

- Work only on the Go application and repository-level support docs or local
  skills that directly support it.
- Do not reintroduce the retired .NET range downloader runtime code; keep
  historical context in `docs/range-migration.md`.
- Do not import old local source folders, release packages, UI packages,
  temporary screenshots, runtime databases, downloaded tiles, logs, binaries, archives, or
  machine-local run data.
- Reviewed documentation images follow `AGENTS.md` and stay in `docs/assets/`.
- Keep placeholders for service tokens; do not commit real tokens.
- Preserve the detailed bilingual commit-message requirement.
- Apply the functionality/performance and stable-dependency review rules in
  `AGENTS.md`; document concrete benefit and compatibility before upgrading.

## Validation Route

- Admin region tiler code: run `go test ./...` from
  `apps/admin-region-tiler`.
- Changed frontend JavaScript: run `node --check` on the changed file.
- Repository docs, local skill, or handoff changes: run
  `node scripts/check_docs.mjs` from the repository root, compare with source,
  and use the sensitive-value scan in `docs/validation-chain.md`.
- Environment-owned audits may supplement these checks when available; do not
  require a personal home directory or global skill to validate this repository.

## Merge Guidance

- Treat `apps/admin-region-tiler` as the backend base unless new evidence says
  otherwise.
- Treat `docs/range-migration.md` as the historical note for the ported .NET
  range workflow.
- Keep contracts for map sources, area selection, task creation, task status,
  retry records, and artifacts explicit when changing the unified app.
- Complete the `AGENTS.md` branch rule through `docs/merge-plan.md`: merge and
  validate all development work on `main`, then remove integrated non-main
  branches locally and remotely and verify the actual final branch lists.

## Output Expectation

Close with:

- surfaces changed
- checks run and checks missing
- app or repo risk
- next highest-value action
