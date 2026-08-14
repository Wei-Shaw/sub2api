# Errors

## Three shapes, one per protocol

An error is written in the shape of whichever protocol you called, so your
existing client parses it without a special case for this gateway.

### Anthropic paths — `/v1/messages`, and gateway endpoints

```json
{
  "type": "error",
  "error": {
    "type": "permission_error",
    "message": "..."
  }
}
```

`error.type` is one of `authentication_error`, `permission_error`,
`invalid_request_error`, `not_found_error`, or `api_error`.

### OpenAI-compatible paths

```json
{
  "error": {
    "message": "...",
    "type": "insufficient_quota",
    "param": null,
    "code": "insufficient_quota"
  }
}
```

### Gemini paths — `/v1beta`

```json
{
  "error": {
    "code": 403,
    "message": "...",
    "status": "PERMISSION_DENIED"
  }
}
```

### Gateway-level rejections

A request stopped before it reaches a protocol handler — a malformed key header,
for example — uses a flat pair instead:

```json
{
  "code": "api_key_in_query_deprecated",
  "message": "..."
}
```

## Status codes

| Status | Meaning | What to do |
| --- | --- | --- |
| `400` | Malformed request, an out-of-range parameter, or a body over the size limit. | Fix the request. Read `message`; it names the offending field. Text endpoints have a tighter body limit than the image and video ones. |
| `401` | No key, unknown key, or a disabled key. | Check the header. See [Authentication](/docs/authentication). |
| `403` | Key has no group, or its group cannot serve this endpoint. | Ask the operator to assign the key, or call the endpoint your platform supports. |
| `404` | Path does not exist, or the feature is off in this deployment. | Check the [API reference](/docs/api-reference). |
| `429` | Rate limit, concurrency limit, or exhausted quota. | Back off and retry — except on `insufficient_quota`, where retrying cannot help. |
| `5xx` | Gateway or upstream failure. | Retry with backoff. If it persists, it is not your request. |

## Retrying

- Retry `429` and `5xx`. Do not retry `400`, `401`, or `403` — the same request
  will fail the same way.
- Use exponential backoff with jitter. Synchronised retries from many clients
  turn one slow moment into an outage.
- Treat `insufficient_quota` as terminal. It is a `429` by status, but the cause
  is your balance, not traffic, and no amount of waiting clears it.
- Streaming responses can fail after the first byte. A stream that ends early is
  a failure even though the status line said `200`; handle a truncated stream the
  same way you handle a `5xx`.

## Diagnosing quickly

1. `curl {{SITE_ORIGIN}}/v1/models -H "Authorization: Bearer $API_KEY"` — if
   this answers, the key and its group are fine and the problem is in the
   request body or the specific endpoint.
2. If it returns `401`, the key is wrong. If `403`, the key needs a group.
3. Compare the model you sent against the list that endpoint returned. A model
   your group does not carry is the most common cause of a request that looks
   correct but is refused.
