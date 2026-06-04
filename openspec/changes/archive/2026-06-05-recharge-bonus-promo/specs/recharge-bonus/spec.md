## ADDED Requirements

### Requirement: Admin Configures Recharge Bonus Campaign

The admin SHALL be able to configure a global recharge bonus campaign through the existing `系统设置 → 支付设置` view. The configuration SHALL be persisted as a single JSON setting `RECHARGE_PROMO` and SHALL include:

- `enabled` (boolean) — whether the campaign is currently active.
- `valid_from` (RFC3339 timestamp, optional) — earliest moment the campaign applies.
- `valid_until` (RFC3339 timestamp, optional) — last moment the campaign applies (inclusive of the timestamp, exclusive of the next millisecond).
- `tiers` (array) — at least one entry when `enabled = true`. Each entry has:
  - `min_amount` (positive number, in payment currency).
  - `bonus_rate` (number in `[0, 1)`).

The system SHALL validate, both client-side and server-side, that:

- Tier `min_amount` values are strictly ascending.
- Each `bonus_rate` is in `[0, 1)`.
- If both `valid_from` and `valid_until` are present, `valid_from < valid_until`.
- When `enabled = true`, `tiers.length ≥ 1`.

When the configuration is saved, the system SHALL recompute a stable `version` token derived from a hash of the canonical promo JSON; the `version` SHALL change if and only if the functional content changed.

#### Scenario: Admin saves a valid 3-tier campaign

- **WHEN** an admin submits `enabled = true` with tiers `[(100, 0.03), (500, 0.05), (1000, 0.08)]` and `valid_until = 2026-07-04T00:00:00Z`
- **THEN** the system persists the JSON, returns success, and `GetPaymentConfig` returns the campaign on subsequent reads.

#### Scenario: Admin submits non-ascending tiers

- **WHEN** an admin submits tiers `[(500, 0.05), (100, 0.03)]`
- **THEN** the request fails with HTTP 400, `reason = INVALID_RECHARGE_PROMO`, and `message` mentions "tiers must be ascending by min_amount".

#### Scenario: Admin submits a bonus rate of 1.0 or higher

- **WHEN** an admin submits a tier with `bonus_rate = 1.0`
- **THEN** the request fails with `INVALID_RECHARGE_PROMO` referencing the rate range `[0, 1)`.

#### Scenario: Admin disables the campaign

- **WHEN** an admin sets `enabled = false`
- **THEN** the system saves the configuration and subsequent recharge fulfillments compute `bonus_amount = 0` regardless of `tiers`.

### Requirement: Bonus Resolution and Rounding

The system SHALL provide a deterministic function `ResolveRechargeBonus(payAmount, promo, now) -> (rate, bonus)` that computes the bonus for a given `pay_amount` and active promo configuration at the given moment.

Resolution rules:

- If `promo == nil`, `promo.enabled = false`, or `now` is outside `[valid_from, valid_until]` (when those are present), the function SHALL return `(0, 0)`.
- Otherwise, the system SHALL pick the **highest** tier whose `min_amount ≤ payAmount`. If no tier matches, it SHALL return `(0, 0)`.
- Otherwise, `rate = matchedTier.bonus_rate` and `bonus = ceil2(payAmount × rate)`, where `ceil2(x) = math.Ceil(x × 100) / 100`.

The bonus computation SHALL use a fixed-point decimal library (e.g. `shopspring/decimal`) to avoid floating-point drift; the final result SHALL be a `float64` rounded **up** to two decimal places.

#### Scenario: Pay amount below all tiers

- **GIVEN** tiers `[(100, 0.03), (500, 0.05)]` and `pay_amount = 80`
- **WHEN** the system resolves the bonus
- **THEN** it returns `(0, 0)`.

#### Scenario: Pay amount matches the lowest tier

- **GIVEN** tiers `[(100, 0.03), (500, 0.05)]` and `pay_amount = 100`
- **WHEN** the system resolves the bonus
- **THEN** it returns `(0.03, 3.00)`.

#### Scenario: Pay amount matches a higher tier

- **GIVEN** tiers `[(100, 0.03), (500, 0.05), (1000, 0.08)]` and `pay_amount = 999`
- **WHEN** the system resolves the bonus
- **THEN** it returns `(0.05, 49.95)`.

#### Scenario: Pay amount produces a fractional bonus that must round up

- **GIVEN** a tier with `bonus_rate = 0.0333` and `pay_amount = 100`
- **WHEN** the system resolves the bonus
- **THEN** it returns `(0.0333, 3.33)` — `ceil(3.33000) = 3.33`.
- **AND WHEN** `pay_amount = 101`
- **THEN** it returns `(0.0333, 3.37)` — `ceil(3.3633) = 3.37`.

