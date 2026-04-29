# Multi-Currency Checkout With USD Ledger Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Cho phép khách nạp/mua bằng tiền địa phương như VND/KRW/CNY ở checkout, nhưng toàn bộ balance/subscription/fulfillment nội bộ vẫn hạch toán bằng USD.

**Architecture:** Multi-currency chỉ nằm ở payment boundary. Backend tạo order với `ledger_amount/currency` và `payment_amount/currency`, lưu immutable FX snapshot tại thời điểm tạo order, provider thu local currency, webhook xác nhận đúng local amount/currency, fulfillment cộng USD. FX rates are admin-managed in DB/settings; phase sau thêm persisted exchange-rate table + quote endpoint, không dùng external FX API/auto-sync cho checkout.

**Tech Stack:** Go backend + Ent + Gin + provider abstraction, Vue 3/TypeScript frontend, Vitest, Go unit tests, existing payment config/service/provider stack.

---

## 0. Current Context

Repo: `/root/projects/sub2api`

Current branch when plan was written: `main` tracking `origin/main`, clean working tree.

Important current anchors:

- `backend/internal/service/payment_config_service.go`
  - Already has `PAYMENT_LEDGER_CURRENCY`, `PAYMENT_ALLOWED_CURRENCIES`, `PAYMENT_MANUAL_FX_RATES_JSON`.
  - Current defaults are `defaultLedgerCurrency = "USD"`, `defaultPaymentCurrencyCSV = "CNY,USD"`, `defaultManualFXRatesJSON = {"USD":1,"CNY":1}`.
- `backend/internal/service/payment_exchange.go`
  - Already has `resolveFXSnapshot(...)` and amount tolerance helpers.
  - Currently resolves manual `RatePaymentToLedger` but does not own full conversion/rounding policy.
- `backend/internal/service/payment_order.go`
  - `CreateOrder` currently sets `ledgerAmount` and `paymentAmount` from the same input value in most paths.
  - `createOrderInTx` already stores `payment_currency`, `payment_amount`, `ledger_currency`, `ledger_amount`, `fx_rate_payment_to_ledger`, `fx_source`, `fx_timestamp`.
- `backend/ent/schema/payment_order.go`
  - Existing order schema already has the required payment-vs-ledger fields; no migration required just to persist order FX snapshot.
- `backend/internal/service/payment_service.go`
  - Backend `CreateOrderRequest` already has `PaymentCurrency`.
  - `CreateOrderResponse` already exposes payment/ledger/FX fields.
- `backend/internal/handler/payment_handler.go`
  - `/payment/checkout-info` currently does not expose ledger currency, allowed currencies, FX metadata, or minor units.
- `frontend/src/types/payment.ts`
  - Frontend `CreateOrderRequest`, `PaymentOrder`, and `CheckoutInfoResponse` do not yet model payment/ledger/FX fields completely.
- `frontend/src/views/user/PaymentView.vue`
  - Checkout still hardcodes `¥` in several places and treats amount as a single currency.
- Provider realities:
  - `backend/internal/payment/provider/sepay.go`: VND only, whole number amount required.
  - `backend/internal/payment/provider/stripe.go`: currently hardcodes `cny` and uses `YuanToFen`; needs dynamic currency/minor-unit handling before using Stripe for KRW/USD/etc.

## 1. Product Rules / Invariants

1. **Ledger is USD-only** unless product explicitly changes later.
   - User balance remains USD.
   - Plan price remains USD.
   - Usage, fulfillment, dashboards, refunds are interpreted in USD ledger terms unless explicitly labeled as provider payment amount.
2. **Local currency is checkout/provider-only.**
   - VND/KRW/CNY only affect amount user sees/pays and provider request/webhook verification.
3. **Snapshot FX once.**
   - Never recalculate FX at webhook/fulfillment time.
   - Order stores immutable `payment_amount`, `payment_currency`, `ledger_amount`, `ledger_currency`, `fx_rate_payment_to_ledger`, source, timestamp.
4. **Backend owns all money math.**
   - Frontend can preview, but backend recalculates/validates before creating the order.
