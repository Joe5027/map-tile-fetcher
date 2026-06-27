# TianDiTuDownLoader Web

Web version of the restored TianDiTu tile downloader.

The app starts one parent job with independent child jobs for:

- `img`: satellite imagery.
- `cia`: road/annotation layer.
- `vec`: electronic map layer.

All child jobs share the same token, bounding box, zoom range, concurrency setting, and container output root. Each layer writes to its own subdirectory under `/data/tiles`, keeps its own failure queue, and can be retried independently.

## Local build

```powershell
dotnet build .\TianDiTuDownLoader.Web.csproj -c Release
```

## Publish for Docker

```powershell
dotnet publish .\TianDiTuDownLoader.Web.csproj -c Release -r linux-musl-x64 --self-contained true -p:PublishSingleFile=false -p:InvariantGlobalization=true -o .\publish-linux-musl
```

## Docker

```powershell
docker build -t tianditu-downloader-web:local .
docker run --rm -p 18080:8080 -v C:\tiles\range-downloader:/data/tiles --name tianditu-downloader-web tianditu-downloader-web:local
```

Open:

```text
http://localhost:18080
```

The container writes tiles to `/data/tiles`. Mount that path to a host directory for persistence. Output is grouped by layer, for example `/data/tiles/img/0/0/0.png`, `/data/tiles/cia/0/0/0.png`, and `/data/tiles/vec/0/0/0.png`.
