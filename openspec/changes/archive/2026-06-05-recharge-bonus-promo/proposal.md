## Why

Operators want to run **recharge bonus campaigns** ("满 X 元加赠 Y%") to drive larger top-ups. Today the only knob is `BALANCE_RECHARGE_MULTIPLIER`, which is a permanent CNY→USD conversion factor for *every* recharge — it cannot express tiered, time-bounded promotions, and it conceptually mixes "currency conversion" with "marketing bonus". We need a separate, configurable promotion mechanism that:

- Adds **bonus balance** on top of the normally credited balance, independent of the existing multiplier.
- Lets admins define **multiple amount tiers** with different bonus rates.
- Surfaces the active campaign clearly to users (rate hint, per-tier markers, bonus line in the breakdown).
- Uses **red dots** on the recharge tab and on bonus-eligible amount presets to attract attention, dismissed once per user (across browsers via localStorage).

## What Changes

### Backend
- Add a new payment setting `RECHARGE_PROMO` (JSON) to `payment_config` with shape:
  ```json
  { "enabled": true,
    "valid_from": "2026-06-04T00:00:00Z",  // optional
    "valid_until": "2026-07-04T00:00:00Z", // optional
    "tiers": [ {"min_amount": 100, "bonus_rate": 0.03},
               {"min_amount": 500, "bonus_rate": 0.05},
               {"min_amount": 1000,"bonus_rate": 0.08} ] }
  ```
  Stored as a single JSON value; threshold-based matching: the **highest tier whose `min_amount ≤ pay_amount`** wins. Validation: tiers strictly ascending by `min_amount`, `bonus_rate ∈ [0, 1)`, time window if both fields present implies `valid_from ≤ valid_until`.
- New `PaymentConfig.RechargePromo *RechargePromo` field exposed via `GetPaymentConfig` / admin `UpdateSettings`.
- New helper `service.ResolveRechargeBonus(payAmount float64, promo *RechargePromo, now time.Time) (rate float64, bonus float64)`:
  - Returns `(0, 0)` if disabled, outside time window, or no tier matched.
  - Otherwise picks the highest matching tier and returns `bonus = ceilToCents(payAmount × rate)` (always rounded **up** to two decimals, distinct from the existing banker's rounding used for `creditedBalance`).
- `payment_orders` schema: add two columns
  - `bonus_amount decimal(20,2) NOT NULL DEFAULT 0`
  - `bonus_rate   decimal(10,4) NOT NULL DEFAULT 0`
  Populated at **fulfillment time** (when the order moves to `Paid → Recharging → Completed`) using the promo config that was active when the order was created. Re-quoted at fulfillment to avoid stale snapshots.
- Order fulfillment for `order_type = balance` credits the user's balance with `creditedBalance + bonusAmount` in a single DB transaction; `bonusAmount` is auditable on the order.
- Refund flow:
  - Computed effective refund = `pay_amount` (refunded to gateway) and balance reduction = `creditedBalance + bonus_amount`.
  - **Before triggering the refund**, verify `user.balance ≥ creditedBalance + bonus_amount`. If insufficient, fail with new reason `BALANCE_INSUFFICIENT_FOR_REFUND` (returned to both user and admin refund endpoints).
  - The refund deducts both credited and bonus portions atomically.
- `GetCheckoutInfo` response gains `recharge_promo`:
  ```json
  "recharge_promo": {
    "enabled": true,
    "valid_until": "...",
    "tiers": [ {"min_amount":100,"bonus_rate":0.03}, ... ],
    "version": "2026-06-03T14:01:22Z"      // server-side hash/timestamp to invalidate dismissed dots
  }
  ```

### Frontend
- **Admin · 系统设置 → 支付设置**: new "充值活动" section with toggle, time window pickers, dynamic tier table (add / remove rows, sort by `min_amount`, validate ascending).
- **`PaymentView` 充值 tab**:
  - Renders a campaign banner above `AmountInput` when `recharge_promo.enabled` (e.g. "满 ¥100 加赠 3%・满 ¥500 加赠 5%・满 ¥1000 加赠 8%").
  - `AmountInput` presets that match a bonus tier render a small red badge ("+3%" / "+8%") next to the amount and a top-right red dot when not dismissed.
  - Breakdown card adds a new row `赠送 / Bonus` (formatted as `+$xx.xx`) below `到账余额`, plus a `合计入账 / Total credited` summary row.
- **Tab bar red dot**: when `recharge_promo.enabled` and the user has not yet dismissed the current campaign, show a red dot on the `充值` tab.
- **Red-dot dismissal**:
  - localStorage key: `recharge-promo-seen:<userId>:<promoVersion>` → boolean.
  - Dismissed when the user (a) clicks the recharge tab, or (b) clicks any preset that has a red dot.
  - **No daily refresh**. Once dismissed for the current `promoVersion`, both tab dot and tier dots disappear until admin saves a new promo (which mints a new `version`).
- New composable `useRechargePromoDot(userId, promo)` exposes `{ shouldShow, dismiss }`.
- i18n: new keys under `payment.promo.*` (banner, bonusLine, totalCredited, tierBadge, etc.) for `zh-CN` and `en-US`.

### Tests
- Backend: tier resolution (boundary, empty, time-window), bonus rounding-up, refund insufficient-balance rejection, fulfillment credits both portions, JSON validation in admin update.
- Frontend: composable dismissal logic, breakdown rendering with/without promo, red-dot persistence across reloads, admin form validation.

## Capabilities

### New Capabilities
- `recharge-bonus`: Configurable, time-bounded recharge bonus campaigns with tiered bonus rates, per-order `bonus_amount` accounting, refund-safety guard, and frontend red-dot promotion (admin config + checkout breakdown + dismissal state).

### Modified Capabilities
*(None — `BALANCE_RECHARGE_MULTIPLIER` and the rest of the payment pipeline keep their current contracts; bonus is additive.)*

## Impact

- **Schema migration**: `payment_orders` gets two new non-nullable columns with defaults — additive, safe to roll forward; no data backfill needed.
- **API surface**:
  - `GET /api/payment/checkout-info` response gains `recharge_promo` (additive, optional).
  - `POST /api/admin/settings` accepts new `RECHARGE_PROMO` setting; validation errors use existing setting-update error contract.
  - Refund endpoints (`POST /api/payment/orders/:id/refund-request`, `POST /api/admin/payment-orders/:id/refund`) gain a new failure reason `BALANCE_INSUFFICIENT_FOR_REFUND`.
- **Affected code**: `payment_config_service.go`, `payment_amounts.go`, `payment_fulfillment.go`, `payment_refund*.go`, `payment_handler.go`, `setting_handler.go`, ent schema `payment_order.go`; frontend `PaymentView.vue`, `AmountInput.vue`, admin `PaymentSettings*.vue`, `useRechargePromoDot.ts`, i18n files.
- **No external dependencies** added.
- **No breaking changes**: existing recharges with `recharge_promo.enabled = false` (or unset) behave exactly as today.
