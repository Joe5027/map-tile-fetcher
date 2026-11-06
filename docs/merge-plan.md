# Merge Plan

The repository has moved from a two-app handoff into a single Go application.
The target product offers two download modes in one Web backend:

- bounding-box range download
- administrative-region download

## Completed Merge Work

- Go is the only backend runtime kept in the repository.
- SQLite is retained as the task control database.
- Bounding-box request validation and tile math exist in Go.
- `/api/tasks` accepts both region and bbox task creation.
- The Web UI has two creation modes: range and administrative region.
- File-tree output is packaged as ZIP; MBTiles output remains a direct artifact.
- Failure records are persisted in SQLite and exposed through
  `GET /api/tasks/:id/failures`.
- The legacy `.NET` range downloader runtime has been retired.

## Current Base

Keep `apps/admin-region-tiler` as the base for:

- API and optional auth
- map source configuration
- area selection: bbox and GeoJSON regions
- task and child-source creation
- task status, pause, resume, cancel, and purge
- scheduled jobs
- SQLite task/run/source/artifact/failure metadata
- output packaging and downloads
- Docker and service deployment assets

## Remaining Cleanup

- Continue reducing old `plans` naming in favor of `tasks` only after migration
  compatibility is no longer needed.
- Keep cumulative retry artifacts, explicit reconciliation and source budgets
  covered by the real API lifecycle suite and repair acceptance contracts.
- Keep browser UI smoke and release preflight running in CI before release.
- Keep real service tokens out of Git and keep runtime outputs in ignored
  filesystem paths.

## Branch Convergence

`main` is the only long-term branch. The pre-upgrade baseline was `450f40d`;
current implementation/validation status belongs in
[the handoff](long-term-memory.md).

1. Inspect status, local branches, and worktrees. Within granted remote access,
   inspect actual heads with `git ls-remote --heads origin` and synchronize refs
   before comparing commits.
2. Account for every non-main local/remote branch and integrate unique changes
   into `main`. Preserve unrelated work, resolve conflicts, and validate the
   exact candidate. Report unfinished work instead of discarding it.
3. Follow current remote protection and required checks. Use a checked PR head
   when required and a bilingual merge message; do not bypass protection or
   assume historical branch settings still apply.
   Before calling a remote merge API, write its exact proposed title and body
   to a temporary file and run `node scripts/validate_commit_message.mjs --file
   <file>`. Both language sections need at least three bullet lines. Remote
   merge APIs do not invoke the local commit hook; branch checks alone cannot
   validate a message that is supplied only at merge time.
4. Verify the resulting `main` checks, synchronize local `main`, and detach
   integrated branches from their worktrees before deleting them locally and
   remotely. Preserve user changes and history; never force-delete unmerged work.
5. Verify local branches and actual remote heads contain only `main`, then
   prune stale tracking refs. Record final commit, checks, cleanup, and any
   remaining blocker in the handoff.

Code delivery does not itself authorize production deployment. Verify current
production state separately when deployment is requested.
