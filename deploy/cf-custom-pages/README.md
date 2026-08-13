# Cloudflare Custom Pages

Trang lỗi thay cho trang mặc định của Cloudflare. Mục tiêu: giữ nhận diện của mình,
không để user thấy trang lạ khi origin chết hoặc WAF chặn nhầm.

| File | Loại page trong dashboard | Token bắt buộc |
|---|---|---|
| `500s.html` | 500 Class Errors | `::CLOUDFLARE_ERROR_500S_BOX::` |
| `waf-block.html` | WAF Block / 1000 Class Errors | `::CLOUDFLARE_ERROR_1000S_BOX::` |

> Dashboard hiện đúng token yêu cầu ngay khi chọn loại page, và **từ chối lưu nếu
> thiếu token**. Nếu tên token khác bảng trên, lấy theo dashboard — đừng sửa mò.

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
