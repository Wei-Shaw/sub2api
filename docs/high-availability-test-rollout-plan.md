# 高可用调度策略：离线克隆测试与切换计划

本文针对线上 Sub2API 部署在远程服务器、低峰停机后复制 `/opt/sub2api`，再用修改后的代码复用数据副本进行测试的场景。

## 1. 总原则

- 新策略默认关闭，旧配置没有新字段时必须保持当前行为。
- 只扩展候选账号的健康评分、排序和运行时熔断，不重写已有资格过滤、并发槽位、计费和流式 failover 边界。
- 不修改已应用的 migration，不删除或覆盖生产数据卷。
- 测试环境使用生产数据的副本，并使用独立 Compose project、网络和端口。
- 生产切换前保留旧镜像、旧配置、数据库备份和 Redis 备份。

## 2. 先确认线上数据到底在哪里

在远程服务器执行只读检查：

```bash
cd /opt/sub2api
docker compose config > /tmp/sub2api-compose-rendered.yml
docker compose ps
docker inspect sub2api --format '{{json .Mounts}}'
docker inspect sub2api-postgres --format '{{json .Mounts}}'
docker inspect sub2api-redis --format '{{json .Mounts}}'
```

必须确认：应用 `/app/data`、PostgreSQL `PGDATA`、Redis `/data` 是否分别映射到 `/opt/sub2api/data`、`/opt/sub2api/postgres_data`、`/opt/sub2api/redis_data`。如果使用 `sub2api_data`、`postgres_data`、`redis_data` 等命名卷，直接复制 `/opt/sub2api` 不会复制数据库和 Redis，必须额外导出命名卷。

## 3. 低峰停机与备份

### 3.1 记录配置和版本

```bash
cd /opt/sub2api
docker compose images > backup-compose-images.txt
docker compose config > backup-compose-rendered.yml
cp -a .env backup.env
sha256sum backup.env backup-compose-rendered.yml > backup-manifest.sha256
```

必须保留 `.env` 中的 `POSTGRES_PASSWORD`、`JWT_SECRET`、`TOTP_ENCRYPTION_KEY`，以及 `data/config.yaml`（如果存在）。测试副本应继续使用相同密钥，否则会出现登录会话失效或 TOTP 不可用。

### 3.2 停止写入者

```bash
cd /opt/sub2api
docker compose stop sub2api
docker compose stop postgres redis
docker compose ps
```

确认容器已停止后再复制 PostgreSQL/Redis 文件。禁止使用 `docker compose down -v`，它可能删除命名卷。

### 3.3 目录挂载部署

仅当 `docker inspect` 确认使用目录挂载时：

```bash
mkdir -p /opt/sub2api-ha-test
rsync -aHAX --numeric-ids /opt/sub2api/ /opt/sub2api-ha-test/
chmod 600 /opt/sub2api-ha-test/.env
```

测试副本必须包含应用 data、PostgreSQL data、Redis data 和 `.env`，不能只复制应用源码。

### 3.4 命名卷部署

先列出并核对精确卷名：

```bash
docker volume ls
docker inspect <postgres_volume_name>
docker inspect <redis_volume_name>
```

推荐先启动数据库和 Redis 做逻辑备份：

```bash
docker compose start postgres redis
docker compose exec -T postgres pg_dumpall -U "$POSTGRES_USER" > /opt/sub2api/postgres-backup.sql
docker compose exec -T redis redis-cli --rdb /data/redis-backup.rdb
docker compose stop postgres redis
```

恢复时使用测试专用 PostgreSQL/Redis。不要猜测 `/var/lib/docker/volumes` 下的目录，也不要直接复制运行中的 PostgreSQL 数据目录。

## 4. 启动隔离测试环境

仓库已提供 `deploy/docker-compose.ha-test.yml` 作为 `docker-compose.local.yml` 的覆盖层。它只负责以下差异：从当前工作区构建应用、默认把测试端口绑定到 `0.0.0.0:18080` 以便无界面 Linux 服务器外部观测、使用独立容器名、把三个持久化目录改到 `deploy/ha-test/`，并在验证阶段默认使用 Debug 日志。正式运行使用的 `docker-compose.local.yml` 不被修改，继续按线上 `.env` 使用 `127.0.0.1`。可通过 `HA_BIND_HOST=127.0.0.1` 收紧测试环境为仅本机访问。该文件依赖 Compose 的 `!override`，要求 Docker Compose `v2.24.4` 或更高版本：

