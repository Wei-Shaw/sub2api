# sub2api Multi-Currency Top-up Frontend Integration Guide

Tài liệu này dành cho frontend web/app khác muốn tích hợp flow **nạp tiền local currency** vào sub2api.

## 1. Mục tiêu tích hợp

sub2api vẫn hạch toán nội bộ bằng **USD** duy nhất, nhưng người dùng có thể nạp bằng tiền địa phương ở checkout:

- Việt Nam: `VND`
- Trung Quốc: `CNY`
- Hàn Quốc: `KRW` nếu backend/provider đã bật
- USD: chỉ dùng khi backend trả `USD` trong `allowed_payment_currencies`

Frontend chỉ cần làm đúng flow:

1. Lấy cấu hình checkout từ backend.
2. Cho user chọn currency/provider hợp lệ.
3. Gọi quote endpoint để backend khóa tỷ giá/amount.
4. Gọi create order bằng `quote_id`.
5. Điều hướng/hiển thị QR/Stripe/WeChat theo response.
6. Poll/verify trạng thái order nếu cần.

> Rule quan trọng: frontend **không tự quyết định USD credit cuối cùng**. Frontend có thể preview, nhưng backend quote/create-order là nguồn sự thật.

---

## 2. API envelope chung

Base API mặc định:

```text
/api/v1
```

Response thành công có dạng:

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

Frontend hiện tại unwrap `data`. Nếu app khác không có interceptor thì phải đọc dữ liệu từ `response.data.data`.

Response lỗi thường có dạng:

```json
{
  "code": 400,
  "message": "payment quote currency mismatch",
  "reason": "INVALID_PAYMENT_QUOTE",
  "metadata": {}
}
```

Auth:

- Các endpoint user-facing `/payment/...` cần JWT auth.
- Header:

```http
Authorization: Bearer <access_token>
Content-Type: application/json
Accept-Language: vi
```

Public recovery endpoints không cần auth, xem mục 9.

---

## 3. Concepts bắt buộc frontend phải hiểu

### 3.1 Ledger amount vs payment amount

| Field | Ý nghĩa |
|---|---|
| `payment_amount` | Số tiền user trả bằng local currency, ví dụ `200000 VND` |
| `payment_currency` | Currency user trả, ví dụ `VND` |
| `ledger_amount` | Số USD backend cộng vào balance/subscription, ví dụ `7.84 USD` |
| `ledger_currency` | Luôn là `USD` ở hệ thống hiện tại |
| `fx_rate` hoặc `fx_rate_payment_to_ledger` | Tỷ giá snapshot: `ledger = payment * rate` |
| `fx_source` | Nguồn tỷ giá admin đã cấu hình, ví dụ `manual_admin` |
| `fx_timestamp` | Thời điểm tỷ giá được snapshot |

Ví dụ:

```text
200000 VND * 0.0000392 = 7.84 USD
```

User thấy: “Bạn thanh toán ₫200,000” và “Bạn nhận ~$7.84 USD”.

### 3.2 amount_mode

`amount_mode` cho biết field `amount` trong request đang là loại tiền nào.

| amount_mode | Dùng khi nào | Ý nghĩa của `amount` |
|---|---|---|
| `payment` | Nạp balance bằng local currency | `amount` là số tiền user trả, ví dụ `200000` VND |
| `ledger` | Flow USD/ledger cũ | `amount` là USD ledger amount |

Với app mới triển khai local top-up, dùng:

```json
"amount_mode": "payment"
```

### 3.3 quote_id

`quote_id` là signed quote token do backend cấp, có TTL ngắn, hiện tại khoảng **10 phút**.

Quote khóa bất biến:

- user
- order type
- payment type
- payment currency
- payment amount
- ledger amount
- FX rate/source/timestamp

Frontend nên luôn tạo quote trước khi tạo order, rồi gửi `quote_id` vào create order.

---

