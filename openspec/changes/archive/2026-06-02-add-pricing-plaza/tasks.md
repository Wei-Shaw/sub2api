# Implementation Tasks

## 1. Backend: Pricing Plaza Service

- [x] 1.1 Create `backend/internal/service/pricing_plaza_service.go` with `PlazaService` struct and constructor wired to `PricingService` (LiteLLM lookup), `BillingService` (fallback prices + image-tier resolver), `groupRepo`, `accountRepo` (narrowed via `PlazaAccountSource`), `subscriptionPlanRepo`, and `paymentConfigService`.
- [x] 1.2 Implement `ListModelRows(ctx, filter)` returning `[]ModelPlazaRow` + `CurrencyMeta`:
  - [x] 1.2.1 Query eligible groups (`status=active`, `is_exclusive=false`, `subscription_type=standard`, ORDER BY `id`).
  - [x] 1.2.2 Build `groupID → []models` from accounts: load all active accounts, expand `keys(account.Credentials["model_mapping"])` (wildcard keys stripped), and for every `gid ∈ account.GroupIDs` add the model name; dedup per group; platform = `account.Platform`.
  - [x] 1.2.3 For each `(group, model)` pair, run the price-resolution chain: LiteLLM (`PricingService.GetModelPricing`) → hardcoded fallback (`BillingService` fallback table via a new exported helper) → drop.
  - [x] 1.2.4 Branch on `pricing.mode == "image_generation"`:
    - Token branch: emit a row with `base.input_per_token`, `base.output_per_token`, site-prices = base × `rate_multiplier`, `multiplier = rate_multiplier`, `discount_percent`.
    - Image branch: reuse the same logic as `BillingService.getImageUnitPrice` for tiers `1K`, `2K`, `4K` (expose an internal helper to avoid logic duplication); emit one row with three price columns and `multiplier = image_rate_independent ? image_rate_multiplier : rate_multiplier`.
  - [x] 1.2.5 Apply post-aggregation filters (`group_id`, `platform`, `q` substring on model name, case-insensitive).
  - [x] 1.2.6 Sort within group: token rows alphabetically by model name, then image rows alphabetically.
- [x] 1.3 Implement `ListPlanCards(ctx)` returning `[]PlanPlazaCard` + `CurrencyMeta`:
  - [x] 1.3.1 Query `SubscriptionPlan` where `for_sale=true`, ORDER BY `sort_order ASC, id ASC`.
  - [x] 1.3.2 Resolve linked `Group`; skip plans whose group is missing or not `active`.
  - [x] 1.3.3 Build `models[]` for each card from the union of `keys(account.Credentials["model_mapping"])` over active accounts associated with the linked group.
  - [x] 1.3.4 Pass through `price`, `original_price`, `features`, `validity_days` verbatim (treat as CNY).
- [x] 1.4 Add `CurrencyMeta` populator that reads `BalanceRechargeMultiplier` from `paymentConfigService.GetPaymentConfig()` and emits `{ balance_recharge_multiplier, model_native: "USD", plan_native: "CNY" }`.
- [x] 1.5 Add in-memory cache layer in front of both `List*` methods (60 s TTL, keyed by `multiplier + group revision + plan revision`); invalidate on group / account / plan / setting mutations via existing repository hooks where available.

## 2. Backend: Public HTTP Routes

- [x] 2.1 Add a new `internal/server/routes/plaza.go` registering two routes under `/api/v1/plaza`:
  - [x] 2.1.1 `GET /api/v1/plaza/models` — bound to `PlazaHandler.ListModels`.
  - [x] 2.1.2 `GET /api/v1/plaza/plans` — bound to `PlazaHandler.ListPlans`.
- [x] 2.2 Wire both routes into the **public, unauthenticated** chain (the same chain that serves `/api/v1/settings/public`); explicitly *not* in the `apikey-auth` or `user-auth` chains.
- [x] 2.3 Apply edge rate limiting consistent with existing public endpoints.
- [x] 2.4 Create `internal/handler/plaza_handler.go` with:
  - [x] 2.4.1 `ListModels`: parse optional `group_id`, `platform`, `q` query params; clamp `q` to 64 chars; delegate to `PlazaService.ListModelRows`.
  - [x] 2.4.2 `ListPlans`: no query params in v1; delegate to `PlazaService.ListPlanCards`.
- [x] 2.5 Define DTOs in `internal/handler/dto/plaza.go`:
  - `ModelPlazaRowDTO` (token variant: input/output prices; image variant: nullable token fields + `image_prices: { tier_1k, tier_2k, tier_4k }`)
  - `PlanPlazaCardDTO`
  - `CurrencyMetaDTO`
  - `PlazaModelsResponseDTO { rows, currency_meta }`
  - `PlazaPlansResponseDTO { cards, currency_meta }`

## 3. Backend: Tests

