# cyber 会话屏蔽用户白名单

## 修改范围

- `backend/internal/service/domain_constants.go`
- `backend/internal/service/settings_view.go`
- `backend/internal/service/setting_parse.go`
- `backend/internal/service/setting_update.go`
- `backend/internal/service/setting_gateway_runtime.go`
- `backend/internal/service/openai_cyber_session_block.go`
- `backend/internal/handler/openai_gateway_handler.go`
- `backend/internal/handler/dto/settings.go`
- `backend/internal/handler/admin/setting_handler.go`
- `backend/internal/handler/admin/setting_handler_update.go`
- `backend/internal/handler/admin/setting_handler_audit.go`
- `frontend/src/api/admin/settings.ts`
- `frontend/src/views/admin/SettingsView.vue`
- `frontend/src/components/common/UserIDWhitelistInput.vue`

## 必要原因

运营需要让指定用户绕过 cyber 会话自动屏蔽。判断必须发生在 gateway 热路径：白名单用户既不能被本地拦截，也不能在上游 `cyber_policy` 命中后写入屏蔽表。上游命中后的风控事件、用量行和 ops 错误日志仍照常落库。

## 配置

共享 settings key `cyber_session_block_user_whitelist`，值为 JSON `[]int64`。空名单表示不对任何人豁免。保存时最多 1000 项，超过上限返回 `INVALID_CYBER_SESSION_BLOCK_USER_WHITELIST`，不会静默截断。gateway 与开关、TTL 一起缓存约 60 秒。

## 验证方式

开启屏蔽后，白名单用户的同一会话标识不会被 `IsCyberSessionBlocked` 拦截，也不会被 `MarkCyberSessionBlocked` 写入 Redis。WebSocket 同连接内的 `cyberBlockedThisConn` 也不会因本 turn 的 cyber 命中而被置位，因此后续 turn 不会被本地关连接。`recordCyberPolicyIfMarked` 仍会写入 cyber 事件。非白名单用户行为保持原样。

## 后续

若 official 原生提供会话屏蔽用户豁免，删除本字段及相关判断，改用官方实现。