## 4. Endpoint 1: lấy checkout info

```http
GET /api/v1/payment/checkout-info
```

### Response data shape

```ts
interface CheckoutInfoResponse {
  methods: Record<string, MethodLimit>
  global_min: number
  global_max: number
  plans: SubscriptionPlan[]
  balance_disabled: boolean
  balance_recharge_multiplier: number
  recharge_fee_rate: number
  help_text: string
  help_image_url: string
  stripe_publishable_key: string
  paddle_client_token: string
  paddle_environment: string

  ledger_currency: string
  allowed_payment_currencies: string[]
  manual_fx_rates: Record<string, number>
  currency_meta: Record<string, CurrencyMeta>
  fx_status: PaymentFXStatus
}

interface MethodLimit {
  daily_limit: number
  daily_used: number
  daily_remaining: number
  single_min: number
  single_max: number
  fee_rate: number
  available: boolean
}

interface CurrencyMeta {
  minor_units: number
  symbol: string
}

interface PaymentFXStatus {
  source: string
  updated_at?: string
  stale_after_seconds: number
  stale: boolean
  missing_currencies: string[]
}
```

### Ví dụ response rút gọn

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "methods": {
      "sepay": {
        "daily_limit": 100000,
        "daily_used": 0,
        "daily_remaining": 100000,
        "single_min": 1,
        "single_max": 1000,
        "fee_rate": 0,
        "available": true
      },
      "stripe": {
        "daily_limit": 100000,
        "daily_used": 0,
        "daily_remaining": 100000,
        "single_min": 1,
        "single_max": 1000,
        "fee_rate": 0.03,
        "available": true
      }
    },
    "plans": [],
    "balance_disabled": false,
    "balance_recharge_multiplier": 1,
    "recharge_fee_rate": 0,
    "ledger_currency": "USD",
    "allowed_payment_currencies": ["USD", "CNY", "VND"],
    "manual_fx_rates": {
      "USD": 1,
      "CNY": 0.14,
      "VND": 0.0000392
    },
    "currency_meta": {
      "USD": { "minor_units": 2, "symbol": "$" },
      "CNY": { "minor_units": 2, "symbol": "¥" },
      "VND": { "minor_units": 0, "symbol": "₫" },
      "KRW": { "minor_units": 0, "symbol": "₩" }
    },
    "fx_status": {
      "source": "manual_admin",
      "updated_at": "2026-04-29T12:00:00Z",
      "stale_after_seconds": 86400,
      "stale": false,
      "missing_currencies": []
    }
  }
}
```

### Frontend nên làm gì với checkout-info?

- Nếu `balance_disabled = true`, ẩn/disable nạp balance.
- Chỉ hiển thị payment method có `available = true`.
- Chỉ hiển thị currency nằm trong `allowed_payment_currencies`.
- Nếu `fx_status.stale = true`, hiện cảnh báo tỷ giá cũ.
- Nếu currency nằm trong `fx_status.missing_currencies`, không cho submit currency đó.
- Dùng `currency_meta` để format số tiền; không hardcode `¥`, `$`, `₫`.

---

## 5. Chọn currency mặc định

Recommended heuristic:

1. Nếu user từng chọn currency và vẫn còn allowed, dùng lại.
2. Theo locale/browser:
   - `vi-*` → `VND`
   - `ko-*` → `KRW`
   - `zh-*` → `CNY`
3. Nếu không match, dùng currency đầu tiên trong `allowed_payment_currencies`.
4. Nếu không match, chỉ dùng `USD` khi backend trả `USD` trong `allowed_payment_currencies`; nếu không thì block submit và báo thiếu cấu hình currency.

Lưu ý: chỉ chọn `KRW` nếu backend trả `KRW` trong `allowed_payment_currencies` và provider đang hỗ trợ.

---

## 6. Endpoint 2: tạo quote trước khi nạp

```http
POST /api/v1/payment/quote
```

### Request cho balance top-up local currency

```json
{
  "amount": 200000,
  "amount_mode": "payment",
  "payment_currency": "VND",
  "payment_type": "sepay",
  "order_type": "balance"
}
```

Field:

| Field | Required | Ghi chú |
|---|---:|---|
| `amount` | yes | Với `amount_mode=payment`, đây là local amount user nhập |
| `amount_mode` | recommended | Dùng `payment` cho local top-up |
| `payment_currency` | yes | `VND`, `CNY`, `USD`, ... |
| `payment_type` | yes | `sepay`, `stripe`, `paddle`, `alipay`, `wxpay`, ... |
| `order_type` | recommended | `balance` cho nạp tiền |
| `plan_id` | optional | Dùng cho subscription flow, không cần cho top-up |

### Response data shape

```ts
interface PaymentQuoteResult {
  quote_id: string
  expires_at: string
  amount: number
  amount_mode: 'ledger' | 'payment'
  payment_type: string
  order_type: string
  plan_id?: number
  payment_amount: number
  payment_currency: string
  ledger_amount: number
  ledger_currency: string
  fx_rate: number
  fx_source: string
  fx_timestamp: string
}
```

### Ví dụ response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "quote_id": "eyJ...signed-token...",
    "expires_at": "2026-04-29T12:10:00Z",
    "amount": 200000,
    "amount_mode": "payment",
    "payment_type": "sepay",
    "order_type": "balance",
    "payment_amount": 200000,
    "payment_currency": "VND",
    "ledger_amount": 7.84,
    "ledger_currency": "USD",
    "fx_rate": 0.0000392,
    "fx_source": "manual_admin",
    "fx_timestamp": "2026-04-29T12:00:00Z"
  }
}
```

