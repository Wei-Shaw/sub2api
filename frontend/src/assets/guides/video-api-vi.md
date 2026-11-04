# Tài liệu tích hợp API tạo video (Grok)

Tài liệu dành cho khách hàng gọi API video qua Sub2API. Hợp đồng dưới đây mô tả API gateway hiện có trong mã nguồn Sub2API; khả năng sử dụng thực tế phụ thuộc vào domain của đơn vị vận hành, API key, group được gán và model được cho phép.

## 1. Luồng tổng quan

1. Tạo một video bằng `POST /v1/videos/generations` (hoặc alias `POST /v1/videos`).
2. Lưu `request_id` trả về.
3. Gọi `GET /v1/videos/generations/{request_id}` để kiểm tra trạng thái, lặp lại cách quãng khi tác vụ còn chạy.
4. Khi tác vụ hoàn tất, tải video qua `GET /v1/videos/generations/{request_id}/content`.

API key phải được cấp quyền truy cập model video Grok và được gắn với group có tài khoản Grok phù hợp. Dùng đúng Base URL do nhà cung cấp dịch vụ gửi; không suy ra domain từ ví dụ. Trong dashboard, vào **Khóa API → Sử dụng khóa → API → Xem mẫu** để kiểm tra khả năng của key; kết quả không bảo đảm mọi request đều thành công.

## 2. Xác thực và header

Các endpoint cần API key Sub2API trong header Bearer:

```http
Authorization: Bearer [REDACTED]
Content-Type: application/json
Accept: application/json
```

Không nhúng API key trong mã nguồn công khai, trình duyệt hay log. Nếu key bị lộ, thu hồi và tạo key mới.

## 3. Tạo video

`POST {BASE_URL}/v1/videos/generations`

Alias tương thích OpenAI: `POST {BASE_URL}/v1/videos`.

Header: `Authorization`, `Content-Type: application/json`, `Accept: application/json`.

Ví dụ text-to-video dưới đây chỉ dùng khi nhà cung cấp xác nhận `grok-imagine-video` được cấp cho key. Thay giá trị bằng thông tin được cấp; `BASE_URL` là domain gốc, không có `/v1` ở cuối:

```bash
export BASE_URL="https://api.example.com"
read -s -p "Sub2API API key: " SUB2API_API_KEY
printf "\n"
export SUB2API_API_KEY
```

```bash
curl --fail-with-body -X POST "$BASE_URL/v1/videos/generations" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "model": "grok-imagine-video",
    "prompt": "A cinematic sunrise over the mountains, slow camera movement",
    "duration": 8,
    "aspect_ratio": "16:9",
    "resolution": "720p"
  }'
```

Phản hồi tạo tác vụ là JSON bất đồng bộ; ví dụ minh họa, không phải kết quả gọi API thực tế:

```json
{
  "request_id": "<request-id>"
}
```

Giữ nguyên `request_id` cho cả bước xem trạng thái và tải nội dung. Gateway chuyển tiếp schema phản hồi từ upstream; tùy relay, tên trường có thể khác, cần kiểm tra phản hồi thực tế và hướng dẫn nhà cung cấp. Ví dụ sau giả định phản hồi có `request_id`:

```bash
export REQUEST_ID="<request-id-tu-phan-hoi>"
```

### Các trường request

| Trường | Kiểu | Mô tả |
| --- | --- | --- |
| `model` | string, bắt buộc | Model video được phép cho API key/group. Ví dụ: `grok-imagine-video`. Alias model phải được tài khoản/group cấu hình cho phép. |
| `prompt` | string | Mô tả nội dung video. |
| `duration` | number | Thời lượng mong muốn, tính bằng giây. Quy tắc chuẩn hóa **tính phí** được mô tả riêng bên dưới; không phải cam kết upstream chấp nhận mọi giá trị. |
| `aspect_ratio` | string | Tỷ lệ khung hình mong muốn, ví dụ `16:9`. Hỗ trợ cụ thể phụ thuộc model/upstream. |
| `resolution` | string | Độ phân giải mong muốn. Các mức tính phí gồm `480p`, `720p`, `1080p`; việc model thực sự hỗ trợ mức nào phải xác nhận với nhà cung cấp. |
| `image`, `images`, `reference_images` | tùy upstream | Gateway nhận diện tham chiếu ảnh; cấu trúc và khả năng hỗ trợ cụ thể phải xác nhận với nhà cung cấp. |

