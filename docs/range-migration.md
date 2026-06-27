# Range Downloader Migration Note

The former `.NET 6` range downloader has been retired from the runtime tree.
Its useful behavior was ported into `apps/admin-region-tiler`:

- bounding-box task creation through `POST /api/tasks` with `mode: "bbox"`
- bbox tile enumeration in Go
- Tianditu `img`, `cia`, and `vec` layer source creation
- Leaflet-based range preview in the static Web UI
- tile estimate display
- shared Go task engine, worker progress, retries, SQLite state, failures, and
  artifacts

The retired app originally came from `01-TianDiTuDownLoader_web`. Do not
reintroduce the .NET runtime code unless the user explicitly asks for an
archival branch or separate reference package.

Current validation for the unified implementation:

```powershell
cd apps/admin-region-tiler
go test ./...
node --check .\static\script.js
```