### UX sau khi có quote

Hiển thị rõ:

```text
Bạn thanh toán: ₫200,000
Bạn nhận: $7.84 USD
Tỷ giá đã khóa đến: 12:10
Nguồn tỷ giá: manual_admin
```

Nếu quote expired hoặc user đổi amount/currency/provider, tạo quote mới.

---

## 7. Endpoint 3: tạo order từ quote_id

```http
POST /api/v1/payment/orders
```

### Request recommended

```json
{
  "amount": 200000,
  "amount_mode": "payment",
  "payment_currency": "VND",
  "quote_id": "eyJ...signed-token...",
  "payment_type": "sepay",
  "order_type": "balance",
  "return_url": "https://your-web.example.com/payment/result",
  "payment_source": "hosted_redirect",
  "is_mobile": false
}
```

Có thể gửi đủ `amount`, `amount_mode`, `payment_currency` cùng `quote_id` để backend validate mismatch. Khi có `quote_id`, backend sẽ dùng snapshot trong quote làm nguồn thật.

### Response data shape

```ts
type CreateOrderResultType = 'order_created' | 'oauth_required' | 'jsapi_ready'

interface CreateOrderResult {
  order_id: number
  amount: number
  payment_amount?: number
  ledger_amount?: number
  payment_currency?: string
  ledger_currency?: string
  fx_rate_payment_to_ledger?: number
  fx_source?: string
  fx_timestamp?: string

  pay_url?: string
  qr_code?: string
  client_secret?: string
  checkout_id?: string
  pay_amount: number
  fee_rate: number
  expires_at: string
  result_type?: CreateOrderResultType
  payment_type?: string
  out_trade_no?: string
  payment_mode?: string
  resume_token?: string
  oauth?: WechatOAuthInfo
  jsapi?: WechatJSAPIPayload
  jsapi_payload?: WechatJSAPIPayload
}
```

`pay_amount` là số tiền provider cần thu sau fee nếu có. Với nhiều UI, phần chính nên hiển thị `pay_amount` hoặc `payment_amount` tùy context:

- Hướng dẫn thanh toán/provider: ưu tiên local amount provider cần thu (`pay_amount` nếu có fee).
- Credit nội bộ: dùng `ledger_amount` + `ledger_currency`.

### Ví dụ SePay response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "order_id": 123,
    "amount": 7.84,
    "payment_amount": 200000,
    "ledger_amount": 7.84,
    "payment_currency": "VND",
    "ledger_currency": "USD",
    "fx_rate_payment_to_ledger": 0.0000392,
    "fx_source": "manual_admin",
    "fx_timestamp": "2026-04-29T12:00:00Z",
    "qr_code": "https://...",
    "pay_amount": 200000,
    "fee_rate": 0,
    "expires_at": "2026-04-29T12:30:00Z",
    "payment_type": "sepay",
    "out_trade_no": "PAY20260429...",
    "payment_mode": "qrcode",
    "resume_token": "eyJ..."
  }
}
```

---

## 8. Xử lý launch payment theo provider

Sau create order, frontend quyết định action theo response.

### 8.1 QR/bank transfer providers: SePay, native QR

Nếu response có `qr_code`:

- Hiển thị QR image.
- Hiển thị số tiền cần chuyển: `pay_amount` + `payment_currency`.
- Hiển thị order code/out trade no nếu UI cần.
- Poll hoặc nút “Tôi đã thanh toán” gọi verify endpoint.

### 8.2 Redirect providers

Nếu response có `pay_url`:

- Redirect cùng tab hoặc mở popup tùy UX.
- Lưu recovery state trước khi redirect.

### 8.3 Stripe

Nếu response có `client_secret`:

- Dùng Stripe SDK với `stripe_publishable_key` từ checkout-info/config.
- Confirm PaymentIntent theo UI hiện tại của app.

### 8.4 Paddle

Nếu response có `checkout_id`:

- Dùng Paddle SDK với `paddle_client_token` và `paddle_environment`.

### 8.5 WeChat OAuth / JSAPI

Nếu:

```json
"result_type": "oauth_required"
```

và có:

```json
"oauth": { "authorize_url": "..." }
```

thì redirect user tới `oauth.authorize_url`.

Nếu:

```json
"result_type": "jsapi_ready"
```

và có `jsapi` hoặc `jsapi_payload`, gọi WeixinJSBridge.

Quan trọng với WeChat resume: preserve các query/state sau nếu tự build redirect:

- `payment_type`
- `amount`
- `order_type`
- `plan_id`
- `amount_mode`
- `payment_currency`
- `quote_id`

---

## 9. Recovery / kiểm tra trạng thái order

### 9.1 Verify order bằng out_trade_no

Authenticated:

```http
POST /api/v1/payment/orders/verify
```

Body:

```json
{
  "out_trade_no": "PAY20260429..."
}
```

Public legacy-compatible:

```http
POST /api/v1/payment/public/orders/verify
```

Body giống trên.

### 9.2 Resolve public bằng resume_token

```http
POST /api/v1/payment/public/orders/resolve
```

Body:

```json
{
  "resume_token": "eyJ..."
}
```

Dùng endpoint này cho result page sau redirect nếu app không chắc còn auth state.

### 9.3 Get order authenticated

```http
GET /api/v1/payment/orders/{id}
```

### 9.4 My orders

```http
GET /api/v1/payment/orders/my?page=1&page_size=20&status=PENDING
```

---

## 10. Provider/currency compatibility hiện tại

Frontend không nên hardcode quá sâu, nhưng có thể dùng mapping UX ban đầu này để tránh người dùng chọn option chắc chắn fail:

| Provider/method | Currency nên dùng |
|---|---|
| `sepay` | `VND` only |
| `alipay`, `alipay_direct` | thường `CNY` |
| `wxpay`, `wxpay_direct` | thường `CNY` |
| `stripe` | dynamic, tùy backend/provider config; vẫn phải để backend validate |
| `paddle` | dynamic/contract-tested; vẫn phải để backend validate |

Backend vẫn là nguồn validate cuối cùng. Nếu provider/currency mismatch, backend trả 400-style error trước khi gọi provider.

---

## 11. Formatting tiền tệ

Không hardcode decimal places.

Recommended helper:

```ts
interface CurrencyMeta {
  minor_units: number
  symbol: string
}

