# DeepSeek 平台集成 — 高层设计

详见 Superpowers Design Doc: `docs/superpowers/specs/2026-05-29-deepseek-platform-design.md`

## 架构决策

1. **独立 platform 字符串 + 复用 OpenAI 转发逻辑**
   - DeepSeek 有独立的 `platform = "deepseek"` 常量
   - 路由层分发到 OpenAIGatewayHandler（格式完全兼容）
   - 调度、模型白名单、配额与 OpenAI 解耦

2. **API Key 先行，预留 OAuth**
   - 本次仅支持 API Key 类型
   - 新建 DeepSeekTokenProvider，OAuth 方法返回 ErrNotImplemented

3. **无需数据库变更**
   - platform 字段为通用字符串，无需 migration
