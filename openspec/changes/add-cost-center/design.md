## Context

The application already has authoritative records for user token usage (`usage_logs`), payment orders (`payment_orders`), balance adjustments/redeem codes, subscriptions, and account-level billing multipliers. These records answer operational billing questions but do not form an auditable profit report. The change is cross-cutting: it adds administrator-managed expense records, captures new billing-source snapshots, calculates two deliberately different profit views, and exposes a time-filtered console.

Only activity created after this change is enabled is in scope. Historical rows are not backfilled or reclassified. All cost-center monetary values are stored as USD decimal snapshots; payment records retain their original currency and the conversion rate used at settlement.

## Goals / Non-Goals

**Goals:**

- Provide a single ledger query for settled cash income, realized consumption income, promotional/free consumption, manually entered operating expenses, and profit.
- Record one-off and recurring expenses with an immutable auditable event for every actual or confirmed occurrence.
- Preserve the source of every new usage and balance event sufficiently to distinguish paid balance, subscription, recharge bonus, administrator grant, and unknown/unallocated value.
- Allocate subscription-plan revenue over actual token-quota consumption within the validity window while keeping unused and expired entitlement visible but unrecognized by default.
- Make account expense entry available from create, bulk-edit, and account action workflows, with bulk amounts applied independently to each selected account.
- Make account expense entry available directly in the cost center with searchable account selection and explicit audit context.
- Keep current user charging, payment fulfillment, and operational upstream-cost calculations unchanged while excluding automatic upstream cost from the cost center.

**Non-Goals:**

- Reconstructing or migrating historical revenue, balances, expenses, or subscription usage.
- Replacing the existing balance ledger, payment order lifecycle, or usage billing source of truth.
- Full general-ledger accounting, tax filing, payroll accounting, or multi-currency accounting beyond settlement-time USD conversion.
- Automatically guessing the source of legacy or unclassified balances.

## Decisions

### 1. Add a cost-center event ledger rather than mutable totals

Introduce append-oriented records for `income`, `expense`, `promotional_consumption`, and `subscription_recognition`, with source type/id, account/user/plan references, event time, USD amount, original money metadata, status, notes, and operator/audit data. Reports aggregate events instead of trusting denormalized totals. Corrections are compensating reversal events, never destructive edits to settled events.

**Alternative rejected:** adding `total_expense` to `accounts`; it cannot represent recurring costs, time ranges, corrections, or global expenses.

### 2. Separate cash and realized-profit calculations

Cash income is settled payment-order money minus refunds, payment fees, and rebates. Realized income is paid-balance consumption plus the recognized portion of subscription consumption. Recharge and administrator gifts are not income; their consumed value is shown separately. A recharge promotion's persisted `bonus_amount` is also recorded once as a settled expense when its balance order completes, using the payment order as the idempotent audit source. Account costs are recorded by administrators as one-off or recurring account expenses because usage-derived upstream estimates are not sufficiently accurate. The API returns both cash profit and operating profit so one number cannot be mistaken for the other.

**Alternative rejected:** treating every token request as income; it double-counts prepaid recharge and overstates revenue for free/gift usage.

### 3. Capture immutable USD snapshots at event creation

Every new event stores `amount_usd` and, when relevant, `original_amount`, `original_currency`, and `fx_rate`. Payment settlement creates the cash-income snapshot using the settlement-time rate. Account and global expenses require USD input in the first version. Usage finalization snapshots income and funding-source classification only; it does not write automatic upstream-cost expenses.

**Alternative rejected:** converting at report time using current settings; historical reports would change when exchange rates or pricing settings change.

### 4. Use explicit recurring expense plans and confirmation states

An expense plan may be one-off or recurring (daily, weekly, monthly, quarterly, yearly, or custom interval), with start/end, amount, account/global target, category, and active state. A scheduler materializes due occurrences as `pending`; an administrator confirms the actual payment to mark the event `settled`, or cancels/adjusts it with an audit trail. Pending events are excluded from cash profit but may be included in a forecast view.

**Alternative rejected:** silently accruing recurring costs as settled; this would report cash that was not paid.

