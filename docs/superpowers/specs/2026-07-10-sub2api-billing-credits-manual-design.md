# Sub2API Billing And Credits Manual Design

## Purpose

Create a self-service operations manual for configuring Sub2API v0.1.149 so
that:

- Sub2API continues to account for user balances and model usage in USD.
- Users pay through CNY payment channels.
- CNY recharge amounts convert to USD balance using a fixed operator-managed
  exchange rate.
- Payment product names use the suffix `Credits`.

The manual is documentation only. It must not change production settings,
connect to the production admin dashboard, or prescribe database edits.

## Audience

The primary reader is the AoxToken operator configuring the production
Sub2API instance through its admin dashboard. The reader should not need to
inspect Sub2API source code to follow the procedure.

## Document Location

The final manual will be written to:

```text
docs/sub2api-billing-credits-configuration.md
```

## Configuration Model

The manual will keep payment currency and credited balance conceptually
separate:

```text
CNY paid by user
  → balance recharge multiplier
  → USD-denominated Sub2API balance
  → displayed by AoxToken as Credits where AoxToken owns the UI
```

The worked example will use a fixed rate of:

```text
1 USD = 7.20 CNY
```

The corresponding balance recharge multiplier is:

```text
1 CNY = 1 / 7.20 USD = 0.13888889 USD
```

The subscription conversion setting, where applicable, is:

```text
Subscription USD-to-CNY rate = 7.20
```

## Manual Structure

The manual will contain:

1. Scope, supported version, goals, and non-goals.
2. A quick configuration summary.
3. Pre-change safeguards, including recording current values.
4. Exact Sub2API admin navigation and field values.
5. Exchange-rate formulas and examples for CNY 10, 50, 100, and 500.
6. An explanation of the difference between the balance recharge multiplier
   and subscription USD-to-CNY rate.
7. Verification procedures for checkout display, payment currency, credited
   balance, order product name, and rounding.
8. A rollback procedure restoring the recorded values.
9. Common problems and answers covering reversed rates, symbol-only changes,
   rounding, refunds, Stripe or other multi-currency providers, and later rate
   changes.

## Safety Rules

- Do not replace `$` globally with `¥`.
- Do not rename database fields or convert historical balances.
- Do not modify live settings while following documentation-development work.
- Test with the smallest payment amount allowed by the configured provider.
- Treat the exchange rate as a fixed commercial rate until the operator
  deliberately updates it; do not imply automatic foreign-exchange updates.
- Explain that changing the rate affects future orders and must not be assumed
  to revalue existing balances or historical orders.

## Verification Criteria

The manual is complete when a reader can:

- Calculate both exchange-rate fields without reversing them.
- Locate and fill the relevant Sub2API v0.1.149 settings.
- Understand that internal balances remain USD-denominated.
- Confirm that a CNY payment produces the expected USD/Credits balance.
- Confirm that the payment product name contains `Credits`.
- Restore the previous settings without touching the database.

The document must contain no credentials, production secrets, live admin
URLs containing tokens, unfinished markers, or instructions that require
source-code modification.
