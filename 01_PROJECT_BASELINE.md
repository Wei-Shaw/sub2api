# Sub2API 项目基线

更新时间：2026-07-11

## 产品与调用方

- Sub2API 是内部 AI API / 视频网关控制面。
- 当前关键调用方是 QCanvas（TapCanvas），使用 API Key 创建视频任务并轮询结果；当前契约不使用 webhook。
- 本轮交付口径是内部 mock 试运行，不是生产 READY。

## 技术与部署

- 后端：Go + Gin，PostgreSQL，Redis，Wire。
- 前端：Vue 3 + TypeScript + pnpm。
- 部署：Docker Compose；Windows / Docker Desktop 试运行优先使用 named volumes，避免 NTFS bind mount 导致 PostgreSQL `initdb/chmod` 失败。
- 构建：官方根 `Dockerfile` 是主线；`Dockerfile.delivery` 只用于当前离线交付兼容，差异必须可解释、可验证、不可永久漂移。

## 当前代码基线

- Repo：`D:\sub2api-trunk`
- Branch：`wujie/video-capture-moat-20260702`
- Cleanup 起始 HEAD：`36de34b81c3a0981fd02fc1dc945d7dc60b587be`
- 本地交付包与 linked worktree 只 ignore、不删除；不得进入 Git。
- 2026-07-11 Grok cleanup 后，业务改动已分批本地提交；工作树应收口为 Git clean。

## 主要设计来源

- `docs/superpowers/specs/2026-07-10-reliability-core-design.md`
- `docs/superpowers/plans/2026-07-10-reliability-core-implementation.md`
- `docs/api/video-gateway-contract.md`
- `docs/superpowers/codex-handoff/deliverables/2026-07-11-GROK-CLEANUP-MERGE-review.md`
