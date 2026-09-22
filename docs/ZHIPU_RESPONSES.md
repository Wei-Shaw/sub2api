# Zhipu Coding Plan: native Responses

Zhipu documents a dedicated Responses base URL for GLM Coding Plan:
<https://docs.bigmodel.cn/cn/coding-plan/tool/codex>.

In the account editor, select **Zhipu → Coding Plan → Adaptive**. The editor
shows separate Chat Completions, Anthropic, and Responses URLs. The Responses
base URL defaults to `https://open.bigmodel.cn/api/v1`. Sub2API forwards Responses requests to
`https://open.bigmodel.cn/api/v1/responses` through its existing native Responses
HTTP/JSON and SSE handling. A custom base URL remains supported.

The relevant account credentials are:

```json
{
  "account_mode": "coding",
  "api_protocol": "adaptive",
  "base_url": "https://open.bigmodel.cn/api/coding/paas/v4",
  "api_base_urls": {
    "chat_completions": "https://open.bigmodel.cn/api/coding/paas/v4",
    "anthropic": "https://open.bigmodel.cn/api/anthropic",
    "responses": "https://open.bigmodel.cn/api/v1"
  }
}
```

Supply the Coding Plan API key through the account editor's API key field.
The Responses URL is separate from the Chat Completions URL
`https://open.bigmodel.cn/api/coding/paas/v4`.

## Compatibility

- Coding Plan `adaptive` accounts route each inbound protocol to its native
  endpoint. Existing adaptive accounts without a Responses URL use the official
  default automatically; no data migration or additional switch is required.
  This changes their Responses routing from the Chat bridge to the native endpoint.
- Explicit **Responses** mode is also supported with
  `base_url: https://open.bigmodel.cn/api/v1`.
- Pay-as-you-go and accounts without a Coding Plan mode do not advertise this
  capability. Switching the editor to pay-as-you-go resets an explicit Responses
  selection to Chat Completions.
- To return to the previous behavior, select **Chat Completions**
  and use the corresponding Chat Completions base URL.
- This does not claim support for every model or OpenAI Responses feature.
  WebSocket transport, `/responses/compact`, hosted tools, and server-managed
  conversation storage require separate upstream verification. Zhipu requests
  do not inherit the DeepSeek/Kimi-specific rewriting of `store` and
  `previous_response_id`; callers should use the upstream-supported parameters.

## Validation

Offline regression tests cover explicit and adaptive native routing, default and
custom URLs, native JSON/SSE forwarding, function-call history and tool results,
usage accounting, stale probe flags, and account create/edit round trips.

A live smoke test against the official endpoint with `glm-5.3`, `store: false`,
and low reasoning effort verified streaming completion plus a two-request
function-call/tool-result exchange. These live checks validate the upstream
contract; they are separate from the mocked Sub2API gateway tests. Other models,
including `glm-5.3-flash`, are not established by these checks.