#### Scenario: Resolution outside the valid time window

- **GIVEN** `valid_until = 2026-06-04T00:00:00Z` and `now = 2026-06-04T00:00:01Z`
- **WHEN** the system resolves the bonus
- **THEN** it returns `(0, 0)`.

### Requirement: Order Carries Bonus Audit Fields

Every `payment_orders` row SHALL persist `bonus_amount` (decimal(20,2), default 0) and `bonus_rate` (decimal(10,4), default 0). On fulfillment of an `order_type = balance` order, the system SHALL:

1. Re-evaluate the active recharge promo at the moment of fulfillment using `pay_amount`.
2. Set `bonus_rate` and `bonus_amount` on the order to the result.
3. Credit the user's balance with `credited_balance + bonus_amount` in a single database transaction.

Subscription orders (`order_type = subscription`) SHALL keep `bonus_amount = 0` and `bonus_rate = 0`.

#### Scenario: Balance order with active campaign records bonus and credits user

- **GIVEN** an active campaign with tiers `[(100, 0.05)]`, `BALANCE_RECHARGE_MULTIPLIER = 1`, and a paid order with `pay_amount = 200`, `user.balance = 0`
- **WHEN** fulfillment runs
- **THEN** the order has `bonus_rate = 0.05`, `bonus_amount = 10.00`, `amount = 200.00` (credited balance), and `user.balance = 210.00`.

#### Scenario: Subscription order ignores promo

- **GIVEN** an active campaign and an order with `order_type = subscription`, `pay_amount = 199`
- **WHEN** fulfillment runs
- **THEN** the order has `bonus_rate = 0`, `bonus_amount = 0`, and the user's balance is unaffected by bonus.

#### Scenario: Campaign expires between order creation and fulfillment

- **GIVEN** a paid order created while a campaign was active, where the campaign's `valid_until` is reached before fulfillment runs
- **WHEN** fulfillment runs
- **THEN** the order has `bonus_amount = 0` and the user receives only the standard `credited_balance`.

### Requirement: Refund Reverses Bonus Atomically

The system MUST claw back both the credited balance and the bonus when a recharge order is refunded, and MUST refuse refunds that would push the user's balance negative.

When a refund is issued for an `order_type = balance` order whose `bonus_amount > 0`:

- The system SHALL first verify `user.balance ≥ amount + bonus_amount`.
- If the balance is insufficient, the refund SHALL fail with `reason = BALANCE_INSUFFICIENT_FOR_REFUND` and **no** gateway refund SHALL be attempted.
- If the balance is sufficient, the system SHALL deduct `amount + bonus_amount` from the user's balance and issue the gateway refund of `pay_amount` in a single transaction.

For the first release, partial refunds SHALL be rejected for orders with `bonus_amount > 0` (return `reason = PARTIAL_REFUND_NOT_SUPPORTED_FOR_BONUS_ORDER`).

#### Scenario: Refund succeeds when balance covers credited + bonus

- **GIVEN** an order with `amount = 200`, `bonus_amount = 10`, `pay_amount = 200`, and `user.balance = 250`
- **WHEN** the refund is issued
- **THEN** `user.balance` becomes `40`, `pay_amount = 200` is refunded via the gateway, and the order moves to a refunded status.

#### Scenario: Refund rejected when balance is insufficient

- **GIVEN** an order with `amount = 200`, `bonus_amount = 10`, and `user.balance = 150`
- **WHEN** the refund is requested
- **THEN** the request fails with `BALANCE_INSUFFICIENT_FOR_REFUND` and the gateway is **not** called.

#### Scenario: Partial refund rejected for bonus order

- **GIVEN** an order with `bonus_amount = 10`
- **WHEN** an admin requests a partial refund
- **THEN** the request fails with `PARTIAL_REFUND_NOT_SUPPORTED_FOR_BONUS_ORDER`.

### Requirement: Checkout API Surfaces Active Campaign

The `GET /api/payment/checkout-info` response SHALL include a `recharge_promo` object whenever the configured campaign would resolve to a non-zero bonus for at least one tier at the current server time. The shape SHALL be:

```
recharge_promo: {
  enabled:     boolean,
  valid_from:  RFC3339 string | null,
  valid_until: RFC3339 string | null,
  tiers:       [{min_amount: number, bonus_rate: number}, ...],
  version:     string                    // stable hash of the campaign content
}
```

When the campaign is disabled or outside its time window, the response SHALL omit `recharge_promo` (or set it to `null`); the frontend SHALL treat both forms as "no active campaign".

#### Scenario: Active campaign is exposed to the checkout

