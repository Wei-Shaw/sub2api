# P0 可靠性内核收口交接

更新时间：2026-07-10 Asia/Shanghai
状态：内部可用 / 待复核
仓库：`D:\sub2api-trunk`
分支：`wujie/video-capture-moat-20260702`

## 结论

P0 可靠性内核的本地代码、测试、静态检查、Testcontainers 集成测试和全分支复核已完成。

- 代码证据 HEAD：`68edec77`
- 文档收口 HEAD：`74c4b1ff`（本交接与最新 HTML 审查包）
- 独立审计：`docs/superpowers/codex-handoff/deliverables/2026-07-10-GROK45-RELIABILITY-AUDIT.md`

没有 push、部署、真实 Provider/支付调用，也没有读取 `.env`、密钥或 token。因此不得表述为生产或产品 READY。

## 本轮收口

- 余额 reservation、账务流水、任务终态结算、Outbox 重试、任务归档/交付和运行时 Wiring 已落地。
- Dry-run reconciliation 和指标记录已接入真实服务链路，不再只有测试桩。
- 全分支复核发现并修复：过期 reservation 的 Outbox 注册、历史账务误漂移、过期任务再次被 worker 领取、reconciliation 新异常饥饿，以及远程交付被误写为本地下载。

## 验证

| 命令 | 结果 |
|---|---|
| `backend/go test ./... -count=1` | exit 0 |
| `backend/go test -tags=integration ./... -count=1` | exit 0（Testcontainers） |
| `backend/golangci-lint run ./...`、`go vet ./...` | exit 0；0 issues |
| `frontend/npx.cmd eslint . --ext .ts,.vue --max-warnings=0` | exit 0 |
| `frontend/npx.cmd vue-tsc --noEmit` | exit 0 |
| `frontend/npx.cmd vitest run --reporter=basic` | exit 0 |
| `python tools/secret_scan.py --include-untracked` | exit 0；无高置信命中 |
| `backend/go generate ./cmd/server` 连续两次 | 同一 SHA-256：`53EA182966735752152BD9F4C6D84CFA95AE31E74E5D5F85E7E1FF8D9C3B121A` |
| `git diff --check` | exit 0 |

前端 Vitest 的既有错误分支日志和 Browserslist 陈旧提示没有造成失败。

## 待复核与风险

1. 尚未在受控 mock-only 本地服务做浏览器截图闭环。
2. 未做真实 Provider、生产数据、支付或部署验证；这些都需要逐项授权。
3. A3–A6 后续阶段未开始。

## 回滚

优先用 `git revert <commit>` 按主题回退。可靠性末两笔代码提交为 `198e87bc`、`68edec77`。禁止 `reset --hard`、`clean`、rebase；已应用数据库 migration 时须用新的向后兼容迁移回退。

## 入口

- [最新审查包](../../../reviews/LATEST_REVIEW_PACKAGE.html)
- [当前目标](../../../goals/03_CURRENT_GOAL.md)
- [实现计划](../../plans/2026-07-10-reliability-core-implementation.md)
- [Grok 独立审计](./2026-07-10-GROK45-RELIABILITY-AUDIT.md)
