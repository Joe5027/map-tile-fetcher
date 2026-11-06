# Dependency decisions — 2026-11-06

Versions were checked against official releases and the package registries during
this upgrade. These are reproducible choices, not a standing assertion that a
version will remain latest. Future work checks the dependencies it touches and
records the benefit of upgrading or the reason to retain them.

The source baseline used Go 1.25.0 (local compiler 1.25.3), Gin 1.9.1, CORS
1.4.0, Viper 1.7.1, modernc SQLite 1.47.0, Logrus 1.6.0 and a 2019 x/image
pseudo-version. Playwright was globally resolved and Cesium was absent. Shapely
was 2.1.2; the Docker runtime used floating `alpine:latest`. The table below
records the reviewed candidates selected for this change.

| Dependency | Locked decision | Relevant benefit and validation |
| --- | --- | --- |
| Go | 1.27.3 in `go.mod`, CI and Docker | Current stable toolchain; required by the upgraded network stack. Full tests, vet, native packages and Linux race. [Releases](https://go.dev/doc/devel/release) |
| Gin / CORS | 1.12.0 / 1.7.10 | Current routing/binding and cross-origin fixes; validate login, payload validation, task routes and downloads. [Gin](https://github.com/gin-gonic/gin/releases/tag/v1.12.0), [CORS](https://github.com/gin-contrib/cors/releases/tag/v1.7.10) |
| Viper | 1.21.0 | Modern configuration parsing and smaller dependency graph; retain TOML, environment overrides and list settings. [Release](https://github.com/spf13/viper/releases/tag/v1.21.0) |
| modernc SQLite | 1.61.1 | Cancellation during lock waits and statement handling matter to workers, recovery and batch writes. [Changelog](https://gitlab.com/cznic/sqlite/-/raw/master/CHANGELOG.md) |
| x/image | 0.47.0 | Replace the 2019 image package now that WebP decoding is used; verify real WebP pixels. [Official module](https://pkg.go.dev/golang.org/x/image@v0.47.0) |
| x/crypto | 0.58.0; candidate equals lock | The released changes include a Salsa20 LoongArch keystream fix and SSH algorithms; this app uses bcrypt, so these are not claimed as a password speedup or a fix to an observed bcrypt bug. Retain the upgraded compatible Go module graph and verify login/hash migration. [Release tag](https://go.googlesource.com/crypto/+/refs/tags/v0.58.0), [released fix](https://go.googlesource.com/crypto/+/2437861a30358e949c7be3c9f876233fd28c20fe), [registry](https://proxy.golang.org/golang.org/x/crypto/@latest) |
| x/sys | 0.49.0; candidate equals lock | The app now directly uses Windows/Unix disk-space and maintenance-lock APIs. Released changes include Darwin pointer-lifetime corrections and Linux syscall fixes; those are platform-specific, not a measured Windows download improvement. Retain the maintained OS API dependency with the updated toolchain; process-restart/lock and disk-budget tests exercise the app's use. [Release tag](https://go.googlesource.com/sys/+/refs/tags/v0.49.0), [Darwin fix](https://go.googlesource.com/sys/+/cdfd9840b35fca673dd1b27ef69a13b9f5be0d04), [registry](https://proxy.golang.org/golang.org/x/sys/@latest) |
| x/term | 0.47.0; candidate equals lock | Admin CLI password input uses `ReadPassword`. The release line fixes terminal `ReadLine` handling of data returned with EOF and updates x/sys; this is not the same path as password input, so no unmeasured CLI improvement is claimed. Retain API compatibility and the shared x/sys version. [Release tag](https://go.googlesource.com/term/+/refs/tags/v0.47.0), [ReadLine fix](https://go.googlesource.com/term/+/7c2fb74aea627b563f897ce0ba62658826a024c8), [registry](https://proxy.golang.org/golang.org/x/term/@latest) |
| Logrus | 1.10.2 | Incorporates formatter/entry race and reentrant logging deadlock fixes plus lower allocation overhead; relevant to parallel workers. No logging framework migration. [1.10 changes](https://github.com/sirupsen/logrus/releases/tag/v1.10.0), [patch release](https://github.com/sirupsen/logrus/releases/tag/v1.10.2) |
| Node | 24.22.2 | Retain the existing 24 LTS line and update its patch level; use `.node-version` for CI and the same version for Docker assets. Node is a build/test dependency, not an installed-app runtime. [Release](https://nodejs.org/en/blog/release/v24.22.2) |
| Playwright | 1.65.0 | Project-local lock and matching browser installation replace floating global tools. [Release](https://github.com/microsoft/playwright/releases/tag/v1.65.0) |
| Cesium | 1.147.0 | Offline viewer and actual quantized-mesh interoperability test. npm prepares `static/vendor/cesium`; packaged builds include it. [Release](https://github.com/CesiumGS/cesium/releases/tag/1.147) |
| Shapely | 2.2.0 | Scalar geometry/context improvements apply to region maintenance. Hash-locked CPython 3.13 Windows/Linux wheels; all region fixtures rerun. [Release notes](https://shapely.readthedocs.io/en/stable/release/2.x.html) |
| Python | 3.13.17 in `.python-version`; CI reads the file | Replaces the local 3.13.2 validation baseline and floating CI 3.13 selection with the newest stable 3.13 patch available for both Windows x64 and Linux x64 in the official Actions manifest. Bundled pip 26.2.2 updates certificate trust, relevant when installing the region-tool wheels; keep the CPython 3.13 ABI and existing wheel hashes. Use a private runtime/venv rather than replacing system Python. [Release](https://www.python.org/downloads/release/python-31317/), [runner manifest](https://github.com/actions/python-versions/blob/main/versions-manifest.json), [Windows archive hashes](https://www.python.org/ftp/python/3.13.17/windows-3.13.17.json) |
| Alpine | 3.24.4 | Current stable runtime image; native binary remains CGO-free. [Downloads](https://alpinelinux.org/downloads/) |
| GitHub Actions | checkout 7.0.1, setup-go 7.0.0, setup-node/setup-python 7.1.0 | Node 24 action runtimes, maintained toolchain/cache handling and manifest retry fixes. Exact release tags; validate on hosted runners. [Checkout](https://github.com/actions/checkout/releases/tag/v7.0.1), [Go](https://github.com/actions/setup-go/releases/tag/v7.0.0), [Node](https://github.com/actions/setup-node/releases/tag/v7.1.0), [Python](https://github.com/actions/setup-python/releases/tag/v7.1.0) |

Retained deliberately:

- Leaflet 1.9.4: current stable 1.x API supports the existing map workflow;
  migrating to a prerelease adds no needed capability. [Release](https://github.com/Leaflet/Leaflet/releases/tag/v1.9.4)
- `atlasdatatech/orb` 0.2.2 replacement: the downloader needs its streaming tile
  enumeration; replacing it blindly would remove used functionality.
- NumPy 2.2.6 retained after reviewing 2.5.4: used by the Shapely environment,
  not directly by these scripts; the newer linear algebra changes do not
  improve this workload. [Release](https://github.com/numpy/numpy/releases/tag/v2.5.4)
- nested-logrus-formatter 1.3.0: 1.3.1 changes trace-level color only; this app
  does not enable trace output. [Release](https://github.com/antonfisher/nested-logrus-formatter/releases/tag/v1.3.1)
- `shortid` and `pb.v1`: registry lookup found no newer version on the selected
  module paths. A progress-bar major-version migration is outside this change.
- `shiena/ansicolor` `v0.0.0-20230509054315-a9deabde6e02`: the official Go
  registry's latest version is the existing lock. Its last commit adds module
  metadata; the console writer already serves Windows logging, with no newer
  release or demonstrated need for a replacement. [Registry](https://proxy.golang.org/github.com/shiena/ansicolor/@latest),
  [commit](https://github.com/shiena/ansicolor/commit/a9deabde6e02)

Python candidates were also checked on 2026-11-06. The latest stable feature
release is [3.15.2](https://www.python.org/downloads/release/python-3152/), but
switching minor versions would invalidate the CPython 3.13 NumPy/Shapely wheel
selection and require a separately validated dependency-lock update. This
maintenance workload does not establish a benefit from those language/runtime
features. [3.13.18](https://www.python.org/downloads/release/python-31318/) adds
SSL certificate distrust and other security fixes, but is source-only; the
official Actions manifest currently supplies Linux/RHEL archives and no Windows
archive for that patch. Do not claim that 3.13.17 includes those additional
3.13.18 fixes. Reconsider the patch when a supported common binary is available,
or review a minor-version migration with regenerated wheel hashes as its own
change. No NumPy major/minor upgrade is needed for the selected 3.13.17 patch.

The local Windows check used the PSF 3.13.17 x64 ZIP from that release's
Windows manifest, verified SHA-256
`b26ca4b4ee51e157836049934d2b8eabd55939be296f3130254251b6df96ec2f`,
and created a separate ignored venv. Installing the unchanged wheel lock with
`--require-hashes --only-binary=:all:` succeeded; Python 3.13.17, NumPy 2.2.6
and Shapely 2.2.0 passed all 18 Python tests (36.005 seconds). No installer,
registry edit or system-Python replacement was used. Linux package availability
was verified in the Actions manifest; hosted CI must validate that platform on
the final candidate.

`go mod tidy` removes unused historical dependencies including
`mattn/go-sqlite3`, cloud clients and the old configuration dependency chain.
The compiled SQLite implementation remains pure Go. Transitive dependencies are
recorded in `go.sum` and `package-lock.json`; no blanket `go get -u` is used.

## Reproduce

From `apps/admin-region-tiler`, with `python` resolving to the exact version in
the root `.python-version` (check `python --version` first):

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
