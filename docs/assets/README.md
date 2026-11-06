# Documentation Images

These reviewed images use local, sanitized fixtures. No credentials, user tasks
or downloaded production data appear in them. `terrain-preview.png` shows
synthetic elevation, not a real geographic DEM.
The 2D screenshots use neutral local tiles and show the actual selection
geometry and controls; the capture does not contact commercial basemap servers.

From `apps/admin-region-tiler`, reproduce with:

```powershell
npm ci
npx playwright install chromium
node scripts/capture_docs.mjs
```

The capture command runs the normal UI smoke and repair checks, starts isolated
local services, generates real quantized-mesh fixtures and loads local Cesium.
External terrain/imagery requests are rejected. It checks 390/768/1440 pixel
layouts before writing the four 1440-pixel documentation images.

`screenshots.json` records source revision, working-tree state, hashes of the
rendered frontend sources, dependency versions and terrain/browser assertions.
Source hashes distinguish an in-progress capture from its eventual committed
source without requiring a self-referential commit hash. Refresh images and
the manifest whenever the illustrated flow changes.

Temporary captures and runtime outputs belong in ignored temporary directories.
Only these intentional documentation assets are tracked.
