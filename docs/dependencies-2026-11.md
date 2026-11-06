# Dependency decisions — 2026-11-06

Versions were checked against official releases and the package registries during
this upgrade. These are reproducible choices, not a standing assertion that a
version will remain latest. Future work checks the dependencies it touches and
records the benefit of upgrading or the reason to retain them.

| Dependency | Locked decision | Relevant benefit and validation |
| --- | --- | --- |
| Go | 1.27.3 in `go.mod`, CI and Docker | Current stable toolchain; required by the upgraded network stack. Full tests, vet, native packages and Linux race. [Releases](https://go.dev/doc/devel/release) |
| Gin / CORS | 1.12.0 / 1.7.10 | Current routing/binding and cross-origin fixes; validate login, payload validation, task routes and downloads. [Gin](https://github.com/gin-gonic/gin/releases/tag/v1.12.0), [CORS](https://github.com/gin-contrib/cors/releases/tag/v1.7.10) |
| Viper | 1.21.0 | Modern configuration parsing and smaller dependency graph; retain TOML, environment overrides and list settings. [Release](https://github.com/spf13/viper/releases/tag/v1.21.0) |
| modernc SQLite | 1.61.1 | Cancellation during lock waits and statement handling matter to workers, recovery and batch writes. [Changelog](https://gitlab.com/cznic/sqlite/-/raw/master/CHANGELOG.md) |
| x/image | 0.47.0 | Replace the 2019 image package now that WebP decoding is used; verify real WebP pixels. [Official module](https://pkg.go.dev/golang.org/x/image@v0.47.0) |
| Logrus | 1.10.2 | Incorporates formatter/entry race and reentrant logging deadlock fixes plus lower allocation overhead; relevant to parallel workers. No logging framework migration. [1.10 changes](https://github.com/sirupsen/logrus/releases/tag/v1.10.0), [patch release](https://github.com/sirupsen/logrus/releases/tag/v1.10.2) |
| Node | 24.22.2 | Retain the existing 24 LTS line and update its patch level; use `.node-version` for CI and the same version for Docker assets. Node is a build/test dependency, not an installed-app runtime. [Release](https://nodejs.org/en/blog/release/v24.22.2) |
| Playwright | 1.65.0 | Project-local lock and matching browser installation replace floating global tools. [Release](https://github.com/microsoft/playwright/releases/tag/v1.65.0) |
| Cesium | 1.147.0 | Offline viewer and actual quantized-mesh interoperability test. npm prepares `static/vendor/cesium`; packaged builds include it. [Release](https://github.com/CesiumGS/cesium/releases/tag/1.147) |
| Shapely | 2.2.0 | Scalar geometry/context improvements apply to region maintenance. Hash-locked CPython 3.13 Windows/Linux wheels; all region fixtures rerun. [Release notes](https://shapely.readthedocs.io/en/stable/release/2.x.html) |
| Alpine | 3.24.4 | Current stable runtime image; native binary remains CGO-free. [Downloads](https://alpinelinux.org/downloads/) |
| GitHub Actions | checkout 7.0.1, setup-go 7.0.0, setup-node/setup-python 7.1.0 | Node 24 action runtimes, maintained toolchain/cache handling and manifest retry fixes. Exact release tags; validate on hosted runners. [Checkout](https://github.com/actions/checkout/releases/tag/v7.0.1), [Go](https://github.com/actions/setup-go/releases/tag/v7.0.0), [Node](https://github.com/actions/setup-node/releases/tag/v7.1.0), [Python](https://github.com/actions/setup-python/releases/tag/v7.1.0) |

Retained deliberately:

- Leaflet 1.9.4: current stable 1.x API supports the existing map workflow;
  migrating to a prerelease adds no needed capability. [Release](https://github.com/Leaflet/Leaflet/releases/tag/v1.9.4)
- `atlasdatatech/orb` 0.2.2 replacement: the downloader needs its streaming tile
  enumeration; replacing it blindly would remove used functionality.
- NumPy 2.2.6: used by the Shapely environment, not directly by these scripts;
  the newer linear algebra changes do not improve this workload.
- nested-logrus-formatter 1.3.0: 1.3.1 changes trace-level color only; this app
  does not enable trace output. [Release](https://github.com/antonfisher/nested-logrus-formatter/releases/tag/v1.3.1)
- `shortid` and `pb.v1`: registry lookup found no newer version on the selected
  module paths. A progress-bar major-version migration is outside this change.

`go mod tidy` removes unused historical dependencies including
`mattn/go-sqlite3`, cloud clients and the old configuration dependency chain.
The compiled SQLite implementation remains pure Go. Transitive dependencies are
recorded in `go.sum` and `package-lock.json`; no blanket `go get -u` is used.

## Reproduce

From `apps/admin-region-tiler`:

```sh
go mod verify
go test ./...
go vet ./...
npm ci --no-audit --no-fund
npx playwright install chromium
node scripts/smoke_ui.mjs
python -m venv .venv-region
# Windows uses .venv-region/Scripts/python; Linux uses .venv-region/bin/python.
.venv-region/Scripts/python -m pip install --require-hashes --only-binary=:all: -r scripts/requirements-region.lock
.venv-region/Scripts/python -B -m unittest discover -s scripts -p 'test_*.py' -v
```

Runtime recovery, terrain and packaging acceptance results are recorded in
[the upgrade record](upgrade-2026-11.md); dependency installation alone does not
prove those workflows.
