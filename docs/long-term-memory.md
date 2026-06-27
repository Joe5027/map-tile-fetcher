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
- On 2026-06-27, a repository-local AI enhancement tranche added
  `docs/ai-operating-handbook.md` and linked it from `AGENTS.md`,
  `docs/project-map.md`, `docs/validation-chain.md`,
  `docs/automation-guardrails.md`, `docs/knowledge-graph.md`,
  `.codex/skills/README.md`, and `.codex/skills/two-projects-handoff/SKILL.md`.
- On 2026-06-27, runtime preflight reported the current session's practical
  routes as `shell_command`, `node_repl`, `openaiDeveloperDocs`, and Exa, with
  no configured-not-exposed gap.
- On 2026-06-27, workspace audit found no high-signal drift before the AI
  enhancement tranche; global audit warned that `imagegen`, `openai-docs`,
  `plugin-creator`, and `skill-creator` are overgrown global skills.
- On 2026-06-27, `apps/admin-region-tiler/scripts/smoke_ui.mjs` was added as a
  Playwright browser smoke test for login, region task payload creation, and
  bbox task payload creation. It starts a temporary Go server by default and
  intercepts `/api/tasks` POSTs so it does not launch real tile downloads.

## Decisions

- Keep the first enhancement tranche workspace-local instead of modifying the
  global `.codex` control surface.
- Use `docs/project-map.md` as the primary context map for future sessions.
- Use `docs/done-definition.md` and `docs/validation-chain.md` as the local
  validation contract.
- Use `.codex/skills/two-projects-handoff/` as the local repository skill for
  scoped maintenance, release, validation, and cleanup work.
- Use `docs/automation-guardrails.md` for read-only recurring review prompts.
- Use `docs/ai-operating-handbook.md` as the compact route for repository-local
  AI enhancement, deep-execution, validation, automation, and memory work.
- Keep the AI operating handbook as a thin connector over existing repository
  docs, not a parallel methodology layer.
- Keep global overgrown-skill remediation outside this repository batch unless
  the user explicitly asks to change global `.codex` surfaces.
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
- Documentation-only AI control-surface changes do not require `go test ./...`
  or `node --check` unless app source files or frontend JavaScript changed.
- UI smoke automation changes require `node --check .\scripts\smoke_ui.mjs` and
  `node .\scripts\smoke_ui.mjs` with local or global Playwright available; keep
  generated database files, screenshots, traces, and tile outputs out of Git.
- Future sessions should rerun runtime preflight before making capability,
  MCP, or plugin availability claims because session exposure can change.
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
- 2026-06-27 AI-control tranche validation passed:
  `audit_environment.py --mode workspace --workspace . --format text`,
  `validate_handoff_contract.py --path docs\long-term-memory.md --format text`,
  repository AI-control path existence checks, stale two-application wording
  scan, and the sensitive-value scan from `docs/done-definition.md`.
- `go test ./...` and `node --check .\static\script.js` were intentionally not
  run for the 2026-06-27 AI-control tranche because it changed only repository
  docs and local skill guidance.
- 2026-06-27 UI smoke tranche validation passed:
  `go test ./...`, `node --check .\static\script.js`,
  `node --check .\scripts\smoke_ui.mjs`, and
  `node .\scripts\smoke_ui.mjs`.
- The 2026-06-27 UI smoke run verified login, region task creation payload
  shape, bbox mode switching, bbox estimate update, bbox task creation payload
  shape, and two accepted confirmation dialogs without launching real downloads.

## Next Action

- Run the new UI smoke script in CI or a release preflight once a stable
  Playwright browser cache is available, then decide whether to make it a
  required release gate alongside `go test ./...`.
