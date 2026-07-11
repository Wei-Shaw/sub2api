# Gateway JavaScript Hooks (JS Handler)

Sub2API can run **per-account JavaScript scripts** to rewrite request and response payloads on the gateway. Scripts execute in an embedded [Goja](https://github.com/dop251/goja) VM with a fixed timeout and fail-open behavior (errors keep the previous body).

This document describes the **sub2api-native** model (script library + account binding). It is inspired by the upstream [cpa-plugin-jshandler](https://github.com/router-for-me/cpa-plugin-jshandler) API, but configuration and activation differ.

中文版见 [JSHANDLER_CN.md](./JSHANDLER_CN.md)。

---

## Table of Contents

- [Quick Start](#quick-start)
- [How Activation Works](#how-activation-works)
- [Storage Layout](#storage-layout)
- [Hook API](#hook-api)
- [Provider Coverage](#provider-coverage)
- [Multi-Script Chain](#multi-script-chain)
- [Admin UI](#admin-ui)
- [Admin HTTP API](#admin-http-api)
- [Error Handling & Limits](#error-handling--limits)
- [Example Script](#example-script)
- [Differences from CPA Plugin](#differences-from-cpa-plugin)

---

## Quick Start

1. **Admin → Settings → Gateway** → open **Gateway JavaScript hooks**.
2. Turn **Enabled** on and set a **timeout** (default `1s`, e.g. `500ms`, `2s`).
3. **Upload** a `.js` file into the script library (max **512 KiB**).
4. **Edit an account** → bind one or more scripts from the library (order matters).
5. Send traffic through that account’s group/key; only bound accounts run hooks.

Hooks never run when:

- Global config `enabled` is `false`, or
- The selected account has no `jshandler_script_id` / `jshandler_script_ids` in `extra`.

---

## How Activation Works

| Layer | Mechanism |
|-------|-----------|
| Global | Setting key `jshandler_config`: `{ "enabled": bool, "timeout": string }` |
| Account | `extra.jshandler_script_ids` (ordered list, preferred) and legacy `extra.jshandler_script_id` (single string) |
| Runtime | Load each ID from the library registry → compile/cache by file mtime → call hooks |

There is **no** directory auto-scan and **no** global `script_paths` list. Every executable script must be uploaded to the library and bound on an account.

---

## Storage Layout

Root directory: `{pricing.data_dir}/jshandler/` (config `pricing.data_dir`, default `./data`).

```
{data_dir}/jshandler/
  registry.json          # library index
  scripts/
    {id}.js              # script source
```

`registry.json` entries include `id`, `name`, `filename`, `created_at`, `updated_at`.

Script IDs are generated as hex UUIDs (no dashes). Allowed pattern: `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`.

---

## Hook API

Scripts define **global functions** (not `module.exports`). Only the following hooks are wired in production:

| Function | When |
|----------|------|
| `on_after_auth_request(ctx)` | After account selection, before the body is forwarded upstream |
| `on_after_nonstream_response(ctx)` | After a full non-streaming upstream response is assembled |
| `on_after_stream_response(ctx)` | For each streaming data payload (and a synthetic header-init call) |

### Not available in production

- **`on_before_request`** — present in the upstream CPA plugin and in unit tests only. Sub2API request paths always call `on_after_auth_request`.

### Return values

- Return a modified **object** (same shape as `ctx`) and set `body` / `chunk` / `headers`.
- Or return a **string** to replace only `body` (request / non-stream) or `chunk` (stream).
- Set `headers[name] = null` or `[]` to delete a header.
- Stream: return an **empty string** `chunk` to **drop** that chunk from the client.

**Request headers:** mutations are written onto the inbound Gin request. Whether a header actually reaches the upstream provider depends on that path’s header construction (many Anthropic/OpenAI paths only forward an **allowlist**, so arbitrary custom headers such as `X-JSHandler-Req` may not appear upstream even though the hook ran). Prefer rewriting **`body`** when you need a reliable, provider-agnostic effect.

**Response headers:** custom headers from non-stream / stream hooks are applied to the client response (hop-by-hop names are ignored).

### Request: `on_after_auth_request(ctx)`

```javascript
{
  id: "request-id",
  body: "...",              // request body string
  headers: {},              // request headers (string or string[])
  url: "",                  // always empty in sub2api
  model: "gpt-4",           // client / original model
  protocol: "openai_chat",  // same as source_format
  source_format: "openai_chat",
  sourceFormat: "openai_chat",
  to_format: "codex",       // inferred upstream family
  toFormat: "codex",
  account_platform: "...",  // when known
  mapped_model: "..."       // when known
}
```

Typical `source_format` values: `anthropic_messages`, `openai_chat`, `openai_responses`, `gemini_native`.

### Non-stream response: `on_after_nonstream_response(ctx)`

```javascript
{
  id: "request-id",
  body: "...",              // full response body
  req: { body: "...", headers: {}, url: "" },
  protocol: "openai_chat",
  headers: {},              // response headers
  chunk: null,
  history_chunks: null
}
```

After a body rewrite, hop-by-hop headers from the script are ignored, and **`Content-Length` is always cleared** on the writer so Go can set the correct length for the rewritten body (clients may still see a recalculated `Content-Length`).

### Stream response: `on_after_stream_response(ctx)`

```javascript
{
  id: "request-id",
  body: null,
  req: { body: "...", headers: {}, url: "" },
  protocol: "openai_chat",
  headers: {},              // response headers (mutable)
  chunk: "...",             // current data payload (not full SSE framing)
  header_init: false,       // true on the synthetic first call
  history_chunks: []        // frozen string array, max 64 recent payloads
}
```

Notes:

- **Anthropic SSE**: `chunk` is the `data:` JSON only; the original `event:` name is preserved when rebuilding the block.
- **OpenAI SSE**: `chunk` is the data-line JSON; empty / `[DONE]` lines are not passed to the hook.
- **Header-init**: one call with empty `chunk` and `header_init: true` before the first real payload so headers can be adjusted without emitting a chunk.
- VM **state persists across chunks** within a stream (you can keep counters, buffers, etc.).

### Console

`console.log(...)` is forwarded to server logs at info level (`jshandler console`).

---

## Provider Coverage

Request hooks run on major gateway paths after account selection, including:

- Anthropic Messages (including Antigravity forward)
- OpenAI Chat Completions
- OpenAI Responses (HTTP + WebSocket ingress)
- Gemini native (`v1beta`)
- OpenAI ↔ Anthropic compatibility layers

Response hooks (non-stream + stream) are wired for the same surfaces where the gateway owns the upstream response.

### Second request-hook on protocol conversion

When the gateway **re-encodes** the body into another upstream shape, `on_after_auth_request` runs **again** on the converted body so scripts see the actual upstream payload:

| Conversion | Second `source_format` |
|------------|------------------------|
| Claude Messages → Gemini | `gemini_native` |
| OpenAI Responses → Chat fallback | `openai_chat` |

### Content moderation

If a request hook changes the body, content moderation is **re-checked** on the rewritten body before forward.

---

## Multi-Script Chain

1. Resolve ordered IDs from `extra.jshandler_script_ids` (duplicates removed, first wins).
2. For each ID, load the library script and run the hook.
3. Pipe `body` / `headers` / `chunk` to the next script.
4. A failed or missing script is **skipped**; the chain continues.
5. Stream: if a script drops a chunk (`DropChunk`), later scripts in the chain do not see that chunk.

---

## Admin UI

### Settings → Gateway

- Enable / disable
- Timeout
- Script library: list, upload `.js`, preview (syntax highlight), edit name/source, delete

### Edit Account

- Ordered multi-select of library scripts (add, remove, reorder)
- On save:
  - `extra.jshandler_script_ids = [...]`
  - `extra.jshandler_script_id = first id` (legacy compatibility)
  - empty selection removes both keys

---

## Admin HTTP API

Base path (admin auth required): **`/api/v1/admin/gateway/jshandler`**  
(Exact admin prefix follows your deployment’s API mount; routes are registered under the admin group as `/gateway/jshandler`.)

Response envelope: `{ "code": 0, "message": "...", "data": ... }` (`code: 0` = success).

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/config` | Read config (`data.config` is a JSON **string** of `{enabled,timeout}`) |
| `PUT` | `/config` | Body: `{ "enabled": bool, "timeout": "1s" }` |
| `GET` | `/scripts` | List library entries |
| `POST` | `/scripts` | Multipart: `file` (required, `.js`), optional `name` |
| `GET` | `/scripts/:id` | Metadata + `content` |
| `PUT` | `/scripts/:id` | JSON: `{ "name"?: string, "content"?: string }` (at least one) |
| `DELETE` | `/scripts/:id` | Remove from registry and disk |

### Common error reasons

| Reason | Meaning |
|--------|---------|
| `INVALID_SCRIPT_ID` | Bad id format |
| `SCRIPT_NOT_FOUND` | Unknown id |
| `EMPTY_SCRIPT_CONTENT` | Empty / whitespace-only content |
| `SCRIPT_TOO_LARGE` | Content over limit |
| `NO_SCRIPT_CHANGES` | Update with no effective change (e.g. empty name only) |
| `JSHANDLER_UNAVAILABLE` | Service not configured |

Upload size limit: **512 KiB**. Runtime load refuses files larger than **8 MiB**.

---

## Error Handling & Limits

| Topic | Behavior |
|-------|----------|
| Missing hook function | No-op; keep input |
| Timeout / compile / runtime error | Log warning; keep previous chain state |
| Hot reload | Recompile when script file mtime changes |
| Config cache | ~60s TTL; invalidated on admin config update |
| Default timeout | `1s` per hook budget (stream: per chunk after session open) |

---

## Example Script

```javascript
function on_after_auth_request(ctx) {
  // Tag body with source format for debugging
  try {
    var o = JSON.parse(ctx.body || "{}");
    o._js_source = ctx.source_format || "";
    ctx.body = JSON.stringify(o);
  } catch (e) {}
  return ctx;
}

function on_after_nonstream_response(ctx) {
  if (typeof ctx.body === "string" && ctx.body.indexOf("hello") >= 0) {
    ctx.body = ctx.body.replace(/hello/g, "hello-js");
  }
  return ctx;
}

function on_after_stream_response(ctx) {
  if (ctx.header_init) {
    // Optional: adjust response headers only
    return ctx;
  }
  if (typeof ctx.chunk === "string") {
    ctx.chunk = ctx.chunk.replace(/hello/g, "hello-js");
  }
  return ctx;
}
```

---

## Differences from CPA Plugin

| Topic | CPA `cpa-plugin-jshandler` | Sub2API |
|-------|----------------------------|---------|
| Config | YAML `script_paths[]` + timeout | DB `enabled` + `timeout` only |
| Who runs scripts | Global path list for all traffic | **Per-account** library IDs |
| `on_before_request` | Supported | **Not wired** |
| Storage | Arbitrary paths | `{data_dir}/jshandler/` registry |
| Built-in samples | Plugin `scripts/` | None; upload manually |
| Admin | Config files | REST + Settings + account UI |
| Stream history | Full history | Cap **64** chunks |
| Extra ctx | Formats / model | Also `account_platform`, `mapped_model`, stream `header_init` |

The tree `cpa-plugin-jshandler/` in this repository (if present) is **reference only** and is not loaded by the gateway.

---

## Related Code

- Engine / hooks: `backend/internal/service/jshandler/`
- Gateway wiring: `backend/internal/service/gateway_jshandler.go`, `backend/internal/handler/jshandler_*.go`
- Admin API: `backend/internal/handler/admin/jshandler_handler.go`
- Account extra keys: `backend/internal/service/jshandler_account.go`
- Frontend: Settings gateway card + `EditAccountModal` script binding
