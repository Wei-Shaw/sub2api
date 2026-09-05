# 非标端口 + Cloudflare DNS-01 自动证书部署指南

> **适用场景**：服务器 80/443 端口被商家封锁，只能用非标端口（如 8443）对外提供 HTTPS；域名托管在 Cloudflare，不走橙云代理（直连）；希望证书自动签发并自动续期。

本方案是对**官方 Docker 部署**（`deploy/docker-compose.local.yml`）的**增量补丁**——不修改官方文件，只通过追加一个 compose 叠加文件的方式加入 Caddy 反代。升级官方 compose 时不会冲突。

## 目录文件说明

| 文件 | 作用 |
|---|---|
| [Dockerfile.caddy](Dockerfile.caddy) | 构建带 Cloudflare DNS 插件的 Caddy 镜像（不用改，保留即可） |
| [Caddyfile](Caddyfile) | Caddy 反代配置（需改域名和邮箱两处） |

> `caddy.yml`（增量 compose）和需要追加到 `.env` 的变量**都写在本教程里**，按阶段 5、6 的步骤复制创建即可。

**架构**：

```
客户端  ──https──►  服务器:8443  ──►  Caddy 容器  ──内网──►  sub2api 容器:8080
                                        ▲
                                        │ 自动走 DNS-01 挑战
                                        ▼
                                  Cloudflare DNS API
```

## 为什么必须走 DNS-01 挑战

Let's Encrypt 三种验证方式：

| 方式 | 要求 | 你的情况 |
|---|---|---|
| HTTP-01 | 80 端口可达 | ❌ 被封 |
| TLS-ALPN-01 | 443 端口可达 | ❌ 被封 |
| **DNS-01** | 能操作域名 DNS | ✅ CF API 可自动化 |

DNS-01 通过在域名下添加 TXT 记录证明所有权，不依赖任何入站端口，因此服务器端口被封也能签证书。

---

## 阶段 0：先确认三件事

### 0.1 确认 8443 端口真的开着

商家封了 80/443，你要确认 8443 不在黑名单。**在本地电脑**执行：

```bash
nc -vz YOUR_SERVER_IP 8443
```

- ❌ 长时间没反应（30 秒+）→ 防火墙挡了，去商家控制台放通 8443
- ✅ 很快报 `Connection refused` → 端口通的，只是现在还没服务，正常

### 0.2 确认你有这些

- [ ] 域名（假设 `example.com`），DNS 托管在 Cloudflare
- [ ] 服务器 SSH 登录方式（IP + 用户名 + 密码/密钥）
- [ ] 服务器是 Linux（Ubuntu/Debian/CentOS 都行）
- [ ] 服务器有公网 IP（假设 `1.2.3.4`）

### 0.3 想好两个值

- **子域名**：比如 `api.example.com`（下面都用这个占位）
- **邮箱**：Let's Encrypt 用它发证书到期提醒

---

## 阶段 1：在 Cloudflare 做两件事

### 1.1 添加 DNS A 记录（灰云朵）

1. 登录 Cloudflare → 选中你的域名
2. 左侧菜单 **DNS** → **Records**
3. 点 **Add record**
4. 按这样填：
   - Type: **A**
   - Name: **api**（只填前缀，不要写完整域名）
   - IPv4 address: **1.2.3.4**（你服务器的公网 IP）
   - Proxy status: **点橙云让它变成灰色**（DNS only）—— 关键步骤，橙云会走 CF 代理，但 CF 免费版不代理 8443
   - TTL: Auto
5. 点 **Save**

验证（本地终端）：
```bash
dig +short api.example.com
# 应输出 1.2.3.4；若输出 104.xx.xx.xx 说明云朵还是橙的，回去点灰
```

### 1.2 创建 API Token

Caddy 签发证书时需要临时往 DNS 加一条 TXT 记录，我们给它一把只能改**这一个域名** DNS 的钥匙。

1. Cloudflare 右上角头像 → **My Profile** → 左侧 **API Tokens**
2. 点 **Create Token**
3. 找到 **Edit zone DNS** 模板，点右边的 **Use template**
4. 配置：
   - Token name: `sub2api-caddy-dns`（随便起）
   - Permissions（模板已自动填好，确认即可）：
     - `Zone` - `DNS` - `Edit`
     - `Zone` - `Zone` - `Read`
   - **Zone Resources**：选 `Include` - `Specific zone` - `example.com`（**只选你这一个域名**，不要用 All zones）
