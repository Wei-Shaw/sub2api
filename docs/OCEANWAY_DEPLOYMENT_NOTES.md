# OceanWay 双域名部署注意事项

本文用于部署 `ocean-way.top` 与 `oceanway.site` 的域名分流版本。核心原则是：`ocean-way.top` 是完整 sub2api 服务入口，`oceanway.site` 是面向非技术用户的静态引流站点，只展示主页、文档和 Agents Hub，不提供登录、控制台、管理后台或 API 服务。

## 目标状态

| 域名 | 角色 | 默认页面 | 允许页面 | 不应开放 |
| --- | --- | --- | --- | --- |
| `ocean-way.top` | 完整 public 服务域名 | `/home` | sub2api 全功能 | 无特殊限制 |
| `oceanway.site` | 引流静态站域名 | `/internal-home` | `/`, `/internal-home`, `/docs`, `/agents` | `/login`, `/dashboard`, `/admin/*`, `/keys`, `/usage`, `/api/*`, `/v1/*` |

注意：`oceanway.site` 不能只靠前端路由限制。前端守卫只是用户体验层，真正边界必须在 Caddy 或其他反向代理层拦截。

## 部署前检查

1. 确认 DNS：
   - `ocean-way.top` 的 A 记录应指向 la-vps：`64.188.30.215`
   - `oceanway.site` 的 A 记录应指向 la-vps：`64.188.30.215`
   - DNS 未生效前，不要急着 reload Caddy 申请证书。

2. 确认当前 Caddy 入口：
   - 不要直接改 `/etc/caddy/Caddyfile` 主文件。
   - 应编辑 `/etc/caddy/sites/*.conf`。
   - 当前线上曾出现 `oceanway.site` 反代到 `origin.oceanway.site` 的配置；如果本次部署要直接跑在 la-vps，必须改成反代本机 `127.0.0.1:8080`。

3. 确认 Docker 端口：
   - 推荐把 compose 中的服务端口从 `0.0.0.0:8080:8080` 改成 `127.0.0.1:8080:8080`。
   - 对外只暴露 Caddy 的 80/443，不让公网直接访问容器 8080。

4. 备份：
   - 先备份 `/root/sub2api-deploy`。
   - 先备份 PostgreSQL 数据。
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

以下是建议形态，部署时按服务器现有文件拆分调整：

```caddy
(oceanway_headers) {
  header {
    Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
    X-Content-Type-Options "nosniff"
    Referrer-Policy "strict-origin-when-cross-origin"
  }
}

ocean-way.top {
  encode zstd gzip
  reverse_proxy 127.0.0.1:8080
  import oceanway_headers
}

oceanway.site {
  encode zstd gzip

  @blocked_api path /api /api/* /v1 /v1/*
  respond @blocked_api 404

  @blocked_app path /home /login /register /email-verify /forgot-password /reset-password /dashboard /dashboard/* /admin /admin/* /keys /keys/* /usage /usage/* /subscriptions /subscriptions/* /redeem /redeem/* /purchase /purchase/* /payment /payment/* /auth /auth/* /setup
  redir @blocked_app /internal-home 302

  reverse_proxy 127.0.0.1:8080
  import oceanway_headers
}

# Optional: legacy API compatibility for existing users still using
# http://64.188.30.215:8080/v1 as Base API. Keep this temporary.
http://64.188.30.215:8080 {
  bind 64.188.30.215
  encode zstd gzip

  @legacy_v1 path /v1 /v1/*
  handle @legacy_v1 {
    reverse_proxy 127.0.0.1:8080
  }

  handle {
    respond 404
  }
}
```

说明：

- `/assets/*` 不要拦截，否则前端静态资源会加载失败。
- `/docs` 与 `/agents` 需要保留。
- `/api/*` 与 `/v1/*` 建议返回 `404`，不要重定向到 HTML 页面。
- 旧 IP 兼容块只允许 `/v1` 与 `/v1/*`，不要让 IP 入口访问 `/login`、`/home`、`/dashboard` 或 `/api/*`。
- 如果 Caddy 与 Docker 同时涉及 8080，必须让 Docker 只监听 `127.0.0.1:8080`，并在旧 IP 块中使用 `bind 64.188.30.215`，否则 Caddy 会因为 `:8080` 端口冲突启动失败。
- 如果还保留 `ai.oceanway.site`，需要单独决定它是完整服务域名还是引流域名，不要默认跟 `oceanway.site` 混在一起。

修改后执行：

```bash
caddy validate --config /etc/caddy/Caddyfile
systemctl restart caddy
```

## Docker 部署注意

推荐使用固定镜像 tag，不要继续依赖 `latest`：

```text
image: sub2api:oceanway-YYYYMMDD
```

如果从本地构建并传到服务器，流程一般是：

```bash
docker build -t sub2api:oceanway-YYYYMMDD .
docker save sub2api:oceanway-YYYYMMDD | gzip > sub2api-oceanway-YYYYMMDD.tar.gz
scp sub2api-oceanway-YYYYMMDD.tar.gz root@64.188.30.215:/root/
```

服务器上：

```bash
gunzip -c /root/sub2api-oceanway-YYYYMMDD.tar.gz | docker load
cd /root/sub2api-deploy
docker compose up -d
docker compose ps
docker logs --tail=100 sub2api
```

如果 compose 文件还暴露公网端口，建议改为：

```yaml
ports:
  - "127.0.0.1:8080:8080"
```

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

浏览器人工检查：

- `ocean-way.top` 顶部允许进入控制台。
- `oceanway.site` 顶部不出现登录或控制台入口。
- `oceanway.site` 上点击“文档”仍留在 `oceanway.site/docs`。
- `oceanway.site` 上点击 “Agents Hub” 仍留在 `oceanway.site/agents`。
- `oceanway.site/agents` 的“联系”入口回到 `/internal-home#contact`。

## 常见风险

1. `ocean-way.top` DNS 没指向 la-vps
   Caddy 证书申请失败，页面打不开。

2. `oceanway.site` 还反代到 `origin.oceanway.site`
   你在 la-vps 更新镜像不会生效，看到的仍然是 origin 服务器内容。

3. 8080 暴露到公网
   如果 Docker 直接暴露 `0.0.0.0:8080`，用户可以绕过 Caddy 访问 `http://64.188.30.215:8080/login` 或后台 API。Docker 应绑定到 `127.0.0.1`。如需兼容旧 API 用户，只让 Caddy 监听 `64.188.30.215:8080`，并仅代理 `/v1/*`。

4. `api_base_url` 配错
   如果写成 `https://oceanway.site/v1`，文档会引导用户使用一个被你刻意关闭的 API 域名。应写 `https://ocean-way.top/v1`。

5. 只做前端限制
   前端可以阻止普通用户误点，但不能作为安全隔离。Caddy 必须拦 `/api/*`、`/v1/*` 和应用后台路径。

6. 使用 `latest` 镜像
   回滚和定位问题困难。建议使用带日期的固定 tag。

## 回滚方案

1. 恢复 compose 中的旧镜像 tag。
2. `docker compose up -d`。
3. 恢复 Caddy 站点配置备份。
4. `caddy validate --config /etc/caddy/Caddyfile`。
5. `systemctl reload caddy`。
6. 如果数据库设置改错，恢复备份或把 `internal_home_domains` 清空。

回滚时优先恢复服务可用性，再处理页面细节。
