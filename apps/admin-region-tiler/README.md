
# 地图下载器 Tiler - map tiles downloader

A well-polished tile downloader

一个极速地图下载框架，支持谷歌、百度、高德、天地图、Mapbox、OSM、四维、易图通等。

- 支持多任务多线程配置，可任意设置

- 支持不同层级设置不同下载范围，以加速下载

- 支持轮廓精准下载，支持轮廓裁剪

- 支持矢量瓦片数据下载

- 支持文件和MBTILES两种存储方式

- 支持自定义瓦片地址

## 使用方式

1. 下载源代码在对应的平台上自己编译

2. 直接release发布页面, 下载对应平台的预编译程序

参照配置文件中的示例url更改为想要下载的地图地址，即可启动下载任务~
> 例如: url = "http://mt0.google.com/vt/lyrs=s&x={x}&y={y}&z={z}" ,地址中瓦片的xyz使用{x}{y}{z}代替，其他保持不变。

## 谷歌地图说明
- 影像层
  谷歌影像，分有偏移和无偏移两种，下载国内有偏移的影像需要在连接中加地区字段，如下为大陆地区偏移影像
  > url = "http://mt0.google.com/vt/lyrs=s&gl=CN&x={x}&y={y}&z={z}"
- 标注层
  影像标注，中文标注只有火星坐标，谷歌并不提供无偏移标注图层，所以通常只能下载有偏移的标注层，如下为大陆地区偏移标注
  > url = "http://mt0.google.com/vt/lyrs=h&gl=CN&x={x}&y={y}&z={z}"
- 使用
  在实际的使用中，要么保持系统的无偏移（这个时候需要校准有偏移的标注层），要么保持影像和标注的都有偏移，使用火星算法处理自己的数据

#### 谷歌图层类型lyrs=
- h 街道图，透明街道+标注
- m 街道图
- p 街道图
- r 街道图
- s 影像无标注
- t 地形图
- y 影像含标注


## 天地图说明
- 天地图影像,img_w
  > url = "https://t0.tianditu.gov.cn/DataServer?T=img_w&x={x}&y={y}&l={z}&tk=YOUR_TIANDITU_TOKEN"
- 影像标注层,cia_w
  > url = "https://t0.tianditu.gov.cn/DataServer?T=cia_w&x={x}&y={y}&l={z}&tk=YOUR_TIANDITU_TOKEN"

- 天地图矢量(地形图),vec_w
  > url = "https://t0.tianditu.gov.cn/DataServer?T=vec_w&x={x}&y={y}&l={z}&tk=YOUR_TIANDITU_TOKEN"
- 矢量标注层,cva_w
  > url = "https://t0.tianditu.gov.cn/DataServer?T=cva_w&x={x}&y={y}&l={z}&tk=YOUR_TIANDITU_TOKEN"

> 工具已经处理了天地图429限制，请合理使用！！！

## 本地启动

1. 安装 Go 1.25 或更高版本
2. 在项目根目录执行 `go run .`
3. 浏览器打开 `http://127.0.0.1:8081/`

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

脚本需要本地或全局可用的 Playwright 包。如缺失，可执行
`npm install -g playwright` 和 `npx playwright install chromium`。

完整发布预检会依次运行 Go 测试、前端脚本语法检查、UI 冒烟脚本语法检查和
UI 冒烟测试：

```powershell
node .\scripts\release_preflight.mjs
```

## 生产部署

- Docker 部署可直接使用 `Dockerfile` 和 `docker-compose.yml`
- Linux 二进制部署可参考 `deploy/DEPLOYMENT.md`
- Nginx 反向代理示例在 `deploy/nginx/tiler.conf`
- systemd 服务示例在 `deploy/systemd/tiler.service`

## 当前版本说明

- Web 界面已支持登录、计划任务创建、任务进度展示和产物下载
- 默认登录账号为 `admin`，默认密码为 `adminmap`；生产部署请通过 `.env` 修改
- 新建任务接口支持 `立即执行` 和 `单次定时执行`
- 任务完成后：
  - `file` 输出会自动打包为 ZIP，供用户下载
  - `mbtiles` 输出可直接作为下载产物
- 用户任务、运行记录和会话已持久化到 `data/tiler.db`
- 任务支持 `取消` 和 `彻底删除`
- `geojson/` 目录已作为可持久化区域资源目录，后续新增区域文件无需重建镜像
- GeoJSON 读取与任务创建失败会返回 API 错误，不再直接退出服务
- Docker Compose 支持通过 `.env` 覆盖 `HOST_PORT`、`APP_PORT`、`APP_DATABASE` 和默认登录账号
- 当前页面仍使用 Tailwind CDN，适合本地开发和联调

## 合并方向

本应用是后续统一产品的 Go 后端基座。新的内部包结构从 `internal/` 开始：

- `internal/api`: 统一 HTTP API
- `internal/auth`: 可选登录和会话
- `internal/config`: 应用配置、地图源和输出路径
- `internal/area`: bbox 和行政区划区域选择
- `internal/planner`: 任务定义、运行记录和子任务规划
- `internal/downloader`: 瓦片枚举、下载、重试和写入
- `internal/artifact`: ZIP、MBTiles 和兼容产物
- `internal/web`: 统一静态前端辅助

旧 `.NET` 范围下载器的框选交互和简单天地图流程已迁移到当前 Go 应用，旧运行代码已退役。
