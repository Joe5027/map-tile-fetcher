# Map Tile Fetcher

Map Tile Fetcher is being merged into a single Go-based Web application for
downloading map tiles by either a drawn bounding box or administrative GeoJSON
regions.

The current repository still contains both source applications during the merge
transition. The Go application is the backend base; the .NET range downloader is
kept only as the reference for bounding-box selection and the simple Tianditu
download flow until those capabilities are fully ported.

## Repository Layout

- `apps/admin-region-tiler` - Go 1.25+ Web app for downloading tiles by
  administrative GeoJSON regions, with multiple map sources, tasks, scheduled
  runs, persistent state, Docker deployment, and output downloads.
- `apps/range-downloader` - legacy .NET 6 reference app for drawing a bounding
  box on a map, entering a Tianditu token, and downloading `img`, `cia`, and
  `vec` tile layers.
- `docs/merge-plan.md` - planned path for merging both apps into one product.
- `README_ZH.md` - Chinese README matching this document.

The repository does not include old restored source folders, UI design
packages, temporary directories, screenshots, runtime databases, downloaded
tiles, or historical release archives.

## Product Direction

- Backend: Go only.
- Database: SQLite as a lightweight control database for tasks, task runs,
  statuses, optional auth/session records, failures, artifacts, and scheduling.
- Storage: downloaded tiles, MBTiles files, ZIP/TAR archives, logs, and runtime
  data stay on the filesystem and out of Git.
- Web UI: a single static frontend with two modes:
  - Range Download
  - Administrative Region Download

## Quick Validation

Validate the Go backend:

```powershell
cd apps/admin-region-tiler
go test ./...
```

Validate the legacy range reference while it remains in the repository:

```powershell
cd apps/range-downloader
dotnet build .\TianDiTuDownLoader.Web.csproj -c Release
```

Validate frontend scripts when changed:

```powershell
node --check apps/admin-region-tiler/static/script.js
node --check apps/range-downloader/wwwroot/app.js
```

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
