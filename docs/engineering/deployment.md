# NanaFox 当前部署与运维

最后核对：2026-11-15。本文是生产目标的唯一部署入口；历史日期文档、迁移前模板不能覆盖本文。运行前再次检查宿主机与 Docker 状态。

主机级配置（Caddyfile、compose 文件、备份脚本、监控）统一放在私有仓库 `nana-fox/nanafox-ops`，本仓库不再保留副本。改主机配置先改那里，再同步到服务器。

## 当前生产拓扑

唯一生产主机：TmhHost，登录用 `ssh tmhhost`，2 vCPU / 约 4 GiB 内存 + 2 GiB swap，40 GB 根盘，Docker 29.x、compose v5。2026-11-10 从阿里云新加坡旧主机迁来；旧主机不再运行 Router/Studio，只临时把旧 DNS 缓存的流量转发过来。

2026-11-19 起：生产主机公网 IP 从国内大陆直连不通，router、router2、studio 三个域名都开了 Cloudflare 代理（免费版：100 秒无响应断开，流式不受影响；单次上传 100MB）。`ssh tmhhost` 走 Tailscale；主机访问国内服务（微信支付等）经深圳机转发。详见私有仓库 `nana-fox/nanafox-ops` README。

| 服务 | 域名 | 容器与宿主机回环端口 | 数据/配置 |
|---|---|---|---|
| Router v0.2.20 | router.nanafox.com | nanafox-router-formal-router，127.0.0.1:18083 → 8080 | 卷 nanafox-router-formal_routerdata |
| Router PostgreSQL 18 | 仅内部网络 | nanafox-router-formal-postgres（网络别名 fx-production-postgres） | 卷 nanafox-router-formal_pgdata |
| Router Redis 8 | 仅内部网络 | nanafox-router-formal-redis（网络别名 fx-production-redis） | 卷 nanafox-router-formal_redisdata |
| Studio | studio.nanafox.com | nanafox-studio-formal-app，127.0.0.1:18790 → 8788 | 卷 nanafox-studio-formal-data-20261110T021929Z |
| Studio PostgreSQL 18 | 仅内部网络 | nanafox-studio-formal-postgres | 卷 nanafox-studio-formal-pg-20261110T021929Z |
| 网关 Caddy 2.11.2 | 上述两个域名 | nanafox-gateway，host 网络，占 80/443 | /srv/nanafox-gateway |
| 图像创作插件 | Router /tools/image-playground/ | 网关静态文件 + Router 内置 API | /srv/nanafox-gateway/site/image-playground/prod-current，release 8bba7e8 |

### Router

- 目录 `/srv/nanafox-router-formal/`，compose 文件 `compose.json`，项目名 `nanafox-router-formal`。文件里有相对路径挂载，**必须在该目录下执行 compose**。
- 应用配置 `config.original.json` 只读挂载为 `/app/data/config.yaml`；`redis.conf` 只读挂给 Redis。Router 网络 `nanafox-router-formal_database`（internal）+ `nanafox-router-formal_egress`。
- 资源：内存 1g、内存加 swap 1280m、1 CPU；环境变量 `GOMEMLIMIT=900MiB`、`GATEWAY_REQUEST_BODY_MEMORY_BUDGET_BYTES=268435456`（256 MiB）；日志 json-file 50m × 3。原因见 [2026-11-15 OOM 记录](../ops/2026-11-15-router-oom-memory-limits.md)。不要退回旧的 768m / 512 MiB 预算。
- 只重建 Router（不动 PG/Redis）：

  ```bash
  cd /srv/nanafox-router-formal && sudo docker compose -f compose.json up -d --no-deps router
  ```

### Studio

- 目录 `/srv/nanafox-studio-formal/`，项目名 `nanafox-studio-formal`，compose 文件 `compose.yaml` + `compose.normal-runtime.yaml`（后者覆盖启动命令，两份都要带上）。env 文件 `production.env`、`postgres.env` 在同目录，不要外传。
- `STUDIO_PUBLIC_ORIGIN=https://studio.nanafox.com`、`ROUTER_AUTH_BASE_URL=https://router.nanafox.com`。生图、支付保持开启。内存 512m、1 CPU、停止窗口 600 秒、no-new-privileges。
- 只重建 Studio：

  ```bash
  cd /srv/nanafox-studio-formal && sudo docker compose -f compose.yaml -f compose.normal-runtime.yaml up -d --no-deps studio
  ```

  Studio 配置连续性用 Studio 仓库 `deploy/check-config-continuity.mjs` 核验；停止前处理活动任务。

### 网关（Caddy）

