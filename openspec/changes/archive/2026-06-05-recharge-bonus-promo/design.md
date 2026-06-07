## Context

Today balance recharge is a single linear formula:

```
credited_balance = round2( pay_amount × BALANCE_RECHARGE_MULTIPLIER )
```

`BALANCE_RECHARGE_MULTIPLIER` is a permanent CNY→USD conversion factor (default 1.0) configured in `payment_config_service.go`. It is **not** a marketing knob — changing it permanently re-prices every future recharge and is also read by the pricing plaza for currency display.

Operators want a separate, time-bounded **promotional bonus** that can be turned on/off without touching the multiplier. The new feature must:

1. Add `bonus_amount` to the user's balance on top of the existing `credited_balance`.
2. Support multiple amount tiers with different bonus rates (e.g. `≥¥100 → 3%`, `≥¥500 → 5%`).
3. Be visible at multiple points in the UI: a banner, per-tier badges, a breakdown row, and an attention-grabbing red dot.
4. Survive refunds correctly: refunding an order must claw back **both** credited and bonus portions.

Stakeholders: end users (clearer perceived value), admins (configurable campaigns), finance (clean per-order audit of bonus vs. paid).

## Goals / Non-Goals

**Goals:**
- One JSON setting (`RECHARGE_PROMO`) drives everything (admin UI, checkout response, fulfillment, validation).
- Bonus is **fully independent** from `BALANCE_RECHARGE_MULTIPLIER`. The two compose by addition, never multiplication.
- Per-order auditability: every recharge order records which `bonus_rate` and `bonus_amount` were applied.
- Refund safety: a refund that would push the user's balance negative is rejected — never silently truncated.
- Red-dot UX is purely client-side state in `localStorage`, with **no daily refresh** — once dismissed for the current campaign, it stays gone until admin saves a new campaign (which mints a new `version`).

**Non-Goals:**
- Multiple concurrent campaigns. Only one campaign can be active at a time.
- Per-user / per-segment / coupon-code campaigns. The campaign is global.
- Dynamically adjusting `bonus_amount` after the order is created (e.g. price-match). Bonus is computed once at fulfillment time using the order's `pay_amount`.
- Changing the meaning of `BALANCE_RECHARGE_MULTIPLIER` or `RECHARGE_FEE_RATE`.
- Server-side persistence of "user has seen the red dot". The red dot is purely local and per-browser.

## Decisions

### D1. Bonus is additive, not multiplicative

```
credited_balance = round2 ( pay_amount × multiplier )         // unchanged
bonus_amount     = ceil2  ( pay_amount × bonus_rate )         // NEW
total_credited   = credited_balance + bonus_amount            // applied to user.balance
```

**Why**: keeps marketing-bonus accounting clean and reversible. Refunds and reports can clearly separate "what we credited as currency conversion" from "what we gave away as a promotion".

**Alternative considered**: collapse both into one effective multiplier per tier. Rejected because (a) it conflates currency conversion with promotion, (b) the existing multiplier is also used by the pricing plaza for CNY⇄USD display, which would no longer be a stable number, and (c) per-order audit becomes ambiguous.

### D2. Threshold tiers, ascending, "highest match wins"

```
tiers: [{min_amount: A1, bonus_rate: R1},
        {min_amount: A2, bonus_rate: R2}, ...]   with A1 < A2 < ...
```

For a given `pay_amount`, pick the tier with the largest `min_amount ≤ pay_amount`. If no tier matches (or `pay_amount < A1`), bonus is zero.

**Why**: simplest mental model both for admins ("what threshold to hit and what %") and for users (the higher I pay, the better the rate). Validation is trivial (strict ascending check).

**Alternative considered**: closed intervals `[min, max)`. Rejected — admins would have to keep `max[i] = min[i+1]` in sync; risk of gaps or overlaps; UI more complex.

### D3. Bonus rounds **up** to two decimals (`ceil2`)

`ceil2(x) = math.Ceil(x × 100) / 100`. Distinct from the existing `creditedBalance` which uses banker's rounding via `decimal.Round(2)`.

**Why**: matches the user-facing requirement ("赠送部分精确到两位小数向上取整"), and biases the rounding error toward the user (a fairness optic on a marketing perk).

**Implementation note**: `ceil2` lives in `payment_amounts.go` next to `calculateCreditedBalance`, using `shopspring/decimal` to avoid float drift.

### D4. New columns on `payment_orders`, NOT on a join table

```
bonus_amount  decimal(20,2)  NOT NULL  DEFAULT 0
bonus_rate    decimal(10,4)  NOT NULL  DEFAULT 0
```

**Why**: every recharge order has at most one promo applied; a side table would only add joins for zero-value rows. Defaults of 0 make the migration trivially backwards compatible. `bonus_rate` is denormalized so reports can group "all 5% recharges in May" without re-joining the historical config.

**Alternative considered**: store the entire promo snapshot in `provider_snapshot` JSON. Rejected — that field is reserved for payment provider state; mixing in bonus state hurts queryability.

### D5. Bonus is computed at **fulfillment**, using the live promo at that moment

The promo is re-evaluated when the order moves to `Recharging`. We do **not** snapshot the promo at order creation.