Gateway chuyển tiếp request tới upstream, không xác thực đầy đủ schema video. Model `grok-imagine-video-1.5` trong mã nguồn được ghi nhận là **image-to-video**, cần ảnh đầu vào; không dùng request chỉ có prompt với model này. Hãy yêu cầu nhà cung cấp xác nhận cấu trúc ảnh và model được cấp trước khi tích hợp. Trạng thái video có thể trả metadata hoặc trường bổ sung từ upstream.

## 4. Kiểm tra trạng thái

`GET {BASE_URL}/v1/videos/generations/{request_id}`

```bash
curl --fail-with-body "$BASE_URL/v1/videos/generations/$REQUEST_ID" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Accept: application/json"
```

Ví dụ trạng thái hoàn tất theo định dạng xAI:

```json
{
  "status": "done",
  "model": "grok-imagine-video",
  "video": {
    "url": "/v1/videos/<request-id>/content",
    "duration": 8,
    "respect_moderation": true
  }
}
```

Đây là ví dụ minh họa; gateway không chuẩn hóa status và có thể viết lại `video.url` thành đường dẫn content tương đối. Phía tính phí nhận diện `done`, `completed`, `succeeded` khi có URL video. Các trạng thái đang xử lý có thể gồm `pending`, `queued`, `running`; `failed`, `expired`, `cancelled` không được xem là hoàn tất. Danh sách và chi tiết lỗi phụ thuộc upstream; không giả định mọi upstream trả tất cả trạng thái trên.

Polling nên thực hiện cách quãng (ví dụ vài giây), có backoff và dừng khi tác vụ thành công hoặc thất bại. Không gửi liên tục ở tần suất cao.

## 5. Tải file video

`GET {BASE_URL}/v1/videos/generations/{request_id}/content`

```bash
curl --fail --location \
  "$BASE_URL/v1/videos/generations/$REQUEST_ID/content" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  --output video.mp4
```

Endpoint trả nội dung nhị phân video (không phải JSON); lưu body thành file và kiểm tra `Content-Type` thực tế, không mặc định mọi file đều là MP4. Gateway proxy nội dung qua tài khoản đã tạo tác vụ, yêu cầu đúng người dùng, API key và group khi tạo. Có thể hỗ trợ header `Range` khi tải. Chỉ gọi sau khi status cho biết video đã sẵn sàng; URL/task có thể hết hạn hoặc không còn khả dụng.

## 6. Model, quyền truy cập và giới hạn

- Các family trong mã nguồn gồm `grok-imagine-video` và `grok-imagine-video-1.5` (cần ảnh đầu vào). Tên được chấp nhận cuối cùng phụ thuộc model mapping/allowlist tại group và tài khoản đang hoạt động.
- Việc có endpoint không đồng nghĩa mọi key đều có quyền: group phải cho phép tính năng tạo media, model phải qua allowlist và phải có tài khoản Grok đủ điều kiện.
- Sub2API cũng đăng ký route sửa video (`POST /v1/videos/edits`) và nối dài video (`POST /v1/videos/extensions`) cho group Grok. Tài liệu này không đặc tả payload của hai tính năng đó vì schema phụ thuộc upstream; không gửi payload ví dụ mục tạo mới sang các endpoint này.
- Không có thông tin base URL, quota, giới hạn RPM, thời gian lưu tác vụ hay bảng giá cụ thể trong hợp đồng mã nguồn chung. Các giá trị này do nhà vận hành cấp cho khách hàng.

## 7. Tính phí

Video là tác vụ bất đồng bộ: phí usage được ghi nhận khi trạng thái hoàn tất có URL video, qua kiểm tra status hoặc khi tải content; cùng một task được chống ghi nhận trùng. Đơn giá có thể cấu hình theo model family và độ phân giải ở cấp group; hệ thống cũng áp dụng hệ số video/rate của group. Giá tiền thực tế, đơn vị tiền, thuế và chính sách hoàn phí cần đối chiếu bảng giá của nhà cung cấp, không suy ra từ tài liệu tích hợp này.

Thời lượng tính phí được chuẩn hóa 1–15 giây, mặc định 8 giây khi không cung cấp thời lượng hợp lệ. Resolution được tính theo `480p`, `720p`, `1080p`; mặc định runtime là `480p` nếu không xác định được. Hãy khai báo duration/resolution rõ ràng để dự toán chính xác.

## 8. Lỗi và khắc phục

Một số lỗi gateway dùng envelope tương thích OpenAI:

```json
{
  "error": {
    "type": "invalid_request_error",
    "message": "..."
  }
}
```

