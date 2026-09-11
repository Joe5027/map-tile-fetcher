# 全量修复验收记录

本次沿用 PR #3 与 `agent/audit-hardening`，保留 `e2cfcdb` 的密码升级、会话、
响应大小限制和单轮询器加固。长期分支统一为 `main`。本次交付仅涉及代码、
测试、配置示例和文档，没有执行生产部署或生产数据库修复。

## 10 类问题与验证

修复前基线为 `e2cfcdb`。`repair_contract_test.go` 使用基线已有接口，在临时
归档中原样运行，以下八项 Go 契约均在基线失败、在当前版本通过。前端另有两项
旧版复现。临时归档、数据库、模拟瓦片和截图均不进入 Git。

| 编号 | 基线实测失败 | 当前行为及主要回归 |
| --- | --- | --- |
| 1 产物覆盖 | 两次运行发布到相同文件名 | `TestRepairContractIssue1RunIsolation`；`TestSameNameArchivesNeverOverwrite`。实际路径包含父任务、子任务、运行 ID；显示名不决定路径。 |
| 2 注入与不受管链接 | `renderStandaloneTask` 接受 `javascript:` 下载地址 | `scripts/security_ui_test.mjs` 与浏览器恶意名称检查。动态文本转义，下载地址限定为对应任务的受管入口。 |
| 3 重试与完整产物 | 最新运行失败后，原可用产物下载返回 409 | `TestRepairContractIssue3LastPublishedDownload`；`TestRetryPublishesCompleteCumulativeArtifact`；真实 API ZIP/MBTiles 多轮重试。四坐标先成功三份，再只补一个失败坐标，最终输出包含全部四个坐标。 |
| 4 删除不完整或不安全 | 父任务删除后仍有子任务输出目录 | `TestRepairContractIssue4GroupDeletion`；共享文件、符号链接、清理中断、重复删除及子清理清单测试。删除记录和生成文件，保留内置及其他任务引用资源。 |
| 5 创建中途失败 | 数据库创建失败时删除请求引用的已有区域文件 | `TestRepairContractIssue5CreationFileOwnership`；`TestSecondChildFailureRollsBackAPIAndGeneratedFile`。父子与规范化记录同事务；失败仅清理本请求新生成文件。 |
| 6 资源无上限 | 配置父任务上限 6，仍接受两图层合计 8 个瓦片 | `TestRepairContractIssue6ParentBudget`；大 bbox 描述、取消枚举、持久队列及真实四子任务测试。默认总预算 100 万，最多同时运行 3 个子任务。 |
| 7 强制线程数 | 请求 1 个线程，实际为 20 | `TestRepairContractIssue7RequestedWorkers`；1/3/20/50 与来源上限组合测试。默认 3，可选 1～50，实际值取来源上限和用户设置较小值。 |
| 8 预览凭证过期 | 注册响应没有到期时间 | `TestRepairContractIssue8PreviewExpiry`；浏览器推进 16 分钟、合并注册、单次失效恢复、账号缓存清理。提前 60 秒续期。 |
| 9 区域预览与切换 | 区域响应缺少层级列表和所选层级 | `TestRepairContractIssue9AreaLevels`；完整多 Feature、层级切换、倒序响应测试。独立预览覆盖层，不修改新建任务选区。 |
| 10 移动端与轮询 | 390px 视口被旧样式撑成 1080px 页面 | `scripts/repair_ui_checks.mjs`。390/768/1440px 的创建和任务页无整体横向溢出，菜单在视口内；401 停止轮询，真实重新登录后只有一个轮询器。 |

额外实测修复：连接池新连接未设置 SQLite 等待时间导致恢复接口报锁错误；
父任务错误汇总部分失败；产物提交前提前显示完成；清理单个旧产物时误删另一个
任务文件树中的文件；父任务删除遗留此前中断的子清理记录。这些均有新增回归。

## 真实集成范围

`TestAPILifecycle` 编译并启动真实应用二进制，通过 HTTP 调用真实调度器，
调度器启动正常独立工作进程；数据库与输出均在临时目录，瓦片服务仅使用本地
HTTP 模拟服务。覆盖：

- 四个子任务中三个执行、一个排队；暂停仍占槽，恢复后完成。
- 活跃或暂停任务拒绝删除；取消后删除；父任务级联清理全部关联表。
- ZIP 和 MBTiles 的部分失败、重复失败、多轮重试、完整产物下载及无失败再次重试。
- 首次全部失败且没有历史成功瓦片时，可从失败记录重试。
- 历史成功输出缺失时返回 `409 baseline_missing`，显式重新创建后完成。
- 定时排队任务经真实服务停止和重新启动后继续执行。
- 显式历史核对返回 202；不通过任务列表 GET 扫描或修复数据。

