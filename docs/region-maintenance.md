# 区域资源维护

两个工具的输入始终只读，不能直接更新部署目录。先检查，再显式生成独立资源包；
包内不含任务生成文件。输入不合格会报告失败，不会修补、简化或套用其他区域。

## 环境与命令

在 `apps/admin-region-tiler` 中使用 CPython 3.13 的独立环境：

```powershell
python -m venv .venv-region
.venv-region/Scripts/python -m pip install --require-hashes --only-binary=:all: -r scripts/requirements-region.lock
.venv-region/Scripts/python scripts/download_aliyun_geojson.py --root .
.venv-region/Scripts/python scripts/download_aliyun_geojson.py --root . --output-dir ../region-bundle --apply
.venv-region/Scripts/python scripts/materialize_geojson_from_mirror.py --project-root . --mirror-root /path/to/mirror
.venv-region/Scripts/python -B -m unittest discover -s scripts -p test_region_tools.py -v
```

Linux 将 `.venv-region/Scripts/python` 替换为 `.venv-region/bin/python`。
依赖锁定 Shapely 2.1.2、NumPy 2.2.6，包含 PyPI Windows/Linux amd64 wheel 的 SHA-256。
发布预检可设置 `REGION_PYTHON` 为该环境 Python 的绝对路径。

## 输入与结果

- 默认检查模式不访问网络、不创建输出、不写 Python 字节码。缺失或无效输入退出 `1`。
- `--apply` 才允许下载和写包；已有输出、输入与输出重叠、符号链接或 junction 均拒绝。
- `--deploy-root`、`--overwrite` 提前以 `2` 退出。没有清理原目录或部署功能。
- `--limit` 仅选取前 N 个区域，不能代表 dry run；输出目录只列本次处理的区域。
- 下载参数默认 `--workers 8 --timeout 30 --retries 3`，范围分别为 1～16、1～120 秒、1～5 次。
- 单文件/响应最多 32 MiB，只重试暂时网络故障、429、500、502、503、504；TLS 错误不重试。
- stdout 输出 JSON，诊断参数错误写 stderr。退出 `0` 表示选中项全部通过，`1` 为缺失/失败/取消，`2` 为参数错误。
- 包含 `geojson/regions.json`、区域文件、`manifest.json` 来源哈希与提取方法、`report.json` 差异及缺失报告。
- `INCOMPLETE` 在最先创建，只有全部写入并校验后才删除；存在该标记的目录不能投入使用。
  取消会停止排队工作，已开始的网络请求受超时约束后结束。失败目录留给维护者检查。

名称、编码、父级、全部匹配 Feature、闭合环、坐标范围及拓扑均校验。
城市 404 时只允许从父级原始数据精确提取目标编码，不能复制父级几何。
镜像合并必须覆盖目录列出的全部子区域，任何子区域缺失即失败。
工具要求来源采用 WGS84；不会执行未经确认的坐标系转换。
独立资源包仍需人工审核来源、范围和使用条件，代码交付不包含生产替换操作。
