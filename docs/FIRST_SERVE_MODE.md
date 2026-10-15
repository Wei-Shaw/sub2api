# 首服模式

首服模式为 OpenAI 账号选择代理出口，并按固定间隔轮换 IP。默认开启，默认每 240 秒切换一次，可在账号配置中改为 1–86400 秒。HTTP/SSE、非流式请求、压缩、图片、向量、搜索、token 计数、Seedance、Live 创建及侧带连接、Responses WebSocket 都使用当前首服代理；自动透传开关保持原值。

## 默认行为

- 新建、导入 OpenAI OAuth、Setup Token 和 API Key 账号时，默认写入首服模式。
- OAuth 账号默认设置 Codex 指纹收敛为 `full`。这不会修改 `openai_passthrough`。
- 首次建立组合时生成一个路由 ID。后续切换代理时更换出口 IP 和设备指纹，复用同一个路由 ID。
- HTTP 与 WebSocket 同账号共享范围默认是 `reuse_scope: "account"`；按会话模式仍可显式选择 `session`。
- `previous_response_id` 保留在客户端请求体中。到达轮换时间后不会因为该字段而继续停留旧代理，也不会把它改成路由 ID。

## 配置

配置保存在 `accounts.extra.openai_first_serve`，无需数据库迁移：

```json
{
  "openai_first_serve": {
    "reuse_scope": "account",
    "rotate_seconds": 240,
    "proxy_mode": "all",
    "proxy_ids": []
  }
}
```

`proxy_mode` 为 `selected` 时至少选择两个属于账号代理组的代理；`all` 使用代理组内所有可用成员。新增或导入账号未指定代理组时，自动选择有至少两个可用成员的启用组；若已指定单个代理，则仅选择包含该代理的组。没有合适的组时仍可保存已开启的账号，界面和导入结果会提示待配置；绑定代理组后即可请求，不会静默直连。

旧配置中的 `ttl_minutes`、`ttft_seconds`、`max_switches` 和 `cooldown_seconds` 仍可读取，便于平滑升级，但不再参与轮换决策；缺少 `rotate_seconds` 时使用 240 秒默认值。

## 轮换流程

1. 首次请求选择代理并生成路由 ID。
2. 到达 `rotate_seconds` 后，在下一次请求或 WebSocket 下一轮消息开始时选择下一个可用代理。空闲时不主动请求上游，正在输出的请求不会被中断。
3. 选择成功后保留路由 ID，生成新的设备指纹，更新当前代理和下一次轮换时间。IP 与指纹使用同一个 `rotate_seconds`，周期内不续期。HTTP 的 `previous_response_id`、输入和工具结果保留；WebSocket 的续接规则见下文。
4. 没有可用替代代理时保留旧代理和旧指纹，并在下一请求再次尝试，不使用首 token 阈值、连续切换上限或冷却暂停。
5. 首 token 仍会记录到状态页用于诊断，但不会触发切换。

首服把路由 ID 写入出站 `session_id`、`session-id`、Responses `prompt_cache_key` 和相关客户端元数据；完全收敛时对外会话标识也统一。不会把不同会话的输入、历史或响应链合并。路由 ID 与每轮上游生成的 `response.id` / 下一轮使用的 `previous_response_id` 是不同字段。

开启指纹收敛的首服账号使用周期内固定的设备指纹（`x-codex-installation-id`）。它在同一账号的 HTTP、WebSocket 握手、Responses 请求体及内嵌 turn metadata 中保持一致；成功换 IP 后生成新值，正在进行的旧请求及其重试继续使用原值。该值仅保存在网关内存和请求副本中，不修改账号持久化的指纹种子或导入的设备 ID。显式关闭指纹收敛时保留原行为。

`session_id`、`prompt_cache_key` 等路由标识继续复用；`turn_id`、响应 ID 等每轮标识仍按原协议更新，不作为设备指纹固定 240 秒。

WebSocket 每轮跟随共享路由的当前代理。换代理时建立新的物理连接；能安全重放完整本地历史时，去掉 `previous_response_id` 并发送完整历史重建响应链，否则保留原字段尝试上游续接，不再因历史不完整推迟换 IP。上游若无法跨连接续接，仍可能要求客户端提供完整历史。切换后的出站路由 ID 保持不变，但状态里的 WebSocket `conn_id` 是物理连接 ID，会变化；HTTP `conn_id` 表示路由 ID。

Live 侧带连接在拨号或重连时选用当前代理，已建立的实时连接持续使用原出口直到结束或重连。管理侧的登录授权、token 刷新、健康检查不属于网关推理转发入口。

## 使用与验证

1. 在代理管理中准备代理组，至少包含两个不同出口 IP。
2. 新建或编辑 OpenAI 账号，确认首服开关已开启，设置切换间隔并选择代理组。
3. 连续发起 HTTP/SSE 或 WebSocket 请求，检查状态页中的“下次 IP 切换”和“已切换”计数。
4. 切换前后比较出站 `session_id` / `prompt_cache_key`，应保持相同；代理地址或出口 IP 以及 `x-codex-installation-id` 应发生变化，同一周期内设备指纹应一致。

运行状态只保存在当前网关进程内。多节点部署时，同账号跨会话共享时，请让该账号的请求落到同一节点；进程重启后会重新建立组合。
