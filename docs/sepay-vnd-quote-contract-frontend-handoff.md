# SePay VND Quote Contract Frontend Handoff

Tài liệu này dùng để gửi frontend cho lượt fix **SePay / VND quote contract**. Backend vẫn giữ ledger nội bộ bằng `USD`; currency local chỉ dùng ở checkout/provider boundary.

## 1. Backend behavior sau lượt fix

### Nguồn cấu hình currency

Currency support của từng payment method/provider **không hardcode trong code**. Admin cấu hình ở mục **Settings → Payment** qua field `currency_capabilities` / setting backend:

```text
PAYMENT_CURRENCY_CAPABILITIES_JSON
```

Ví dụ cấu hình SePay chỉ nhận VND:

```json
{
  "methods": {
    "sepay": ["VND"]
  }
}
```

Hoặc cấu hình theo provider:

```json
{
  "providers": {
    "sepay": ["VND"]
  }
}
```

FX cũng là admin-managed trong Payment settings, không dùng client-side fallback:

```text
PAYMENT_LEDGER_CURRENCY=USD
PAYMENT_MANUAL_FX_RATES_JSON={"USD":1,"VND":0.0000392}
PAYMENT_FX_RATES_STALE_AFTER_SECONDS=86400
```

> Nếu `VND` đã được bật cho SePay nhưng thiếu FX `VND -> USD`, backend trả lỗi 4xx `FX_RATE_MISSING`, không trả 500.

## 2. Endpoint: checkout-info

```http
GET /api/v1/payment/checkout-info
Authorization: Bearer <accessToken>
```

Frontend phải ưu tiên currency ở **method-level**:

```ts
const sepayCurrencies = checkout.methods.sepay?.allowed_payment_currencies ?? []
```

`allowed_payment_currencies` global là union các method đang khả dụng, dùng để fallback/filter tổng quan. Không dùng global list để ép SePay nhận currency khác.

### Expected response rút gọn khi SePay bật VND

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "methods": {
      "sepay": {
        "payment_type": "sepay",
        "allowed_payment_currencies": ["VND"],
        "fee_rate": 0,
        "daily_limit": 0,
        "single_min": 0,
        "single_max": 0
      }
    },
    "ledger_currency": "USD",
    "allowed_payment_currencies": ["VND"],
    "manual_fx_rates": {
      "USD": 1,
      "VND": 0.0000392
    },
    "currency_meta": {
      "USD": { "minor_units": 2, "symbol": "$" },
      "VND": { "minor_units": 0, "symbol": "₫" }
    },
    "fx_status": {
      "source": "manual",
      "stale_after_seconds": 86400,
      "stale": false,
      "missing_currencies": []
    }
  }
}
```

### Nếu thiếu FX VND

Checkout vẫn có thể báo method-level `VND`, nhưng sẽ flag để UI chặn/warn:

```json
{
  "fx_status": {
    "stale": true,
    "missing_currencies": ["VND"]
  }
}
```

Frontend nên disable submit hoặc hiển thị lỗi cấu hình: “VND FX rate chưa được admin cấu hình”.

## 3. Endpoint: quote

```http
POST /api/v1/payment/quote
Authorization: Bearer <accessToken>
Content-Type: application/json
```

### Request SePay VND

```json
{
  "amount": 200000,
  "amount_mode": "payment",
  "payment_currency": "VND",
  "payment_type": "sepay",
  "order_type": "balance"
}
```

### Expected success

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "quote_id": "<signed_quote_token>",
    "expires_at": "2026-04-30T...Z",
    "amount": 200000,
    "amount_mode": "payment",
    "payment_type": "sepay",
    "order_type": "balance",
    "payment_amount": 200000,
    "payment_currency": "VND",
    "ledger_amount": 7.84,
    "ledger_currency": "USD",
    "fx_rate": 0.0000392,
    "fx_source": "manual",
    "fx_timestamp": "2026-04-30T...Z"
  }
}
```

`quote_id` khóa snapshot gồm user, amount, payment currency, ledger amount, FX rate/source/timestamp và hết hạn sau TTL ngắn. Frontend không tự tính giá trị ledger cuối cùng.

### Default khi không truyền payment_currency

Nếu method có đúng 1 currency configured, backend default theo method đó. Với SePay configured `["VND"]`, payload dưới đây sẽ quote VND, không default sang CNY:

```json
{
  "amount": 200000,
  "amount_mode": "payment",
  "payment_type": "sepay",
  "order_type": "balance"
}
```

Nếu method có nhiều currency, backend trả `PAYMENT_CURRENCY_REQUIRED`; frontend phải bắt user chọn currency.

## 4. Endpoint: create order

```http
POST /api/v1/payment/orders
Authorization: Bearer <accessToken>
Content-Type: application/json
```

Frontend nên gửi lại cùng amount/mode/currency/payment_type và kèm `quote_id`:

```json
{
  "amount": 200000,
  "amount_mode": "payment",
  "payment_currency": "VND",
  "payment_type": "sepay",
  "order_type": "balance",
  "quote_id": "<signed_quote_token>",
  "is_mobile": false
}
```