- **GIVEN** an enabled campaign with tiers `[(100, 0.03), (500, 0.05)]` and `valid_until` in the future
- **WHEN** the user fetches `/api/payment/checkout-info`
- **THEN** the response includes `recharge_promo` with the same tiers and a non-empty `version`.

#### Scenario: Disabled campaign is hidden from checkout

- **GIVEN** a campaign with `enabled = false`
- **WHEN** the user fetches `/api/payment/checkout-info`
- **THEN** the response either omits `recharge_promo` or sets it to `null`.

#### Scenario: Version stability across no-op saves

- **GIVEN** an admin saves the campaign without changing any field
- **WHEN** the new checkout response is compared with the previous one
- **THEN** `recharge_promo.version` is unchanged.

### Requirement: User-Facing Recharge Breakdown Shows Bonus

The `PaymentView` recharge tab SHALL render the active campaign so the user understands the perceived value before paying. Concretely:

- When `recharge_promo` is present, a banner above `AmountInput` SHALL display the campaign tiers in human-readable form, plus the localized `valid_until` if provided.
- Each `AmountInput` preset whose value matches a non-zero tier SHALL display a small bonus badge (e.g. `+5%`) adjacent to the amount.
- The breakdown card under the amount section SHALL include a new line `赠送 / Bonus` showing `+$xx.xx` (always two decimals, ceil-rounded), positioned below the existing `到账余额` line, and a final `合计入账 / Total credited` summary line.
- All amounts SHALL be formatted using the existing `formatPaymentAmount` helper for the selected currency.

#### Scenario: User selects a preset that matches a tier

- **GIVEN** a campaign with tiers `[(100, 0.03), (500, 0.05)]`
- **WHEN** the user selects the `500` preset
- **THEN** the breakdown shows `到账余额 $500.00`, `赠送 +$25.00`, and `合计入账 $525.00`.

#### Scenario: User enters a custom amount below all tiers

- **GIVEN** a campaign with tiers `[(100, 0.03)]`
- **WHEN** the user enters `90`
- **THEN** the breakdown does NOT show a bonus row, and the campaign banner remains visible.

#### Scenario: Campaign disabled hides breakdown bonus

- **GIVEN** the API returns no `recharge_promo`
- **WHEN** the user opens the recharge tab
- **THEN** no banner, no bonus badges, and no bonus row are rendered.

### Requirement: Red-Dot Promotion and Dismissal

The recharge tab and bonus-eligible amount presets SHALL display a red dot when (a) `recharge_promo` is present and (b) the current user has not yet dismissed the campaign on this browser.

Dismissal state SHALL be stored in `localStorage` under the key `recharge-promo-seen:<userId>:<promoVersion>`. The system SHALL NOT include any time component in the key — once dismissed for a `version`, the dot stays dismissed for that version.

The system SHALL dismiss the dot (write the key) under either of these user actions:

1. The user clicks the **recharge tab** while the tab dot is visible.
2. The user clicks **any preset** that currently shows a tier red dot.

After dismissal, both the tab dot and all per-tier dots SHALL disappear immediately and remain hidden until either the user logs in as a different user or the admin saves a campaign with a different `version`.

#### Scenario: First-time visitor sees red dots

- **GIVEN** the user has no `recharge-promo-seen:<userId>:<v1>` key in localStorage and the active campaign has version `v1`
- **WHEN** the user opens `PaymentView`
- **THEN** the recharge tab shows a red dot, and any preset matching a tier shows a red dot.

#### Scenario: Clicking the tab dismisses both tab and tier dots

- **GIVEN** the conditions above
- **WHEN** the user clicks the recharge tab
- **THEN** both the tab dot and all preset red dots disappear, and `localStorage` contains `recharge-promo-seen:<userId>:<v1> = "1"`.

#### Scenario: Clicking a preset dot dismisses everything

- **GIVEN** the user has not yet dismissed and clicks a preset that has a red dot
- **WHEN** the click is registered
- **THEN** the same dismissal write happens, the tab dot also disappears, and the preset is selected as the current amount.

#### Scenario: Admin updates the campaign, dots reappear

- **GIVEN** the user previously dismissed campaign version `v1` and the admin saves a substantively different campaign that mints `v2`
- **WHEN** the user revisits `PaymentView`
- **THEN** the tab dot and tier dots reappear because no `recharge-promo-seen:<userId>:<v2>` key exists yet.

#### Scenario: Different user on the same browser

- **GIVEN** user A dismissed `v1`
- **WHEN** user B logs in on the same browser and opens `PaymentView`
- **THEN** user B sees the red dots — user A's localStorage key does not match user B's `userId`.