```text
docker compose version
```

### 4.1 准备测试数据目录

在仓库根目录创建测试目录，并将已经停机复制或逻辑恢复的数据放入其中：

```text
deploy/ha-test/.env
deploy/ha-test/data/
deploy/ha-test/postgres_data/
deploy/ha-test/redis_data/
```

`.env` 必须来自测试副本，并保留原 `POSTGRES_PASSWORD`、`POSTGRES_USER`、`POSTGRES_DB`、`JWT_SECRET` 和 `TOTP_ENCRYPTION_KEY`。`deploy/ha-test/` 已被 Git 忽略，禁止强制加入版本控制。

使用原始 `postgres_data` 前先读取 `postgres_data/PG_VERSION`。测试覆盖文件默认使用 PostgreSQL 18；若数据目录为其他主版本，通过 `HA_POSTGRES_IMAGE` 指定完全匹配的镜像，例如：

```dotenv
HA_POSTGRES_IMAGE=postgres:17-alpine
HA_REDIS_IMAGE=redis:8-alpine
HA_BIND_HOST=0.0.0.0
HA_SERVER_PORT=18080
HA_IMAGE_TAG=ha-test-local
HA_LOG_LEVEL=debug
```

不得使用 PostgreSQL 18 直接挂载 PostgreSQL 15/16/17 的数据目录。Linux 原始 PGDATA 复制到 Windows Docker Desktop 如果出现权限、符号链接或版本错误，改用 `pg_dumpall` 导出并恢复到全新的 `deploy/ha-test/postgres_data/`，不要修改原数据副本。Redis 不包含用户、账号和分组等主数据；跨平台恢复困难时可以使用空的 `redis_data` 验证冷启动。

### 4.2 检查渲染结果

启动前必须先渲染配置，确认端口和卷没有指向生产目录：

```bash
docker compose --env-file deploy/ha-test/.env \
  -p sub2api-ha-test \
  -f deploy/docker-compose.local.yml \
  -f deploy/docker-compose.ha-test.yml config
```

渲染结果必须满足：应用端口为 `${HA_BIND_HOST}:${HA_SERVER_PORT}:8080`（无界面服务器默认 `0.0.0.0:18080:8080`），三个宿主机目录均位于当前仓库的 `deploy/ha-test/`，网络名为 `sub2api-ha-test_sub2api-network`，容器名带 `ha-test`。

### 4.3 构建和启动

```bash
docker compose --env-file deploy/ha-test/.env \
  -p sub2api-ha-test \
  -f deploy/docker-compose.local.yml \
  -f deploy/docker-compose.ha-test.yml up -d --build
docker compose --env-file deploy/ha-test/.env \
  -p sub2api-ha-test \
  -f deploy/docker-compose.local.yml \
  -f deploy/docker-compose.ha-test.yml ps
docker compose --env-file deploy/ha-test/.env \
  -p sub2api-ha-test \
  -f deploy/docker-compose.local.yml \
  -f deploy/docker-compose.ha-test.yml logs --tail=200 sub2api
curl -fsS http://127.0.0.1:18080/health
```

Windows PowerShell 的续行符使用反引号 `` ` ``，也可以把上述每条 Compose 命令写成单行执行。绑定 `0.0.0.0` 会让测试端口暴露在服务器所有网卡上，应使用云安全组/防火墙限制来源 IP；测试结束后建议停止容器或设置 `HA_BIND_HOST=127.0.0.1`。

停止并保留测试数据：

```bash
docker compose --env-file deploy/ha-test/.env \
  -p sub2api-ha-test \
  -f deploy/docker-compose.local.yml \
  -f deploy/docker-compose.ha-test.yml down
