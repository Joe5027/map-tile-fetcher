# Long-Term Memory

## Facts

- This repository is now a single Go Web application under
  `apps/admin-region-tiler`.
- The retired range downloader was a .NET 6 minimal API plus static frontend for
  bounding-box Tianditu tile downloads across `img`, `cia`, and `vec`.
- The range workflow has been ported into the Go app with bbox task creation,
  bbox tile math, Tianditu layer source creation, Leaflet range preview, tile
  estimates, shared worker execution, SQLite state, failures, and artifacts.
- The Go app uses Gin, SQLite, static frontend assets, administrative GeoJSON
  region resources, scheduling, worker execution, artifacts, deployment, and
  optional auth/session records.
- The global workspace contract requires `AGENTS.md`, `docs/project-map.md`,
  `docs/done-definition.md`, and `.codex/skills/`.
- The actual repository license is Apache License 2.0.

## Decisions

- Keep the first enhancement tranche workspace-local instead of modifying the
  global `.codex` control surface.
- Use `docs/project-map.md` as the primary context map for future sessions.
- Use `docs/done-definition.md` and `docs/validation-chain.md` as the local
  validation contract.
- Use `.codex/skills/two-projects-handoff/` as the local repository skill for
  scoped maintenance, release, validation, and cleanup work.
- Use `docs/automation-guardrails.md` for read-only recurring review prompts.
- Use Go as the only backend runtime.
- Retain SQLite as the task control database.
- Every validated change batch must be committed immediately with detailed
  English and Chinese commit notes.
- Keep the old .NET range downloader as documentation only in
  `docs/range-migration.md`; do not reintroduce runtime code unless explicitly
  requested.

## Assumptions

- Repository-level docs and local AI control surfaces are in scope because they
  directly support the Go application.
- Go and Node are expected to be available for validation; each future session
  should verify actual tool availability before claiming behavior.
- GeoJSON resources are intentional repository data; broad scans should exclude
  them unless the task is about region data.
- Real service tokens must stay out of Git.

## Validation

- `go test ./...` passed in `apps/admin-region-tiler`.
- `node --check apps/admin-region-tiler/static/script.js` passed.
- HTTP smoke passed for the Go app on port `18081`: `/`, Leaflet static asset,
  and `/api/auth/login` returned `200`.
- `docs/long-term-memory.md` passed the handoff contract validator.
- Sensitive-value scans found only documented placeholders and documented
  development defaults.
- Generated-file scans found no tracked or untracked runtime/build output after
  smoke cleanup.

## Next Action

- Add browser automation for the two UI modes when a local Playwright or
  equivalent dependency is available: login, switch to range mode, click two map
  points, verify bbox/estimate updates, create a scheduled bbox task, switch to
  region mode, and verify region task creation.
