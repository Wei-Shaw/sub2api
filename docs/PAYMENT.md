# Payment System Configuration Guide

Sub2API has a built-in payment system that enables user self-service top-up without deploying a separate payment service. Two gateways are supported: GPM Pay (Vietnamese bank transfer via VietQR) and [NOWPayments](https://nowpayments.io) (crypto).

---

## Table of Contents

- [Supported Payment Methods](#supported-payment-methods)
- [Quick Start](#quick-start)
- [System Settings](#system-settings)
- [Provider Configuration](#provider-configuration)
- [Provider Instance Management](#provider-instance-management)
- [Webhook Configuration](#webhook-configuration)
- [Payment Flow](#payment-flow)
- [Refunds](#refunds)
- [Migrating from SePay](#migrating-from-sepay)

---

## Supported Payment Methods

| Payment type | Provider key | Description |
|--------------|--------------|-------------|
| `gpmpay_bank_transfer` | `gpmpay` | VietQR bank transfer, settled in VND |
| `nowpayments_crypto` | `nowpayments` | NOWPayments hosted crypto invoice |

> Evaluate the security, reliability, and compliance of any payment provider on
> your own — this project does not endorse or guarantee any of them.

---

## Quick Start

1. Go to Admin Dashboard → **Settings** → **Payment Settings** tab
2. Enable **Payment**
3. Configure basic parameters (amount range, timeout, etc.)
4. Add at least one GPM Pay or NOWPayments provider instance in **Provider Management**
5. Register the webhook URL in the provider's dashboard (see [Webhook Configuration](#webhook-configuration))
6. Users can now top up from the frontend

---

## System Settings

Configure the following in Admin Dashboard **Settings → Payment Settings**:

### Basic Settings

| Setting | Description | Default |
|---------|-------------|---------|
| **Enable Payment** | Enable or disable the payment system | Off |
| **Product Name Prefix** | Prefix shown on the payment page | - |
| **Product Name Suffix** | Suffix (e.g., "Credits") | - |
| **Min / Max Amount** | Per-order amount range | - |
| **Daily Limit** | Per-user daily total | - |
| **Max Pending Orders** | Per-user concurrent unpaid orders | - |
| **Order Timeout** | Minutes before an unpaid order expires | 30 |
| **Load Balance Strategy** | `round-robin` or `least-amount` across instances | `round-robin` |

### Currency

GPM Pay always settles in **VND**, which is a zero-decimal currency: an amount of
`250000` means ₫250,000 and fractional amounts are rejected.

NOWPayments prices each invoice in the instance's configured `currency` (`USD` or
`VND`; the admin dialog defaults to `USD`). The customer still pays in crypto and
NOWPayments converts at checkout time.

The subscription rate setting (1 USD = X gateway currency) converts a plan's USD
price into the settlement currency. It is opt-in: leave it at `0` and plan prices
are charged as-is.

### Top-up in USD or VND

Account balances are denominated in USD, but GPM Pay settles in VND. A customer
may type the top-up amount in either currency; the picker sits next to the
quick-amount buttons and is hidden when the gateway already settles in USD.

The rate comes from the Vietcombank published board
(`portal.vietcombank.com.vn/Usercontrols/TVPortal.TyGia/pXML.aspx`), **Sell**
column — we are selling USD to the customer, so the sell rate is the one that
applies. It is fetched at most once an hour and the last successful fetch is
persisted, so a restart does not leave the system without a rate.

| Setting | Description | Default |
|---------|-------------|---------|
| **Exchange Rate Markup** | Percent added on top of the published sell rate | `0` |
| **Exchange Rate Max Age** | Hours a cached rate may be used for while the provider is unreachable | `24` |

Rounding is deliberately asymmetric, always away from us: the VND charged is
rounded **up** to a whole dong, and the USD credited is truncated **down** to the
cent. The customer never gets balance we were not paid for.

If the rate provider is unreachable and the cached rate is older than the max age,
top-up order creation fails with `EXCHANGE_RATE_STALE` rather than pricing an
order against a rate of unknown age. With no cached rate at all the code is
`EXCHANGE_RATE_UNAVAILABLE`. Paying in the gateway's own currency still needs a
rate, because the balance credited is in USD.

`GET /api/v1/payment/exchange-rate?payment_type=<type>` returns the rate the
backend will price with, so the top-up form previews the same number the order
is created at. The frontend preview is never authoritative — the backend
re-derives both amounts from the submitted `amount` and `amount_currency`.

---

## Provider Configuration

Instance config is encrypted at rest with `security.secret_encryption_key`, and
saving an instance fails until that key is set. Sensitive fields are never
returned by the admin API; when editing an instance, leaving a sensitive field
blank keeps the stored value.

### GPM Pay

GPM Pay watches a Vietnamese bank account and signs a webhook for every transfer
into it. It has no upstream order: Sub2API builds the VietQR itself, with an
order code in the transfer memo, and matches incoming transfers by that code.

| Field | Sensitive | Required | Description |
|-------|-----------|----------|-------------|
| `apiToken` | **Yes** | Yes | GPM Pay API token, used to look up transactions when a webhook was missed |
| `webhookSecret` | **Yes** | Yes | Signing secret of the GPM Pay HMAC webhook. You choose it; enter the same value in GPM Pay |
| `bankBin` | No | Yes | 6-digit NAPAS BIN of the receiving bank (e.g. `970422` for MB), used to build the VietQR |
| `accountNumber` | No | Yes | The bank account GPM Pay watches |
| `allowSimulated` | No | No | `true` credits transfers made with GPM Pay's simulator. Testing only — anyone with dashboard access can simulate a transfer. Default `false` |

`webhookSecret`, `bankBin` and `accountNumber` cannot be changed while the
instance still has in-progress orders: the pending QR codes point at that
account, and their webhooks are verified with that secret.

### NOWPayments

| Field | Sensitive | Required | Description |
|-------|-----------|----------|-------------|
| `apiKey` | **Yes** | Yes | NOWPayments API key |
| `ipnSecretKey` | **Yes** | Yes | IPN secret generated in the NOWPayments dashboard. NOWPayments has no order lookup to double-check a callback, so the signature is the only proof it is genuine |
| `env` | No | No | `production` (default) or `sandbox`. Sandbox API keys do not work against production and vice versa |
| `currency` | No | No | Fiat currency invoices are priced in: `USD` or `VND` |
| `payCurrency` | No | No | Lock every invoice to one coin (e.g. `usdttrc20`). Leave blank to let the customer pick at checkout |

`apiKey`, `ipnSecretKey`, `env` and `currency` cannot be changed while the
instance still has in-progress orders.

---

## Provider Instance Management

Add instances in Admin Dashboard **Settings → Payment Settings → Provider Management**.

- **Supported types** — the payment method this instance offers (each gateway has one)
- **Limits** — per-method daily limit and single-order min/max; a limits entry
  under the gateway key (`gpmpay` or `nowpayments`) applies to every method of the instance
- **Sort order** — display order on the checkout page

Several enabled instances may serve the same payment method; orders are spread
across them by the configured load-balance strategy.

---

## Webhook Configuration

The admin provider dialog shows the exact URL for your deployment.

### GPM Pay

```
https://your-domain.com/api/v1/payment/webhook/gpmpay
```

1. Pick a webhook secret and save it in the GPM Pay instance in Sub2API
   (`webhookSecret`), with the instance enabled.
2. In the GPM Pay dashboard (**Integrations → Webhooks**), register the URL above
   as an HMAC webhook with the same secret, and leave `fireOnSimulated` off.

GPM Pay pings the URL when the webhook is registered, which is why the secret has
to be saved in Sub2API first. The ping is acknowledged and ignored.

**How a webhook is trusted.** Each request carries
`X-GPMPay-Signature: t=<unix>,v1=<hex>`, where `v1` is HMAC-SHA256 with the
webhook secret over `"<t>." + rawBody`. A request is rejected if the signature
does not match or `t` is more than 300 seconds from the server clock. GPM Pay
posts every transfer on the account, so outgoing transfers and transfers whose
memo carries no order code are acknowledged and ignored. Simulated transfers are
ignored too unless the instance sets `allowSimulated=true`.

The order is recovered from the memo code, which is the order's `out_trade_no`
without the underscore, uppercased (banks routinely uppercase memos and strip
punctuation). A transfer that pays less than the order, or more than a
small rounding tolerance, is not credited and is recorded in the order's audit log as
`PAYMENT_AMOUNT_MISMATCH`.

### NOWPayments

```
https://your-domain.com/api/v1/payment/webhook/nowpayments
```

Add the URL as the IPN callback in the NOWPayments dashboard
(**Settings → Payments → IPN**) and paste the IPN secret it generates into the
instance's `ipnSecretKey`.

**How a callback is trusted.** The `x-nowpayments-sig` header must be the
HMAC-SHA512, keyed with the IPN secret, of the body re-serialized with sorted
keys. Callbacks without a valid signature are rejected. Only `finished` credits
the order; `failed` and `expired` fail it; intermediate states (`waiting`,
`confirming`, `sending`, `partially_paid`) are acknowledged and the order keeps
waiting. The credited amount is the invoice's `price_amount` in the configured
fiat currency, never the crypto amount actually sent.

---

## Payment Flow

```
User selects amount and payment method
       │
       ▼
  Create Order (PENDING)
  ├─ Validate amount range, pending order count, daily limit
  ├─ Load balance to select provider instance
  └─ GPM Pay:     build a VietQR locally (no upstream call) with the
                  order code in the transfer memo
     NOWPayments: create a hosted invoice and get its invoice URL
       │
       ▼
  GPM Pay:     user scans the QR on the checkout page and transfers
  NOWPayments: user is sent to the invoice URL and pays in crypto
       │
       ▼
  Signed webhook → order located and amount checked → Order PAID
       │
       ▼
  Auto top-up to user balance → Order COMPLETED
```

### Order Status Reference

| Status | Description |
|--------|-------------|
| `PENDING` | Waiting for user to complete payment |
| `PAID` | Payment confirmed, awaiting balance credit |
| `COMPLETED` | Balance credited successfully |
| `EXPIRED` | Timed out without payment |
| `CANCELLED` | Cancelled by user |
| `FAILED` | Balance credit failed, admin can retry |

Historical orders may still carry refund statuses (`REFUNDED`, `REFUND_PENDING`,
…) from before the refund feature was removed. They render normally but nothing
produces them any more.

### Timeout and Fallback

- Before marking an order as expired, the background job queries the upstream
  payment status first
- A background job also re-checks unexpired pending orders, so a missed callback
  is reconciled without waiting for expiry
- The background job runs every 60 seconds

For GPM Pay the upstream query searches the account's incoming transactions for
the order code (using `apiToken`). For NOWPayments the callback is the real
settlement signal: a new order only holds the invoice id, which the payment
lookup cannot resolve, so the query reports it as pending.

---

## Refunds

Sub2API has no refund flow: there are no refund endpoints, no admin refund
actions, and no user refund requests. Handle refunds outside Sub2API (a bank
transfer back to the customer, or through NOWPayments) and adjust the user's
balance manually from the admin dashboard.

---

## Migrating from SePay

SePay has been removed. Migration `248_retire_sepay_provider.sql` runs at startup
and:

- disables every `sepay` provider instance (instances are kept, because
  historical orders reference them)
- strips `sepay_*` entries from the `ENABLED_PAYMENT_TYPES` setting, leaving
  GPM Pay and NOWPayments entries untouched
- snapshots both beforehand into `payment_provider_instances_backup_248` and
  `settings_payment_backup_248`

Historical orders keep their `sepay_*` payment types and still display. Pending
SePay orders are left as they are: the gateway can no longer be queried, so they
simply expire unpaid. Before deploying, check for pending SePay orders, since a
customer who pays one after the upgrade will not be credited automatically.
