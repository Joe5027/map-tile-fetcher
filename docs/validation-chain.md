# Validation Chain

Use this chain before claiming completion. Pick the narrowest path that matches
the touched surface, but keep the chain order intact.

## 1. Scope Gate

Confirm the change affects only:

- `apps/range-downloader`
- `apps/admin-region-tiler`
- repository-level docs, rules, local skills, or validation surfaces that support
  those two apps

Do not import old source folders, release packages, screenshots, runtime data,
or generated tile outputs.

## 2. Source Truth Gate

For source or behavior changes, read the owning files before editing:

- range downloader: `apps/range-downloader/README.md`, `Program.cs`, affected
  `wwwroot/*`
- admin tiler: `apps/admin-region-tiler/README.md`, `main.go`, `server.go`,
  `runtime.go`, `task.go`, `db.go`, affected `static/*`
- merge or release docs: `PROJECT_MANIFEST.md`, `docs/merge-plan.md`,
  `README.md`, `LICENSE`

For AI-control changes, read:

- `AGENTS.md`
- `docs/project-map.md`
- `docs/done-definition.md`
- `.codex/skills/README.md`

## 3. Syntax And Build Gate

Run the relevant command from the app directory:

```powershell
cd apps/range-downloader
dotnet build .\TianDiTuDownLoader.Web.csproj -c Release
```

```powershell
cd apps/admin-region-tiler
go test ./...
```

```powershell
cd apps/range-downloader
node --check .\wwwroot\app.js
```

```powershell
cd apps/admin-region-tiler
node --check .\static\script.js
```

For handoff artifacts:

```powershell
python C:\Users\32674\.codex\skills\deep-execution-upgrade\scripts\validate_handoff_contract.py --path docs\long-term-memory.md --format text
```

For workspace control-surface changes:

```powershell
python C:\Users\32674\.codex\skills\deep-execution-upgrade\scripts\audit_environment.py --mode workspace --workspace . --format text
```

## 4. Secret And Generated-File Gate

Run the sensitive-value scan from `docs/done-definition.md`.

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
