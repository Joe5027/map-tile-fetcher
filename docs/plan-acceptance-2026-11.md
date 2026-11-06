# 升级计划覆盖核对

本表按已接受的实施方案核对实现、执行证据与交付门槛。既有测试全部通过，
不等于新增需求全部验收；最终核查补齐了地形估算、备用来源、准备进度、
单遍基线复制、SQLite 空间计算及真实多尺寸业务操作的缺口。

## 功能和验证对应

下列测试路径相对 `apps/admin-region-tiler`。具体运行结果和性能值见
[升级证据](upgrade-2026-11.md)，命令入口见[验证链路](validation-chain.md)。

| 方案要求 | 实现与可执行证据 |
| --- | --- |
| 行政区、矩形、多边形实际参数及来源约束 | `static/script.js` 共用参数/来源；`smoke_ui.mjs` 校验非默认参数；`workflow_acceptance.mjs` 的三模式真实 POST/worker 链路 |
| WebP、零重试、自定义状态码、连接复用和来源关闭复用 | `download_policy_regression_test.go`：四项旧实现失败后通过；连接数另有固定夹具计数 |
| 创建摘要、防重复点击、永久删除确认 | browser smoke/repair；创建不弹确认框，提交中只发一次 POST；真实 workflow 的永久删除取消发 0 次 purge，确认发 1 次后任务 404/卡片消失 |
| 阶段、归属、已提交结果持久化 | `run_phase.go`、`recovery_runtime.go`；原 run 复用、暂停意图、控制记录迁移回归 |
| 服务退出后续传、暂停不自动运行、释放执行槽 | `recovery_process_test.go`：ZIP、MBTiles、暂停、队列容量和重启；断言已保存坐标请求数不增加 |
| 不存在同库重复调度或旧 worker 发布覆盖 | server 独占锁、worker run 锁和 owner token；真实第二服务拒绝和旧 worker 释放锁回归 |
| 下载、转换、打包、发布恢复及旧产物保留 | 前三阶段真实终止服务；`terrain_recovery_process_test.go` 检查已生成网格修改时间；发布使用事务 abort、缺失/损坏 ZIP 故障与恢复测试，不混称发布瞬间真实 kill |
| bbox 流式计数、行政区去重目录复用 | `enumeration.go`；`acceptance_geometry_test.go` 使用真实狭长、多岛、重叠 GeoJSON；估计超限但实际未超限可执行，真实超额仍拒绝 |
| 准备阶段估计/实际反馈及父累计预算 | `acceptance_progress_test.go`：200ms 节流、准确最终数、缓存复用；父预算幂等累计测试 |
| 重试及时入队、无全局锁内基线扫描 | `TestLargeBaselineRetryQueuesWithoutReadingOrBlockingOtherTask`：65,535 瓦片基线持独占锁仍可入队并操作另一任务 |
| 基线只读取校验一次 | `prepareRetryBaseline` 同一遍读取、验证、复制；`acceptance_baseline_test.go` 对 512 个坐标计数恰好 512 次读取，缺失项须有失败记录 |
| MBTiles 单 writer、有界批事务、提交后成功、无普通 VACUUM | `mbtiles_batch.go`；256 条/8 MiB/250ms 边界、单大瓦片不拒绝、事务回滚/TMS、I/O 失败、真实 MBTiles 续传及事务微基准 |
| 磁盘按复制/转换/打包需求判断 | 有效 SQLite 页含索引、扣空闲页；区分首次复制与已有目标续传的 journal 边界；raw/ZIP 恢复扣已有目标、保留单文件原子替换峰值；实际复制测量及真实可用空间的 exact-fit/少 1 字节检查。没有填满用户磁盘 |
| 原图像及矢量不回退 | 图像下载/格式测试；`TestVectorDownloadPreservesProtobufBytes` 检查 ZIP/MBTiles 矢量载荷逐字节一致 |
| 50 父分页、状态统计、兼容旧数组、消除 N+1 | `task_page_test.go`：1,000 父 × 3 子、固定 8 次批查询、旧响应等价；全局统计仍扫描紧凑状态行，未宣称全部计算仅与页大小有关 |
| 可见时轮询、隐藏停止、返回刷新、局部更新保交互 | smoke/repair 检查焦点、选择、菜单和展开；真实 workflow 在三宽度实际滚动容器中验证后台刷新不改变位置和选择 |
| 授权清理节流、阶段与继续/重试/改源入口 | grant 到期/每分钟清理测试；阶段显示和 task actions browser checks；真实重试只补缺失请求 |
| 地形图片、矢量和真实高程明确区分 | 正常及备用来源目录包含 ter/cta/Terrarium；来源 GET 失败的真实页面验收，现有 Token 配置路径保留 |
| Terrarium 默认 14、可选 15，估算及进度含祖先/halo | `terrain_estimate_checks.mjs` 90 组对照及 z15 公式；实际 DOM 的 bbox 1～2°/z2 为 14 原始请求、3 网格；后台精确目录和预算测试 |
| 65×65 全球采样、Mercator/TMS/单根/祖先/二进制正确 | `internal/terrain` 独立解码断言已知、负、零高程，三角、边缘、量化、日期变更线、包围信息、缺父和 gzip |
| 缺片不补零、父级回退、补齐与转换复用 | terrain worker 累计重试、首次缺根、透明数据、真实转换中断；真实浏览器在仅 1/5 网格时检验缺子坐标 100m 及 Cesium 上采样网格 100m，随后单请求补齐；有效 raw/mesh 复用 |
| layer.json、terrain、署名、服务示例、离线预览和 ZIP | 真实 worker ZIP/认证路由/HTTP gzip；包内 README 含 Nginx 与 Cesium 示例；本地 Cesium 四视角/独立高度采样，外网请求为 0 |
| 导出来源不带凭据 | 合法无名称、带 userinfo/query/fragment 的来源请求先复现泄露再修复；下载 URL 原样保留，导出名称和 URL 清理 |
| 旧数据库副本迁移及旧产物继续下载 | `TestLegacyDatabaseCopyMigration` 同时保留失败历史及 ready ZIP；原库未变，副本幂等迁移后经真实下载 handler 读取相同字节 |
| 三尺寸真实创建/暂停/恢复/重试/预览/下载 | `workflow_acceptance.mjs`：390/768/1440 × 行政区/矩形/多边形共 9 链；新预览窗口显式采用并断言对应宽度；真实 API/SQLite/worker，下载 ZIP 独立验证 CRC、5 网格、4,225 顶点和 100m 高程 |
| 预览及验收效率 | 按需绘制、设色显式重绘；真实帧/高度/交互验收和同工具链 A/B；分支只跑 PR 检查，保留 main 检查，npm 缓存与 headless-only 安装避免重复工作 |
| 首瓦片、总时、事务、连接、内存和临时盘证据 | 原下载引擎夹具保留；新增 `benchmark_workflow.py` 测真实 API 到发布，包括 ZIP、OS 进程树 RSS、完整运行目录及 SQLite 临时目录；采样局限明确记载，不把微基准当整任务提速 |
| 依赖最新稳定审查与合理升级/保留、锁定统一 | [依赖决策](dependencies-2026-11.md) 对应 go/npm/Python 锁文件与 Go/Node/Python/Docker/Actions 固定版本；Python 保留 3.13 ABI，选两平台共同可用补丁，不替换全局运行时 |
| README、双语手册、配置/API、当前截图 | 根及应用 README、手册、terrain/build 文档；四张本地脱敏图、可复现 capture 命令及前端源码哈希 |
| AI 规则、技能、地图、图谱、记忆、验证和自动化规范 | `AGENTS.md` 为规则唯一入口，local skill 引用它；project-map、knowledge-graph、五段 long-term-memory、portable check_docs 和只读 automation 模板；没有创建定时任务 |