- 容器 `nanafox-gateway`，镜像 `caddy:2.11.2-alpine`，host 网络，restart unless-stopped。
- `/srv/nanafox-gateway/` 下：`Caddyfile`（只读挂载）、`site/`（fx-web-assets、image-playground）、`data/`（ACME 证书，Caddy 自动续期）、`config/`、`logs/`（router.log、studio.log，自动滚动）。
- 改完 Caddyfile 后先校验再热加载，不停服：

  ```bash
  sudo docker exec nanafox-gateway caddy validate --config /etc/caddy/Caddyfile && sudo docker exec nanafox-gateway caddy reload --config /etc/caddy/Caddyfile
  ```

- Caddyfile 以 `nana-fox/nanafox-ops` 为准。本仓库原来的 `deploy/Caddyfile.*` 快照已删除，请勿从 Git 历史里拿旧文件覆盖线上。
- 旧临时网关 `nanafox-migration-caddy-stage` 已停止，只留作回退，清理前不要启动（会抢 80/443）。

### 主机上的其他残留

`/srv/` 下 `nanafox-router-prod`、`nanafox-studio-prod`、`nanafox-router-candidate`、`migration` 及对应已停止容器（`nanafox-router-prod-*`、`nanafox-studio-prod-*`、`nanafox-router-candidate-*`、`nanafox-migration-*`）是迁移过程材料，不是生产，也不是测试环境。不要启动、不要当回退目标，清理需单独确认。同机 `claude-us-*` 容器属其他业务，与 NanaFox 无关。

## 旧服务与测试边界

阿里云新加坡旧主机：2026-11-10 起不再运行 Router/Studio，只把旧 DNS 缓存的流量转发到新主机；新网关为此临时信任其转发头，旧机转发撤掉时一起删除。旧机上的库、镜像和回退容器（如 `fx-production-router-rollback-*`）只是历史材料，不能在旧机重启生产。

更早的旧主机（`sub2api-prod`、`sub2api-test`、`/etc/sub2api/test.yaml`、回环 8081、`router-test.nanafox.com`）已退役，不是发布目标。当前没有 Router/Studio 测试环境。不要因合并 main 自动部署，也不要擅自在生产主机建测试栈。

## 应用更新流程

1. 明确本次授权的项目、版本，固定 release 提交与候选镜像 digest，按 Git 工作流验证候选。没有测试环境时只报告本地验证缺口。
2. 在生产主机对运行态做只读预检（脚本从当前受审阅版本经 SSH stdin 运行，需 sudo 读 Docker）：

   ```bash
   ssh tmhhost 'sudo -n python3 -' < deploy/check-router-production.py
   ```

   错主机、错挂载、旧卷、网络、端口或 Router 资源限制不匹配即失败；通过只证明目标匹配，不等于应用验收或备份通过。
3. 私有保存当前 `docker inspect`、image ID、compose 文件和 Caddyfile；核对最近备份时间。不要把 env、密钥或 inspect 全量输出到终端/仓库。
4. 把新镜像 load 到主机并打 tag，修改对应 compose 文件里的 `image`（改动同步提交到 `nana-fox/nanafox-ops`），再用上面的命令只重建目标应用。不重建 PostgreSQL/Redis，不执行 `compose down -v`。保留上一镜像以便回退。
5. 不在生产主机 `git pull` 构建。**旧 `bash deploy/deploy-server.sh prod` 已禁用**。
6. 检查下列验收项；失败先保全新数据，再回退。
7. 验证通过后按发布分支流程合并 main，记录运行的 image ID 与源码提交。仅文档/运维规则更新不重建应用。

### 回退

- 应用回退：把 compose 文件里的 `image` 改回上一个 image ID，再执行同一条只重建命令。当前 Router image ID `sha256:183fac4972a13a6784f38a643d97486f43daa3a4fb695b0b7fde844d220fdb69`（tag `sub2api:prod-v0.2.20-a9367cc4c`），Studio `sha256:ce5d240d3cfa69a07272ba005b8817255ecbf730746569133067c567bc482f4a`。新主机上目前没有 v0.2.17 镜像；需要退到更旧版本时先从备份或旧机导出镜像。
- 只回退 compose 配置：2026-11-15 改限制前的原文件私有保存在 `/srv/nanafox-ops/window-20261115/compose.json.orig`。
- 网关回退：先 `caddy validate` 再 `reload`；失败时恢复上一份 Caddyfile 再 reload。
- 数据库：迁移后已有新写入，不能用旧快照覆盖生产库。v0.2.20 回退边界（新管理密钥/MCP 授权、按张计费、邮箱黑名单产生后禁止直接退旧镜像）见 [v0.2.20 发布记录](../ops/2026-11-08-router-v0.2.20-deployment.md)。

