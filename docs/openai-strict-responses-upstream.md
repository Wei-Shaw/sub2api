# Strict OpenAI Responses upstreams

Sub2API can forward OpenAI Responses requests to an API-key account without rewriting the request body, SSE frames, or upstream response body. This is intended for upstreams that implement the Responses protocol themselves and need the original Codex request envelope.

One working use case is [`codex-chatgpt-web`](https://github.com/miuuyy/codex-chatgpt-web), an unofficial local bridge that drives a signed-in ChatGPT web session. In that arrangement, Sub2API remains the public API gateway while the browser session stays on a trusted workstation or bridge VM.

## What strict mode forwards

Strict mode supports these HTTP endpoints:

- `POST /v1/responses`
- `POST /v1/responses/compact`

It preserves the original JSON bytes and SSE data while applying a narrow header policy. Downstream credentials, cookies, hop-by-hop headers, and credential-shaped headers are never copied upstream. The account can either add its own Bearer credential or explicitly select no upstream authentication.

Strict mode does not add browser automation, ChatGPT authentication, cookie handling, or consumer-product logic to Sub2API. Those responsibilities stay in the upstream bridge.

## Recommended topology

```text
Codex or another Responses client
              |
              | HTTPS + Sub2API API key
              v
           Sub2API
              |
              | HTTPS + private bridge header
              v
  private TLS reverse proxy on the Sub2API host
              |
              | private reverse tunnel
              v
  127.0.0.1:17841 on the bridge workstation or VM
              |
              v
   codex-chatgpt-web + signed-in browser profile
```

Keep the browser bridge bound to loopback. Do not publish port `17841`, a browser debugging port, browser cookies, or launcher control tokens.

For a workstation proof, a restricted reverse SSH identity can expose one listener only on the remote Docker gateway:

```bash
ssh \
  -i /path/to/restricted-key \
  -o IdentitiesOnly=yes \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=15 \
  -o ServerAliveCountMax=2 \
  -NT \
  -R 172.18.0.1:19090:127.0.0.1:17841 \
  bridge-forwarder@your-sub2api-host
```

Restrict that SSH key and user to remote forwarding and the exact `PermitListen` address. Bind the listener to the Docker gateway, not `0.0.0.0` or a public interface. Permit only the TLS proxy container to reach the listener in the host firewall.

## Add guarded TLS in front of the tunnel

A hardened Sub2API deployment may correctly reject an `http://` account base URL. Do not disable URL validation globally. Put the private tunnel behind TLS and require a random bridge header.

The following Caddy pattern reuses an existing valid certificate, adds no public port, and returns 404 when the secret is absent:

```caddyfile
@codex_chatgpt_web_bridge {
  path /_internal/codex-chatgpt-web/*
  header X-Codex-Bridge-Auth {$CODEX_CHATGPT_WEB_BRIDGE_SECRET}
}
handle @codex_chatgpt_web_bridge {
  uri strip_prefix /_internal/codex-chatgpt-web
  reverse_proxy http://172.18.0.1:19090
}

@codex_chatgpt_web_bridge_denied path /_internal/codex-chatgpt-web/*
handle @codex_chatgpt_web_bridge_denied {
  respond "" 404
}
```

Map the TLS hostname to the Compose network gateway only inside the Sub2API container so this hop does not leave the host:

```yaml
services:
  sub2api:
    extra_hosts:
      - "sub2api.example.com:172.18.0.1"
```

Use your actual Compose gateway instead of assuming `172.18.0.1`. Store the bridge secret outside version control. Test both outcomes before adding the account:

- the correct header returns the bridge `/healthz` JSON;
- a missing or incorrect header returns HTTP 404.

## Prepare `codex-chatgpt-web`

1. Install the launcher from the bridge project's official release instructions.
2. Sign in to ChatGPT inside the launcher-owned browser profile.
3. Run the launcher browser smoke test.
4. Install the browser-only models and confirm the bridge doctor reports a healthy loopback Responses proxy.
5. Keep the launcher running and the host awake.

Use a release containing [the effort-popover recovery fix](https://github.com/miuuyy/codex-chatgpt-web/pull/133). Version 2.1.11 without that fix can intermittently observe the current ChatGPT effort slider and then lose it during a popover rerender. Manually opening the menu is not a reliable deployment procedure.

Browser-only mode exposes these model IDs when the signed-in account supports them:

- `chatgpt-web/light`
- `chatgpt-web/medium`
- `chatgpt-web/high`
- `chatgpt-web/extra-high`
- `chatgpt-web/pro`

Availability is determined by the signed-in ChatGPT account. Do not advertise a tier that the bridge capability check did not expose.

## Configure the Sub2API group

Create an OpenAI group for the bridge. A conservative first deployment uses:

- concurrency: `1`;
- a low RPM limit;
- custom model listing enabled;
- only the bridge model IDs actually available to the signed-in account.

Custom model listing filters account-discovered models; it does not invent model IDs. Add identity model mappings on the account for every model you want `/v1/models` to advertise:

```json
{
  "chatgpt-web/light": "chatgpt-web/light",
  "chatgpt-web/medium": "chatgpt-web/medium",
  "chatgpt-web/high": "chatgpt-web/high",
  "chatgpt-web/extra-high": "chatgpt-web/extra-high",
  "chatgpt-web/pro": "chatgpt-web/pro"
}
```

These identity mappings support discovery only. Strict Responses forwarding branches before model mapping and preserves the request body.

## Configure the strict account

Create an account with:

| Setting | Value |
| --- | --- |
| Platform | `openai` |
| Account type | API key |
| Base URL | `https://sub2api.example.com/_internal/codex-chatgpt-web` |
| Responses forwarding mode | `strict_raw` |
| Upstream authentication | `none` |
| Concurrency | `1` initially |
| Header overrides | Enable `X-Codex-Bridge-Auth` with the random proxy secret |
| Groups | The bridge group created above |

The equivalent account fields are:

```json
{
  "platform": "openai",
  "type": "apikey",
  "credentials": {
    "api_key": "unused-because-upstream-auth-is-none",
    "base_url": "https://sub2api.example.com/_internal/codex-chatgpt-web",
    "openai_upstream_auth_mode": "none",
    "header_override_enabled": true,
    "header_overrides": {
      "x-codex-bridge-auth": "replace-with-a-random-secret"
    },
    "model_mapping": {
      "chatgpt-web/high": "chatgpt-web/high"
    }
  },
  "extra": {
    "openai_responses_forward_mode": "strict_raw",
    "openai_responses_mode": "force_responses",
    "openai_responses_supported": true
  }
}
```

The placeholder API key satisfies the API-key account shape but is not sent when upstream authentication is `none`. Never reuse the Sub2API downstream API key as this placeholder or bridge header secret.

## Client request envelope

Native Codex requests already include the required turn metadata. A manual curl test must include the same metadata both in `client_metadata` and on the current user message:

```bash
thread_id="thread_demo_001"
turn_id="turn_demo_001"
metadata=$(jq -nc \
  --arg thread_id "$thread_id" \
  --arg turn_id "$turn_id" \
  '{thread_id:$thread_id,turn_id:$turn_id}')
body=$(jq -nc \
  --arg metadata "$metadata" \
  --arg turn_id "$turn_id" \
  '{
    model:"chatgpt-web/high",
    client_metadata:{"x-codex-turn-metadata":$metadata},
    input:[{
      type:"message",
      role:"user",
      internal_chat_message_metadata_passthrough:{turn_id:$turn_id},
      content:[{
        type:"input_text",
        text:"Reply with exactly: BRIDGE READY"
      }]
    }],
    stream:false
  }')

curl "$SUB2API_BASE_URL/v1/responses" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -H "x-codex-turn-metadata: $metadata" \
  --data-binary "$body"
```

Use `stream: true` to validate SSE. A successful stream includes one `response.created`, one or more `response.output_text.delta` events, one `response.completed`, and the final `data: [DONE]` sentinel.

## Acceptance checklist

Do not call the integration complete until all of these pass through Sub2API:

1. `GET /v1/models` lists the intended `chatgpt-web/*` IDs.
2. A non-streaming prompt returns HTTP 200 and `status: completed`.
3. An SSE prompt preserves event framing and the expected final text.
4. A second request with the first response's `previous_response_id` recalls prior context and stays on the same strict account.
5. `POST /v1/responses/compact` returns a valid compacted response and preserves an asserted fact.
6. Disconnecting a long streaming client cancels the upstream browser turn; bridge HTTP and browser turn counters return to zero without `response.completed`.
7. The TLS bridge path returns 404 without its secret header.
8. The public Sub2API health check remains green after the proxy and account changes.

The browser-only reference deployment used for this guide passed all eight checks with `chatgpt-web/high`.

## Does this use ChatGPT web allowance?

Yes. For a `chatgpt-web/*` request, the bridge sends the prompt through the signed-in ChatGPT browser session. The ChatGPT plan's web-product availability and limits apply; an OpenAI API key or API credit balance is not used for that upstream turn.

Sub2API still performs its own downstream authentication, usage observation, and any group billing rules you configure. ChatGPT can independently rate-limit the browser account, change available tiers, require sign-in again, or change its UI.

This bridge is unofficial browser automation. Do not use it to evade access controls or usage limits. Prefer one bridge per trusted ChatGPT account or tenant instead of pooling one browser session across unrelated users.

## Tools and full mode

This guide validates browser-only mode. Browser-only responses can use capabilities that ChatGPT itself exposes, but they cannot access the local Codex computer or workspace.

The bridge's optional full mode uses an OpenAI Tunnel and a runtime key to connect a native Codex harness. That is a separate privileged setup with a larger trust boundary; follow the bridge project's instructions and test it independently. It is not required for strict Responses forwarding and was not part of the browser-only acceptance matrix above.

## Operational notes

- Keep the bridge, reverse tunnel, and TLS proxy supervised and restartable.
- Monitor bridge `/healthz`, tunnel liveness, and Sub2API health separately.
- A bridge restart loses its local `previous_response_id` replay state; Sub2API affinity cannot reconstruct state the bridge no longer has.
- Re-run the smoke and acceptance checklist after ChatGPT UI changes or a bridge upgrade.
- Treat bridge login state, launcher descriptors, control tokens, proxy secrets, SSH keys, and downstream API keys as separate secrets.