function normalizeCurrency(currency?: string) {
  return (currency || '').trim().toUpperCase()
}

function currencyMinorUnits(currency: string, meta: Record<string, CurrencyMeta>) {
  const code = normalizeCurrency(currency)
  const units = meta[code]?.minor_units
  if (units === undefined) {
    throw new Error(`missing currency metadata for ${code}`)
  }
  return units
}

function formatMoney(amount: number, currency: string, meta: Record<string, CurrencyMeta>) {
  const code = normalizeCurrency(currency)
  const units = currencyMinorUnits(code, meta)
  const symbol = meta[code]?.symbol
  if (!symbol) {
    throw new Error(`missing currency symbol for ${code}`)
  }
  return `${symbol}${Number(amount || 0).toLocaleString(undefined, {
    minimumFractionDigits: units,
    maximumFractionDigits: units,
  })}`
}
```

Ví dụ:

```ts
formatMoney(200000, 'VND', meta) // ₫200,000 hoặc format locale tương ứng
formatMoney(7.84, 'USD', meta)   // $7.84
formatMoney(10000, 'KRW', meta)  // ₩10,000
```

---

## 12. Recommended frontend state model

```ts
type PaymentAmountMode = 'ledger' | 'payment'
type OrderType = 'balance' | 'subscription'

type TopupState = {
  checkoutInfo?: CheckoutInfoResponse
  selectedCurrency: string
  selectedPaymentType: string
  amountInput: number
  quote?: PaymentQuoteResult
  quoteLoading: boolean
  quoteError?: string
  order?: CreateOrderResult
  orderLoading: boolean
  orderError?: string
}
```

Quote invalidation rules:

Tạo quote mới khi một trong các field đổi:

- `amountInput`
- `selectedCurrency`
- `selectedPaymentType`
- `order_type`
- `plan_id`

Không submit nếu:

- chưa có quote
- quote expired
- quote currency/payment type khác selection hiện tại
- FX stale/missing theo checkout-info

---

## 13. End-to-end top-up pseudo-code

```ts
async function loadCheckout() {
  const res = await api.get('/payment/checkout-info')
  const info = res.data.data

  state.checkoutInfo = info
  state.selectedCurrency = chooseDefaultCurrency(info.allowed_payment_currencies)
  state.selectedPaymentType = chooseDefaultPaymentType(info.methods)
}

async function createQuote() {
  const res = await api.post('/payment/quote', {
    amount: state.amountInput,
    amount_mode: 'payment',
    payment_currency: state.selectedCurrency,
    payment_type: state.selectedPaymentType,
    order_type: 'balance',
  })

  state.quote = res.data.data
}

async function createOrder() {
  if (!state.quote) throw new Error('Missing payment quote')

  const res = await api.post('/payment/orders', {
    amount: state.amountInput,
    amount_mode: 'payment',
    payment_currency: state.selectedCurrency,
    quote_id: state.quote.quote_id,
    payment_type: state.selectedPaymentType,
    order_type: 'balance',
    return_url: `${window.location.origin}/payment/result`,
    payment_source: 'hosted_redirect',
    is_mobile: /Android|iPhone|iPad/i.test(navigator.userAgent),
  })

  const order = res.data.data
  state.order = order
  launchPayment(order)
}

