# Release Handoff Prompt

你正在接手一个准备 GitHub 开源的 Map Tile Fetcher 仓库。当前仓库只保留一个可运行项目：

- `apps/admin-region-tiler`
  - Go 1.25+ Web 项目
  - 功能重点：范围框选下载、行政区划 GeoJSON 下载、多地图源、多任务/子任务、计划任务、SQLite 控制数据库、失败记录、产物下载、Docker/服务部署

旧 .NET 范围下载器已退休：

- 原目录：`apps/range-downloader`
- 原交接来源：`01-TianDiTuDownLoader_web`
- 迁移说明：`docs/range-migration.md`
- 不要重新把旧运行代码、旧发布包、截图、UI 设计包、tmp、templates 或运行数据混进仓库。

请按这个顺序接手：

1. 先阅读 `PROJECT_MANIFEST.md`，确认当前只包含 `apps/admin-region-tiler` 一个可运行项目。
2. 检查敏感信息：
   - 运行当前交接要求的敏感值扫描。
   - 同时参考 `docs/done-definition.md` 中的通用扫描规则。
   期望没有真实 token 或密码，只允许看到 `YOUR_TIANDITU_TOKEN`、`YOUR_MAPBOX_TOKEN` 这种占位符。
3. 验证 Go 项目：
   - `cd apps/admin-region-tiler`
   - 安装/使用 Go 1.25+
   - 执行 `go test ./...`
   - 前端脚本变更后执行 `node --check .\static\script.js`
4. 不要提交运行目录或构建目录：`.env`、`data/`、`output/`、`tiles/`、`bin/`、`obj/`、`publish*/`、历史二进制和压缩包都不应进 Git。
5. README 语言分工：
   - `README.md`: English
   - `README_ZH.md`: 中文
6. 每一次验证完成的变动都需要立即提交 git，且 commit body 必须有英文、中文和 Validation 三段。

当前仓库已完成的合并状态：

- Go 后端是唯一运行后端。
- 范围框选下载已迁移到 Go app 的 `mode: "bbox"` 任务。
- 行政区划下载保留在同一 Go app 中。
- SQLite 保留为控制数据库，只存任务、运行、任务源、产物和失败记录等元数据。
- 下载瓦片、MBTiles、ZIP 产物和运行日志走文件系统，不进入 Git。
