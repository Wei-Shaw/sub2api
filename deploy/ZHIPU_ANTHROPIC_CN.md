# Sub2API 接入智谱 Anthropic 兼容端点：部署与复现

本文记录 2026-07-10 在 macOS ARM64 上完成的本地 Docker 部署。所有示例均不包含真实管理员密码、数据库密码、智谱上游令牌或 Sub2API 下游 API Key。

## 1. 已部署结果

```text
Claude Code / Anthropic SDK
  -> http://127.0.0.1:8080
  -> Sub2API 独占分组 zhipu-anthropic
  -> Anthropic API-key 账号 zhipu-anthropic
  -> Authorization: Bearer <智谱上游令牌>
  -> https://open.bigmodel.cn/api/anthropic/v1/messages?beta=true
```

本机资源：

| 资源 | 值 |
| --- | --- |
| Sub2API 地址 | `http://127.0.0.1:8080` |
| 管理员邮箱 | `admin@sub2api.local` |
| 管理员密码 | 仅保存在 `deploy/.env`，权限 `0600` |
| 客户端配置 | `deploy/client.env`，权限 `0600`，已被 Git 忽略 |
| 分组 | `zhipu-anthropic`，ID `2`，Anthropic，独占，标准计费 |
| 上游账号 | `zhipu-anthropic`，ID `1`，API Key，Bearer，自动透传 |
| 下游 Key | `zhipu-claude-code`，ID `1`，绑定分组 `2` |
| 本地管理员余额 | `1000` USD 测试额度，继续使用 Sub2API 默认计费 |
| 上游账号并发 | `2`，用于 Claude Code 同时发起主模型和 Haiku 请求 |

模型映射：

| 客户端模型 | 智谱实际上游模型 | 原因 |
| --- | --- | --- |
| `glm-4.7` | `glm-4.7` | 直接映射 |
| `glm-5.2[1m]` | `glm-5.2` | 智谱端点实际接受的 1M 模型代码 |
| `glm-5.2` | `glm-5.2` | Claude Code 会去掉 `[1m]` 后缀，需要兼容别名 |

## 2. 已验证版本

| 组件 | 版本或不可变摘要 |
| --- | --- |
| 基础源码提交 | `6dd3274aafbc1a7a91304380fb3d7e50406841e0` |
| Docker | `29.6.1` |
| Docker Compose | `v5.2.0` |
| Sub2API | `weishaw/sub2api@sha256:33e2d98a4291da1ec3b5493f0b36164698ea2f20fd9e917f1500486025716d81` |
| PostgreSQL | `postgres@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15` |
| Redis | `redis@sha256:9d317178eceac8454a2284a9e6df2466b93c745529947f0cd42a0fa9609d7005` |
| Claude Code 烟雾测试 | `2.1.199` |

`docker-compose.local.yml` 当前使用浮动镜像标签。严格复现时应先拉取上表摘要对应的镜像，或在私有 override 文件中将镜像改为摘要；升级前先做 PostgreSQL 逻辑备份。

## 3. 从空环境部署

### 3.1 准备源码与目录

```bash
git clone https://github.com/Wei-Shaw/sub2api.git
cd sub2api
git checkout 6dd3274aafbc1a7a91304380fb3d7e50406841e0
mkdir -p deploy/data deploy/postgres_data deploy/redis_data
```

也可以从本文末尾的 Git bundle 恢复本次部署分支和标签。

### 3.2 生成服务配置

```bash
cd deploy
cp .env.example .env
chmod 600 .env
```

编辑 `.env`，至少设置以下内容。所有 `<...>` 值都必须替换；不要把渲染后的文件提交到 Git。

```dotenv
BIND_HOST=127.0.0.1
SERVER_PORT=8080
SERVER_MODE=release
RUN_MODE=standard
TZ=Asia/Shanghai

POSTGRES_PASSWORD=<openssl-rand-hex-32>
REDIS_PASSWORD=<openssl-rand-hex-32>
ADMIN_EMAIL=admin@sub2api.local
ADMIN_PASSWORD=<openssl-rand-hex-32>
JWT_SECRET=<openssl-rand-hex-32>
TOTP_ENCRYPTION_KEY=<openssl-rand-hex-32>

SECURITY_URL_ALLOWLIST_ENABLED=true
SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP=false
SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS=false
SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS=api.openai.com,api.anthropic.com,api.kimi.com,open.bigmodel.cn,api.minimaxi.com,generativelanguage.googleapis.com,cloudcode-pa.googleapis.com,*.openai.azure.com
```

