# Project Manifest

本仓库只包含两个项目，并已按开源单仓库结构放在 `apps/` 下：

1. `apps/range-downloader`
   - 原交接目录：`01-TianDiTuDownLoader_web`
   - 范围：范围框选地图下载
   - 内容：.NET 6 Web 版天地图瓦片下载器，支持地图框选范围、三图层下载、任务状态和产物打包。

2. `apps/admin-region-tiler`
   - 原交接目录：`02-tiler-master`
   - 范围：行政区划地图下载
   - 内容：Go Web 服务版地图下载器，支持行政区划 GeoJSON、多地图源、多任务、计划任务、Docker 部署。

明确没有打包：

- `TianDiTuDownLoader_restored_source`
- `TianDiTuDownLoader_fixed_publish`
- `tianditu-ui-redesign-package`
- `ui-package`
- `tmp`
- `templates`
- 外层截图、设计草稿、旧发布包、运行日志、运行数据库、瓦片下载结果

已清理：

- `01-TianDiTuDownLoader_web` 排除了 `bin/`、`obj/`、`publish-linux-musl/`、`data/`。
- `02-tiler-master` 排除了 `.env`、`data/`、`output/`、`.tmp-region-data/`、历史二进制、历史压缩包和日志。
- 两个项目中的真实天地图/Mapbox token 已替换为空值或占位符。

当前状态：

- 这不是最终合并后的单项目，只是把两个后续需要合并的项目按 `apps/` 结构干净交接。
- 后续合并方案见 `docs/merge-plan.md`。
