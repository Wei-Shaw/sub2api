# Design: Tích hợp SePay Payment Gateway (VietQR + Webhook)

Ngày: 2026-08-14
Branch: `payment-sepay`
Trạng thái: Đã duyệt bởi user

## 1. Mục tiêu & phạm vi

Thêm payment gateway **SePay** vào sub2api theo luồng **VietQR + Webhook** (chuyển khoản ngân hàng QR):

- User tạo đơn → hệ thống sinh QR VietQR chuẩn NAPAS (tự build payload EMV, **không gọi API SePay** khi tạo payment) → khách quét QR bằng app ngân hàng, chuyển khoản với nội dung chứa mã đơn → SePay phát hiện giao dịch qua webhook → hệ thống match mã đơn và xác nhận.
- **API v2** (`https://userapi.sepay.vn/v2`) chỉ dùng cho `QueryOrder`/verify (Bearer token, rate limit 3 req/s).
- Hỗ trợ **cả recharge (nạp tiền VND) và subscription** (cần thêm setting quy đổi USD→VND).
- **Refund: không hỗ trợ** — SePay không có API hoàn tiền (tiền về bằng chuyển khoản thủ công).

Ngoài phạm vi: hosted checkout "Cổng thanh toán SePay" (`/v1/checkout/init`, thẻ/QN NAPAS), VA theo đơn hàng (BIDV/Sacombank/VCB), OAuth 2.0 webhook.

Tài liệu tham khảo SePay (URL docs mới — URL cũ `/vi/sepay-api/v2` đã 404):
- Tổng quan API v2: https://developer.sepay.vn/vi/sepay-api/v2/gioi-thieu
- Xác thực: https://developer.sepay.vn/vi/sepay-api/v2/xac-thuc
- Giao dịch: https://developer.sepay.vn/vi/sepay-api/v2/giao-dich/danh-sach
- Webhook payload: https://developer.sepay.vn/vi/sepay-webhooks/tich-hop-webhook
- Webhook security: https://developer.sepay.vn/vi/sepay-webhooks/xac-thuc
- QR & mã thanh toán: https://developer.sepay.vn/vi/sepay-webhooks/tao-qr-va-form-thanh-toan

## 2. Luồng thanh toán

```
User chọn "SePay" → POST /orders → order (out_trade_no: sub2_20260814aB3kX9mQ)
  → Provider.CreatePayment: KHÔNG gọi API SePay — tự sinh chuỗi QR EMV VietQR
    (bin + số TK + amount + nội dung = out_trade_no) → trả field QRCode
  → Frontend render QR bằng lib qrcode hiện có (PaymentQRDialog), poll VerifyOrder
  → Khách quét QR bằng app ngân hàng → chuyển khoản (app tự điền TK/số tiền/nội dung)
  → SePay phát hiện giao dịch → POST /api/v1/payment/webhook/sepay (JSON + HMAC)
  → Match code ↔ out_trade_no → fulfillment xác nhận (validate amount sẵn có)
```

Điểm thiết kế then chốt: **mã chuyển khoản = `out_trade_no`** (format `sub2_` + YYYYMMDD + 8 ký tự alphanumeric random — đã unique). Admin cấu hình "Cấu hình mã thanh toán" trên SePay dashboard (prefix `sub2_`) để SePay trích `code` từ nội dung chuyển khoản. Việc khớp mã chịu được hai dạng `code` (có/không prefix) — xem 3.3.

## 3. Provider `backend/internal/payment/provider/sepay.go`

### 3.1 Config instance

Lưu trong config map đã encrypt của provider instance (pattern easypay):

| Key | Bắt buộc | Mặc định | Ý nghĩa |
|---|---|---|---|
| `apiToken` | ✅ | | Bearer token (64 ký tự) cho API v2 — QueryOrder/verify |
| `apiBase` | | `https://userapi.sepay.vn` | Override cho sandbox `https://userapi-sandbox.sepay.vn` |
| `bankAccountNumber` | ✅ | | Số tài khoản thụ hưởng |
| `bankBin` | ✅ | | Mã ngân hàng 6 số (VCB `970436`, ACB `970416`, Techcombank `970407`...) |
| `accountName` | | | Tên người nhận, hiển thị ở hint UI |
| `webhookSecret` | khuyến nghị | | HMAC-SHA256 secret (`X-SePay-Signature`) |
| `webhookApiKey` | nếu không có secret | | Dùng so `Authorization: Apikey XXX` |
| `currency` | | `VND` | |

### 3.2 CreatePayment

