# Anthropic 严格透传

严格模式面向原生 Anthropic API Key。有效请求的业务 JSON 与上游响应正文按原始字节转发；认证由网关替换，站内准入、余额、并发、计费和审计继续执行。

## 开启

在账号管理中为 Anthropic API Key 账号开启「严格透传（替换认证）」，即 `extra.anthropic_passthrough=true`。沿用上游的显式开启规则；未设置、false 或类型无效时仍使用现有兼容模式。OAuth、Setup Token、Bedrock 和 Antigravity 不属于此模式。无需数据库迁移。

严格模式忽略账号模型映射、头覆写、工具混淆和预热拦截；组级策略若要求修改正文则明确拒绝，不静默降级。请停用该组的模型重写、reasoning effort 改写和消息变换，并把需要使用的真实模型名纳入准入/计价配置。关闭严格模式仍会启用其他兼容清理；关闭不是修复签名问题的方法。

## 已修复

- Messages 与 count_tokens 保留空文本、thinking / redacted_thinking / signature / data、工具参数、defer_loading 缓存标记、context_management、block_binding、fallbacks 和未知字段。
- 严格账号拒绝需要修复的畸形 JSON，业务正文不剥离模型 `[1m]` 后缀；有效 JSON 不重新序列化。账号选择和计价仍使用上游原有的规范化模型标识；兼容账号保留 JSON 修复与模型正文规范化。
- 严格模式跳过历史思考预过滤与签名失败后的删块、转文本重试；不靠显式 thinking 配置推断历史签名能否保留。
- 传递 query、业务扩展头、上游状态码与错误正文；不跟随 3xx 重定向，不因上游 HTTP 错误更换账号重试。
- SSE 保留 LF/CRLF、注释、event/id/data、末尾未换行的内容；不注入本地 ping，流中断后仍结算已观测到的 usage。
- 响应工具名还原仅在确实存在本次请求映射的协议 name 字段执行，不扫描或改写文本、参数、签名及不透明数据。
- 仅有一个配置账号且为可用严格 Anthropic API Key 的组，可直接访问上游 Models、模型详情和 Files 的列表、上传、详情、下载、删除。分页和 multipart/binary 字节保留。

## 准确的边界

这里承诺的是通过网关准入的原生业务请求。网关仍校验 API Key、JSON、模型可用性和额度，保留安全限制、请求/响应大小及流超时限制；不承诺所有错误请求都绕过本地校验。上游响应压缩由本跳协商为 identity，避免压缩正文破坏 SSE 观察与计费。HTTP 连接、压缩、传输分块、头字段大小写/顺序与 TLS 不属于业务字节透传范围。认证头与 Cookie/Cookie2 在两个方向都不转发；查询中的 key/api_key、hop-by-hop 头以及响应 Set-Cookie/Set-Cookie2 不跨边界传递，缺失的 Content-Type/Anthropic-Version 可补默认值。

Files 属于上游账号的共享命名空间，不是每个站内 API Key 独立的文件空间。仅应把需要共享此命名空间的调用方放在该组；多账号池保持本地汇总 Models，Files 不随机选账号。新增端点不包含 Message Batches 等全部 Anthropic API，不应将此功能称为整个 Anthropic 平台的全 API 代理。

前置反代必须将 `/v1/messages`、`/v1/messages/count_tokens`、`/v1/models` 及其详情路径、`/v1/files` 及子路径直接交给本网关。关闭前置模型目录/请求/响应适配器，以及基于 `/v1/models` 的重复 auth_request；本应用已完成 API Key 鉴权，额外用上游模型目录鉴权会使目录故障影响所有请求。响应缓冲关闭，保留足够超时，避免对业务 JSON/SSE 做 sub_filter。

若链路还经过另一套 Sub2API，对方也必须应用修复并启用严格模式。本站不能恢复已经被下一跳删除的字段或改写的签名。验证应包括原始字节对照、真实签名双向回放、篡改签名拒绝、工具名与参数字面量、流式 usage 落库；仅看 200 或 input_transformations=[] 不足以证明透传。

## 回归命令

```sh
cd backend
go test -tags=unit ./... -count=1
go vet -tags=unit ./...
go mod verify
```

前端执行 locale 完整性检查、Vue 类型检查与生产构建。验证必须区分 Mock 字节一致性测试与真实 Anthropic 签名验收；前者不证明后者通过。
