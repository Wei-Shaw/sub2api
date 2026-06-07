## 1. Backend · Schema & Config

- [x] 1.1 Add `bonus_amount decimal(20,2) NOT NULL DEFAULT 0` and `bonus_rate decimal(10,4) NOT NULL DEFAULT 0` fields to `backend/ent/schema/payment_order.go` and run `go generate ./ent` to refresh the ent client.
- [x] 1.3 Add a new `RechargePromo` Go struct (`backend/internal/service/payment_config_recharge_promo.go`) with fields `Enabled bool`, `ValidFrom *time.Time`, `ValidUntil *time.Time`, `Tiers []RechargePromoTier`, `Version string`, plus `RechargePromoTier{MinAmount float64, BonusRate float64}`.
- [x] 1.4 Add setting key `SettingRechargePromo = "RECHARGE_PROMO"` in `backend/internal/service/payment_config_service.go`; serialize/deserialize as JSON and surface as `PaymentConfig.RechargePromo *RechargePromo`.
- [x] 1.5 Implement `validateRechargePromo(*RechargePromo) error` covering: ≥ 1 tier when enabled, strictly ascending `MinAmount`, `BonusRate ∈ [0, 1)`, `MinAmount > 0`, `ValidFrom < ValidUntil` when both present. Return `infraerrors.BadRequest("INVALID_RECHARGE_PROMO", ...)` with a precise message per failing rule.
- [x] 1.6 Compute `RechargePromo.Version` as a stable hex hash (sha1 or sha256 truncated) of the canonical JSON of the validated promo (sorted keys, normalized timestamps); the version SHALL stay identical across no-op saves.
- [x] 1.7 Wire the new field into `PaymentConfigService.UpdatePaymentConfig` and `GetPaymentConfig`; update `UpdatePaymentConfigRequest` to accept `RechargePromo *RechargePromo`.

## 2. Backend · Bonus Resolution & Fulfillment

- [x] 2.1 Add `ceil2(value float64) float64` and `ResolveRechargeBonus(payAmount float64, promo *RechargePromo, now time.Time) (rate float64, bonus float64)` in `backend/internal/service/payment_amounts.go`, using `shopspring/decimal` for the rate × amount multiplication and `math.Ceil(x*100)/100` for the final round-up.
- [x] 2.2 In the recharge fulfillment path (`backend/internal/service/payment_fulfillment.go`, the `order_type = balance` branch), call `ResolveRechargeBonus` with the **current** time, write `bonus_rate` and `bonus_amount` onto the order via the ent updater, and increase `user.balance` by `credited_balance + bonus_amount` in the same DB transaction.
- [x] 2.3 Ensure subscription orders never set `bonus_*` fields (default 0 on insert).
- [x] 2.4 Add unit tests in `payment_amounts_test.go`:
  - `pay_amount` below all tiers → `(0, 0)`.
  - Lowest / highest tier matching.
  - Round-up boundary (`bonus_rate = 0.0333`, `pay_amount = 100` and `101`).
  - Disabled promo, time-window-out promo, nil promo all return `(0, 0)`.
- [x] 2.5 Add an integration-style test in `payment_fulfillment_test.go` that runs the balance fulfillment with an active promo and asserts `user.Balance`, `order.BonusAmount`, and `order.BonusRate` after fulfillment.

## 3. Backend · Refund Guard

- [x] 3.1 In the balance refund path (`backend/internal/service/payment_refund*.go`), before calling `gwRefund`, compute `requiredDeduction := order.amount + order.bonus_amount` and reject with `infraerrors.BadRequest("BALANCE_INSUFFICIENT_FOR_REFUND", "balance not enough to claw back credited + bonus")` when `user.balance < requiredDeduction`. The check SHALL run inside the same transaction as the balance update to avoid TOCTOU races.
- [x] 3.2 On a successful refund, deduct `amount + bonus_amount` from `user.balance` (today the code only deducts `amount`; the test for D6 must drive this code change).
- [x] 3.3 Reject partial refunds when `order.bonus_amount > 0` with reason `PARTIAL_REFUND_NOT_SUPPORTED_FOR_BONUS_ORDER` in both the user-initiated and admin-initiated flows.
- [x] 3.4 Add `payment_refund_bonus_test.go` covering: successful refund with sufficient balance, rejection on insufficient balance (gateway not called — verify via mock), rejection of partial refund.

## 4. Backend · Checkout API

- [x] 4.1 Extend `backend/internal/dto` (or wherever `CheckoutInfoResponse` is defined) with an optional `RechargePromo` field carrying the same shape returned by the API spec (Requirement: Checkout API Surfaces Active Campaign).
- [x] 4.2 In `payment_handler.go::GetCheckoutInfo`, when `payment_config.RechargePromo != nil && enabled` AND we are within the time window, populate `recharge_promo`. When disabled or out of window, leave `recharge_promo` unset (omit empty / null).
- [x] 4.3 Add a handler test asserting the response includes/excludes `recharge_promo` correctly across enabled / disabled / time-windowed cases, including a stable `version` across no-op saves.

## 5. Backend · Admin Settings

