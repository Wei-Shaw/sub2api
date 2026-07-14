# Gateway JavaScript Hooks (JS Handler)

Sub2API can run **JavaScript scripts** to rewrite request and response payloads on the gateway. Scripts execute in an embedded [Goja](https://github.com/dop251/goja) VM with a fixed timeout and fail-open behavior (errors keep the previous body).

Binding model:

| Hook | Binding |
|------|---------|
| `on_before_request` | **Group** `jshandler_script_ids` (all API keys in the group) |
| `on_after_auth_request` / response hooks | **Account** `extra.jshandler_script_ids` |

This document describes the **sub2api-native** model (script library + group/account binding). It is inspired by the upstream [cpa-plugin-jshandler](https://github.com/router-for-me/cpa-plugin-jshandler) API, but configuration and activation differ.

中文版见 [JSHANDLER_CN.md](./JSHANDLER_CN.md)。

---

## Quick Start

1. **Admin → Settings → Gateway** → open **Gateway JavaScript hooks**.
2. Turn **Enabled** on and set a **timeout** (default `1s`, e.g. `500ms`, `2s`).
3. **Upload** a `.js` file into the script library (max **512 KiB**).
4. **Edit a group** → bind scripts for `on_before_request` (pre–account selection, all keys in the group).
5. **Edit an account** → bind scripts for after-auth / response hooks.
6. Send traffic with a key bound to that group.

Hooks never run when:

- Global config `enabled` is `false`, or
- The relevant binding is empty (group scripts for before, account scripts for after/response).

---

## How Activation Works

| Layer | Mechanism |
|-------|-----------|
| Global | Setting key `jshandler_config`: `{ "enabled": bool, "timeout": string }` |
| Group | `groups.jshandler_script_ids` → `on_before_request` (once, before SelectAccount) |
| Account | `extra.jshandler_script_ids` (ordered list, preferred) and legacy `extra.jshandler_script_id` |
| Runtime | Load each ID from the library registry → compile/cache by file mtime → call hooks |

There is **no** directory auto-scan and **no** global `script_paths` list. Auth cache snapshot includes group script IDs; group updates invalidate auth cache by group.

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

Scripts define **global functions** (not `module.exports`).

| Function | When |
|----------|------|
| `on_before_request(ctx)` | After moderation, **before** account selection (group scripts) |
| `on_after_auth_request(ctx)` | After account selection, before the body is forwarded upstream (account scripts) |
| `on_after_nonstream_response(ctx)` | After a full non-streaming upstream response is assembled |
| `on_after_stream_response(ctx)` | For each streaming data payload (and a synthetic header-init call) |

### Return values

- Return a modified **object** (same shape as `ctx`) and set `body` / `chunk` / `headers`.
- Or return a **string** to replace only `body` (request / non-stream) or `chunk` (stream).
- Set `headers[name] = null` or `[]` to delete a header.
- Stream: return an **empty string** `chunk` to **drop** that chunk from the client.

**Request headers:** mutations are written onto the inbound Gin request. Whether a header actually reaches the upstream provider depends on that path’s header construction (many Anthropic/OpenAI paths only forward an **allowlist**). Prefer rewriting **`body`** when you need a reliable, provider-agnostic effect.

**Response headers:** custom headers from non-stream / stream hooks are applied to the client response (hop-by-hop names are ignored).

### Request shape (before / after-auth)

```javascript
{
  id: "request-id",
  body: "...",              // request body string
  headers: {},              // request headers (string or string[])
  url: "",                  // always empty in sub2api
  model: "gpt-4",           // client / original model (path model for Gemini native)
  protocol: "openai_chat",  // same as source_format
  source_format: "openai_chat",
  sourceFormat: "openai_chat",
  to_format: "codex",       // empty for on_before_request
  toFormat: "codex",
  account_platform: "...",  // empty for on_before_request
  mapped_model: "..."       // empty for on_before_request
}
```

Typical `source_format` values: `anthropic_messages`, `openai_chat`, `openai_responses`, `gemini_native`.

### `on_before_request`

- Runs **once** per request after the first content moderation check and **before** account selection / channel mapping.
- Failover does **not** re-run before-hooks.
- After body rewrite, content moderation is **re-checked**.
- OpenAI-compatible and Anthropic Messages paths refresh **`model`** and **`stream`** from the rewritten body (invalid/missing `stream` keeps the previous value). Sticky session hashing uses the rewritten body where applicable.
- Anthropic Messages also refreshes Claude Code / haiku-probe context after rewrite.
- **OpenAI `/v1/messages`**: group before + account after-auth on the Anthropic body; after protocol conversion, account hooks re-run on the Responses or Chat Completions body so scripts that rewrite by `source_format` can target the actual upstream payload. Prefer **idempotent** after-auth scripts (or branch on `source_format`) because both stages run.
- **OpenAI Responses WebSocket**: group `on_before_request` on the first client message (handler) and on each subsequent turn **before** account `on_after_auth_request`; account after-auth then Fast Policy run after before on follow-up turns. Channel mapping is recomputed per turn when model changes.
- **Gemini native** (`/v1beta/...`): model/action come from the **URL path**; body rewrites affect payload and sticky hashing only, not path-based routing.

### Non-stream / stream response hooks

Same as account-bound after-auth: see `on_after_nonstream_response` / `on_after_stream_response` with `req`, `headers`, `chunk`, `header_init`, `history_chunks` (max 64). After a body rewrite, hop-by-hop headers from the script are ignored and **`Content-Length` is cleared** so Go can set the correct length.

### Console

`console.log(...)` is forwarded to server logs at info level (`jshandler console`).

---

## Provider Coverage

Request hooks run on major gateway paths:

- Anthropic Messages (including Antigravity forward)
- OpenAI Chat Completions
- OpenAI Responses (HTTP + WebSocket ingress)
- Gemini native (`v1beta`)
- OpenAI ↔ Anthropic compatibility layers

### Second request-hook on protocol conversion

When the gateway **re-encodes** the body into another upstream shape, `on_after_auth_request` runs **again** on the converted body:

| Conversion | Second `source_format` |
|------------|------------------------|
| Claude Messages → Gemini | `gemini_native` |
| OpenAI Responses → Chat fallback | `openai_chat` |

---

## Multi-Script Chain

1. Resolve ordered IDs (duplicates removed, first wins).
2. For each ID, load the library script and run the hook.
3. Pipe `body` / `headers` / `chunk` to the next script.
4. A failed or missing script is **skipped**; the chain continues.
5. Stream: if a script drops a chunk (`DropChunk`), later scripts do not see that chunk.

---

## Admin UI

### Settings → Gateway

- Enable / disable, timeout, script library (list, upload `.js`, preview, edit, delete)

### Groups

- Ordered multi-select of library scripts for `on_before_request`

### Edit Account

- Ordered multi-select for after-auth / response hooks
- On save: `extra.jshandler_script_ids = [...]`, legacy `extra.jshandler_script_id = first id`

---

## Admin HTTP API

Base path (admin auth): **`/api/v1/admin/gateway/jshandler`**  
Response envelope: `{ "code": 0, "message": "...", "data": ... }`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/config` | Read config (`data.config` is a JSON **string** of `{enabled,timeout}`) |
| `PUT` | `/config` | Body: `{ "enabled": bool, "timeout": "1s" }` |
| `GET` | `/scripts` | List library entries |
| `POST` | `/scripts` | Multipart: `file` (required, `.js`), optional `name` |
| `GET` | `/scripts/:id` | Metadata + `content` |
| `PUT` | `/scripts/:id` | JSON: `{ "name"?: string, "content"?: string }` (at least one) |
| `DELETE` | `/scripts/:id` | Remove from registry and disk |

Group create/update accept `jshandler_script_ids` (update: omit = no change; empty array clears).

### Common error reasons

| Reason | Meaning |
|--------|---------|
| `INVALID_SCRIPT_ID` | Bad id format |
| `SCRIPT_NOT_FOUND` | Unknown id |
| `EMPTY_SCRIPT_CONTENT` | Empty / whitespace-only content |
| `SCRIPT_TOO_LARGE` | Content over limit |
| `NO_SCRIPT_CHANGES` | Update with no effective change |
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
function on_before_request(ctx) {
  // Runs for all keys in the group, before account selection
  try {
    var o = JSON.parse(ctx.body || "{}");
    if (o.model === "gpt-4") {
      o.model = "gpt-4o";
    }
    ctx.body = JSON.stringify(o);
  } catch (e) {}
  return ctx;
}

function on_after_auth_request(ctx) {
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
| Who runs scripts | Global path list for all traffic | **Group** before + **account** after/response |
| `on_before_request` | Supported | **Group-bound** (wired) |
| Storage | Arbitrary paths | `{data_dir}/jshandler/` registry |
| Built-in samples | Plugin `scripts/` | None; upload manually |
| Admin | Config files | REST + Settings + group/account UI |
| Stream history | Full history | Cap **64** chunks |

The tree `cpa-plugin-jshandler/` in this repository (if present) is **reference only** and is not loaded by the gateway.

---

## Related Code

- Engine / hooks: `backend/internal/service/jshandler/`
- Group binding: `backend/internal/service/jshandler_group.go`
- Account extra keys: `backend/internal/service/jshandler_account.go`
- Gateway wiring: `backend/internal/handler/jshandler_openai.go`, `jshandler_*.go`
- Admin API: `backend/internal/handler/admin/jshandler_handler.go`
- Migration: `backend/migrations/173_add_group_jshandler_script_ids.sql`
- Frontend: Settings gateway card + Groups form + `EditAccountModal`
