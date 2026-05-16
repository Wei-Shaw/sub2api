# OceanWay 部署运行手册

这份文档是给后续 Codex 会话或人工运维直接执行用的。开始部署前先读本文件，不要凭旧记忆部署。

## 一句话结论

- `ocean-way.top` 是完整 sub2api 服务入口，包含登录、控制台、后台和 `/v1` API。
- `oceanway.site` 是面向非技术用户的引流站点，只允许 `/`、`/internal-home`、`/docs`、`/agents`，不开放登录、控制台、后台、`/api/*` 或 `/v1/*`。
- 当前是双服务器架构：`la-vps` 只做公网入口和域名分流，真实应用跑在 `ali-vps`。
- 普通前后端代码更新只部署 `ali-vps` 的应用镜像，不要在 `la-vps` 上部署 sub2api 应用。
- 镜像 tag 只是部署标记，不能作为应用版本号传入。不要使用 `--build-arg VERSION=${TAG}`。

## 当前线上拓扑

| 节点 | 地址 | 职责 |
| --- | --- | --- |
| `la-vps` | `64.188.30.215` | DNS 指向这里；运行 Caddy；负责双域名分流、旧 IP `/v1` 兼容入口、反代到 ali origin |
| `ali-vps` | `43.112.125.18` | 主业务服务器；运行 `sub2api`、PostgreSQL、Redis |
| `origin.oceanway.site` | 指向 `ali-vps` | 只给 `la-vps` 反代访问，不作为公开用户入口 |

访问链路：

```text
用户
  -> ocean-way.top / oceanway.site / 64.188.30.215:8080
  -> la-vps Caddy
  -> https://origin.oceanway.site
  -> ali-vps Caddy
  -> 127.0.0.1:8080
  -> ali-vps sub2api container
```

`ali-vps` 当前关键路径：

```text
/opt/sub2api/docker-compose.yml
/opt/sub2api/source
/opt/sub2api/data
/opt/sub2api/postgres_data
/opt/sub2api/redis_data
/root/sub2api-backups
```

`la-vps` 只在修改域名分流、Caddy 规则或旧 IP 兼容入口时才需要操作。

## 部署决策

| 变更类型 | 操作服务器 | 是否需要改 Caddy |
| --- | --- | --- |
| 前端页面、后端代码、Dockerfile、资源文件 | `ali-vps` | 否 |
| 只改部署文档 | 不需要部署 | 否 |
| 修改 `ocean-way.top` / `oceanway.site` 路由边界 | `la-vps` | 是 |
| 修改 `origin.oceanway.site` 访问限制 | `ali-vps` | 是 |
| 数据库迁移或一次性脚本 | `ali-vps` | 否 |

如果只是重新部署应用，后续会话应直接执行“应用部署流程”，不要碰 `la-vps`。

## 部署前检查

在本地仓库先确认要部署的代码范围：

```bash
git status --short --branch
git log --oneline -5 --decorate
```

建议先提交并推送再部署。注意：`git archive HEAD` 只包含已提交的 `HEAD`，不会包含未提交改动。如果用户明确要求部署未提交改动，先和用户确认是提交后部署，还是使用本地 Docker 直接从脏工作区构建。

推荐本地验证：

```bash
corepack pnpm --dir frontend run typecheck
corepack pnpm --dir frontend run build
(cd backend && go test ./...)
```

如果本机没有 `pnpm` 命令，使用 `corepack pnpm`。如果本机没有 Docker CLI，使用下面的“远端构建流程”。

## 应用部署流程

### 1. 生成部署变量

在本地仓库执行：

```bash
TAG=oceanway-$(date +%Y%m%d-%H%M)
COMMIT=$(git rev-parse --short HEAD)
echo "TAG=${TAG}"
echo "COMMIT=${COMMIT}"
cat backend/cmd/server/VERSION
```

`TAG` 示例：`oceanway-20260513-0921`。

应用版本来自 `backend/cmd/server/VERSION`，例如 `0.1.126`。页面左上角会显示 `v0.1.126`，不应显示 `voceanway-...`。

### 2A. 本地有 Docker 时

在本地构建镜像：

```bash
docker build -t sub2api:${TAG} --build-arg COMMIT=${COMMIT} .
docker run --rm sub2api:${TAG} /app/sub2api --version
```

