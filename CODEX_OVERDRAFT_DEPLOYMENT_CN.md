# sub2api-overdraft 部署与运维指南

本文档面向希望部署本项目、验证 Codex 5h / 7d 额度透支状态，以及后续安全升级的使用者。

本项目是 [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api) 的非官方衍生版本，公开项目名为 **sub2api-overdraft**。内部二进制名、Docker 服务名、数据目录和 Go module 仍保留 `sub2api`，这是为了兼容原项目并降低同步上游更新时的冲突。

## 重要说明

- 本功能仅表示：Codex 5h 或 7d 用量达到 95% 后开始注入兼容请求形态，并在额度达到 100% 后通过真实业务结果或单次独立探测确认上游是否仍允许继续调用。
- `passed` 只代表探测时上游仍返回有效响应，不承诺之后始终可用，也不会绕过认证失效、账号禁用、网络故障或其他风控限制。
- 探测请求和透支期请求都可能产生真实上游用量和费用。
- 此行为可能不符合上游服务条款。部署者必须自行确认合规性，并承担账号限制、服务中断等风险。
- 官方 Sub2API 安装脚本和 `weishaw/sub2api:latest` 镜像不包含本 Fork 的透支功能。
- 本 Fork 的后台更新检查读取 `DeanZFC/sub2api-overdraft`，不会把官方 Sub2API Release 当作本项目更新。

## 功能范围

开启 `gateway.codex_quota_overdraft_enabled` 后，本项目会：

- 对符合条件的 OpenAI OAuth 普通账号和 Agent Identity 账号启用透支调度，Shadow 子账号除外。
- 支持 `/v1/responses`、转换为 Responses 的 `/v1/chat/completions` 和 `/v1/messages`，以及 Responses WebSocket v2。
- 在适用请求的最后一条用户消息后注入一对无操作工具调用，用于保持与参考实现一致的请求形态。
- 用量达到 95% 后开始对适用业务请求注入；达到 100% 后，注入业务成功直接确认 `passed`，明确额度 429 直接确认 `failed`。没有业务结果可用时，同周期最多补充 1 次独立探测。
- 管理页面的 OpenAI OAuth 常规文本“测试账号连接”也使用相同请求形态，并接入同一套额度探测状态机。
- 分别维护 5h、7d 透支周期，并在管理页面显示状态、请求数、Token、账号金额和预计恢复时间。
- 将探测状态保存在现有 `accounts.extra` JSONB 字段中，不新增数据库表，不需要手动执行数据库迁移（migration）。

以下端点不启用透支：`/responses/compact`、图片生成、Embedding、Count Tokens 和 Live；API Key、Shadow、图片及 Compact 的后台测试也不启用透支。

## 前置条件

- Linux amd64 或 arm64 服务器
- Git
- Docker 20.10 或更高版本
- Docker Compose v2，命令为 `docker compose`
- 建议至少 2 核 CPU、4 GB 内存和 20 GB 可用磁盘
- 可以访问 GitHub、Docker Hub、Go module 和 npm/pnpm 依赖源

首次源码构建时间取决于服务器性能和网络状况。PostgreSQL 与 Redis 已包含在 Compose 中，不需要提前单独安装。

## 全新部署

### 1. 克隆项目

```bash
sudo mkdir -p /opt
cd /opt
sudo git clone https://github.com/DeanZFC/sub2api-overdraft.git
sudo chown -R "$(id -u):$(id -g)" /opt/sub2api-overdraft
cd /opt/sub2api-overdraft/deploy
```

仓库默认分支是 `codex-overdraft`。确认当前分支：

```bash
git branch --show-current
```

预期输出：

```text
codex-overdraft
```

### 2. 创建环境配置

```bash
cp .env.example .env
chmod 600 .env
```

至少设置以下值，不要把真实密钥提交到 Git：

```bash
POSTGRES_PASSWORD=替换为随机密码
JWT_SECRET=替换为随机密钥
TOTP_ENCRYPTION_KEY=替换为随机密钥
ADMIN_EMAIL=你的管理员邮箱
ADMIN_PASSWORD=替换为强密码
SERVER_PORT=8080
```

可分别使用以下命令生成随机值：

```bash
openssl rand -hex 32
```

