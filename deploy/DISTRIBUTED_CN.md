# Sub2API 双服务器分布式部署手册

本文用于把已有 Sub2API 单机部署扩展为“两台应用节点 + 共享 PostgreSQL/Redis + 云负载均衡”。示例新增应用服务器为 `43.155.229.90`。

## 架构

```text
用户
  |
云负载均衡 CLB/LB
  |---------------------------|
旧服务器 Sub2API              新服务器 Sub2API
旧服务器 PostgreSQL/Redis  <--  新服务器通过内网/VPN 访问
```

核心原则：

- 新服务器只跑 Sub2API 应用，不新建 PostgreSQL/Redis。
- 两个应用节点必须连接同一套 PostgreSQL 和 Redis。
- `JWT_SECRET`、`TOTP_ENCRYPTION_KEY`、OAuth 密钥必须在两个节点完全一致。
- PostgreSQL/Redis 只允许内网/VPN 访问，并用防火墙只放行新服务器。
- 不需要重启整台服务器，也不要执行 `systemctl restart docker`。

## 1. 旧服务器检查

进入旧服务器的 Sub2API 部署目录，确认 `.env` 里这些值不是空值或默认占位：

```bash
grep -E '^(POSTGRES_PASSWORD|REDIS_PASSWORD|JWT_SECRET|TOTP_ENCRYPTION_KEY|POSTGRES_USER|POSTGRES_DB)=' .env
```

要求：

- `POSTGRES_PASSWORD` 必须是固定值。
- `REDIS_PASSWORD` 建议必须设置；一旦 Redis 要给另一台服务器访问，不能空密码。
- `JWT_SECRET` 必须是固定值，否则用户登录状态会在多节点之间失效。
- `TOTP_ENCRYPTION_KEY` 必须是固定值，否则 2FA 数据可能在不同节点不可读。

如果 `REDIS_PASSWORD`、`JWT_SECRET` 或 `TOTP_ENCRYPTION_KEY` 为空，先在低峰期补齐，然后只重建相关容器，不要重启 Docker daemon。

## 2. 旧服务器开放共享数据层

优先使用旧服务器的内网 IP 或 VPN IP，例如 `10.0.0.10`。不要把 PostgreSQL/Redis 直接绑定到 `0.0.0.0`。

如果旧服务器使用 `docker-compose.local.yml`：

```bash
POSTGRES_BIND_HOST=10.0.0.10 REDIS_BIND_HOST=10.0.0.10 \
docker compose -f docker-compose.local.yml \
  -f docker-compose.primary-distributed.override.yml up -d
```

如果旧服务器当前使用部署脚本生成的 `docker-compose.yml`：

```bash
POSTGRES_BIND_HOST=10.0.0.10 REDIS_BIND_HOST=10.0.0.10 \
docker compose -f docker-compose.yml \
  -f docker-compose.primary-distributed.override.yml up -d
```

这一步可能会短暂重建 PostgreSQL/Redis 容器，建议低峰期执行。数据目录/卷不删除，正常不会丢数据。

然后只放行新服务器访问数据库和 Redis：

```bash
# ufw 示例
ufw allow from 43.155.229.90 to any port 5432 proto tcp
ufw allow from 43.155.229.90 to any port 6379 proto tcp
```

```bash
# firewalld 示例
firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="43.155.229.90" port protocol="tcp" port="5432" accept'
firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="43.155.229.90" port protocol="tcp" port="6379" accept'
firewall-cmd --reload
```

## 3. 新服务器部署应用节点

在 `43.155.229.90` 上创建部署目录：

```bash
mkdir -p ~/sub2api-app
cd ~/sub2api-app
```

把本仓库里的两个文件上传到新服务器：

```bash
scp deploy/docker-compose.standalone.yml root@43.155.229.90:~/sub2api-app/
scp deploy/.env.standalone.example root@43.155.229.90:~/sub2api-app/.env
```

如果这些文件已经发布到你的 GitHub 仓库，也可以直接在新服务器下载：

```bash
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/docker-compose.standalone.yml -o docker-compose.standalone.yml
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/.env.standalone.example -o .env
```

然后设置权限并编辑配置：

```bash
chmod 600 .env
nano .env
```

编辑 `.env`：