- [x] 3.1 Unit tests in `pricing_plaza_service_test.go` covering:
  - [x] 3.1.1 Token-only group with two accounts: model union dedup, multiplier and discount calculation.
  - [x] 3.1.2 Group with mixed token + image-generation models: token rows precede image rows; image row carries three tier prices.
  - [x] 3.1.3 Image-tier resolution path: group with `image_price_1k` set, group without (LiteLLM × tier factor × image multiplier); group with `image_rate_independent=true`.
  - [x] 3.1.4 LiteLLM miss + fallback hit → row rendered with fallback price as `base`.
  - [x] 3.1.5 LiteLLM miss + fallback miss → row dropped silently.
  - [x] 3.1.6 Group filtered out when `is_exclusive=true`, `status≠active`, or `subscription_type=subscription`.
  - [x] 3.1.7 Filters: `group_id` exact match; `platform` exact match; `q` case-insensitive substring on model name.
  - [x] 3.1.8 Group with zero associated active accounts → no rows for that group.
  - [x] 3.1.9 `CurrencyMeta` reflects current `BalanceRechargeMultiplier`.
- [x] 3.2 Unit tests for `ListPlanCards`:
  - [x] 3.2.1 `for_sale=false` plans excluded.
  - [x] 3.2.2 Plan with deleted/inactive group → excluded.
  - [x] 3.2.3 Card `models[]` equals union of `keys(account.Credentials["model_mapping"])` under the linked group.
  - [x] 3.2.4 Sort order honored.
- [x] 3.3 Handler / route tests in `plaza_handler_test.go`:
  - [x] 3.3.1 Both endpoints respond `200` with no auth header.
  - [x] 3.3.2 Both endpoints respond `200` with an invalid `Authorization` header (anonymous chain ignores it).
  - [x] 3.3.3 `q` param longer than 64 chars is rejected with `400`.
- [x] 3.4 Cache layer tests: same response served from cache within TTL; mutation invalidates.

## 4. Frontend: Plaza Page

- [x] 4.1 Add route `/plaza` (root tab landing) and child routes `/plaza/models` and `/plaza/plans`.
- [x] 4.2 Build `useCurrencyToggle` hook reading `currency_meta.balance_recharge_multiplier` from response payload, exposing `display: "CNY" | "USD"` (default `"CNY"`), `format(amount, native)` helper that converts on-the-fly.
- [x] 4.3 Build `<ModelPlazaTable />`:
  - [x] 4.3.1 Columns: Group / Model / Type / Base Price / Site Price / Multiplier / Discount.
  - [x] 4.3.2 Token row: base + site shown as `input/output per Mtok`.
  - [x] 4.3.3 Image row: base + site shown as three sub-columns (1K / 2K / 4K) per the user's UI choice (form A); cells aligned across the three tiers.
  - [x] 4.3.4 Top-of-page filters: group dropdown (populated from response), platform dropdown, search box (debounced 250 ms).
  - [x] 4.3.5 Currency toggle in the header: `CNY ⇄ USD`; updates all numeric cells via the format helper without refetching.
- [x] 4.4 Build `<PlanPlazaCards />`:
  - [x] 4.4.1 Card per plan with name, price (struck-through original price if `original_price > price`), discount badge, validity, features bullet list, included-models chip list (truncate at 10 + "+N more").
  - [x] 4.4.2 Same currency toggle in the page header (shared with model view).
- [x] 4.5 Empty states: "No models yet" / "No plans for sale" when the respective list is empty.
- [x] 4.6 Page is reachable without login; navbar entry visible to anonymous visitors.

## 5. Frontend: Tests

- [x] 5.1 Snapshot test for `<ModelPlazaTable />` rendering token + image rows with mock data.
- [x] 5.2 Test `useCurrencyToggle`: switching toggle updates rendered cells without re-issuing a network call.
- [x] 5.3 Test filter interactions: group filter narrows rows; search box does case-insensitive match; clearing filters restores full set.
- [x] 5.4 Test `<PlanPlazaCards />` correctly shows discount when `original_price > price` and hides struck-through line otherwise.

## 6. Documentation

- [x] 6.1 Update `README.md` (English + CN + JA) with a one-paragraph "Pricing Plaza" feature blurb and the two endpoint paths.
- [x] 6.2 Add an example response payload for each endpoint to the API reference docs.
- [x] 6.3 Add a note in the admin docs that "exclusive groups will not appear on the model plaza" so operators understand visibility implications.

## 7. Verification

- [x] 7.1 `go test ./internal/service/... ./internal/handler/... ./internal/server/...` green.
- [ ] 7.2 Manual smoke test: anonymous `curl` to both endpoints on a dev deployment returns expected JSON shape including `currency_meta`.
- [ ] 7.3 Visual QA: load `/plaza/models` as logged-out user, toggle currency, exercise each filter, scroll image rows.
- [ ] 7.4 Visual QA: load `/plaza/plans`, confirm struck-through `original_price` and discount badge.
