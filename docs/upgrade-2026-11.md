# November 2026 Upgrade Record

This record describes the implementation approved on 2026-11-06. It is not a
production deployment record. Tests use temporary databases and local tile
fixtures; no user database or existing downloads are used.

## Delivery Status

| Batch | State |
| --- | --- |
| Portable rules, dependency review, main completion and handoff | Committed: `da33be6` |
| Download configuration and format correctness | Committed: `32ab1ca`, `79d0184` |
| Automatic recovery, queue slots and storage performance | `53b7af2`; full Go suite, precise staged recovery regressions and vet passed |
| Dependency/toolchain upgrade | `9ca929a`; isolated tests and all six PR CI jobs passed, including Windows/Linux repeat packages |
| Cesium terrain generation and offline preview | Codec, pipeline, process recovery and actual offline Cesium checks passed |
| Source selection, paginated history and responsive interactions | Shared sources, parameters, pagination and responsive browser regressions passed |
| Manuals, reproducible images, native packages and final main checks | Local preflight and reviewed images passed; exact platform/main state is linked below |

## Download Configuration Regressions

The new `download_policy_regression_test.go` reproduced all four backend defects
on the previous implementation: three TCP connections instead of one; configured
404/503 retry decisions reversed; explicit zero retries changed to two; real
WebP rejected as an unknown image format. The same four tests pass after repair.

Default HTTP requests now reuse connections. A source may explicitly set a
`Connection: close` header when it actually needs one. Source `retry_statuses`
controls HTTP retry decisions in both attempts and extra passes; transport
failures retain their normal retry behavior. `max_retries = 0` disables extra
attempts, while `retry_passes` remains a separate setting. WebP is validated and
preserved without image recompression.

The source catalogue exposes only concurrency and minimum-delay values under
`fetch_policy`; source headers and proxy credentials are not included.

Both creation modes now use the visible download, saver and interval settings.
The browser regression failed against the old bbox implementation, then passed
with non-default values and zero delay. The inline task summary replaces the
creation confirmation dialog; duplicate submit events during an active request
produce one POST. Responsive, password, preview and session regressions passed.

Validation is recorded in each bilingual commit and in the acceptance evidence
below; do not infer platform results from local checks alone.

## Terrain Core

`internal/terrain` decodes Terrarium pixels, samples a globally aligned 65 × 65
grid, and writes gzip quantized-mesh with TMS availability and complete ancestor
chains. Tests independently decode the binary and check signed/zero heights,
Mercator latitude, triangle winding, edges, dateline sharing and missing input.
No missing height is replaced with zero. `go test ./internal/terrain` and
`go vet ./internal/terrain` passed with Go 1.27.3.

The whole-world Mercator root cannot have a finite conservative horizon point.
The encoder disables that optional culling through NaN for such coarse tiles.
Actual Cesium 1.147 multi-view validation passed, as recorded below. Windows
race verification is unavailable with the local CGO-disabled toolchain;
Linux race verification is part of CI acceptance.

## Task History Fixture

`TestTaskPageMatchesLegacyAndBoundsWork` creates 1,000 parent tasks with three
children each. On the local Windows fixture, the legacy response took 1.918 s
and 4,689,609 bytes; the 50-parent page took 57.8 ms and 234,695 bytes, using
eight batch queries. This measures API construction on that fixture, not a
universal latency guarantee. Global status aggregation still scans compact
status data; full run/artifact/failure records are limited to the selected page.

`GET /api/tasks/page?limit=50&offset=0&status=all` returns `items`, `total`, and
`counts`. The original array endpoint remains compatible. Browser checks cover
pagination, last-page deletion, keyed updates, focus/menu/selection/expansion
preservation, phase display and stopping polling when the task page is hidden.
Download-grant expiry is checked on use; bulk expiry cleanup runs at most once
per minute.

## Dependency Baseline

