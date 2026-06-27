# 给另一台电脑的接手提示词

复制下面这段给另一台电脑上的 Codex/AI 助手：

```text
你正在接手一个准备 GitHub 开源、后续还要合并为一个项目的交接包。请严格只处理当前包里的两个子项目，不要把原机器外层目录里的旧源码、旧发布包、UI 设计包、tmp、templates、截图或运行数据混进仓库。

当前包结构：
- apps/range-downloader
  - 对应会话：019e8281-cf58-7bd1-916c-d8ccaa89c2ec，范围框选地图下载
  - .NET 6 Web 项目，原本来自天地图下载小程序的还原/改造版本
  - 功能重点：地图点击框选范围、输入天地图 token、按 img/cia/vec 三图层下载、任务状态、失败记录、产物 tar.gz
- apps/admin-region-tiler
  - 对应会话：019d7567-0bd8-7ab1-bd5d-5388b93519ad，行政区划地图下载
  - Go Web 项目
  - 功能重点：行政区划 GeoJSON、多地图源、多任务/子任务、计划任务、产物下载、Docker/服务部署

请按这个顺序接手：
1. 先阅读 `PROJECT_MANIFEST.md`，确认只包含这两个项目。
2. 检查敏感信息：
   运行敏感信息扫描，重点检查天地图、Mapbox、密码和 access token 相关字面值。
   期望没有真实 token 或密码，只允许看到 `YOUR_TIANDITU_TOKEN`、`YOUR_MAPBOX_TOKEN` 这种占位符。
3. 分别验证两个项目：
   - `cd apps/range-downloader`
   - 安装/使用 .NET 6 SDK，执行 `dotnet build .\TianDiTuDownLoader.Web.csproj -c Release`
   - 如需 Docker 镜像，先执行 README 里的 `dotnet publish ... -o .\publish-linux-musl`，再 `docker build`
   - `cd ..\admin-region-tiler`
   - 安装 Go 1.25+，执行 `go test ./...`
4. 开源前确认许可证文件，当前仓库使用 Apache License 2.0。
5. 不要提交运行目录或构建目录：`.env`、`data/`、`output/`、`tiles/`、`bin/`、`obj/`、`publish*/`、历史二进制和压缩包都不应进 Git。
6. 单仓库初始目录保持双项目结构：
   - `apps/range-downloader`
   - `apps/admin-region-tiler`
   - 根目录放统一 README、LICENSE、docs/merge-plan.md
7. 后续合并建议：
   - 先统一产品目标：范围框选下载 + 行政区划下载是否作为同一 Web 后台的两个模式。
   - 优先保留 `02-tiler-master` 的多任务、数据库、行政区划、部署能力。
   - 借鉴 `01-TianDiTuDownLoader_web` 的地图框选交互、范围预览和简洁下载流。
   - 合并前不要直接混拷代码，先画接口边界：地图源配置、区域选择、任务创建、任务状态、产物下载。

本包在原机器上的验证情况：
- `apps/admin-region-tiler`: `go test ./...` 已通过。
- `apps/range-downloader`: 使用 .NET SDK `6.0.428` 执行 `dotnet build .\TianDiTuDownLoader.Web.csproj -c Release` 已通过，0 警告 0 错误。
- 两个前端脚本已通过 `node --check` 语法检查。
```

## 快速说明

这个包是“两个项目的干净交接包”，不是最终合并版。请先把两个项目分别跑通，再决定统一仓库结构和合并路线。