Build payload EMV VietQR (chuẩn EMVCo QRCPS-MPM của NAPAS) bằng TLV builder tự viết (~80 dòng, kèm CRC16-CCITT poly 0x1021 init 0xFFFF):

| Tag | Giá trị |
|---|---|
| 00 | `01` (Point of Initiation) |
| 01 | `12` (dynamic — có amount) |
| 38 | template con: 00=`A000000727` (GUID NAPAS), 01=`bankBin`, 02=`bankAccountNumber` |
| 53 | `704` (VND) |
| 54 | amount — integer VND (zero-decimal) |
| 58 | `VN` |
| 62 | template con: 08 = `out_trade_no` (nội dung chuyển khoản) |
| 63 | CRC16 của chuỗi payload + `6304` |

Trả `CreatePaymentResponse{QRCode: payload, Currency: "VND"}`. Không có `TradeNo` (chưa có giao dịch upstream — giống easypay popup mode). Frontend dùng lại `PaymentQRDialog` render bằng lib `qrcode` — **không** phụ thuộc `vietqr.app`.

### 3.3 VerifyNotification(rawBody, headers)

Xác thực (ưu tiên theo thứ tự):
1. Nếu có `webhookSecret`: kiểm tra `X-SePay-Signature: sha256={hex}` với `hmac_sha256(X-SePay-Timestamp + "." + rawBody, secret)`, so constant-time (`hmac.Equal`), và timestamp lệch ≤ 300 giây (chống replay). Dùng **raw body bytes gốc**, không re-serialize.
2. Nếu không: so `Authorization: Apikey {key}` với `webhookApiKey` constant-time.

Parse JSON payload webhook:
```json
{
  "id": 92704, "gateway": "Vietcombank",
  "transactionDate": "2024-07-02 11:08:33",
  "accountNumber": "1017588888", "subAccount": "",
  "code": "SUB2_20260814AB3KX9MQ", "content": "...",
  "transferType": "in", "transferAmount": 5000000,
  "referenceCode": "FT24012345678", "accumulated": 0
}
```

- `transferType == "out"` → trả `nil, nil` (event không liên quan — handler ack 200).
- `code` null/rỗng → error "missing payment code".
- Match: normalize `code` (uppercase, strip non-alphanumeric) so với `out_trade_no` đã normalize tương tự (bank app có thể uppercase nội dung). Vì SePay trích `code` theo prefix cấu hình trên dashboard (có thể trả về mã **không gồm** prefix `sub2_`), phép khớp thử cả hai dạng: full `out_trade_no` và `out_trade_no` đã bỏ prefix `sub2_`. Chiến lược hai dạng này áp dụng thống nhất cho cả `VerifyNotification`, `extractOutTradeNo` (lookup instance) và `QueryOrder`.
- Map: `OrderID` = out_trade_no khớp, `Amount` = `transferAmount`, `TradeNo` = `referenceCode` (fallback `id`), `Status` = success, `RawData` = rawBody, `Metadata` = {accountNumber, gateway} để đối chiếu instance.

### 3.4 QueryOrder(tradeNo)

`tradeNo` theo convention của hệ thống là out_trade_no. Gọi:
```
GET {apiBase}/v2/transactions?q={out_trade_no}&transfer_type=in
Authorization: Bearer {apiToken}
```
Tìm transaction có `code` khớp (normalize như 3.3) → `QueryOrderResponse{Status: "paid", Amount: amount_in, TradeNo: reference_number, PaidAt: transaction_date}`. Không thấy → `Status: "pending"`. Lỗi HTTP 401/429 → error rõ ràng kèm `Retry-After` nếu có.

### 3.5 Refund / QueryRefund / CancelPayment

Không implement. SePay không có API hoàn tiền. `GetRefundEligibleProviders` phải exclude sepay.

## 4. Tích hợp hệ thống (backend)

- `backend/internal/payment/types.go`: thêm `TypeSePay PaymentType = "sepay"` + case trong `GetBasePaymentType`.
- `backend/internal/payment/provider/factory.go`: `case payment.TypeSePay: return NewSePay(instanceID, config)`.
- `backend/internal/handler/payment_webhook_handler.go`:
  - Route mới `webhook.POST("/sepay", webhookHandler.SepayNotify)` trong `routes/payment.go`.
  - `extractOutTradeNo`: case sepay — parse JSON lấy `code` (để `GetWebhookProviders` lookup đúng instance khi có nhiều tài khoản ngân hàng; không tìm thấy order → fallback thử tất cả instance sepay — HMAC secret riêng từng instance sẽ tự xác định đúng cái nào).
  - `writeSuccessResponse`: case sepay → **JSON `{"success": true}` HTTP 200** — SePay bắt buộc đúng body này mới tính là thành công (khác với text "success" mặc định).