每个随机值单独执行 `openssl rand -hex 32` 生成。不要复用密码。

### 3.3 启动并验活

```bash
docker compose --env-file .env -f docker-compose.local.yml config --quiet
docker compose --env-file .env -f docker-compose.local.yml pull
docker compose --env-file .env -f docker-compose.local.yml up -d --wait --wait-timeout 180
docker compose --env-file .env -f docker-compose.local.yml ps
curl --fail --silent http://127.0.0.1:8080/health | jq .
```

预期三个容器均为 `healthy`，健康接口返回 `{"status":"ok"}`。PostgreSQL 和 Redis 不应发布宿主机端口。

### 3.4 首次管理员确认

打开 `http://127.0.0.1:8080`，使用 `.env` 中的管理员账号登录。管理员本人阅读 `docs/legal/admin-compliance.zh.md`，并按页面要求确认当前版本；不要在自动化脚本中静默代签。

### 3.5 创建分组、账号和下游 Key

后台按 `deploy/zhipu-anthropic.account.example.json` 创建资源：

1. 创建名为 `zhipu-anthropic` 的 Anthropic 独占标准分组，倍率 `1`。
2. 把该分组追加到管理员的允许分组列表，不要覆盖原列表。
3. 为管理员设置足够的本地余额。本次设置为 `1000` USD，Sub2API 仍按默认模型价格记账。
4. 创建 Anthropic `apikey` 账号；Base URL 只能填写 `https://open.bigmodel.cn/api/anthropic`，不要附加 `/v1`。
5. 认证方案选择 `Authorization: Bearer`，开启 Anthropic API Key 自动透传。
6. 按模板配置三个模型映射和并发 `2`。
7. 创建绑定该分组的下游 Key，并保存到 `deploy/client.env`；这里保存的是 Sub2API Key，不是智谱上游令牌。

实际智谱令牌只在运行时写入账号凭据。API 返回会脱敏，但数据库备份仍包含凭据，因此必须加密保存。

## 4. Claude Code 客户端

复制模板并写入新生成的 Sub2API 下游 Key：

```bash
cd deploy
cp zhipu-anthropic.env.example client.env
chmod 600 client.env
```

加载配置：

```bash
set -a
source /path/to/sub2api/deploy/client.env
set +a
claude
```

若 `~/.claude/settings.json` 已存在智谱直连配置，其 `env` 字段可能覆盖当前 shell 环境。可选择更新该文件为 Sub2API 地址和下游 Key，或用下面的隔离命令做烟雾测试：

```bash
set -a
source deploy/client.env
set +a
export ANTHROPIC_API_KEY="$ANTHROPIC_AUTH_TOKEN"
unset ANTHROPIC_AUTH_TOKEN

claude --bare \
  --setting-sources '' \
  --settings '{}' \
  --no-session-persistence \
  --model glm-4.7 \
  -p 'Reply with exactly OK.'
```

隔离参数会禁用用户、项目和本地 Claude 设置，仅用于诊断，不适合作为日常启动参数。

## 5. 验证

默认验证脚本不会显示令牌或模型回复正文，但会真实调用上游：

```bash
cd deploy
./verify-zhipu-anthropic.sh
```

覆盖项目：

- `/health`；
- `/v1/models` 中的三个客户端模型名；
- `glm-4.7` 非流式消息；
- `glm-5.2[1m]` 流式消息及 `message_stop`；
- `/v1/messages/count_tokens`。

2026-07-10 实际验收结果：

| 项目 | 结果 |
| --- | --- |
| 健康检查 | 通过 |
| 账号测试 `glm-4.7` | `test_complete=true` |
| 账号测试 `glm-5.2` | `test_complete=true` |
| `/v1/models` | 返回 `glm-4.7`、`glm-5.2`、`glm-5.2[1m]` |
| `glm-4.7` 非流式 | HTTP 200，标准 message/usage |
| `glm-4.7` 流式 | HTTP 200，包含 `message_stop` |
| `glm-5.2[1m]` 非流式 | HTTP 200，实际上游模型 `glm-5.2` |
| `glm-5.2[1m]` 流式 | HTTP 200，包含 `message_stop` |
| `count_tokens` | HTTP 200 |
| Claude Code `glm-4.7` | 通过，usage User-Agent 为 `claude-cli/2.1.199` |
| Claude Code GLM-5.2 | 请求和映射正确；测试时智谱间歇返回 `529 overloaded` |

