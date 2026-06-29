# GitHub Star Conversion Playbook

This playbook is the first-stage execution guide for improving Map Tile
Fetcher conversion from repository visitor to star. It is intentionally focused
on Chinese GIS and map-development audiences.

## GitHub Repository Surface

Set the repository About description to:

```text
自托管 Go Web 地图瓦片下载器，支持框选范围、GeoJSON/行政区划、任务进度、MBTiles/ZIP 导出。
```

Set topics to:

```text
go, gis, map-tiles, tile-downloader, mbtiles, geojson, leaflet, offline-maps, tianditu, mapbox
```

Homepage can stay empty until there is a hosted demo or documentation site.

## Release Checklist

1. Run the release preflight:

   ```powershell
   cd apps\admin-region-tiler
   node .\scripts\release_preflight.mjs
   ```

2. Build release assets from `apps/admin-region-tiler`:

   ```powershell
   $env:GOOS='windows'; $env:GOARCH='amd64'; go build -o ..\..\dist\release\v0.1.0\windows-amd64\map-tile-fetcher.exe .
   $env:GOOS='linux'; $env:GOARCH='amd64'; go build -o ..\..\dist\release\v0.1.0\linux-amd64\map-tile-fetcher .
   Remove-Item Env:\GOOS
   Remove-Item Env:\GOARCH
   ```

3. Copy runtime resources into each asset directory:

   ```powershell
   foreach ($target in @($winDir, $linuxDir)) {
     Copy-Item -LiteralPath .\conf.toml -Destination $target -Force
     Copy-Item -LiteralPath .\.env.example -Destination $target -Force
     Copy-Item -LiteralPath .\README.md -Destination $target -Force
     Copy-Item -LiteralPath .\static -Destination $target -Recurse -Force
     Copy-Item -LiteralPath .\geojson -Destination $target -Recurse -Force
     Copy-Item -LiteralPath .\deploy -Destination $target -Recurse -Force
     New-Item -ItemType Directory -Force -Path (Join-Path $target 'data'), (Join-Path $target 'output') | Out-Null
   }
   ```

4. Package:

   ```powershell
   Compress-Archive -Path ..\..\dist\release\v0.1.0\windows-amd64\* -DestinationPath ..\..\dist\release\map-tile-fetcher-v0.1.0-windows-amd64.zip -Force
   tar -czf ..\..\dist\release\map-tile-fetcher-v0.1.0-linux-amd64.tar.gz -C ..\..\dist\release\v0.1.0\linux-amd64 .
   ```

5. Create GitHub Release `v0.1.0` and paste the content from
   `docs/releases/v0.1.0.md`.

## Distribution Copy

Short post:

```text
做了一个 Go + Leaflet 的自托管地图瓦片下载器：支持框选 bbox、GeoJSON/行政区划、多地图源、任务进度、失败记录，以及 ZIP/MBTiles 导出。

如果你需要下载自己有授权的地图瓦片、生成离线地图素材或按行政区划批量切范围，可以试试这个工具。

GitHub: https://github.com/Joe5027/map-tile-fetcher
```

Long post title ideas:

- 做了一个 Go + Leaflet 自托管地图瓦片下载器
- 用 Go 做一个支持 bbox 和 GeoJSON 的地图瓦片下载器
- 开源一个支持 MBTiles/ZIP 导出的地图瓦片下载 Web 工具

Do not use “求 star” as the primary call to action. Lead with the concrete GIS
workflow and let the repository page earn the star.

## First Distribution Round

Recommended order:

1. V2EX: technical share, focus on Go + Leaflet + self-hosted workflow.
2. 掘金 or CSDN: tutorial-style article, include screenshots and quick start.
3. 知乎/GIS groups: short copy plus screenshot or GIF.
4. GitHub related projects/issues only when the discussion is directly relevant.

Use the same three links each time:

- Repository: `https://github.com/Joe5027/map-tile-fetcher`
- Release notes: `docs/releases/v0.1.0.md` or the GitHub Release URL after publishing.
- Demo screenshots: `docs/assets/dashboard-overview.png`,
  `docs/assets/bbox-task-creation.png`, `docs/assets/artifact-download.png`.

## Feedback Loop

Before posting, record the GitHub Insights > Traffic baseline:

| Date | Views | Unique visitors | Clones | Stars | Notes |
| --- | ---: | ---: | ---: | ---: | --- |
| 2026-06-29 |  |  |  |  | Before first distribution |

After posting, record the same numbers at:

- 24 hours
- 72 hours
- 7 days

Decision rules:

- High views, low stars: improve README first screen, screenshots, and release
  clarity.
- Low views: test a clearer title and add one more targeted channel.
- Repeated setup questions: add the answer to README and user manual.
- Repeated legal/token questions: expand the compliance note and FAQ.
