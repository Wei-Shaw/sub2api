# Grok 完整整合进度（单 PR）

分支：`feat/grok-complete-integration`（基准 `upstream/main`）

## 已完成阶段

1. **模型目录与可配置映射** — 默认禁止 gpt/claude→grok-4.5；设置项 `grok_default_text_model` / `grok_cross_client_model_map_enabled`
2. **密码登录 + SSO 校验** — `POST .../oauth/password`、`.../oauth/sso-token`；不落库密码/raw SSO
3. **视频按模型族定价** — `groups.video_model_prices` JSONB；计费顺序：模型×分辨率 → 旧三列 → 官方默认
4. **free 档本地用量软门禁 + 支付失败临时下线** — `gateway.grok.free_quota_*`；仅明确 free 的 OAuth 走调度过滤器；402 / spending-limit 403 tempUnschedule；管理端探测不走门禁

## 已完成收尾

5. **媒体与 Voice** — image/video/voice 路由、模型与分辨率计价、异步 video status 完成后一次性计费、错误语义与请求参数规范化。
6. **网关协议** — Grok Responses/chat bridge、tool protocol、cache、web_search、SSE/错误过滤均以 `upstream/main` 实现为底完成；不删除 main 的 compact。
7. **管理端与前端** — OAuth/SSO/密码建号、重新授权、quota probe、调度阈值、模型映射配置和 monitor 入口已对齐。
8. **运行安全** — Redis 跨实例 OAuth 会话与一次性消费、SSO Cookie 域/路径隔离、导入 probe 有界去重队列、媒体资格失败关闭。

## 明确不纳入

- `openai_gateway_grok_active_delta.go`：legacy 的实验性 HTTP 增量优化，依赖独立设置与会话状态存储；其默认开启、强制 store 和多副本一致性风险不满足当前基线。main 的完整请求与 compact 路径保留，故不构成功能回退。
- `openai_grok_timeline.go`：跨平台网关调试时间线基础设施，不是 Grok 协议能力，且 main 无对应依赖。
- `capacity_provider_grok.go`：依赖 legacy 的通用容量预测子系统与迁移，无法独立纳入 Grok PR。
- legacy merge/CI、重复 compact、跨平台大包和删除 main 能力的提交均未搬运。

## personal-dev 覆盖矩阵

| legacy 能力/路径 | 结论 |
|---|---|
| `pkg/xai/models`, OAuth/CLI identity、billing/quota | 已在 PR 按 main 接口重构，并补运行时模型映射配置、官方 CLI 身份和账单绝对金额 |
| `pkg/xai/sso_device`, password/SSO admin flow | 已重构；raw SSO/密码不落库，CookieJar 隔离，token 限长，OAuth 凭证形状统一 |
| `grok_token_provider`, `grok_credential_failure` | main 已有等价刷新和失败分类；PR 补 reauth、模型级 quota block、team cooldown 与刷新竞态保护 |
| `grok_quota_*`, free gate、import probe | 已重构；本地 rolling usage、失败关闭媒体资格、管理探测绕过、有界去重队列均覆盖 |
| `grok_media`, audio/voice、video billing | 已重构；模型族价格、参数规范化、status done 后原子一次性计费和错误语义均覆盖 |
| `openai_gateway_grok*` chat/tools/cache/SSE | 以 main 的 Responses/chat bridge、tool protocol、cache、compact 为准增量增强；未搬运删除 compact 的分叉 |
| web search、admin settings、前端 OAuth/SSO/password/monitor | 已在同一 PR 完成后端路由、计费字段、设置项、入口和回归测试 |
| migration 157/176/181/182/190 | 旧分叉重复编号或清理迁移；main 已有 Grok schema，PR 新字段使用当前迁移序号，不复用旧编号 |
| timeline、capacity provider、HTTP active-delta | 仅这三类明确排除，理由见上节；均非 main 基线功能正确性能力 |

## 原则

以 main 为底重写，不整文件 pick personal-dev；migration 使用新序号（如 217）。

## 验证

- Backend Grok/xAI/redissession、service、repository、admin probe 定向单测通过。
- Frontend typecheck 通过；Grok 定向 Vitest 16/16 通过。
- 全量 frontend 仍有 upstream/main 已存在的 `GroupsView.getLiveCapability` mock 失败，不属于本 PR 引入。

历史阶段 4–8 的本地实现已在上述收尾阶段合并记录；本文件不再保留“待完成”状态，避免与当前单 PR 内容冲突。
