# Upstream model pricing for OpenAI pools

Edit an OpenAI API-key account in pool mode and turn on **Sync upstream model prices**. This also enables the account's billing probe. Run a manual probe to check compatibility before sending traffic to the account. The setting is off by default. For admin API clients, set `extra.upstream_billing_model_pricing_sync_enabled` on an account update.

The synchronized prices determine account cost statistics and account quota consumption for supported traffic. Customer charges use the platform's configured tariff. With profit control enabled on a group, scheduling checks the synchronized price against the group's minimum margin and safety buffer. The comparison checks all applicable meters and context bands because usage is unknown at admission time. A missing, invalid, expired, or unrepresentable price makes the opted-in account ineligible for that request. The probe picks up price changes on its next successful refresh. Use the existing probe interval and manual refresh controls to manage that window.

## Provider contract

`GET /v1/sub2api/billing?include=model_pricing&models=model-a,model-b` uses the account API key as a bearer token. `models` is an optional list of up to 64 exact model IDs. The provider can return its bounded catalog when the list is absent. The base billing response retains schema version `1` and its existing multiplier fields. The optional `model_pricing` member looks like this for a flat per-request model:

```json
{
  "version": 1,
  "currency": "USD",
  "models": {
    "model-a": {
      "mode": "per_request",
      "bands": [{"through_context": null, "request_price": 0.01}]
    }
  }
}
```

- `mode` is `token` or `per_request`. `request_price` is USD per request. Token prices are USD per token. Both are before the key's existing resolved rate and peak multiplier.
- `bands` are ordered by inclusive `through_context`. The first band starts at zero. Subsequent bands start one token after the preceding upper bound. The final band uses `null` for no upper bound. Context is the sum of uncached input, cache reads, and cache writes.
- Each token band supplies `token_prices` for `default`, `priority`, `flex`, and `ultrafast`. Each tier supplies `input`, `output`, `cache_read`, `cache_write`, `cache_write_1h`, `cache_write_30m`, `cache_write_12h`, `image_input`, `image_output`, and `image_cache_read`. Use an explicit zero for a free meter. The cache write TTLs are alternative ways of pricing the same write tokens. Image meters describe token usage.
- Optional fields are `effort_rates` per band (multipliers keyed by reasoning effort), and `time_pricing`, `forced_tier` (`priority` or `ultrafast`), and `valid_before` (RFC 3339) per model. Time pricing uses the channel schedule format with `timezone` and `periods` of `start_time`, `end_time`, and `multiplier`.

Sub2API publishes quotes for the authenticated key's OpenAI group when its routes agree on a representable tariff. This includes token, cache, long-context, service-tier, time-dependent, and per-request pricing. Response-model billing, request-time tier rewrite policies, compact routing, and separate media or search charges are outside this contract. Missing model pricing leaves the legacy multiplier probe usable. An upstream must implement this extension before its model prices can be synchronized.