Expected response giữ đủ local amount + USD ledger snapshot:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "order_id": 123,
    "out_trade_no": "PAY20260430...",
    "qr_code": "https://...",
    "pay_url": "https://...",
    "payment_currency": "VND",
    "payment_amount": 200000,
    "ledger_currency": "USD",
    "ledger_amount": 7.84,
    "fx_rate": 0.0000392
  }
}
```

## 5. Error contract frontend cần xử lý

### Currency không support bởi method/provider

```json
{
  "code": 400,
  "message": "payment currency is not supported by selected provider",
  "reason": "UNSUPPORTED_PAYMENT_CURRENCY",
  "metadata": {
    "payment_currency": "CNY",
    "payment_type": "sepay",
    "supported_currency": "VND"
  }
}
```

Frontend action: refresh `checkout-info`, chọn lại currency theo `methods[payment_type].allowed_payment_currencies`.

### Thiếu payment_currency với method multi-currency

```json
{
  "code": 400,
  "message": "payment_currency is required for this payment method",
  "reason": "PAYMENT_CURRENCY_REQUIRED",
  "metadata": {
    "payment_type": "paddle",
    "supported_currencies": "KRW,USD"
  }
}
```

Frontend action: bắt user chọn currency, không tự guess global default.

### Thiếu FX rate

```json
{
  "code": 400,
  "message": "missing FX rate for payment currency",
  "reason": "FX_RATE_MISSING",
  "metadata": {
    "payment_currency": "VND",
    "ledger_currency": "USD"
  }
}
```

Frontend action: báo lỗi cấu hình/admin, không retry vô hạn.

### Quote mismatch/expired

```json
{
  "code": 400,
  "message": "payment quote has expired",
  "reason": "PAYMENT_QUOTE_EXPIRED"
}
```

Frontend action: tạo quote mới rồi create order lại.

## 6. TypeScript shape đề xuất

```ts
export interface CheckoutInfoResponse {
  methods: Record<string, MethodLimits>
  ledger_currency: string
  allowed_payment_currencies: string[]
  manual_fx_rates: Record<string, number>
  currency_meta: Record<string, CurrencyMeta>
  fx_status: PaymentFXStatus
}

export interface MethodLimits {
  payment_type: string
  allowed_payment_currencies?: string[]
  fee_rate: number
  daily_limit: number
  single_min: number
  single_max: number
}

export interface CurrencyMeta {
  minor_units: number
  symbol: string
}

export interface PaymentFXStatus {
  source: string
  updated_at?: string
  stale_after_seconds: number
  stale: boolean
  missing_currencies: string[]
}

export interface CreatePaymentQuoteRequest {
  amount: number
  amount_mode: 'payment' | 'ledger'
  payment_currency?: string
  payment_type: string
  order_type: 'balance' | 'subscription'
  plan_id?: number
}

export interface PaymentQuoteResult {
  quote_id: string
  expires_at: string
  amount: number
  amount_mode: 'payment' | 'ledger'
  payment_type: string
  order_type: string
  payment_amount: number
  payment_currency: string
  ledger_amount: number
  ledger_currency: string
  fx_rate: number
  fx_source: string
  fx_timestamp: string
}

export interface CreateOrderRequest extends CreatePaymentQuoteRequest {
  quote_id?: string
  is_mobile?: boolean
}
```

## 7. UI checklist

- Render currency options từ `checkout.methods[method].allowed_payment_currencies`; fallback global chỉ khi method-level thiếu.
- Với SePay, chỉ hiện/chọn `VND` khi backend trả `["VND"]`.
- VND dùng `minor_units = 0`; không cho nhập số lẻ.
- Dùng `currency_meta[currency].symbol`; không hardcode `¥`.
- Hiển thị tách biệt:
  - user trả: `payment_amount payment_currency`, ví dụ `200,000 ₫`;
  - user nhận: `ledger_amount ledger_currency`, ví dụ `$7.84 USD`.
- Nếu `fx_status.missing_currencies` chứa currency đang chọn, disable nút nạp và báo admin config thiếu FX.
- Khi quote hết hạn/mismatch, gọi quote lại trước khi create order.

## 8. E2E pseudo-code

```ts
const checkout = await paymentAPI.getCheckoutInfo()

const method = 'sepay'
const methodCurrencies = checkout.methods[method]?.allowed_payment_currencies ?? []
const currency = methodCurrencies.includes('VND') ? 'VND' : methodCurrencies[0]

if (!currency) throw new Error('No supported currency for selected payment method')
if (checkout.fx_status.missing_currencies.includes(currency)) {
  throw new Error(`Missing FX rate for ${currency}`)
}

const quote = await paymentAPI.createQuote({
  amount: 200000,
  amount_mode: 'payment',
  payment_currency: currency,
  payment_type: method,
  order_type: 'balance',
})

const order = await paymentAPI.createOrder({
  amount: 200000,
  amount_mode: 'payment',
  payment_currency: quote.payment_currency,
  payment_type: quote.payment_type,
  order_type: quote.order_type as 'balance',
  quote_id: quote.quote_id,
  is_mobile: false,
})

if (order.qr_code) showQRCode(order.qr_code)
else if (order.pay_url) window.location.href = order.pay_url
```

## 9. Admin config cần có trước khi frontend test SePay/VND

Trong Settings → Payment:

1. `ledger_currency`: `USD`
2. `manual_fx_rates`: có ít nhất:

```json
{
  "USD": 1,
  "VND": 0.0000392
}
```

3. `currency_capabilities`:

```json
{
  "methods": {
    "sepay": ["VND"]
  }
}
```

4. SePay provider instance enabled và supported type gồm `sepay`.

Nếu config trên chưa có, frontend có thể vẫn nhận lỗi đúng contract (`FX_RATE_MISSING`, `PAYMENT_CURRENCY_REQUIRED`, hoặc `UNSUPPORTED_PAYMENT_CURRENCY`) thay vì 500.