透支开关在公开部署覆盖文件中默认开启。也可以在 `.env` 中显式配置：

```bash
GATEWAY_CODEX_QUOTA_OVERDRAFT_ENABLED=true
```

### 3. 创建数据目录

```bash
mkdir -p data postgres_data redis_data
```

这些目录和 `.env` 已被 Git 忽略，不会在正常的 `git add .` 中上传。

### 4. 从源码构建并启动

后续所有 Compose 命令都应同时带上这两个文件：

```bash
docker compose \
  -f docker-compose.local.yml \
  -f docker-compose.overdraft.yml \
  up -d --build
```

`docker-compose.local.yml` 提供应用、PostgreSQL、Redis、网络和数据卷；`docker-compose.overdraft.yml` 把应用镜像切换为本仓库源码构建，并默认开启透支功能。

查看容器状态：

```bash
docker compose \
  -f docker-compose.local.yml \
  -f docker-compose.overdraft.yml \
  ps
```

三个容器应处于 `Up` 或 `healthy` 状态：

```text
sub2api
sub2api-postgres
sub2api-redis
```

查看启动日志：

```bash
docker compose \
  -f docker-compose.local.yml \
  -f docker-compose.overdraft.yml \
  logs -f sub2api
```

默认访问地址为 `http://服务器IP:8080`。

## 配置文件位置与优先级

源码仓库中的 `deploy/config.example.yaml` 只是完整配置模板，不是容器实际读取的运行配置。

Compose 本地目录部署的实际配置通常位于：

```text
/opt/sub2api-overdraft/deploy/data/config.yaml
```

它在容器内对应：

```text
/app/data/config.yaml
```

如果手动通过 YAML 开启，请确保层级正确：

```yaml
gateway:
  codex_quota_overdraft_enabled: true
```

环境变量 `GATEWAY_CODEX_QUOTA_OVERDRAFT_ENABLED` 的优先级高于 YAML。本项目提供的 Compose 覆盖文件默认将它设为 `true`，因此全新部署不需要手动编辑 `data/config.yaml`。

检查容器实际收到的环境变量：

```bash
docker compose \
  -f docker-compose.local.yml \
  -f docker-compose.overdraft.yml \
  exec sub2api sh -lc 'printf "%s\n" "$GATEWAY_CODEX_QUOTA_OVERDRAFT_ENABLED"'
```

预期输出为 `true`。

## 从现有 Sub2API 部署迁移

迁移前先备份 `.env`、运行配置和数据库。不要删除 `data`、`postgres_data`、`redis_data`。

如果服务器上的 `/opt/sub2api` 已经是 Git 仓库，并且包含手工修改，先检查：

```bash
cd /opt/sub2api
git status --short
git branch --show-current
git remote -v
```

有未提交代码时先保存：

```bash
git stash push -u -m "server changes before codex-overdraft switch"
```

然后连接本 Fork 并切换到公开分支：

```bash
git remote set-url origin https://github.com/DeanZFC/sub2api-overdraft.git
git fetch origin
git switch codex-overdraft 2>/dev/null || \
  git switch -c codex-overdraft --track origin/codex-overdraft
git pull --ff-only origin codex-overdraft
```

如果 Git 报 `detected dubious ownership`，确认目录确实是本项目后再执行：

```bash
git config --global --add safe.directory /opt/sub2api
```

已有的服务器专用 Compose 文件可以保留，但推荐改用仓库内公开的覆盖文件：

```bash
cd /opt/sub2api/deploy
docker compose \
  -f docker-compose.local.yml \
  -f docker-compose.overdraft.yml \
  up -d --build --force-recreate sub2api
```

切换分支后不要直接执行 `git stash pop`。先用 `git stash show --stat` 检查旧 stash；本分支已经包含透支代码，直接恢复旧代码可能造成重复修改和冲突。

## 判断功能是否生效

### 第一层：确认运行的是源码构建镜像

```bash
docker inspect sub2api --format '{{.Config.Image}}'
```

预期输出：

```text
sub2api-overdraft:local
```

如果输出 `weishaw/sub2api:latest`，说明当前仍在运行官方镜像，本功能不会生效。

### 第二层：确认开关已开启

