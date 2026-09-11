# 七项维护验收记录

基线 `1109e4d`，分支 `codex/maintenance-fixes`。不操作生产数据、不创建标签或 Release。

| 项目 | 进度 | 验证依据 |
| --- | --- | --- |
| M1/M2/M7 | 本地及 Windows/Linux CI 通过 | `scripts/test_region_tools.py`，独立依赖锁定，输入只读/路径/404/拓扑/多 Feature/磁盘失败/取消/HTTP 边界 |
| M3 | 本地及 Linux CI 通过 | `password_test.go` 覆盖规则/会话撤销/并发/回滚/限流/维护锁/一致性备份；真实网页改密、重新登录、三个屏宽通过 |
| M4 | 调查完成，0 补齐、5 保留明确缺失 | 官方资料及原提供方 404 记录见区域核查；新增/过时缺口门禁、API 原因、页面禁用测试 |
| M5/M6 | 本地及双系统安装包 CI 通过 | 构建身份/标签/脏状态/无 Git、确定性归档、文件白名单；Windows 原生、Linux 隔离容器和 Docker 身份验证通过 |

M4 完整发布预检与 `go vet ./...` 通过，15 个 Python 用例包含当前目录核对、
新增缺口及过时例外反例。Go API 与浏览器验证缺失原因和禁用选项。

工具修复前：新增 `--output-dir` 默认只读契约在旧 CLI 上退出 2，测试失败；
修复后该契约与其余 12 项本地模拟用例通过。测试仅使用临时目录和本地 HTTP 服务。
工具用法及迁移说明见 [区域维护](region-maintenance.md)。

M3 首次针对性测试暴露 Windows SQLite 文件 URI 错误，修正盘符路径后通过。
完整 `node scripts/release_preflight.mjs`、`go vet ./...` 与交接校验通过；
浏览器密码弹窗截图在本地忽略的 `tmp/repair-ui/`，不进入 Git。
离线恢复测试使用临时数据库，确认备份仍可用原密码、目标使用新密码且任务内容保持不变。

M5/M6 验证：Go 1.25.3、Python 3.13.2，两次构建均生成相同 SHA-256。
开发验证包 `dev+4ed880765e7c.dirty`，Windows ZIP 为
`55fcf725b7a996bdee8fabd252f0dd6084e0bb1367e5a361a7e7da86a5961dcc`，Linux tar.gz 为
`95d063e51f654ba374bb036ab9572cea6b696ef600b0bb20764b2ca7cc00a441`。
这是提交前实现的验证记录；最终干净提交包由 CI 重新构建，不能将上述开发包当作正式 Release。
两个包各核对 3270 个清单文件，解压启动、登录、静态资源、版本一致性和 1 个本地瓦片产物通过。
Linux 执行权限及 systemd 路径通过，Docker 注入同样元信息后 `--version` 一致。
最终本地预检通过 18 个 Python 用例、完整 Go 套件、JavaScript 检查、三屏宽浏览器回归及敏感值/生成文件扫描。
`go vet ./...`、仓库审计、交接校验通过。Linux race 和干净源码包的双系统检查由最终 CI 验证。

## 干净提交验收

`6d4d088b21c56543915aae7a20852697ea84cc83` 的
[push CI](https://github.com/Joe5027/map-tile-fetcher/actions/runs/34571794614) 全部通过，
包括 Linux race/vet、完整预检、Windows/Linux 依赖安装和双系统原生包验证。
PR #6 同一提交的双系统包任务也已通过。后续仅交付记录更新仍须重新通过保护检查。

本地从该干净提交重建并原生验证 Windows/Linux 包，两个包均通过 3270 个文件清单及真实下载。
版本 `dev+6d4d088b21c5`，`dirty=false`；Windows ZIP SHA-256 为
`0654e5b1099b83f31f9c7e057012b950f2e9d3bd9c50d630992ca1d55c7c99a4`，Linux tar.gz 为
`ff702a938311019c205bede2f3e04d771f7b67293f094b55bc0bc097e567743a`。
所有测试服务均使用临时目录/本地 HTTP，测试进程退出；没有生产数据、标签或 Release 写入。
最终合并及合并后主分支 CI 由 [PR #6](https://github.com/Joe5027/map-tile-fetcher/pull/6) 和
GitHub Actions 记录，不能把旧的失败或被后续推送取消的运行算作通过。