`retry_integration_test.go` 另检查最终 ZIP/MBTiles 坐标集合、只请求失败坐标、
旧产物路径保留、失败解决运行、发布事务中断和启动恢复。`recovery_edges_test.go`
覆盖旧库副本重复迁移、发布前取消、取消准备/枚举及预期坐标以外的历史失败。
单元测试和模拟服务不代表真实地图服务的可用性或生产数据可恢复性。

## 配置与接口约定

| 配置 | 默认 | 说明 |
| --- | --- | --- |
| `task.max_tiles` / `TASK_MAX_TILES` | 1000000 | 每个父任务所有图层累计；旧任务重执行也检查。区域按包围范围保守计数。 |
| `task.max_active` / `TASK_MAX_ACTIVE` | 3 | 运行中的子任务槽；暂停继续占用，结束或取消释放。 |
| `task.workers` / `TASK_WORKERS` | 3 | 用户可选 1～50；来源 `worker_count` 是上限。 |
| `source_policies.base_delay_ms` | 按来源 | 实际间隔取来源最低间隔和用户设置较大值，另可叠加配置抖动。 |

- `GET /api/config/limits` 返回实际配置，页面从接口读取。
- 创建超预算返回 400，含 `code=tile_budget_exceeded`、`estimatedTiles`、`limit`
  和 `conservative`；接受排队仍返回 201 与任务记录。
- `queue` 提供状态、到期时间和排队位置；子任务响应含实际线程和间隔。
- `progress` 等原字段表示本次运行，`integrity` 表示累计完整度。
- 失败列表默认仅未解决事件；`?history=true` 包括原始历史及解决时间、解决运行。
  汇总数按子任务、层级、坐标及来源地址去重。
- `POST /api/tasks/:id/retry-failures` 接受后返回 202；缺失历史成功基线返回 409。
- `POST /api/tasks/:id/reconcile` 返回 202，任务查询可读核对状态；
  `POST /api/tasks/:id/recreate` 创建新的完整任务，需要用户主动操作。
- 下载地址不变，始终选择最近已成功发布产物。失败、取消、空间不足不切换入口。
- `DELETE /api/tasks/:id` 仍为取消；`DELETE /api/tasks/:id/purge` 为删除记录及生成文件。

## 迁移与历史数据

结构迁移通过现有 `initSchema` 增量执行，可重复运行。新增失败解决列与索引，
以及完整度、覆盖坐标、待发布记录、持久队列和清理进度表。结构失败时启动退出，
不会继续启动调度。已用旧失败表结构的 SQLite 副本验证两次迁移，原库保持不变。

部署时应先备份控制数据库与原输出目录，并在隔离副本上验证新版本。活跃 SQLite
应使用一致性备份接口或停写后复制，不能仅复制可能仍有 WAL 写入的主文件。
副本校验应使用隔离端口、隔离输出目录，检查迁移错误和任务可读性；不要把副本
中的计划任务指向真实地图服务。以上是部署操作说明，本次没有执行这些生产操作。

历史数据采用按需核对：从现存文件树、ZIP 或 MBTiles 校验预期坐标与瓦片内容；
仅能证明完整时标为完整。核对不下载瓦片，也不删除原事件或旧产物。服务启动只
恢复未完成发布记录；中断的下载标为中断失败，原有待执行队列保留。

失败重试先复制可用基线到新运行目录，再补失败坐标。复制前要求可用空间覆盖
基线大小并预留 1 GiB；这不代表整个下载/打包必然有足够空间，运行中写盘错误仍
会终止本次运行并保留旧入口。若旧成功瓦片已丢失且无法证明完整基线，不能把少量
重试文件当成完整结果；使用“重新创建完整任务”显式重新下载。

## 重现命令与交付门槛

在 `apps/admin-region-tiler` 执行：

```powershell
go test ./...
go vet ./...
node scripts/release_preflight.mjs
```

发布预检包括真实 Go 集成、JS 语法、渲染安全、浏览器回归、敏感值和生成文件扫描。
Linux CI 另执行 `go test -race ./...`。本机 Windows 没有 C 编译器，race 由 Linux CI
提供；已成功运行的修复批次可在 PR #3 的 Checks 中追溯。

旧版证据重现：将 `e2cfcdb` 归档到临时目录，把当前 `repair_contract_test.go` 原样
放到该副本应用目录，执行 `go test -run TestRepairContract -v .`，应有八项预期失败。
当前目录执行 `node scripts/security_ui_test.mjs e2cfcdb` 应拒绝旧版脚本下载 URL；
设置 `TILER_REPAIR_BASELINE=e2cfcdb` 运行发布预检，会额外检查旧 CSS 的 1080px 溢出。
历史对照需完整 Git 历史；常规 CI 不需要此环境变量。

PR #3 必须在最新提交的 `Commit Message` 与 `Admin Region Tiler` 全绿后合并，
保留严格分支保护和双语合并正文。合并后再次等待 `main` CI 全绿，才删除修复分支。
旧 `main` 缺失双语正文造成的历史 CI 失败不通过改写历史掩盖。