5. **Provider compatibility must be explicit.**
   - SePay => `VND` only.
   - WeChat/Alipay current paths => `CNY` unless provider implementation proves otherwise.
   - Stripe => only currencies supported by configured Stripe payment method + account after dynamic currency implementation.
   - KRW should not be enabled in production UI until there is a provider that actually supports KRW.
6. **Rounding is currency-aware.**
   - VND/KRW/JPY: 0 decimal places.
   - USD/CNY: 2 decimal places.
   - Converting USD to local amount to collect: round up.
   - Converting local top-up to USD credit: round down.

## 2. Acceptance Criteria

Backend:

- Creating a VND top-up order with manual FX rate correctly stores and returns:
  - `payment_currency = VND`
  - `payment_amount = local VND amount`
  - `ledger_currency = USD`
  - `ledger_amount = USD credit`
  - `fx_rate_payment_to_ledger` and source/timestamp.
- Creating a USD/default legacy order still works without requiring frontend changes.
- SePay receives VND whole-number amount, not USD.
- Webhook/verification compares provider paid amount/currency against the stored payment amount/currency, and fulfillment credits stored USD amount.
- Provider/currency mismatch returns a clear 400-style error before creating a provider payment.
- Existing payment order lifecycle tests still pass.

Frontend:

- Payment page lets user select/display local payment currency.
- User can see both:
  - amount they pay in local currency,
  - amount credited/used internally in USD.
- No payment UI hardcodes `¥` except intentional CNY-specific copy if any.
- API types include payment/ledger/FX fields.
- Existing payment tests updated and passing.

Ops/Admin:

- Admin-managed FX is the only source of truth for checkout rates.
- No external FX API/auto-sync config is used for payment checkout.
- Stale/missing FX policy is visible and tested so missing admin rates fail loudly instead of being masked.

## 3. Execution Prep

Before implementation, do this in a fresh work branch:

```bash
cd /root/projects/sub2api
git fetch origin
git checkout main
git pull --ff-only origin main
git checkout -b feat/multi-currency-checkout-usd-ledger
```

Do not commit secrets. Payment FX rates are edited by Admin and stored in DB/settings; do not add external FX API keys/config for checkout.

## 4. Phase 1 — Backend Currency Math + Manual FX Order Creation

### Task 1: Add currency metadata and rounding helpers

**Objective:** Centralize currency minor units and conversion rounding so backend does not scatter money math.

**Files:**

- Modify: `backend/internal/service/payment_exchange.go`
- Create or modify tests: `backend/internal/service/payment_exchange_test.go`

**Implementation shape:**

Add helpers near existing FX functions:

- `currencyMinorUnits(currency string) int`
- `roundPaymentAmountForCollection(amount float64, currency string) float64`
- `roundLedgerAmountForCredit(amount float64, currency string) float64`
- `convertPaymentToLedger(paymentAmount float64, snapshot fxSnapshot) (float64, error)`
- `convertLedgerToPayment(ledgerAmount float64, snapshot fxSnapshot) (float64, error)`

Use `RatePaymentToLedger` as: `ledger = payment * rate`.

Examples to test:

- `VND -> USD`: `255000 VND * 0.000039215686 = 10.00 USD`
- `USD -> VND`: `10 USD / 0.000039215686 = 255000 VND`
- `KRW`/`VND` round to whole units.
- `USD`/`CNY` round to 2 decimals.
- Invalid/zero/NaN rates return errors.

**Verification:**

```bash
cd /root/projects/sub2api/backend
go test -tags=unit ./internal/service -run 'TestCurrency|TestFX|TestPaymentExchange' -count=1
```

### Task 2: Make CreateOrder compute ledger/payment amounts separately

**Objective:** Stop treating `req.Amount` as both provider amount and ledger amount.

**Files:**

- Modify: `backend/internal/service/payment_service.go`
- Modify: `backend/internal/service/payment_order.go`
- Test: `backend/internal/service/payment_order_lifecycle_test.go` or new `payment_order_currency_test.go`

**Contract decision for Phase 1:**

