# Map Tile Fetcher

Map Tile Fetcher 是一个基于 Go 的 Web 应用，用于按地图框选范围或行政区划
GeoJSON 下载地图瓦片。

旧 .NET 范围下载器的核心流程已经迁移到 Go 应用中，旧运行代码已退休。历史迁移说明见
[`docs/range-migration.md`](docs/range-migration.md)。

## 仓库结构

- `apps/admin-region-tiler` - Go 1.25+ Web 应用，支持范围下载、行政区划下载、
  多地图源、子任务、计划任务、SQLite 控制状态、失败记录、Docker 部署和产物下载。
- `docs/merge-plan.md` - 当前合并后的清理和架构方向。
- `README.md` - 与本文对应的英文 README。

仓库不包含旧还原源码目录、UI 设计包、临时目录、截图、运行数据库、下载瓦片或历史发布包。

## 产品方向

- 后端：只保留 Go。
- 数据库：SQLite 作为轻量控制数据库，用于任务、运行记录、任务源、状态、可选登录会话、
  失败记录、产物索引和计划任务。
- 存储：下载瓦片、MBTiles、ZIP 产物、日志和运行数据放在文件系统中，不进入 Git。
- 前端：一个静态 Web 界面，提供两个模式：
  - 范围框选下载
  - 行政区划下载

## 快速验证

验证 Go 后端：

```powershell
cd apps/admin-region-tiler
go test ./...
```

前端脚本变更后验证：

```powershell
cd apps/admin-region-tiler
node --check .\static\script.js
```

运行浏览器 UI 冒烟测试，覆盖登录、行政区划和范围框选任务创建 payload：

```powershell
cd apps/admin-region-tiler
node .\scripts\smoke_ui.mjs
```

该脚本需要本地或全局可用的 Playwright 包。如缺失，可执行
`npm install -g playwright` 和 `npx playwright install chromium`。

运行完整发布预检：

```powershell
cd apps/admin-region-tiler
node .\scripts\release_preflight.mjs
```

## 本地运行注意事项

- 真实天地图或 Mapbox token 只能放在本地配置中。
- 登录默认启用。可信本地开发可设置 `AUTH_ENABLED=false` 免登录；部署环境应保持登录启用并覆盖默认用户名和密码。
- `.env`、`data/`、`output/`、`tiles/`、`bin/`、`obj/`、`publish*/` 和发布包不得进入 Git。
- `YOUR_TIANDITU_TOKEN`、`YOUR_MAPBOX_TOKEN` 等占位符可以保留在示例中。

## 提交规则

每个验证完成的变更批次必须立即提交。每次提交信息必须包含详细英文和中文说明，并列出
实际运行的验证命令。详见 [`docs/commit-policy.md`](docs/commit-policy.md)。

## 许可证

本仓库使用 Apache License 2.0。详见 [`LICENSE`](LICENSE)。
