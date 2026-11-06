# Project Manifest

本仓库当前只保留一个可运行项目，并已完成旧范围下载器的 Go 迁移：

1. `apps/admin-region-tiler`
   - 原交接目录：`02-tiler-master`
   - 当前定位：统一 Go Web 地图瓦片下载器
   - 内容：行政区/矩形/多边形共用地图来源，支持 ZIP/MBTiles 与 Terrarium → Cesium 地形、异常续传、暂停排队、分页历史、SQLite 控制数据库、累计产物与 Docker/原生安装包。

已退休：

- `apps/range-downloader`
  - 原交接目录：`01-TianDiTuDownLoader_web`
  - 原内容：.NET 6 Web 版天地图范围下载器
  - 迁移状态：bbox 瓦片数学、天地图 `img`/`cia`/`vec` 下载源、范围任务创建、Leaflet 范围预览和双模式 UI 已迁移到 `apps/admin-region-tiler`
  - 历史说明：见 `docs/range-migration.md`

明确没有打包：

- `TianDiTuDownLoader_restored_source`
- `TianDiTuDownLoader_fixed_publish`
- `tianditu-ui-redesign-package`
- `ui-package`
- `tmp`
- `templates`
- 外层截图、设计草稿、旧发布包、运行日志、运行数据库、瓦片下载结果

已清理：

- 旧 .NET 运行代码已从 `apps/range-downloader` 移除。
- Go 项目排除了 `.env`、`data/`、`output/`、`.tmp-region-data/`、历史二进制、历史压缩包和日志。
- 真实天地图/Mapbox token 已替换为空值或占位符。

当前状态：

- 统一实现方向已落到 Go 应用中。
- 高程、恢复与性能升级状态及实际验证见 [升级记录](docs/upgrade-2026-11.md)。
- 正式脱敏文档图片位于 `docs/assets/`，由本地夹具生成；不引入运行截图或用户数据。
- 后续清理和演进见 `docs/merge-plan.md`。
