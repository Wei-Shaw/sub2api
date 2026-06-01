# Pricing Plaza Spec Delta

## ADDED Requirements

### Requirement: Anonymous Model Plaza Endpoint

The system SHALL expose `GET /api/v1/plaza/models` as an unauthenticated public endpoint that returns the list of `(group, model, pricing)` rows aggregated across all eligible groups, plus a `currency_meta` block carrying the current `BALANCE_RECHARGE_MULTIPLIER`. The endpoint SHALL NOT require any authentication header and SHALL ignore any credentials supplied in the request.

#### Scenario: Anonymous client requests model rows

- **WHEN** an anonymous client (no `Authorization` header, no API key) issues `GET /api/v1/plaza/models`
- **THEN** the response is `200 OK` with a JSON body of shape `{ "rows": [...], "currency_meta": { "balance_recharge_multiplier": <float>, "model_native": "USD", "plan_native": "CNY" } }`

#### Scenario: Invalid auth header is ignored

- **WHEN** a client issues `GET /api/v1/plaza/models` with `Authorization: Bearer not-a-real-token`
- **THEN** the response is `200 OK` and the response body is identical to what an anonymous client would receive

### Requirement: Anonymous Subscription Plan Plaza Endpoint

The system SHALL expose `GET /api/v1/plaza/plans` as an unauthenticated public endpoint that returns subscription plan cards plus the same `currency_meta` block. The endpoint SHALL NOT require authentication.

#### Scenario: Anonymous client requests plan cards

- **WHEN** an anonymous client issues `GET /api/v1/plaza/plans`
- **THEN** the response is `200 OK` with a JSON body of shape `{ "cards": [...], "currency_meta": { "balance_recharge_multiplier": <float>, "model_native": "USD", "plan_native": "CNY" } }`

### Requirement: Group Eligibility for Model Plaza

The system SHALL include a group on the model plaza if and only if **all** of the following hold: `status == active`, `is_exclusive == false`, `subscription_type == standard`, and the group has at least one model after computing the union of `account.Credentials["model_mapping"]` keys over the active accounts associated with that group. Groups failing any predicate SHALL produce zero rows.

#### Scenario: Exclusive group is hidden

- **WHEN** the system aggregates plaza rows and group `G` has `is_exclusive == true`
- **THEN** no rows for `G` appear in the response, regardless of `G`'s account or model configuration

#### Scenario: Subscription-typed group is hidden from model plaza

- **WHEN** the system aggregates plaza rows and group `G` has `subscription_type == "subscription"`
- **THEN** no rows for `G` appear in the model plaza response (such groups belong to the plan plaza)

#### Scenario: Inactive group is hidden

- **WHEN** the system aggregates plaza rows and group `G` has `status != "active"`
- **THEN** no rows for `G` appear

#### Scenario: Group with no accounts produces zero rows

- **WHEN** group `G` is otherwise eligible but has zero active accounts (or all associated accounts have an empty `model_mapping` entry)
- **THEN** `G` contributes zero rows to the response

### Requirement: Model Set From Account Model Mapping

For each eligible group, the system SHALL derive the model set as the deduplicated union of `keys(account.Credentials["model_mapping"])` (wildcard keys excluded) for every account `A` such that `A.status == active` AND `G.id ∈ A.GroupIDs`. The displayed `platform` for each model SHALL be `account.Platform`. The system SHALL NOT include models from `LiteLLM`, hardcoded fallback, or channels that are not present in any associated account's `model_mapping`.

#### Scenario: Two accounts with overlapping models

- **WHEN** group `G` is associated with accounts `A1` (`model_mapping = {"m1":"m1","m2":"m2"}`) and `A2` (`model_mapping = {"m2":"m2","m3":"m3"}`)
- **THEN** the system emits exactly three model rows for `G`: one each for `m1`, `m2`, `m3`

#### Scenario: Disabled account ignored

- **WHEN** group `G` is associated with accounts `A1` (`status=active`, mapping has `m1`) and `A2` (`status=disabled`, mapping has `m2`)
- **THEN** the response contains only the row for `m1`

#### Scenario: Wildcard mapping keys are skipped

- **WHEN** account `A` has `model_mapping = {"claude-*":"claude-3-5-sonnet","claude-opus-4-5":"claude-opus-4-5"}`
- **THEN** the plaza emits only the row for `claude-opus-4-5`; the wildcard pattern `"claude-*"` is ignored

#### Scenario: Account without model_mapping contributes nothing

- **WHEN** account `A` has no `model_mapping` entry in its `Credentials` (or it is empty) and is not on a platform with a default mapping (e.g. Antigravity)
- **THEN** `A` contributes zero models to its associated groups

### Requirement: Price Resolution Chain With Fallback

For each `(group, model)` pair, the system SHALL resolve a base price by: (1) querying the LiteLLM pricing service; (2) if missed, querying the hardcoded fallback table used by the billing service; (3) if both miss, the row SHALL be silently dropped from the response (no error, no placeholder row). The site price SHALL always equal `base × group.rate_multiplier` for token-billed models.

