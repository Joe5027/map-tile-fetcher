# Commit Policy

All commits in this repository must use detailed bilingual messages in English
and Chinese. This keeps the project readable for contributors who work in either
language and preserves enough context for future merge work.

Every validated change batch must be committed immediately before the next
implementation batch starts. Do not leave validated changes uncommitted while
continuing into unrelated code, schema, UI, or documentation work.

## Required Format

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

## Example

```text
docs(repo): initialize public handoff structure / 初始化公开交接结构

English:
- Added repository-level documentation, license alignment, and merge planning.
- Kept both downloader apps separated under apps/ for the initial release.
- Documented ignored runtime and build artifacts to reduce release risk.

中文:
- 新增仓库级文档、许可证对齐和合并计划。
- 初始开源阶段将两个下载器应用保留在 apps/ 下独立维护。
- 记录运行和构建产物的忽略规则，降低发布风险。

Validation:
- Ran sensitive token scan.
- Ran .NET Release build for apps/range-downloader.
- Ran go test ./... for apps/admin-region-tiler.
```

## Local Template

The repository includes `.gitmessage`. To use it locally:

```powershell
git config commit.template .gitmessage
```

This local Git setting is optional, but the bilingual detailed format is
required for every commit.
