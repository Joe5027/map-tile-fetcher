---
name: two-projects-handoff
description: Use for repository-specific maintenance, release preparation, validation, and merge planning in this two-application map tile downloader handoff workspace.
---

# Two Projects Handoff

Use this local skill when work is about this repository's two applications,
their release readiness, validation path, or future merge plan.

## Trigger Conditions

Apply this skill when the task mentions:

- `apps/range-downloader`
- `apps/admin-region-tiler`
- this repository's GitHub handoff, release, license, validation, or merge plan
- range download vs administrative-region download unification
- repository-local AI docs, project map, done definition, or handoff memory

Do not use this skill for unrelated global `.codex` changes unless the user
explicitly asks to connect this repository to global operating policy.

## Required Context Order

1. `AGENTS.md`
2. `README.md`
3. `PROJECT_MANIFEST.md`
4. `docs/project-map.md`
5. `docs/done-definition.md`
6. the README and source files for the affected app

Avoid broad reads of `apps/admin-region-tiler/geojson/` unless the task is
about region resources.

## Repository Rules

- Work only on the two in-repository applications and repository-level support
  docs or local skills that directly support them.
- Do not import old local source folders, release packages, UI packages,
  screenshots, runtime databases, downloaded tiles, logs, binaries, archives, or
  machine-local run data.
- Keep placeholders for service tokens; do not commit real tokens.
- Preserve the detailed bilingual commit-message requirement.

## Validation Route

- Range downloader code: run
  `dotnet build .\TianDiTuDownLoader.Web.csproj -c Release` from
  `apps/range-downloader`.
- Admin region tiler code: run `go test ./...` from
  `apps/admin-region-tiler`.
- Changed frontend JavaScript: run `node --check` on the changed file.
- Repository docs or local skill changes: run the workspace audit and source
  consistency checks from `docs/validation-chain.md`.
- Handoff-memory changes: validate `docs/long-term-memory.md` with the global
  handoff validator.

## Merge Guidance

- Treat `apps/admin-region-tiler` as the backend base unless new evidence says
  otherwise.
- Treat `apps/range-downloader` as the range-selection UX and simple layer-task
  reference.
- Do not merge by copying files between apps first. Define contracts for map
  sources, area selection, task creation, task status, retry records, and
  artifacts before porting code.

## Output Expectation

Close with:

- surfaces changed
- checks run and checks missing
- app or repo risk
- next highest-value action
