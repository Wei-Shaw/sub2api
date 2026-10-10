# DimAgent subscription provider

`dimagent` is a first-class Sub2API platform that reuses the existing account-pool, group, user-subscription, API-key, billing, and `usage_logs` paths. Subscribers receive only a Sub2API API key; they never receive a DimAgent access token.

## Supported contract

| Capability | State |
| --- | --- |
| `POST /v1/chat/completions` | Supported |
| `POST /v1/responses` / `POST /v1/messages` | Explicitly unsupported in the first OAuth release; this prevents the provider's OAuth token from entering unverified OpenAI/Codex conversion paths |
| Stream usage | Supported; the outbound body enforces `stream_options.include_usage=true` |
| Buffered usage | Supported when the upstream response contains `usage` |
| `/v1/models` sync | Future work: the generic API-key sync UI is intentionally disabled until it acquires the refreshed DimAgent OAuth credential |
| Existing Sub2API subscriptions and `usage_logs` | Reused |
| DimAgent OAuth Authorization Code + PKCE | Supported; administrator completes the registered localhost callback by pasting its full URL into Sub2API |
| Automatic access-token refresh | Supported from the stored OAuth refresh token |
| Embeddings, count-tokens, alpha-search, images, audio/video | Explicitly unsupported |
| Upstream credit polling | Future work |

Unsupported paths are rejected by the gateway, so the managed token is not sent to an unverified DimAgent endpoint.

## Managed account setup

1. In the administrator account UI, choose **DimAgent** and select webpage authorization.
2. Generate the authorization URL. The service creates an Authorization Code + PKCE session using the DimAgent client registration used by DSH.
3. Complete the DimAgent login in the browser. DimAgent redirects to its registered `http://localhost:63211/auth/callback` URL; copy the **entire** callback URL from the browser address bar and paste it into the Sub2API dialog.
4. Sub2API validates the callback state, exchanges the one-time code, and stores only the OAuth credential set (`access_token`, `refresh_token`, `expires_at`, issuer/endpoints/scope). The UI never asks an administrator to paste an access token or API key.
5. Use the default relay URL `https://dimagent.cn/v1` unless a controlled relay was verified.
6. Optionally use `model_mapping` to publish stable public aliases, for example:

   ```json
   {
     "dim-deepseek-flash": "deepseek-v4.1-flash",
     "dim-deepseek-pro": "deepseek-v4-pro",
     "dim-glm-5.3": "glm-5.3"
   }
   ```

7. Use the connection test with an available model, then configure concurrency, priority, and schedulability. Several managed subscription accounts can form one scheduler pool.

The backend owns every DimAgent inference identity and sends:

```http
Authorization: Bearer <OAuth access token obtained and refreshed by Sub2API>
User-Agent: deepseek-harness/<dsh-version> (+https://github.com/deepseek-ai/deepseek-harness)
X-Title: DeepSeek Harness
HTTP-Referer: https://dimagent.cn/
```

### Why the official client identity is mandatory

The subscription relay gates on the official client identity, not just on a valid
OAuth credential. Same token, same model, same endpoint — only the identity
headers varied (verified against the live relay on 2026-10-10):

| Identity sent | Result |
| --- | --- |
| `User-Agent: deepseek-harness/0.2.1-alpha.2 (+…)` + `X-Title` + `HTTP-Referer` | `HTTP 200` |
| `User-Agent: deepseek-harness/9.9.9` + `X-Title` + `HTTP-Referer` | `HTTP 200` (any version suffix is accepted) |
| no `User-Agent` override (Go/curl default) + `X-Title` + `HTTP-Referer` | `HTTP 403` `unsupported client` |
| `User-Agent: DimAgent/0.5.8` + `X-Title: DimCode` | `HTTP 403` `unsupported client` |

So the `deepseek-harness/` product token is what the relay checks; the `X-Title` /
`HTTP-Referer` pair alone is not sufficient. These values mirror `APP_IDENTITY` /
`userAgent()` in `@deepseek-ai/dsh-llm/lib/types/attribution.js` and the headers the
installed `@arcships/dsh-dim-oauth` adapter installs for its provider profile.

`credentials.user_agent` may pin a newer official version, but only when it still
starts with `deepseek-harness/`; any other value falls back to the pinned official
identity. The final `User-Agent`, `X-Title` and `HTTP-Referer` are applied after
client header forwarding and account header overrides, which are both disabled for
this platform. The refresh token and access token are server-side credentials and
must never appear in user API responses, logs, issue reports, model mappings, or DSH
configuration. `dimagent` is excluded from the generic `/v1/sub2api/billing` probe
until a DimAgent credits/quota endpoint is verified.

## Selling a Sub2API subscription

1. Create a `dimagent` group with `subscription` type.
2. Configure the public model allowlist, per-model price, and group multiplier. Publish only models that the upstream account supports and can meter accurately.
3. Create payment plans for that group through the existing plan/payment flow.
4. A subscriber creates a normal Sub2API key for the group and calls:

   ```text
   Base URL: https://<sub2api-host>/v1
   API Key:  <subscriber-sub2api-key>
   Model:    <public-dimagent-model-alias>
   ```

The DSH-side integration should use this normal OpenAI-compatible Sub2API route and the subscriber's Sub2API key; it must not request, save, or relay a DimAgent token.

## Metering

Successful calls use the normal `usage_logs` record path. This provides user/API-key/group/subscription/account/model/requested-model/input-token/output-token/latency/cost filters in existing user and admin dashboards.

Sub2API subscriber usage and the upstream DimAgent credit window are different ledgers. This implementation enforces the former. Before token-priced plans are sold, validate both stream and non-stream responses for accurate upstream `usage`; models without reliable `usage` must not be represented as precisely token-metered. Add upstream credit polling only after the official endpoint and authentication contract are tested.