Add an explicit amount mode to backend request:

```go
type CreateOrderRequest struct {
    UserID          int64
    Amount          float64
    AmountMode      string // "ledger" default, or "payment"
    PaymentCurrency string
    PaymentType     string
    // existing fields...
}
```

Constants:

```go
const (
    PaymentAmountModeLedger  = "ledger"
    PaymentAmountModePayment = "payment"
)
```

Behavior:

- Subscription order:
  - Ignore user-provided amount for pricing.
  - `ledgerAmount = plan.Price`.
  - Resolve FX snapshot from requested `payment_currency`.
  - `paymentAmount = convertLedgerToPayment(ledgerAmount, snapshot)`.
- Balance top-up with `amount_mode = payment`:
  - User entered local amount.
  - `paymentAmount = req.Amount`.
  - `ledgerAmount = convertPaymentToLedger(paymentAmount, snapshot)`.
  - Apply `BalanceRechargeMultiplier` to credited ledger amount in a clearly named variable.
- Balance top-up with default/legacy `amount_mode = ledger`:
  - Preserve existing clients: `ledgerAmount = req.Amount`.
  - `paymentAmount = convertLedgerToPayment(ledgerAmount, snapshot)`.

Important: fee handling must be reviewed carefully. Current code uses:

```go
payAmountStr := payment.CalculatePayAmount(paymentAmount, feeRate)
```

Keep fee in **payment currency** because it is what provider collects. Daily limits should continue using ledger amount.

**Tests:**

- VND top-up, `amount_mode=payment`, amount `255000`, rate `0.000039215686` => ledger `10.00 USD`, payment `255000 VND`.
- USD legacy top-up, no payment currency/mode => current behavior remains compatible.
- Subscription plan `$10`, VND selected => payment amount `255000 VND`, ledger `10 USD`.
- Missing FX rate for non-USD currency returns clear error.

**Verification:**

```bash
cd /root/projects/sub2api/backend
go test -tags=unit ./internal/service -run 'TestPaymentOrder.*Currency|TestCreateOrder.*Currency|TestPaymentOrderLifecycle' -count=1
```

### Task 3: Validate provider/currency compatibility before provider invocation

**Objective:** Fail fast when user selects a currency unsupported by selected provider.

**Files:**

- Modify: `backend/internal/service/payment_order.go`
- Possibly modify: `backend/internal/payment/provider/stripe.go`
- Test: `backend/internal/service/payment_order_currency_test.go`

**Implementation shape:**

Add service helper:

```go
func validateProviderCurrency(providerKey, paymentCurrency string) error
```

Rules for first implementation:

- `sepay` => `VND`
- `wxpay`, `wxpay_direct`, `alipay`, `alipay_direct` => `CNY`
- `stripe`, `card`, `link` => initially `CNY` until dynamic Stripe currency task lands; then allow configured set.
- `paddle` => handle separately if existing provider supports currency, otherwise reject non-supported currency.

Call after `selectCreateOrderInstance` and before `createOrderInTx` / provider invocation.

**Verification:**

```bash
cd /root/projects/sub2api/backend
go test -tags=unit ./internal/service -run 'Test.*ProviderCurrency|Test.*Sepay.*Currency' -count=1
```

### Task 4: Ensure provider request uses local payment amount and stored snapshot

**Objective:** Provider should receive local amount/currency, while ledger amount remains available for metadata/records.

**Files:**

- Modify: `backend/internal/service/payment_order.go`
- Test: `backend/internal/service/payment_order_currency_test.go`
- Existing provider tests: `backend/internal/payment/provider/sepay_test.go`

Check `invokeProvider(...)` builds `payment.CreatePaymentRequest` with:

- `Amount`: formatted pay amount in payment currency after fee.
- `PaymentCurrency`: order/request payment currency.
- `LedgerCurrency`: order ledger currency.
- `LedgerAmount`: formatted ledger amount.

Add/adjust tests with fake provider capturing create request.

**Verification:**

```bash
cd /root/projects/sub2api/backend
go test -tags=unit ./internal/service -run 'Test.*CreatePaymentRequest|Test.*InvokeProvider' -count=1
go test ./internal/payment/provider -run TestSepay -count=1
```