## 验收与真实流量证明

- Router `/health`、首页、插件静态资源、用户/管理员插件 API；未签名 `POST /internal/v1/studio-auth/login` 应返回 401，404 表示 trusted adapter 配置缺失。
- Studio `/api/health`、`/api/ready`、正式深链、带正式 Origin 的认证请求、可信 Router 签名链路与 R2 文件读取。
- 核对账号、用户、分组、订单、余额、模板及关键配置；区分 API 可见计数和包含软删除记录的物理表行数。
- 模型调用用确实支持的模型；支付配置读取、下单与真实扣款分别记录。真实付款需要具体交易授权。未执行的生成/交互登录不能写成通过。
- 以唯一 query 标记请求正式域名，在 `/srv/nanafox-gateway/logs/` 查到同一标记，证明是新主机处理；旧 DNS 缓存的请求可能经旧机转发。单看 HTTP 200 不证明。
- 观察内存（含 OOM 次数：`sudo docker inspect nanafox-router-formal-router --format '{{.State.OOMKilled}} {{.RestartCount}}'` 及内核日志）、磁盘、错误和备份。

## 备份

- NAS 每天北京时间 03:35 拉取完整备份（两生产库、角色、配置、容器参数、证书和静态产物），03:45 单独拉取 Studio。NAS 用固定命令（forced command）的 SSH key 登录 `tmhhost`，只能执行导出，不能开 shell。
- 主机上的导出脚本：`/srv/nanafox-ops/backup/export-backup.py`（源码在 `nana-fox/nanafox-ops`）。
- MinIO bucket `nanafox-postgres-backups`：完整备份 `fx-production/<UTC时间>/`，Studio `studio-production/<UTC时间>/`。只有全部上传并回读一致后才写 `last-success.json`；检查其时间和对象，不能用 cron 存在代替备份成功。
- 不包含：Redis AOF、Docker 镜像层、Studio /data 卷、R2 全量对象。账号以 PG 为准。
- 本仓库 `deploy/backup/` 是旧主机时期的版本，路径和容器名已过期，仅作历史参考。

## 后续核对项（不能标为已完成）

- 线上 Caddyfile 尚未设置 `max_header_size 64KB` / `read_header 10s`；Studio 站点用的是 `encode zstd gzip` 简写，静态资源仍是 immutable 缓存。按 [EDGE_SECURITY](../../deploy/EDGE_SECURITY.md) 在 `nana-fox/nanafox-ops` 里修改并单独验证流式请求。
- 完整异机恢复演练未完成；Redis AOF、Studio /data、R2 对象没有独立恢复点。
- 旧机转发撤掉后，删除网关里对旧机的 trusted_proxies。
- 清理迁移残留容器、卷和 `nanafox-migration-caddy-stage`（单独确认）。
- Landing（nanafox.com）不在当前网关，部署位置另行核对。

## 历史版本记录（不是当前状态）

- 2026-11-10 迁到 TmhHost，Router/Studio 镜像与迁移前一致。
- 2026-11-08 v0.2.20：源码 `a9367cc4c45d4e59bc97e31ea400f103cc7ebc75`，镜像 tag `sub2api:prod-v0.2.20-a9367cc4c`，迁移 328 项（至 273）；在旧主机容器 `fx-production-router`（回环 18080）上发布，Studio 当时为 `fx-production-studio`（18789）。见 [v0.2.20 发布记录](../ops/2026-11-08-router-v0.2.20-deployment.md)。
- 2026-10-18 v0.2.17：源码 `9d4bed3da5e1805c473b2e1abf64deee17123d6f`、镜像 `sha256:e6df74f02efd3fb4b0f4adb9794ca84ef92314bdc945c5e8981d476f0017ecbe`，见 [v0.2.17 发布记录](../ops/2026-10-18-router-v0.2.17-deployment.md)。
- 2026-10-12 v0.2.14 snapshot1 热修：源码 `d32154752f70e07c1ddd7a738a8e5ddf5e5514a4`，见 [snapshot1 发布记录](../ops/2026-10-12-ops-snapshot-guard-deployment.md)、[延迟与 503 恢复记录](../ops/2026-10-12-router-latency-recovery.md)。
- 2026-10-08 v0.2.14，见 [v0.2.14 发布记录](../plans/2026-10-08-upstream-v0.2.14-execution.md)。
- 2026-09-06/07 从更早的旧主机迁到阿里云新加坡，见 [迁移记录](../plans/2026-09-07-router-production-migration.md)。

更早的旧流程通过 Git 历史查阅，不复制回当前操作入口。
