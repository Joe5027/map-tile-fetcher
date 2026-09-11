# 生产部署说明

## 方案

- 单机部署
- Go 服务监听 `8081`
- Nginx 反向代理到 `127.0.0.1:8081`
- 数据库存放在 `data/tiler.db`
- 下载产物存放在 `output/`
- 下载区域文件存放在 `geojson/`

## Docker 部署

首次启动前，复制 `.env.example` 为 `.env` 并在本地编辑
`AUTH_DEFAULT_USERNAME`、`AUTH_DEFAULT_PASSWORD`，保持登录启用。

```bash
cp .env.example .env
# 编辑 .env 中的初始账号后再启动
docker compose up -d --build
```

访问：

```text
http://<server-ip>:HOST_PORT
```

默认情况下 `HOST_PORT=8081`，容器内服务端口 `APP_PORT=8081`。如果服务器上 `8081` 已被占用，只需要修改 `.env` 中的 `HOST_PORT`。

## Linux 二进制部署

1. 编译或上传 `tiler` 二进制到 `/opt/tiler`
2. 上传 `conf.toml`、`static/`、`geojson/`
3. 创建目录：

```bash
mkdir -p /opt/tiler/data /opt/tiler/output
useradd -r -s /usr/sbin/nologin tiler
chown -R tiler:tiler /opt/tiler
```

4. 首次启动前，在本地 `conf.toml` 的 `[auth]` 中设置初始用户名和密码，并限制
   配置文件读取权限。二进制不会自动读取 `.env`；当前 systemd 示例也未配置
   `EnvironmentFile`，仅复制 `.env` 不会生效。
5. 安装 `deploy/systemd/tiler.service`
6. 执行：

```bash
systemctl daemon-reload
systemctl enable --now tiler
systemctl status tiler
```

## Nginx 反向代理

可直接使用 `deploy/nginx/tiler.conf`，放到：

```text
/etc/nginx/conf.d/tiler.conf
```

然后执行：

```bash
nginx -t && systemctl reload nginx
```

## 初始账号

- 用户名：`admin`
- 密码：`adminmap`

以上仅为开发默认值，部署时需在首次启动前替换。应用没有已实现的改密页面或接口，
修改 `AUTH_DEFAULT_PASSWORD` 不会重置数据库里已有同名用户的密码；不要通过删除
数据库来改密，以免丢失任务记录。已有默认账号的处置需单独安排。

Docker 的 `.env` 中可以覆盖以下配置；源码或二进制需使用进程环境变量或
`conf.toml`（`HOST_PORT` 仅用于 Docker 端口映射）：

- `HOST_PORT`
- `APP_PORT`
- `APP_DATABASE`
- `AUTH_DEFAULT_USERNAME`
- `AUTH_DEFAULT_PASSWORD`
- `TASK_MAX_TILES`
- `TASK_MAX_ACTIVE`
- `TASK_WORKERS`

## 数据持久化目录

以下目录都需要保留在服务器磁盘上：

- `data/`
  - 持久化用户、会话、计划任务、运行记录
- `output/`
  - 持久化下载结果和 ZIP/MBTiles 产物
- `geojson/`
  - 持久化区域配置文件，后续新增区域直接放到这里
- `conf.toml`
  - 运行配置

建议备份：

- `data/`
- `output/`
- `geojson/`
- `.env`
- `conf.toml`

## 新增区域文件

后续如果要增加新的下载区域：

1. 将新的 `.geojson` 文件放入服务器的 `geojson/` 目录
2. 不需要重建镜像
3. 不需要重启服务
4. 页面刷新后即可在“下载区域配置”中看到新文件

## 删除任务说明

- `取消` 只改变任务状态，不会删除记录
- `删除` 会清理任务记录、历史运行、产物、失败记录和关联元数据；父任务包括全部子任务。
- 运行、暂停或打包中的任务需要先取消并等待结束。清理失败后可重试。
- 内置区域文件和仍有其他任务引用的文件受保护；任务生成的区域文件在最后一个引用解除后清理。
- 升级前的备份、副本验证和历史核对方式见 [修复验收记录](../../../docs/repair-acceptance-2026-09-11.md)。

## 构建及账号维护入口

Windows/Linux 包统一使用 `scripts/build_release.py`，详见仓库
`docs/build-and-install.md`。可执行文件为 `tiler.exe` / `tiler`，与本目录 systemd 模板一致。
`tiler --version` 不初始化数据库；旧配置 `app.version` 不覆盖真实构建身份。
已有用户在账号菜单改密，忘记密码时先停止主服务及全部工作进程，再执行
`tiler admin reset-password --database /opt/tiler/data/tiler.db --username admin`。
隐藏终端输入、备份和维护锁规则见 `docs/password-recovery.md`。