GLM-5.2 的 `529` 是上游瞬时容量错误。最小 API 和账号自检均已成功，不应通过修改模型映射来掩盖该错误；等待后重试即可。

`529` 后 Sub2API 可能暂时把唯一账号标记为不可调度，此时 `/v1/models` 会退回默认 Claude 模型列表。等待冷却窗口，或由管理员调用 `POST /api/v1/admin/accounts/1/recover-state` 恢复运行态，再重新执行验证脚本。不要在上游持续过载时循环强制恢复和重试。

## 6. 日常运维

```bash
cd deploy

# 状态
docker compose --env-file .env -f docker-compose.local.yml ps
curl --fail http://127.0.0.1:8080/health

# 日志；共享日志前必须脱敏
docker compose --env-file .env -f docker-compose.local.yml logs --tail 200 sub2api

# 重启应用
docker compose --env-file .env -f docker-compose.local.yml restart sub2api

# 停止；bind mount 数据不会被删除
docker compose --env-file .env -f docker-compose.local.yml down
```

不要执行 `rm -rf data postgres_data redis_data`，也不要在未备份数据库前升级到新镜像。

### 轮换智谱令牌

本次令牌曾在对话中明文出现，应在智谱控制台生成新令牌后立即轮换：

1. 在后台编辑账号 `zhipu-anthropic`；
2. 仅替换上游 API Key，保留 Base URL、Bearer、自动透传和模型映射；
3. 执行 `glm-4.7`、`glm-5.2` 账号测试；
4. 撤销旧令牌；
5. 再运行本文验证脚本。

下游 Key 可在 Sub2API 中独立撤销和重建，不需要改智谱令牌。

## 7. 数据备份与恢复

Git 只能备份代码、模板和文档，不能备份 PostgreSQL 中的账号状态、上游令牌和下游 Key。

在线逻辑备份示例：

```bash
cd deploy
mkdir -p /secure/backup/location
docker compose --env-file .env -f docker-compose.local.yml exec -T postgres \
  pg_dump -U sub2api -d sub2api -Fc \
  > /secure/backup/location/sub2api-$(date +%F).dump
```

备份文件包含敏感凭据，必须立即用 `age`、GPG 或组织认可的工具加密；不要放入 Git。还应单独加密备份 `deploy/.env`。恢复时必须保留原 `TOTP_ENCRYPTION_KEY`，否则已有 2FA/相关加密数据不可解密。

恢复到空数据库：

1. 准备同主版本 PostgreSQL 与新的空数据库；
2. 先启动 PostgreSQL 和 Redis，不启动 Sub2API；
3. 使用 `pg_restore --clean --if-exists` 恢复逻辑备份；
4. 放回权限 `0600` 的 `.env`；
5. 启动 Sub2API 并执行健康检查和本文验证脚本。

## 8. Git 备份

本次复现材料位于独立分支 `ops/zhipu-anthropic-deploy`，标签为 `deploy/zhipu-anthropic-20260710-v1`。真实 `.env`、`client.env` 和三个数据目录均被 Git 忽略。

创建或更新本地备份：

```bash
git status --short
git diff --cached --check
git tag -a deploy/zhipu-anthropic-20260710-v1 -m 'Reproducible Zhipu Anthropic deployment'
git bundle create ../sub2api-zhipu-anthropic-20260710.bundle \
  refs/heads/ops/zhipu-anthropic-deploy \
  refs/tags/deploy/zhipu-anthropic-20260710-v1
git bundle verify ../sub2api-zhipu-anthropic-20260710.bundle
```

从 bundle 恢复：

```bash
git clone sub2api-zhipu-anthropic-20260710.bundle sub2api-restored
cd sub2api-restored
git switch ops/zhipu-anthropic-deploy
```

当前仓库的 `origin` 指向官方上游 `Wei-Shaw/sub2api`，不能作为个人备份直接推送。需要异地备份时，应新建私有仓库并添加名为 `backup` 的 remote，再只推送上述精确分支和标签；不要使用 `--mirror` 或 `--all`。

## 9. 安全与合规边界

- 仅在智谱套餐与工具授权范围内使用，不公开转售或向无关第三方共享上游订阅。
- `deploy/client.env` 中只能放 Sub2API 下游 Key。
- 智谱上游令牌只保存在账号凭据和加密数据库备份中。
- 日志、错误截图、Git diff、shell history 和进程参数都不得出现真实令牌。
- 本次按用户要求保留 Sub2API 默认计费行为；GLM-5.2 的 fallback 价格不代表智谱真实账单。
