# 七项维护验收记录

基线 `1109e4d`，分支 `codex/maintenance-fixes`。不操作生产数据、不创建标签或 Release。

| 项目 | 进度 | 验证依据 |
| --- | --- | --- |
| M1/M2/M7 | 本地工具修复完成，等待 CI | `scripts/test_region_tools.py`，独立依赖锁定，输入只读/路径/404/拓扑/多 Feature/磁盘失败/取消/HTTP 边界 |
| M3 | 本地验收通过，等待 CI | `password_test.go` 覆盖规则/会话撤销/并发/回滚/限流/维护锁/一致性备份；真实网页改密、重新登录、三个屏宽通过 |
| M4 | 待核实 | 五个区域逐项来源调查及缺口校验 |
| M5/M6 | 待实现 | 统一构建身份、确定性 Windows/Linux 安装包与启动验证 |

工具修复前：新增 `--output-dir` 默认只读契约在旧 CLI 上退出 2，测试失败；
修复后该契约与其余 12 项本地模拟用例通过。测试仅使用临时目录和本地 HTTP 服务。
工具用法及迁移说明见 [区域维护](region-maintenance.md)。

M3 首次针对性测试暴露 Windows SQLite 文件 URI 错误，修正盘符路径后通过。
完整 `node scripts/release_preflight.mjs`、`go vet ./...` 与交接校验通过；
浏览器密码弹窗截图在本地忽略的 `tmp/repair-ui/`，不进入 Git。
离线恢复测试使用临时数据库，确认备份仍可用原密码、目标使用新密码且任务内容保持不变。
