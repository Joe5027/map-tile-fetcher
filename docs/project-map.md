# Project Map

This repository is now a single Go Web application for map tile downloads. The
former .NET range downloader has been retired after its bbox workflow was ported
into the Go app.

## Scope

In scope:

- `apps/admin-region-tiler`
- repository-level docs, validation, release, and post-merge cleanup surfaces

Out of scope unless the user explicitly changes direction:

- old restored source folders
- retired .NET runtime code
- release packages, screenshots, UI design packages, temporary directories
- runtime databases, downloaded tiles, logs, archives, and local secrets

## Application

| Path | Stack | Primary role | Entry points | Narrow validation |
| --- | --- | --- | --- | --- |
| `apps/admin-region-tiler` | Go 1.25+, Gin, SQLite, static frontend | Unified map tile downloader with bbox range mode, administrative-region mode, persistent tasks, scheduled runs, multiple map sources, failures, artifacts, and deploy assets | `main.go`, `server.go`, `runtime.go`, `task.go`, `db.go`, `static/script.js`, `scripts/smoke_ui.mjs`, `scripts/release_preflight.mjs` | `go test ./...`; `node --check .\static\script.js` when frontend JS changes; `node .\scripts\smoke_ui.mjs` for UI smoke; `node .\scripts\release_preflight.mjs` for full preflight |

## Admin Region Tiler Map

- `main.go`
  - loads `conf.toml`, environment overrides, logging, database, runtime
    manager, and HTTP server
  - supports worker mode via runtime flags/environment
- `internal/`
  - package boundaries for API, auth, config, area selection, planning,
    downloading, artifacts, and static Web helpers
  - `internal/area` validates bbox and region area selectors plus zoom ranges
  - `internal/planner` normalizes unified task requests before persistence or
    execution
  - `internal/downloader` contains bbox tile math shared by API validation and
    range UI estimates
- `server.go`
  - Gin routes for login, current user, task CRUD, task control, artifact
    download, failure records, map source config, region catalog, and GeoJSON
    file listing
  - accepts legacy region `levels` requests and unified `mode: "bbox"` requests
  - converts bbox requests to ignored `data/generated-areas` GeoJSON so the
    existing Go worker and artifact pipeline can run them
- `db.go`
  - SQLite schema, user/session records, legacy-compatible plan/run records,
    normalized `tasks`, `task_sources`, `artifacts`, and `failures` tables,
    child relations, and interrupted-plan recovery
- `runtime.go`
  - scheduler, active run coordination, worker launch, pause/resume/cancel,
    artifact preparation, purge safety, and parent status aggregation
- `worker_process.go`
  - isolated worker execution, run-progress persistence, artifact finalization,
    and failure-record persistence
- `task.go`
  - tile download engine, output setup, file/MBTiles writing, retry and throttle
    behavior, proxy rotation, request headers, final status, and tile failure
    capture
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
  - exposes two creation modes: administrative-region downloads through the
    existing region catalog flow, and bounding-box Tianditu range downloads
    through the unified `/api/tasks` path
- `deploy/`
  - Linux, Nginx, and systemd deployment references
- `scripts/smoke_ui.mjs`
  - browser smoke test for login, administrative-region payload creation, and
    bounding-box payload creation
  - starts a temporary Go server by default and intercepts `/api/tasks` POSTs
    so smoke runs do not launch real downloads
- `scripts/release_preflight.mjs`
  - one-command release preflight for Go tests, JavaScript syntax checks, and
    UI smoke validation
- `.github/workflows/validate.yml`
  - GitHub Actions validation entrypoint that installs Go, Node, browser
    automation dependencies, and runs the release preflight
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

## Migration Note

The retired `apps/range-downloader` app originally supplied the simpler
Tianditu range workflow. Its useful behavior was ported into the Go app:

- map click bbox selection
- bbox preview and tile estimate
- Tianditu token input
- `img`, `cia`, and `vec` layer source creation
- range tasks through `/api/tasks`

See `docs/range-migration.md` for the historical note.

## Validation And Safety Anchors

- Source-of-truth license: Apache License 2.0 in `LICENSE` and `README.md`.
- Safe token placeholders include `YOUR_TIANDITU_TOKEN`, `YOUR_MAPBOX_TOKEN`,
  and `YOUR_MAPBOX_SKU`.
- `adminmap` is a documented development default password; production must use
  `.env` overrides.
- Generated or local-only paths stay out of Git: `.env`, `data/`, `output/`,
  `tiles/`, `bin/`, `obj/`, `publish*/`, logs, binaries, and archives.
- Browser UI smoke validation uses an optional local or global browser
  automation package. If that package is unavailable, run the relevant
  `node --check` command, perform a manual browser check for the touched flow,
  and record the missing automated smoke as a validation limit.
- UI smoke runs should not commit screenshots, traces, runtime databases, or
  generated downloads.

## AI Control Surface Map

| Path | Role | When to load |
| --- | --- | --- |
| `docs/ai-operating-handbook.md` | Compact operating entrypoint for AI-assisted repository work | Deep-execution, AI enhancement, local skill, validation, automation, or memory updates |
| `.codex/skills/two-projects-handoff/SKILL.md` | Repository-local skill for scoped map tile downloader maintenance | Any release, validation, merge, or AI-control task in this workspace |
| `docs/validation-chain.md` | Ordered validation and commit gate | Before claiming completion or committing a validated batch |
| `docs/knowledge-graph.md` | Durable relationship graph | When recovering context or avoiding broad re-exploration |
| `docs/long-term-memory.md` | Handoff-style restart state | Long-running, merge, release, or AI-control work |
| `docs/automation-guardrails.md` | Read-only recurring review constraints | Automation prompt creation or review |

## Context Loading Order

For future AI-assisted work, load context in this order:

1. `AGENTS.md`
2. `README.md`
3. `PROJECT_MANIFEST.md`
4. `docs/project-map.md`
5. `docs/done-definition.md`
6. `docs/ai-operating-handbook.md` for repository-local AI control-surface work
7. `docs/knowledge-graph.md` and `docs/long-term-memory.md` when continuity,
   release state, or relationship recovery matters
8. `apps/admin-region-tiler/README.md` and the affected source files
9. deeper files only when the current edit still lacks evidence

Avoid starting with broad reads of `apps/admin-region-tiler/geojson/` unless the
task is explicitly about region files.