The committed source at `d90fcbc` was extracted into a fresh temporary directory,
then tested with the new `go.mod`/`go.sum`: `go test ./...` and `go vet ./...`
passed. This separates dependency compatibility from concurrent recovery and
terrain implementation. The Shapely 2.2.0 hash-locked environment passed all 18
Python tests. Project-local `npm ci` prepared Cesium 1.147.0 and Playwright
1.65.0; the existing browser suite passed with those locked versions.

Go toolchain downloads and a checksum-verified Node 24.22.2 executable use local
caches only; the user's global runtimes were not replaced. See
[dependency decisions](dependencies-2026-11.md) for official release links,
upgrade benefits and deliberately retained versions.

## Recovery And Storage

Persisted execution ownership, phases and coordinate manifests recover the same
run after service interruption. Manual pause remains paused across restart and
releases its scheduler slot. OS file locking and ownership checks prevent a stale
worker from publishing over a replacement. Recovery reuses valid raw tiles;
the previous published artifact remains available until replacement succeeds.

Real process tests terminate service/worker execution during ZIP and MBTiles
downloads, paused execution and ZIP packing. An 832-file packing interruption
recovered in 14.56 seconds without another source download; the two workers
persisted eight packing progress updates and no `.partial` file entered the ZIP.
The reported duration is one local fixture observation, not a service guarantee.
I/O fault tests reproduced a paused condition-variable hang and incorrect
`cancelled` status; both now pass, retaining the original storage error.

BBox enumeration uses arithmetic counts and streamed coordinates. Polygon
coordinates are deduplicated into one reusable manifest; exact counts enforce
the existing configured budget. Retry baseline validation runs in the worker,
outside the scheduler lock. MBTiles uses one writer, a prepared statement and
bounded transactions (256 records, 8 MiB of tile bytes or 250 ms), counting
success after commit. A single larger valid tile is accepted in its own batch.
Consumed batch references are released after commit; baseline copying shares
the byte boundary instead of retaining up to 256 large tile payloads.
Ordinary output no longer runs `VACUUM`. Disk checks estimate actual copy,
conversion and ZIP work; no implicit extra 1 GiB reserve blocks small jobs.

Atomic file writes use unique temporary names and serialize only the final
replacement of the same coordinate. A 32-writer regression reproduced Windows
sharing/rename failures with the old temporary-file approach and passes after
the fix. Uncommitted partial bytes cannot be mistaken for a valid saved tile.

The isolated 256-record storage microbenchmark measured about 986 ms with
autocommit and 9 ms in one bounded transaction. This measures SQLite storage,
not whole-task acceleration. `scripts/benchmark_download.py` compares committed
revisions using the same loopback fixture and records first committed tile,
elapsed time, connections, sampled heap and output size. ZIP packaging and OS
RSS are outside that fixture; whole-task measurements are reported separately.

A second server using the same database is rejected before migration or
scheduler recovery; worker processes retain the shared maintenance lock. The
duplicate-server test reproduced the old concurrent startup and now passes.
Missing/corrupt pending ZIPs are returned to the original run for rebuilding,
preserving its raw data, the previous published artifact and unrelated valid
publications. Both failure cases were reproduced before the repair.

The packing-interruption fixture waits up to 90 seconds to reach an actual
unfinished ZIP. A full concurrent run showed the earlier 25-second arrival
window could expire on a slow disk; this is an injection deadline, not a
performance requirement. The revised precise regression passes.

## Terrain Integration And Browser Evidence

The application persists source kind, encoding, projection, valid levels and
attribution with child tasks. Public Terrarium defaults to level 14 and allows
15. Terrain raster/annotation presets remain ZIP/MBTiles; vector terrain is
labelled separately from elevation. One child terrain job cannot mix source
encodings/URLs; invalid combinations fail before task or queue records exist.

Raw PNG, derived mesh and published artifacts have separate integrity records.
A real conversion interruption with 341 level-0–4 meshes resumed the same run,
reused 32 previously completed meshes by modification time and made no repeat
raw requests (341 total). Missing root data blocks only that publication; retry
can reuse the raw baseline even when there was no first published artifact.