```

禁止增加 `-v`。测试 Compose 只覆盖测试镜像、容器名、网络、端口和数据目录，不能指向线上 `/opt/sub2api` 或复用生产命名卷。测试结束后将 `HA_LOG_LEVEL` 恢复为 `info` 再进行接近生产负载的性能测试，避免 Debug 日志影响结果。

## 5. 代码实现分阶段计划

### 阶段 A：开关和兼容骨架

- 在 `gateway.openai_scheduler` 下增加策略与绑定字段：
  - `strategy`: `legacy`（默认）或 `high_availability`；未知值启动校验失败。
  - `sticky_binding_mode`: `keep_original`（默认）或 `rebind_on_failover`。
  - `failover_on_health_escape`: 是否把健康评分触发的粘性逃逸视为一次可迁移的 failover，默认 `false`。
  - `high_availability_error_rate_weight` / `high_availability_ttft_weight`: 高可用模式下的健康评分权重，默认均为 `1.5`。
  - `health_circuit_enabled`、`health_circuit_failure_threshold`、`health_circuit_window_seconds`、`health_circuit_cooldown_seconds`: 账号级短时熔断参数，默认开启、`3/60/30`。
- `strategy=legacy` 时不改变现有候选排序、粘性绑定和错误处理；旧配置文件、旧数据库和旧 Redis key 无需迁移。
- `strategy=high_availability` 只在现有 OpenAI 高级调度器启用时生效；没有高级调度器时保持原调度路径。
- `sticky_binding_mode=keep_original` 保留原会话绑定，健康恢复后自然回到原账号；`rebind_on_failover` 仅在真实请求 failover 或显式允许的健康逃逸成功后迁移绑定。
- `failover_on_health_escape=false` 将“健康评分逃逸”和“请求失败 failover”分开，避免仅因 EWMA 变差就永久迁移会话。
- 新策略只在现有候选过滤完成后介入排序。
- 保留分组、平台、模型能力、配额、RPM、窗口成本、并发和会话过滤。
- 保留 `UpstreamFailoverError`、最大切换次数和“流已输出后禁止切号”规则。
- 健康指标缺失、Redis 不可用或评分异常时回退旧排序。

### 阶段 B：账号级运行时指标

- 按账号、平台、规范化模型记录成功、429、认证失败、5xx、网络错误、超时和 TTFT。
- 使用有界窗口或 EWMA、样本下限和 TTL；一次请求不能决定账号排序。
- 上下文超限、参数错误、内容政策拒绝和请求级容量降载不应惩罚账号。
- 健康状态与现有账号限流/临时不可调度状态分开，避免破坏恢复逻辑。

优先复用仓库已有 OpenAI 高级调度器、账号错误率 EWMA、TTFT EWMA 和粘性逃逸能力，不新增第二套 OpenAI 调度器。

### 阶段 C：高可用评分

首个实现复用已有错误率/TTFT EWMA 和评分权重，在 `high_availability` 下提高成功率、TTFT 的排序影响，但不改变资格过滤和槽位协议。当前已加入按账号错误类别计数及短时健康熔断。

当前已落地的第一阶段代码范围：配置字段、默认值、启动校验、示例配置、健康权重覆盖，以及健康逃逸时的粘性绑定保留判定；尚未改变现有 `UpstreamFailoverError` 的重试上限或流式边界。

进度更新：真实请求切换绑定已通过显式 `RebindStickySessionAfterFailover` 接入 OpenAI Responses、Chat、Images、Alpha Search 和 WebSocket 切号点；错误观测已增加 429、认证、5xx、超时、网络、请求级和请求范围分类；高可用模式下连续账号级错误达到阈值会短暂摘除账号，成功后恢复。高可用模式不会因请求参数或客户端容量错误降低账号 EWMA，旧模式仍保持原有统计入口。

新增完成：冷启动有限探索已使用真实请求 admission 实现，每分钟按 `cold_start_probe_quota_per_minute` 给无历史样本账号有限机会，不生成额外后台上游请求；Redis 可用时通过 `GatewayCache` 可选扩展共享账号失败窗口与 circuit 冷却，Redis 故障自动退回进程内 EWMA/circuit，不修改已有 Redis key。

定时测试计划新增 `include_in_health_samples` 开关，数据库和管理界面默认均为关闭，现有计划升级后保持原行为。显式开启且策略为 `high_availability` 时，最近 5 分钟的定时测试结果作为权重 `0.25` 的独立探测信号；最近 15 分钟存在真实请求样本时真实流量占主导。探测延迟不写入真实 TTFT，探测结果不触发或清除 circuit，确定性 400/404/422、模型不支持、内容策略错误和 Runner 取消不会惩罚账号。

该开关通过新增迁移 `229_scheduled_test_health_samples.sql` 以 `ADD COLUMN IF NOT EXISTS ... DEFAULT false` 落地，只增加定时测试计划字段，不重写已有计划或结果数据。若合并官方更新时上游已占用迁移号 `229`，合并前仅重命名本迁移为当时最新编号，不修改已经发布过的迁移内容。

### 阶段 D：失败切换和观测

- 首字前的账号级 429、认证失败、账号级 5xx、连接失败和超时：排除当前账号并切换。
- 非池模式默认不在同账号重复请求；池模式的同账号重试保持原规则。
- 首字后流错误继续禁止换号，避免拼接两个账号的响应。
- 记录候选数、健康分、失败分类、切换次数、最终账号和耗时，不记录 API Key、OAuth token 或完整敏感请求体。

观测闭环已按后端运维用途落地，不新增前端面板、数据库表或对外 API：

- `openai.ha_scheduler.candidates`（Debug）：最多记录评分最高的 16 个候选，包含账号 ID、综合分、错误率 EWMA、TTFT EWMA、样本来源、优先级和当前负载；超过上限只记录截断数量。
- `openai.ha_scheduler.selection`（Debug）：记录分组、模型、候选数、Top-K、排除数量、选号层、最终选择账号和调度耗时；无可用账号、取消和超时使用稳定的结果分类。
- `openai.ha_scheduler.account_result`（Debug）：记录真实请求的成功/失败、失败类别、更新后的健康度、样本来源和本地 circuit 状态。
- `openai.ha_scheduler.circuit_opened`（Warn）与 `circuit_recovered`（Info）：只在本地 circuit 状态发生转换时记录，避免每个失败请求重复告警。
- `openai.ha_scheduler.health_cache_fallback`（Warn）：Redis 健康状态读写失败时记录回退本地状态，同一进程最多每分钟一条。
- 原有各入口的 `upstream_failover_switching` 保留切号次数与上游状态；HTTP 成功终态 `request_completed` 以及 WebSocket 的 `openai.websocket_turn_completed` 现在同时记录最终账号、切号次数和请求/turn 总耗时。
- 上述请求链路日志复用 `request_id` / `client_request_id`，可按请求关联；不记录账号名称、凭据、代理认证信息、请求体或响应体。
- `SnapshotOpenAIAccountSchedulerMetrics` 增加成功/失败、各错误分类、circuit 开启/恢复及定时探测成功/失败累计值。该快照只在当前进程内累计，重启归零，现阶段用于测试断言和后续运维接入。

生产默认 `LOG_LEVEL=info` 时只输出低频状态转换；数据副本测试和短时调优可临时使用 `LOG_LEVEL=debug` 查看候选与逐账号结果，结束后应恢复 `info`，避免高流量下增加日志量。

## 6. 测试顺序

1. 策略关闭回归：现有测试通过，行为等同当前版本。
2. 单元测试：评分、样本衰减、冷启动、熔断恢复、同分随机和 Redis 故障回退。
3. 模拟上游：429、500、503、慢首字、连接超时、首字后断流和确定性 400。
4. 数据副本验证：登录、API Key、分组、账号凭据、模型映射、渠道配置和定时任务。
5. 低并发压测：观察成功率、TTFT、429、5xx、切换次数、等待队列、数据库写入和计费记录。

测试副本使用真实账号凭据时仍可能产生上游费用、限流和账号状态变化，应限制模型、并发、请求频率和测试时长。

验证状态：已使用 Go 1.26.5 完成本轮改动文件格式化；`./internal/config`、`./internal/service`、`./internal/repository`、`./internal/handler/admin` 和 `./cmd/server` 的目标编译/测试通过；迁移包测试通过；新增的高可用权重、健康逃逸粘性模式、failover 绑定、错误分类、健康熔断、冷启动配额、定时探测低权重融合和观测指标/日志测试通过。前端依赖尚未安装，`vue-tsc` 未执行；全量服务测试、Docker 数据副本测试、真实上游故障矩阵和上线演练仍待统一验证阶段执行。

在安装 Go 的 CI 或开发环境继续运行：

```bash
gofmt -w backend/internal/config/config.go backend/internal/config/config_test.go \
  backend/internal/service/openai_account_scheduler.go \
  backend/internal/service/openai_account_scheduler_reset_test.go \
  backend/internal/service/openai_account_scheduler_test.go \
  backend/internal/service/openai_gateway_scheduling.go
