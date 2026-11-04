# antigravity

Antigravity 上游客户端与 Claude/Gemini 请求、响应、流和 schema 转换。文件按 `client`, `oauth`, `request_*`, `response_*`, `stream_*`, `*_types` 前缀组织。

`StreamingProcessor` 保留 SSE byte 输出；同进程协议桥接优先使用 `ProcessLineEvents` / `FinishEvents` typed sink，避免事件重复序列化和解析。

工具结果文字和 base64 图片/文档在同一次解码中提取，媒体位于 functionResponse 后的 inline parts。`FetchAvailableModelsFromURL` 将目录发现固定到生成端点，禁止重定向和端点回退；账号出口上下文由 application 调用方保留。Hub UA 版本仍允许已保存设置和环境变量覆盖。

`system_identity.go` 只中和系统指令开头的客户端身份与 attribution；原生 Anthropic 转发和用户消息不调用该规则。
