# AI 协作记忆

> 压缩记忆全文见 `docs/ai/context/20260619-151920-sub2api-compressed-memory_CN.md`。
> 后续新增长期上下文统一新建到 `docs/ai/context/YYYYMMDD-HHMMSS-*.md`，不要覆写、重命名或删除历史文档。

## 核心定论

- 采用“三项目串联方案 A”：Sub2API 是唯一公网 API 入口，也是唯一用户 Key、计费和用量事实源；CLIProxyAPI 只作为内网账号池、OAuth、协议转换和轮询上游；yui.web/shop 只保留展示、说明和跳转。
- 当前主链路：`Cloudflare Tunnel -> nginx 127.0.0.1:8080 -> Sub2API 127.0.0.1:18080 -> CLIProxyAPI 127.0.0.1:8317`。
- 不要让 Sub2API、yui.web、CLIProxyAPI 同时对同一个用户 Key 做状态判定或扣费。
- 不要在文档、提交或日志中记录完整 API Key、内部 token、HMAC secret。

## 运行态摘要

- `aaccx.pw/shop` 归 yui.web；`aaccx.pw/v1/*`、`/api/*` 和 Sub2API 控制台路由归 Sub2API；`api.aaccx.pw` 也是 Sub2API 入口。
- CLIProxyAPI 是聚合上游，不是单个静态 OpenAI Key；作为 Sub2API 上游账号时必须启用 `credentials.pool_mode=true`，并让 401/403/429 在同账号内重试。
- 套餐分组显示名为 `codex-pool-19-usd`、`codex-pool-29-usd`、`codex-pool-49-usd`，分别对应每日 19/29/49 USD；`codex-pool-local-unlimited` 是本机自用无限额分组。
- yui.web 旧 Key 已按 `orders` 迁入 Sub2API；旧邀请码和旧 API Key 发放业务应退役为 410/只读历史，不要继续写入。
- `15951875192@phone.com` 是管理员和本机 Codex Local Key 所属账号，不要按普通用户删除；如需隐藏，用角色筛选或备注标识。

## 易踩坑

- 前端 chunk 命名已从源头改为 `pkg-*`；不要再把真实 `/assets/pkg-*` 反向 rewrite 到 `/assets/vendor-*`。公网入口兼容只保留 `app-index-* -> index-*` 方向。
- Docker 构建需要复制 `docs/legal/*.md`，否则前端 raw import 会失败。
- 更新 Sub2API account credentials 时要带回 `base_url` 等非敏感字段；后端只保留未提交的敏感字段，非敏感字段会被 incoming credentials map 覆盖。
- 当前 SMTP 未配置，注册验证码和忘记密码邮件不能真实发送；忘记密码实现是邮件重置链接 token，不是用户手输验证码。
- `/purchase` 在未配置支付服务商时展示手动收款码，引导用户去 `/redeem`；该路径不创建支付订单、不自动开通订阅、不写账单或用量。

## 运行记录

- 2026-06-19：`18405650929@phone.com` 用户存在且状态 active，已有 active API Key（掩码 `sk-yui-l...OQjSJH`），绑定 `codex-pool-19-usd`，订阅 active 且到期时间 `2026-07-17 16:06:37.531+08`；使用该 Key 访问 `https://aaccx.pw/v1/models` 和 `https://api.aaccx.pw/v1/models` 均返回 200，模型列表包含 `gpt-5.5`、`gpt-5.4` 等 10 个模型。结果见 `docs/ai/context/20260619-152007-18405650929-api-key-public-models-result_CN.md`。
