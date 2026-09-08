# Cyber 会话屏蔽的身份边界

`cyber_session_block_enabled` 控制上游实际返回 `cyber_policy` 后的本地会话屏蔽。仅凭请求历史长度或共享来源信息，不判定会话已被封锁。

## 支持的身份

本地封锁按认证后的 API Key ID 和明确会话 ID 派生独立哈希，按以下顺序读取请求头：

- `session-id`
- `session_id`
- `conversation_id`
- `X-Session-Id`
- `X-OpenCode-Session`
- `X-Conversation-ID`

同一会话在 HTTP / WebSocket 重连时应保持 ID 稳定。共用一个 API Key 的转发网关必须为不同下游用户的不同会话提供互不复用的 ID，例如由可信网关按用户与会话生成不透明标识。客户端声明的会话 ID 不是用户认证凭据；本机制用于抑制重复请求，不替代上游内容审核。

`prompt_cache_key`、`X-Session-Affinity`、IP、User-Agent、历史相似度和条目数均不构成封锁身份。没有明确会话 ID 时，不写入或查询跨请求的会话封锁；上游真实策略拒绝仍正常透传并记录。同一 WebSocket 连接已有的命中后阻断语义保留。

## 升级影响

新的会话封锁使用独立的 v2 哈希命名空间。旧版无法区分显式会话 ID 和缓存 key，不能安全继承其封锁记录；已有记录留在 Redis 中按原 TTL 自行过期，不迁移或删除。升级后的新命中按配置 TTL 重新记录。

滚动升级期间，旧实例仍执行旧逻辑；需要所有实例运行修复版本才能消除该误拦。本变更不自动解除下游网关已经记录的用户或会话限制。

问题记录：[上游 issue #6831](https://github.com/Wei-Shaw/sub2api/issues/6831)。
