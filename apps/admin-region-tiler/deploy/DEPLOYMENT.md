# 生产部署说明

## 方案

- 单机部署
- Go 服务监听 `8081`
- Nginx 反向代理到 `127.0.0.1:8081`
- 数据库存放在 `data/tiler.db`
- 下载产物存放在 `output/`
- 下载区域文件存放在 `geojson/`

## Docker 部署

```bash
cp .env.example .env
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

4. 安装 `deploy/systemd/tiler.service`
5. 执行：

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

## 首次登录

- 用户名：`admin`
- 密码：`adminmap`

建议上线后第一时间修改默认密码。

`.env` 中可以覆盖：

- `HOST_PORT`
- `APP_PORT`
- `APP_DATABASE`
- `AUTH_DEFAULT_USERNAME`
- `AUTH_DEFAULT_PASSWORD`

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
- `删除` 会彻底删除：
  - `plans` 中的任务记录
  - `task_runs` 中的运行记录
  - 该任务对应的产物文件与输出目录
- `删除` 不会删除共享的 `geojson/` 区域文件
