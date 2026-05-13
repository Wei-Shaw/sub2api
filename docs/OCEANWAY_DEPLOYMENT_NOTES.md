# OceanWay 双域名部署注意事项

本文用于部署 `ocean-way.top` 与 `oceanway.site` 的域名分流版本。核心原则是：`ocean-way.top` 是完整 sub2api 服务入口，`oceanway.site` 是面向非技术用户的静态引流站点，只展示主页、文档和 Agents Hub，不提供登录、控制台、管理后台或 API 服务。

## 目标状态

| 域名 | 角色 | 默认页面 | 允许页面 | 不应开放 |
| --- | --- | --- | --- | --- |
| `ocean-way.top` | 完整 public 服务域名 | `/home` | sub2api 全功能 | 无特殊限制 |
| `oceanway.site` | 引流静态站域名 | `/internal-home` | `/`, `/internal-home`, `/docs`, `/agents` | `/login`, `/dashboard`, `/admin/*`, `/keys`, `/usage`, `/api/*`, `/v1/*` |

注意：`oceanway.site` 不能只靠前端路由限制。前端守卫只是用户体验层，真正边界必须在 Caddy 或其他反向代理层拦截。

## 当前线上拓扑

当前不是“应用直接跑在 la-vps”的单机部署，而是双服务器入口代理部署：

| 节点 | 角色 | 当前职责 |
| --- | --- | --- |
| `la-vps` / `64.188.30.215` | 入口服务器 | DNS 指向这里；运行 Caddy；负责双域名分流、旧 IP `/v1` 兼容入口、反代到 ali origin |
| `ali-vps` / `43.112.125.18` | 主业务服务器 | 运行 `sub2api`、PostgreSQL、Redis；承载真实业务流量 |
| `origin.oceanway.site` | ali origin 域名 | 只给 `la-vps` 反代访问；不作为用户公开入口 |

当前入口关系：

```text
用户 -> ocean-way.top / oceanway.site / 64.188.30.215:8080
     -> la-vps Caddy
     -> https://origin.oceanway.site
     -> ali-vps sub2api
```

因此：

- 应用镜像更新部署到 `ali-vps` 的 `/opt/sub2api`。
- `la-vps` 通常只改 Caddy 配置，不加载应用镜像。
- `la-vps` 上的 sub2api 应用容器不作为当前主服务使用；如果保留数据库/Redis，也只是回滚或备份参考。

## 部署前检查

1. 确认 DNS：
   - `ocean-way.top` 的 A 记录应指向 la-vps：`64.188.30.215`
   - `oceanway.site` 的 A 记录应指向 la-vps：`64.188.30.215`
   - DNS 未生效前，不要急着 reload Caddy 申请证书。

2. 确认当前 Caddy 入口：
   - 不要直接改 `/etc/caddy/Caddyfile` 主文件。
   - 应编辑 `/etc/caddy/sites/*.conf`。
   - 当前线上应由 la-vps 反代到 `https://origin.oceanway.site`，不要改成反代本机 `127.0.0.1:8080`，除非你明确要回退为 la-vps 单机部署。
   - 反代到 origin 时需要把 `Host` 设置为 `origin.oceanway.site`，同时保留原始访问域名到 `X-Forwarded-Host`。

3. 确认 Docker 端口：
   - `ali-vps` 上的 sub2api 容器由本机 Caddy/origin 接入，不需要直接暴露给公网用户。
   - `la-vps` 的 80/443 是公开入口；`64.188.30.215:8080` 只作为旧用户 `/v1` 兼容入口。
   - 不要让 `la-vps:8080` 直接暴露完整 sub2api 页面或 `/api/*`。

4. 备份：
   - 在 `ali-vps` 上先备份 `/opt/sub2api`。
   - 在 `ali-vps` 上先备份 PostgreSQL 数据。
   - 不要把 `.env` 内容贴到聊天或日志里。

## 应用配置

部署新镜像后，需要在后台设置中配置：

```text
internal_home_domains = oceanway.site
site_name = OceanWay AI
api_base_url = https://ocean-way.top/v1
```

`api_base_url` 不建议继续写 `https://oceanway.site/v1`，因为 `oceanway.site` 不再提供 API 服务。否则 `/docs` 页面会展示错误的 API Base URL。

旧的 `home_content` 不再是主页来源。保留在数据库里也不会作为新版主页渲染依据，但后续可以清理，避免误判。