5. 点 **Continue to summary** → **Create Token**
6. **立刻复制页面显示的 token**（形如 `a1b2c3d4e5...` 一长串）。**关闭后就再也看不到了**，丢了只能重建
7. 找个地方临时保存，后面要用

验证 token 可用：
```bash
curl -sS "https://api.cloudflare.com/client/v4/user/tokens/verify" \
  -H "Authorization: Bearer 刚复制的token"
# 看到 "status": "active" 就对了
```

---

## 阶段 2：服务器环境准备

### 2.1 SSH 登录

```bash
ssh root@1.2.3.4
# 或用密钥：ssh -i ~/.ssh/id_rsa root@1.2.3.4
```

**下面所有命令都在服务器上执行**。

### 2.2 装 Docker 和 Docker Compose

```bash
# 官方一键安装脚本（Ubuntu/Debian/CentOS 都支持）
curl -fsSL https://get.docker.com | sh

# 启动 Docker 并设为开机自启
systemctl enable --now docker

# 验证
docker --version           # 应显示 Docker version 27.x 之类
docker compose version     # 应显示 Docker Compose version v2.x
```

如果 `docker compose version` 报命令不存在：
```bash
apt-get install -y docker-compose-plugin   # Debian/Ubuntu
yum install -y docker-compose-plugin       # CentOS/RHEL
```

---

## 阶段 3：拿到本方案的补丁文件

### 情况 A：你**还没装** sub2api

clone 整个仓库，把 `deploy/` 拿出来当部署目录：

```bash
cd ~
git clone https://github.com/Wei-Shaw/sub2api.git
mv sub2api/deploy ~/sub2api-deploy
rm -rf sub2api
cd ~/sub2api-deploy
```

之后按官方 README 完成 sub2api 的初次启动，再回来继续阶段 4。

### 情况 B：你**已经用官方 docker-deploy.sh 装好了** sub2api

部署目录里只有 `docker-compose.yml` + `.env` + 数据目录，没有 `caddy-dns01/`。**只需要把这一个目录补进去**（本地 macOS 用 `tar` 管道传，避开某些服务器 SFTP 子系统失败的问题）：

```bash
# 在本地仓库目录执行
cd /path/to/sub2api/deploy
COPYFILE_DISABLE=1 tar czf - caddy-dns01 | \
  ssh root@SERVER_IP 'cd /root/sub2api-deploy && tar xzf - && rm -f caddy-dns01/._*'
```

> 为什么不用 `scp -r`？OpenSSH 9+ 的 `scp` 默认走 SFTP 子系统，部分服务器没启 sftp-server，会报 `Connection closed`。`tar | ssh` 只用纯 SSH 通道，最稳。
> 为什么 `COPYFILE_DISABLE=1` + `rm -f ._*`？macOS 的 BSD `tar` 会把 AppleDouble 元数据打包，传到 Linux 后产生 `._Caddyfile` 之类的垃圾文件，必须清掉。

### 情况 C：兼容两种官方 compose 文件

后续命令统一以 `docker-compose.yml` 为例。如果你用的是 `docker-compose.local.yml`，把 **所有 `-f docker-compose.yml`** 替换成 `-f docker-compose.local.yml` 即可。

最终 `~/sub2api-deploy/` 应该包含：

```
~/sub2api-deploy/
├── docker-compose.yml            # 官方（或 docker-compose.local.yml）
├── .env                          # 你已配好的官方 .env
├── caddy-dns01/                  # 本方案的补丁目录
│   ├── Caddyfile                 # 阶段 5 里修改
│   ├── Dockerfile.caddy          # 不用动
│   ├── GUIDE.md                  # 本教程
│   └── caddy/                    # caddy 容器的所有持久数据（首次启动后自动创建）
│       ├── data/                 #   - Let's Encrypt 证书 + ACME 账户私钥【绝对不能删】
│       ├── config/               #   - Caddy 自动生成的配置缓存
│       └── logs/                 #   - 访问日志
└── data/ postgres_data/ redis_data/   # 官方部署的数据目录
```

**所有持久数据都是宿主机本地目录**——证书、配置、数据库都能直接在文件系统看到，备份/迁移只需 `tar` 整个 `sub2api-deploy/`。

---