function launchPayment(order: CreateOrderResult) {
  saveRecovery(order)

  if (order.result_type === 'oauth_required' && order.oauth?.authorize_url) {
    window.location.href = order.oauth.authorize_url
    return
  }

  if (order.result_type === 'jsapi_ready' && (order.jsapi || order.jsapi_payload)) {
    callWeixinJSBridge(order.jsapi || order.jsapi_payload)
    return
  }

  if (order.client_secret) {
    openStripe(order.client_secret)
    return
  }

  if (order.checkout_id) {
    openPaddle(order.checkout_id)
    return
  }

  if (order.pay_url) {
    window.location.href = order.pay_url
    return
  }

  if (order.qr_code) {
    showQrCode(order.qr_code)
    return
  }

  throw new Error('Unsupported payment response')
}
```

---

## 14. UI copy nên hiển thị

Ở màn hình nhập amount:

```text
Đơn vị thanh toán: VND
Số tiền thanh toán: ₫200,000
Dự kiến nhận: $7.84 USD
Tỷ giá: 1 VND = 0.0000392 USD
Tỷ giá cập nhật lúc: 2026-04-29 12:00 UTC
```

Ở order/payment panel:

```text
Cần thanh toán: ₫200,000
Số dư được cộng: $7.84 USD
Mã đơn hàng: PAY20260429...
Hết hạn: 2026-04-29 12:30 UTC
```

Không nên chỉ hiện một con số `amount`, vì user có thể nhầm local amount với USD credit.

---

## 15. Common errors frontend nên xử lý

| reason/message | Ý nghĩa | Cách xử lý UX |
|---|---|---|
| `PAYMENT_DISABLED` | Payment đang tắt | Hiện thông báo bảo trì/thử lại sau |
| `INVALID_PAYMENT_QUOTE` | Quote sai, expired, mismatch | Tạo lại quote |
| `PAYMENT_QUOTE_EXPIRED` | Quote hết hạn | Tạo lại quote và yêu cầu user xác nhận lại amount |
| provider/currency mismatch | Currency không hỗ trợ method | Yêu cầu đổi provider/currency |
| missing FX rate | Chưa có tỷ giá currency đó | Disable currency, báo admin/config chưa sẵn sàng |
| stale FX | Tỷ giá quá cũ | Hiện cảnh báo hoặc block theo backend |
| daily/single limit exceeded | Vượt limit | Hiển thị min/max/daily remaining từ checkout-info |

Frontend không cần tự parse hết `message`, nhưng nên ưu tiên `reason` nếu backend trả.

---

## 16. Checklist tích hợp cho frontend app khác

- [ ] Có JWT auth và gửi `Authorization: Bearer ***`.
- [ ] Gọi `GET /payment/checkout-info` khi vào payment page.
- [ ] Dùng `allowed_payment_currencies` + `currency_meta` thay vì hardcode currency.
- [ ] Với balance top-up, gửi `amount_mode: "payment"`.
- [ ] Gọi `POST /payment/quote` trước khi create order.
- [ ] Hiển thị `payment_amount/payment_currency` và `ledger_amount/ledger_currency` riêng biệt.
- [ ] Gửi `quote_id` vào `POST /payment/orders`.
- [ ] Lưu recovery state gồm `order_id`, `out_trade_no`, `resume_token`, amount/currency fields.
- [ ] Xử lý các kiểu response: `qr_code`, `pay_url`, `client_secret`, `checkout_id`, `oauth`, `jsapi`.
- [ ] Result page có thể resolve bằng `resume_token` hoặc verify bằng `out_trade_no`.
- [ ] Không tự credit balance ở frontend; chỉ backend/webhook quyết định paid/completed.

---

## 17. Minimal TypeScript types copy-paste

```ts
export type PaymentAmountMode = 'ledger' | 'payment'
export type OrderType = 'balance' | 'subscription'

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

export interface MethodLimit {
  daily_limit: number
  daily_used: number
  daily_remaining: number
  single_min: number
  single_max: number
  fee_rate: number
  available: boolean
}

