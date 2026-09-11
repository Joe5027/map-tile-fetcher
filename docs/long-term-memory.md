# Long-Term Memory

## Facts

- M4 investigated all five catalog gaps. No verified complete WGS84 boundaries
  were obtained; preserve explicit reasons and evidence in
  `docs/region-gap-investigation-2026-09-11.md`. The gap gate rejects new missing
  files and stale exceptions. Do not approximate geometry from announcements.

- M3 adds authenticated password changes, transactional session revocation,
  conditional login/legacy upgrades and offline recovery with shared/exclusive
  maintenance locks. See `docs/password-recovery.md`; offline recovery never
  runs ordinary initialization or task recovery.

- Maintenance implementation now uses `codex/maintenance-fixes` from `1109e4d`.
  M1/M2/M7 replace direct region deployment with offline inspection and new,
  validated bundles. Regression and remaining batch status are tracked in
  `docs/maintenance-acceptance-2026-09-11.md`; no production or Release actions.

- 2026-09-11 repository cleanup: removed 24 redundant docs, unused package
  scaffolding, unreferenced static files and a generated report. Actual region
  geometry, runtime compatibility tables and active UI/test assets remain.
  `docs/maintenance-audit-2026-09-11.md` records the deletion evidence and seven
  unresolved maintenance issues, including five confirmed missing region files.
- README follow-up PR #4 merged as `37cfe7c`; its main CI passed and the
  temporary `codex/readme-sync` branch was removed locally and remotely.
- 2026-09-11 README follow-up: root documentation now covers repair behavior,
  limits, legacy handling and validation. Source/binary runs read process
  environment variables, not `.env` automatically; initial credentials do not
  reset existing users. GitHub's published `v0.3.0` preview predates the repairs;
  current behavior refers to `main`. PR #3 was merged as `8b0933b`, its resulting
  main CI passed, and `agent/audit-hardening` was removed locally and remotely.
- 2026-09-11 repair delivery: the ten-defect implementation, baseline failure
  evidence, current regression results and migration procedures are indexed in
  `docs/repair-acceptance-2026-09-11.md`. Use that file before repeating audits.
- Real API tests now build the application, start ordinary worker processes,
  and verify queue/control/retry/artifact/deletion/restart behavior with isolated
  SQLite and loopback tile fixtures. UI smoke alone is not the integration proof.
- Repair commits retained the PR #3 security baseline and added run isolation,
  recoverable publication, cumulative retries, historical reconciliation,
  persistent queues, budgets, preview renewal and responsive UI.
- Remote protection was rechecked on 2026-09-11: PR required, strict latest
  `Commit Message` and `Admin Region Tiler` checks, admin enforcement enabled,
  force pushes and branch deletion disabled on `main`.

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
- On 2026-06-27, `apps/admin-region-tiler/scripts/release_preflight.mjs` and
  `.github/workflows/validate.yml` were added so local release checks and
  GitHub Actions use the same validation route.

## Decisions

- Keep `main` as the only long-term branch. PR #3 and its branch cleanup are
  complete. For future batches, merge the checked PR head and remove its
  temporary branch only after the resulting main CI succeeds.
- Defaults: 1,000,000 tiles per parent across sources, three active children,
  paused children retaining their slot, and three requested workers (1-50).
- Preserve legacy records and artifacts; reconcile explicitly without tile
  downloads. Missing successful baselines require an explicit full recreation.
- This delivery excludes production deployment and execution of production
  data repair. Do not infer production permission from code-delivery approval.

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
- Release preflight or CI workflow changes require
  `node --check .\scripts\release_preflight.mjs` and
  `node .\scripts\release_preflight.mjs`.
- Future sessions should rerun runtime preflight before making capability,
  MCP, or plugin availability claims because session exposure can change.
- GitHub Actions workflow syntax could not be locally linted on 2026-06-27
  because `actionlint` and a local YAML parser were unavailable; the first
  remote workflow run remains the source of truth for runner-specific behavior.
- GeoJSON resources are intentional repository data; broad scans should exclude
  them unless the task is about region data.
- Real service tokens must stay out of Git.

## Validation

- 2026-09-11: eight portable backend contracts failed on archived `e2cfcdb` and
  passed on the repaired code. The old renderer accepted a script download URL;
  old CSS produced a 1080px document at a 390px viewport. Repaired checks passed.
- Full local release preflight passed, including real API integration,
  ZIP/MBTiles coordinate checks, rendering safety, 390/768/1440px layouts,
  16-minute preview renewal, response ordering and 401/re-login polling.
- `go vet ./...` passed locally. Windows lacks a C compiler for local race
  instrumentation; Linux CI runs `go test -race ./...` and `go vet ./...`.
- Linux CI for `677186d` and preceding repair batches passed; the final PR head
  and resulting main merge must each pass the same checks before branch cleanup.
- Legacy database copy migration was repeated successfully without changing
  the source database or queuing downloads. No production database was used.

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
- 2026-06-27 release preflight tranche validation passed:
  `node --check .\scripts\release_preflight.mjs` and
  `node .\scripts\release_preflight.mjs`.
- Local workflow validation on 2026-06-27 was limited to manual source
  inspection because `actionlint` was not installed and Ruby/YAML parsing was
  unavailable in the session.

## Next Action

- M3 password rotation passed local preflight and vet. Continue M4 documented region gaps and M5/M6 build
  identity and packaging on the maintenance branch. Commit/push each verified
  batch, merge only after all validation and then verify new main CI.
- Before any future release, rerun preflight and verify `main` CI. Plan a
  separate, explicitly authorized deployment and database backup/copy validation.
