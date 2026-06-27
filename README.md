# Map Tile Fetcher

Map Tile Fetcher is a Go-based Web application for downloading map tiles by
either a drawn bounding box or administrative GeoJSON regions.

The former .NET range downloader has been retired after its range workflow was
ported into the Go app. Historical migration notes are in
[`docs/range-migration.md`](docs/range-migration.md).

## Repository Layout

- `apps/admin-region-tiler` - Go 1.25+ Web app with range download,
  administrative-region download, multiple map sources, child tasks, scheduled
  runs, SQLite control state, retry/failure records, Docker deployment, and
  artifact downloads.
- `docs/merge-plan.md` - current post-merge cleanup and architecture direction.
- `README_ZH.md` - Chinese README matching this document.

The repository does not include old restored source folders, UI design
packages, temporary directories, screenshots, runtime databases, downloaded
tiles, or historical release archives.

## Product Direction

- Backend: Go only.
- Database: SQLite as a lightweight control database for tasks, task runs,
  task sources, statuses, optional auth/session records, failures, artifacts,
  and scheduling.
- Storage: downloaded tiles, MBTiles files, ZIP archives, logs, and runtime data
  stay on the filesystem and out of Git.
- Web UI: one static frontend with two modes:
  - Range Download
  - Administrative Region Download

## Quick Validation

Validate the Go backend:

```powershell
cd apps/admin-region-tiler
go test ./...
```

Validate frontend scripts when changed:

```powershell
cd apps/admin-region-tiler
node --check .\static\script.js
```

Run the browser UI smoke test for login plus region and bbox task creation
payloads:

```powershell
cd apps/admin-region-tiler
node .\scripts\smoke_ui.mjs
```

The smoke script requires the Playwright package to be available locally or
globally. If needed, install it with `npm install -g playwright` and
`npx playwright install chromium`.

## Local Runtime Notes

Runtime secrets and generated data must stay local:

- Put real Tianditu or Mapbox tokens in local configuration only.
- Keep `.env`, `data/`, `output/`, `tiles/`, `bin/`, `obj/`, `publish*/`, and
  release archives out of Git.
- Placeholder values such as `YOUR_TIANDITU_TOKEN` and `YOUR_MAPBOX_TOKEN` are
  safe to keep in examples.

## Contribution Rule

Every validated change batch must be committed immediately. Each commit message
must include detailed English and Chinese sections plus the exact validation
commands that were run. See [`docs/commit-policy.md`](docs/commit-policy.md).

## License

This repository is released under the Apache License 2.0. See [`LICENSE`](LICENSE).