- Registry: `Register` theo `SupportedTypes()` trả về `[]PaymentType{TypeSePay}` — không cần đổi registry.go.

## 5. Subscription USD→VND

Song song với `SUBSCRIPTION_USD_TO_CNY_RATE` hiện có:

- Setting mới `SUBSCRIPTION_USD_TO_VND_RATE` (1 USD = X VND, 0 = tắt).
- `calculateSubscriptionGatewayBaseAmount(amount, usdToCnyRate, currency)` mở rộng nhận thêm `usdToVndRate`: rate > 0 và currency CNY → dùng rate CNY; rate > 0 và currency VND → dùng rate VND, round 0 chữ số thập phân. Currency khác → giữ hành vi price trực tiếp.
- Expose qua: `PaymentConfig` struct, admin settings DTO (`payment_subscription_usd_to_vnd_rate`), checkout-info DTO (frontend hiển thị giá quy đổi như đang làm với CNY).
- Validation: subscription order qua sepay khi rate VND = 0 → chặn lúc create order với lỗi rõ ràng (tránh giá 9.9 VND).

## 6. Frontend

- `frontend/src/components/payment/providerConfig.ts`:
  - `sepay: ['sepay']` trong mapping provider→methods, thêm vào `METHOD_ORDER`, thêm `WEBHOOK_PATHS.sepay = '/api/v1/payment/webhook/sepay'` (dùng cho hint notify URL).
  - Danh sách config fields (mục 3.1) với `sensitive: true` cho `apiToken`, `webhookSecret`, `webhookApiKey`.
- Admin settings: form provider SePay + hint (đặc biệt: hướng dẫn cấu hình prefix mã thanh toán trên SePay dashboard phải khớp `sub2_`).
- User checkout: method card "SePay — Chuyển khoản ngân hàng (VND)"; QR dialog reused; hiển thị thêm hint số TK + tên + "chuyển đúng nội dung và số tiền".
- i18n: `en`, `zh` (các locale repo đang có).

## 7. Error handling & security

- HMAC fail / sai API key → 400 "verify failed" → SePay retry theo schedule của chúng.
- Thiếu config bắt buộc → error khi tạo provider instance (giống easypay `NewEasyPay`).
- So sánh secret luôn constant-time (`hmac.Equal`).
- Amount mismatch (chuyển thiếu/thừa): fulfillment hiện có ghi audit `PAYMENT_AMOUNT_MISMATCH` + từ chối → webhook trả 500 → SePay retry; admin xử lý thủ công (hoàn qua chuyển khoản + hủy/thử lại đơn trong admin UI).
- `maxWebhookBodySize` 1MB áp dụng như các provider khác.
- Rate limit API v2 3 req/s: QueryOrder là single call, không loop — không cần throttle riêng.

## 8. Testing

- `sepay_test.go`:
  - EMV builder: vector chuẩn (payload VietQR công khai + CRC đúng), amount integer VND, nội dung chứa out_trade_no.
  - VerifyNotification: HMAC đúng/sai, replay >300s, thiếu signature, `Authorization: Apikey` đúng/sai, `transferType=out` → nil, `code` null → error, match case-insensitive.
  - QueryOrder: mock HTTP server — tìm thấy (paid), không thấy (pending), 401, 429.
  - Config validation: thiếu từng field bắt buộc.
- `payment_webhook_handler_test.go`: response sepay là JSON `{"success":true}`; `extractOutTradeNo` với body sepay.
- Service tests: USD→VND rate (rate>0 convert + round 0 dp; rate=0 chặn subscription sepay; recharge không bị ảnh hưởng).
- Frontend specs: `providerConfig.spec.ts`, `paymentFlow.spec.ts` cập nhật cho sepay.

## 9. Vấn đề đã cân nhắc và loại bỏ

- **URL ảnh `vietqr.app`** thay vì tự sinh EMV: bị loại — phụ thuộc dịch vụ ngoài cho mọi giao dịch + phải thêm chế độ `<img>` cho frontend + lộ mã đơn/số tiền qua query string bên thứ 3.
- **Framework generic bank-transfer**: over-engineering, YAGNI.
- **Sinh mã thanh toán riêng** (random hex riêng, khác out_trade_no): phức tạp hóa storage/matching mà out_trade_no đã unique + khó đoán (8 ký tự random).
- **Dùng `id` làm TradeNo chính**: `referenceCode` ngân hàng hữu ích hơn cho đối soát; `id` chỉ là fallback.
