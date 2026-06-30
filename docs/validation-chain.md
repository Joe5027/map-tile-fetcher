# Validation Chain

Use this chain before claiming completion. Pick the narrowest path that matches
the touched surface, but keep the chain order intact.

## 1. Scope Gate

Confirm the change affects only:

- `apps/admin-region-tiler`
- repository-level docs, rules, local skills, or validation surfaces that support
  the Go app

Do not import old source folders, release packages, screenshots, runtime data,
generated tile outputs, or retired .NET runtime code.

## 2. Source Truth Gate

For source or behavior changes, read the owning files before editing:

- admin tiler: `apps/admin-region-tiler/README.md`, `main.go`, `server.go`,
  `runtime.go`, `task.go`, `db.go`, affected `static/*`
- merge or release docs: `PROJECT_MANIFEST.md`, `docs/merge-plan.md`,
  `README.md`, `LICENSE`

For AI-control changes, read:

- `AGENTS.md`
- `docs/project-map.md`
- `docs/done-definition.md`
- `docs/ai-operating-handbook.md`
- `docs/knowledge-graph.md` when relationship recovery or continuity matters
- `.codex/skills/README.md`

## 3. Syntax And Build Gate

Run the relevant command from the app directory:

```powershell
cd apps/admin-region-tiler
go test ./...
```

```powershell
cd apps/admin-region-tiler
node --check .\static\script.js
```

For browser UI smoke automation:

```powershell
cd apps/admin-region-tiler
node --check .\scripts\smoke_ui.mjs
node .\scripts\smoke_ui.mjs
```

For release preflight or CI workflow changes:

```powershell
cd apps/admin-region-tiler
node --check .\scripts\release_preflight.mjs
node .\scripts\release_preflight.mjs
```

The release preflight runs Go tests, frontend syntax checks, UI smoke,
tracked-file sensitive-value scanning, and tracked generated-file scanning.

For handoff artifacts:

```powershell
python C:\Users\32674\.codex\skills\deep-execution-upgrade\scripts\validate_handoff_contract.py --path docs\long-term-memory.md --format text
```

For workspace control-surface changes:

```powershell
python C:\Users\32674\.codex\skills\deep-execution-upgrade\scripts\audit_environment.py --mode workspace --workspace . --format text
```

For runtime capability or MCP/plugin routing claims:

```powershell
python C:\Users\32674\.codex\skills\deep-execution-upgrade\scripts\runtime_preflight.py --format text
```

## 4. Secret And Generated-File Gate

Run the sensitive-value scan from `docs/done-definition.md`, or run the full
release preflight when the changed surface already requires it.

Then check Git state:

```powershell
git status --short
```

Expected generated files should not appear. If build outputs appear, do not
commit them.

## 5. Report Gate

Close with:

- changed surfaces
- checks run
- checks missing
- assumptions and environment limits
- next highest-value action

For environment-upgrade work, also include:

- agents or skills used
- surfaces added or updated
- residual risk

## 6. Commit Gate

After validation passes, commit the batch before starting the next batch:

```powershell
git status --short
git add <validated files>
git commit
```

The commit message must follow `.gitmessage` and include English, Chinese, and
Validation sections.

This repository also ships a versioned local hook and CI check for that rule:

```powershell
git config core.hooksPath .githooks
node .\scripts\validate_commit_message.mjs --commit HEAD
```

The local `commit-msg` hook rejects non-bilingual or incomplete commit messages
before the commit is created. GitHub Actions validates pushed and pull-request
commits with the same script; make the `Commit Message` check required in
branch protection if the remote must block merges instead of only reporting a
failed check.