go test ./backend/internal/config ./backend/internal/service
go test ./backend/internal/repository
go test ./backend/internal/handler -run '^$'
```

## 7. 生产切换与回滚

单实例直接替换容器不是严格的无感切换；当前进程退出可能中断在途请求。推荐：新版本构建不可变镜像 tag，在副本完成验证；新实例先以 `legacy` 启动并通过健康检查；小比例流量灰度；稳定后再打开 `high_availability`；旧实例停止接收新连接并等待 SSE 排空；异常时切回旧实例。

若当前没有反向代理或负载均衡，只能低峰停机替换，仍会影响正在进行的请求；应把这一点纳入维护窗口通知。双实例前确认定时检测、渠道监控、清理和聚合任务有可靠分布式锁，否则会重复执行后台副作用。

回滚只回滚应用镜像和配置，不回滚用户数据；新数据库结构必须独立、向前兼容、只增不改并先备份。禁止执行 `docker compose down -v`、删除 Docker volume 或覆盖原 `/opt/sub2api`。

## 8. 推荐提交拆分

```text
feat: add scheduler strategy switch
feat: add sticky binding failover mode
feat: add account runtime health metrics
feat: add high availability candidate scoring
feat: add failover and circuit breaker tests
docs: document test and rollout procedure
```

每个提交都应可编译、可测试、可回退；不要混入无关格式化或重构。

## 9. 官方更新后的合并维护

高可用改动应尽量成为一层独立策略，而不是复制或大范围改写官方调度流程。这样官方更新后可以先同步 `upstream/main`，再重放自己的少量提交。

### 9.1 更新源与职责边界（必须遵守）

当前远程约定如下：

```text
upstream = https://github.com/Wei-Shaw/sub2api.git       # 官方更新源，只读使用
origin   = https://github.com/bianchen-xiaobai/sub2api.git # 自己的 Fork，保存定制分支
```

- `upstream/main` 是“官方是否有新代码”的唯一判断依据。
- `origin/feature/ha-scheduler` 是经过测试、可部署的定制版本；它不是官方更新源。
- 面板的“检查更新/立即更新”固定查询 `Wei-Shaw/sub2api` 的 GitHub Release，并下载官方二进制；它不识别 `feature/ha-scheduler`、自己的 Fork 或未合并的本地代码。
- 因此，在 HA 测试和正式定制部署中，**禁止点击面板的“立即更新”**。它会丢失当前容器内的 HA 二进制修改，且不会更新 Git 工作区或定制镜像。
- 面板的“检查更新”只能作为官方发布提醒，不能作为“当前定制分支是否已更新”的部署依据。

推荐分支关系：

```text
upstream/main                 官方仓库
        |
        +-- main              只跟随官方、保持干净
                |
                +-- feature/ha-scheduler   高可用策略
