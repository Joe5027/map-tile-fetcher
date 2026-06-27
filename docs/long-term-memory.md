# Long-Term Memory

## Facts

- This repository is a clean GitHub handoff for two applications:
  `apps/range-downloader` and `apps/admin-region-tiler`.
- The range downloader is a .NET 6 minimal API plus static frontend for
  bounding-box Tianditu tile downloads across `img`, `cia`, and `vec`.
- The admin region tiler is a Go 1.25+ Gin/SQLite application for
  administrative-region tile tasks, scheduling, worker execution, artifacts,
  deployment, and region resources.
- The global workspace contract requires `AGENTS.md`, `docs/project-map.md`,
  `docs/done-definition.md`, and `.codex/skills/`.
- The first workspace audit found missing `docs/project-map.md`,
  `docs/done-definition.md`, and `.codex/skills/`.
- Runtime preflight showed `shell_command`, `node_repl`, `exa`, and
  `openaiDeveloperDocs` documented as exposed; local PowerShell is healthy.
- The actual repository license is Apache License 2.0.

## Decisions

- Keep the first enhancement tranche workspace-local instead of modifying the
  global `.codex` control surface.
- Use `docs/project-map.md` as the primary context map for future sessions.
- Use `docs/done-definition.md` and `docs/validation-chain.md` as the local
  validation contract.
- Use `.codex/skills/two-projects-handoff/` as the local repository skill for
  scoped maintenance, release, and merge work.
- Use `docs/automation-guardrails.md` for read-only recurring review prompts.
- Treat `apps/admin-region-tiler` as the likely backend base for the future
  merged product and `apps/range-downloader` as the bounding-box UX reference.

## Assumptions

- Repository-level docs and local AI control surfaces are in scope because they
  directly support the two allowed applications.
- App build tools are expected to be available, but each future session should
  verify actual `.NET`, Go, and Node availability before claiming code behavior.
- GeoJSON resources are intentional repository data; broad scans should exclude
  them unless the task is about region data.

## Validation

- Workspace audit after the enhancement reported no missing workspace contract
  checks and no high-signal drift.
- `docs/long-term-memory.md` passed the handoff contract validator.
- License scan now aligns on Apache License 2.0 across `LICENSE`, `README.md`,
  release handoff guidance, and the new AI control-surface docs.
- Sensitive-value scan found only documented placeholders, environment variable
  names, and the documented development default password `adminmap`.
- `node --check apps/range-downloader/wwwroot/app.js` passed.
- `node --check apps/admin-region-tiler/static/script.js` passed.
- `go test ./...` passed in `apps/admin-region-tiler`.
- `dotnet build .\TianDiTuDownLoader.Web.csproj -c Release` could not run in
  `apps/range-downloader` because no .NET SDK is installed in this environment.

## Next Action

- Install or make available a .NET 6+ SDK, then rerun the range downloader
  Release build before claiming the .NET app baseline is validated on this
  machine.
