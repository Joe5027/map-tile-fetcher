# Long-Term Memory

## Facts

- The only runtime is `apps/admin-region-tiler`: Go 1.27.3, SQLite and a static Leaflet/Cesium frontend, Apache License 2.0. Retired .NET code stays in [migration history](range-migration.md).
- The November upgrade implements durable restart recovery, pause capacity release, batched MBTiles storage, exact coordinate budgets, paginated history and Terrarium-to-Cesium offline terrain. [PR #8](https://github.com/Joe5027/map-tile-fetcher/pull/8) and [upgrade evidence](upgrade-2026-11.md) track the implementation and checks.
- Main requires a PR and the `Commit Message` and `Admin Region Tiler` checks, including administrators. Main is the only long-term branch; read actual Git refs and PR/workflow state for transient delivery status rather than relying on an old branch inventory in this document.
- No production deployment, user database replacement or broad real-provider download is included. Existing region gaps remain in [boundary investigation](region-gap-investigation-2026-09-11.md).

## Decisions

- Keep Go/SQLite/static frontend. Correct functioning and measured performance take precedence; only actual correctness, resource and business constraints justify rejection.
- Terrain uses public Terrarium PNG -> globally sampled 65×65 quantized-mesh, EPSG:3857/TMS, one root and complete ancestors. Raw cache and derived mesh integrity remain separate. Missing heights stay missing; this is visualization data without engineering datum conversion.
- Abnormal restart resumes original work; manual pause remains paused and releases capacity. One server owns each database. Damaged pending publication is rebuilt without blocking the service or replacing the previous published artifact.
- Latest stable dependency reviews and explicit retention reasons live in [the dependency record](dependencies-2026-11.md). Lockfiles feed development, CI and packages.
- Integrate and validate exact code through protected main, then delete integrated non-main refs and verify actual remote heads. Never discard unique work.
- Local skills reference canonical `AGENTS.md`; reviewed fixture documentation images are permitted. Recurring automation templates remain read-only; no scheduled automation was created.

## Assumptions

- The target is local/small-team operation and offline 3D visualization.
- Fault, browser and package checks use isolated directories, databases and loopback tile sources. One public root tile was used only to verify Terrarium decoding.
- Geographic accuracy, production deployment, power-loss durability and engines other than the locked Cesium version require their own validation.

## Validation

- Go unit/integration tests and vet passed. Actual service/worker interruption cases cover ZIP/MBTiles download, pause, terrain conversion and packing, original-run reuse, committed-tile reuse and retained publication. Duplicate-server and damaged-publication regressions were reproduced before repair and passed afterward.
- Pagination covers 1,000 parents × three children, eight batch queries and compatibility with the original array API. UI smoke/repair covers actual parameters, zero creation confirmation dialogs, 390/768/1440 layouts and preservation of focus/selection/expansion.
- Actual offline Cesium checks load real gzip meshes, independently sample heights, inspect finite root geometry and render four camera views with zero external requests. Four reviewed images and source hashes are reproducible through [the image capture procedure](assets/README.md).
- Hash-locked Shapely passed 18 Python tests; Go modules verified. The dependency baseline passed all six PR CI jobs, including Linux race and Windows/Linux repeated native package builds. Final-revision CI evidence is linked from [PR #8 checks](https://github.com/Joe5027/map-tile-fetcher/pull/8/checks) and [Validate runs](https://github.com/Joe5027/map-tile-fetcher/actions/workflows/validate.yml).
- Portable documentation links/structure/rule checks and the local workspace AI audit passed. Exact commands, fixture metrics, failed-then-fixed regressions and limits are in the upgrade record and bilingual commits.

## Next Action

- For the next approved change, start from [project map](project-map.md), [done definition](done-definition.md) and [validation chain](validation-chain.md); reuse the relevant fixture instead of repeating broad exploration.
- Read Git status, worktrees, actual remote heads and the exact commit's workflow result before merge/release work. Apply [the merge procedure](merge-plan.md), retain validated effective work and finish branch cleanup.
- Production deployment, engineering elevation use, authoritative region-boundary completion and a formal versioned Release are separate work; do not infer authorization from this upgrade record.