## Caddy 建议配置

以下是当前双服务器形态的建议配置。部署时按服务器现有文件拆分调整。

### la-vps 入口配置

`la-vps` 负责公网入口和域名分流，反代目标是 `origin.oceanway.site`：

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

# Optional: legacy API compatibility for existing users still using
# http://64.188.30.215:8080/v1 as Base API. Keep this temporary.
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

`ali-vps` 提供 origin 站点，建议只允许 `la-vps` 访问，避免用户绕过入口分流：

```caddy
origin.oceanway.site {
  encode zstd gzip

  @not_la_vps not remote_ip 64.188.30.215
  respond @not_la_vps 404

  reverse_proxy 127.0.0.1:8080
}
```

说明：

- `/assets/*` 不要拦截，否则前端静态资源会加载失败。
- `/docs` 与 `/agents` 需要保留。
- `/api/*` 与 `/v1/*` 建议返回 `404`，不要重定向到 HTML 页面。
- 旧 IP 兼容块只允许 `/v1` 与 `/v1/*`，不要让 IP 入口访问 `/login`、`/home`、`/dashboard` 或 `/api/*`。
- 在当前双服务器方案中，`la-vps:8080` 由 Caddy 监听用于旧 API 兼容；`ali-vps:8080` 是应用容器本机端口。
- 如果还保留 `ai.oceanway.site`，需要单独决定它是完整服务域名还是引流域名，不要默认跟 `oceanway.site` 混在一起。

修改后执行：

```bash
caddy validate --config /etc/caddy/Caddyfile
systemctl restart caddy
```

## Docker 部署注意

应用镜像部署目标是 `ali-vps`，不是 `la-vps`。推荐使用固定镜像 tag，不要继续依赖 `latest`：

```text
image: sub2api:oceanway-YYYYMMDD-HHMM
```

本地构建并上传到 `ali-vps`：

```bash
TAG=oceanway-YYYYMMDD-HHMM
docker build -t sub2api:${TAG} .
docker save sub2api:${TAG} | gzip > sub2api-${TAG}.tar.gz
scp sub2api-${TAG}.tar.gz ali-vps:/root/
```

如果本机没有 Docker CLI，使用源码包上传到 `ali-vps` 后远端构建：

```bash
TAG=oceanway-YYYYMMDD-HHMM
COMMIT=$(git rev-parse --short HEAD)
git archive --format=tar HEAD | gzip > /tmp/sub2api-source-${TAG}-${COMMIT}.tar.gz
scp /tmp/sub2api-source-${TAG}-${COMMIT}.tar.gz ali-vps:/root/
ssh ali-vps
mkdir -p /root/sub2api-backups/pre-deploy-${TAG}
cp -a /opt/sub2api/docker-compose.yml /root/sub2api-backups/pre-deploy-${TAG}/docker-compose.yml
cp -a /opt/sub2api/source /root/sub2api-backups/pre-deploy-${TAG}/source
rm -rf /opt/sub2api/source
mkdir -p /opt/sub2api/source
tar -xzf /root/sub2api-source-${TAG}-${COMMIT}.tar.gz -C /opt/sub2api/source
cd /opt/sub2api/source
docker build -t sub2api:${TAG} --build-arg COMMIT=${COMMIT} .
```

注意：`TAG` 只是镜像部署标记，不要传给 `--build-arg VERSION`。应用版本默认来自 `backend/cmd/server/VERSION`，这样页面左上角会显示上游同步后的版本号，例如 `v0.1.126`。

在 `ali-vps` 上加载镜像并重启应用：

```bash
ssh ali-vps
TAG=oceanway-YYYYMMDD-HHMM
gunzip -c /root/sub2api-${TAG}.tar.gz | docker load
cd /opt/sub2api

# 修改 docker-compose.yml 中的 image：
# image: sub2api:oceanway-YYYYMMDD-HHMM

docker compose up -d --no-build
docker compose ps
docker logs --tail=100 sub2api
```

`ali-vps` 的 compose 可以保留应用本机端口给 origin Caddy 使用。推荐不要把应用端口作为公开用户入口；公网入口仍走 `la-vps`。

如果需要明确绑定本机，可使用：

```yaml
ports:
  - "127.0.0.1:8080:8080"
```

