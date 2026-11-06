# Project Rules

These rules apply to every Codex or AI-assisted change in this repository.

## Repository Scope

- Only work on the in-repository Go application:
  - `apps/admin-region-tiler`
- The old .NET range downloader has been retired after its bbox workflow was
  ported into the Go app. Keep historical notes in `docs/range-migration.md`
  instead of reintroducing runtime code.
- Do not import old local source folders, release packages, UI design packages,
  temporary directories, runtime screenshots, databases, downloaded tiles, or
  machine-local run data.
- Reviewed, sanitized documentation images in `docs/assets/` are allowed. Use
  local fixtures, record how to reproduce them, and refresh affected images
  when the user flow changes.
- Keep generated files out of Git, including `.env`, `data/`, `output/`,
  `tiles/`, `bin/`, `obj/`, `publish*/`, logs, binaries, and archives.

## Functionality, Performance, And Dependencies

- Prioritize usable functionality and measured performance. Reproduce a reported
  failure before changing its owner; retain only validation needed for correct
  data, actual resource constraints, or explicit business rules. Do not block a
  valid workflow with speculative checks or silently ignore user settings.
- Keep the Go backend, SQLite, and static frontend unless a requested capability
  demonstrates a need to change them. Prefer small changes over new frameworks
  or parallel abstractions.
- At the start of each development task, check the latest stable versions and
  official release notes for the project's dependencies and build/test tools;
  reuse a dated review from the same task. Record current/candidate versions,
  concrete project benefit, compatibility impact, and the upgrade decision.
  Upgrade when the new version helps this project and validate the affected
  flows; otherwise state why it is retained. Do not use prereleases by default.
- Lock validated dependency versions and keep development, CI, packaging, and
  container toolchains consistent. A floating `latest` install is not an upgrade
  record. Dependency review does not authorize production deployment.

## Branch Completion

- `main` is the only long-term branch. When work uses another branch, completion
  includes integrating its changes into `main` without regressions and deleting
  all other local and remote branches after their changes have been integrated.
- Inspect actual remote refs and worktrees, reconcile unique commits, validate
  the exact merge candidate, and follow existing branch protection. After the
  resulting `main` checks pass, synchronize the local checkout and remove the
  integrated branches. Do not force-delete unmerged work or bypass required checks.
- Verify the final local and remote branch lists; cached `origin/*` refs alone
  do not prove remote cleanup. If unfinished work or unavailable remote access
  prevents completion, report the remaining branch and next action explicitly.
- Use [the merge procedure](docs/merge-plan.md); preserve unrelated user work
  and existing authorization boundaries for remote operations.

## Commit Requirements

Every commit must have a detailed bilingual commit message in English and
Chinese. Use this structure:

```text
<type>(<scope>): <English summary> / <中文摘要>

English:
- What changed.
- Why it changed.
- User or developer impact.

中文:
- 修改了什么。
- 为什么修改。
- 对用户或开发者的影响。

Validation:
- Exact commands or checks run.
- Any checks intentionally not run, with reason.
```

Keep the subject concise, but do not omit the bilingual body. Prefer
Conventional Commit types such as `feat`, `fix`, `docs`, `chore`, `refactor`,
`test`, and `ci`.

After each validated change batch, commit immediately. Do not accumulate
multiple unrelated validated batches in one working tree. The commit body must
name the checks that were actually run and any checks that were intentionally
not run.

## Validation Before Completion

- Before a delivery commit or push, reconcile every accepted plan item with
  implementation, executed evidence and updated documentation. Close real gaps;
  passing the existing suite alone does not prove complete plan coverage. Keep
  post-push CI, protected merge and branch cleanup as explicit delivery checks,
  and verify them before claiming the entire plan complete.
- Run the narrowest meaningful check before claiming completion.
- Commit the validated batch before starting the next implementation batch.
- For `apps/admin-region-tiler`, prefer: `go test ./...`.
- For frontend script edits, run `node --check` on the changed JavaScript file.
- If a check cannot be run, state the exact environment limit.
- Run `node scripts/check_docs.mjs` from the repository root for documentation,
  local rules, or skill changes. Update affected README/manual/config/API
  explanations and documentation images in the same behavior change batch.

## Local AI Control Surfaces

- Start substantial work from `docs/project-map.md`, then `docs/done-definition.md`
  and the README for the affected app.
- For repository-local AI enhancement, deep-execution, or control-surface work,
  use `docs/ai-operating-handbook.md` as the compact execution entrypoint after
  the project map and done definition.
- Use `docs/validation-chain.md` to pick the narrowest meaningful check and
  `docs/automation-guardrails.md` for read-only recurring review prompts.
- Use `docs/knowledge-graph.md` to recover durable relationships before broad
  exploration.
- For merge, release, or long-running handoff work, update
  `docs/long-term-memory.md` when a durable fact, decision, assumption,
  validation result, or next action changes.
- Prefer the local skill in `.codex/skills/two-projects-handoff/` for
  repository-specific AI-assisted maintenance and post-merge cleanup.