#### Scenario: LiteLLM hit

- **WHEN** LiteLLM returns pricing for `model = "claude-opus-4-5"` and group `G` has `rate_multiplier = 0.8`
- **THEN** the row's `base.input_per_token` and `base.output_per_token` come from LiteLLM, and `site.input_per_token = base.input_per_token × 0.8`, `site.output_per_token = base.output_per_token × 0.8`

#### Scenario: LiteLLM miss, fallback hit

- **WHEN** LiteLLM has no entry for `model = "private-llm-1"` but the hardcoded fallback table has it
- **THEN** the row is rendered using the fallback prices as `base`, with `site = base × group.rate_multiplier`

#### Scenario: Both miss

- **WHEN** neither LiteLLM nor the fallback table contains pricing for `model = "experimental-x"`
- **THEN** no row is emitted for `(any group, "experimental-x")` and the API response does not include a placeholder

### Requirement: Token Model Row Shape

For a model whose LiteLLM/fallback `mode` is anything other than `image_generation` (i.e. token-billed), the system SHALL emit one row per `(group, model)` containing token-prefix price fields and SHALL omit the `image_prices` block.

#### Scenario: Token row fields present

- **WHEN** the response contains a row for a token model
- **THEN** the row contains `billing_type = "token"`, numeric `base.input_per_token`, `base.output_per_token`, `site.input_per_token`, `site.output_per_token`, `multiplier`, `discount_percent`, and the `image_prices` field is absent or `null`

#### Scenario: Discount calculation

- **WHEN** `group.rate_multiplier = 0.8` for a token row
- **THEN** the row's `discount_percent` equals `20.0` (computed as `(1 - rate_multiplier) × 100`, rounded to one decimal)

### Requirement: Image Model Row Shape (Three Tier Columns)

For a model whose LiteLLM `mode == "image_generation"`, the system SHALL emit one row per `(group, model)` carrying three image price tiers (`tier_1k`, `tier_2k`, `tier_4k`) inside a single `image_prices` block. The system SHALL reuse the existing `getImageUnitPrice` function in `billing_service.go` to compute each tier so plaza prices match billing-side resolution. The displayed `multiplier` on the row SHALL equal `image_rate_multiplier` if `image_rate_independent == true`, otherwise `rate_multiplier`.

#### Scenario: Group with image price overrides

- **WHEN** group `G` has `image_price_1k = 0.032`, `image_price_2k = 0.048`, `image_price_4k = 0.064` and `image_rate_independent = false`
- **THEN** the image row's `image_prices.tier_1k.site = 0.032`, `tier_2k.site = 0.048`, `tier_4k.site = 0.064`, and the upstream `base` prices for each tier come from LiteLLM × the existing tier factors (`1.0×`, `1.5×`, `2.0×`)

#### Scenario: Group without image price overrides

- **WHEN** group `G` has all three `image_price_*` fields unset and `image_rate_independent = false`, `rate_multiplier = 0.8`
- **THEN** each tier's `site` equals LiteLLM-`base × tier_factor × 0.8`

#### Scenario: Independent image multiplier

- **WHEN** group `G` has `image_rate_independent = true` and `image_rate_multiplier = 0.5`, `rate_multiplier = 1.0`
- **THEN** the image row's displayed `multiplier = 0.5` and tier site prices use `0.5` rather than `1.0`

#### Scenario: Image and token rows coexist for same group

- **WHEN** group `G` has both token-billed and image-generation models in its account union
- **THEN** the response contains both kinds of rows for `G`; token rows precede image rows in the per-group ordering

### Requirement: Currency Conventions and Toggle Source

The system SHALL return model row prices as numeric values in **USD** (the natural unit of LiteLLM and the fallback table) and subscription plan prices as numeric values in **CNY** (the unit at which `SubscriptionPlan.price` is collected by the payment layer). Both endpoints SHALL include a `currency_meta.balance_recharge_multiplier` field reading the live `BALANCE_RECHARGE_MULTIPLIER` system setting so clients can convert losslessly between display currencies. The endpoints SHALL NOT accept a `currency` query parameter; conversion is a client-side display concern.

#### Scenario: Multiplier reflects current setting

- **WHEN** `BALANCE_RECHARGE_MULTIPLIER` is set to `0.14` at the moment of the request
- **THEN** both endpoints return `currency_meta.balance_recharge_multiplier == 0.14`

#### Scenario: Native units declared

- **WHEN** any plaza endpoint is called
- **THEN** the response declares `currency_meta.model_native == "USD"` and `currency_meta.plan_native == "CNY"`

#### Scenario: No server-side currency switch

- **WHEN** a client passes `?currency=CNY` to either plaza endpoint
- **THEN** the server ignores the parameter and returns prices in the natively declared units

### Requirement: Model Plaza Filter Parameters

