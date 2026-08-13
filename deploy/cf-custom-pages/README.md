# Cloudflare Custom Pages

Trang lỗi thay cho trang mặc định của Cloudflare. Mục tiêu: giữ nhận diện của mình,
không để user thấy trang lạ khi origin chết hoặc WAF chặn nhầm.

| File | Dùng ở đâu | Token |
|---|---|---|
| `500s.html` | Custom Pages → **5XX Errors** | `::CLOUDFLARE_ERROR_500S_BOX::` (bắt buộc) |
| `1000s.html` | Custom Pages → **1XXX Errors** | `::CLOUDFLARE_ERROR_1000S_BOX::` (bắt buộc) |
| `waf-block.html` | **WAF custom rule → Block → custom response** | `::RAY_ID::`, `::CLIENT_IP::` (tuỳ chọn) |

> **Trang chặn của WAF không phải lỗi 1XXX.** Token `::CLOUDFLARE_ERROR_1000S_BOX::`
> chỉ được thay ở slot *1XXX Errors*; dán nó vào response của WAF custom rule thì
> nó hiện nguyên chữ trên trang. Ở đó chỉ dùng được token tuỳ chọn: `::RAY_ID::`,
> `::CLIENT_IP::`, `::GEO::`.
>
> Danh sách token chuẩn:
> <https://developers.cloudflare.com/rules/custom-errors/reference/error-tokens/>

Token bắt buộc theo doc: `::CAPTCHA_BOX::` (Interactive / Country / Managed
Challenge, I'm Under Attack), `::IM_UNDER_ATTACK_BOX::` (Non-Interactive
Challenge), `::CLOUDFLARE_ERROR_500S_BOX::` (5XX), `::CLOUDFLARE_ERROR_1000S_BOX::`
(1XXX). Ba token tuỳ chọn `::CLIENT_IP::`, `::RAY_ID::`, `::GEO::` dùng được ở
mọi trang lỗi, custom asset và inline response.

## Giới hạn: không giấu được hoàn toàn

Token bắt buộc phải có, và chính nó render ra khối thông tin của Cloudflare
(Ray ID, tên dịch vụ). Tuỳ biến được: layout, màu, chữ, logo. Không bỏ được:
khối token đó.

Đừng `display:none` khối token. Mất Ray ID thì không tra được request trong
Security Events, hết đường debug khi user báo bị chặn nhầm.

## Host file ở đâu

Cloudflare **tự fetch URL** lúc bấm Publish rồi cache lại. Yêu cầu:

- URL public, HTTPS, trả `200` và `Content-Type: text/html`.
- **Không được nằm sau Cloudflare Access.** Zone này đang bật Access trên
  `api.lamtung.dev` — đặt file ở đó thì Cloudflare fetch về sẽ dính trang login,
  lưu vào là hỏng trang lỗi.
- GitHub `raw.githubusercontent.com` **không dùng được**: trả `text/plain`.

Chọn một trong hai:

1. **Cloudflare Pages** (gọn nhất, free): tạo project mới, upload thư mục này,
   lấy URL `https://<project>.pages.dev/500s.html`.
2. **Subdomain riêng không bật Access**, ví dụ `status.lamtung.dev`, trỏ vào một
   site block riêng trong Caddy chỉ `file_server`.

## Áp dụng

Dashboard → chọn zone → **Rules → Custom Pages** (bản cũ: Error Pages) →
từng loại page → **Custom Pages → URL** → dán link → **Publish**.

Cloudflare fetch ngay lúc publish. Sửa file sau đó thì phải bấm publish lại,
không tự cập nhật.

## Kiểm tra

```bash
# Trang 500: tắt tạm app, gọi qua domain
docker compose stop sub2api
curl -s https://api.lamtung.dev/ | head -20
docker compose start sub2api
```

Trang WAF khó dựng lại thật — cách nhanh là tạm thêm một Custom Rule chặn đúng
IP của mình, thử, rồi xoá rule.

## Lưu ý cho traffic API

Custom Page áp **toàn zone**, không tách được `/v1/*` với `/admin`. Client gọi
`/v1/*` (Codex, Claude Code) sẽ nhận HTML thay vì JSON khi gặp lỗi 5xx ở tầng
Cloudflare — giống hệt trang mặc định, không tệ hơn, nhưng đừng kỳ vọng nó giúp
gì cho client tự động. Giá trị thật chỉ ở phần người dùng mở bằng trình duyệt.