```

日常同步流程：

```bash
# 0. 前提：工作区必须干净；未完成的 HA 修改先形成可追踪提交并推送。
git status
git add <本次 HA 改动涉及的文件>
git commit -m "feat: describe the completed HA change"
git push origin feature/ha-scheduler

# 1. 只获取两个远程分支，不修改代码；先判断官方是否有待合并提交。
git fetch origin upstream --prune
git log --oneline origin/feature/ha-scheduler..upstream/main
# 无输出：当前定制分支已包含官方 main 的全部提交，无需进行官方合并。
# 有输出：这些是官方新增、尚未进入定制分支的提交，继续下面流程。

# 可选：用一个数字同时查看双方各自领先多少提交。
# 输出的左侧是定制分支独有提交数，右侧是尚未合并的官方提交数。
git rev-list --left-right --count origin/feature/ha-scheduler...upstream/main

# 2. 先把本地官方镜像分支快进到 upstream/main，禁止在 main 写定制代码。
git switch main
git pull --ff-only upstream main

# 3. 将定制提交重放到新的官方 main 之上。
git switch feature/ha-scheduler
git pull --ff-only origin feature/ha-scheduler
git rebase main
```

解决冲突后必须执行：

```bash
git status
git add <已解决文件>
git rebase --continue
go test ./backend/internal/service/... ./backend/internal/handler/...
git push --force-with-lease origin feature/ha-scheduler
```

`rebase` 会改写定制分支提交历史，因此只允许使用 `--force-with-lease` 推送；禁止使用不带保护的 `--force`。发生无法安全判断的冲突时，不要强行覆盖官方版本；先执行 `git rebase --abort`，保留冲突现场并重新阅读官方改动。

### 9.2 合并后的验证与部署协议

合并完成不等于可以更新线上。必须依次完成：

1. 运行冲突涉及模块与 HA 回归测试；前端有改动时先安装依赖并运行类型检查/构建。
2. 基于 `feature/ha-scheduler` 构建新的不可变镜像 tag，例如 `sub2api:ha-20260824-<短提交号>`；不得复用或覆盖正在运行的 tag。
3. 使用 `deploy/docker-compose.ha-test.yml` 和生产数据副本启动隔离测试，完成第 6 节的登录、数据兼容、故障切换和低并发验证。
4. 记录将部署的 Git commit、镜像 tag、官方基线 commit 和测试结果；只有通过验证的 `origin/feature/ha-scheduler` commit 可以进入生产。
5. 正式环境按第 7 节低峰切换。生产停止前保留旧镜像 tag；异常时只回滚应用镜像和配置，不回滚数据库数据。

部署时拉取的应是自己的定制镜像或由当前分支在服务器构建的镜像，绝不能执行 `docker pull weishaw/sub2api:latest` 作为定制版本更新手段。若线上暂时采用源码构建，仍须在服务器核对目标 commit 与 `origin/feature/ha-scheduler` 一致后，再执行 Compose 的 `up -d --build`。

### 9.3 给后续 AI/维护者的操作准则

- 先执行 `git status`、`git remote -v`、`git branch -vv`，确认当前分支、远程和工作区状态；绝不覆盖用户未提交的改动。
- 只要存在未提交 HA 改动，就先完成测试、按逻辑拆分提交、推送到 `origin/feature/ha-scheduler`，再开始官方同步。
- “是否有官方更新”使用 `git fetch origin upstream --prune` 后的 `git log origin/feature/ha-scheduler..upstream/main` 判断，不调用面板更新接口。
- 官方更新合入一律采用 `git rebase main`；冲突只解决 HA 改动与官方改动的真实语义冲突，不为了让 Git 通过而删除官方逻辑或 HA 逻辑。
- 每次 rebase 后检查数据库 migration 编号是否与上游冲突。尚未发布的本分支 migration 可以改号；已经部署到任何共享/生产数据库的 migration 绝不能改名、改内容或删除。
- 不执行 `git reset --hard`、`git checkout --`、`docker compose down -v`、删除持久化数据或面板“立即更新”。
- 未通过测试、数据副本验证或无法解释的冲突，不得部署；应保留分支状态并报告阻塞原因。

为降低冲突范围，遵守以下约束：

- 不修改官方已有 migration、数据表和用户/账号字段；新增结构必须使用新的独立 migration。
- 不直接改写 `SelectAccountWithLoadAwareness` 的大段流程；将高可用排序放入独立 helper/接口，并在一个稳定调用点接入。
- 不修改现有错误分类、`UpstreamFailoverError`、流式输出边界和并发槽位协议；通过新增策略函数或配置开关扩展。
- 配置新增字段集中放在配置结构、默认值、校验和示例文件中，不散落修改大量调用方。
- 健康指标使用独立 key 前缀、类型和文件；不要改变现有 Redis key 格式。
- 不执行全仓库格式化，不混入无关重命名、依赖升级或前端改动。
- 每个提交只完成一个逻辑主题，并保持可编译、可测试、可回退。

官方更新后的验证顺序：

1. 在 `main` 上完成 fast-forward 同步，并运行官方现有测试。
2. 在高可用分支 rebase 后运行冲突涉及模块的单元测试和 failover/调度测试。
3. 验证策略关闭时与官方行为一致。
4. 使用数据副本启动 Docker 测试环境，验证登录、分组、账号、模型映射、计费和切号。
5. 通过后再构建新的不可变镜像，不要直接覆盖旧镜像 tag。

如果官方重构了调度入口，应先重新定位“候选过滤完成、账号最终选择之前”的新稳定扩展点，再移植高可用 helper；不要把旧版本的整段函数原样强行合并过去。
