# Validation Chain

Use this chain before claiming completion. Pick the narrowest path that matches
the touched surface, but keep the chain order intact.

## 1. Scope Gate

Confirm the change affects only:

- `apps/admin-region-tiler`
- repository-level docs, rules, local skills, or validation surfaces that support
  the Go app

Do not import old source folders, release packages, temporary screenshots,
runtime data, generated tile outputs, or retired .NET runtime code. Reviewed
documentation images in `docs/assets/` follow `AGENTS.md`.

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
npm ci --no-audit --no-fund
npx playwright install chromium
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
It also runs `python -B -m unittest discover -s scripts -p test_*.py -v`.
Use the Python version pinned in root `.python-version` (currently 3.13.17).
Set `REGION_PYTHON` to that isolated interpreter installed with
`pip install --require-hashes --only-binary=:all: -r scripts/requirements-region.lock`.
See [region tooling](region-maintenance.md) and [build/install validation](build-and-install.md).
Windows/Linux package CI builds twice, compares checksums, extracts in a fresh
directory and verifies login, resources, version and a real loopback download.
The application CI job allows 30 minutes for browser installation, all nine
real workflows and race/vet. The former 20-minute budget cancelled a healthy
run after an observed eight-minute browser installation; no cases are skipped.

The Go suite includes `TestAPILifecycle`, which builds the real app binary and
exercises HTTP, the scheduler, SQLite and ordinary worker processes against a
local tile fixture. It does not contact real tile providers. UI smoke still
intercepts selected creation payloads for focused regression checks. The separate
`node scripts/workflow_acceptance.mjs` check uses real browser clicks and actual
API/worker requests for administrative, rectangular and polygonal tasks at
390/768/1440 px, including pause, resume, retry, preview and downloaded ZIP
content. Layout-only checks do not substitute for those workflows.
Preflight also runs rendering-security contracts and responsive/preview/login
regressions. Linux CI additionally runs `go test -race ./...` and `go vet ./...`.
Use [repair acceptance](repair-acceptance-2026-09-11.md) for issue mapping,
baseline red/green commands, legacy migration and artifact coordinate evidence.

New recovery/elevation changes require the relevant real-worker fixtures,
not just the pure codec tests: parent termination, paused capacity, cumulative
retry, missing root/child elevation, and authenticated compressed terrain
delivery. `task_page_test.go` checks 1,000 parents with three children, fixed
batch query count and equivalent legacy responses. Browser terrain acceptance
must load the actual mesh with local Cesium and block external requests.

Performance evidence is reproducible without contacting providers:

```powershell
python -B scripts/benchmark_download.py --ref 450f40d --ref HEAD --output tmp/performance.json
python -B scripts/benchmark_workflow.py --ref 450f40d --working-tree --output tmp/workflow-performance.json
go test -run '^$' -bench BenchmarkMBTilesWrites -benchtime=3x -count=1 .
```

The first fixture measures the download engine, excluding ZIP packing; heap and
output peaks are sampled every 10 ms, not OS RSS. Compare exact revisions and
record toolchain and machine details. Timing is evidence, not a flaky pass gate.
The workflow fixture measures real API acceptance through worker publication,
including ZIP packing, sampled OS process-tree RSS and all isolated runtime
temporary paths. It verifies the downloaded artifact bytes after timing stops.
`--working-tree` permits the plan's pre-commit comparison without inventing a
completed commit; the report records the base commit and compiled source hash.
Follow [the complete plan coverage checklist](plan-acceptance-2026-11.md) before
delivery; passing the old suite does not cover new acceptance requirements.

For repository documentation, handoff state, rules, or local skills, run from
the repository root:

```powershell
node --check scripts/check_docs.mjs
node scripts/check_docs.mjs
```

This checks repository-local Markdown links/images, required entrypoints, and
canonical rule references plus the five ordered handoff sections. External URLs and historical absolute local
links are not resolved; the latter are reported as skipped. Source-to-document
comparison and visual inspection of changed documentation images remain required.
Run an environment skill's audit when that skill requires it and is available;
do not make another user's global installation a repository validation gate.
Use successful current-session calls when reporting runtime capabilities.

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

## 7. Main And Branch Cleanup

Apply [the branch convergence procedure](merge-plan.md#branch-convergence).
Validate the merged result on `main`, reconcile every non-main branch's unique
work, then remove integrated local and remote branches and verify actual refs.
Report any unresolved branch or check; do not call the delivery complete while
the required convergence remains unfinished.