`GET /api/v1/plaza/models` SHALL accept three optional query parameters: `group_id` (exact match against `group.id`), `platform` (exact match against `group.platform`), and `q` (case-insensitive substring match against the model name). The `q` parameter SHALL be rejected with HTTP `400` if longer than 64 characters. Filters apply after row aggregation; an empty result is a valid `200` response.

#### Scenario: Group filter narrows result

- **WHEN** the client requests `/api/v1/plaza/models?group_id=12`
- **THEN** the response contains rows only for the group whose `id == 12`

#### Scenario: Search filter is case-insensitive substring

- **WHEN** the client requests `/api/v1/plaza/models?q=opus`
- **THEN** the response contains every row whose model name (case-folded) contains the substring `"opus"`

#### Scenario: Empty filter result

- **WHEN** the client requests a filter combination that matches no rows
- **THEN** the response is `200 OK` with `rows: []` and `currency_meta` populated

#### Scenario: Oversized search string rejected

- **WHEN** the client requests `/api/v1/plaza/models?q=<65-or-more-char-string>`
- **THEN** the response is `400` with an error code indicating the search parameter is too long

### Requirement: Subscription Plan Card Shape

Each card in the plan plaza response SHALL include `id`, `name`, `price`, `original_price`, `discount_percent` (computed as `(1 - price/original_price) × 100` when `original_price > price`, otherwise `0`), `features`, `validity_days`, `group_summary` (summarizing the linked group), and `models` (the union of `keys(account.Credentials["model_mapping"])` over active accounts associated with the linked group, wildcards excluded, capped at the first 50 names with the remainder count exposed as `models_overflow`).

#### Scenario: Plan with original price greater than price

- **WHEN** a `SubscriptionPlan` has `price = 199`, `original_price = 299`
- **THEN** the card's `discount_percent ≈ 33.4` (rounded to one decimal)

#### Scenario: Plan with original price equal to price

- **WHEN** a plan has `price == original_price`
- **THEN** the card's `discount_percent == 0` and the frontend is expected to omit the struck-through original-price line

#### Scenario: Plan models cap

- **WHEN** the linked group's union of account `model_mapping` keys has 75 distinct names
- **THEN** the card's `models` array contains the first 50 names and `models_overflow == 25`

### Requirement: Plan Eligibility for Plaza

The system SHALL include a subscription plan on the plan plaza if and only if `for_sale == true` AND its linked `Group` exists with `status == active`. Plans with missing or inactive groups SHALL be excluded.

#### Scenario: For-sale plan with active group

- **WHEN** plan `P` has `for_sale = true` and its linked group has `status = active`
- **THEN** `P` appears in the plaza response

#### Scenario: For-sale plan with inactive group

- **WHEN** plan `P` has `for_sale = true` but its linked group has `status = "disabled"`
- **THEN** `P` does NOT appear in the plaza response

#### Scenario: Off-sale plan

- **WHEN** plan `P` has `for_sale = false`
- **THEN** `P` does NOT appear in the plaza response

### Requirement: Ordering

The system SHALL order model plaza rows primarily by `group.id ASC`, then within each group by row kind (`token` rows before `image` rows), then by model name ascending (case-insensitive). The system SHALL order plan plaza cards by `sort_order ASC, id ASC` (the existing convention on `SubscriptionPlan`).

#### Scenario: Group ordering by id

- **WHEN** two eligible groups exist with `id = 5` and `id = 12`
- **THEN** all rows for group `5` precede all rows for group `12`

#### Scenario: Token rows before image rows within a group

- **WHEN** group `G` has both token-billed and image-generation rows
- **THEN** every token row for `G` appears before every image row for `G`

#### Scenario: Plan cards by sort_order then id

- **WHEN** plans `P1 (sort_order=10, id=3)`, `P2 (sort_order=5, id=7)`, `P3 (sort_order=10, id=2)` are all on sale
- **THEN** the response order is `P2, P3, P1`

### Requirement: Channel Configuration Decoupling

The plaza SHALL ignore all channel-level configuration when computing displayed model rows, including `channel.model_pricing`, `channel.model_mapping`, `channel_model_pricing` overrides, and `channel.restrict_models` allow-lists. The model set is built exclusively from `account.Credentials["model_mapping"]`. Plaza prices represent the list price for a `(group, model)` pair, not the realized billing price for any specific channel.

#### Scenario: Channel-level price override has no effect on plaza

- **WHEN** a channel under group `G` carries a `channel_model_pricing` row that overrides `claude-opus-4-5`'s output price to `$10/Mtok`
- **THEN** the plaza row for `(G, claude-opus-4-5)` still computes `site.output_per_token = LiteLLM_base × G.rate_multiplier`, ignoring the channel override

#### Scenario: Channel model mapping has no effect on plaza model set

- **WHEN** a channel under group `G` declares `claude-opus-4-5 → claude-opus-4-5-internal` in its `model_mapping`, but no account in `G` has `claude-opus-4-5` in its own `model_mapping`
- **THEN** the plaza response does NOT include a row for `(G, claude-opus-4-5)`