**Why**: typical recharge flow is < 5 minutes, so config drift is negligible. Re-evaluating at fulfillment means a campaign that was disabled mid-flight does not retroactively pay out, and a tier that was promoted upward does not under-pay. This matches industry conventions ("promo rate at the moment of payment") and minimizes "I started checkout 3h ago and the promo expired" disputes — if the promo is still valid at the moment money lands, the user gets it.

**Trade-off**: a user who started checkout while a promo was active could lose the bonus if the campaign expires before they pay. We accept this; the UI shows the `valid_until` clearly so the user understands the urgency.

### D6. Refund deducts `credited_balance + bonus_amount`, with insufficient-balance guard

```
if user.balance < (order.amount_credited + order.bonus_amount):
    return error BALANCE_INSUFFICIENT_FOR_REFUND
else:
    user.balance -= (order.amount_credited + order.bonus_amount)
    gateway.refund(order.pay_amount)
```

**Why**: matches the requirement ("退款时bonus也要退， 余额不够不让退"). The guard prevents creating negative balances when a user has already spent the bonus. We return a typed error so the UI can render a localized message.

**Note**: `order.amount_credited` is the existing `amount` field (post-multiplier credited balance). On a partial refund (which the system already supports for some flows), we prorate proportionally — out of scope for the first cut; we will only allow **full refunds** for orders carrying a non-zero `bonus_amount` to keep the math obvious.

### D7. Red-dot state: localStorage, keyed by `userId + promoVersion`, no time component

```
key   = `recharge-promo-seen:${userId}:${promo.version}`
value = "1"  (or absent)
```

`promo.version` is a server-issued string updated whenever admin saves the campaign (e.g. ISO timestamp of the last update). When admin enables a new campaign, the version changes → old localStorage keys become irrelevant → red dots reappear.

**Why no daily refresh**: the user explicitly removed that requirement. The version-based invalidation is strictly better — it ties dismissal to the campaign identity, not to wall-clock time, so users don't get re-pestered every day for a campaign they've already seen.

**Why `userId` in the key**: avoids two users on the same browser inheriting each other's dismissal.

### D8. Tab-level dismissal cascades to tier-level dismissal

Both the tab red dot and the per-tier red dots share the **same** localStorage key. Clicking the tab OR clicking any preset that has a red dot writes the key.

**Why**: matches the requirement ("点过一次后消失" 整体一次方案). One key keeps the dismissal logic trivial and the user's attention is acknowledged regardless of which surface they interacted with.

### D9. Admin UI: dynamic tier table inside an existing settings form

The "充值活动" section lives in the existing `PaymentSettings*.vue` admin view (next to fee rate / multiplier), so we don't introduce a new admin route. Tier rows are managed client-side and validated on submit.

**Validation rules** (both client- and server-side):
- `enabled` is a bool.
- If `enabled = true`, `tiers` MUST have ≥ 1 entry.
- `tiers[].min_amount > 0`, ascending strictly.
- `tiers[].bonus_rate ∈ [0, 1)`. (We disallow ≥ 100% as a sanity guard.)
- If both `valid_from` and `valid_until` are set, `valid_from < valid_until`.

## Risks / Trade-offs

- **[Bonus inflation in tests]** New non-zero columns on `payment_orders` could break stale fixtures that hardcode order JSON. → Mitigation: defaults of 0 keep backwards compatibility; new tests cover non-zero cases.
- **[Rounding mismatch user vs server]** Frontend previews bonus as `ceil2(amount × tier.bonus_rate)`; server confirms at fulfillment. If a user changes the amount mid-flight or admin changes the tier, the displayed preview may differ from the final credit. → Mitigation: re-fetch checkout-info on tab-focus and re-evaluate on `amount` change; show the server-computed bonus on the order detail page.
- **[Refund guard surprises admin]** Admin clicks "refund" and gets a "balance insufficient" error. → Mitigation: surface the user's current balance and the required deduction in the error UI; document the policy in admin help text.
- **[Promo version churn]** If admin saves the same config repeatedly, the version still changes, re-showing red dots. → Mitigation: compute `version` from a stable hash of the canonical promo JSON, not from `updated_at`. If nothing functionally changed, version stays the same.
- **[Concurrent campaigns]** Out of scope, but we leave the JSON shape forward-compatible (a future `campaigns: []` array could replace the current `tiers/valid_from/valid_until` if needed).
- **[Time zone confusion]** `valid_from` / `valid_until` are stored as RFC3339 with timezone; admin UI uses the user's local zone for display. → Backend always compares in UTC.

## Migration Plan

1. Ent migration: add `bonus_amount`, `bonus_rate` columns to `payment_orders`. Default 0, NOT NULL → safe forward; rollback drops the columns (no data loss, since older code doesn't read them).
2. Deploy backend with `RECHARGE_PROMO` setting reader + writer + fulfillment integration. With no setting saved, behavior is identical to today.
3. Deploy frontend with admin form + checkout banner. Without `recharge_promo` in the response, banner and red dots stay hidden.
4. Admin enables their first campaign. Verify: red dot shows, breakdown shows bonus row, refund correctness in staging.

Rollback strategy: disable the campaign via admin (`enabled: false`) → frontend hides everything immediately. Hard rollback: revert backend deploy; the new columns remain harmless (default 0).

## Open Questions

- Do we want an admin-side "preview" that shows the current bonus for a sample amount? *(Nice-to-have, can be a follow-up.)*
- Do we surface the historical `bonus_amount` in the admin order list? *(Yes — added as a column in tasks.)*