## 阶段 4：准备 `.env`（官方配置 + 本方案追加）

### 4.1 先按官方流程准备 .env

```bash
cd ~/sub2api-deploy
cp .env.example .env

# 生成随机密钥
echo "POSTGRES_PASSWORD=$(openssl rand -hex 32)"
echo "JWT_SECRET=$(openssl rand -hex 32)"
echo "TOTP_ENCRYPTION_KEY=$(openssl rand -hex 32)"
```

编辑 `.env`，把上面三个随机值填进对应位置：

```bash
nano .env
```

至少填：
```
POSTGRES_PASSWORD=上面生成的第一个
JWT_SECRET=上面生成的第二个
TOTP_ENCRYPTION_KEY=上面生成的第三个
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=设一个强密码
```

### 4.2 追加本方案需要的新变量

#### 4.2.1 改 BIND_HOST（不是追加）

官方 `.env.example` 里通常**已经有** `BIND_HOST=0.0.0.0`。直接追加 `BIND_HOST=127.0.0.1` 会变成两条同名变量、行为不确定。**用 `sed` 修改原行**：

```bash
# 检查当前值
grep '^BIND_HOST=' .env

# 改成 127.0.0.1
sed -i 's/^BIND_HOST=0\.0\.0\.0$/BIND_HOST=127.0.0.1/' .env

# 确认改完了
grep '^BIND_HOST=' .env    # 应输出：BIND_HOST=127.0.0.1
```

如果你的 `.env` 里**没有** `BIND_HOST` 这一行（旧版本 .env.example），就把它一起追加到下面那段里。

#### 4.2.2 追加 token 等新变量

```bash
cat >> .env <<'EOF'

# ===== Caddy DNS-01 方案追加 =====
# Cloudflare API Token（阶段 1.2 创建的那个，权限 Zone:DNS:Edit + Zone:Zone:Read）
CLOUDFLARE_API_TOKEN=粘贴阶段1.2保存的_cf_token

# Caddy 对外监听端口（默认 8443；商家只放通其它端口时改这里）
# CADDY_PORT=8443
EOF
```

上面写进去的 token 是**占位符**。执行完后用 `nano .env` 把 `CLOUDFLARE_API_TOKEN=粘贴阶段1.2保存的_cf_token` 这一行改成你的真实 token。

**关键变量解释**：

| 变量 | 作用 |
|---|---|
| `CLOUDFLARE_API_TOKEN` | Caddy 通过它调用 CF API 完成 DNS-01 挑战和续期 |
| `BIND_HOST=127.0.0.1` | 让官方 compose 里 sub2api 的 8080 端口只绑定本机，禁止外部直连绕过 Caddy。Caddy 容器通过 Docker 内网访问 sub2api，不受此限制 |
| `CADDY_PORT` | Caddy 对外监听端口，默认 8443；商家只开其它端口时改这里 |

保护好 `.env`：
```bash
chmod 600 .env
```

---

## 阶段 5：准备 caddy.yml 和 Caddyfile

### 5.1 创建 caddy.yml（增量 compose 叠加文件）

这个文件只追加 `caddy` 服务，不修改官方服务的任何字段，官方升级时不会冲突。

**数据用 bind mount 而不是命名 volume**：证书、配置、日志都直接落在 `caddy-dns01/caddy/{data,config,logs}/`，跟官方 sub2api/postgres/redis 的本地目录设计一致；备份只需 `tar` 整个 `caddy-dns01/`，不用 `docker run alpine` 单独打包 volume。

> ⚠️ **关键陷阱**：docker compose 多 `-f` 文件叠加时，`build.context` 和 `volumes` 里的相对路径 **不是** 相对于 `caddy.yml` 自身，**而是相对于"第一个 -f 文件"的目录**（即 `~/sub2api-deploy/`）。所以下面所有相对路径都带 `./caddy-dns01/` 前缀。

在 `~/sub2api-deploy/` 下执行下面这整段（heredoc 用单引号 `'EOF'`，防止 shell 展开里面的 `${...}`）：

