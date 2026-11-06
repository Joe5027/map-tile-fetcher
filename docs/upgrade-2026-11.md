# November 2026 Upgrade Record

This record describes the implementation approved on 2026-11-06. It is not a
production deployment record. Tests use temporary databases and local tile
fixtures; no user database or existing downloads are used.

## Delivery Status

| Batch | State |
| --- | --- |
| Portable rules, dependency review, main completion and handoff | Committed: `da33be6` |
| Download configuration and format correctness | Committed: `32ab1ca`, `79d0184` |
| Automatic recovery, queue slots and storage performance | Implemented; full Go suite and actual process interruption checks passed |
| Dependency/toolchain upgrade | Locked and tested on an isolated source snapshot; native package/CI checks pending |
| Cesium terrain generation and offline preview | Terrain codec and sampling tested; application integration in progress |
| Source selection, paginated history and responsive interactions | Pagination and visible-page refresh tested; terrain source UI pending |
| Manuals, reproducible images, native packages and final main checks | Pending |

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

Validation is recorded in each bilingual commit. Full upgrade acceptance,
benchmarks and platform checks remain pending until their batches finish.

## Terrain Core

`internal/terrain` decodes Terrarium pixels, samples a globally aligned 65 × 65
grid, and writes gzip quantized-mesh with TMS availability and complete ancestor
chains. Tests independently decode the binary and check signed/zero heights,
Mercator latitude, triangle winding, edges, dateline sharing and missing input.
No missing height is replaced with zero. `go test ./internal/terrain` and
`go vet ./internal/terrain` passed with Go 1.27.3.

The whole-world Mercator root cannot have a finite conservative horizon point.
The encoder disables that optional culling through NaN for such coarse tiles;
actual Cesium 1.147 multi-view validation is still required before delivery.
Windows race verification is unavailable with the current CGO-disabled toolchain;
Linux race verification remains part of final acceptance.

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
Missing/corrupt pending ZIPs return to the original run for rebuilding, keeping
raw data, the previous artifact and unrelated valid publications. Both failure
cases were reproduced before repair. The packing injection now waits up to 90
seconds for an actual unfinished ZIP; the earlier 25-second deadline proved
sensitive to slow disks and was not a product performance requirement.
