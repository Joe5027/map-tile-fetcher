# Map Tile Fetcher

**中文**：一个自托管的地图瓦片下载 Web 工具，用 Go + SQLite + Leaflet 提供浏览器界面，支持框选范围、GeoJSON/行政区划、多地图源、任务进度和 ZIP/MBTiles 导出。

**English**: A self-hosted Go Web app for downloading authorized map tiles by bounding box or GeoJSON/admin regions, with task progress, failure records, retries, and ZIP/MBTiles exports.

> **合规提醒 / Compliance**：请只用于你有权访问和下载的地图瓦片服务。Use this tool only with map tile services that you are authorized to access and download. The project does not grant permission to bypass third-party terms, quotas, or access controls.

![Map Tile Fetcher dashboard](docs/assets/dashboard-overview.png)

- [中文说明](#中文说明)
- [English Guide](#english-guide)

## 中文说明

### 核心能力

- **框选范围下载**：在 Leaflet 地图上框选 bbox，设置 zoom 和图层后创建任务。
- **行政区划/GeoJSON 下载**：按内置区域目录或 GeoJSON 层级创建下载计划。
- **多地图源配置**：支持天地图、Mapbox、OSM、Google 样例和自定义瓦片 URL。
- **任务和产物管理**：独立任务页查看历史、进度、失败记录，并下载 ZIP/MBTiles。

### 适合谁

- 需要在内网、个人服务器或项目服务器上自托管瓦片下载工具的 GIS/地图开发者。
- 需要把授权地图源按 bbox、行政区划或 GeoJSON 范围批量转成离线包的人。
- 需要 ZIP 文件树或 MBTiles 产物，用于离线地图、测试数据或内部制图流程的团队。

### 三分钟启动

源码运行：

```powershell
git clone https://github.com/Joe5027/map-tile-fetcher.git
cd map-tile-fetcher\apps\admin-region-tiler
go run .
```

打开 `http://127.0.0.1:8081/`。

开发默认账号：

- 用户名：`admin`
- 密码：`adminmap`

生产部署前请复制 `.env.example` 为 `.env`，并修改默认账号和密码。

### Docker 镜像和部署

如果你要从源码构建镜像，需要 Dockerfile。本仓库已经提供：

- `apps/admin-region-tiler/Dockerfile`
- `apps/admin-region-tiler/docker-compose.yml`
- `apps/admin-region-tiler/.dockerignore`

推荐用 Docker Compose：

```powershell
cd apps\admin-region-tiler
Copy-Item .env.example .env
notepad .env
docker compose up --build -d
docker compose ps
```

打开 `http://127.0.0.1:8081/`。

手动构建并运行镜像：

```powershell
cd apps\admin-region-tiler
Copy-Item .env.example .env
docker build -t map-tile-fetcher:latest .
docker run --rm --name map-tile-fetcher `
  -p 8081:8081 `
  --env-file .env `
  -v "${PWD}\data:/app/data" `
  -v "${PWD}\output:/app/output" `
  -v "${PWD}\geojson:/app/geojson" `
  -v "${PWD}\conf.toml:/app/conf.toml:ro" `
  map-tile-fetcher:latest
```

Docker 配置项：

| 配置 | 默认值 | 说明 |
| --- | --- | --- |
| `HOST_PORT` | `8081` | 宿主机暴露端口，浏览器访问这个端口。 |
| `APP_PORT` | `8081` | 容器内应用监听端口，通常不用改。 |
| `APP_DATABASE` | `tiler.db` | SQLite 数据库文件名，保存在 `data/`。 |
| `AUTH_DEFAULT_USERNAME` | `admin` | 默认登录用户名，生产部署必须修改。 |
| `AUTH_DEFAULT_PASSWORD` | `adminmap` | 默认登录密码，生产部署必须修改。 |
| `TZ` | `Asia/Shanghai` | 容器时区。 |

Compose 会持久化 `data/`、`output/`、`geojson/` 和 `conf.toml`。如果端口被占用，把 `.env` 里的 `HOST_PORT=8081` 改成其他端口，例如 `HOST_PORT=18081`。

### 使用流程

![bbox task creation](docs/assets/bbox-task-creation.png)

1. 登录后进入 `新建计划` Tab。
2. 选择 `行政区划下载` 或 `范围框选下载`。
3. 输入你有权使用的服务 token，或选择不需要 token 的自定义地图源。
4. 设置线程、请求间隔、产物格式和执行时间。
5. 创建任务后自动进入 `我的任务` Tab。

![artifact download](docs/assets/artifact-download.png)

`我的任务` Tab 独立展示历史任务、运行状态、进度、失败数和产物入口。任务完成后可以下载 ZIP 文件树或 MBTiles；失败记录会持久化，便于调整并发、请求间隔或代理后重试。

### 配置地图源

示例配置在 `apps/admin-region-tiler/conf.toml`。

安全占位符：

- `YOUR_TIANDITU_TOKEN`
- `YOUR_MAPBOX_TOKEN`
- `YOUR_MAPBOX_SKU`

真实 token 只应保存在本地 `.env`、本地配置或部署平台的密钥管理中，不要提交到 Git。

### 发布和安装

- GitHub Release：[`v0.1.0`](https://github.com/Joe5027/map-tile-fetcher/releases/tag/v0.1.0)
- 发布说明：[`docs/releases/v0.1.0.md`](docs/releases/v0.1.0.md)
- 用户手册：[`docs/user-manual-zh.md`](docs/user-manual-zh.md)
- English manual: [`docs/user-manual.md`](docs/user-manual.md)

如果使用二进制发布包，解压后保持 `conf.toml`、`static/`、`geojson/` 和可执行文件在同一目录，再启动程序。

## English Guide

### What It Does

Map Tile Fetcher is a self-hosted Go Web app for downloading authorized map tiles and packaging them for offline use.

- **Bounding-box downloads**: draw a bbox on a Leaflet map, choose zoom levels and layers, then create a task.
- **GeoJSON/admin-region downloads**: create tasks from built-in region catalogs or GeoJSON files.
- **Configurable map sources**: Tianditu, Mapbox, OSM, Google examples, and custom tile URLs.
- **Task and artifact management**: view task history, progress, failures, retries, and download ZIP/MBTiles artifacts.

### Who It Is For

- GIS or map developers who need a self-hosted tile downloader.
- Teams that need authorized tiles exported by bbox, admin region, or GeoJSON area.
- Projects that need ZIP file trees or MBTiles artifacts for offline maps, internal workflows, or test data.

### Quick Start

Run from source:

```powershell
git clone https://github.com/Joe5027/map-tile-fetcher.git
cd map-tile-fetcher\apps\admin-region-tiler
go run .
```

Open `http://127.0.0.1:8081/`.

Development login:

- Username: `admin`
- Password: `adminmap`

Before production deployment, copy `.env.example` to `.env` and change the default credentials.

### Docker Image And Deployment

If you build the image from source, you need a Dockerfile. This repository already includes one, so users do not need to write it manually:

- `apps/admin-region-tiler/Dockerfile`
- `apps/admin-region-tiler/docker-compose.yml`
- `apps/admin-region-tiler/.dockerignore`

Recommended Docker Compose workflow:

```powershell
cd apps\admin-region-tiler
Copy-Item .env.example .env
notepad .env
docker compose up --build -d
docker compose ps
```

Open `http://127.0.0.1:8081/`.

Manual image build and run:

```powershell
cd apps\admin-region-tiler
Copy-Item .env.example .env
docker build -t map-tile-fetcher:latest .
docker run --rm --name map-tile-fetcher `
  -p 8081:8081 `
  --env-file .env `
  -v "${PWD}\data:/app/data" `
  -v "${PWD}\output:/app/output" `
  -v "${PWD}\geojson:/app/geojson" `
  -v "${PWD}\conf.toml:/app/conf.toml:ro" `
  map-tile-fetcher:latest
```

On Linux/macOS, use POSIX-style mounts:

```bash
-v "$PWD/data:/app/data"
-v "$PWD/output:/app/output"
-v "$PWD/geojson:/app/geojson"
-v "$PWD/conf.toml:/app/conf.toml:ro"
```

Docker environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `HOST_PORT` | `8081` | Host port exposed by Docker Compose. |
| `APP_PORT` | `8081` | App port inside the container. |
| `APP_DATABASE` | `tiler.db` | SQLite database file stored under `data/`. |
| `AUTH_DEFAULT_USERNAME` | `admin` | Default login username. Change it for production. |
| `AUTH_DEFAULT_PASSWORD` | `adminmap` | Default login password. Change it for production. |
| `TZ` | `Asia/Shanghai` | Container timezone. |

Docker Compose persists `data/`, `output/`, `geojson/`, and `conf.toml`. If port `8081` is already in use, change `HOST_PORT=8081` in `.env`, for example to `HOST_PORT=18081`.

### Usage Flow

1. Open the `Create Task` tab.
2. Choose `Admin Region` or `Bounding Box`.
3. Enter only tokens for services that you are authorized to use, or choose a custom source that does not need a token.
4. Set concurrency, request interval, artifact format, and schedule.
5. After task creation, switch to the `My Tasks` tab to track progress and download artifacts.

### Map Source Configuration

Example map-source configuration lives in `apps/admin-region-tiler/conf.toml`.

Safe placeholders:

- `YOUR_TIANDITU_TOKEN`
- `YOUR_MAPBOX_TOKEN`
- `YOUR_MAPBOX_SKU`

Real tokens should stay in local `.env`, local config, or your deployment secret manager. Do not commit real tokens to Git.

### Release And Install

- GitHub Release: [`v0.1.0`](https://github.com/Joe5027/map-tile-fetcher/releases/tag/v0.1.0)
- Release notes: [`docs/releases/v0.1.0.md`](docs/releases/v0.1.0.md)
- Chinese manual: [`docs/user-manual-zh.md`](docs/user-manual-zh.md)
- English manual: [`docs/user-manual.md`](docs/user-manual.md)

For binary release packages, keep the executable, `conf.toml`, `static/`, and `geojson/` in the same directory before starting the app.

### Developer Validation

```powershell
cd apps\admin-region-tiler
go test ./...
node --check .\static\script.js
node .\scripts\release_preflight.mjs
```

The release preflight runs Go tests, JavaScript checks, browser UI smoke tests, sensitive-value scanning, and tracked generated-file scanning.

## Repository Notes

The old .NET range downloader runtime has been retired. Its bbox workflow has been ported into the Go app under `apps/admin-region-tiler`; historical notes are kept in [`docs/range-migration.md`](docs/range-migration.md).

## License

Apache License 2.0. See [`LICENSE`](LICENSE).