### 5. Determine subscription recognition from a frozen quota valuation

At paid subscription fulfillment, snapshot plan price, standard token quota valuation, validity interval, and the derived realization factor (`price / standard_quota_value`). Each usage row linked to that subscription produces at most one recognition event, valued as standard consumption multiplied by that factor, capped at the paid entitlement. Unused entitlement remains deferred. Expired unused entitlement is visible as expired entitlement and is not recognized by default.

**Alternative rejected:** recognizing the entire plan price at purchase; it is simple but hides deferred service obligations and makes period profit misleading.

### 6. Preserve billing-source classification from the existing usage path

Use the existing balance-source and subscription associations as inputs. New usage/balance events must persist a normalized source snapshot (`paid_balance`, `subscription`, `recharge_bonus`, `admin_grant`, `affiliate_grant`, `unknown`) and link to the originating order, subscription, or adjustment when available. If a source cannot be identified for a new event, classify it as `unknown` and surface it in reconciliation rather than infer it.

### 7. Scope access to administrators and audit writes

Cost-center reads, expense writes, plan confirmations, reversals, and exports require the existing administrator authorization model. Every mutation records operator identity, timestamp, reason/notes, and the source object. The UI exposes only authorized navigation and actions.

### 8. Resolve readable people and accounts at query time

Cost-center event listings join the event's `user_id`, `operator_id`, and `account_id` to return readable names in one query. Manual and reversal events prefer the operator as their source person; usage-derived events use the consuming user. Historical automatic `upstream` events stay in the append-only ledger but are excluded by the common report predicate.

### 9. Classify account-targeted expense writes on the server

The administrator expense endpoint derives `source_type=account` whenever `account_id` is present and records the authenticated administrator as `operator_id`. The cost-center form loads account choices in complete paginated batches and displays account name, platform, and ID so similarly named and child accounts remain distinguishable.

### 10. Allow account-optional cost-center expense entry

The cost-center append-cost form treats the account and note as optional. When an account is selected, the client submits its ID and the server classifies the expense as account-targeted; when no account is selected, the client omits `account_id` and the server retains the global `manual` classification. Empty notes are omitted from the request. The form includes an `audit_account_expense` category for separately identified audited account spending and labels its free-text field simply as “Note”.

## Risks / Trade-offs

- [Usage finalization is asynchronous] → Make event creation idempotent by usage/request identifier and reconcile missing events; never create duplicate recognition or cost events on retry.
- [Recurring plan materialization runs late or twice] → Use a unique key of plan plus occurrence period and a transactional state transition; expose pending/missed occurrences for review.
- [Subscription quota valuation differs from actual model pricing] → Freeze the plan valuation at purchase and display the factor; do not silently recompute historical recognition when pricing settings change.
- [Some existing usage sources are not classified] → Keep an explicit `unknown` bucket and a reconciliation count; do not backfill history in this change.
- [Manual account expenses can be entered twice] → Keep immutable event audit fields and use reversals for corrections; do not combine them with unreliable automatic upstream estimates.
- [Large usage tables make ad-hoc reports slow] → Query indexed event timestamps and source dimensions; add daily aggregation only after measuring report latency.

## Migration Plan

1. Deploy schema and code that can create new cost-center events while leaving historical rows untouched.
2. Enable settlement, usage-finalization, subscription-fulfillment, and expense workflows to write events behind the administrator feature permission.
3. Verify idempotency and reconciliation counters on a staging dataset containing new activity only.
4. Enable the administrator route and UI after the write paths are producing valid USD snapshots.
5. Roll back by disabling the route/write feature and retaining already-created ledger rows; no existing billing tables or historical data are modified.

## Open Questions

- Whether payment-provider fees and affiliate rebates are already available as settled USD snapshots in all providers, or require a provider-specific adapter before they can be included in cash profit.
- Whether the first report endpoint needs daily pre-aggregation, or direct event/usage queries meet the expected data volume.
- Whether expired subscription entitlement should ever be recognized by an explicit administrator action; the default remains unrecognized.
