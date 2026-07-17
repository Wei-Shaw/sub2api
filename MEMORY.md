# 项目记忆

## 基础信息

- 项目：sub2api
- 记录日期：2026-07-11
- 代码结构：`backend/` 为 Go 服务端，`frontend/` 为 TypeScript/Vite 前端，前端使用 pnpm 锁文件。
- 远端：`origin` 指向自有 Fork `https://github.com/GitClound/sub2api.git`；`upstream` 指向开源作者仓库 `https://github.com/Wei-Shaw/sub2api`。

## 开发偏好

- 新增功能优先通过新建文件或独立模块实现，尽量减少对开源 `sub2api` 原有代码的修改，以降低后续功能集成和同步上游升级时的冲突与维护成本。

## 会话记录

- 2026-07-11：项目原本缺少 `AGENTS.md` 与 `MEMORY.md`，因用户确认而在根目录创建基础版本。
- 2026-07-11：本地 Docker 部署容器名为 `sub2api`、`sub2api-postgres`、`sub2api-redis`；三者健康。当天 `/responses` 的主要失败来自 OpenAI 上游账号返回 502/503/524，不是本地 Docker、PostgreSQL 或 Redis 服务不可用。
- 2026-07-11：排查上游失败时，可从 PostgreSQL 的 `ops_error_logs.upstream_errors` 读取脱敏的 `upstream_request_id`、上游账号及错误详情，供上游服务商按时间和请求 ID 检索。
- 2026-07-11：`ahriapi.eu.cc` 的 OpenAI API Key 账号在宿主机可读取 `/v1/models`，但 Docker 内 Go/curl 的 TLS 握手会被上游以 EOF 中断；账号 10 的 Key 已被上游禁用，账号 11 的 Key 和模型端点有效。
- 2026-07-11：应用当前未配置可用代理；Git 远端为 `https://github.com/Wei-Shaw/sub2api`，本机拉取时经配置代理或直连均发生 TLS/连接重置，需先恢复 GitHub 网络链路后同步最新代码。
- 2026-07-11：模型同步对 OpenAI API Key 账号在标准请求返回 TLS EOF 时，使用 `req/v3` 的 Chrome 指纹重试一次；单元测试与读取 Docker PostgreSQL 账号 11 凭据的显式集成测试均已验证成功。
- 2026-07-11：隔离测试实例使用 `sub2api_test` 数据库、独立 `sub2api-test-redis` 容器和 `http://localhost:18081`；`sub2api-test` 为修复版验证容器，未替换原 `sub2api` 服务。
- 2026-07-11：OpenAI 调度先按请求分组形成候选池，再按账号优先级升序选择（数值越小越优先）；当前 `codex5.4-5.6`（组 5）未包含 DragonAPI（账号 4），仅包含考拉API（账号 8）等账号，因此该组的 `gpt-5.6-terra` 请求不会调度到 DragonAPI。
- 2026-07-11：Git 双远端已配置：自有 Fork 为 `origin`，开源作者仓库为 `upstream`。同步上游前先执行 `git fetch upstream --prune` 并审查差异；禁止直接向 `upstream` 推送。
- 2026-07-11：TLS EOF 模型同步修复已提交并推送至自有 Fork 分支 `codex/fix-openai-model-sync-tls-eof`，提交为 `2fd0464e`；可据此创建指向 Fork 主分支或开源 `upstream/main` 的 PR。
- 2026-07-11：已向开源作者仓库提交 PR [#4065](https://github.com/Wei-Shaw/sub2api/pull/4065)，目标为 `Wei-Shaw/sub2api:main`，来源为 `GitClound:codex/fix-openai-model-sync-tls-eof`；当前为 Open 状态。
- 2026-07-11：隔离实例账号 11 的 `gpt-5.6-luna`、`gpt-5.6-terra`、`gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`、`gpt-5.4-mini` 经宿主机直连 `/v1/responses` 均返回 HTTP 200；管理页测试 `gpt-5.6-sol` 则返回 `unexpected EOF`，进一步确认模型与 Key 可用，故障位于 Docker 内访问 `ahriapi.eu.cc` 的 TLS 链路。
- 2026-07-11：提交 `2fd0464e` 的 Chrome TLS 回退仅覆盖 OpenAI 模型同步 `/v1/models`，账号连接测试和真实 `/v1/responses` 转发仍走 `HTTPUpstream.DoWithTLS`；现有账号 TLS Profile 功能又仅允许 Anthropic OAuth/SetupToken，OpenAI API Key 无法靠配置启用，因此要么绑定可用代理，要么在 OpenAI API Key 测试与网关转发链路增加 Chrome 指纹传输策略。
- 2026-07-12：Docker Desktop 正在运行的数据已通过 C: Junction 指向 `E:\DockerDesktopData\disk` 与 `E:\DockerDesktopData\main`；经 boss老板明确确认，C: 的 `disk.local-backup-20260711-212159`、`main.local-backup-20260711-212159` 已删除并释放约 10.6 GiB。Junction 与 `E:\DockerDesktopMigrationBackup` 离线备份仍保留。
- 2026-07-12：`deploy/test-data/` 是隔离测试产生的本地运行目录，包含测试二进制、配置和日志，不纳入 Git 提交。
- 2026-07-12：当前 `http://localhost:18081` 的 `sub2api-test` 实际连接 `sub2api-test-postgres` 中的 `sub2api` 数据库（应用环境 `DATABASE_DBNAME=sub2api`），不是旧记录中的 `sub2api_test`。
- 2026-07-12：组 5 当前已包含 DragonAPI 账号 4/12 和 ahriapi 账号 11；账号优先级数值越小越优先，但高优先账号并发槽满时新会话会下沉，随后约 1 小时的 OpenAI sticky session / `previous_response_id` 绑定会让同一任务继续使用原账号。因此持续命中 ahriapi 不代表 Dragon 优先级失效。
- 2026-07-12：Docker 实际端口为旧 `deploy/sub2api` 使用 `http://localhost:8080`、隔离 `sub2api-test` 使用 `http://localhost:18081`；两组三容器均为 `restart=unless-stopped`。测试组三容器当前仍连接 `deploy_sub2api-network`，移除旧 Compose 项目前应先迁到独立网络，避免测试实例继续依赖旧项目网络。
- 2026-07-13：已将 `sub2api-test` 验证版正式迁移到 `http://localhost:8080`：镜像标记为 `sub2api:production`，正式容器恢复为 `sub2api`、`sub2api-postgres`、`sub2api-redis`，均健康且使用 `restart=unless-stopped`；当前数据已迁入 `deploy/data`、`deploy/postgres_data`、`deploy/redis_data`。迁移备份及旧正式数据位于 `E:\DockerTestReplica\promotion-backups\20260713-142817`；77 张数据库表逐表核对，仅正式应用启动后日志与指标表各新增 1 行。旧 `sub2api-test*` 容器保持停止作为回滚点，`18081` 当前下线。
- 2026-07-13：旧测试三容器已重建为 Docker Compose 项目 `test`，配置位于 `E:\DockerTestReplica\compose\docker-compose.yml`，凭据仍仅保存在同目录的隐藏 env 文件中；测试组使用独立网络 `test_sub2api-test-network`，三个服务均设置 `restart: "no"` 并在验证 `http://localhost:18081`、PostgreSQL、Redis 后保持停止。Docker Desktop 当前显示运行中的 `deploy` 正式组和已停止的 `test` 测试组，正式 `http://localhost:8080` 不受影响。原容器配置备份位于 `E:\DockerTestReplica\container-config-backups\20260713-151802`。
- 2026-07-15：正式 `sub2api`、PostgreSQL、Redis 均健康且资源占用正常；`sub2api` 的历史 `RestartCount=7` 来自 Docker/PostgreSQL 同步启动时应用先遇到 `connection refused` / `database system is starting up` 后自动恢复，并非 OOM 或持续崩溃。当前网关失败主因是账号 24（`api.lajiang.xyz`）集中返回 `503 Service temporarily unavailable`；同时 Dragon 账号 4/12 会以 `500 sensitive_words_detected` 拒绝部分请求，aiapibank 账号存在分组权限 `403`、`unexpected EOF` 和少量 `502/503`。故障集中时可调度账号不足会触发 `no available accounts` 并向客户端返回 `502`，随后账号 25/26 已持续成功处理请求。
- 2026-07-15：boss老板提出新增独立的上游中转站管理能力，参考 `worryzyy/upstream-hub`、`bejix/upstream-ops` 与 New API 接口；目标是在尽量隔离现有代码的前提下，按平台和模型同步多个上游的分组、倍率与可用性，以充值折算倍率 `K`（默认 1）计算有效倍率 `P=X/K`，优先调度最低可用倍率并在失败时自动下沉。初步方向是复用现有账号的模型过滤、优先级、健康状态和失败切换，由新增模块管理上游凭据、倍率快照及其与受管账号的映射。
- 2026-07-15：boss老板确认上游资源池按调用协议分类、模型品牌筛选；允许模块仅管理 `sub2api-auto-*` 上游 Key；仅有 API Key 时使用固定线路；最低层满载等待最多 2 秒后下沉；倍率回切只影响新会话；首期使用独立的 OpenAI/Claude/Gemini/Grok 自动分组。实现需以新增文件为主，参考 Upstream Hub/UpstreamOps 的优质设计并减少后续合并上游 sub2api 的冲突。
- 2026-07-15：上游中转站 V1 已以 `backend/internal/upstreamstation/` 独立模块和迁移 `174_upstream_station_pool.sql` 落地；受管账号用 `managed_by=upstream_station_pool`、站点/线路 ID、有效倍率和远程分组等 `extra` 字段隔离。服务启动 30 秒后首次同步，之后每 5 分钟同步一次。
- 2026-07-15：成本调度仅在候选池全部为上游资源池受管账号时启用，按 `P=X/K` 分层，最低层满载等待最多 2 秒后下沉；手工账号混入时保持原调度。sticky session 和 `previous_response_id` 先于成本选择，线路失败时才换线。
- 2026-07-15：模型发现失败会保留旧模型/倍率快照并让对应受管账号退出调度；远程分组消失会标错，恢复后自动恢复健康。人工 `schedulable` 开关与健康状态分离，同步恢复不会重新打开人工关闭的线路。
- 2026-07-15：NewAPI 创建 `sub2api-auto-*` Key 时，`POST /api/token/` 可能不返回 ID；实现会重新分页查询同名 Key，再通过 `POST /api/token/:id/key` 读取完整 Key。固定线路的 `K` 继承站点配置，站点 `K` 更新会立即传播到线路、倍率快照和受管账号。
- 2026-07-15：管理页 `/admin/upstreams` 以调用协议为主分类，支持模型品牌、具体模型、站点类型和健康状态筛选；桌面/移动视口已验证无翻译键和页面级横向溢出。当前工作区缺少前端 `node_modules`，前端检查使用 `C:\tmp\sub2api-upstream-build-workspace-20260715` 与既有依赖 junction；Wire 因本地缺失所需 `go.sum` 项未重新生成，`wire_gen.go` 已按 `wire.go` 手工同步。
- 2026-07-15：上游资源池 Docker 集成测试沿用 `E:\DockerTestReplica\compose\docker-compose.yml`；`test` 项目的 app/PostgreSQL/Redis 当前均停止，数据库使用外部卷 `sub2api-replica-postgres-20260711`，app 仍引用旧 `sub2api:tls-eof-test`。验证新功能时应先备份测试库，再构建独立的 `sub2api:upstream-pool-<commit>` 镜像标签并仅替换 test app；本地 `new-api` 夹具当前也处于停止状态。
- 2026-07-15：上游资源池隔离测试已切到克隆卷 `sub2api-upstream-pool-postgres-c2260f8e` 与修复镜像 `sub2api:upstream-pool-c2260f8e-fix1`，test app 位于 `http://localhost:18081` 且保持健康；正式 `sub2api:production`（`http://localhost:8080`）及其他部署未改动。
- 2026-07-15：Docker 集成测试发现当 `openai_advanced_scheduler_enabled=false` 时旧 OpenAI load-aware 路径会绕过上游受管池的成本分层，导致最低倍率满载后直接下沉；现已让纯受管候选池在旧路径复用成本分层选择，混合手工账号仍保持旧逻辑。5 个目标单元测试通过；真实网关验证最低层满载等待约 2.2 秒后下沉、503 按 `cheap->expensive` 切换、sticky 会话保持原线路、恢复后新会话回到 cheap；桌面/390px 管理页、添加站点弹窗与控制台检查均通过。
- 2026-07-15：上游管理界面的 `K` 字段已改为“充值倍率”，“K 来源”改为“充值倍率来源”，公式中的 `P = X / K` 保留数学表达；对应 locale 回归测试、Docker 完整前端/后端构建和桌面/390px UI 检查通过。test app 当前镜像为 `sub2api:upstream-pool-d52a4661-zh-k`。
- 2026-07-15：桌面 `AI中转账号密码.txt` 是上游站点凭据来源，仅记录文件位置，不记录凭据值。已向 `http://localhost:18081` 的 test 克隆环境幂等录入其中 20 个唯一站点；文件标注的 new2api 站点实际暴露 Sub2API 接口，其中 `灵思智域` 使用原域名、`lajiang.xyz` 使用 API 域名 `api.lajiang.xyz`。正式 `http://localhost:8080`、`delay` 和其他部署未改动。
- 2026-07-17：test 应用已通过 `E:\DockerTestReplica\compose\docker-compose.deploy-baseline.yml` 切换到正式 deploy 同一 `sub2api:production` 镜像（Sub2API `0.1.151`），仍使用 test 独立的端口 `18081`、克隆 PostgreSQL 卷 `sub2api-upstream-pool-postgres-c2260f8e` 和独立 Redis；切换前后 deploy 容器镜像、创建时间、重启次数、端口和挂载指纹一致，`8080` 与 `18081` 健康检查均通过。
- 2026-07-17：Docker Desktop backend 曾在 `10:52:34` 以 `exit status 2` 退出，同期宿主机 Go linker 报 Windows `VirtualAlloc errno=1455`；未发现 deploy Compose 或数据库连接配置变化。清理残留 Docker Desktop 前端进程、终止 `docker-desktop` WSL 后重新启动，原 deploy 三容器按现有配置恢复健康。