### Task 5: Extend handler request parsing and checkout-info response

**Objective:** Expose currency metadata to frontend and accept selected currency/mode.

**Files:**

- Modify: `backend/internal/handler/payment_handler.go`
- Modify tests or create: `backend/internal/handler/payment_handler_currency_test.go`

Request JSON additions for create order:

```json
{
  "amount": 255000,
  "amount_mode": "payment",
  "payment_currency": "VND",
  "payment_type": "sepay",
  "order_type": "balance"
}
```

Checkout info response additions:

```json
{
  "ledger_currency": "USD",
  "allowed_payment_currencies": ["USD", "CNY", "VND", "KRW"],
  "manual_fx_rates": { "USD": 1, "VND": 0.000039215686 },
  "currency_meta": {
    "USD": { "minor_units": 2, "symbol": "$" },
    "VND": { "minor_units": 0, "symbol": "₫" },
    "KRW": { "minor_units": 0, "symbol": "₩" },
    "CNY": { "minor_units": 2, "symbol": "¥" }
  }
}
```

Do not expose secret provider config.

**Verification:**

```bash
cd /root/projects/sub2api/backend
go test -tags=unit ./internal/handler -run 'TestPayment.*Checkout|TestPayment.*CreateOrder' -count=1
```

## 5. Phase 2 — Frontend Local Currency UX

### Task 6: Update frontend payment API types

**Objective:** Make TypeScript contract match backend response/request.

**Files:**

- Modify: `frontend/src/types/payment.ts`
- Modify tests: `frontend/src/api/__tests__/payment.spec.ts` if request forwarding tests are added.

Add fields:

- `PaymentConfig.ledger_currency`
- `PaymentConfig.allowed_payment_currencies`
- `PaymentConfig.manual_fx_rates`
- `CheckoutInfoResponse.ledger_currency`
- `CheckoutInfoResponse.allowed_payment_currencies`
- `CheckoutInfoResponse.manual_fx_rates`
- `CheckoutInfoResponse.currency_meta`
- `CreateOrderRequest.payment_currency`
- `CreateOrderRequest.amount_mode`
- `CreateOrderResult.payment_amount`
- `CreateOrderResult.payment_currency`
- `CreateOrderResult.ledger_amount`
- `CreateOrderResult.ledger_currency`
- `CreateOrderResult.fx_rate`
- `CreateOrderResult.fx_source`
- `CreateOrderResult.fx_timestamp`
- `PaymentOrder` equivalent fields.

**Verification:**

```bash
cd /root/projects/sub2api
pnpm --dir frontend run typecheck
```

### Task 7: Add a central money formatter and currency preview helper

**Objective:** Remove hardcoded symbols and keep rounding/display consistent.

**Files:**

- Create: `frontend/src/utils/money.ts`
- Test: `frontend/src/utils/__tests__/money.spec.ts`

Functions:

```ts
export function currencyMinorUnits(currency: string): number
export function formatMoney(amount: number, currency: string, locale?: string): string
export function convertPaymentToLedger(paymentAmount: number, ratePaymentToLedger: number): number
export function convertLedgerToPayment(ledgerAmount: number, ratePaymentToLedger: number, paymentCurrency: string): number
```

Use `Intl.NumberFormat` for display where possible. For VND, ensure `255000` displays as a whole amount, e.g. `255.000 ₫` or `₫255,000` depending locale.

**Verification:**

```bash
cd /root/projects/sub2api
pnpm --dir frontend exec vitest run src/utils/__tests__/money.spec.ts
```

### Task 8: Add currency selector and preview in PaymentView

**Objective:** Let users top up in local currency and see USD credit.

**Files:**

- Modify: `frontend/src/views/user/PaymentView.vue`
- Modify: `frontend/src/views/user/__tests__/PaymentView.spec.ts`
- Possibly modify i18n locale files under `frontend/src/locales` if these labels use translations.

Behavior:

