# Sub2API Self-Heal Watch

自动巡检 Sub2API 容器状态、HTTP endpoints 和日志错误模式，发现异常时自动重启服务。

## 功能

- 检查 Docker 容器状态（running / exited / unhealthy）
- 检查 HTTP endpoints（`/`, `/health`, `/v1/models`）
- 扫描最近 100 行日志中的错误模式（账号耗尽、代理连接失败、OAuth 超时等）
- 检查容器内 `HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY`/`UPDATE_PROXY_URL` 环境变量是否指向正确的 NAS 代理地址
- 发现异常时自动重启 `docker compose restart`
- 重启后再次验证 `/health` 是否恢复 200

## 错误模式列表

检测以下日志关键字，任一出现即触发自愈：

| 模式 | 含义 |
|---|---|
| `no available accounts` | 无可用账号 |
| `unsupported_country_region_territory` | 账号地区不支持 |
| `account_select_failed` | 账号选择失败 |
| `token_refresh.retry_attempt_failed` | Token 刷新重试失败 |
| `OAuth refresh attempt timed out` | OAuth 刷新超时 |
| `account OAuth credentials permanently rejected` | OAuth 凭证永久拒绝 |
| `proxyconnect tcp: dial tcp 127.0.0.1:7890` | 代理地址错误（指向了容器内 localhost 而非 NAS IP） |

## 快速开始

### 前置要求

- 已部署 Sub2API（Docker Compose）
- 可通过 SSH 访问运行 Sub2API 的 NAS/服务器
- SSH key 已配置好免密码登录

### 参数说明

| 参数 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `-h HOST` | `SELF_HEAL_HOST` | `192.168.10.20` | NAS SSH 地址 |
| `-u USER` | `SELF_HEAL_USER` | `l890852` | SSH 用户名 |
| `-k KEY` | `SELF_HEAL_KEY` | `~/.ssh/id_ed25519_nas` | SSH 私钥路径 |
| `-d DIR` | `SELF_HEAL_COMPOSE_DIR` | `/volume1/docker/sub2api` | docker-compose.yml 所在目录 |
| `-p PORT` | `SELF_HEAL_PORT` | `3014` | Sub2API Web 端口 |
| `-P URL` | `SELF_HEAL_PROXY_URL` | *(空)* | HTTP 代理地址（curl -x 参数） |
| `-i SEC` | `SELF_HEAL_INTERVAL` | `3600` | 巡检间隔（秒），仅循环模式生效 |
| `--no-restart` | — | false | 仅告警，不自动重启 |
| `--dry-run` | — | false | 打印操作但不执行 |

### 示例

```bash
# 单次检查（适用于 cron）
SELF_HEAL_ONCE=true SELF_HEAL_HOST=192.168.10.20 \
  SELF_HEAL_COMPOSE_DIR=/volume1/docker/sub2api \
  SELF_HEAL_PROXY_URL=http://192.168.10.20:7890 \
  ./self-heal-watch.sh

# 持续循环（适用于 systemd service 或 screen）
./self-heal-watch.sh -h 192.168.10.20 -u l890852 \
  -d /volume1/docker/sub2api -P http://192.168.10.20:7890 -i 3600

# 仅告警，不重启（集成到自己的告警系统时）
./self-heal-watch.sh --no-restart
```

### 配合 Cron 使用（推荐）

```crontab
# 每小时巡检一次，发现问题自动修复
0 * * * * SELF_HEAL_ONCE=true SELF_HEAL_HOST=192.168.10.20 SELF_HEAL_COMPOSE_DIR=/volume1/docker/sub2api SELF_HEAL_PROXY_URL=http://192.168.10.20:7890 /path/to/self-heal-watch.sh >> /var/log/sub2api-self-heal.log 2>&1
```

### 配合 Systemd 使用

```ini
# /etc/systemd/system/sub2api-self-heal.service
[Unit]
Description=Sub2API Self-Heal Watch
After=network-online.target

[Service]
Type=simple
User=youruser
Environment="SELF_HEAL_HOST=192.168.10.20"
Environment="SELF_HEAL_COMPOSE_DIR=/volume1/docker/sub2api"
Environment="SELF_HEAL_PROXY_URL=http://192.168.10.20:7890"
ExecStart=/opt/sub2api/self-heal-watch.sh
Restart=on-failure
RestartSec=30

[Install]
WantedBy=multi-user.target
```

## 关键注意事项

### 代理地址必须用 NAS IP，不能用容器内 localhost

Docker Compose 中如果写了 `HTTP_PROXY=127.0.0.1:7890`，容器内 localhost 指向容器自己而非宿主 NAS，会导致代理完全失效。正确写法：

```yaml
environment:
  HTTP_PROXY: "http://192.168.10.20:7890"
  HTTPS_PROXY: "http://192.168.10.20:7890"
  ALL_PROXY: "http://192.168.10.20:7890"
  UPDATE_PROXY_URL: "http://192.168.10.20:7890"
```

本脚本会检测容器 env 中是否仍残留 `127.0.0.1:7890`，发现后告警。

## License

MIT — 与 Sub2API 主仓库保持一致。