```bash
docker exec sub2api sh -lc 'printf "%s\n" "$GATEWAY_CODEX_QUOTA_OVERDRAFT_ENABLED"'
```

预期输出 `true`。修改 `.env` 后必须重建或重新创建应用容器：

```bash
docker compose \
  -f docker-compose.local.yml \
  -f docker-compose.overdraft.yml \
  up -d --force-recreate sub2api
```

### 第三层：查看真实探测日志

账号 5h/7d 用量达到 95% 后开始注入；只有额度达到 100%，或者收到带明确额度信息的上游 429，才会产生透支确认状态。额度达到 100% 后，可以在管理页面对该 OpenAI OAuth 账号执行常规文本“测试账号连接”；测试请求本身会使用透支请求形态，并直接参与同一额度周期的判定。额度未耗尽时没有探测日志是正常现象。

```bash
docker logs --since 30m sub2api 2>&1 | \
  grep -E 'codex_quota_overdraft_(probe|state|pause|stale_rate_limit)'
```

关键日志：

| 日志 | 含义 |
| --- | --- |
| `codex_quota_overdraft_business_passed` | 注入后的真实业务或账号测试成功，直接确认透支可用 |
| `codex_quota_overdraft_business_exhausted` | 注入后的业务请求收到明确额度 429，直接确认额度耗尽并暂停账号 |
| `codex_quota_overdraft_probe_passed` | 至少一次真实探测成功，透支成立，账号继续参与调度 |
| `codex_quota_overdraft_probe_failed` | 单次独立探测明确返回额度限制，账号暂停至额度恢复时间 |
| `codex_quota_overdraft_probe_attempt` | 单次探测结果，包含模型、HTTP 状态码、结果和原因，不记录凭据及完整响应正文 |
| `codex_quota_overdraft_probe_inconclusive` | 网络、超时、5xx、普通瞬时 429 等导致无法确认；同周期不自动重试 |
| `codex_quota_overdraft_probe_claim_failed` | 无法在 PostgreSQL 中原子领取探测任务，需要排查数据库或版本 |
| `codex_quota_overdraft_state_persist_failed` | 状态无法写入 `accounts.extra`，页面可能不显示最新结果 |
| `codex_quota_overdraft_pause_applied` | `failed` 状态、账号暂停和调度通知已经原子提交 |
| `codex_quota_overdraft_stale_rate_limit_cleared` | 探测已证明账号可用，并清除了并发 429 遗留的账号级限流状态 |

真正“透支成功”的最直接证据是：

```text
codex_quota_overdraft_business_passed
```

如果没有业务成功证据，也可以由 `codex_quota_overdraft_probe_passed` 确认。其中会包含 `account_id`、`model` 和 `quota_window`。

### 第四层：查看数据库持久化状态

```bash
docker compose \
  -f docker-compose.local.yml \
  -f docker-compose.overdraft.yml \
  exec -T postgres sh -lc \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -x -c "
    SELECT id, name, extra->'\''codex_quota_overdraft_probe'\'' AS overdraft_probe
    FROM accounts
    WHERE platform = '\''openai'\'' AND type = '\''oauth'\''
    ORDER BY id;
  "'
```

重点看 JSON 中的 `status`：

| 状态 | 页面显示 | 含义 |
| --- | --- | --- |
| `pending` | 透支探测中 | 单次独立探测正在执行 |
| `passed` | 透支中 | 注入业务或独立探测成功，账号继续调度并开始统计透支用量 |
| `failed` | 已确认限额 | 注入业务或独立探测明确受额度限制，暂停到恢复时间 |
| `inconclusive` | 探测无法确认 | 单次探测受网络、瞬时限流或上游异常干扰；同周期不自动重试 |
| `recovered` | 额度已恢复 | 原额度周期结束，相关暂停已清理 |

页面显示 `透支中`，数据库为 `passed`，并且后续业务请求返回成功，三者同时满足即可确认功能完整生效。

## 探测逻辑

每个额度周期采用以下混合判定流程：

