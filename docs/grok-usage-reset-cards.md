# Grok usage reset cards

Grok's website can offer a one-use reset card for the shared weekly usage pool.
The account's normal weekly reset time and a Sub2API scheduling cooldown are
separate from this consumable card.

In an OAuth account's quota area, **Grok reset cards** opens a manual Web SSO
flow. Supply the SSO for the Grok account to reset, query the available cards,
and confirm redemption of the selected card. The selected Sub2API account
supplies the proxy only; the supplied Web session determines which Grok account
is queried and reset. Sub2API does not claim that a supplied SSO belongs to the
selected OAuth identity, and does not clear its scheduling cooldown or rewrite
its cached billing after redemption. Refresh quota and recover state separately
if needed.

The SSO is held only for the open dialog and the current server request. It is
not saved in account credentials, snapshots, browser storage, or request caches.
Changing the selected account or closing the dialog clears the SSO and cards.
Only administrators can use the endpoints. No redemption happens on page load,
quota refresh, or without confirmation.

## Protocol and evidence

The official website's `grok_api_v2.GrokBuildBilling` service exposes:

- `POST https://grok.com/grok_api_v2.GrokBuildBilling/GetRemainingResets`
- `POST https://grok.com/grok_api_v2.GrokBuildBilling/RedeemReset`

These use binary gRPC-Web (`application/grpc-web+proto`), not JSON Responses
requests. An empty query is a five-byte uncompressed empty message frame.
The list response's `tokens`, the redeem request's `token_id`, and the redeem
response's `still_redeemable` are field 1. Each ResetToken contains `token_id`
(string, field 1), `validity_start` (Timestamp, field 2), and `validity_end`
(Timestamp, field 3). The final `grpc-status: 0` trailer must be present before
the response is treated as successful.

The contract was checked against the official website's published protobuf
descriptor and a read-only successful list request on 2026-09-09. Redemption is
covered with synthetic protocol and service tests; no real card was consumed
during development. The official Grok Build 1.0.24 release and its published
billing source did not expose an equivalent Build OAuth reset operation. A
Build OAuth credential must not be silently substituted for the Web SSO.

References:

- [Official Web billing descriptor](https://cdn.grok.com/_next/static/chunks/3e0azggdug22h.js)
- [Official Web reset hooks](https://cdn.grok.com/_next/static/chunks/3bspeqfgwzzdy.js)
- [Official Grok Build billing extension](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-shell/src/extensions/billing.rs)
- [grok2api Web quota transport reference](https://github.com/chenyme/grok2api/blob/8913b53/backend/internal/infra/provider/web/quota.go)

The backend pins the destination to grok.com, disables redirects, honors the
selected proxy without a direct fallback, and never forwards this SSO to an
account's custom OAuth upstream. Immediately before redemption it re-queries
the cards with the same SSO and checks that the chosen card is still valid.
Redemption is sent once. Network failures, malformed responses, and missing
success trailers require a fresh query; they never trigger automatic retries.
Session/proxy rejection is surfaced without attempting to bypass website access
controls. The official usage page remains available as a fallback.