One public root PNG was read as a bounded source-format smoke check: HTTP 200,
106,274 bytes, decoded heights -7,527 to 5,657 m; the 65×65 sampled mesh ranged
from -5,989.75 to 4,897 m. This verifies the source encoding, not geographic or
engineering accuracy, and is not a large-area download.

`node scripts/capture_docs.mjs` passed the complete browser/repair suite and
generated the reviewed [documentation images](assets/README.md). Actual Cesium
1.147 loaded 85 generated meshes through HTTP with real gzip headers. The root
mesh had 31,395 finite vertex components; independent samples returned about
316.247, 600 and 884.087 m. Global and three close camera views rendered meshes,
29 local terrain requests completed, and external requests/browser errors were
zero. Camera checks wait for new render frames after movement, not stale
`tilesLoaded` state. This specifically validates Cesium 1.147 compatibility.

The preview uses local Cesium assets, a metre-labelled elevation color ramp and
an optional plain-color mode. It does not request ion or online imagery. Tests
cover 390/768/1440 px, real parameter submission, zero creation confirmation
dialogs, page visibility, focus/selection/menu preservation, pagination,
password/session behavior and task actions. Synthetic screenshot terrain is
explicitly labelled and source file hashes accompany the captured images.

## Download Engine Fixture Comparison

Command: `python apps/admin-region-tiler/scripts/benchmark_download.py --ref
450f40d --ref 53b7af2 --output tmp/performance-final.json`. Each revision is
extracted separately. The fixture is 256 one-pixel PNGs, four download workers,
zero delay and loopback HTTP. The old revision used Go 1.25.3 and the new one
Go 1.27.3, so this compares the delivered implementations/toolchains together.

| Metric | Old ZIP file tree | New ZIP file tree | Old MBTiles | New MBTiles |
| --- | ---: | ---: | ---: | ---: |
| Total download engine time, ms | 180.463 | 429.230 | 507.320 | 71.733 |
| First committed tile, ms | 20.214 | 30.900 | 50.465 | 70.608 |
| Requests / TCP connections | 256 / 256 | 256 / 4 | 256 / 256 | 256 / 4 |
| Sampled peak Go heap, bytes | 2,900,832 | 2,797,064 | 4,470,992 | 1,984,856 |
| Sampled peak output size, bytes | 17,408 | 17,408 | 57,880 | 45,056 |

The MBTiles engine took about 86% less time on this fixture, with a later first
durable commit because writes are batched. File-tree writes were slower: unique
temporary files and atomic replacement add filesystem operations to preserve
restart correctness. This cost is retained deliberately; there is no claim
that every ZIP download became faster. Tiny files emphasize filesystem cost
and loopback requests remove real network latency. Timings are one paired run,
not a statistical throughput guarantee; earlier exploratory runs had the same
direction but different magnitudes.

Sampling occurs every 10 ms and can miss short peaks. It measures Go heap, not
OS RSS, and output file sizes, not total filesystem allocation. ZIP compression
is excluded from this engine fixture; the separate 832-file packing interruption
case validates packaging and progress-write behavior. The 256-record storage
microbenchmark separately compares 256 transactions with one bounded batch.

## Final Plan Coverage Audit

