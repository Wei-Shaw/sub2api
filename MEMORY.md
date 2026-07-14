# 项目记忆

## 基础信息

- 项目：sub2api
- 记录日期：2026-07-11
- 代码结构：`backend/` 为 Go 服务端，`frontend/` 为 TypeScript/Vite 前端，前端使用 pnpm 锁文件。
- 远端：`origin` 指向自有 Fork `https://github.com/GitClound/sub2api.git`；`upstream` 指向开源作者仓库 `https://github.com/Wei-Shaw/sub2api`。

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