- 用量低于 95% 时不注入，达到 95% 后对适用的普通 OAuth 文本请求持续注入。
- 达到 100% 后，注入业务返回成功即记为 `passed`；返回明确 `quota_limited` 即记为 `failed`，暂停账号并切换到其他账号。
- 如果额度刚从 95% 以下跳到 100%，当前业务请求没有注入，系统仅补充 1 次独立 Responses 探测，最长 20 秒，并优先使用当前业务模型。
- 网络错误、超时、5xx、普通瞬时 429、无效响应等记为 `inconclusive`，不会因此误停账号。
- 401/403 继续由原有认证失效逻辑处理。
- 账号禁用、过载、代理故障、模型冷却等非额度限制不会被本功能绕过。

独立探测无法确认后，同一额度周期不会自动重试，也不会持续消耗探测请求。后续已经注入的真实业务成功或明确额度 429 仍可把状态更新为 `passed` 或 `failed`；同周期 `failed` 为终态，成功结果不能覆盖它。

多实例部署通过 PostgreSQL 原子领取（atomic claim）保证同一账号、同一额度周期只有一个实例发起探测。

## 反向代理

Nginx 需要把请求转发到宿主机的 `8080` 端口。使用 Codex CLI 时，建议在 Nginx `http` 块启用：

```nginx
underscores_in_headers on;
```

流式响应位置至少应包含：

```nginx
proxy_http_version 1.1;
proxy_buffering off;
proxy_cache off;
proxy_read_timeout 3600s;
proxy_send_timeout 3600s;
```

出现 `duplicate upstream "sub2api_backend"` 时，表示多个 Nginx 配置文件定义了同名 `upstream`。使用以下命令找出重复项，然后只保留一个定义：

```bash
grep -Rns 'upstream sub2api_backend' /etc/nginx/nginx.conf /etc/nginx/conf.d /etc/nginx/sites-enabled
```

修改后先验证再重载：

```bash
sudo nginx -t
sudo systemctl reload nginx
```

## 日常升级本 Fork

源码镜像的当前版本来自仓库根目录的 `FORK_VERSION`。后台检查更新时读取 GitHub 上 `codex-overdraft` 分支的同名文件：远端版本更高时显示更新提示，版本一致时显示最新。源码构建不会执行二进制在线更新或在线回退，以免官方程序覆盖 Fork 功能。

运行数据均位于被 Git 忽略的目录中，正常 `git pull` 不会覆盖：

```text
deploy/.env
deploy/data/
deploy/postgres_data/
deploy/redis_data/
```

升级步骤：

```bash
cd /opt/sub2api-overdraft
git status --short
git switch codex-overdraft
git pull --ff-only origin codex-overdraft

cd deploy
docker compose \
  -f docker-compose.local.yml \
  -f docker-compose.overdraft.yml \
  up -d --build
```

如果服务器目录是 `/opt/sub2api`，只需替换第一条路径。升级完成后重新执行“判断功能是否生效”中的镜像、环境变量和日志检查。

更新后可在管理后台点击刷新，版本应显示为类似 `v0.1.179-overdraft.1`，更新方式应提示源码构建使用 `git pull`。也可以直接检查容器内二进制版本：

```bash
docker exec sub2api /app/sub2api -version
```

维护者发布新的源码版本时必须递增 `FORK_VERSION`，例如从 `0.1.177-overdraft.3` 改为 `0.1.177-overdraft.4`。同步到新的上游 Sub2API 版本时，使用 `0.1.179-overdraft.1` 这样的版本号。

## 合并 Sub2API 官方更新

本仓库保留两个分支角色：

- `main`：尽量跟随官方 `Wei-Shaw/sub2api`，作为上游基线。
- `codex-overdraft`：本项目默认分支，包含透支功能和公开文档。

维护者可这样合并官方更新：

```bash
git remote get-url upstream >/dev/null 2>&1 || \
  git remote add upstream https://github.com/Wei-Shaw/sub2api.git
git fetch upstream
git switch codex-overdraft
git merge upstream/main
```

解决冲突后执行项目测试，提交并推送：

```bash
cd backend
go test -tags=unit ./internal/repository ./internal/handler ./internal/service \
  -run 'CodexQuotaOverdraft|AccountSchedulingThreshold|SchedulerSnapshot|SchedulableAccountQueryScopesCodex' \
  -count=1
go test ./internal/repository ./internal/handler ./internal/service ./cmd/server \
  -run '^$' -count=1

cd ../frontend
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm exec vitest run \
  src/components/account/__tests__/UsageProgressBar.spec.ts \
  src/components/account/__tests__/AccountUsageCell.spec.ts

cd ..
git diff --check
git push origin codex-overdraft
```