```bash
cd ~/sub2api-deploy
mkdir -p caddy-dns01/caddy/data caddy-dns01/caddy/config caddy-dns01/caddy/logs
cat > caddy-dns01/caddy.yml <<'EOF'
# =============================================================================
# Caddy 反代增量 compose 文件（叠加到官方 docker-compose.yml 使用）
#
# 用法：在 deploy/ 目录下执行
#   docker compose -f docker-compose.yml -f caddy-dns01/caddy.yml up -d
#
# 本文件只追加 caddy 服务，不修改任何官方服务配置。
# 官方 sub2api 容器是否对外暴露 8080 由 .env 中的 BIND_HOST 控制
# （必须设为 127.0.0.1，否则外部流量会绕过 Caddy 直接打到明文 8080）。
#
# 数据存储：用 bind mount 而不是命名 volume，证书直接落在
#   caddy-dns01/caddy/data/   ← Let's Encrypt 证书 + ACME 账户私钥【绝对不能删】
#   caddy-dns01/caddy/config/ ← Caddy 自动配置缓存
#   caddy-dns01/caddy/logs/   ← 访问日志
#
# 注意：所有相对路径相对于"第一个 -f 文件"的目录（即 deploy/），
# 不是相对于本文件，所以下面带 ./caddy-dns01/ 前缀。
# =============================================================================

services:
  caddy:
    build:
      context: ./caddy-dns01
      dockerfile: Dockerfile.caddy
    container_name: sub2api-caddy
    restart: unless-stopped
    ports:
      # 对外端口由 .env 的 CADDY_PORT 控制，默认 8443
      - "${CADDY_PORT:-8443}:${CADDY_PORT:-8443}"
    volumes:
      - ./caddy-dns01/Caddyfile:/etc/caddy/Caddyfile:ro
      - ./caddy-dns01/caddy/data:/data
      - ./caddy-dns01/caddy/config:/config
      - ./caddy-dns01/caddy/logs:/var/log/caddy
    environment:
      # Caddyfile 通过 {env.CLOUDFLARE_API_TOKEN} 读取此值
      - CLOUDFLARE_API_TOKEN=${CLOUDFLARE_API_TOKEN:?CLOUDFLARE_API_TOKEN is required}
    depends_on:
      - sub2api
    networks:
      - sub2api-network

networks:
  # 与官方 compose 同名，多文件叠加时合并为同一网络
  sub2api-network:
    driver: bridge
EOF
```

验证文件写入正常（注意 `-f` 用的是你实际的 compose 文件名）：
```bash
docker compose -f docker-compose.yml -f caddy-dns01/caddy.yml config > /dev/null && echo OK
```

### 5.2 修改 Caddyfile

```bash
nano caddy-dns01/Caddyfile
```

找到并修改两处：

```caddy
email YOUR_EMAIL@example.com          # ← 改成你的邮箱
...
api.example.com:8443 {                # ← 改成你的域名，:8443 保留
```

> 如果阶段 4 里把 `CADDY_PORT` 改成其它值（比如 9443），这里的 `:8443` 也要同步改成 `:9443`。两处必须一致。

保存：`Ctrl+O` → 回车 → `Ctrl+X`。

---

## 阶段 6：启动

### 6.1 设个 alias 简化命令

每次手敲 `-f docker-compose.yml -f caddy-dns01/caddy.yml` 太长。先做个别名（**`dc=` 后面的部分按你的实际 compose 文件名改**）：

```bash
echo 'alias dc="docker compose -f /root/sub2api-deploy/docker-compose.yml -f /root/sub2api-deploy/caddy-dns01/caddy.yml"' >> ~/.bashrc
source ~/.bashrc
```

下面所有命令都用 `dc` 别名。

### 6.2 构建 Caddy 镜像并启动（一步搞定）

第一次会下载 Caddy builder 镜像 + 一堆 Go 模块 + 编译，**约 2~5 分钟**。

> ⚠️ **如果你的服务器有 fail2ban / SSH 连接限流**（典型表现：执行长命令时 SSH 突然断开），用下面这种**后台启动 + 日志重定向**的方式，避免 SSH 长会话被踹：
>
> ```bash
> # 后台跑，日志写到 /tmp/caddy-build.log
> nohup setsid dc up -d --build </dev/null >/tmp/caddy-build.log 2>&1 &
> echo "PID=$!"
> ```
>
> 否则直接同步执行也行：
> ```bash
> dc up -d --build
> ```

> ⚠️ **sub2api 容器会被 Recreate**——因为 `BIND_HOST` 从 `0.0.0.0` 改成了 `127.0.0.1`，docker compose 检测到 ports 段变化会销毁并重建 sub2api 容器。**预计 5-15 秒服务中断**。