- Default selected currency from checkout info:
  1. previously selected currency in local storage if still allowed,
  2. browser locale heuristic (`vi`=>`VND`, `ko`=>`KRW`, `zh`=>`CNY`),
  3. first allowed currency,
  4. `USD` only when it is explicitly allowed/configured.
- For balance top-up:
  - Amount input is in selected payment currency.
  - Create order sends `amount_mode: "payment"` and `payment_currency`.
  - Preview shows: `You pay 255,000 ₫` and `You receive 10.00 USD`.
- For subscription:
  - Plan base price remains USD.
  - Preview shows local equivalent using selected currency.
  - Create order sends `payment_currency`, but backend derives ledger from plan.
- Replace all `¥{{ ... }}` in checkout summary/buttons with `formatMoney(...)`.

**Verification:**

```bash
cd /root/projects/sub2api
pnpm --dir frontend exec vitest run src/views/user/__tests__/PaymentView.spec.ts
pnpm --dir frontend run typecheck
```

### Task 9: Update payment status/result panels to display both amounts where available

**Objective:** Order details should not confuse local provider amount with USD credit.

**Files:**

- Modify: `frontend/src/components/payment/PaymentStatusPanel.vue` if it displays amount.
- Modify: `frontend/src/views/user/PaymentResultView.vue`
- Modify tests:
  - `frontend/src/components/payment/__tests__/PaymentStatusPanel.spec.ts`
  - `frontend/src/views/user/__tests__/PaymentResultView.spec.ts`

Display rules:

- Main payment instruction: local `payment_amount/payment_currency`.
- Credit/subscription value: USD `ledger_amount/ledger_currency`.
- Existing orders without new fields fall back to legacy `amount/pay_amount` with current defaults.

**Verification:**

```bash
cd /root/projects/sub2api
pnpm --dir frontend exec vitest run \
  src/components/payment/__tests__/PaymentStatusPanel.spec.ts \
  src/views/user/__tests__/PaymentResultView.spec.ts
```

## 6. Phase 3 — Quote Endpoint for Safer Checkout Preview

Phase 1/2 can work by exposing manual rates in checkout info and recalculating on create order. Quote endpoint makes UX and security cleaner because the preview and create order share one server snapshot.

### Task 10: Add backend quote types and route

**Objective:** Server returns a short-lived quote for local/ledger conversion.

**Files:**

- Modify: `backend/internal/service/payment_service.go`
- Create: `backend/internal/service/payment_quote.go`
- Modify: `backend/internal/handler/payment_handler.go`
- Modify: `backend/internal/server/routes/payment.go`
- Test: `backend/internal/service/payment_quote_test.go`
- Test: `backend/internal/handler/payment_quote_handler_test.go`

API:

```http
POST /api/v1/payment/quote
```

Request:

```json
{
  "order_type": "balance",
  "amount": 255000,
  "amount_mode": "payment",
  "payment_currency": "VND",
  "payment_type": "sepay",
  "plan_id": 0
}
```

Response:

```json
{
  "quote_id": "signed-or-persisted-id",
  "ledger_currency": "USD",
  "ledger_amount": 10.0,
  "payment_currency": "VND",
  "payment_amount": 255000,
  "fx_rate_payment_to_ledger": 0.000039215686,
  "fx_source": "manual",
  "fx_timestamp": "...",
  "expires_at": "..."
}
```

Implementation option:

- Phase 3A uses a signed quote token because the repo already has HMAC-signed payment resume tokens. This avoids DB churn/migrations while still locking amount/currency/FX snapshot between preview and order creation.
- A signed quote is a short-lived immutable snapshot, **not** an idempotency key and **not** single-use. Reusing the same quote can create another order until expiration; provider/order idempotency remains a separate concern.
- If auditability or single-use semantics become required, add a later persisted `payment_quotes` table with `consumed_order_id`, `consumed_at`, and user/order uniqueness checks.

**Verification:**

```bash
cd /root/projects/sub2api/backend
go test -tags=unit ./internal/service -run TestPaymentQuote -count=1
go test -tags=unit ./internal/handler -run TestPaymentQuote -count=1
```

