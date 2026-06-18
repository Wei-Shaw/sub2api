# 方案 A 本地链路恢复与验收记录

## 背景

本次继续执行方案 A：Sub2API 作为唯一公网 API 入口，CLIProxyAPI 作为内网上游账号池，yui.web/shop 作为展示和跳转入口。

上一轮最小链路验证中，`/v1/models` 和 `/v1/responses` 已经通过，但 Sub2API 转发 `/v1/chat/completions` 时出现 502。错误表现为 Sub2API 将 `cliproxy-local-openai` 账号标记为 `status=error`、`schedulable=false`，错误信息来自 CLIProxyAPI 内部账号池中的某个上游账号 401。

## 根因

CLIProxyAPI 对 Sub2API 来说不是单个静态 OpenAI API Key，而是一个聚合上游账号池。Sub2API 默认按“单账号凭证”处理 API Key 上游：当上游返回 401 时，会认为该账号凭证失效，并把整个 `cliproxy-local-openai` 账号禁用。

这不符合 CLIProxyAPI 的实际语义。CLIProxyAPI 内部某个账号临时 401，不代表 Sub2API 到 CLIProxyAPI 的内部 Key 失效。

Sub2API 代码中已有适合该场景的能力：`Account.Credentials.pool_mode=true`。池模式下，对 401、403、429 等可重试状态码，Sub2API 会优先在同一个账号上重试，并且不会把聚合上游账号直接标记为本地错误。

## 本次配置变更

通过 Sub2API 管理 API 更新账号 `cliproxy-local-openai`：

- 账号 ID：`1`
- 分组 ID：`2`
- `credentials.base_url` 保持 `http://host.docker.internal:8317/v1`
- `credentials.pool_mode` 设置为 `true`
- `credentials.pool_mode_retry_count` 设置为 `3`
- `credentials.pool_mode_retry_status_codes` 设置为 `[401, 403, 429]`
- 清除账号错误状态
- 恢复账号 `schedulable=true`

未在文档中记录完整内部 Key、用户 Key、管理员 Token 或密码。

## 验证结果

本次验收命令均在本机执行，未打印敏感值。

| 验证项 | 结果 |
| --- | --- |
| CLIProxyAPI `/v1/models` 使用 Sub2API 内部 Key | HTTP 200，返回模型列表，`data_count=6` |
| Sub2API `/v1/models` 使用测试用户 Key | HTTP 200，返回模型列表，`data_count=10` |
| Sub2API `/v1/responses` 使用测试用户 Key | HTTP 200，返回 message 输出 |
| Sub2API `/v1/chat/completions` 使用测试用户 Key | HTTP 200，assistant 内容为 `pong` |
| Sub2API 错误 Key | HTTP 401，错误码 `INVALID_API_KEY` |
| Sub2API 管理端用量查询 | HTTP 200，测试用户已有 3 条用量记录 |
| 上游账号状态 | `status=active`、`schedulable=true`、`error_message=""` |

## 后续注意事项

- CLIProxyAPI 作为聚合上游接入 Sub2API 时，必须启用 Sub2API account `pool_mode`。
- 更新账号 credentials 时必须带回 `base_url` 等非敏感字段；Sub2API 的 `MergePreservingSensitiveCreds` 只会保留未提交的敏感字段，非敏感字段会以 incoming map 为准。
- 不建议第一阶段强制设置 `openai_responses_mode=force_chat_completions`，因为当前 `/v1/responses` 已可直接通过。
- 后续公网映射时只暴露 Sub2API，不暴露 CLIProxyAPI 和 yui.web 的 legacy Key 发放接口。
