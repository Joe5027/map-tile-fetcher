# Project Rules

These rules apply to every Codex or AI-assisted change in this repository.

## Repository Scope

- Only work on the in-repository Go application:
  - `apps/admin-region-tiler`
- The old .NET range downloader has been retired after its bbox workflow was
  ported into the Go app. Keep historical notes in `docs/range-migration.md`
  instead of reintroducing runtime code.
- Do not import old local source folders, release packages, UI design packages,
  temporary directories, screenshots, runtime databases, downloaded tiles, or
  machine-local run data.
- Keep generated files out of Git, including `.env`, `data/`, `output/`,
  `tiles/`, `bin/`, `obj/`, `publish*/`, logs, binaries, and archives.

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

- Run the narrowest meaningful check before claiming completion.
- Commit the validated batch before starting the next implementation batch.
- For `apps/admin-region-tiler`, prefer: `go test ./...`.
- For frontend script edits, run `node --check` on the changed JavaScript file.
- If a check cannot be run, state the exact environment limit.

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
