# OpenAI web bridge quick start

This guide takes a Sub2API operator from a working [`codex-chatgpt-web`](https://github.com/miuuyy/codex-chatgpt-web) browser bridge to a usable `chatgpt-web/*` API group.

The bridge is unofficial browser automation. It sends prompts through a signed-in ChatGPT web session, so the ChatGPT plan's web-product availability and limits apply. It does not convert web allowance into OpenAI API credit.

For the full security rationale, reverse-tunnel configuration, native request envelope, and acceptance matrix, see [Strict OpenAI Responses upstreams](./openai-strict-responses-upstream.md).

## Before you start

You need:

- a Sub2API deployment that includes strict Responses forwarding;
- a running browser bridge with a signed-in ChatGPT session;
- an HTTPS route from the Sub2API container to the bridge;
- a secret header or equivalent private transport protecting that route; and
- administrator access to create an account, group, and downstream API key.

Keep the bridge bound to loopback. Never expose its browser debugging port, cookies, launcher control token, or loopback Responses port publicly.

## 1. Prove the bridge first

In the bridge launcher:

1. Sign in to ChatGPT using the launcher-owned browser profile.
2. Install the browser-only models.
3. Run the authenticated launcher smoke test.
4. Confirm the bridge health endpoint reports ready and lists the model tiers available to that account.

Start with `chatgpt-web/light` for the first end-to-end test. Higher-effort lanes take longer and are more sensitive to ChatGPT UI lifecycle changes.

Do not continue to Sub2API configuration until the bridge works directly.

## 2. Connect Sub2API to the bridge

The proven reference topology keeps the bridge on a trusted workstation and uses a restricted reverse SSH tunnel to the Sub2API host:

```text
Client
  -> HTTPS + Sub2API API key
Sub2API
  -> guarded private HTTPS route
TLS proxy on the Sub2API host
  -> restricted reverse tunnel
codex-chatgpt-web on loopback
  -> signed-in ChatGPT browser session
```

You may run the bridge on the Sub2API host only if that host can maintain the bridge's supported graphical browser profile and interactive ChatGPT sign-in. A headless VM deployment is not yet the reference setup.

Before creating the account, verify both sides of the guard:

- the correct private header reaches the bridge health endpoint; and
- a missing or incorrect header returns HTTP 404.

See [Recommended topology](./openai-strict-responses-upstream.md#recommended-topology) and [Add guarded TLS in front of the tunnel](./openai-strict-responses-upstream.md#add-guarded-tls-in-front-of-the-tunnel) for concrete examples.

## 3. Create the bridge group

In **Admin -> Groups**, create an OpenAI group dedicated to the bridge.

Use these initial values:

| Setting | Initial value |
| --- | --- |
| Platform | OpenAI |
| Concurrency | `1` |
| RPM | Low while validating |
| Custom models | Enabled |
| Models | Only tiers reported by the bridge |

Do not advertise a tier that the signed-in account did not expose.

## 4. Create the OpenAI account

In **Admin -> Accounts -> Add account**:

1. Select **OpenAI**.
2. Select **API Key** as the account shape.
3. Enter a clear name such as `ChatGPT Web bridge`.
4. Set **Base URL** to the guarded HTTPS bridge route.
5. Under **Responses forwarding**, select **Strict raw Responses**.
6. Enable **No upstream HTTP authentication** when the private route is protected by the tunnel and secret header.
7. Leave the upstream API key blank in no-auth mode. Do not reuse the downstream Sub2API key.
8. Enable header overrides and add the private bridge header.
9. Disable upstream billing probes for the no-auth bridge account.
10. Keep Responses WebSocket mode off. Strict mode is HTTP/SSE-only.
11. Add the bridge group created in the previous step.

The stored account fields should resolve to:

```json
{
  "platform": "openai",
  "type": "apikey",
  "credentials": {
    "base_url": "https://sub2api.example.com/_internal/codex-chatgpt-web",
    "openai_upstream_auth_mode": "none",
    "header_override_enabled": true,
    "header_overrides": {
      "x-bridge-secret": "replace-with-a-random-secret"
    }
  },
  "extra": {
    "openai_responses_forward_mode": "strict_raw",
    "openai_responses_mode": "force_responses",
    "openai_responses_supported": true,
    "openai_apikey_responses_websockets_v2_mode": "off"
  }
}
```

The field values remain generic. Sub2API does not store ChatGPT cookies, browser credentials, or consumer-session tokens.

## 5. Add model mappings

Add an identity mapping for each tier that the account should advertise:

```json
{
  "chatgpt-web/light": "chatgpt-web/light",
  "chatgpt-web/medium": "chatgpt-web/medium",
  "chatgpt-web/high": "chatgpt-web/high",
  "chatgpt-web/extra-high": "chatgpt-web/extra-high",
  "chatgpt-web/pro": "chatgpt-web/pro"
}
```

These mappings make the models discoverable. Strict forwarding still preserves the original request body.

## 6. Create the downstream key

In **Admin -> API Keys**, create a key bound to the bridge group.

This is the key clients use with the public Sub2API base URL. Keep it separate from:

- the bridge secret header;
- the restricted SSH key;
- launcher control credentials; and
- any OpenAI API key.

## 7. Connect a client

### BYOK Chat

Use:

| Field | Value |
| --- | --- |
| Provider | Custom OpenAI-compatible endpoint |
| Base URL | `https://your-sub2api.example.com/v1` |
| API key | The downstream key created above |

Save the profile, fetch models, select `chatgpt-web/light`, and send a short exact-marker prompt.

Current BYOK Chat releases detect `chatgpt-web/*` models and use native `/v1/responses` with the required per-turn metadata. Older releases sent `/v1/chat/completions` and cannot drive this bridge correctly.

### Native Codex clients

Use the public Sub2API base URL and downstream key. Native Codex Responses requests already include the required turn metadata. A hand-written request must include the metadata described in [Client request envelope](./openai-strict-responses-upstream.md#client-request-envelope).

## 8. Verify the complete path

Do not call the setup complete until all of these pass:

1. `GET /v1/models` lists only the intended `chatgpt-web/*` tiers.
2. A `chatgpt-web/light` request returns HTTP 200, `status: completed`, and the exact marker.
3. An SSE request includes `response.created`, output deltas, `response.completed`, and `data: [DONE]`.
4. A continuation using `previous_response_id` stays on the same strict account.
5. The guarded bridge route returns 404 without its secret header.
6. Bridge HTTP and browser turn counters return to zero after success and cancellation.

Then test the higher-effort tiers you intend to expose.

## Troubleshooting

| Symptom | Meaning | Action |
| --- | --- | --- |
| Model fetch returns the expected model list | Account, group, key, and guarded route are connected | Continue with `chatgpt-web/light` |
| `Invalid redirect value` in BYOK Chat | The BYOK Chat Worker is older than the edge redirect fix | Refresh after updating BYOK Chat to a current release |
| `/v1/chat/completions` returns 502 | The client used Chat Completions against a Responses-only bridge | Use a current BYOK Chat release or another native Responses client |
| `previous_response_not_found` | The bridge restarted or continuation affinity/state was lost | Start a new response chain and inspect bridge restarts |
| `ChatGPT response DOM disappeared` | The browser bridge lost the active ChatGPT response DOM | Retry with `chatgpt-web/light`, verify the bridge directly, and update the bridge before blaming Sub2API |
| Models are missing | Group custom models or account model mappings are incomplete | Compare both lists with the bridge health catalog |
| 401 from Sub2API | The downstream key is wrong or not assigned to the bridge group | Check the client key and group binding |
| 404 from the private bridge route | The secret header is absent or wrong | Check the account header override and TLS proxy guard |

## Operational boundary

- Keep the bridge host awake and the signed-in browser session healthy.
- Monitor Sub2API, the TLS proxy, tunnel, bridge process, and browser session separately.
- Re-run the acceptance checks after any ChatGPT UI, bridge, proxy, or Sub2API update.
- Use one trusted ChatGPT account per bridge or tenant. Do not pool unrelated users through one browser session.
- Treat higher-effort tiers as independently qualified capabilities; one passing tier does not prove every tier is stable.
