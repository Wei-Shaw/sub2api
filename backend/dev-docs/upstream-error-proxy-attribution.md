# Upstream error proxy attribution

## Scope

Gateway inference errors are stored in `ops_error_logs.upstream_errors`. Each
array item represents one upstream attempt, including same-account retries and
cross-account failover attempts. Proxy attribution is stored on that item; it
must not be reconstructed later from `accounts.proxy_id`.

This repository has one gateway implementation under `backend/internal/service`.
There is no separate `gateway/backend` source copy to synchronize.

## Event contract

Every newly persisted event contains these fields. `proxy_id` is emitted even
when its value is JSON `null`:

| Field | Meaning |
| --- | --- |
| `proxy_id` | Managed proxy ID used by the attempt; `null` for direct or unknown routes. |
| `proxy_name` | Snapshotted proxy name, `direct/no_proxy`, or `unknown`. |

Only proxy ID and name are stored. Proxy URL, protocol endpoint, host, port,
username, password, and authorization data are excluded.

The two sentinel names have strict meanings:

- `direct/no_proxy`: the transport was explicitly given no proxy.
- `unknown`: the route cannot be proven from event-time evidence. This includes
  legacy events, pre-transport credential failures, and OpenAI WebSocket use of
  the default HTTP client.

Invariant: `proxy_id` is `null` if and only if `proxy_name` is one of the two
sentinels. A managed proxy with a blank name (defensive only; `proxies.name` is
non-empty) is labeled `proxy`. Both the struct normalizer used at append time
and the JSON normalizer used on detail reads enforce this, so an event can never
carry `proxy_id=null` together with a real proxy name.

## Why the account snapshot is event-time evidence

Account selection loads `ProxyID` and `Proxy` into one in-memory snapshot. HTTP,
TLS-fingerprint, Bedrock, Gemini, Antigravity, OpenAI, and OpenAI WebSocket
forwarding use that snapshot for managed proxy routing.
Inference transports use a managed proxy only when both the binding ID and its
hydrated proxy object are present; the attribution accessors use the same rule.
OpenAI WebSocket additionally retains the existing default-client environment
proxy behavior when that snapshot does not yield a usable proxy. The transport
does not switch to a database backup proxy during a request.

Expired proxy fallback happens before scheduling by atomically rewriting the
account binding:

- backup proxy: `proxy_id` becomes the backup ID and
  `proxy_fallback_origin_id` stores the expired proxy ID;
- direct fallback: `proxy_id` becomes null and
  `proxy_fallback_origin_id` stores the expired proxy ID.

Custom Anthropic relays receive the same snapshotted proxy as their encoded
relay proxy parameter. OpenAI WebSocket requests keep the existing transport
behavior: configured account proxies use the proxy client, while an empty
account proxy leaves the HTTP client unset so `coder/websocket` uses
`http.DefaultClient`, including its `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`
handling.

For WebSocket errors without a usable account proxy, the event stores
`proxy_id=null` and `proxy_name=unknown`. It does not infer whether the default
client selected an environment proxy or a direct connection after the fact.
This applies to every event produced while a request is on the WebSocket
transport, including WS handshake/fallback errors and the first-output timeout
raised by the WS passthrough relay; `opsUpstreamWSProxyAttribution` is the
accessor for those sites and never returns `direct/no_proxy`.

Credential acquisition failures that occur before the inference transport is
opened are also recorded as unknown. The account's proxy binding is not treated
as proof that the credential operation or an inference request used that route.

Each failure site constructs a complete `OpsUpstreamErrorEvent`. Its literal
sets `proxy_id` and `proxy_name` from the same route input used by the attempt.
Credential-free accessors copy only the managed proxy ID and name. A fallback
proxy needs no special event flag: its event-time ID and name are the historical
route evidence. Shared helpers that assemble the event on behalf of a caller
(`handleUpstreamTransportError`, `handleOpenAIUpstreamTransportError`,
`appendOpenAICompactFallbackRetryOps`, `recordOpenAIRawStreamTruncation`,
`newOpenAIFirstOutputTimeoutError`) stamp attribution themselves, so a caller
cannot forget it. Every non-test `OpsUpstreamErrorEvent{` literal in
`backend/internal/service` must either set `ProxyName` or be passed to one of
those helpers; grep for literals missing `ProxyName` before merging changes
that add a failure site.

`appendOpsUpstreamError` accepts only the completed event. It does not receive
an `Account`, query current account state, or infer proxy attribution while
appending. Once appended, the event contains only scalar snapshot values, so
later account mutation or failover cannot change an earlier event.

Queue sanitization keeps attempt events newest-first under hard bounds:

- the newest 16 attempts retain `detail` and `upstream_response_body` (each
  capped at the queue body limit) plus `upstream_url`/`message` up to 2048
  bytes;
- older attempts keep timestamps, account, proxy, status, kind, stage, scope,
  reason and a `upstream_url`/`message` trimmed to 512 bytes; an attempt whose
  only payload was a cleared `detail` is still retained;
- at most 256 attempts and roughly 512 KiB of serialized JSON are kept per
  request. When either bound drops attempts, the oldest retained event carries
  `dropped_earlier_attempts` with the number of discarded earlier attempts. The
  newest attempt is always retained because it decides the request outcome.

## Legacy events and analytics

Legacy JSON is not rewritten in the database. Detail reads edit only the
`proxy_id`/`proxy_name` keys of events that lack valid attribution, so keys
written by older struct versions and the stored key order are preserved. The
shared parser applies the same rule to the decoded structs. Both materialize
missing attribution as:

```json
{
  "proxy_id": null,
  "proxy_name": "unknown"
}
```

Proxy and region aggregation must use the event fields first. A safe grouping
rule is:

1. A non-null event `proxy_id` groups by that ID and uses the event
   `proxy_name` as the historical label. A join to `proxies` may enrich current
   metadata but must not replace the event label.
2. `proxy_id=null` and `proxy_name=direct/no_proxy` groups as direct.
3. Missing attribution, or `proxy_id=null` and `proxy_name=unknown`, groups as
   unknown. Do not join through the account's current `proxy_id` and present it
   as historical attribution.

If an operator deliberately produces a current-account cohort for legacy data,
the report must be labeled as a current snapshot; it is not historical proxy
attribution.
