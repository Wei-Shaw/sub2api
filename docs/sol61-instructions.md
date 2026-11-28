# GPT-6.1 Sol 默认指令

GPT-6.1 Sol 的请求转发不再在空 `instructions` 时自动补入 Codex 编程助手模板。客户端提供的非空指令继续保留；需要该编程助手行为的客户端可以显式发送自己的指令。

普通 Chat Completions 转为 Responses 并通过 API Key 转发时，客户端的 system 文本会提升至顶层 `instructions`，纯文本消息提升后不再重复放入 input。JSON object 模式和包含图片等非文本内容的 system 消息保留既有兼容处理；developer 消息仍保留在 input 中。没有 system 时使用空指令字段，不添加替代身份或行为提示。

该变更适用于 GPT-6.1 Sol 及受支持的推理强度/compact 别名，不改变其他模型的默认指令，也不删除供 Codex 客户端使用的模型能力描述。已经由客户端显式发送的长指令不会被删除。

账号经第三方 Responses 网关转发时，对方仍可能自行填充空 instructions；本地修复无法关闭第三方的默认模板。上线后应分别复测 Chat 有/无 system、Responses 有/无 instructions，并核对实际命中的账号、响应 usage 和站内账本。总输入 Tokens 包含缓存读取，不能只比较数据库的非缓存 input_tokens。

此修改不涉及数据库迁移或配置开关。回退二进制会恢复此前的默认模板行为。检测工具的评级需要部署后的真实请求复测，本 PR 不承诺评分结果。