`la-vps` 只有在修改入口分流、旧 IP 兼容、域名拦截规则时才需要 reload Caddy；普通前后端代码更新不需要在 `la-vps` 加载新镜像。

## 验证清单

DNS：

```bash
dig +short ocean-way.top
dig +short oceanway.site
```

完整服务域名：

```bash
curl -I https://ocean-way.top/
curl -I https://ocean-way.top/home
curl -I https://ocean-way.top/login
curl -I https://ocean-way.top/api/v1/settings/public
curl -I https://ocean-way.top/v1/models
```

引流静态站域名：

```bash
curl -I https://oceanway.site/
curl -I https://oceanway.site/internal-home
curl -I https://oceanway.site/docs
curl -I https://oceanway.site/agents
curl -I https://oceanway.site/login
curl -I https://oceanway.site/dashboard
curl -I https://oceanway.site/admin
curl -I https://oceanway.site/api/v1/settings/public
curl -I https://oceanway.site/v1/models
```

旧 IP 兼容入口，如果仍有用户未迁移：

```bash
curl -I http://64.188.30.215:8080/v1/models
curl -I http://64.188.30.215:8080/login
curl -I http://64.188.30.215:8080/home
curl -I http://64.188.30.215:8080/api/v1/settings/public
```

预期结果：

- `https://ocean-way.top/` 打开 public 首页。
- `https://oceanway.site/` 打开 internal 引流首页。
- `https://oceanway.site/docs` 可访问。
- `https://oceanway.site/agents` 可访问。
- `https://oceanway.site/login` 应 `302` 到 `/internal-home`。
- `https://oceanway.site/api/*` 与 `/v1/*` 应返回 `404`，不能返回 sub2api API 数据。
- `http://64.188.30.215:8080/v1/*` 可以进入后端鉴权，未带 Key 时通常返回 `401`。
- `http://64.188.30.215:8080/login`、`/home` 与 `/api/*` 应返回 `404`。
- 直接访问 `https://origin.oceanway.site/health` 如果不是从 `la-vps` 发起，应返回 `404` 或被拒绝。

浏览器人工检查：

- `ocean-way.top` 顶部允许进入控制台。
- `oceanway.site` 顶部不出现登录或控制台入口。
- `oceanway.site` 上点击“文档”仍留在 `oceanway.site/docs`。
- `oceanway.site` 上点击 “Agents Hub” 仍留在 `oceanway.site/agents`。
- `oceanway.site/agents` 的“联系”入口回到 `/internal-home#contact`。

## 常见风险

1. `ocean-way.top` DNS 没指向 la-vps
   Caddy 证书申请失败，页面打不开。

2. 只在 la-vps 更新应用镜像
   当前应用实际跑在 ali-vps，la-vps 只是入口代理。只在 la-vps 加载新镜像不会改变线上页面或后端行为。应用镜像必须部署到 ali-vps。

3. 8080 暴露到公网
   当前 `la-vps:8080` 是 Caddy 的旧 API 兼容入口，只能开放 `/v1/*`。如果它能打开 `/login`、`/home` 或 `/api/*`，说明绕过限制了。

4. `api_base_url` 配错
   如果写成 `https://oceanway.site/v1`，文档会引导用户使用一个被你刻意关闭的 API 域名。应写 `https://ocean-way.top/v1`。

5. 只做前端限制
   前端可以阻止普通用户误点，但不能作为安全隔离。Caddy 必须拦 `/api/*`、`/v1/*` 和应用后台路径。

6. 使用 `latest` 镜像
   回滚和定位问题困难。建议使用带日期的固定 tag。

7. origin 被公网直接访问
   `origin.oceanway.site` 应只允许 `la-vps` 访问。否则用户可能绕过 `oceanway.site` 的静态站限制，直接访问完整服务。

## 回滚方案

1. 在 `ali-vps` 恢复 compose 中的旧镜像 tag。
2. 在 `ali-vps` 执行 `docker compose up -d`。
3. 如果本次改过入口配置，再在 `la-vps` 恢复 Caddy 站点配置备份。
4. 在对应服务器执行 `caddy validate --config /etc/caddy/Caddyfile`。
5. 在对应服务器执行 `systemctl reload caddy`。
6. 如果数据库设置改错，恢复备份或把 `internal_home_domains` 清空。

回滚时优先恢复服务可用性，再处理页面细节。