必须确认输出类似：

```text
Sub2API 0.1.126 (commit: xxxxxxxx, built: ...)
```

如果输出里出现 `Sub2API oceanway-...`，说明错误地把镜像 tag 当成应用版本了，停止部署并检查构建命令。

打包并上传：

```bash
docker save sub2api:${TAG} | gzip > /tmp/sub2api-${TAG}.tar.gz
scp /tmp/sub2api-${TAG}.tar.gz ali-vps:/root/
```

在 `ali-vps` 加载镜像：

```bash
ssh ali-vps "TAG='${TAG}' bash -s" <<'EOF'
set -euo pipefail
gunzip -c "/root/sub2api-${TAG}.tar.gz" | docker load
docker run --rm "sub2api:${TAG}" /app/sub2api --version
EOF
```

### 2B. 本机没有 Docker 时

使用源码包上传到 `ali-vps` 后远端构建。这个流程是当前 Mac 环境常用路径。

本地打包已提交的 `HEAD`：

```bash
TAG=oceanway-$(date +%Y%m%d-%H%M)
COMMIT=$(git rev-parse --short HEAD)
ARCHIVE=/tmp/sub2api-source-${TAG}-${COMMIT}.tar.gz
REMOTE_ARCHIVE=/root/$(basename "${ARCHIVE}")
git archive --format=tar HEAD | gzip > "${ARCHIVE}"
scp "${ARCHIVE}" ali-vps:/root/
```

在 `ali-vps` 备份旧源码并构建：

```bash
ssh ali-vps "TAG='${TAG}' COMMIT='${COMMIT}' ARCHIVE='${REMOTE_ARCHIVE}' bash -s" <<'EOF'
set -euo pipefail
BACKUP_DIR=/root/sub2api-backups/pre-deploy-${TAG}-$(date +%Y%m%d-%H%M%S)

mkdir -p "${BACKUP_DIR}"
cp -a /opt/sub2api/docker-compose.yml "${BACKUP_DIR}/docker-compose.yml"
cp -a /opt/sub2api/source "${BACKUP_DIR}/source"

rm -rf /opt/sub2api/source
mkdir -p /opt/sub2api/source
tar -xzf "${ARCHIVE}" -C /opt/sub2api/source

cd /opt/sub2api/source
printf 'VERSION file: '
cat backend/cmd/server/VERSION
docker build -t sub2api:${TAG} --build-arg COMMIT=${COMMIT} .
docker run --rm sub2api:${TAG} /app/sub2api --version
EOF
```

不要执行：

```bash
docker build -t sub2api:${TAG} --build-arg VERSION=${TAG} ...
```

这会导致前端版本显示为 `voceanway-...`，属于错误部署。

### 3. 切换线上镜像

在 `ali-vps` 执行：

```bash
ssh ali-vps "TAG='${TAG}' COMMIT='${COMMIT}' bash -s" <<'EOF'
set -euo pipefail
cd /opt/sub2api

cp docker-compose.yml docker-compose.yml.bak-before-${TAG}-$(date +%Y%m%d-%H%M%S)
perl -0pi -e "s|image: sub2api:[^\n]+|image: sub2api:${TAG}|; s|COMMIT: [^\n]+|COMMIT: ${COMMIT}|" docker-compose.yml

docker compose up -d --no-build
docker compose ps
docker inspect sub2api --format 'Image={{.Config.Image}} Status={{.State.Status}} Health={{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}'
docker exec sub2api /app/sub2api --version
docker logs --tail=120 sub2api
EOF
```

必须看到：

```text
Image=sub2api:oceanway-YYYYMMDD-HHMM Status=running Health=healthy
Sub2API 0.1.xxx
```

`docker compose up` 必须在 `/opt/sub2api` 执行。不要在 `la-vps` 执行应用 compose。

## 部署后验证

从本地执行：