The [plan coverage matrix](https://github.com/Joe5027/map-tile-fetcher/blob/main/docs/plan-acceptance-2026-11.md)
maps every accepted work item to implementation and executable evidence. The
final review found and closed gaps that earlier passing tests did not cover:

- Terrain creation estimates include ancestors and the sampling halo. The
  fallback catalogue includes terrain sources when the catalogue request fails.
  Ninety formula/brute-force comparisons and the actual browser check confirm
  14 raw requests / 3 meshes for a 1–2 degree bbox at level 2.
- Preparation reports its unique count and estimate; a 512-coordinate retry
  baseline is read, validated and copied in exactly one pass. A locked
  65,535-tile baseline does not prevent retry enqueue or another task operation.
- Real skinny, island and overlapping GeoJSON fixtures exercise actual unique
  budgets. A legacy database copy preserves failure history and downloads the
  same published ZIP bytes after migration; the original database is unchanged.
- SQLite copy estimates include live table/index pages, exclude free pages,
  and distinguish new destinations from resume journals. The real copy tests
  compare measured temporary usage against those estimates. Disk-boundary tests
  use actual free space without filling the user's drive.
  Resume copying also credits existing raw/ZIP target files and reserves only
  net growth plus one atomic replacement overlap. The failed-before regression
  previously charged 296 bytes for four tiny tiles despite three already being
  present; it now needs 74 bytes growth plus 74 bytes replacement space.
- Nine real browser chains cover all three area modes at 390/768/1440 pixels:
  create, pause, resume, failed-tile retry, extent/terrain preview and ZIP
  download. They use actual API, database and worker execution; retry issues
  exactly one missing source request. Independent ZIP parsing verifies CRC,
  five meshes, 4,225 vertices per mesh and known 100 m elevations. Background
  refresh preserves selection and the actual scroll container at each width.
  Before retry, only the root is available (1/5 meshes); each case independently
  samples 100 m at the missing child and renders an actual level-3 upsampled
  mesh whose 62 non-skirt vertices are all 100 m. Cancelling permanent deletion
  sends zero purge requests; accepting sends exactly one real purge, removes
  the card and makes the task return HTTP 404.
- Exported unnamed terrain sources no longer include URL userinfo, query or
  fragment in metadata. A legal source reproduced the leak before repair;
  the original download URL still works. Ordinary vector ZIP/MBTiles payloads
  are checked byte for byte.
- Python is pinned to 3.13.17 across local verification and CI. It is the common
  supported Windows/Linux patch selected for the existing 3.13 wheel lock;
  the dependency record explains why 3.13.18 and 3.15.2 were not selected.

All four documentation images were refreshed again after the coverage fixes.
The capture now waits for the actual CSS color transition and checks tab state
and final colors: the earlier image could show the old task-tab color during
the 180 ms transition despite the correct bbox state. It does not alter the
product DOM or CSS to manufacture an image.

The full-workflow memory measurement also found that count-only MBTiles
batches retained too much data for large PNG tiles. The 8 MiB boundary fixes
that measured regression without rejecting a valid large tile. Actual saver
and retry-copy tests force later batches to fail and verify that the earlier
committed batch remains usable; timer-driven commits cannot mask those tests.
The large-BLOB resume fixture measured 8,449,544 additional bytes against an
8,757,928-byte estimate. A preflush failure also records the just-consumed tile,
so it cannot disappear from the retryable failure set.

## Whole-Workflow Fixture Comparison

Command: `python apps/admin-region-tiler/scripts/benchmark_workflow.py --ref
450f40d --working-tree --output tmp/workflow-performance-final.json`, using
Python 3.13.17. This is a real API -> scheduler -> worker -> publication
comparison, including ZIP creation. The loopback source serves 256 identical,
deterministic 256×256 RGB PNGs with four download workers and zero delay.
Each downloaded artifact is independently checked for 256 byte-identical tiles.

The old implementation uses Go 1.25.3; the candidate uses Go 1.27.3. The
candidate was measured before commit, based on `f7724ce`, with non-test Go,
module and config source hash
`815863e364550a33bc0f4cf0110f52cb11d7825bf35685722fbf0503d3aa6704`.
The script records the same source fingerprint when comparing committed refs.

| Metric | Old ZIP | Candidate ZIP | Old MBTiles | Candidate MBTiles |
| --- | ---: | ---: | ---: | ---: |
| API creation to publication, ms | 4,256.146 | 3,335.214 | 2,075.098 | 756.719 |
| First externally observed committed tile, ms | 253.933 | 183.956 | 1,154.242 | 550.498 |
| Requests / TCP connections | 256 / 256 | 256 / 4 | 256 / 256 | 256 / 4 |
| Sampled simultaneous process-tree RSS, bytes | 59,637,760 | 62,595,072 | 71,651,328 | 81,350,656 |
| Sampled peak runtime-directory size, bytes | 98,642,541 | 89,825,252 | 150,581,367 | 51,388,373 |
| Published artifact size, bytes | 50,476,438 | 50,464,150 | 50,536,448 | 50,528,256 |

The candidate completes sooner and uses fewer connections on this workload,
but both formats use more process memory than the old implementation. An
earlier candidate without the byte bound reached 162,963,456 bytes of MBTiles
process-tree RSS; the 8 MiB boundary cuts that observed peak roughly in half
while preserving its throughput. There is no claim that every memory or
latency metric improves, and the tiny-file engine fixture above still shows
the filesystem cost of durable atomic file replacement.

RSS and logical file sizes are sampled every 20 ms and can miss short peaks.
The process tree includes the owned service and workers; recorded per-process
OS peak counters include startup and are not additive simultaneous peaks.
All worker temp variables, including SQLite/VACUUM, point inside the isolated
runtime directory. Its size includes the control database, raw data, coordinate
catalogues, journals and packaging; it excludes build source/binary and does
not measure physical filesystem allocation. First-tile observation uses a
complete saved file or the persisted MBTiles success counter, so includes
polling/progress latency. Verification downloads occur after the timing ends.
This is one quiet paired run without a timing threshold, not a statistical
throughput or memory guarantee for other machines or provider payloads.

## Preview And Validation Cost

The user asked why acceptance was taking so long. Linux logs showed nine real
workflow cases taking about 65–75 seconds each, run serially, while branch-push
and PR triggers duplicated the same pipeline. Stage profiling showed no fixed
30/60-second sleep: a local create/pause/resume/partial-publication path took
about 1.7 seconds and retry about 0.36 seconds. Continuous Cesium software
rendering delayed ordinary browser interactions. Repeated script downloads
were not the main bottleneck (about 130–170 ms per Cesium script response).

A matched Windows single-case comparison used Node 24.22.2, Playwright 1.65.0,
the same 390px popup and actual browser actions. Continuous rendering took
32.570 seconds including startup; on-demand rendering took 21.341 seconds.
The main workflow excluding startup/catalogue setup fell from 26.051 to
14.656 seconds, and the four button actions plus frame checks from 12.735 to
1.678 seconds. This is a single local comparison, not a timing guarantee.
The 500 ms idle observation saw four rendered frames before and one final
frame after; camera and both color changes still produced real rendered
frames. Height, fallback, one-request repair, ZIP and deletion checks passed.

The production preview now renders on demand without reducing resolution or
mesh detail. Camera changes and tile loads trigger frames, and color changes
explicitly request one. A separate accuracy issue was fixed in the browser
fixture: a popup inherited the context's 1440px viewport even when its parent
page was 390px. Each popup now receives and asserts the actual scenario width.
The older standalone capture already tested the toolbar at all three widths;
parent-page width alone is not evidence of popup width.

The workflow keeps all nine real cases. Branch pushes no longer duplicate PR
checks; `main` pushes and manual dispatch retain the full workflow. Lockfile
npm caching and headless-shell-only installation avoid unnecessary downloads
without changing the tested browser. Verified unchanged application evidence
is reused for documentation/CI-only edits; required remote gates remain.
Release preflight prints each stage's elapsed time so a future slowdown can
be located without rerunning the full suite just to add timing instrumentation.

The final local nine-workflow run passed in 141.941 seconds including startup,
catalogue setup, all cases, deletion and cleanup, using Node 24.22.2. Each
390/768/1440px popup was independently sized and checked. All nine idle
observations recorded zero frames; each world/fit/color action produced a
real frame. Color pixels changed from `[93,170,145,255]` to
`[158,180,172,255]` and back, without the test forcing a color redraw. Parent
fallback at 100 m, exactly one repair request and five valid mesh files per
ZIP still passed. This local duration is not a like-for-like comparison with
the older hosted Linux workflow; the paired comparison above measures the gain.
The final documentation capture passed in 60.176 seconds, including normal
UI/repair checks, four real terrain views, height sampling and zero external
requests. Its idle-frame count was zero and color pixels changed/restored.
All five manifest source hashes match the rendered frontend; the refreshed
images were reviewed. The unchanged backend/Python suites were reused from
their passing candidate checks instead of rerun for these frontend/CI edits.

## Acceptance Evidence And Limits

The initial integrated local release preflight passed with the pinned Node 24.22.2:
18 Python fixtures, `go test ./...` (main package 107.054 s), all JavaScript
syntax/rendering checks, complete browser smoke/repair, sensitive-value scan
and generated-file scan. Separate `go vet ./...`, `go mod verify`, three release
builder contract tests, documentation checks and the workspace AI audit passed.
Screenshot hashes match the five rendered frontend source files. Native packages
include the terrain guide, its reviewed images and Cesium license/credits.

After the plan coverage fixes, the entire local release preflight passed again
with Go 1.27.3, Node 24.22.2 and Python 3.13.17: all 18 Python tests, the full
Go suite (main package 106.173 s), syntax/rendering contracts, 90 terrain
estimate comparisons, normal UI/repair and all nine real browser workflows,
including parent fallback and permanent-deletion confirmation. Both sensitive
and generated-file scans passed with the new files staged. Separate vet,
module verification, documentation checks (33 Markdown files / 84 local
links), whitespace checks and the workspace AI audit passed. The measured Go
source fingerprint and all five screenshot source hashes match the candidate.
There are no outstanding implementation or local acceptance items in the plan
matrix; exact-revision CI, native packages and branch convergence remain the
delivery gates executed after a candidate commit exists.

Candidate `603fd84` passed all six jobs in push run `42226081819`, including
Linux race (main package 116.004 s), both repeated native packages and Docker.
PR run `42226090169` spent roughly eight minutes installing the browser;
its Go tests and eight real browser workflows passed before the job's old
20-minute limit cancelled the ninth. The application job now allows 30 minutes
for the measured installation + full preflight + race workload. Every check
and all nine cases remain required; this changes only the CI time budget.

The [PR #8 checks](https://github.com/Joe5027/map-tile-fetcher/pull/8/checks)
and [Validate workflow runs](https://github.com/Joe5027/map-tile-fetcher/actions/workflows/validate.yml)
are the authoritative evidence for each exact Git revision: Linux release
preflight/race/vet, Windows/Linux region tests, repeat native packages and Linux
Docker identity. Read actual `main` and remote refs before delivery instead of
using a cached or permanently embedded transient branch list. The dependency
baseline `9ca929a` already passed all six jobs in run `42211344918`.

The local Windows toolchain cannot run Go race with CGO disabled; Linux CI owns
that check. Full operating-system power-loss behavior was not tested. The
workflow fixture above adds OS-RSS evidence to the earlier heap-only comparison.
Synthetic terrain fixtures and one public root PNG do not validate
real-world vertical accuracy, every external provider or other terrain engines.
No production deployment, user database migration/replacement or wide-area real
download was performed. Existing authoritative-boundary gaps are unchanged and
remain documented in the region investigation.

## Main Integration Audit

PR #8 candidate `c4a8829` passed all six jobs in both PR run `42216034887`
and push run `42216027791`. It was merged as `f0a2865`; the resulting tree
`db9b02728254de5edd83d17d337de746c2659749` exactly matches the checked PR
merge candidate. The local Windows package passed manifest, CLI/API identity,
login, asset and worker/download verification (3,629 manifest files).

The merge message supplied through the API had only two bullets in each
language, while the repository validator requires three. Local verification of
`f0a2865` detected that metadata error. The initial follow-up `f7724ce` records the audit and
adds mandatory validation of the exact proposed merge message before the API
call; that initial commit changes no application code or validation rule.
The subsequent coverage audit adds the implementation and acceptance fixes
described above, carried by [PR #9](https://github.com/Joe5027/map-tile-fetcher/pull/9).
The historical merge
message is retained rather than rewriting protected main; a new compliant
commit and merge receive their own checks. Final exact-revision workflow state
remains available through the links above.