## 推送与合并后必须完成的检查

这些操作依赖远端候选或最终提交，不能在尚未推送时伪称通过：

1. 精确候选通过现有 Validate：Go/浏览器/Python/文档预检、Linux race/vet、
   Windows/Linux 两次一致安装包及原生验证、Docker 身份验证。
2. 用经过本地 `--file` 校验的实际双语说明走受保护 PR；核对合并后的代码树。
3. 核对最终 main 的实际工作流，通过后同步本地并删除已整合的非 main 分支；
   再读取真实远端 heads，不能只看缓存引用。
4. 最终交付报告给出真实 main SHA、CI 和本地/远端分支结果。历史合并说明
   `f0a2865` 的条目不足已记录，不以改写历史或放宽校验掩盖。

这些规则见[合并流程](merge-plan.md)。源码内保留可重复核验的方法和链接；
临时分支列表与流水线状态以实时 Git/CI 为准。

## 已接受的范围边界

验收对象是服务异常退出后的恢复，不是独立 worker 崩溃的无限自动重启。
不包括操作系统断电、生产部署、替换用户数据库、广域真实下载、工程高程基准
转换、GeoTIFF 或云端发布。地形按固定网格采样并量化，既有区域权威边界缺口
保持明确记录。性能采样和合成地形测试不被描述为所有机器或真实测绘精度保证。
