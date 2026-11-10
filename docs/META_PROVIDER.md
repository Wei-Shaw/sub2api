# Meta Muse Code provider

Meta Muse Code (api.meta.ai) is a first-class provider in this fork. It is not part of
upstream Wei-Shaw/sub2api, so the whole feature lives in the fork keep-set
(AGENTS.md section 2.H).

## What it is

| Item | Value |
| --- | --- |
| Platform id | meta |
| Display name | Meta Muse |
| Gateway | OpenAI gateway (platform_list: PlatformGatewayOpenAI) |
| Upstream base URL | https://api.meta.ai/v1 (a minted key may return its own base_url) |
| Native protocol | Responses only (chat.completions inbound is converted to Responses) |
| OAuth host | https://auth.meta.com, client_id 1031625952748946 (RFC 8628 device grant) |
| Key mint endpoint | POST https://api.meta.ai/muse-code/key with {"dca_token": "..."} |

## Two ways to add an account

### 1. Login with Meta (device flow, recommended)

1. Admin -> Accounts -> Create -> platform **Meta Muse**.
2. Press **Login with Meta**. The gateway requests a device code and shows a
   one-time user code plus a verification link.
3. Open the link, approve the login on Meta. The admin UI polls the token endpoint
   (honouring slow_down) until Meta issues the DCA token.
4. The gateway immediately mints the inference API key and stores the account with:
   - api_key (the inference credential)
   - dca_token (used only to re-mint)
   - base_url, email, name, subscription tier fields

Re-minting later: POST /api/v1/admin/meta/accounts/{id}/mint, or let the gateway mint
automatically on the next request when the stored api_key is missing.

### 2. API key

Create the account with type **API key** and paste the key. No DCA token is stored and
no device flow is involved.

## Request requirements (why the identity headers exist)

Meta attests the client per request. Every inference request carries the Muse Code
client identity; dropping it makes subscription keys rejected:

- User-Agent: muse-build/1.4.14 (non-interactive; <linux|macos>-<x86_64|aarch64>; build dec7d7497fecdad135ec5835a8161dbc92e55b58)
- X-Client-Id: tbh:exec
- X-Tbh-Session-Id / X-Meta-Ai-Gateway-Session-Id / X-Tbh-Thread-Id / X-Tbh-Root-Session-Id: one UUIDv7 per request
- X-Tbh-Agent-Role: main
- X-Tbh-Contributor-Mode: 1 for *-contributor model variants

Tool-bearing Responses requests additionally need the canonical muse namespace
(submit_reminder_decision, strict schema) appended to the tools array. The gateway
appends it without touching caller tool order or indexes.

Unsupported field: web_search tools must not carry search_content_types.

## Models

The model list is discovered live from the upstream `GET /models` endpoint, so it
follows whatever the account's key can actually serve:

- The admin account test picker loads the live list for the account.
- The normal "sync upstream models" action (account form, and the preview used
  before creation) reads the same endpoint.
- Only `muse-*` IDs are kept; the same key can expose unrelated surfaces.
- The picker is not cached: opening "Test account" costs one upstream round-trip
  for that account (and mints a key first when only the DCA token is stored).
  The static fallback covers a slow or failing upstream.

The static fallback (mirrors the captured Muse client release) is used when the
upstream call fails:

muse-spark-1.3, muse-spark-1.3-contributor, muse-spark-1.2, muse-spark-1.2-contributor,
muse-spark-1.1, muse-wonder-1.0, muse-wonder-1.0-contributor, muse-wonder-full-1.0,
muse-wonder-full-1.0-contributor.

The *-contributor variants are special: they take the contributor-mode header.

## Rate limits and quota

Meta returns resets_at (epoch seconds) on throttling:

- 429 with error.resets_at: the account is parked until that instant.
- 429 whose message says "subscription quota"/"quota exhausted", or whose code is
  rate_limit_exceeded/quota together with resets_at: same cooldown, but recorded as
  the credential's subscription quota.
- 404 with resets_at: parked until the reset; 404 without it gets a 5 minute cooldown.
  That last case is deliberate: a key whose plan does not expose a model from the static
  catalog answers 404, and the short cooldown keeps the scheduler from retrying it on
  every turn. A client cannot trigger it, because ingress already rejects models outside
  the group whitelist before an upstream call is made.

## Limitations

- Chat Completions is not a native endpoint; those requests are converted and billed
  on the Responses path.
- There is no usage-query endpoint. Usage is passive: Meta reports the current
  subscription windows in a `response.subscription_usage` event during streamed
  turns, and the gateway samples that into the account's 5h/weekly usage cells.
  A fresh account shows no usage until its first streamed turn completes. The
  event is stripped before it could reach downstream clients, and a
  `subscription` object on a non-stream response is stripped the same way.
  The sampled keys (`meta_usage_*`) are scheduler-neutral, so recording usage
  never touches the scheduler outbox.
- The device flow session store is in-process with a TTL equal to the device code
  lifetime; a restart during login simply requires starting the login again.
- That store holds at most 4096 pending logins. When a new login pushes past the cap,
  the session expiring soonest is evicted first, so an admin login still in progress can
  be dropped and has to be restarted. The cap is deliberate: it bounds memory if the
  admin UI is left polling.
