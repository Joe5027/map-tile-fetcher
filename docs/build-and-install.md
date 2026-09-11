# 构建与安装

构建入口为 `apps/admin-region-tiler/scripts/build_release.py`，需要 Git、Go 1.25+、Python 3.13。
打包器只依赖 Python 标准库，不要求区域工具的 Shapely 环境。

## 构建身份

`tiler --version`、`GET /api/version`、帮助文本、启动日志和页面使用同一
`version`、`commit`、`builtAt`、`dirty`。版本查询不初始化数据库。
旧 `app.version` 配置仍可读取但不覆盖构建身份。

干净且精确匹配 `vX.Y.Z` 标签的打包构建使用标签；其他构建为 `dev+提交短号`。
显式 `--allow-dirty` 只用于开发验证，版本追加 `.dirty`，不允许伪装为标签构建。
普通 `go build` 使用 Go 内嵌的提交和脏状态；没有注入信息且没有 Git 元信息时返回
`unknown`，`dirty` 为 `null`。普通源码构建不会自行推断 Release 标签。
`builtAt` 使用源码提交时间，作为确定性构建时间戳，而不是每次打包的墙上时钟。

## Windows 与 Linux 包

在仓库根目录执行，`--ref` 必须明确指定提交或引用，输出目录必须不存在：

```powershell
python apps/admin-region-tiler/scripts/build_release.py --ref HEAD --output-dir dist/verification
python apps/admin-region-tiler/scripts/verify_package.py dist/verification/map-tile-fetcher-<version>-windows-amd64.zip
```

Linux 原生验证：

```bash
python apps/admin-region-tiler/scripts/verify_package.py dist/verification/map-tile-fetcher-<version>-linux-amd64.tar.gz
```

默认同时生成 Windows amd64 ZIP、Linux amd64 tar.gz，二进制分别为 `tiler.exe` 和 `tiler`。
`--target windows|linux` 可选择平台。`--metadata-json` 只打印构建身份，不生成文件。
不带 `--allow-dirty` 时工作区必须干净；正常构建从指定 Git 提交快照读取文件。
开发脏构建只读取 Git 已跟踪文件，新文件应先加入暂存区。

包包含配置示例、静态资源、区域资源、部署示例、许可证、手册、`build-info.json` 和
`manifest.json`。输出目录另有 `SHA256SUMS`，不包含真实 `.env`、数据库、下载、日志和测试。
固定源码和相同 Go/Python/zlib 工具链下，排序、归档时间和 `-trimpath` 保证可重复校验和。
`INCOMPLETE` 存在表示构建未完成，不作为可用安装包。脚本不推送、不建标签、不发布 Release。

解压后在包目录运行 `tiler.exe` 或 `./tiler`，保持 `conf.toml`、`static/`、`geojson/` 同级。
首次启动前设置进程环境 `AUTH_DEFAULT_USERNAME`、`AUTH_DEFAULT_PASSWORD`；二进制不自动加载 `.env`。
systemd 安装到 `/opt/tiler`，模板执行 `/opt/tiler/tiler -c /opt/tiler/conf.toml`。
`verify_package.py` 在全新临时目录核对清单、CLI/API 身份、登录、静态资源和本地模拟下载产物。

## Docker

通过同一打包脚本取得元信息，再传入 Docker：

```powershell
$info = python apps/admin-region-tiler/scripts/build_release.py --ref HEAD --metadata-json | ConvertFrom-Json
docker build --build-arg VERSION=$($info.version) --build-arg COMMIT=$($info.commit) --build-arg BUILT_AT=$($info.builtAt) --build-arg DIRTY=$($info.dirty.ToString().ToLower()) -t map-tile-fetcher:dev apps/admin-region-tiler
```

Docker 默认构建身份为 `unknown`，不凭空声明版本。CI 对比传入元信息与容器 `--version`。
生产安装、服务停机、数据恢复以及正式标签/Release 需另行安排，本轮不执行。
