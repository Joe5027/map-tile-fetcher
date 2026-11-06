# User Manual

This manual explains how to use Map Tile Fetcher after the Go-only merge.

## 1. Start The App

From the Go app directory:

```powershell
cd apps/admin-region-tiler
npm ci --omit=dev --no-audit --no-fund
go run .
```

Open `http://127.0.0.1:8081/` in a browser.

Default local login:

- Username: `admin`
- Password: `adminmap`

For trusted local-only use, login can be disabled with `AUTH_ENABLED=false`.
For deployment, keep login enabled and override the default username and
password through local environment variables or deployment secrets.

## 2. Service Credentials

The **Service Credentials** section is shared by both download modes.

- **Tianditu Token** replaces `YOUR_TIANDITU_TOKEN` in Tianditu source URLs and
  is also used by the range preview when Tianditu preview layers are selected.
- **Mapbox Token** replaces `YOUR_MAPBOX_TOKEN` in Mapbox source URLs.
- **Mapbox SKU** replaces `YOUR_MAPBOX_SKU` when supplied; when left empty, the
  placeholder SKU query parameter is removed from the request URL.

Credentials are submitted for task creation and preview registration. Task
source URLs are retained in the private control database. Preview registration
renews 60 seconds before expiry; logout and credential changes clear its cache.
Do not commit
real tokens, `.env` files, runtime databases, downloaded tiles, or generated
archives to Git.

## 3. Administrative Region Download

Use this mode when the target area is represented by maintained GeoJSON region
files.

1. Choose **Administrative Region Download**.
2. Enter a task name.
3. Fill the service credential fields required by the selected map source.
4. Select a map source provider and one or more layers.
5. Configure region levels. The default flow starts from world and China, then
   lets you choose province, city, and district levels when available.
6. Choose worker count, save workers, request delay, output format, and schedule.
7. Review the inline summary and submit. Submission is disabled while the request is pending.

The app creates one parent task with child source tasks. Each selected layer runs
as a child task under the same parent task.

## 4. Range Download

Use this mode when the target area should be selected directly on the map.

1. Choose **Range Download**.
2. Enter a task name.
3. Choose sources from the shared catalogue and fill credentials only when required.
4. Use Tianditu shortcuts or choose imagery, vector or elevation sources directly.
5. Select rectangle or polygon drawing mode.
6. Click the map to define the range:
   - Rectangle: click two opposite corners.
   - Polygon: click at least three points.
7. Confirm or edit the coordinate and zoom fields.
8. Review the tile estimate, output format, and schedule.
9. Review the inline summary and submit with the displayed worker, saver and delay settings.

Range coordinates use WGS84 longitude/latitude with the Web Mercator tile
scheme.

## 5. Scheduling

Each task supports:

- **Immediate**: starts after creation.
- **Once**: starts at the selected local date and time.

Immediate, scheduled and retry runs share a durable queue (`queued`). The default
limit is three running children. Paused children release their slot; resuming
queues them until capacity is available. Restart automatically continues the
original run, reusing valid committed tiles. Explicitly paused work stays paused.
Conversion and packaging can resume after the old worker relinquishes ownership.
The parent budget is 1,000,000 unique requests across all selected sources.
Complex or overlapping geometry is deduplicated during preparation; a bounding
estimate alone does not reject it. Terrain counts include ancestors and adjacent
source rasters needed for sampling. Workers default to 3 and accept 1-50, capped
by the source policy; effective delay is at least the source minimum.

## 6. Output Formats

The task creation form supports:

- **ZIP file tree**: tile files are written to the filesystem and packaged as a
  ZIP artifact.
- **MBTiles**: MBTiles output is exposed directly as the artifact.
- **Cesium terrain**: convert Terrarium elevation to a quantized-mesh ZIP, with an offline 3D preview in the application.

