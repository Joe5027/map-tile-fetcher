# Map Tile Fetcher

Self-hosted Go Web app for downloading authorized map tiles by bounding box or
GeoJSON/admin regions. It provides a tabbed UI for task creation and task
history, plus ZIP/MBTiles artifacts, retries, and Docker deployment assets.

> Use this tool only with map tile services that you are authorized to access
> and download. The project does not grant permission to bypass third-party
> terms, quotas, or access controls.

## Features

- Bounding-box task creation with a Leaflet map.
- GeoJSON/admin-region task creation.
- Configurable map sources, including Tianditu, Mapbox, OSM, Google examples,
  and custom tile URLs.
- Immediate or one-time scheduled tasks.
- ZIP file-tree and MBTiles outputs.
- Task history, progress, failure records, retries, pause/resume/cancel, and
  artifact downloads.

## Run From Source

```powershell
go run .
```

Open `http://127.0.0.1:8081/`.

Development login:

- Username: `admin`
- Password: `adminmap`

Override the default credentials before production deployment.

## Docker

This directory already includes the Docker assets needed to build the image:

- `Dockerfile`
- `docker-compose.yml`
- `.dockerignore`

Recommended Docker Compose workflow:

```powershell
Copy-Item .env.example .env
docker compose up --build -d
```

Manual image build:

```powershell
docker build -t map-tile-fetcher:latest .
```

Manual run:

```powershell
docker run --rm --name map-tile-fetcher `
  -p 8081:8081 `
  --env-file .env `
  -v "${PWD}\data:/app/data" `
  -v "${PWD}\output:/app/output" `
  -v "${PWD}\geojson:/app/geojson" `
  -v "${PWD}\conf.toml:/app/conf.toml:ro" `
  map-tile-fetcher:latest
```

Use `.env.example` for port, database, timezone, and default auth settings.

## Validation

```powershell
go test ./...
node --check .\static\script.js
node .\scripts\release_preflight.mjs
```
