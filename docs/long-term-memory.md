# Long-Term Memory

## Facts

- On 2026-11-06 the source baseline is `main@450f40d`, combining maintenance
  repairs, batched region loading, Tianditu previews, artifact download grants,
  and removal of login credential hints. Local inspection and the root agent's
  `git ls-remote --heads origin` found only `main`; stale tracking refs do not
  prove another remote branch exists.
- The single app is `apps/admin-region-tiler`: Go, SQLite, and a static Leaflet
  frontend. The retired .NET runtime remains in [migration history](range-migration.md)
  only. The repository uses Apache License 2.0.
- The approved upgrade is in progress: fix evidenced workflow/performance
  problems, add terrain imagery and elevation terrain usable in Cesium, review
  beneficial stable dependency upgrades, and synchronize user/AI documentation.
  This first batch changes rules and portable documentation validation; it does
  not implement or validate the application upgrade.
- Prior evidence lives in [repair acceptance](repair-acceptance-2026-09-11.md)
  and [maintenance acceptance](maintenance-acceptance-2026-09-11.md). Five region
  gaps remain documented in [boundary investigation](region-gap-investigation-2026-09-11.md).
- Historical production notes recorded `V0.3.14` from `1f187a0` and an October
  password rotation. Production has not been reverified in this implementation;
  source convergence does not prove deployment.

## Decisions

- Keep Go, SQLite, and the static frontend. Prioritize usable correct downloads
  and measured performance; understand actual data/business constraints before
  removing checks. Follow the authoritative rules in `AGENTS.md`.
- Keep `main` as the only long-term branch. Integrate unique work, check the
  merged result, then delete all other local/remote branches under
  [the merge procedure](merge-plan.md).
- Preserve task/artifact compatibility, cumulative retries, and the last usable
  published output. Do not silently turn retry into a full download or invent
  administrative boundaries to hide missing geometry.
- Keep AI maintenance workspace-local, using the existing project map, knowledge
  graph, validation chain, local skill, and this handoff. Recurring reviews
  remain read-only by default.
- Commit validated batches with detailed bilingual notes. Keep private tokens
  and runtime outputs out of Git; reviewed fixture-based documentation images
  are allowed in `docs/assets/`.

## Assumptions

- The target is local/small-team use with the existing runtime architecture.
- Check current tool availability. Repository validation must not depend on a
  personal user profile or globally installed AI skill.
- Production deployment, credential changes, real-provider download trials, and
  publication require applicable authorization and current evidence.

## Validation

- Before implementation, the root agent reported passing `go test ./...`,
  `go vet ./...`, frontend syntax checks, and browser smoke. These are baseline
  results, not proof of the pending upgrade.
- The workspace control-surface audit passed before this rules batch.
- `node --check scripts/check_docs.mjs`, `node scripts/check_docs.mjs`, and
  `git diff --check` passed for this rules batch: 28 Markdown files and 59 local
  links/images, with no missing targets. Commit is pending. Linux race, native
  packages, full upgrade integration, and production have not been revalidated.

## Next Action

- Commit the validated rules/portable-documentation batch, then implement
  the approved application changes in bounded validated batches. Update this
  handoff as each batch finishes; do not claim the whole upgrade early.
- At integration, update README/manuals and reproducible images, run applicable
  full checks, and verify final `main` and branch cleanup state.
