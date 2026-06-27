# Automation Guardrails

Recurring automation for this repository must stay low-side-effect by default.
It should review and report, not edit files, launch downloads, or publish
artifacts.

## Default Posture

- Read-only unless the user explicitly asks for write behavior.
- No automatic commits, pushes, dependency upgrades, service starts, tile
  downloads, or external-system writes.
- Avoid commands that intentionally create runtime data, databases, downloaded
  tiles, build outputs, or archives during a recurring review.
- Keep output short and actionable.

## Required Output Sections

Every recurring review should use exactly these sections:

- `Facts`
- `Checks Run / Checks Missing`
- `Risk`
- `Next Highest-Value Action`

Clean anomaly-only runs may be archived silently when the automation platform
supports it. Blocked runs and high-signal findings must remain visible.

## Workspace Review Prompt

Use this as the base prompt for a read-only workspace review:

```text
Review the current repository as a read-only Codex workspace harness check.

Facts:
- Start from AGENTS.md, docs/project-map.md, docs/done-definition.md, docs/ai-operating-handbook.md, and git status.
- Confirm the repository only contains the in-scope app under apps/admin-region-tiler.
- Confirm retired .NET range downloader runtime code has not reappeared.
- Check for drift between README.md, PROJECT_MANIFEST.md, RELEASE_HANDOFF_PROMPT.md, LICENSE, and docs/merge-plan.md.
- Check that docs/knowledge-graph.md still names the current single-app architecture and AI control surfaces.
- Check that docs/long-term-memory.md still follows the handoff contract if it exists.
- Check for sensitive token or password literals, allowing documented placeholders and the documented development default admin password only.

Checks Run / Checks Missing:
- Run read-only scans only.
- Do not run builds, start servers, download tiles, create databases, write archives, or edit files during this recurring review.
- Do not run `scripts/smoke_ui.mjs` during recurring read-only reviews unless
  the automation was explicitly authorized to start a temporary local server and
  browser.
- List any stronger checks that should be run manually or in an explicit implementation turn.

Risk:
- Report only real drift, missing validation, scope creep, generated-file risk, or secret-handling risk.

Next Highest-Value Action:
- Name exactly one concrete fix or validation action.
```

## Recommended Read-Only Commands

```powershell
git status --short
```

```powershell
python C:\Users\32674\.codex\skills\deep-execution-upgrade\scripts\audit_environment.py --mode workspace --workspace . --format text
```

```powershell
rg -n "Apache|MIT|License|许可证|许可" README.md PROJECT_MANIFEST.md RELEASE_HANDOFF_PROMPT.md docs LICENSE
```

```powershell
rg -n "TIANDITU|MAPBOX|TOKEN|PASSWORD|SECRET|adminmap|YOUR_" -g "!**/bin/**" -g "!**/obj/**" -g "!**/.git/**" -g "!**/data/**" -g "!**/output/**" -g "!**/tiles/**" -g "!**/geojson/**" .
```

```powershell
python C:\Users\32674\.codex\skills\deep-execution-upgrade\scripts\validate_handoff_contract.py --path docs\long-term-memory.md --format text
```

## AI Control-Surface Review Prompt

Use this prompt when the recurring review is specifically about AI harness
drift:

```text
Review the current repository as a read-only AI control-surface drift check.

Facts:
- Start from AGENTS.md, docs/project-map.md, docs/ai-operating-handbook.md, docs/validation-chain.md, docs/knowledge-graph.md, docs/long-term-memory.md, .codex/skills/README.md, and .codex/skills/two-projects-handoff/SKILL.md.
- Confirm the local skill still describes the repository as a single Go map tile downloader workspace.
- Confirm AI guidance still routes through existing docs instead of creating a parallel methodology layer.
- Confirm validation commands match docs/done-definition.md and docs/validation-chain.md.
- Confirm automation guidance remains read-only by default.

Checks Run / Checks Missing:
- Run read-only scans only.
- Do not edit files, start servers, run downloads, create databases, write archives, or commit.
- List any stronger validation that should be run in an explicit implementation turn.

Risk:
- Report only real drift, missing validation, stale capability routing, invalid handoff memory, or scope creep.

Next Highest-Value Action:
- Name exactly one concrete fix or validation action.
```

## Escalation Rules

Escalate to a direct implementation turn when a review finds:

- license or release-doc drift
- real secret exposure
- generated files in Git state
- missing required workspace contract surfaces
- invalid handoff contract
- validation commands that are stale or no longer match the project
- local skill or AI operating handbook drift

Escalation should still preserve the user's repository rules and run the
narrowest meaningful validation before reporting completion.
