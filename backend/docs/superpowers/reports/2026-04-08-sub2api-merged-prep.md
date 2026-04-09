# Sub2API merged 提交准备说明

## 当前结论

- 现在已经没有阻碍开始 `merged` 提交准备的硬阻塞。
- 当前状态已经从“还差主热点链”切到“只剩提交前说明与人工确认项”。
- 尚未提交的仅剩两处准备性变更：
  - `backend/cmd/server/wire_gen.go`
  - `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`

## 可按 merged 看待的主干链

- 渠道定价 / 模型映射 / 限制主链
  - `channel_service`、`ModelPricingResolver`、handler glue、usage 写回已重新收口。
- 统一计费写回主链
  - `GatewayService.RecordUsage` / `RecordUsageWithLongContext`
  - `OpenAIGatewayService.RecordUsage`
  - `BillingService` 的共享成本复用层
  - OpenAI compatibility 入口上的 `ChannelUsageFields -> RecordUsage` 已重新接通。
- OpenAI 普通调度热路径
  - 最近几轮已收回明显的非 upstream 分叉，并保留必要 guardrail 边界。
- Sora 移除主链
- usage/admin 过滤与基础展示主链

## 必须保留说明的本地差异

- OpenAI `active/exhausted/-Sys` guardrail
  - 包括 target-group、tool continuation、相关 handler/service 入口保护语义。
- routing observability 超集链
  - `routing_*` usage 写库、ops/request details、相关管理端钻取能力。
- 用户侧 pricing factor 展示链
  - `BillingTier`、`PricingSource`、effective multiplier / unit price 等解释层字段。
- `sub2api-openai` OpenCode 推荐配置链
  - 后端 metadata mirror、Codex OAuth 过滤、前端 `UseKeyModal` 生成链。

## 提交前确认项

- 确认 `wire_gen.go` 的签名对齐修复一并带入最终提交。
- 在 merged 提交说明里明确：
  - 哪些 upstream 主干已吸收并可视为 merged
  - 哪些差异属于明确保留的本地语义，不应被后续 upstream 覆盖
- 最后一轮人工复核建议只盯：
  - `billing_service.go` / resolver 语义是否存在未记录的额外本地偏移
  - 最近两次 checkpoint 之后的少量准备性改动是否都已进入说明范围

## 建议的下一步

1. 先把当前 2 个未提交文件收成一个极小检查点。
2. 然后基于本说明整理真正的 `merged` 提交消息与保留差异说明。

## 2026-04-09 新 upstream 23 提交分桶

### 已等价折叠，不需单独吸收

- `5088e915` `Merge pull request #1417`
- `276f499c` `Merge pull request #1418`
- `5c203ce6` `Merge pull request #1428`
- `47cd1c52` `Merge pull request #1467`
- `06e2756e` `Merge pull request #1501`

说明：这些只是对应功能链的 merge wrapper，不应单独作为同步批次处理。

### 明确不该继续吸收

- `77ba9e72` `Merge branch 'Wei-Shaw:main' into fix/openai-gateway-content-session-hash-fallback`

说明：这是 topic branch 回灌旧 `main` 的中间 merge，不是功能本体；继续吸它只会把热点文件旧快照重新卷进来。

### 真正还需下一轮同步的远端尾巴

- Google Search / grounding
  - `c8cfad7c`
  - `0ebe0ce5`
  - `dd5978f2`
  - `d978ac97`
  - 当前状态：已开始吸收，`antigravity/request_transformer.go` 与 `gemini_messages_compat_service.go` 已恢复“函数工具 + Google Search 同时保留”以及 AI Studio `googleSearch -> google_search` 规范化，并补上对应测试。
- 空 base64 图片清理
  - `936fce68`
  - `f00351c1`
  - 当前状态：已开始吸收，`chatcompletions_to_responses.go` 与 `openai_gateway_service.go` 已恢复空 base64 图片输入清理，且 passthrough 非流式路径也重新接回 SSE-to-JSON 识别，相关测试与后端整包验证已通过。
- content-based session hash fallback
  - `c5aac125`
  - `4fb16030`
  - `cf9efefd`
  - 当前状态：已开始吸收，`openai_gateway_service.go` 已恢复 content-based session seed fallback，补上 `openai_content_session_seed.go` 及对应测试，并通过后端整包验证。
- channel cleanup
  - `9151d34d`
  - 当前状态：已开始吸收，渠道计费模式校验已从 `admin/channel_handler.go` 收回 `channel_service.go`，Create/Update 统一走 service 侧配置校验，相关测试与后端整包验证已通过。
- 非流式 SSE 检测扩展
  - `9e515ea7`
  - 当前状态：已开始吸收，`handleNonStreamingResponse` 已把基于 `Content-Type: text/event-stream` 的非流式 SSE 检测放宽到所有账号类型，仅保留 OAuth 的 body heuristic 兜底，并补上 API key 场景测试；后端整包验证已通过。
- Anthropic OAuth 伪装修复
  - `1c9a2128`
  - 当前状态：已开始吸收，`gateway_service.go` 已恢复 Claude Code 风格的 system array masquerade、非 haiku OAuth 所需的 `claude-code` beta 组合，以及对应测试断言；后端整包验证已通过。
- billing header / CCH signing
  - `e51c9e50`
  - `b982076e`
  - 当前状态：已开始吸收，`gateway_billing_header.go` 与测试已补回，`enable_cch_signing` 设置链已重新接到 backend/frontend，`GatewayService` 的 messages / count_tokens 请求也已接回 billing header 版本同步与可选 CCH 签名；后端整包与前端 typecheck 已通过。
- 低风险尾巴
  - `7060596a`
  - `f54e9d0b`
  - `0d69c0cd`
  - 当前状态：已开始吸收，Go 已提升到 `1.26.2`，CI/Docker 构建镜像版本同步更新，`VERSION` 提升到 `0.1.110`，README 也补回上游的 sponsor/Sora 状态说明；后端整包验证已通过。

### 下一批建议

- 首推 **Google Search / grounding** 这一批。
- 原因：当前本地代码中缺口明确、影响面集中、且不落在已确认保留的本地主差异链上。
