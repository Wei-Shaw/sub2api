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
