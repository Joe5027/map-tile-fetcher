# AI Operating Handbook

This is the compact operating entrypoint for AI-assisted work in this
repository. It links the existing control surfaces instead of replacing them.

## Purpose

- Preserve the user's preference for direct, evidence-based execution.
- Keep future AI sessions scoped to the single Go application.
- Make validation, memory, automation, and capability routing recoverable from
  versioned repository files.
- Avoid relying on chat history for restart-critical state.

## Scope

Use this handbook for repository-local AI control-surface work, including:

- rules and local skill alignment
- project map, knowledge graph, and long-term memory updates
- validation-chain and done-definition changes
- read-only automation prompt updates
- release or handoff readiness checks

Do not use it to justify adding retired .NET runtime code, generated runtime
data, release archives, temporary screenshots, UI design packages, or local
secrets. Reviewed documentation images follow the exception in `AGENTS.md`.

## Operating Route

Start substantial work from this packet:

1. `AGENTS.md`
2. `docs/project-map.md`
3. `docs/done-definition.md`
4. `docs/ai-operating-handbook.md`
5. `docs/validation-chain.md`
6. `docs/knowledge-graph.md` and `docs/long-term-memory.md` when continuity or
   release state matters
7. `apps/admin-region-tiler/README.md` and affected source files for code work

Stop loading context when the next edit or validation step is justified. Avoid
broad reads of `apps/admin-region-tiler/geojson/` unless the task is explicitly
about region resources.

## User Preferences Captured

- Prefer direct implementation over advice-only responses when the request asks
  for building, fixing, or strengthening the workspace.
- Ask for clarification only when a missing answer materially changes
  correctness or validation quality.
- Explore before major edits, but keep exploration narrowed to the current
  decision.
- Prefer deterministic checks, source-to-doc comparison, and explicit validation
  limits over confidence claims.
- Commit each validated change batch immediately with the required bilingual
  commit message.
- Keep recurring automation read-only unless the user explicitly requests write
  behavior.
- Apply the functionality/performance, dependency-review, branch-completion,
  and documentation rules in `AGENTS.md`; keep their authoritative text there.

## Control Surfaces

| Surface | Role |
| --- | --- |
| `AGENTS.md` | Repository rules, scope, commit discipline, and validation floor. |
| `.codex/skills/two-projects-handoff/SKILL.md` | Local routing for this map tile downloader workspace. |
| `.codex/skills/README.md` | Inventory and boundary for workspace-local skills. |
| `docs/project-map.md` | Primary architecture and context-loading map. |
| `docs/done-definition.md` | Local completion and validation standard. |
| `docs/validation-chain.md` | Ordered checks before completion and commit. |
| `docs/knowledge-graph.md` | Durable relationship map for fast recovery. |
| `docs/long-term-memory.md` | Handoff-style facts, decisions, assumptions, validation, and next action. |
| `docs/automation-guardrails.md` | Read-only recurring review posture and prompts. |

## Validation Route

For documentation or local skill changes, run from the repository root:

```powershell
node scripts/check_docs.mjs
```

```powershell
rg -n "TIANDITU|MAPBOX|TOKEN|PASSWORD|SECRET|adminmap|YOUR_" -g "!**/bin/**" -g "!**/obj/**" -g "!**/.git/**" -g "!**/data/**" -g "!**/output/**" -g "!**/tiles/**" -g "!**/geojson/**" .
```

```powershell
git status --short
```

For Go code changes, run `go test ./...` from `apps/admin-region-tiler`. For
frontend JavaScript changes, run `node --check .\static\script.js` from the app
directory. The local documentation check validates file/image links and the
handoff section contract; compare behavioral claims against their owning source
as well. An available environment skill may add its own audit, but another
developer's home directory or global skill installation is not a prerequisite.

## Memory Rules

Update `docs/long-term-memory.md` when a durable fact, decision, assumption,
validation result, or next action changes for merge, release, long-running, or
AI-control work.

Keep the handoff sections exactly:

1. `Facts`
2. `Decisions`
3. `Assumptions`
4. `Validation`
5. `Next Action`

Use `docs/knowledge-graph.md` for relationship recovery and
`docs/long-term-memory.md` for the latest restart-ready state. Keep current
status separate from historical evidence; link existing acceptance records
instead of accumulating completed tasks and machine capability inventories.

## Automation Rules

Recurring automation for this repository should:

- stay read-only by default
- avoid builds, service starts, downloads, tile output, database creation, and
  commits
- report with `Facts`, `Checks Run / Checks Missing`, `Risk`, and
  `Next Highest-Value Action`
- name exactly one highest-value recommendation
- escalate to a direct implementation turn when it finds real drift, secret
  exposure, generated files, invalid handoff memory, or stale validation
  commands

## Capability Routing

Current-session tool evidence wins over configured inventory. Check the actual
tool needed by the task. On Windows use PowerShell 7 explicitly; use `rg` and
the available patch tool for focused work. Environment preflight tools are
optional, environment-owned diagnostics, not repository dependencies. Treat
unexposed plugins as fallback routes, not hard dependencies.

## Maintenance Priorities

Browser automation and cumulative failure retries are implemented and covered
by the validation chain. Start from [current handoff state](long-term-memory.md)
and its acceptance links instead of recreating completed tasks. Keep legacy `plans`
tables while compatibility still depends on them. Global environment work is
outside repository maintenance.
