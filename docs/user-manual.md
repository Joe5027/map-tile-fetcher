# User Manual

This manual explains how to use Map Tile Fetcher after the Go-only merge.

## 1. Start The App

From the Go app directory:

```powershell
cd apps/admin-region-tiler
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

Credentials are submitted only with the task creation payload. Do not commit
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
7. Submit the task and confirm the summary dialog.

The app creates one parent task with child source tasks. Each selected layer runs
as a child task under the same parent task.

## 4. Range Download

Use this mode when the target area should be selected directly on the map.

1. Choose **Range Download**.
2. Enter a task name.
3. Fill **Tianditu Token** in the shared Service Credentials section.
4. Choose Tianditu layers: `img`, `cia`, and/or `vec`.
5. Select rectangle or polygon drawing mode.
6. Click the map to define the range:
   - Rectangle: click two opposite corners.
   - Polygon: click at least three points.
7. Confirm or edit the coordinate and zoom fields.
8. Review the tile estimate, output format, and schedule.
9. Submit the task and confirm the summary dialog.

Range coordinates use WGS84 longitude/latitude with the Web Mercator tile
scheme.

## 5. Scheduling

Each task supports:

- **Immediate**: starts after creation.
- **Once**: starts at the selected local date and time.

Scheduled tasks appear as `scheduled` or `pending` until their run time is due.

## 6. Output Formats

The task creation form supports:

- **ZIP file tree**: tile files are written to the filesystem and packaged as a
  ZIP artifact.
- **MBTiles**: MBTiles output is exposed directly as the artifact.

Artifacts are stored on disk and indexed in SQLite. SQLite stores control
metadata only; raw tile payloads stay in the filesystem artifact paths.

## 7. Task Management

The task list shows status, progress, child source tasks, failures, and artifact
state.

Available actions depend on task state:

- Pause running tasks.
- Resume paused tasks.
- Cancel scheduled, pending, running, or paused tasks.
- Delete completed, failed, partially failed, or cancelled tasks.
- Download artifacts when artifact status is ready.
- Retry failed tiles when retryable failure records exist.

Deleting a task removes task records and generated task artifacts. Shared
GeoJSON resources are protected from deletion.

## 8. Failure Records And Retry

When tile downloads fail, retryable failures are persisted and can be viewed
through the task response and failure endpoint.

Use **Retry Failed Tiles** from the task action menu when available. The retry
run reuses failure records and does not recreate the full task.

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
- logs, databases, archives, binaries, screenshots, and downloaded tiles

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