监控构建进度：

```bash
tail -f /tmp/caddy-build.log    # 后台模式
# 或者
dc logs -f caddy                  # 直接前台启动后看 caddy
```

构建完成的标志：日志里出现 `naming to docker.io/library/sub2api-deploy-caddy:latest done`。

### 6.3 看容器状态

```bash
dc ps
```

应该看到 4 个服务都是 `Up`，sub2api 和 postgres、redis 是 `(healthy)`。

### 6.4 看 Caddy 证书签发日志

```bash
docker logs sub2api-caddy 2>&1 | grep -iE "obtain|certificate|challenge"
```

**正常 ACME 流程**（实测约 14 秒，文档原说 30s~2min 偏保守）：

```
obtaining certificate, identifier: api.example.com
trying to solve challenge, challenge_type: dns-01
authorization finalized, authz_status: valid
successfully downloaded available certificate chains
certificate obtained successfully, identifier: api.example.com
```

看到 `certificate obtained successfully` 就是成功。

---

## 阶段 7：验证

> ⚠️ **本地有代理（Clash/Surge/Mihomo 等）时，下面的本地 curl/openssl/nc 测试都不可信**——代理可能劫持解析、伪造连通性。如果你的网络环境有这些工具，**改用服务器自身做"环回测试"**：在服务器上 `curl https://你的域名:8443/health`，等价于外部访问。

### 7.1 证书真的签下来了吗

**有代理的环境**：在服务器上执行
```bash
echo | openssl s_client -connect api.example.com:8443 -servername api.example.com 2>/dev/null \
  | openssl x509 -noout -dates -issuer -subject
```

**干净环境**：在本地执行同样的命令。

应看到：
```
notBefore=今天日期
notAfter =90 天后
issuer= ...Let's Encrypt...
subject=CN=你的域名
```

### 7.2 健康检查

```bash
curl -sS https://api.example.com:8443/health
# 应输出：{"status":"ok"}
```

### 7.3 确认 8080 真的对外不可达（防绕过）

**先在服务器上用 ss 看绑定地址**（这个最权威）：
```bash
ss -tlnp | grep ':8080'
# 期望：LISTEN ... 127.0.0.1:8080 ...   ← 必须是 127.0.0.1，不能是 0.0.0.0
```

**再做实际访问验证**（在服务器上用自己的公网 IP 访问，等价于外部）：
```bash
curl -sS -m 5 http://你的服务器公网IP:8080/health
# 期望：Failed to connect ... Could not connect to server
```

如果 `ss` 显示 `0.0.0.0:8080`，说明 `BIND_HOST` 没生效。回阶段 4.2.1 检查，改完后 `dc up -d sub2api` 重启。

### 7.4 打开管理后台

浏览器访问：**`https://api.example.com:8443`**

URL 必须带 `:8443`。用 `.env` 里的 `ADMIN_EMAIL` + `ADMIN_PASSWORD` 登录。

---

## 阶段 8：客户端配置

### Claude Code（CLI）

```bash
export ANTHROPIC_BASE_URL="https://api.example.com:8443"
export ANTHROPIC_AUTH_TOKEN="你在 sub2api 后台创建的 API Key"
claude
```

### Codex CLI（OpenAI 兼容）

编辑 `~/.codex/config.toml`：
```toml
[providers.sub2api]
base_url = "https://api.example.com:8443/v1"
env_key = "SUB2API_KEY"
```

然后 `export SUB2API_KEY=sk-xxx`。

### 通用（curl 测试）

```bash
curl https://api.example.com:8443/v1/messages \
  -H "Authorization: Bearer sk-你的apikey" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4",
    "max_tokens": 100,
    "messages": [{"role": "user", "content": "hi"}]
  }'
```

**要点**：所有 URL 都必须带 `:8443`。

---

## 阶段 9：日常维护

为方便，下面命令都假设你已设置 `dc` 别名（见阶段 6.2）；没设的话把 `dc` 换成完整的 `docker compose -f docker-compose.local.yml -f caddy-dns01/caddy.yml`。

### 升级 sub2api

```bash
dc pull sub2api
dc up -d sub2api
```

### 升级 Caddy（或修改了 Dockerfile.caddy 后）

```bash
dc build caddy
dc up -d caddy
```

### 改了 Caddyfile 后热加载（不中断服务）

