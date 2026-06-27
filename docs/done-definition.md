# Done Definition

Completion in this repository means the change is scoped to the single Go
application surface, generated files stay out of Git, and the strongest
practical check for the touched surface has been run or explicitly recorded as
missing. A validated change batch is not complete until it has been committed
with the required bilingual commit message.

## Universal Criteria

- The change respects `AGENTS.md` repository scope.
- No real Tianditu, Mapbox, password, or access token is committed.
- Runtime and build outputs are not committed.
- Documentation agrees with actual source paths, license, commands, and app
  boundaries.
- The final report separates checks run, checks missing, assumptions, and any
  environment limit.
- Each validated change batch is committed before the next implementation batch
  starts.

## Validation Matrix

| Change surface | Required narrow check |
| --- | --- |
| `apps/admin-region-tiler` Go backend | From `apps/admin-region-tiler`: `go test ./...` |
| `apps/admin-region-tiler/static/script.js` | `node --check .\static\script.js`, plus `go test ./...` if API contracts changed |
| `apps/admin-region-tiler/scripts/smoke_ui.mjs` | `node --check .\scripts\smoke_ui.mjs`, then `node .\scripts\smoke_ui.mjs` with local or global Playwright available |
| `apps/admin-region-tiler/scripts/release_preflight.mjs` or CI workflow | `node --check .\scripts\release_preflight.mjs`, then `node .\scripts\release_preflight.mjs`; this includes Go tests, JS checks, UI smoke, sensitive-value scan, and tracked generated-file scan |
| `apps/admin-region-tiler/static/styles.css` or HTML-only polish | Browser or static inspection when available; otherwise state manual UI check missing |
| Config, release, or repository docs | Source-to-doc comparison plus the sensitive-value scan below |
| Local AI control-surface docs or skills | Workspace audit plus Markdown/source inspection; validate handoff docs with the handoff validator |

Sensitive-value scan:

```powershell
rg -n "TIANDITU|MAPBOX|TOKEN|PASSWORD|SECRET|adminmap|YOUR_" -g "!**/bin/**" -g "!**/obj/**" -g "!**/.git/**" -g "!**/data/**" -g "!**/output/**" -g "!**/tiles/**" -g "!**/geojson/**" .
```

Allowed findings are documented placeholders, environment variable names, and
the development default password `adminmap` when clearly described as a default
that must be overridden for production.

## Documentation Done

Documentation changes are done only when:

- paths and commands have been checked against the repository
- license references agree with `LICENSE`
- app responsibilities agree with `docs/project-map.md`
- merge guidance still matches `docs/merge-plan.md`
- long-running decisions are reflected in `docs/long-term-memory.md` when they
  affect future sessions

## Automation Done

Automation prompts or schedules are done only when:

- they are read-only by default
- they do not auto-edit tracked files
- they avoid starting downloads, writing runtime databases, or producing tile
  outputs
- they use the sections `Facts`, `Checks Run / Checks Missing`, `Risk`, and
  `Next Highest-Value Action`
- they name one highest-value recommendation instead of a broad backlog

## When A Check Is Missing

If a check cannot be run, record:

- exact command not run
- why it could not run
- what lower-signal check was run instead
- remaining risk

Do not claim a code or workflow change is fully proven when its strongest check
was skipped.
