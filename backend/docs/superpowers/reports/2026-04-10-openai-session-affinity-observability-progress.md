# OpenAI Session Affinity / Sticky Observability 进度记录

## 当前完成情况

已完成并提交：

1. `c315408e` `fix(openai): 接入 x-session-affinity 会话信号`
   - 统一 `session_id > conversation_id > x-session-affinity > prompt_cache_key > content_fallback` 优先级
   - 接通 `/v1/responses` 上游 session/cache 注入链

2. `ab93114e` `feat(openai): 生成并透传 sticky 观测摘要`
   - scheduler/service 生成 sticky 观测结果
   - 通过 handler/context/snapshot 传递

3. `e074833e` `feat(ops): 持久化 OpenAI sticky 观测字段`
   - success/error 双链落库 sticky 字段
   - `/v1/chat/completions` success 链补传 `RoutingSnapshot`

4. `75107cd4` `feat(ops): 展示 OpenAI sticky 请求明细`
   - request details 联表查询带出 sticky 字段
   - 明细弹窗直接展示 sticky 来源/结果/父会话键等字段

5. `56d7e477` `feat(ops): 增加 OpenAI sticky 聚合接口`
   - 新增 `/admin/ops/dashboard/openai-sticky`
   - 后端聚合 `sticky_hit_rate`、`sticky_account_switch_rate`、`eval_result_count`、`session_source_count`

6. `89a525d9` `feat(ops): 增加 OpenAI sticky 观测卡片`
   - Dashboard 挂载独立 sticky 卡片
   - 展示 evaluated requests、sticky hits、hit rate、account switch rate、结果分布、来源分布

## 已验证结果

在当前 worktree 上已通过：

- `go test ./internal/handler ./internal/repository ./internal/server/... -count=1`
- `go test -tags unit ./internal/service ./internal/pkg/apicompat -count=1`
- `go build ./cmd/server`
- `pnpm typecheck`
- `git diff --check`

## 剩余边界

按当前计划，主功能链已经跑到卡片挂载完成。

仍可继续增强但不再阻塞当前目标的点：

- 为 sticky 卡片增加更细的 drilldown 交互
- 补更强的前端组件测试
- 根据真实线上数据再微调 sticky 聚合文案和指标说明

## 当前结论

这条 feature worktree 已经形成一个完整、可验证的交付单元：

- 会话信号被真正吃进 sticky 主链
- sticky 结果既能落库，也能在 request details 和 ops dashboard 里看到
- 后端与前端验证都已通过