```bash
dc exec caddy caddy reload --config /etc/caddy/Caddyfile
```

### 看日志

```bash
dc logs sub2api                                             # 后端日志（容器 stdout）
dc logs caddy                                                # 反代日志（容器 stdout）
dc logs -f --tail 100                                        # 实时看最新 100 行
tail -50 caddy-dns01/caddy/logs/sub2api.log                  # Caddy 访问日志（落盘）
```

### 证书续期

**完全自动**。Caddy 到期前 30 天自动走 DNS-01 重新签发并热加载。唯一要求：**不要删 `caddy-dns01/caddy/data/` 目录**（里面是证书和 ACME 账户私钥，删了会触发重新签发，频繁触发会被 Let's Encrypt 限流封 7 天）。

手动触发（用于测试或故障恢复）：
```bash
dc exec caddy caddy reload --force --config /etc/caddy/Caddyfile
```

### 备份

所有数据都在宿主机本地目录，**整个 `sub2api-deploy/` `tar` 一下就行**——证书、配置、数据库、日志全在里面：

```bash
cd ~
tar czf sub2api-backup-$(date +%Y%m%d).tar.gz sub2api-deploy/
```

如果想分门别类（推荐，便于按需恢复）：

```bash
cd ~
# 配置文件（最重要，丢了就要重头来一次）
tar czf sub2api-config-$(date +%Y%m%d).tar.gz \
  sub2api-deploy/.env \
  sub2api-deploy/docker-compose.yml \
  sub2api-deploy/caddy-dns01/Caddyfile \
  sub2api-deploy/caddy-dns01/caddy.yml \
  sub2api-deploy/caddy-dns01/Dockerfile.caddy

# Caddy 数据（证书、ACME 账户）
tar czf caddy-data-$(date +%Y%m%d).tar.gz \
  sub2api-deploy/caddy-dns01/caddy/data \
  sub2api-deploy/caddy-dns01/caddy/config

# 应用数据（数据库 + sub2api 数据）
tar czf sub2api-data-$(date +%Y%m%d).tar.gz \
  sub2api-deploy/data \
  sub2api-deploy/postgres_data \
  sub2api-deploy/redis_data
```

---

## 常见问题