Terrain imagery remains ordinary ZIP/MBTiles data; Mapbox terrain vector tiles
are not elevation. Choose the public Terrarium source for elevation, with maximum
level 14 by default or 15 when needed. No AWS account is required. Open the child
task's 3D preview or download its ZIP. See the [terrain guide](terrain.md).
The preview renders on demand while idle; camera movement and elevation-color
changes still update normally, without reducing visual quality.

Artifacts are stored on disk and indexed in SQLite. SQLite stores control
metadata only; raw tile payloads stay in the filesystem artifact paths.

## 7. Task Management

Task history shows 50 parents per page and supports status filtering. Counts
cover the entire history. Only a visible task page polls every five seconds;
returning refreshes immediately. Background updates preserve focus, selection,
menus and expansion. Preparation, downloading, conversion, verification,
packaging and publication have separate phase feedback.

Available actions depend on task state:

- Pause running tasks.
- Resume paused tasks.
- Cancel scheduled, pending, running, or paused tasks.
- Delete completed, failed, partially failed, or cancelled tasks.
- Download artifacts when artifact status is ready.
- Retry failed tiles when retryable failure records exist.

Deleting a parent removes children, historical runs, generated artifacts,
failures and associated metadata. Cancel active or paused tasks and wait for
completion first. Built-in GeoJSON is never removed; generated regions remain
until their final reference is released. Failed cleanup retains a retryable
manifest. Task/run IDs isolate actual paths, so display names cannot overwrite
other runs. Downloads retain the last published artifact until publication succeeds.

## 8. Failure Records And Retry

When tile downloads fail, retryable failures are persisted and can be viewed
through the task response and failure endpoint.

Use **Retry Failed Tiles** from the task action menu when available. The retry
run copies the previous usable output and fills unresolved retryable coordinates,
then publishes a complete cumulative ZIP/MBTiles. Historical events are retained;
resolved events leave current failure counts. Per-run progress and cumulative
integrity are separate. Disk checks use the additional bytes needed by copying
and packaging, without an unconditional 1 GiB surcharge. An explicit
`task.min_free_disk_bytes` reserve remains available. Unknown future tile sizes
can still exhaust space; a failed write does not replace the published artifact.

Use the task menu's local reconciliation action to verify historical output
without downloading. Retries first queue, then validate the baseline in the
worker. A missing baseline produces a failed run with an actionable error;
explicitly recreate a complete task to download again. A terrain run whose root
failed can reuse its valid raw elevation cache. Retry never silently expands
into a full download. See [upgrade evidence](upgrade-2026-11.md).

Common failure categories:

- `429`: request rate is too high.
- `418` or blocked category: the source may be blocking the current egress IP.
- `502`, `503`, `504`: upstream service instability.
- proxy or network categories: local network or proxy path failure.

Increase request delay, reduce workers, or switch egress/proxy when rate limits
or blocking appear.

## 9. Local Files And Safety

Keep these out of Git:

- `.env`
- `data/`
- `output/`
- `tiles/`
- `bin/`
- `obj/`
- `publish*/`
- logs, databases, archives, binaries, temporary runtime screenshots, and downloaded tiles

Reviewed documentation images in `docs/assets/` are allowed when reproduced
from sanitized local fixtures.

Safe placeholders in docs and examples:

- `YOUR_TIANDITU_TOKEN`
- `YOUR_MAPBOX_TOKEN`
- `YOUR_MAPBOX_SKU`

## 10. Validation For Maintainers

Before publishing or handing off a build:

```powershell
cd apps/admin-region-tiler
node .\scripts\release_preflight.mjs
```

The preflight runs Go tests, frontend syntax checks, browser UI smoke, sensitive
value scanning, and generated-file scanning.

## Password Maintenance

Use the account menu to change your own password. Offline recovery is available via
`tiler admin reset-password --database PATH --username NAME` with hidden terminal input.
Stop all server/worker processes first. See [password recovery](password-recovery.md).
