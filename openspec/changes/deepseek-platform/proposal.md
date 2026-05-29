# DeepSeek 平台集成

## Why

用户需求：Sub2API 当前支持 Anthropic、OpenAI、Gemini、Antigravity 四个平台，需要增加国内平台支持。首选 DeepSeek，因其 API 完全兼容 OpenAI 格式、V4 刚发布热度高、集成成本最低。

## What

- 新增 `platform = "deepseek"` 作为独立平台标识
- 支持 API Key 类型账号（base_url + api_key）
- 复用 OpenAI 兼容转发逻辑（/v1/chat/completions）
- 独立的模型白名单（V3、V4-Flash、V4-Pro、R1 等）
- 预留 OAuth 扩展接口（本次不实现）

## Impact

- 新增平台不影响现有 Anthropic/OpenAI/Gemini/Antigravity 平台功能
- 路由层将 DeepSeek 请求分发到 OpenAIGatewayHandler（格式兼容）
- 前端新增 DeepSeek 平台选择按钮和 API Key 表单
- 无需数据库 schema 变更