```bash
for url in \
  https://ocean-way.top/health \
  https://ocean-way.top/home \
  https://ocean-way.top/docs \
  https://ocean-way.top/agents \
  https://ocean-way.top/login \
  https://ocean-way.top/v1/models \
  https://oceanway.site/internal-home \
  https://oceanway.site/docs \
  https://oceanway.site/agents \
  https://oceanway.site/login \
  https://oceanway.site/v1/models \
  http://64.188.30.215:8080/v1/models \
  http://64.188.30.215:8080/home \
  https://origin.oceanway.site/health; do
  printf '%s -> ' "$url"
  curl -k -sS -o /dev/null -w '%{http_code} %{redirect_url}\n' "$url" || true
done
```

预期结果：

| URL | 预期 |
| --- | --- |
| `https://ocean-way.top/health` | `200` |
| `https://ocean-way.top/home` | `200` |
| `https://ocean-way.top/docs` | `200` |
| `https://ocean-way.top/agents` | `200` |
| `https://ocean-way.top/login` | `200` |
| `https://ocean-way.top/v1/models` | 未带 Key 通常 `401` |
| `https://oceanway.site/internal-home` | `200` |
| `https://oceanway.site/docs` | `200` |
| `https://oceanway.site/agents` | `200` |
| `https://oceanway.site/login` | `302` 到 `/internal-home` |
| `https://oceanway.site/v1/models` | `404` |
| `http://64.188.30.215:8080/v1/models` | 未带 Key 通常 `401` |
| `http://64.188.30.215:8080/home` | `404` |
| `https://origin.oceanway.site/health` | 非 `la-vps` 访问时应为 `404` 或被拒绝 |

验证页面版本：

```bash
curl -k -sS https://ocean-way.top/api/v1/settings/public
```

响应里的 `data.version` 应等于 `backend/cmd/server/VERSION`，不应是 `oceanway-YYYYMMDD-HHMM`。

浏览器人工检查：

- `ocean-way.top` 顶部允许进入控制台。
- `oceanway.site` 顶部不出现登录或控制台入口。
- `oceanway.site/docs` 和 `oceanway.site/agents` 可访问且仍留在 `oceanway.site` 域名下。
- `oceanway.site/agents` 的联系入口回到 `/internal-home#contact`。

## Caddy 配置

只有修改入口分流或域名边界时才操作 Caddy。

### la-vps 入口配置

`la-vps` 负责公网入口和域名分流，反代目标是 `origin.oceanway.site`。不要把 `ocean-way.top` 或 `oceanway.site` 改成反代 `127.0.0.1:8080`，除非明确回退为 la-vps 单机部署。

```caddy
(oceanway_headers) {
  header {
    Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
    X-Content-Type-Options "nosniff"
    Referrer-Policy "strict-origin-when-cross-origin"
  }
}

(oceanway_ali_origin) {
  reverse_proxy https://origin.oceanway.site {
    header_up Host origin.oceanway.site
    header_up X-Forwarded-Host {host}
    header_up X-Forwarded-Proto {scheme}
  }
}

ocean-way.top {
  encode zstd gzip
  import oceanway_ali_origin
  import oceanway_headers
}

oceanway.site {
  encode zstd gzip

  @blocked_api path /api /api/* /v1 /v1/*
  respond @blocked_api 404

  @blocked_app path /home /login /register /email-verify /forgot-password /reset-password /dashboard /dashboard/* /admin /admin/* /keys /keys/* /usage /usage/* /subscriptions /subscriptions/* /redeem /redeem/* /purchase /purchase/* /payment /payment/* /auth /auth/* /setup
  redir @blocked_app /internal-home 302

  import oceanway_ali_origin
  import oceanway_headers
}

http://64.188.30.215:8080 {
  bind 64.188.30.215
  encode zstd gzip

  @legacy_v1 path /v1 /v1/*
  handle @legacy_v1 {
    import oceanway_ali_origin
  }

  handle {
    respond 404
  }
}
```

### ali-vps origin 配置

`ali-vps` 的 `origin.oceanway.site` 建议只允许 `la-vps` 访问，避免用户绕过入口分流：

```caddy
origin.oceanway.site {
  encode zstd gzip

  @not_la_vps not remote_ip 64.188.30.215
  respond @not_la_vps 404

  reverse_proxy 127.0.0.1:8080
}
```

Caddy 修改后，在对应服务器执行：

```bash
caddy validate --config /etc/caddy/Caddyfile
systemctl reload caddy
```

## 应用后台配置

部署新镜像后，后台设置应保持：