更详细的实现文件和升级核对清单见 [CODEX_QUOTA_OVERDRAFT_CUSTOMIZATION.md](CODEX_QUOTA_OVERDRAFT_CUSTOMIZATION.md)。

## 关闭与回滚

如遇上游兼容问题，在 `deploy/.env` 设置：

```bash
GATEWAY_CODEX_QUOTA_OVERDRAFT_ENABLED=false
```

然后重新创建应用容器：

```bash
docker compose \
  -f docker-compose.local.yml \
  -f docker-compose.overdraft.yml \
  up -d --force-recreate sub2api
```

关闭后会恢复官方调度与请求行为。数据库中已有的历史探测 JSON 可以保留，不影响关闭状态。

## 备份

数据库建议使用 `pg_dump` 做一致性备份：

```bash
cd /opt/sub2api-overdraft/deploy
mkdir -p backups
docker compose \
  -f docker-compose.local.yml \
  -f docker-compose.overdraft.yml \
  exec -T postgres sh -lc \
  'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
  > "backups/sub2api-$(date +%Y%m%d-%H%M%S).dump"
```

另外单独备份 `deploy/.env` 和 `deploy/data/config.yaml`。不要把备份、数据库目录、API Key 或 OAuth 凭证上传到公开仓库。

## 常见故障

### 没有任何透支日志

先确认镜像和开关。若账号 5h/7d 尚未达到 100%，没有探测日志属于正常情况。使用管理页面的主动刷新获取最新额度后，再发送一次符合范围的文本请求。

### 页面仍显示“限流中”

页面的“限流中”直接来自账号记录的 `rate_limit_reset_at`。旧实现存在竞态：探测开始后若另一个 429 更新了该时间，即使探测最终 `passed`，严格时间比较也可能不清理它，因此出现“实际透支可用但页面限流中”。当前版本在 `passed/recovered` 时会清除该残留状态，并记录 `codex_quota_overdraft_stale_rate_limit_cleared`；`failed` 不会清理。

其他真实限制仍会正常生效，包括账号禁用、401/403、注入业务或单次探测确认额度耗尽、代理/网络错误、模型级冷却、并发限制或没有明确额度证据的其他上游 429。若状态持续存在，先按 `account_id` 检查上述日志、数据库中的 `extra->'codex_quota_overdraft_probe'` 和 `rate_limit_reset_at`，区分额度透支与普通限流。

### `pq: could not determine data type of parameter $1`

这是旧版原子领取（atomic claim）SQL 的 PostgreSQL 参数类型问题。当前代码已使用：

```sql
jsonb_build_object($1::text, $2::jsonb)
```

拉取最新 `codex-overdraft` 分支并使用 `--build --force-recreate` 重建应用容器。

### API 返回 503 `no available accounts`

这表示账号调度阶段没有可用账号，不是 Nginx 连接故障。检查账号是否属于 API Key 对应分组、是否启用、是否认证有效、是否被其他原因暂停，以及探测状态是否为 `failed`。

### 容器无法绑定 8080

查找占用者：

```bash
sudo ss -ltnp 'sport = :8080'
```

如果是旧的 systemd 版 `sub2api`，先确认后停止并禁用：

```bash
sudo systemctl disable --now sub2api
```

也可以在 `.env` 将 `SERVER_PORT` 改为其他未占用端口。

### PostgreSQL 或 Redis 主机名无法解析

应用、PostgreSQL 和 Redis 必须加入同一个 Compose 网络。始终使用两个 Compose 文件一起启动完整服务，不要只用 `docker run` 单独启动应用容器。

## 开源与归属

- 上游项目：[Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api)
- 透支逻辑参考：[Mxucc/cpa-account-config-manager](https://github.com/Mxucc/cpa-account-config-manager)
- 本项目遵循仓库中的 [GNU LGPL-3.0](LICENSE) 许可证。
- 本项目不是 Sub2API 官方发行版，也不受 OpenAI 或其他上游服务商认可或支持。
