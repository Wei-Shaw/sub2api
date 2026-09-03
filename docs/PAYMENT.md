# Payment System Configuration Guide

Sub2API has a built-in payment system that enables user self-service top-up without deploying a separate payment service. The gateway is [SePay](https://sepay.vn) (Vietnam).

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

---

## Supported Payment Methods

| Payment type | SePay `payment_method` | Description |
|--------------|------------------------|-------------|
| `sepay_bank_transfer` | `BANK_TRANSFER` | VietQR bank transfer |
| `sepay_napas` | `NAPAS_BANK_TRANSFER` | Napas bank transfer |
| `sepay_card` | `CARD` | Visa / Mastercard / JCB |

Each type is an independent button on the checkout page. A provider instance may
offer any subset of them via its **supported types**.

> Evaluate the security, reliability, and compliance of any payment provider on
> your own — this project does not endorse or guarantee any of them.

---

## Quick Start

1. Go to Admin Dashboard → **Settings** → **Payment Settings** tab
2. Enable **Payment**
3. Configure basic parameters (amount range, timeout, etc.)
4. Add at least one SePay provider instance in **Provider Management**
5. Register the webhook URL in the SePay merchant portal (see [Webhook Configuration](#webhook-configuration))
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

The gateway settlement currency is **VND**, which is a zero-decimal currency: an
amount of `250000` means ₫250,000 and fractional amounts are rejected. A provider
instance may select another currency only if the merchant account is enabled for
it.

The subscription rate setting (1 USD = X gateway currency) converts a plan's USD
price into the settlement currency. It is opt-in: leave it at `0` and plan prices
are charged as-is.

---

## Provider Configuration

A SePay provider instance needs the following credentials from the
[SePay merchant portal](https://my.sepay.vn):

| Field | Sensitive | Description |
|-------|-----------|-------------|
| `merchantId` | No | Merchant code |
| `secretKey` | **Yes** | Merchant secret key, used to sign the checkout and verify callbacks |
| `env` | No | `production` or `sandbox` |
| `currency` | No | Settlement currency, `VND` by default |

`secretKey` is encrypted at rest with `security.secret_encryption_key` and is
never returned by the admin API. When editing an instance, leaving the secret
field blank keeps the stored value.

`merchantId`, `secretKey`, `env` and `currency` cannot be changed while the
instance still has in-progress orders — those orders were signed with the old
values and would fail verification.

---

## Provider Instance Management

Add instances in Admin Dashboard **Settings → Payment Settings → Provider Management**.

- **Supported types** — which of the three methods this instance offers
- **Payment mode** — `redirect` (required for SePay; see below), `qrcode`, `popup`
- **Limits** — per-method daily limit and single-order min/max; a limits entry
  under the gateway key `sepay` applies to every method of the instance
- **Sort order** — display order on the checkout page

Several enabled instances may serve the same payment method; orders are spread
across them by the configured load-balance strategy.

---

## Webhook Configuration

Register this URL as the payment notification (IPN) endpoint in the SePay
merchant portal:

```
https://your-domain.com/api/v1/payment/webhook/sepay
```

The admin provider dialog shows the exact URL for your deployment.

**How a callback is trusted.** The callback body is only used to locate the
order. Whether the order is actually paid — and for how much — is decided by a
server-to-server order query against the SePay Open API using the merchant's
Basic credentials. A forged callback therefore cannot mark an order paid. When
the callback carries a `signature`, it is verified with HMAC-SHA256 over the
gateway's canonical field order and a mismatch is rejected outright.

---

## Payment Flow

```
User selects amount and payment method
       │
       ▼
  Create Order (PENDING)
  ├─ Validate amount range, pending order count, daily limit
  ├─ Load balance to select provider instance
  └─ Sign the SePay checkout fields (local HMAC, no upstream call)
       │
       ▼
  Browser is sent to /api/v1/payment/checkout?token=<resume token>
  └─ That page auto-submits the signed form to SePay
     (SePay's checkout accepts POST only, so a plain redirect cannot reach it)
       │
       ▼
  User completes payment at SePay
       │
       ▼
  Webhook callback → upstream order query confirms status and amount → Order PAID
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

---

## Refunds

The SePay SDK exposes no refund API, so Sub2API has no refund flow: there are no
refund endpoints, no admin refund actions, and no user refund requests. Handle
refunds directly in the SePay merchant portal and adjust the user's balance
manually from the admin dashboard.
