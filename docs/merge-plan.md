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
- Improve failure retry UX on top of the persisted failure records.
- Add browser automation for range-mode and region-mode smoke tests when a
  local browser automation dependency is available.
- Keep real service tokens out of Git and keep runtime outputs in ignored
  filesystem paths.