export interface CheckoutInfoResponse {
  methods: Record<string, MethodLimit>
  global_min: number
  global_max: number
  balance_disabled: boolean
  balance_recharge_multiplier: number
  recharge_fee_rate: number
  help_text: string
  help_image_url: string
  stripe_publishable_key: string
  paddle_client_token: string
  paddle_environment: string
  ledger_currency: string
  allowed_payment_currencies: string[]
  manual_fx_rates: Record<string, number>
  currency_meta: Record<string, CurrencyMeta>
  fx_status: PaymentFXStatus
}

export interface CreatePaymentQuoteRequest {
  amount: number
  amount_mode?: PaymentAmountMode
  payment_currency?: string
  payment_type: string
  order_type: OrderType
  plan_id?: number
}

export interface PaymentQuoteResult {
  quote_id: string
  expires_at: string
  amount: number
  amount_mode: PaymentAmountMode
  payment_type: string
  order_type: OrderType
  plan_id?: number
  payment_amount: number
  payment_currency: string
  ledger_amount: number
  ledger_currency: string
  fx_rate: number
  fx_source: string
  fx_timestamp: string
}

export interface CreateOrderRequest {
  amount: number
  amount_mode?: PaymentAmountMode
  payment_currency?: string
  quote_id?: string
  payment_type: string
  order_type: OrderType
  plan_id?: number
  return_url?: string
  payment_source?: string
  openid?: string
  wechat_resume_token?: string
  is_mobile?: boolean
}

export interface CreateOrderResult {
  order_id: number
  amount: number
  payment_amount?: number
  ledger_amount?: number
  payment_currency?: string
  ledger_currency?: string
  fx_rate_payment_to_ledger?: number
  fx_source?: string
  fx_timestamp?: string
  pay_url?: string
  qr_code?: string
  client_secret?: string
  checkout_id?: string
  pay_amount: number
  fee_rate: number
  expires_at: string
  result_type?: 'order_created' | 'oauth_required' | 'jsapi_ready'
  payment_type?: string
  out_trade_no?: string
  payment_mode?: string
  resume_token?: string
  oauth?: {
    authorize_url?: string
    appid?: string
    openid?: string
    scope?: string
    state?: string
    redirect_url?: string
  }
  jsapi?: Record<string, string>
  jsapi_payload?: Record<string, string>
}
```

---

## 18. Ghi chú bảo mật/đúng nghiệp vụ

- Không có FX provider/API key phía client; tỷ giá do Admin cấu hình trong backend DB/settings và frontend chỉ đọc qua checkout/quote response.
- Không đưa provider secret, webhook key, hoặc bất kỳ credential backend nào vào frontend.
- Frontend không được tự tính amount để credit balance.
- Không gọi webhook từ frontend.
- Không tin `manual_fx_rates` ở frontend để tạo order cuối cùng; chỉ dùng preview. Quote/create-order backend mới là nguồn sự thật.
- Không dùng `quote_id` quá hạn. Khi hết hạn, gọi quote mới.
- Không mở `KRW` nếu backend không trả `KRW` trong `allowed_payment_currencies`.

---

## 19. Flow ngắn nhất cho app chỉ cần nạp VND bằng SePay

1. Login lấy JWT.
2. `GET /api/v1/payment/checkout-info`.
3. Kiểm tra `sepay` available và `VND` allowed.
4. User nhập `amount = 200000`.
5. `POST /api/v1/payment/quote`:

```json
{
  "amount": 200000,
  "amount_mode": "payment",
  "payment_currency": "VND",
  "payment_type": "sepay",
  "order_type": "balance"
}
```

6. Hiển thị quote: `payment_amount VND`, `ledger_amount USD`.
7. `POST /api/v1/payment/orders` với `quote_id`.
8. Hiển thị QR `qr_code`, amount cần trả `pay_amount VND`, và order code `out_trade_no`.
9. Poll/verify bằng `/payment/orders/verify` hoặc dùng public resolve/verify ở result page.
