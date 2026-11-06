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
bounded transactions (256 records or 250 ms), counting success after commit.
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

## End-To-End Download Fixture Comparison

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

## Acceptance Evidence And Limits

The integrated local release preflight passed with the pinned Node 24.22.2:
18 Python fixtures, `go test ./...` (main package 107.054 s), all JavaScript
syntax/rendering checks, complete browser smoke/repair, sensitive-value scan
and generated-file scan. Separate `go vet ./...`, `go mod verify`, three release
builder contract tests, documentation checks and the workspace AI audit passed.
Screenshot hashes match the five rendered frontend source files. Native packages
include the terrain guide, its reviewed images and Cesium license/credits.

The [PR #8 checks](https://github.com/Joe5027/map-tile-fetcher/pull/8/checks)
and [Validate workflow runs](https://github.com/Joe5027/map-tile-fetcher/actions/workflows/validate.yml)
are the authoritative evidence for each exact Git revision: Linux release
preflight/race/vet, Windows/Linux region tests, repeat native packages and Linux
Docker identity. Read actual `main` and remote refs before delivery instead of
using a cached or permanently embedded transient branch list. The dependency
baseline `9ca929a` already passed all six jobs in run `42211344918`.

The local Windows toolchain cannot run Go race with CGO disabled; Linux CI owns
that check. OS-RSS and full operating-system power-loss behavior were not
measured. Synthetic terrain fixtures and one public root PNG do not validate
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
`f0a2865` detected that metadata error. This follow-up records the audit and
adds mandatory validation of the exact proposed merge message before the API
call. It changes no application code or validation rule. The historical merge
message is retained rather than rewriting protected main; a new compliant
commit and merge receive their own checks. Final exact-revision workflow state
remains available through the links above.