- `DATABASE_HOST` 改为旧服务器内网/VPN IP。
- `REDIS_HOST` 改为旧服务器内网/VPN IP。
- `DATABASE_PASSWORD` 复制旧服务器 `.env` 的 `POSTGRES_PASSWORD`。
- `REDIS_PASSWORD` 复制旧服务器 `.env` 的 `REDIS_PASSWORD`。
- `JWT_SECRET` 复制旧服务器 `.env` 的 `JWT_SECRET`。
- `TOTP_ENCRYPTION_KEY` 复制旧服务器 `.env` 的 `TOTP_ENCRYPTION_KEY`。
- OAuth/Antigravity/Gemini 相关密钥如果旧服务器有配置，也要复制。

启动新节点：

```bash
docker compose -f docker-compose.standalone.yml up -d
docker compose -f docker-compose.standalone.yml logs -f sub2api
```

验证：

```bash
curl http://127.0.0.1:8080/health
curl http://43.155.229.90:8080/health
```

如果新服务器不能连旧服务器数据库/Redis，先检查内网/VPN 连通性和防火墙：

```bash
nc -vz 10.0.0.10 5432
nc -vz 10.0.0.10 6379
```

## 4. 云负载均衡配置

在云负载均衡中添加两个后端：

- 旧服务器：`8080`
- 新服务器：`43.155.229.90:8080`

推荐配置：

- 监听：`80/443`
- 证书：放在负载均衡层统一管理
- 健康检查路径：`/health`
- 健康检查成功状态码：`200`
- 调度策略：轮询
- 后端协议：HTTP
- 空闲超时：建议不低于 `900s`，避免长流式请求被负载均衡提前断开

先把新服务器低权重加入，观察正常后再改为两台均衡。

### 已使用 Cloudflare Tunnel 的场景

如果线上域名当前已经通过 `cloudflared` 指向旧服务器的 `http://localhost:8080`，也可以在新服务器上新增同一个 Cloudflare Tunnel 的 connector，让 Cloudflare 同时连接两台应用节点。

适用条件：

- 新服务器本机 `curl http://127.0.0.1:8080/health` 正常。
- 新服务器公网或本机 `curl http://43.155.229.90:8080/health` 正常。
- 新服务器能通过共享 PostgreSQL/Redis 读到同一套站点配置，例如 `/api/v1/settings/public` 返回 200。
- 旧服务器当前 `cloudflared_tunnel` 使用 token 模式运行，新增 connector 需要复用同一个 tunnel token。

上线前必须确认：新增 connector 会让真实用户流量有概率打到新服务器，属于真实切流操作。执行前应先得到明确确认。

新增 connector 的典型命令：

```bash
docker run -d \
  --name cloudflared_tunnel \
  --restart unless-stopped \
  --network host \
  cloudflare/cloudflared:latest \
  tunnel --no-autoupdate run --token '<CLOUDFLARE_TUNNEL_TOKEN>'
```

注意：Cloudflare Tunnel 远端配置里的 origin 是 `http://localhost:8080` 时，新服务器上的 `cloudflared` 容器必须使用 `--network host`。否则容器内的 `localhost` 会指向 cloudflared 容器自身，而不是宿主机上的 Sub2API，线上会出现 502。

回滚新 connector：

```bash
docker rm -f cloudflared_tunnel
```

回滚只影响新服务器 connector，不会停止旧服务器上的 tunnel。

## 5. 验证清单

单节点：

```bash
curl http://旧服务器IP:8080/health
curl http://43.155.229.90:8080/health
```

数据一致性：

- 从旧节点登录后台创建或查看用户/API Key。
- 从新节点访问后台确认数据一致。

会话：

- 登录后连续刷新多次。
- 确认请求打到不同节点时不会退出登录。

API：

- 用同一个 API Key 连续请求多次。
- 确认两个节点都能正常代理。

故障：

- 临时从负载均衡摘除一个节点，确认用户仍可访问。
- 不建议直接关数据库/Redis，因为当前数据层仍是单点。

## 6. 回滚

如果新节点异常：

```bash
# 先在云负载均衡摘除新服务器，然后在新服务器执行
docker compose -f docker-compose.standalone.yml down
```

如果旧服务器开放数据库/Redis 后发现风险：

- 先确认新节点已停止或从负载均衡摘除。
- 删除防火墙放行规则。
- 低峰期撤掉 `docker-compose.primary-distributed.override.yml` 后重新 `up -d`。

## 风险

- 当前方案提升应用层并发，但 PostgreSQL/Redis 仍是单点。
- 如果两台服务器不在同一 VPC，必须先建立 VPN/内网隧道。
- 不要把 PostgreSQL/Redis 裸露到公网。
- 不要重启整个 Docker 服务；最多重建相关 compose 容器。
