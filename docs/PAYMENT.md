# Payment System Configuration Guide

Sub2API has a built-in payment system that enables user self-service top-up and subscription purchase without deploying a separate payment service.

---

## Table of Contents

- [Supported Payment Methods](#supported-payment-methods)
- [Quick Start](#quick-start)
- [System Settings](#system-settings)
- [Currency and Amounts](#currency-and-amounts)
- [Provider Configuration](#provider-configuration)
- [Provider Instance Management](#provider-instance-management)
- [Webhook Configuration](#webhook-configuration)
- [Payment Flow](#payment-flow)
- [Upgrading from an Older Release](#upgrading-from-an-older-release)

---

## Supported Payment Methods

| Provider | User-facing method | Settlement currency | How the payer pays |
|----------|--------------------|---------------------|--------------------|
| **SePay** | Vietnamese bank transfer | VND (fixed) | Scans a VietQR code, or copies the bank details and transfers manually. SePay watches the merchant bank account and pushes a webhook when a matching credit lands. |
| **NOWPayments** | Cryptocurrency | Fiat price (USD by default), buyer pays any coin | Opens a hosted NOWPayments invoice; NOWPayments owns coin selection and the exchange-rate countdown. |

> Each provider backs exactly one user-facing method, so **a payment type and a provider key are the same string** (`sepay`, `nowpayments`). There is no alias or routing layer to configure.

> Evaluate the security, reliability, and compliance of any third-party payment provider yourself — this project does not endorse or guarantee any of them.

---

## Quick Start

1. Go to Admin Dashboard → **Settings** → **Payment Settings**
2. Enable **Payment**
3. Configure the basics (amount range, order timeout, fee rate)
4. Add at least one provider instance in **Provider Management** and enable it
5. If you sell USD-priced plans through SePay, set the **USD → VND rate** (see [Currency and Amounts](#currency-and-amounts))
6. Register the webhook URL in the provider's own dashboard (see [Webhook Configuration](#webhook-configuration))

---

## System Settings

Configure the following in Admin Dashboard **Settings → Payment Settings**.

### Basic Settings

| Setting | Description | Default |
|---------|-------------|---------|
| **Enable Payment** | Master switch for the payment system | Off |
| **Product Name Prefix / Suffix** | Wraps the product description sent to the provider | - |
| **Minimum Amount** | Minimum single top-up amount | 1 |
| **Maximum Amount** | Maximum single top-up amount (empty = unlimited) | - |
| **Daily Limit** | Per-user daily cumulative limit (empty = unlimited) | - |
| **Order Timeout** | Order timeout in minutes (minimum 1) | 30 |
| **Max Pending Orders** | Maximum concurrent in-progress orders per user | 3 |
| **Recharge Fee Rate** | Percentage added on top of the order amount, 0–100, at most 2 decimals | 0 |
| **Balance Recharge Multiplier** | Credit granted per unit paid; must be greater than 0 (e.g. `1.1` = 10% bonus credit) | 1 |
| **USD → VND Rate** | Converts a USD price into dong on SePay channels; `0` disables conversion | 0 |
| **Load Balance Strategy** | Strategy for selecting provider instances | Round Robin |
| **Disable Balance Payment** | Blocks paying for plans out of the account balance | Off |

### Enabled Payment Methods

The checkout page shows a method only when **both** are true:

- the method (`sepay` / `nowpayments`) is in the enabled payment types list, and
- at least one **enabled** provider instance of that key exists and accepts the order amount

Leaving the enabled-types list empty falls back to whatever provider instances are enabled.

### Load Balance Strategies

| Strategy | Description |
|----------|-------------|
| **Round Robin** (default) | Distribute orders to instances in rotation |
| **Least Amount** | Prefer instances with the lowest daily cumulative amount |

### Cancel Rate Limiting

Prevents users from repeatedly creating and cancelling orders:

| Setting | Description |
|---------|-------------|
| **Enable Limit** | Toggle |
| **Window Mode** | Sliding / Fixed window |
| **Time Window** + **Unit** | Window duration, in minutes or hours |
| **Max Cancels** | Maximum cancellations allowed within the window |

### Help Information

| Setting | Description |
|---------|-------------|
| **Help Image** | Customer-service QR code or help image (supports upload) |
| **Help Text** | Instructions displayed on the payment page |

---

## Currency and Amounts

Every price in the panel — plan prices and balance top-ups alike — is expressed in **USD**. What the gateway charges depends on the instance:

- **NOWPayments** quotes the invoice in the instance's configured currency (USD by default) and the buyer pays any coin.
- **SePay** always collects **VND**. A USD figure sent to a dong channel unchanged would undercharge by roughly four orders of magnitude, so the **USD → VND rate** is what converts it.

> **If the rate is `0` (unset), no conversion happens and the USD figure is charged as a dong figure.** Set the rate before enabling a SePay instance.

Other amount rules:

- The fee rate is applied on top of the order amount to produce the payable amount.
- Credited balance is `paid amount × balance recharge multiplier`, rounded to 2 decimals.
- VND is a zero-decimal currency: dong amounts are rounded to whole dong before being sent to the bank.

---

## Provider Configuration

Select the provider type when adding an instance in **Provider Management → Add Provider**. The config is validated when you save an **enabled** instance, so a missing or unusable field is reported in the dialog rather than at order time.

### SePay

SePay is a reconciliation service for Vietnamese bank accounts. It has **no "create payment" API**: Sub2API renders a VietQR payload that prefills a transfer to your bank account with the order code as the transfer content, and SePay pushes a webhook once a matching credit lands in the account.

| Parameter | Description | Required |
|-----------|-------------|----------|
| **Bank account number** (`accountNumber`) | The merchant account SePay watches | Yes |
| **Bank code** (`bankCode`) | Short code of the bank, e.g. `MB`, `VCB`, `TCB`, `ACB` | Yes (unless a BIN is set) |
| **Napas BIN** (`bankBin`) | Six-digit VietQR bank identifier. Only needed when the bank code is not recognised; it overrides the code when both are set | No |
| **Account holder name** (`accountName`) | Shown to the payer in the manual transfer details | No |
| **API key** (`apiKey`) | Shared secret configured on the SePay webhook. Callbacks whose `Authorization` header does not match are rejected | Yes |
| **User API token** (`apiToken`) | Enables polling SePay's transaction list. Without it, an order is only ever confirmed by the webhook | No |

Setup in the SePay dashboard:

1. Connect the bank account you want to collect into
2. Create a webhook pointing at `https://your-domain.com/api/v1/payment/webhook/sepay`
3. Set the webhook API key and copy the same value into the instance's **API key** field
4. (Optional) Create a user API token and paste it into **User API token** so pending orders can also be reconciled by polling

### NOWPayments

NOWPayments is a crypto gateway. Sub2API opens a **hosted invoice** and sends the payer to it.

| Parameter | Description | Required |
|-----------|-------------|----------|
| **API Key** (`apiKey`) | NOWPayments API key | Yes |
| **IPN secret** (`ipnSecret`) | Verifies the HMAC-SHA512 signature on every IPN callback | Yes |
| **Payment currency** (`currency`) | Fiat currency the invoice is priced in: USD, EUR, VND, SGD, AUD, GBP | No (default USD) |
| **API Base URL** (`apiBase`) | Override the API endpoint | No (default `https://api.nowpayments.io/v1`) |

Setup in the NOWPayments dashboard:

1. Generate an API key under **Settings → Payments**
2. Set the IPN callback URL to `https://your-domain.com/api/v1/payment/webhook/nowpayments`
3. Generate the IPN secret and copy it into the instance's **IPN secret** field

---

## Provider Instance Management

You can create **multiple instances** of the same provider type for load balancing and risk control:

- **Multi-instance load balancing** — orders are distributed by round-robin or least-amount
- **Independent limits** — each instance has its own min/max amount and daily limit; instances over their limit are skipped during selection
- **Independent toggle** — enable or disable an instance without affecting the others
- **Payment mode** — `qrcode` renders the payload in-page, `redirect` sends the payer to the provider's hosted page. SePay is a QR flow; NOWPayments is a redirect flow
- **Ordering** — drag to reorder instances

### Safety Guards

Some changes are blocked while an instance still has in-progress orders (`PENDING`, `PAID`, `RECHARGING`), because those orders were created against the old merchant identity:

| Action | Blocked when in-progress orders exist |
|--------|----------------------------------------|
| Deleting the instance | Yes |
| Disabling the instance | Yes |
| Removing a supported payment type | Yes |
| Changing SePay `apiKey`, `apiToken`, `accountNumber`, `bankCode`, `bankBin` | Yes |
| Changing NOWPayments `apiKey`, `ipnSecret`, `apiBase`, `currency` | Yes |

Secrets (`apiKey`, `apiToken`, `ipnSecret`) are never returned by the admin API. The edit dialog shows them blank; leaving a secret blank on save keeps the stored value.

---

## Webhook Configuration

Payment callbacks are what confirm an order. Both endpoints are public (no session auth) and authenticate the request themselves.

### Callback URLs

| Provider | Callback path | Authentication |
|----------|---------------|----------------|
| **SePay** | `https://your-domain.com/api/v1/payment/webhook/sepay` | `Authorization: Apikey <apiKey>` (a `Bearer` prefix is also accepted) |
| **NOWPayments** | `https://your-domain.com/api/v1/payment/webhook/nowpayments` | `x-nowpayments-sig` — HMAC-SHA512 of the sorted JSON body, keyed by the IPN secret |

Callback URLs are auto-generated from your site domain when adding an instance — confirm the domain is correct and register the URL in the provider's dashboard.

### Notes

- Use **HTTPS**. Ensure your firewall allows callbacks from the payment platform.
- Events that carry no decision for us are acknowledged and ignored: outgoing transfers and credits with no recognisable order code (SePay), and non-terminal statuses such as `waiting` or `confirming` (NOWPayments).
- SePay matches an order **by the transfer content only**. A payer who edits the transfer description creates an unmatched credit that has to be reconciled by hand.
- Balance credit and subscription activation happen automatically once the callback verifies — no manual step.

---

## Payment Flow

```
User selects an amount or plan and a payment method
       │
       ▼
  Create Order (PENDING)
  ├─ Validate amount range, in-progress order count, daily limit
  ├─ Convert to the gateway currency (USD → VND for SePay)
  ├─ Load balance to select a provider instance
  └─ Call the provider for payment details
       │
       ▼
  User completes payment
  ├─ SePay        → VietQR code + copyable bank transfer details (bank, account,
  │                 holder, amount, transfer content), page polls order status
  └─ NOWPayments  → hosted invoice page in a new window / mobile redirect
       │
       ▼
  Webhook verified → Order PAID → RECHARGING
       │
       ▼
  Balance credited or subscription activated → Order COMPLETED
```

### Order Status Reference

| Status | Description |
|--------|-------------|
| `PENDING` | Waiting for the user to pay |
| `PAID` | Payment confirmed, awaiting fulfilment |
| `RECHARGING` | Fulfilment in progress |
| `COMPLETED` | Balance credited / subscription activated |
| `EXPIRED` | Timed out without payment |
| `CANCELLED` | Cancelled by the user or an admin |
| `FAILED` | Fulfilment failed; an admin can retry it from the order list |

> Refunds are not part of this system: neither a bank transfer nor a crypto payment exposes an automated refund API. Money returned out of band should be handled in your own books.

### Timeout and Reconciliation

- A background job runs every **60 seconds**; in a multi-instance deployment a leader lock ensures only one node performs the sweep.
- Before expiring an order the job queries upstream payment status first, so a user who paid just before the timeout is not expired out from under a delayed webhook.
- The checkout page also asks the server to re-verify a still-pending order while the payer waits — for SePay this only reaches the bank when a **user API token** is configured.

---

## Upgrading from an Older Release

Releases before the SePay/NOWPayments migration shipped EasyPay, Alipay, WeChat Pay, Stripe, and Airwallex. The migration:

- **Disables** instances of the retired providers instead of deleting them — historical orders reference them, and their merchant identity stays visible. They cannot be re-enabled: the admin UI no longer offers those keys and the provider factory rejects them.
- **Removes the refund subsystem.** Refund columns are dropped and orders parked in a refund status become `CANCELLED`.
- **Clears the enabled payment types list**, so the checkout page stops offering methods that no longer resolve to a provider. Re-select the methods you want.
- **Drops the old USD → CNY subscription rate.** The replacement rate converts to **VND** and must be set again — carrying the old number over would undercharge by roughly four orders of magnitude.

After upgrading: add a SePay and/or NOWPayments instance, set the USD → VND rate if you use SePay, re-enable the payment methods, and register the new webhook URLs.