Lỗi middleware xác thực cũng có thể trả dạng `{"code":"...","message":"..."}`; lỗi upstream có thể khác. Không nên giả định mọi lỗi có cùng cấu trúc.

| HTTP | Ý nghĩa thường gặp | Hướng xử lý |
| --- | --- | --- |
| `400` | Thiếu `model`, body rỗng/không đọc được hoặc `request_id` thiếu | Kiểm tra JSON, model và đường dẫn. |
| `401` | API key thiếu/không hợp lệ | Kiểm tra Bearer token, trạng thái và hạn key. |
| `403` | Group/key không được phép tạo media hoặc upstream từ chối policy | Liên hệ nhà cung cấp để kiểm tra quyền/model/nội dung. |
| `404` | Route không bật cho platform, request không tồn tại hoặc key không sở hữu task | Xác nhận endpoint, group và `request_id`; dùng đúng key tạo tác vụ. |
| `413` | Request body vượt giới hạn gateway | Giảm kích thước request; hỏi nhà vận hành giới hạn tối đa. |
| `429` | Rate limit/billing/quota hoặc upstream limit | Tôn trọng `Retry-After` nếu có, chờ rồi thử lại có backoff. |
| `502`/`5xx` | Lỗi upstream/gateway | Lưu request ID/log correlation nếu có, thử lại thận trọng hoặc liên hệ hỗ trợ. |

Không tự động gửi lại yêu cầu tạo mới chỉ vì client timeout: tác vụ có thể đã được tạo và retry có thể sinh thêm video, phát sinh thêm phí. Hãy liên hệ hỗ trợ nếu không nhận được `request_id`.

## 9. Quy trình tích hợp khuyến nghị

- Đọc Base URL, API key, model/allowlist, mức giá và quota từ thông tin cấp riêng bởi nhà cung cấp.
- Tạo tác vụ và lưu `request_id` cùng trạng thái nghiệp vụ phía client.
- Poll trạng thái có backoff; dừng nếu `failed`/`expired`/`cancelled`.
- Khi hoàn tất, tải content bằng cùng key, kiểm tra HTTP status và lưu file an toàn.
- Không log token hoặc URL signed; giới hạn retry cho thao tác tạo để tránh nhân đôi tác vụ.

## 10. API Seedance/Ark — giao thức riêng

Nếu nhà cung cấp cấp model Seedance/Ark, không dùng request Grok ở trên. Đây là giao thức native Ark, payload và phản hồi khác Grok:

- Tạo: `POST /api/v3/contents/generations/tasks`.
- Trạng thái: `GET /api/v3/contents/generations/tasks/{id}`.
- Xóa: `DELETE /api/v3/contents/generations/tasks/{id}`.
- Gửi cùng header Bearer của API key Sub2API và dùng đúng user, key, group đã tạo task. Model và quyền phải được cấp riêng.

Ví dụ tạo task (model chỉ là alias minh họa, cần được cấu hình):

```bash
curl --fail-with-body "$BASE_URL/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-video",
    "content": [{"type":"text", "text":"Sóng biển nhẹ nhàng vỗ vào bờ"}],
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9",
    "generate_audio": true
  }'
```

Lấy `id` từ phản hồi gốc, rồi kiểm tra trạng thái bằng cùng key:

```bash
export TASK_ID="<id-tu-phan-hoi>"
curl --fail-with-body "$BASE_URL/api/v3/contents/generations/tasks/$TASK_ID" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

Trạng thái và URL kết quả theo phản hồi native upstream; ví dụ trạng thái cuối gồm `succeeded`, `failed`, `cancelled`. Khi cần xóa task, dùng `curl -X DELETE` với đường dẫn trạng thái và cùng header xác thực; xóa không đồng nghĩa được hoàn tiền. Lần đầu truy vấn thấy `succeeded`, hệ thống tính phí theo `usage.completion_tokens`, không dùng biểu giá video theo giây của Grok. Hệ thống không tự polling nền; thời hạn Redis mặc định 24 giờ không phải cam kết lưu file. Liên hệ nhà cung cấp nếu cần chi tiết cấu hình Seedance.

## 11. Tham chiếu triển khai

Đường dẫn xác thực là API key middleware của gateway `/v1`; API key phải thuộc đúng user khi truy vấn tác vụ. Tài liệu được đối chiếu với route gateway, handler/service Grok media, capability theo API key, ví dụ tích hợp trong UI và logic billing của Sub2API. API có thể được cấu hình khác nhau theo từng deployment.
