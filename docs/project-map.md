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
- release packages, runtime screenshots, UI design packages, temporary directories
- runtime databases, downloaded tiles, logs, archives, and local secrets

## Application

| Path | Stack | Primary role | Entry points | Narrow validation |
| --- | --- | --- | --- | --- |
| `apps/admin-region-tiler` | Go 1.27.3, Gin, SQLite, static frontend | Unified map tile downloader with bbox range mode, administrative-region mode, persistent tasks, scheduled runs, multiple map sources, failures, artifacts, and deploy assets | `main.go`, `server.go`, `runtime.go`, `task.go`, `db.go`, `static/script.js`, `scripts/smoke_ui.mjs`, `scripts/release_preflight.mjs` | `go test ./...`; `node --check .\static\script.js` when frontend JS changes; `node .\scripts\smoke_ui.mjs` for UI smoke; `node .\scripts\release_preflight.mjs` for full preflight |

## Admin Region Tiler Map

- `main.go`
  - loads `conf.toml`, environment overrides, logging, database, runtime
    manager, and HTTP server
  - supports worker mode via runtime flags/environment
- `internal/`
  - the three implemented packages shared by the main application
  - `internal/area` validates bbox and region area selectors plus zoom ranges
  - `internal/downloader` contains bbox tile math shared by API validation and
    range UI estimates
  - `internal/terrain` decodes Terrarium, samples shared boundaries, emits gzip
    quantized-mesh and validates complete ancestor availability
  - task normalization, authentication and artifact management remain in the
    main package; unused extraction scaffolding has been removed
- `server.go`
  - Gin routes for login, current user, task CRUD, task control, artifact
    download, failure records, map source config, region catalog, and GeoJSON
    file listing
  - accepts legacy region `levels` requests and unified `mode: "bbox"` requests
  - persists bbox area metadata directly and lets the Go worker enumerate bbox
    tiles without generated GeoJSON files
- `db.go`
  - SQLite schema, user/session records, legacy-compatible plan/run records,
    normalized `tasks`, `task_sources`, `artifacts`, and `failures` tables,
    child relations, and interrupted-plan recovery
- `password.go`, `admin.go`, `maintenance_lock_*.go`
  - authenticated password rotation, conditional session creation, attempt limits,
    and offline backup/recovery protected against active server/worker access
- `build_info.go`, `scripts/build_release.py`, `scripts/verify_package.py`
  - common build identity and deterministic Windows/Linux packages; native
    extraction, manifest, login, static-resource and loopback download validation
- `runtime.go`
  - scheduler, active run coordination, worker launch, pause/resume/cancel,
    artifact preparation, purge safety, and parent status aggregation
- `worker_process.go`
  - isolated worker execution, run-progress persistence, artifact finalization,
    and failure-record persistence
- `recovery_runtime.go`, `run_phase.go`
  - parent pipe lifecycle, run ownership/locks, same-run restart recovery,
    persisted phases, pause intent and slot release
- `mbtiles_batch.go`, `storage_space.go`
  - one SQLite writer with 256-tile/8 MiB/250 ms bounded transactions; count success
    only after commit; account for phase-specific additional disk space
- `terrain_pipeline.go`, `terrain_routes.go`
  - source metadata persists inside child levels; raw PNG and derived terrain
    remain separate, with bounded raster cache, reusable meshes and authenticated
    EPSG:3857/TMS preview routes
- `task_page.go`
  - 50-parent pages, effective global status counts and eight batch queries;
    legacy `/api/tasks` remains an array API
- `artifact_lifecycle.go`, `integrity.go`, `reconcile.go`, `disk_*.go`
  - run-isolated output paths, retryable deletion manifests, cumulative retry
    baselines, failure resolution, recoverable publication and explicit local
    historical-output reconciliation
- `limits.go`, `queue.go`, `enumeration.go`, `failure_stream.go`
  - parent tile budgets, persistent child execution slots, cancellable geometry
    enumeration, disk-backed deduplication and bounded retry batches
- `api_lifecycle_test.go`, `retry_integration_test.go`, `recovery_edges_test.go`
  - real application/worker processes and local tile fixtures; cumulative
    ZIP/MBTiles coordinates, migration copies and recovery edge cases
