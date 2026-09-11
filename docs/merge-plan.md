# Merge Plan

The repository has moved from a two-app handoff into a single Go application.
The target product offers two download modes in one Web backend:

- bounding-box range download
- administrative-region download

## Completed Merge Work

- Go is the only backend runtime kept in the repository.
- SQLite is retained as the task control database.
- Bounding-box request validation and tile math exist in Go.
- `/api/tasks` accepts both region and bbox task creation.
- The Web UI has two creation modes: range and administrative region.
- File-tree output is packaged as ZIP; MBTiles output remains a direct artifact.
- Failure records are persisted in SQLite and exposed through
  `GET /api/tasks/:id/failures`.
- The legacy `.NET` range downloader runtime has been retired.

## Current Base

Keep `apps/admin-region-tiler` as the base for:

- API and optional auth
- map source configuration
- area selection: bbox and GeoJSON regions
- task and child-source creation
- task status, pause, resume, cancel, and purge
- scheduled jobs
- SQLite task/run/source/artifact/failure metadata
- output packaging and downloads
- Docker and service deployment assets

## Remaining Cleanup

- Continue reducing old `plans` naming in favor of `tasks` only after migration
  compatibility is no longer needed.
- Keep cumulative retry artifacts, explicit reconciliation and source budgets
  covered by the real API lifecycle suite and repair acceptance contracts.
- Keep browser UI smoke and release preflight running in CI before release.
- Keep real service tokens out of Git and keep runtime outputs in ignored
  filesystem paths.

## Branch Convergence

`main` is the only long-term branch. PR #3 was merged as `8b0933b`; its main CI
passed and `agent/audit-hardening` was deleted. README follow-up PR #4 was merged
as `37cfe7c`, with main CI passing and its temporary branch removed.

For future batches, keep strict required checks (`Commit Message`,
`Admin Region Tiler`) enabled. Merge only the checked PR head with a complete
bilingual merge message, wait for the resulting `main` CI, then remove that
temporary branch and synchronize the local checkout. Preserve historical CI
failures and commit history. Current cleanup findings are tracked in
[the maintenance audit](maintenance-audit-2026-09-11.md).