### Task 11: Allow order creation from quote_id

**Objective:** Avoid frontend resubmitting mutable FX math.

**Files:**

- Modify: `backend/internal/service/payment_service.go`
- Modify: `backend/internal/service/payment_order.go`
- Modify: `backend/internal/handler/payment_handler.go`
- Tests: quote/order integration tests.

Request:

```json
{
  "quote_id": "...",
  "payment_type": "sepay"
}
```

Rules:

- Quote must not be expired.
- Quote user/order_type/payment_type/currency must match request context.
- Order uses quote amounts and FX snapshot exactly.
- Reusing a quote after an order exists should either be rejected or idempotently return the existing pending order; choose one and test it.

**Verification:**

```bash
cd /root/projects/sub2api/backend
go test -tags=unit ./internal/service -run 'TestPaymentQuote|TestCreateOrderFromQuote' -count=1
```

### Task 12: Update frontend to call quote endpoint before create order

**Objective:** Show authoritative backend quote and reduce mismatch errors.

**Files:**

- Modify: `frontend/src/api/payment.ts`
- Modify: `frontend/src/types/payment.ts`
- Modify: `frontend/src/views/user/PaymentView.vue`
- Tests:
  - `frontend/src/api/__tests__/payment.spec.ts`
  - `frontend/src/views/user/__tests__/PaymentView.spec.ts`

Behavior:

- Debounce quote requests when amount/currency/provider changes.
- Disable submit when quote is missing, expired, or mismatched.
- Create order sends `quote_id` when available.
- If quote endpoint fails due stale/missing FX, show actionable error.

**Verification:**

```bash
cd /root/projects/sub2api
pnpm --dir frontend exec vitest run \
  src/api/__tests__/payment.spec.ts \
  src/views/user/__tests__/PaymentView.spec.ts
pnpm --dir frontend run typecheck
```

## 7. Phase 4 — Admin-Managed FX Rates

### Task 13: Add persisted exchange-rate model

**Objective:** Store multiple admin-managed payment-currency -> USD rates with source/freshness as an explicit DB model instead of relying on one settings JSON blob.

**Files:**

- Create: `backend/ent/schema/payment_exchange_rate.go`
- Create migration under `backend/migrations/NNN_add_payment_exchange_rates.sql`
- Modify generated Ent files via `make generate` or repo generation command.
- Test: migration regression if repo pattern requires it.

Suggested fields:

- `base_currency` string, e.g. `USD`
- `quote_currency` string, e.g. `VND`
- `quote_per_base` decimal/float, e.g. `25500`
- `rate_payment_to_ledger` decimal/float, e.g. `0.000039215686`
- `source` string, e.g. `manual_admin`
- `updated_at` timestamptz
- `valid_until` timestamptz nullable
- `is_stale` bool
- unique index `(base_currency, quote_currency, source)` or `(base_currency, quote_currency)` depending desired source policy.

**Verification:**

```bash
cd /root/projects/sub2api/backend
make generate
go test ./migrations -count=1
```

### Task 14: Implement admin exchange-rate repository/API abstraction

**Objective:** Let Admin create/update many currency exchange-rate rows without coupling checkout to external HTTP calls.

**Files:**

- Create: `backend/internal/service/payment_exchange_rate_service.go`
- Create tests: `backend/internal/service/payment_exchange_rate_service_test.go`
- Modify config loading in `backend/internal/service/payment_config_service.go` if new settings are needed.

Config settings to add:

- Admin-managed FX rates stored in DB settings only.
- No external FX API key/provider/auto-sync env config for checkout.
- Admin updates `PAYMENT_MANUAL_FX_RATES_JSON` through the settings UI/API; quote/order creation snapshots that DB value.
- `PAYMENT_FX_STALE_AFTER_MINUTES`
- `PAYMENT_FX_BLOCK_AFTER_MINUTES`
- `PAYMENT_FX_MAX_RATE_CHANGE_PCT`

Service abstraction:

```go
type PaymentExchangeRate struct {
    BaseCurrency string // USD ledger
    QuoteCurrency string // VND/KRW/CNY payment currency
    RatePaymentToLedger float64
    Source string // manual_admin
    UpdatedAt time.Time
}
```