- `recovery_process_test.go`, `terrain_pipeline_test.go`, `task_page_test.go`
  - parent-kill recovery, paused capacity, actual elevation worker output and
    1,000-parent history fixtures
- `performance_fixture_test.go`, `scripts/benchmark_download.py`
  - the same 256-tile loopback workload across Git revisions; sampled heap/disk,
    first committed tile, elapsed time and connection count (ZIP packing excluded)
- `scripts/benchmark_workflow.py`, `scripts/workflow_acceptance.mjs`
  - whole API/worker/publication resource measurements and nine real browser
    action chains across area modes and responsive widths
- `acceptance_*_test.go`
  - plan-gap regressions for geometry, preparation progress, single-read retry,
    bounded copy space, queue responsiveness and vector preservation
- `repair_contract_test.go`
  - portable assertions runnable against both archived pre-repair code and the
    current app; evidence is indexed in `docs/repair-acceptance-2026-09-11.md`
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
  - administrative-region and rectangle/polygon modes share the source catalogue
    and `/api/tasks`; keyed task-page updates preserve interaction state
  - `terrain-preview.html/js` loads local Cesium assets prepared by locked npm
    dependencies; no ion or online imagery is needed
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
  - includes tracked-file sensitive-value and generated-file scans before
    release completion
- `.github/workflows/validate.yml`
  - GitHub Actions validation entrypoint that installs Go, Node, browser
    automation dependencies, and runs the release preflight
- `geojson/`
  - repository-shipped region resources. Avoid broad scanning unless the task is
    specifically about region data.

Important endpoints:

- `POST /api/auth/login`, `POST /api/auth/logout`
- public read-only `GET /api/version`
- protected `POST /api/auth/password`
- protected `GET /api/auth/me`
- protected `POST /api/tasks`
- protected `GET /api/tasks`, `GET /api/tasks/:id`
- protected `GET /api/tasks/page?limit=50&offset=0&status=all`
- protected `PUT /api/tasks/:id/pause|resume`
- protected `DELETE /api/tasks/:id`, `DELETE /api/tasks/:id/purge`
- protected `GET /api/tasks/:id/download`
- protected `GET /api/tasks/:id/terrain-info`, `GET /api/tasks/:id/terrain/*path`
- protected `GET /api/tasks/:id/failures`
- protected `POST /api/tasks/:id/retry-failures|reconcile|recreate`
- protected `GET /api/config/limits`
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
- `adminmap` is a documented development default password. Configure initial
  production credentials before the first start through process environment
  variables or local configuration; Docker injects `.env`. Existing users are
  not reset by changing initial-user settings.
- Generated or local-only paths stay out of Git: `.env`, `data/`, `output/`,
  `tiles/`, `bin/`, `obj/`, `publish*/`, logs, binaries, and archives.
- Browser UI checks use project-local Playwright from `package-lock.json`.
  Run `npm ci` and install its Chromium; do not depend on global packages.
- Runtime screenshots, traces, databases and downloads stay ignored. Reviewed
  fixture-based documentation images may enter `docs/assets/` with reproduction
  instructions, as specified by `AGENTS.md`.

## AI Control Surface Map

| Path | Role | When to load |
| --- | --- | --- |
| `docs/ai-operating-handbook.md` | Compact operating entrypoint for AI-assisted repository work | Deep-execution, AI enhancement, local skill, validation, automation, or memory updates |
| `.codex/skills/two-projects-handoff/SKILL.md` | Repository-local skill for scoped map tile downloader maintenance | Any release, validation, merge, or AI-control task in this workspace |
| `docs/validation-chain.md` | Ordered validation and commit gate | Before claiming completion or committing a validated batch |
| `docs/knowledge-graph.md` | Durable relationship graph | When recovering context or avoiding broad re-exploration |
| `docs/long-term-memory.md` | Handoff-style restart state | Long-running, merge, release, or AI-control work |
| `docs/automation-guardrails.md` | Read-only recurring review constraints | Automation prompt creation or review |
| `docs/maintenance-audit-2026-09-11.md` | Cleanup decisions and remaining maintenance issues | Before deleting more files or planning follow-up fixes |

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
