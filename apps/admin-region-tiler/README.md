# 地图下载器 Tiler - Map Tile Fetcher

这是 Map Tile Fetcher 的 Go Web 应用目录。应用提供登录、任务创建、任务历史、
失败记录、ZIP/MBTiles 产物下载、Docker 部署和发布预检脚本。

> 请只用于你有权访问和下载的地图瓦片服务。项目提供限速、重试和本地 token
> 配置能力，但不会授权绕过任何第三方服务条款、配额或访问限制。

## 功能概览

- 新建计划和我的任务使用独立 Tab，创建任务和查看历史互不拥挤。
- 支持 `行政区划下载` 和 `范围框选下载` 两种创建模式。
- 支持天地图、Mapbox、OSM、Google 样例和自定义瓦片 URL。
- 支持立即执行和单次定时执行。
- 支持 ZIP 文件树和 MBTiles 两种产物。
- 支持失败记录持久化、失败瓦片重试、暂停、恢复、取消和彻底删除。
- `geojson/` 可作为持久化区域资源目录，新增区域文件无需重建镜像。

## 本地源码启动

1. 安装 Go 1.25 或更高版本。
2. 在本目录执行：

```powershell
go run .
```

3. 浏览器打开 `http://127.0.0.1:8081/`。

开发默认账号：

- 用户名：`admin`
- 密码：`adminmap`

可信本地环境可设置 `AUTH_ENABLED=false` 免登录。生产部署应保持登录启用，并通过
`.env` 覆盖默认账号。

## Docker 镜像

### Dockerfile 是否必需

从源码构建镜像时需要 Dockerfile。本目录已经提供 `Dockerfile`，不需要用户自己写。
`docker compose up --build` 会自动读取它构建镜像。

同时提供 `.dockerignore`，用于排除 `.env`、`data/`、`output/`、发布包、数据库和日志，
避免把本地运行数据带进镜像构建上下文。

### Docker Compose

```powershell
Copy-Item .env.example .env
notepad .env
docker compose up --build -d
```

打开 `http://127.0.0.1:8081/`。

常用命令：

```powershell
docker compose ps
docker compose logs -f tiler
docker compose restart tiler
docker compose down
```

### 手动构建和运行

```powershell
Copy-Item .env.example .env
docker build -t map-tile-fetcher:latest .
```

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

Linux/macOS 使用同样参数时，把挂载路径写成 `"$PWD/data:/app/data"` 这种形式。

### Docker 配置

`.env.example` 提供这些默认值：

| 配置 | 默认值 | 说明 |
| --- | --- | --- |
| `TZ` | `Asia/Shanghai` | 容器时区。 |
| `HOST_PORT` | `8081` | 宿主机访问端口。 |
| `APP_PORT` | `8081` | 容器内应用监听端口。 |
| `APP_DATABASE` | `tiler.db` | SQLite 数据库文件名，保存在 `data/`。 |
| `AUTH_DEFAULT_USERNAME` | `admin` | 默认登录用户名。 |
| `AUTH_DEFAULT_PASSWORD` | `adminmap` | 默认登录密码。 |

生产部署至少要修改 `AUTH_DEFAULT_USERNAME` 和 `AUTH_DEFAULT_PASSWORD`。真实地图服务
token 不要写入 Git，可保存在本地配置、环境变量或部署平台密钥管理中。

## 配置地图源

地图源示例在 `conf.toml`。URL 中的瓦片坐标使用 `{x}`、`{y}`、`{z}` 占位：

```toml
url = "http://mt0.google.com/vt/lyrs=s&x={x}&y={y}&z={z}"
```

天地图示例：

```toml
url = "https://t0.tianditu.gov.cn/DataServer?T=img_w&x={x}&y={y}&l={z}&tk=YOUR_TIANDITU_TOKEN"
```

常见安全占位符：

- `YOUR_TIANDITU_TOKEN`
- `YOUR_MAPBOX_TOKEN`
- `YOUR_MAPBOX_SKU`

## UI 冒烟测试

浏览器测试脚本会自动启动临时端口的 Go 服务，登录默认开发账号，验证行政区划
和范围框选两种模式的任务创建 payload，并拦截 `/api/tasks` 提交以避免真实下载。

```powershell
node .\scripts\smoke_ui.mjs
```

如果服务已经在运行，可以指定地址：

```powershell
node .\scripts\smoke_ui.mjs --url http://127.0.0.1:8081
```

脚本需要本地或全局可用的 Playwright 包。如缺失，可执行：

```powershell
npm install -g playwright
npx playwright install chromium
```

## 发布预检

完整发布预检会依次运行 Go 测试、前端脚本语法检查、UI 冒烟脚本语法检查、
UI 冒烟测试、敏感值扫描和 tracked 生成物扫描：

```powershell
node .\scripts\release_preflight.mjs
```

## 生产部署参考

- Docker 部署：`Dockerfile`、`docker-compose.yml`、`.env.example`
- Linux 二进制部署：`deploy/DEPLOYMENT.md`
- Nginx 反向代理示例：`deploy/nginx/tiler.conf`
- systemd 服务示例：`deploy/systemd/tiler.service`

## 当前架构

新的内部包结构从 `internal/` 开始：

- `internal/api`：统一 HTTP API。
- `internal/auth`：可选登录和会话。
- `internal/config`：应用配置、地图源和输出路径。
- `internal/area`：bbox 和行政区划区域选择。
- `internal/planner`：任务定义、运行记录和子任务规划。
- `internal/downloader`：瓦片枚举、下载、重试和写入。
- `internal/artifact`：ZIP、MBTiles 和兼容产物。
- `internal/web`：统一静态前端辅助。

旧 `.NET` 范围下载器的框选交互和简单天地图流程已迁移到当前 Go 应用，旧运行代码已退役。
