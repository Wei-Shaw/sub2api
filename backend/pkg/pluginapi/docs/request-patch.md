# Anthropic OAuth 请求补丁契约

`anthropic.oauth.request_patch.v1` 是现有插件进程上的可选请求补丁能力。它复用安装、签名、UI、配置、健康检查和生命周期，不提供第二套网络或响应处理引擎。

## 调用范围

匹配 `platform=anthropic` 且 `account_type=oauth`，并按账号 ID 命中该能力的灰度。挂点位于宿主完成 Messages、Count Tokens 或管理员账号测试的最终请求构建之后、实际发送之前。经过现有兼容层委托到这些构建器的请求同样受此挂点约束。

API Key、Setup Token、Vertex、Bedrock、OAuth 登录与 Token 刷新保持现有流程。未启用或未命中灰度时不调用插件。没有原始客户端输入捕获和响应观察 RPC。

OpenAI 与 Anthropic 各自拥有绑定。同一插件声明两种能力时共享进程和配置，管理页启用时统一应用所选灰度；不同插件可独立启停和选择灰度。数据库唯一约束仍以能力、平台、账号类型为作用域。

## RPC

定义位于 `v1/request_patch.proto`：

```protobuf
service RequestPatch {
  rpc PatchRequest(RequestPatchRequest) returns (RequestPatchResponse);
}
```

插件继续实现现有 `TransportPlugin` 生命周期，并实现 `RequestPatchServer`。Go SDK 的 `Serve` 按实现选择性注册新服务，握手及原传输 API 版本均保持 1。仅 Claude 插件可以通过嵌入 `UnimplementedTransportPluginServer` 保留不用的 `Forward` 方法。

| 请求字段 | 内容 |
|---|---|
| `request` | 复用 `ForwardRequestStart` 的请求方法、目标主机及路径、脱敏请求头、账号 ID、并发配置、平台、类型和正文长度 |
| `request.url` | 移除 userinfo、完整查询串和 fragment；不能用于改变路由 |
| `request.headers` | 移除认证、Cookie、代理与连接控制等受保护字段 |
| `request.proxy_url` | 始终为空 |
| `body_json` | 宿主构造完成、尚未发送的最终 JSON 对象 |

| 返回字段 | 应用方式 |
|---|---|
| `body_json` | 空字节串表示不改正文；非空表示替换整个 JSON 对象 |
| `remove_headers` | 先按名称忽略大小写移除对应请求头 |
| `set_headers` | 再忽略大小写清理同名头，按插件给定的名称及多个值写入；空值列表表示移除 |

空补丁对象表示不修改。返回 nil、非对象 JSON、无效头或受保护字段变更均返回明确错误，不发送该请求。

## 限制和保护

- 一次 RPC 最长 5 秒；客户端更早取消或超时会传递给 RPC。
- 输入和替换正文各不超过 64 MiB；gRPC 消息上限额外保留 256 KiB 元数据空间。
- 请求头修改的名称和值总大小不超过 64 KiB，名称及值数量受 256 项限制；拒绝无效名称、换行注入和仅大小写不同的重复 set 项。
- 不允许修改认证、API key、Cookie、Host、代理认证、连接控制、Content-Type、Content-Length、Content-Encoding、Expect、Forwarded/X-Forwarded 等头。
- 不允许新增、删除或改变正文的 `model`、`stream`，因为它们参与宿主路由、计费和响应解析。
- 替换正文后同步 `Body`、`GetBody` 和 `ContentLength`，清理过期的长度头和正文摘要；用于发送和后续诊断的正文一致。
- 完整验证后才构造新请求，错误不会留下部分应用的补丁。

插件不可用、RPC 未实现、超时或补丁无效时失败关闭，禁止静默回落为未打补丁的请求。管理员明确停用后恢复原有宿主路径。真实连接、代理、TLS、重试、响应流、用量和计费均继续使用宿主已有实现。

本能力只提供通用扩展入口，不内置第三方客户端指纹、遥测、账号规则或特定供应商的私有协议。