No external provider implementation and no manual fallback path. If an enabled payment currency has no valid admin rate, checkout/quote fails loudly.

**Verification:**

```bash
cd /root/projects/sub2api/backend
go test -tags=unit ./internal/service -run 'TestPaymentExchangeRate|TestPaymentFX' -count=1
```

### Task 15: Admin-managed FX rate status and future exchange-rate table

**Objective:** Let Admin update payment-currency -> USD ledger rates in DB settings, expose stale/missing status, and avoid external API auto-sync.

**Files:**

- Create/modify: `backend/internal/service/payment_fx_status.go`
- Add admin API/UI for exchange-rate rows once the table exists.
- Test: `backend/internal/service/payment_fx_status_test.go`

Rules:

- Admin controls allowed payment currencies and their payment-to-ledger rates.
- Skip ledger currency or store identity rate.
- Validate:
  - rate > 0
  - no NaN/Inf
  - rate does not change more than configured max % unless Admin explicitly confirms in a future UI flow.
- Checkout/order creation:
  - use only the admin-saved rate for the selected payment currency,
  - block non-USD if the rate is missing/stale beyond policy,
  - do not fallback to any provider/API/default rate because that hides configuration errors.

**Verification:**

```bash
cd /root/projects/sub2api/backend
go test -tags=unit ./internal/service -run 'TestPaymentFXStatus|TestResolveFXSnapshot' -count=1
```

### Task 16: Expose FX freshness in checkout/admin config

**Objective:** Admin and user UI can know whether rates are fresh/stale.

**Files:**

- Modify: `backend/internal/handler/payment_handler.go`
- Modify: `frontend/src/types/payment.ts`
- Modify: `frontend/src/views/user/PaymentView.vue`
- Modify admin payment settings if present:
  - `frontend/src/api/admin/payment.ts`
  - relevant admin settings view under `frontend/src/views/admin/`

Checkout info additions:

```json
{
  "fx_rates": {
    "VND": {
      "rate_payment_to_ledger": 0.000039215686,
      "source": "manual",
      "fetched_at": "...",
      "stale": false
    }
  }
}
```

UI:

- Show quote expiration / locked rate.
- If stale warning is returned, show warning and disable submit if backend marks blocked.

**Verification:**

```bash
cd /root/projects/sub2api
make test-frontend-critical
```

## 8. Phase 5 — Stripe/Paddle Provider Currency Cleanup

This phase can be delayed until after SePay VND and current CNY flows work.

### Task 17: Make Stripe currency dynamic and minor-unit aware

**Objective:** Remove `stripeCurrency = "cny"` and `YuanToFen` hardcode.

**Files:**

- Modify: `backend/internal/payment/provider/stripe.go`
- Modify/add tests: `backend/internal/payment/provider/stripe_test.go`

Implementation shape:

- Read `req.PaymentCurrency`, lower-case for Stripe.
- Convert amount using generic `amountToMinorUnits(amount, currency)`.
- Validate payment method types support requested currency if possible.
- Metadata includes order ID and maybe ledger/payment snapshot for reconciliation.

**Verification:**

```bash
cd /root/projects/sub2api/backend
go test ./internal/payment/provider -run TestStripe -count=1
```

### Task 18: Decide Paddle currency contract before enabling local currencies

**Objective:** Fit Paddle into same provider abstraction without leaking Paddle-specific vendor currency semantics into ledger.

**Files:**

- Inspect/modify Paddle provider if present under `backend/internal/payment/provider/`.
- Modify provider config/admin UI if Paddle needs supported-currency config.
- Tests under `backend/internal/payment/provider/*paddle*_test.go` or create one.

Rules:

- If Paddle handles hosted checkout with its own currency conversion, store both:
  - Paddle charged currency/amount from webhook,
  - sub2api ledger USD amount from quote/order snapshot.
- Webhook must verify Paddle transaction/order ID and paid amount/currency when available.
- Do not treat Paddle settlement currency as user ledger currency.

