# Grok 完整整合进度（单 PR）

分支：`feat/grok-complete-integration`（基准 `upstream/main`）

## 已完成阶段

1. **模型目录与可配置映射** — 默认禁止 gpt/claude→grok-4.5；设置项 `grok_default_text_model` / `grok_cross_client_model_map_enabled`
2. **密码登录 + SSO 校验** — `POST .../oauth/password`、`.../oauth/sso-token`；不落库密码/raw SSO
3. **视频按模型族定价** — `groups.video_model_prices` JSONB；计费顺序：模型×分辨率 → 旧三列 → 官方默认

## 待续阶段

- free-tier / cooldown / payment-required 调度
- media/voice 增量与错误语义
- 网关 tool_choice / active-delta（默认关）/ web_search
- 前端 CreateAccount SSO/密码入口
- 详见子代理审计结论（对照 personal-dev vs main）

## 原则

以 main 为底重写，不整文件 pick personal-dev；migration 使用新序号（如 217）。
