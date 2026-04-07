# Sub2API merged 提交草案

## 推荐提交顺序

1. 先提交当前 3 个准备性文件。
2. 再创建真正的 merged 提交。

这样可以把“编译口修复 + merged 说明文档”与“标记主干已 absorb/merged”的语义分开，回溯更清楚。

## 建议的 merged 提交消息

### 推荐 subject

`chore(upstream): 标记渠道计费与网关主链已完成 merged 收口`

### 建议 body

`对齐 channel/pricing/restriction、统一计费写回、OpenAI 与 Anthropic/OpenAI compatibility 入口接线，并收口普通调度热路径的主干分叉。`

`保留 active/exhausted/-Sys guardrail、routing observability 超集、用户侧 pricing factor 展示与 sub2api-openai 推荐配置等已确认本地差异。`

## merged 提交里建议明确写出的范围

- 已可按 merged 看待的主干链
  - 渠道定价 / 模型映射 / 限制主链
  - 统一计费写回主链
  - OpenAI / Anthropic compatibility 入口 glue
  - 普通调度热路径的主干收口
  - Sora 移除主链

- 必须保留说明的本地差异
  - OpenAI `active/exhausted/-Sys` guardrail
  - routing observability 超集链
  - 用户侧 pricing factor 展示链
  - `sub2api-openai` OpenCode 推荐配置链

## 创建 merged 提交前最后建议

- 先提交当前准备性变更：
  - `backend/cmd/server/wire_gen.go`
  - `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`
  - `backend/docs/superpowers/reports/2026-04-08-sub2api-merged-prep.md`
- 然后基于本草案创建真正的 merged 提交。
