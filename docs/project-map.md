# Project Map

This workspace is a clean handoff repository for two map tile downloader
applications. It is not the final merged product. Keep the two application
boundaries explicit until the merge interfaces are designed.

## Scope

In scope:

- `apps/range-downloader`
- `apps/admin-region-tiler`
- repository-level handoff, validation, release, and merge-planning docs

Out of scope unless the user explicitly changes direction:

- old restored source folders
- release packages, screenshots, UI design packages, temporary directories
- runtime databases, downloaded tiles, logs, archives, and local secrets
- direct code mixing between the two apps before interface boundaries exist

## Applications

| Path | Stack | Primary role | Entry points | Narrow validation |
| --- | --- | --- | --- | --- |
| `apps/range-downloader` | .NET 6 minimal API + static frontend | Bounding-box tile download for Tianditu `img`, `cia`, and `vec` layers | `Program.cs`, `wwwroot/index.html`, `wwwroot/app.js` | `dotnet build .\TianDiTuDownLoader.Web.csproj -c Release` |
| `apps/admin-region-tiler` | Go 1.25+, Gin, SQLite, static frontend | Administrative-region task engine with persistent plans, scheduled runs, multiple map sources, artifacts, and deploy assets | `main.go`, `server.go`, `runtime.go`, `task.go`, `db.go`, `static/script.js` | `go test ./...` |

## Range Downloader Map

- `Program.cs`
  - registers the minimal API, static files, HTTP client, and
    `TileDownloadManager`
  - exposes config, job lifecycle, layer lifecycle, failure records, archives,
    and preview tile proxy endpoints
  - contains the task model and tile math in one file, so changes here often
    need both API and behavior validation
- `TileDownloadManager`
  - stores live jobs and persisted `metadata.json`
  - restores stopped jobs and creates task/layer `.tar.gz` archives
  - deletion is constrained to `/data/tiles/tasks`
- `MultiLayerTileJob`
  - parent task coordinator for layer child jobs
  - resolves aggregate state from child layer state
- `LayerTileJob`
  - queues tile URLs, downloads to per-layer directories, records failed tiles,
    and supports stop, resume, and retry
- `wwwroot/app.js`
  - reads defaults from `/api/config`
  - draws or resets the bounding box, estimates tile counts, creates jobs,
    polls job state, and renders layer actions
  - includes coordinate conversion for AMap preview vs WGS84 download inputs

Important endpoints:

- `GET /api/config`
- `GET /api/jobs`, `GET /api/jobs/current`, `GET /api/jobs/{jobId}`
- `POST /api/jobs`
- `POST /api/jobs/{jobId}/stop|resume`
- `POST /api/jobs/{jobId}/layers/{layer}/stop|resume|retry`
- `DELETE /api/jobs/{jobId}`
- `GET /api/jobs/{jobId}/archive`
- `GET /api/jobs/{jobId}/layers/{layer}/archive`
- `GET /api/jobs/{jobId}/layers/{layer}/failures`
- `GET /api/tiles/{layer}/{z}/{x}/{y}`

## Admin Region Tiler Map

- `main.go`
  - loads `conf.toml`, environment overrides, logging, database, runtime
    manager, and HTTP server
  - supports worker mode via runtime flags/environment
- `internal/`
  - future single-app package boundaries for API, auth, config, area selection,
    planning, downloading, artifacts, and static Web helpers
  - `internal/area` validates bbox and region area selectors plus zoom ranges
  - `internal/planner` normalizes unified task requests before persistence or
    execution
- `server.go`
  - Gin routes for login, current user, task CRUD, task control, artifact
    download, map source config, region catalog, and GeoJSON file listing
  - validates task creation requests and builds parent/child plans
  - accepts legacy region `levels` requests and unified `mode: "bbox"`
    requests; bbox requests are converted to ignored `data/generated-areas`
    GeoJSON so the existing Go worker and artifact pipeline can run them
- `db.go`
  - SQLite schema, user/session records, plan records, run records, child plan
    relations, and interrupted-plan recovery
  - includes normalized forward schema tables `tasks`, `task_sources`,
    `artifacts`, and `failures`; current `plans`/`task_runs` writes mirror into
    these tables while the existing runtime remains compatible
- `runtime.go`
  - scheduler, active run coordination, worker launch, pause/resume/cancel,
    artifact preparation, purge safety, and parent status aggregation
- `worker_process.go`
  - isolated worker execution and run-progress persistence
- `task.go`
  - tile download engine, output setup, file/MBTiles writing, retry and throttle
    behavior, proxy rotation, request headers, final status, and in-run tile
    failure capture
- `internal/downloader`
  - contains bbox tile math shared by API validation and future range UI
    estimates
- `fetch_policy.go`
  - per-source fetch behavior, retryable status classification, proxy and
    throttling policy
- `utils.go`
  - GeoJSON path resolution, feature loading, tile response normalization, and
    output helpers
- `conf.toml`
  - default app, output, task, tilemap, fetch-policy, and download-region
    examples. Only placeholder tokens are safe in Git.
- `static/`
  - browser UI for authentication, task creation, progress, task controls,
    map-source and region selection
  - now exposes two creation modes: administrative-region downloads through the
    existing region catalog flow, and bounding-box Tianditu range downloads
    through the unified `/api/tasks` path
- `deploy/`
  - Linux, Nginx, and systemd deployment references
- `geojson/`
  - repository-shipped region resources. Avoid broad scanning unless the task is
    specifically about region data.

Important endpoints:

- `POST /api/auth/login`, `POST /api/auth/logout`
- protected `GET /api/auth/me`
- protected `POST /api/tasks`
- protected `GET /api/tasks`, `GET /api/tasks/:id`
- protected `PUT /api/tasks/:id/pause|resume`
- protected `DELETE /api/tasks/:id`, `DELETE /api/tasks/:id/purge`
- protected `GET /api/tasks/:id/download`
- protected `GET /api/tasks/:id/failures`
- protected `GET /api/maps`
- protected `GET /api/config/tilemaps`
- protected `GET /api/config/regions`
- protected `GET /api/config/region-catalog`
- protected `GET /api/config/geojson-files`

## Merge Direction

The current merge plan keeps `apps/admin-region-tiler` as the likely backend
base because it already has multi-task execution, persistence, scheduling,
region resources, artifacts, and deployment assets.

Use `apps/range-downloader` as the reference for bounding-box selection,
compact Tianditu download flow, layer-specific status, retry records, and user
experience around range preview.

Before moving code, define stable contracts for:

- map source configuration
- area selection: bounding box and GeoJSON regions
- task creation and child-task modeling
- task state and failure retry
- artifact creation and download

## Validation And Safety Anchors

- Source-of-truth license: Apache License 2.0 in `LICENSE` and `README.md`.
- Safe token placeholders include `YOUR_TIANDITU_TOKEN`, `YOUR_MAPBOX_TOKEN`,
  and `YOUR_MAPBOX_SKU`.
- `adminmap` is a documented development default password; production must use
  `.env` overrides.
- Generated or local-only paths stay out of Git: `.env`, `data/`, `output/`,
  `tiles/`, `bin/`, `obj/`, `publish*/`, logs, binaries, and archives.

## Context Loading Order

For future AI-assisted work, load context in this order:

1. `AGENTS.md`
2. `README.md`
3. `PROJECT_MANIFEST.md`
4. `docs/project-map.md`
5. `docs/done-definition.md`
6. the README and source files for the affected app
7. deeper files only when the current edit still lacks evidence

Avoid starting with broad reads of `apps/admin-region-tiler/geojson/` unless the
task is explicitly about region files.