- [x] 5.1 Extend `setting_handler.UpdateSettings` so the admin form payload accepts the new JSON setting; on validation failure, return `INVALID_RECHARGE_PROMO` with the precise reason.
- [x] 5.2 Add admin-side handler tests for: valid 3-tier save, invalid (non-ascending tiers, rate ≥ 1, empty tiers when enabled, invalid date order).

## 6. Frontend · Types, API, Composable

- [x] 6.1 Update `frontend/src/types/payment.ts` (or wherever `CheckoutInfoResponse` is defined) to include the optional `recharge_promo` block matching the backend shape.
- [x] 6.2 Add a composable `frontend/src/composables/useRechargePromoDot.ts` that:
  - Accepts `userId: ComputedRef<number | null>` and `promo: ComputedRef<RechargePromo | null>`.
  - Computes `localStorageKey = recharge-promo-seen:${userId}:${promo.version}` (returns null when either is missing).
  - Exposes `shouldShow: ComputedRef<boolean>` (true when promo enabled, version present, key not set in localStorage, and userId present).
  - Exposes `dismiss(): void` which writes `"1"` under the key and updates a reactive trigger so `shouldShow` flips to false.
  - Listens to the `storage` event to stay in sync across tabs.

## 7. Frontend · PaymentView Recharge Tab

- [x] 7.1 In `PaymentView.vue`, parse `checkout.value.recharge_promo`; expose a `promo` computed and the `useRechargePromoDot` composable's outputs.
- [x] 7.2 Render a campaign banner above `AmountInput` when `promo` is present, using the new i18n keys; show `valid_until` localized via existing date helpers.
- [x] 7.3 Add a `bonusForAmount(amount: number): number` helper in the script section that mirrors the backend resolution logic (ascending tiers, highest match wins, `Math.ceil(amount * rate * 100) / 100`).
- [x] 7.4 Add two new rows to the breakdown card under `creditedBalance`:
  - `赠送 / Bonus` showing `+${bonusForAmount(validAmount)}` when the bonus is non-zero.
  - `合计入账 / Total credited` showing `creditedAmount + bonus` when the bonus is non-zero.
- [x] 7.5 Wire the tab click on the recharge tab to call `dismiss()` when the tab dot is currently visible.

## 8. Frontend · AmountInput Tier Markers

- [x] 8.1 Update `frontend/src/components/payment/AmountInput.vue` to accept an optional `bonusTiers: Array<{min_amount: number, bonus_rate: number}>` prop and an optional `showRedDots: boolean` prop.
- [x] 8.2 For each preset, compute the matching tier (highest `min_amount ≤ preset`) and render:
  - A `+x%` badge near the preset label.
  - A red dot in the top-right corner when `showRedDots && hasMatchingTier`.
- [x] 8.3 Emit a new event `bonusPresetClicked` (or extend the existing select event with metadata) so the parent can call `dismiss()` when a red-dotted preset is clicked.
- [x] 8.4 Update `PaymentView.vue` to pass the new props and react to the new event by calling `dismiss()`.

## 9. Frontend · Admin PaymentSettings Form

- [x] 9.1 Locate the admin payment settings view (file path determined during implementation, search for `BALANCE_RECHARGE_MULTIPLIER` form bindings) and add a new "充值活动 / Recharge Bonus" section.
- [x] 9.2 Render: an enable toggle, two date-time pickers (`valid_from`, `valid_until`, both clearable), and a dynamic table of tiers with columns `min_amount`, `bonus_rate (% input)`, and a remove button. Add an "Add tier" button.
- [x] 9.3 Implement client-side validation matching the backend rules (ascending `min_amount`, `bonus_rate ∈ [0, 100)`, `valid_from < valid_until`, ≥ 1 tier when enabled). Surface inline errors per row.
- [x] 9.4 Submit the resolved JSON with the rest of the payment settings via the existing `UpdateSettings` flow; show backend `INVALID_RECHARGE_PROMO` errors via the existing toast pipeline.

## 10. Frontend · i18n

- [x] 10.1 Add the following keys to `frontend/src/i18n/locales/zh.ts` and `en.ts` (under a new `payment.promo.*` namespace):
  - `banner` (with `{validUntil}` placeholder), `tiersJoiner`, `tier` (with `{minAmount}` and `{rate}`), `bonusLine`, `totalCredited`, `tierBadge` (with `{rate}`), `redDotAria`.
  - Admin-side: `payment.settings.promo.title`, `enable`, `validFrom`, `validUntil`, `tiers`, `tier.minAmount`, `tier.bonusRate`, `addTier`, `removeTier`, error messages mirroring backend reasons.

## 11. Frontend · Tests

- [x] 11.1 Add `useRechargePromoDot.spec.ts`:
  - First-time visitor → `shouldShow = true`.
  - After `dismiss()` → `shouldShow = false` and key is set.
  - Promo `version` change → `shouldShow = true` again.
  - Different `userId` → `shouldShow = true` (other user's key ignored).
- [x] 11.2 Add a `PaymentView.bonus.spec.ts` (or extend an existing test) asserting:
  - Banner renders only when `recharge_promo` is present.
  - Breakdown bonus row renders only when `bonus > 0`.
  - Clicking a tier-eligible preset updates the breakdown total and dismisses the dot.
- [x] 11.3 Add an admin form test asserting client-side validation (ascending tiers, rate range).