| 症状 | 原因 & 修复 |
|---|---|
| `dc up -d --build` 报 `failed to read dockerfile: open Dockerfile.caddy: no such file or directory` | caddy.yml 里的 `build.context: .` 写错了。多 -f 叠加时，相对路径相对于第一个 -f 文件的目录，不是 caddy.yml 自身。改成 `context: ./caddy-dns01` |
| `docker compose config` 警告 `The "xxxxxxx" variable is not set. Defaulting to a blank string.` | `.env` 里某个值（通常是密码）含有 `$xxx` 子串被当成变量引用了。把 `$` 转义成 `$$`（如 `password$abc` → `password$$abc`），重新 `dc up -d` |
| 长 SSH 命令（build / up）执行到一半 SSH 断开 | 服务器有 fail2ban 或 SSH 连接限流。改用 `nohup setsid dc up -d --build </dev/null >/tmp/caddy-build.log 2>&1 &` 后台跑，断了也不影响 |
| `scp` 报 `Connection closed`（SSH 本身能连） | OpenSSH 9+ 默认走 SFTP 子系统，服务器没启 sftp-server。改用 `tar czf - dir \| ssh host 'cd /path && tar xzf -'` 兜底 |
| 文件传到服务器后多了 `._Caddyfile` 之类 | macOS 的 BSD `tar` 默认带 AppleDouble 元数据。下次 `COPYFILE_DISABLE=1 tar ...`；当前先 `rm -f caddy-dns01/._*` |
| 本地 `curl https://域名:8443` 超时但服务器内部 `curl` 可达 | 你本地有代理（Clash/Surge/Mihomo）拦截 8443 或劫持 DNS。换干净网络，或直接信任服务器自身环回测试结果 |
| `dc up -d --build` 后 caddy 一直 `Restarting` | 看 `dc logs caddy`。90% 是 CF token 没填、权限不够、或域名 zone 没选对 |
| `dc up -d` 后立刻 curl 报 `503 Service Unavailable`（约 30 秒后自动恢复） | 正常现象。Caddy 启动时 sub2api 还在 init，反代 health check 第一次失败把上游标记为 down → 拒绝请求。Caddy 默认每 30s 检查一次，下一轮 check 通过就好。看日志会有 `connection refused` → `host is up` 的转换 |
| Caddy 日志卡在 `waiting on propagation` 超过 2 分钟 | token 的 Zone Resources 没包含你的域名，或少了 `Zone:DNS:Edit` 权限 |
| 浏览器提示证书不受信任 | 证书还没签下来。看 Caddy 日志，等 `certificate obtained successfully` |
| `curl https://你的域名:8443/health` 超时（确认非本地代理） | 服务器防火墙没放通 8443（回阶段 0.1 检查），或 Cloudflare A 记录还是橙云 |
| 外部仍能访问 `http://公网IP:8080/health` | `BIND_HOST` 没改成 127.0.0.1，流量绕过 Caddy 明文暴露。回阶段 4.2.1 |
| 登录后台显示 502 | sub2api 容器没起来或没 ready。`dc logs sub2api` 看原因，通常是数据库密码不对或 `.env` 少了变量 |
| 登录后台 401 `INVALID_CREDENTIALS`（密码确信没错） | 1) **检查 email** 是不是 `.env` 里的 `ADMIN_EMAIL`：`grep ^ADMIN_EMAIL= .env`。`docker-deploy.sh` 装的 sub2api 默认会**随机生成一个 `*.local` 占位 email**（不是你给 LE 用的联系邮箱）。2) 检查 `ADMIN_PASSWORD` 是否含 `$` 字符——如果含，docker compose 会把 `$xxx` 当变量替换，sub2api 收到的密码与你以为的不同，hash 对不上 |
| Claude Code 报 403 | 后台里创建的 API Key 还没分配到一个分组（sub2api 要求每个 Key 必须分配到分组才能用） |
| 想换域名 | 改 [Caddyfile](Caddyfile) 里的域名 → `dc exec caddy caddy reload --config /etc/caddy/Caddyfile`，自动签发新证书 |
| 想换端口（8443 → 9443） | 改 `.env` 的 `CADDY_PORT=9443`，同时改 [Caddyfile](Caddyfile) 里 `:8443` → `:9443`，然后 `dc up -d caddy` |
| Let's Encrypt 反复签发被限流 | 别反复删 `caddy-dns01/caddy/data/` 目录。一周内同一域名签发上限 50 次，触发后只能等 7 天 |
| 升级官方 compose 后不知道要不要重建 Caddy | 不用。官方升级改的是 sub2api/postgres/redis，caddy.yml 是独立的，互不影响 |
| 之前用了命名 volume（旧版 GUIDE），想迁到 bind mount | 1) 创建目录：`mkdir -p caddy-dns01/caddy/{data,config,logs}` 2) 停 caddy：`docker stop sub2api-caddy` 3) 拷数据（保留权限）：`cp -a $(docker volume inspect sub2api-deploy_caddy_data --format '{{.Mountpoint}}')/. caddy-dns01/caddy/data/` 4) config 同上 5) 改 caddy.yml 用本目录 6) `docker rm sub2api-caddy && dc up -d caddy` 7) 验证 OK 后 `docker volume rm sub2api-deploy_caddy_data sub2api-deploy_caddy_config` |

---

## 迁移到新服务器

因为所有数据都用 bind mount 落在宿主机本地目录（包括 Caddy 证书），**迁移就是 tar 一下**：

```bash
# === 源服务器 ===
cd ~
dc down                                # 优雅停服，保证数据一致性
tar czf sub2api-complete.tar.gz sub2api-deploy/

# === 传到新服务器 ===
scp sub2api-complete.tar.gz root@新IP:~/

# === 新服务器（先装好 Docker） ===
tar xzf sub2api-complete.tar.gz
cd sub2api-deploy/

# 改 Cloudflare DNS A 记录指向新 IP（在 CF 后台手动改，或用 ddns-go）

# 启动（caddy 直接读已有的证书，不会重新签发）
docker compose -f docker-compose.yml -f caddy-dns01/caddy.yml up -d
```

> 唯一要注意：新服务器上 `caddy-dns01/caddy/data/` 里的证书绑定的是**域名**，不是 IP。所以**域名解析切到新 IP 后**，证书继续有效，无需重新签发。但 DNS 切换前，新服务器对外服务时浏览器会报"证书域名不匹配"。最好的顺序是：先改 DNS、等传播、再启动新服务器的容器。
