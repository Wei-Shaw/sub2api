# CLAUDE.md

Claude Code 在本仓库工作时必须先读本文件。

## 项目说明

Sub2API 是一个 AI API 网关平台。后端 Go，前端 Vue3，数据库 PostgreSQL，缓存 Redis。
发布时前端静态文件通过 `go:embed` 打包进后端二进制，单文件部署。

## 工程规范

详细规范见 `AGENTS.md`，核心要点：

- `origin` = 团队 fork（ddnio/sub2api），`upstream` = 上游（Wei-Shaw/sub2api）
- 日常功能在 `feature/<topic>` 分支开发，不直接推 `main`
- **新功能/改动一律走 git worktree**（见下节），不再原地切分支
- 变更部署方式时，同步更新 `docs/engineering/deployment.md`
- **不要提交任何 config.yaml、密钥、生产密码**

## Worktree 工作流（默认）

本仓库默认用 git worktree 隔离每个改动。Claude Code 用原生 `EnterWorktree` 工具自动建在 `.claude/worktrees/<feature>/`（已被 `.gitignore` 覆盖）。

新建 worktree 后必做的 bootstrap：

1. **拷配置**：`cp ../../backend/config.yaml backend/config.yaml`（路径相对当前 worktree）
2. **装前端依赖**：`pnpm --dir frontend install --frozen-lockfile`
3. **并行跑 dev 时必须改端口**：默认后端 8080、前端 vite 5173，多 worktree 同时跑必须 `export SERVER_PORT=80xx` 和 `pnpm --dir frontend dev -- --port 51xx`，否则端口冲突

跨 worktree 注意：

- 改 `backend/cmd/server/wire_gen.go` 或 `backend/ent/schema/` 前，先 `git worktree list` 确认其他 worktree 没在改同一处（生成文件冲突难合）
- 退出用 `ExitWorktree(action="remove")`；过期目录 `git worktree prune`
- Go build cache 默认全局共享（`~/Library/Caches/go-build`），不必单独管

完整规范见 `docs/engineering/git-workflow.md`。

## 本地开发

```bash
# 后端（需要 Go 1.26+）
cd backend && go run ./cmd/server

# 前端
pnpm --dir frontend dev
```

配置文件放 `backend/config.yaml`（已在 `.gitignore`），连接服务器测试库（Tailscale IP：`100.71.166.42`）。
参考 `docs/engineering/deployment.md` 了解完整配置。

## 部署

生产唯一主机：`ssh tmhhost`（2026-11-10 起）。Router 为 `nanafox-router-formal-router`，回环端口 18083，compose 在 `/srv/nanafox-router-formal/`；Studio 为 `nanafox-studio-formal-app`，18790；网关为 Caddy 容器 `nanafox-gateway`。先读 `docs/engineering/deployment.md` 并运行 `deploy/check-router-production.py` 只读预检。
主机配置（Caddyfile、compose、备份脚本）在私有仓库 `nana-fox/nanafox-ops`，本仓库不放副本。旧 `deploy-server.sh prod` 已退役；旧阿里云新加坡主机不再运行 Router/Studio。没有测试栈，合并 main 不自动发布。数据库自动 migrations 仍需发布前核对备份和兼容性。

## 支付模块

- 已合并 main（2026-03-23）
- 支付服务商：**微信支付 Native Pay v3**（官方直连）
- Provider 实现：`backend/internal/payment/provider/wxpay.go`
- 备用 Provider：`backend/internal/payment/provider/easypay.go`（易支付，可切换）
- 通过 `payment.provider` 配置项切换（`wxpay` 或 `easypay`）
- 微信支付公钥模式（2024年后新商户默认），需配置 `wxpay_public_key_id`（带 `PUB_KEY_ID_` 前缀）
- Migration：`backend/migrations/077_add_payment_tables.sql`
- **密钥不要提交 git**，只放服务器 `/etc/sub2api/` 配置文件里
- 本地密钥文件放 `backend/config/`（已在 `.gitignore`）

## Wire DI

项目使用 Wire 做依赖注入，但 **Go 1.26.1 与 `wire` 生成工具不兼容**，无法运行 `wire gen`。
`backend/cmd/server/wire_gen.go` 需要**手动维护**：新增或修改 Provider 后直接编辑 `wire_gen.go`，不要尝试跑 `wire` 命令。

`ent` 代码生成工具**可以正常运行**（与 wire 不同）：
```bash
cd backend && go generate ./ent/...
```
修改 `backend/ent/schema/` 后需运行此命令重新生成。

## 上游同步

- 上游仓库：`upstream` remote → `https://github.com/Wei-Shaw/sub2api.git`
- 上次同步：2026-03-29，完整 merge upstream/main（v0.1.105），含 TLS 指纹、Privacy 模式、requested_model、Responses↔Anthropic 双向转换等
- **migration 编号说明**：`schema_migrations` 以 filename 为主键，077/078 号存在同号不同内容（我们的 payment + 上游的 requested_model），字典序排列，可安全并存
- **ent 冲突策略**：merge 时接受 upstream 所有 ent 非 schema 生成文件，手工合并 `ent/schema/`，然后运行 `go generate ./ent/...` 重新生成全部

## 关键文件

| 文件 | 说明 |
|------|------|
| `deploy/deploy-server.sh` | 已退役的旧部署脚本（只输出提示） |
| `deploy/docker-compose.yml` | 基础设施（postgres + redis） |
| `deploy/docker-compose.override.yml` | 服务器本地覆盖（端口暴露，不提交） |
| `docs/engineering/deployment.md` | 完整部署文档 |
| `docs/engineering/git-workflow.md` | Git 工作流 |
| `backend/config.yaml` | 本地配置（不提交） |
| `backend/cmd/server/wire_gen.go` | 手动维护的 Wire 注入文件 |
| `backend/internal/payment/provider/wxpay.go` | 微信支付 Native Pay v3 Provider |
| `backend/internal/payment/provider/easypay.go` | 易支付备用 Provider |
| `backend/internal/service/payment_service.go` | 支付业务逻辑（创单、回调、发放权益） |
| `backend/migrations/077_add_payment_tables.sql` | 支付模块 DB migration |