```text
internal_home_domains = oceanway.site
site_name = OceanWay AI
api_base_url = https://ocean-way.top/v1
```

不要把 `api_base_url` 写成 `https://oceanway.site/v1`，因为 `oceanway.site` 不提供 API 服务。

旧的 `home_content` 不再作为新版主页渲染依据。即使数据库里仍保留该内容，也不要依赖它控制首页。

## Key 自动路由迁移

新版用户逻辑是“账号权益 + Key 凭证”：

- 新注册用户会自动获得一把 `Default Key`。
- 用户新建 Key 默认不绑定分组。
- 未绑定分组的 Key 在请求时自动按“订阅优先、额度兜底”选择可用分组。
- 后台分组仍用于账号池、模型路由、倍率、订阅限额和兜底资源池。

用户较少时，可以把普通 public 用户迁移到新逻辑。迁移命令只处理 `role=user` 且 `customer_type=retail` 的用户，不处理管理员和 managed 托管用户。

先在 `ali-vps` 的应用源码目录预览：

```bash
cd /opt/sub2api/source/backend
DATA_DIR=/opt/sub2api/data go run ./cmd/migrate-key-routing
```

确认输出中的数量符合预期后再执行：

```bash
DATA_DIR=/opt/sub2api/data go run ./cmd/migrate-key-routing --execute
```

迁移会做两件事：

- 给没有任何非删除 Key 的普通用户创建一把 `Default Key`，`group_id = null`。
- 清空普通用户现有非删除 Key 的 `group_id`，让这些 Key 统一走自动调度。

迁移前必须确认后台至少有一个可用的额度兜底分组：

```text
subscription_type = standard
status = active
is_exclusive = false
sort_order = 100 左右
```

如果不希望某些 standard 分组被普通用户自动兜底使用，应设置 `is_exclusive = true`，不要只依赖较大的 `sort_order`。

## 回滚

优先恢复服务可用性，再处理页面细节。

在 `ali-vps` 找到上一版镜像：

```bash
docker images --format '{{.Repository}}:{{.Tag}} {{.CreatedSince}}' | grep '^sub2api:'
```

修改 compose 回上一版：

```bash
cd /opt/sub2api
ROLLBACK_TAG=oceanway-上一版
cp docker-compose.yml docker-compose.yml.bak-rollback-$(date +%Y%m%d-%H%M%S)
perl -0pi -e "s|image: sub2api:[^\n]+|image: sub2api:${ROLLBACK_TAG}|" docker-compose.yml
docker compose up -d --no-build
docker compose ps
docker inspect sub2api --format 'Image={{.Config.Image}} Status={{.State.Status}} Health={{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}'
```

如果本次改过 Caddy，再在对应服务器恢复 Caddy 配置备份：

```bash
caddy validate --config /etc/caddy/Caddyfile
systemctl reload caddy
```

如果数据库设置改错，优先在后台恢复设置；严重时再恢复数据库备份。

## 常见错误

1. 在 `la-vps` 部署应用镜像
   当前真实应用跑在 `ali-vps`。只在 `la-vps` 加载新镜像不会改变线上页面或 API。

2. 把镜像 tag 当应用版本
   不要传 `--build-arg VERSION=${TAG}`。页面版本应来自 `backend/cmd/server/VERSION`。

3. 从脏工作区使用 `git archive HEAD`
   `git archive HEAD` 不包含未提交改动。需要先提交，或明确选择本地 Docker 从脏工作区构建。

4. 使用 `latest`
   不利于回滚和定位问题。生产镜像使用 `sub2api:oceanway-YYYYMMDD-HHMM`。

5. 让 `oceanway.site` 暴露 API 或登录
   `oceanway.site` 必须在 Caddy 层拦截 `/api/*`、`/v1/*` 和后台路径，不能只靠前端路由守卫。

6. 旧 IP 兼容入口放开过多路径
   `http://64.188.30.215:8080` 只允许 `/v1` 和 `/v1/*`，其他路径应返回 `404`。

7. 直接公开 origin
   `origin.oceanway.site` 应只允许 `la-vps` 访问，否则会绕过 `oceanway.site` 的限制。

8. 泄露 `.env`
   不要把 `/opt/sub2api/.env` 内容贴到聊天、日志或文档里。
