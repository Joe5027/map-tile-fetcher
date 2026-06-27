# Map Tile Downloaders

This repository is a clean open-source handoff for two related map tile
downloader applications. They are intentionally kept as separate apps for the
initial release, with a later merge plan documented in
[`docs/merge-plan.md`](docs/merge-plan.md).

## Repository Layout

- `apps/range-downloader` - .NET 6 Web app for drawing a bounding box on a map,
  entering a Tianditu token, and downloading `img`, `cia`, and `vec` tile
  layers with task status, retry records, and packaged output.
- `apps/admin-region-tiler` - Go 1.25+ Web app for downloading tiles by
  administrative GeoJSON regions, with multiple map sources, tasks, scheduled
  runs, persistent state, Docker deployment, and output downloads.
- `docs/merge-plan.md` - planned path for merging both apps into one product.

The repository does not include old restored source folders, UI design
packages, temporary directories, screenshots, runtime databases, downloaded
tiles, or historical release archives.

## Quick Validation

Validate the range downloader:

```powershell
cd apps/range-downloader
dotnet build .\TianDiTuDownLoader.Web.csproj -c Release
```

Validate the administrative region tiler:

```powershell
cd apps/admin-region-tiler
go test ./...
```

## Local Runtime Notes

Runtime secrets and generated data must stay local:

- Put real Tianditu or Mapbox tokens in local configuration only.
- Keep `.env`, `data/`, `output/`, `tiles/`, `bin/`, `obj/`, `publish*/`, and
  release archives out of Git.
- Placeholder values such as `YOUR_TIANDITU_TOKEN` and `YOUR_MAPBOX_TOKEN` are
  safe to keep in examples.

## License

This repository is released under the Apache License 2.0. See [`LICENSE`](LICENSE).
