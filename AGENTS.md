# Project Rules

These rules apply to every Codex or AI-assisted change in this repository.

## Repository Scope

- Only work on the two in-repository applications:
  - `apps/range-downloader`
  - `apps/admin-region-tiler`
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

## Validation Before Completion

- Run the narrowest meaningful check before claiming completion.
- For `apps/range-downloader`, prefer:
  `dotnet build .\TianDiTuDownLoader.Web.csproj -c Release`.
- For `apps/admin-region-tiler`, prefer: `go test ./...`.
- For frontend script edits, run `node --check` on the changed JavaScript file.
- If a check cannot be run, state the exact environment limit.

## Local AI Control Surfaces

- Start substantial work from `docs/project-map.md`, then `docs/done-definition.md`
  and the README for the affected app.
- Use `docs/validation-chain.md` to pick the narrowest meaningful check and
  `docs/automation-guardrails.md` for read-only recurring review prompts.
- For merge, release, or long-running handoff work, update
  `docs/long-term-memory.md` when a durable fact, decision, assumption,
  validation result, or next action changes.
- Prefer the local skill in `.codex/skills/two-projects-handoff/` for
  repository-specific AI-assisted maintenance and merge planning.