## 9. Test / Verification Matrix

Targeted backend during implementation:

```bash
cd /root/projects/sub2api/backend
go test -tags=unit ./internal/service -run 'TestCurrency|TestFX|TestPaymentOrder.*Currency|TestCreateOrder.*Currency|TestPaymentQuote' -count=1
go test -tags=unit ./internal/handler -run 'TestPayment.*Checkout|TestPaymentQuote|TestPayment.*CreateOrder' -count=1
go test ./internal/payment/provider -run 'TestSepay|TestStripe' -count=1
```

Full backend gate:

```bash
cd /root/projects/sub2api/backend
make test
```

Frontend targeted:

```bash
cd /root/projects/sub2api
pnpm --dir frontend exec vitest run \
  src/utils/__tests__/money.spec.ts \
  src/api/__tests__/payment.spec.ts \
  src/views/user/__tests__/PaymentView.spec.ts \
  src/views/user/__tests__/PaymentResultView.spec.ts \
  src/components/payment/__tests__/PaymentStatusPanel.spec.ts
pnpm --dir frontend run typecheck
pnpm --dir frontend run lint:check
```

Full repo gate:

```bash
cd /root/projects/sub2api
make test
make build
python3 tools/secret_scan.py
```

## 10. Rollout Plan

1. **Staging / local with manual FX only**
   - Enable `PAYMENT_LEDGER_CURRENCY=USD`.
   - Set `PAYMENT_ALLOWED_CURRENCIES=USD,CNY,VND` initially. Do not add KRW until provider exists.
   - Set `PAYMENT_MANUAL_FX_RATES_JSON` with test values only in environment/admin settings.
2. **Smoke test providers**
   - USD legacy/default order.
   - CNY existing provider flow.
   - VND SePay QR flow with whole-number amount.
3. **Webhook smoke**
   - Confirm paid VND order credits USD ledger amount from stored order, not live FX.
4. **External FX auto-sync removed**
   - Checkout uses Admin-managed DB/settings rates only; no external API or scheduler path is kept.
   - Enable auto FX later in staging with API key in environment only.
5. **Production**
   - Start with VND + existing CNY only.
   - Add KRW after provider decision/testing.

## 11. Risks / Tradeoffs

- **Float money math already exists in repo.** This plan keeps minimal diff, but long-term ideal is decimal/integer minor units. Avoid broad rewrite in this feature unless tests show precision issues.
- **Current order columns are `decimal(20,2)` for payment_amount.** This is okay for VND/KRW whole units and USD/CNY cents, but if future currencies need more precision, schema may need expansion.
- **Stripe dynamic currency can break payment method compatibility.** Keep Stripe currency expansion separate from SePay VND rollout.
- **Quote endpoint persistence vs signed token needs repo-specific decision.** Signed token has less DB churn; persisted quote is easier to audit/idempotency-check.
- **Missing/stale admin FX must not silently over-credit.** Stale policy should block non-USD orders when rates are missing or too old; no provider/default fallback should mask the issue.

## 12. Open Questions Before Coding Phase 4/5

1. If we need richer audit later, move from JSON settings to a `currency_exchange_rates` table with currency pair, rate, source, effective_at, created_by, and audit metadata.
2. Do we want `KRW` visible before choosing a Korea provider, or keep hidden until provider integration exists?
3. For balance top-up, should user enter local amount and receive computed USD, or choose USD packages displayed in local currency? Recommended for VN: local presets like `50k/100k/200k/500k VND`.
4. Should quote be persisted in DB or signed as token? Recommended: signed token for Phase 3 if existing signing helpers are available; DB quote if audit/idempotency is prioritized.

## 13. Recommended Implementation Order

1. Phase 1 backend manual FX order math.
2. Phase 2 frontend currency selector + formatter.
3. SePay VND staging smoke test.
4. Phase 3 quote endpoint.
5. Phase 4 admin-managed FX status/freshness visibility.
6. Phase 5 Stripe/Paddle currency cleanup.

This order gives useful product value quickly while keeping the riskiest external/provider changes isolated.
