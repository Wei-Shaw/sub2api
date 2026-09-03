# Sub2API — Audit bảo mật & Plan xử lý

> Ngày rà soát: 2026-09-03 · Nhánh `main` @ `485af88ac` · Phạm vi: 968 file Go + 765 file frontend
> Phương pháp: 6 agent rà song song theo 6 vùng độc lập. Mã **không** bị sửa trong quá trình rà soát.
> Tài liệu này tự chứa — đủ để một session mới tiếp tục công việc mà không cần context cũ.

Tổng: **34 phát hiện** — 4 Critical · 7 High · 19 Medium · 4 Low.

| Vùng khảo sát | Crit | High | Med | Low |
|---|---:|---:|---:|---:|
| Dữ liệu & thanh toán | 2 | 3 | 11 | 2 |
| Gateway & proxy upstream | 1 | 1 | 2 | 1 |
| Đồng thời & ổn định Go | 1 | 0 | 2 | 0 |
| Frontend Vue | 0 | 2 | 3 | 0 |
| Crypto, secret & triển khai | 0 | 1 | 0 | 1 |
| Xác thực & phân quyền | 0 | 0 | 1 | 0 |

Nhãn **[đã xác minh]** = đã đọc lại trực tiếp trên mã nguồn sau khi agent báo cáo. Phần còn lại giữ nguyên bằng chứng `file:line` từ agent, chưa tự kiểm chứng.

Nhãn **[ĐÃ SỬA]** = đã vá trên nhánh `fix/security-audit-batch-1`, có test chặn tái phát, đã chạy gate.

**Đợt 1 — 7 phát hiện:** C1 · C2 · C3 · H4 · H7 · M17 · M18.

**Đợt 2 — 19 phát hiện:** C4 · H1 · H2 · H3 · H5 · H6 · M1 · M2 · M4 · M5 · M6 · M7 · M9 · M10 · M12 · M14 · M16 · L1 · L4. Trong đó M8 và M13 mới sửa được một phần, M3 hoá ra đã được sửa từ trước, M11 cố tình không sửa.

**Còn lại:** M15 (bản rà soát có liệt kê nhưng **không xếp vào PR nào**) · M19 và L3 (quyết định kiến trúc riêng) · L2 (design choice, không sửa) · PR 6 release 2 (drop cột `key`).

**Chưa chạy được — quan trọng:** máy này **không có Docker**, nên toàn bộ integration test viết trong hai đợt chỉ compile chứ chưa từng chạy. Đáng lo nhất: **migration 238 và 239 chưa từng chạy trên PostgreSQL thật** — khối DO backfill của 238, biểu thức `sha256(convert_to(...))` của 239, partial unique index, và khối DO mới thêm vào 027. Cần một lượt chạy trên PG 16 với dữ liệu thực trước khi merge, đặc biệt là 238 trên những đơn có audit `REFUND_ROLLBACK_FAILED` thật.

Trong lúc sửa, agent tìm thêm **3 lỗi không có trong bản rà soát này** (Phần I-B), và **5 chỗ bản rà soát nói sai hoặc nói thiếu** — đính chính nằm ngay dưới phát hiện tương ứng. Đọc các đính chính trước khi tin số dòng `file:line` còn lại trong tài liệu: chúng đã trôi ở nhiều chỗ.

Gate sau đợt 1: `go build ./...` sạch · `go test -tags=unit ./...` đúng 5 lỗi Windows-local đã biết, không tăng · `vue-tsc --noEmit` exit 0 · `vitest run` 253 file / 1861 test pass.

---

# Phần I — Phát hiện

## Critical

### C1. Ba file migration tự hoàn tác ngay khi vừa chạy **[đã xác minh]** **[ĐÃ SỬA]**

`backend/internal/repository/migrations_runner.go:263` · `migrations/037_ops_alert_silences.sql:27` · `019_migrate_wechat_to_attributes.sql:73` · `024_add_gemini_tier_id.sql:25`

Runner **không có parser goose** — `grep goose backend/internal/repository/migrations_runner.go` trả về rỗng — mà đẩy nguyên nội dung file vào `tx.ExecContext(ctx, content)`. Với PostgreSQL, `-- +goose Down` chỉ là một dòng comment, nên khối Down chạy tiếp ngay sau khối Up trong cùng transaction.

Đúng 3 file trong `migrations/` chứa `+goose Down` (kiểm bằng `grep -rl '+goose Down' backend/migrations/`).

Hậu quả từng file:

- **037** tạo bảng `ops_alert_silences` (dòng 5) rồi `DROP TABLE IF EXISTS` (dòng 27). Không migration nào và không schema Ent nào tạo lại — `grep -rln 'ops_alert_silences' backend/migrations backend/ent/schema` chỉ ra đúng file này. Trong khi đó `backend/internal/repository/ops_repo_alerts.go:650` (INSERT) và `:727` (SELECT) vẫn truy vấn bảng lúc chạy. Tính năng tắt cảnh báo hỏng vĩnh viễn trên **mọi bản cài**.
- **019** khôi phục cột `users.wechat`, chép attribute value ngược về cột đó, xoá `user_attribute_values`, rồi soft-delete definition.
- **024** chạy `UPDATE accounts SET credentials = credentials - 'tier_id'`, gỡ đúng thứ khối Up vừa đặt.

**Mức độ khôi phục** (đã đọc kỹ ba khối Down, nhẹ hơn báo cáo ban đầu của agent):

- 037 — bảng chưa từng có dữ liệu, chỉ cần tạo lại.
- 019 — Down chép dữ liệu **ngược về** `users.wechat` trước khi xoá. Dữ liệu vẫn nằm trong cột đó, khôi phục được.
- 024 — agent báo "gỡ tier_id khỏi cả tài khoản đã có tier thật". Không đúng trong thực tế: migration chạy tuần tự lúc boot **trước khi** phục vụ request, nên tại thời điểm 024 chạy chưa tài khoản nào có `tier_id` ngoài những cái Up vừa đặt. Thiệt hại thật = default `LEGACY` không bao giờ được áp. Khôi phục hoàn toàn bằng cách chạy lại Up.

Cả ba đều fix-forward được, không cần operator nhập tay.

**Đính chính khi sửa.**

- Số hiệu 221/222/223 **đã có người dùng** (`221_group_model_pricing`, `222_group_usage_daily_rollups`, `223_group_usage_rollup_timezone`); cao nhất là 234. Ba migration sửa chữa thực tế là **235/236/237**.
- `backend/migrations/README.md:47` **đã cấm sẵn** điều này từ trước ("The runner does not parse goose Up/Down sections"). Luật có sẵn, không ai canh — giờ `no_goose_directives_test.go` canh.
- Chữ ký helper ngược với mô tả ở Phần III: `newMigrationChecksumCompatibilityRule(fileChecksum, ...acceptedDBChecksums)` — **đối số đầu là checksum file MỚI**, phần đuôi mới là checksum cũ trong DB.
- 236 phải chép `users.wechat` qua `DO $repair$` + `EXECUTE`: trên bản cài đã vá, cột đó không còn và tham chiếu tĩnh sẽ không parse được. Không `DROP COLUMN users.wechat` — không mã Go nào đọc cột đó, drop là rủi ro thừa.
- Đã gỡ luôn `+goose Up` / `StatementBegin` / `StatementEnd` khỏi ba file — không migration nào khác dùng chúng.

### C2. SSRF ở đường passthrough Antigravity, trả nguyên body upstream cho client **[đã xác minh]** **[ĐÃ SỬA]**

`backend/internal/service/antigravity_gateway_upstream.go:28-46`, `:99-101`, `:136-138`

`ForwardUpstream` dựng `upstreamURL = baseURL + "/v1/messages"` thẳng từ `account.GetCredential("base_url")`. Trong toàn bộ file **không có một lần gọi `validateUpstreamBaseURL` nào** (`grep -n 'validateUpstreamBaseURL' <file>` rỗng), dù mọi platform anh em đều lọc:

- Anthropic — `gateway_upstream_request.go:33`
- OpenAI — `openai_gateway_request_body.go:68`
- Grok — `grok_upstream_url.go`
- Gemini — `gemini_messages_compat_service.go:467`

Vì bỏ qua hàm lọc, đường này **không chịu ảnh hưởng của `security.url_allowlist.enabled=true`** ngay cả khi operator bật. Trỏ `base_url` vào `http://169.254.169.254` hoặc một service nội bộ, rồi gọi `/antigravity/v1/messages`: body phản hồi được ghi nguyên văn về phía người gọi ở cả nhánh lỗi (`c.Writer.Write(respBody)`) lẫn nhánh thành công, nên credential IAM chảy thẳng ra API key bên ngoài.

### C3. Frame Bedrock độc hại làm panic goroutine không có recover, sập cả tiến trình **[đã xác minh]** **[ĐÃ SỬA]**

`backend/internal/service/bedrock_stream.go:290-291` (slice) · `:266` (đọc length) · `:74` (goroutine)

```go
headers := data[:headersLength]
payload := data[headersLength : len(data)-4]
```

`headersLength` đọc thẳng từ dây (`bedrockReadUint32(prelude[4:8])`, dòng 266). Kiểm tra duy nhất là `totalLength < 16` (dòng 268); **không có ràng buộc nào** giữa `headersLength` và độ dài thực của `data`. CRC32 chỉ phát hiện hỏng đường truyền, nên một frame tự tính CRC hợp lệ với trường length thù địch vẫn qua được cả hai lớp kiểm tra.

Panic `slice bounds out of range` xảy ra bên trong `go func()` ở dòng 74, không có `defer recover()` ở đâu trong file. Middleware `Recovery()` của gin chỉ bọc goroutine xử lý request, **không bắt được goroutine con** — nên panic không được xử lý và giết cả tiến trình, rớt mọi request đang bay chứ không riêng request Bedrock.

**Đính chính khi sửa.** Ràng buộc đề xuất ở Phần III (`int(headersLength)+4 > len(data)`) **không dùng được**: đó chính là dạng tràn trên 32-bit, và nó kiểm sau khi đã cấp phát. Ràng buộc thật đặt ở tầng prelude, so sánh trong `uint64`: `headersLength <= totalLength - 16` (bao gồm dấu bằng — payload rỗng là frame hợp lệ).

Ngoài ra bản rà soát **bỏ sót một vector thứ hai** trong cùng hàm: `total_length` cũng không có trần, nên `make([]byte, totalLength-12)` cho phép cấp phát ~4 GB từ một frame — độc lập với lỗi panic. Đã chặn bằng trần 16 MiB (đúng giá trị AWS SDK dùng).

### C4. Secret của cổng thanh toán lưu plaintext — regression thầm lặng **[đã xác minh]** **[ĐÃ SỬA]**

`backend/internal/service/payment_config_providers.go:534-540` (ghi) · `:501-518` (đọc) · `backend/ent/schema/payment_provider_instance.go:20`

```go
// encryptConfig serialises a provider config for storage.
// New records are written as plaintext JSON; the historical AES-GCM wrapping
// has been dropped but decryptConfig still accepts old ciphertext during migration.
func (s *PaymentConfigService) encryptConfig(cfg map[string]string) (string, error) {
	data, err := json.Marshal(cfg)
	...
}
```

Lớp AES-256-GCM trong `internal/payment/crypto.go` chỉ còn sống như nhánh fallback đọc dữ liệu cũ. Comment schema vẫn ghi `config 字段存储加密后的密钥信息` (đã mã hoá) — nên đây là **regression**, không phải quyết định thiết kế có công bố.

Một dòng DB rò ra (backup, replica, log dump) là lộ: Stripe `secretKey` + `webhookSecret`, EasyPay `pkey`, wxpay `apiV3Key` + `privateKey`, Alipay `privateKey`, Airwallex `apiKey` + `webhookSecret`. Kẻ tấn công ký được callback thành công hợp lệ cho đơn bất kỳ và tự cộng số dư, vô hiệu hoá toàn bộ lớp verify chữ ký vốn được viết rất chuẩn.

---

## High

### H1. Trần hoa hồng giới thiệu bị vượt gấp đôi khi chạy song song **[ĐÃ SỬA]**

`backend/internal/service/affiliate_service.go:361` · `backend/internal/repository/affiliate_repo.go:117`

`GetAccruedRebateFromInvitee` là SELECT trần không khoá dòng; `AccrueQuota` chạy `UPDATE user_affiliates SET aff_frozen_quota = aff_frozen_quota + $1` không có predicate kiểm trần. Hai lần fulfill thanh toán cho cùng một invitee chạy đồng thời (mỗi lần có audit claim riêng nên cả hai đều đi tiếp) cùng đọc `existing=0` và cùng cộng đủ mức hoa hồng. Lặp lại tuỳ ý bằng nạp tiền song song.

### H2. Thử lại hoàn tiền trừ số dư người dùng lần thứ hai **[ĐÃ SỬA]**

`backend/internal/service/payment_refund.go:701` · `:603` · `:213`

`RollbackRefund` thất bại → `handleGwFail` để đơn ở `REFUND_FAILED` trong lúc số dư **đã bị trừ**. Danh sách trạng thái hợp lệ của `PrepareRefund` (`:213`) lại bao gồm `OrderStatusRefundFailed`, và `prepDeduct` không tra cứu lần trừ trước. Admin bấm hoàn tiền lại từ UI → trừ đủ số tiền hoàn lần nữa.

### H3. Đổi mã quà tặng ghi đè mất hạn subscription đã gia hạn **[ĐÃ SỬA]**

`backend/internal/service/redeem_service.go:667`, `:691`, `:703`

`reduceOrCancelSubscription` là đường mutate subscription **duy nhất** đọc qua `GetByUserIDAndGroupID` thay vì `GetByIDForUpdate`, rồi read-modify-write `ExpiresAt` (`:691`) và ghi đè trọn `Notes` (`:703`).

Kịch bản: T1 đổi mã `validity_days` âm, đọc `expires_at = D`. T2 đổi mã +30 ngày, lấy khoá `FOR UPDATE` mà T1 không bao giờ xin, ghi `D+30`, commit. T1 ghi đè `D-7` — xoá sạch 30 ngày đã trả tiền và clobber note của T2.

### H4. Đọc body upstream Antigravity không giới hạn kích thước **[ĐÃ SỬA]**

`backend/internal/service/antigravity_gateway_upstream.go:127`

`io.ReadAll(resp.Body)` trần, không `io.LimitReader`, trong khi OpenAI/Grok/Claude đều đi qua `ReadUpstreamResponseBody` có chặn theo `cfg.Gateway.*MaxBytes` (xem `internal/service/grok_media.go:706-708`). Ghép với C2, kẻ tấn công chọn được cả đích lẫn kích thước phản hồi → OOM gateway.

**Đính chính khi sửa — phát hiện này đúng một nửa.** Đường lỗi đã đi qua `readUpstreamErrorBody`, vốn **đã có** `io.LimitReader` (`antigravity_gateway_service.go:142`). H4 chỉ áp dụng cho nhánh thành công non-stream. Khuyết tật thật của nhánh lỗi là echo nguyên body — thuộc C2, đã vá bằng `writeMappedClaudeError`.

Ba điểm ghi nhận khi vá C2: (a) `grok_upstream_url.go` **không tồn tại**, Grok lọc ở `openai_gateway_request_body.go:20`; (b) các sibling `if s.cfg != nil` rồi vẫn deref `s.cfg` vô điều kiện trong nhánh allowlist — nil cfg là panic, **lỗi tiềm ẩn riêng, chưa sửa**; (c) nhánh lỗi nay trả `(nil, err)` thay vì `(&ForwardResult{}, nil)`, nên 4xx/5xx không còn ghi bản ghi billing zero-usage — giống mọi đường antigravity khác, nhưng là thay đổi hành vi.

### H5. `TOTP_ENCRYPTION_KEY` là master key trá hình và tự sinh lại mỗi lần khởi động **[ĐÃ SỬA]**

`backend/internal/config/config.go:1925-1937` · `backend/internal/repository/aes_encryptor.go:22-33` · `backend/internal/repository/wire.go:143`

Dù mang tên TOTP, đây là khoá **duy nhất** chống lưng cho `SecretEncryptor` dùng chung toàn ứng dụng. Call site xác nhận trong `internal/service/{backup_service,channel_monitor_service,image_storage_settings,ollama_cloud_usage,plugin_manager,totp_service}.go`: S3 secret key của backup, API key upstream của channel monitor, S3 image storage, cookie Ollama Cloud, plugin config + UI session token.

`deploy/.env.example` và `docker-compose.yml` để trống mặc định. Khi trống, config **tự sinh khoá ngẫu nhiên mới ở mỗi lần khởi tiến trình** → operator không bật 2FA sẽ mất khả năng giải mã toàn bộ secret đã lưu sau mỗi lần restart container.

### H6. API admin gửi nguyên API key của người dùng xuống trình duyệt, che bằng CSS **[ĐÃ SỬA]**

`frontend/src/components/admin/user/UserApiKeysModal.vue:17`

```
{{ key.key.substring(0, 20) }}...{{ key.key.substring(key.key.length - 8) }}
```

Che mắt ở tầng trình bày; key đầy đủ nằm trong response mạng và state component. Ghép với M1 (key lưu plaintext ở DB) thành một đường rò trọn vẹn: cột Postgres → API admin → DOM, không chỗ nào rút gọn thật.

### H7. Trang tuỳ biến nới allowlist DOMPurify cho thẻ iframe **[ĐÃ SỬA]**

`frontend/src/views/user/CustomPageView.vue:249-252`

Riêng file này thêm `ADD_TAGS: ['iframe']` và `ADD_ATTR: ['allowfullscreen', 'frameborder', 'src']`. Mọi chỗ `v-html` khác — AnnouncementBell, AnnouncementPopup, ModelPlazaContent, AdminComplianceDialog, LegalDocumentView — đều dùng config DOMPurify mặc định. Người soạn được custom page nhúng iframe tuỳ ý (kể cả `data:` URI) vào trang mọi user đã đăng nhập đều thấy → clickjacking/phishing trong app shell.

---

**Ghi chú khi sửa.** Đã theo phương án allowlist host + `sandbox` (không cắt hẳn iframe). Instance DOMPurify riêng để hook không rò sang các chỗ `v-html` khác; `src` bắt buộc https tuyệt đối; thuộc tính bị rút về keep-set **trước** khi đặt `sandbox` nên tác giả không tự khai `sandbox` được; không có `allow-same-origin` (đi kèm `allow-scripts` thì trang bị khung tự xóa được sandbox của chính nó).

Danh sách host hiện **hardcode** — không tìm thấy ghi chép nào trong repo về việc trang tùy biến thực tế đang nhúng gì, nên danh sách đó là phỏng đoán và cần đối chiếu với một bản triển khai thật. Đường đọc `custom_page_iframe_hosts` đã nối sẵn; để operator cấu hình được còn thiếu 3 chỗ: setting backend, `PublicSettings` trong `frontend/src/types/index.ts`, và ô nhập trong admin UI.

## Medium

### M1. API key lưu plaintext trong Postgres **[ĐÃ SỬA]**
`backend/ent/schema/api_key.go:37-40` · `backend/internal/repository/api_key_repo.go:111-167`
Cột `key` là `Unique()` không băm; `GetByKeyForAuth`/`GetByKey` tra bằng `apikey.KeyEQ(key)` trên chính cột đó. Backup rò = ra thẳng loạt `sk-...` dùng được ngay. Fix: cột lookup `sha256(key)` hoặc HMAC, hiện raw key đúng một lần lúc tạo.

### M2. Chặn SSRF tắt mặc định trên toàn bộ đường upstream còn lại **[ĐÃ SỬA]**
`backend/internal/util/urlvalidator/validator.go:72-102` · `config.go:1953-1955` · `backend/internal/repository/http_upstream.go:580-605`
Mặc định `security.url_allowlist.enabled=false` (xác nhận ở `config_test.go:810-817`) khiến `validateUpstreamBaseURL` rơi về `ValidateURLFormat` — chỉ kiểm scheme + parse, không chặn `localhost`, RFC1918, link-local, `169.254.169.254`. Cùng cờ đó tắt luôn `shouldValidateResolvedIP()` → mất lớp chống DNS rebinding/redirect. Admin-gated, nhưng ở SaaS multi-tenant admin tenant ≠ chủ hạ tầng.

**Đính chính khi sửa — bật cờ lên là sai, và cũng không vá được lỗi.** Đổi `url_allowlist.enabled` thành `true` sẽ phá hỏng: 11 call site truyền `RequireAllowlist`, bật lên là ép HTTPS bất kể `allow_insecure_http` và bắt khai báo từng host, mọi bản tự triển khai dùng relay riêng chết ngay. Và nó **vẫn không chặn được** `169.254.169.254`, vì `allow_private_hosts` mặc định `true`.

Đã tách theo đúng hai trục vốn có sẵn trong mã: `enabled` giữ `false` (whitelist host, tùy chọn), `allow_private_hosts` đổi `true` → **`false`** và không còn lệ thuộc cờ kia. `shouldValidateResolvedIP` bỏ điều kiện `Enabled` — chính chỗ nối này làm lớp chống DNS rebinding không bao giờ chạy ở cấu hình mặc định.

`deploy/config.example.yaml` trước đó ghi sẵn `allow_private_hosts: true`, ai chép mẫu là mất trắng mặc định mới — đã sửa. Hai đường còn lại (`proxy_probe_service.go`, `crs_sync_service.go`) cũng đã gỡ phụ thuộc tương tự.

### M3. Video Grok đọc chéo tenant vì không kiểm quyền sở hữu `request_id` **[VỐN ĐÃ ĐƯỢC SỬA TỪ TRƯỚC]**
`backend/internal/handler/grok_media.go:45-52` · `backend/internal/service/grok_media.go:607-757`, `:759-850`
`GrokVideoStatus`/`GrokVideoContent` chuyển `request_id` từ client thẳng lên upstream, không ràng buộc id với API key/group tạo ra nó. Account↔Group many-to-many (`account_group.go:5-12`) nên ở pool mode một account Grok phục vụ nhiều tenant. Fix: lưu `request_id -> (account_id, group_id, api_key_id)`, verify trước khi proxy.

**Ghi chú — bản rà soát đọc theo mã đã cũ.** Commit `c831bb979` (tổ tiên của `485af88ac`) đã vá đúng điều này: `GrokMediaVideoRequestSessionHash(requestID, userID, apiKeyID)` ràng buộc quyền sở hữu trước mọi lượt gọi upstream, group làm namespace, và `handler/grok_media.go:263` từ chối account không khớp binding. Không sửa gì.

Đáng chú ý: giải pháp đề xuất trong bản rà soát (bảng `request_id -> (account_id, group_id, api_key_id)`) **tệ hơn** cái đang có: binding hiện nằm trong Redis có TTL và tự hết hạn, còn bảng DB sẽ tự sinh ra đúng vấn đề phình không giới hạn mà chính bản rà soát cảnh báo.

### M4. Khoản giữ của batch image có thể được hoàn hai lần **[ĐÃ SỬA]**
`backend/internal/repository/usage_billing_repo.go:294` · `:340` · `backend/internal/service/batch_image_settlement.go:223`
`captureUsageBillingBatchImageBalance` không có guard `batchImageHoldClaimExists`; guard ở đường release chỉ chứng minh khoản giữ *từng được đặt*, không chứng minh nó *còn treo*. Cả hai đường cùng trừ `frozen_balance` theo `HoldAmount`. Capture xong → một lần thất bại cạn retry gọi release với dedup key khác → toàn bộ `HoldAmount` cộng vào balance lần hai, rút từ quỹ đóng băng của batch khác.

### M5. Migration 027 viết lại toàn bảng và khoá index trong lúc khởi động **[ĐÃ SỬA]**
`backend/migrations/027_usage_billing_consistency.sql:9`, `:32` · `migrations_runner.go:257-278`
`UPDATE usage_logs WHERE request_id = ''` toàn bảng + quét `ROW_NUMBER()` trên mọi `request_id` khác null + `CREATE UNIQUE INDEX` **không** `CONCURRENTLY`, tất cả trong một transaction lúc boot. Trên `usage_logs` lớn: chặn ghi log request, server không boot xong, timeout thì mất trắng tiến độ.

### M6. Dọn index INVALID chỉ áp dụng cho 5 trong số các file `_notx` **[ĐÃ SỬA]**
`migrations_runner.go:292` · `:524` · `migrations/190_add_users_email_alias_dedup_index_notx.sql:6`
`prepareNonTransactionalMigration` chỉ drop index INVALID cho 5 file trong whitelist, trong khi `validateMigrationExecutionMode` buộc **mọi** file `_notx` phải dùng `IF NOT EXISTS`. Các file 062, 072, 148, 151, 155, 190 không được dọn → `CREATE INDEX CONCURRENTLY` bị ngắt để lại index INVALID vĩnh viễn, boot sau `IF NOT EXISTS` bỏ qua và ghi nhận đã áp dụng.

### M7. Hai admin gia hạn subscription cùng lúc chỉ tính một lần **[ĐÃ SỬA]**
`backend/internal/service/subscription_service.go:655`
`ExtendSubscription` đọc `GetByID` rồi ghi `ExtendExpiry` — không transaction, không `FOR UPDATE`, không CAS trên `expires_at` cũ. Hai lệnh +30 ngày đồng thời → khách nhận 30 ngày thay vì 60, không lỗi nào nổi lên. Khuôn đúng đã có sẵn: `updateExistingSubscriptionTerm`.

### M8. Mã nạp tiền chỉ có ~17 bit ngẫu nhiên và được tạo ngoài transaction **[SỬA MỘT PHẦN]**
`backend/internal/service/payment_order.go:216` · `payment_fulfillment.go:343-352`
`fmt.Sprintf("PAY-%d-%d", order.ID, time.Now().UnixNano()%100000)` — hậu tố 5 chữ số trên order id tuần tự. `doBalance` gọi `CreateCode` và `Redeem` ở hai bước không nguyên tử. Crash giữa hai bước để lại mã `unused` còn sống; user bất kỳ dò `PAY-<orderID>-<0..99999>` trên `POST /api/v1/redeem` cướp được khoản nạp của nạn nhân.

### M9. Một repo method dùng nhầm client, tự khoá chính mình **[ĐÃ SỬA]**
`backend/internal/repository/promo_code_repo.go:208` · `backend/internal/service/promo_service.go:118`
`GetUsageByPromoCodeAndUser` là method **duy nhất** trong file dùng `r.client` thay vì `clientFromContext(ctx, r.client)`. Transaction đang giữ connection + khoá `FOR UPDATE` trên `promo_codes` lại xin connection thứ hai → pool cạn thì tự deadlock. Fix một dòng.

### M10. EasyPay chấp nhận endpoint `http://`, gửi khoá thương nhân dạng rõ **[ĐÃ SỬA]**
`backend/internal/payment/provider/easypay.go:69` · `:306` · `:439` (so với `airwallex.go:109`)
`normalizeEasyPayAPIBase` nhận mọi scheme (kể cả không scheme), trong khi `QueryOrder` và `refundAttempts` đặt secret `pkey` vào body POST dưới trường `key=`. Người quan sát mạng lấy `pkey` → ký được `sign` hợp lệ cho `/api/v1/payment/webhook/easypay` với `out_trade_no` tuỳ ý.

### M11. Migration 131 xoá log audit thanh toán không sao lưu **[KHÔNG SỬA — CÓ LÝ DO]**
`backend/migrations/131_affiliate_rebate_hardening.sql:35`, `:40`
`DELETE FROM payment_audit_logs` vô điều kiện mọi dòng trùng `(order_id, action)` để dọn chỗ cho unique index, trên bảng chứng từ tài chính. Migration 220 cùng repo thì có snapshot sang `groups_video_price_backup_220` trước khi xoá.

**Ghi chú — 131 đã chạy ở mọi nơi, không còn gì để sửa.** Những dòng trùng bị xoá đã không còn; thêm snapshot hôm nay — dù sửa 131 hay thêm migration mới — chụp được **con số không**, vì migration không khôi phục được dòng đã xoá. Trên bản cài mới bảng rỗng khi 131 chạy; trên bản restore thiếu tracking table thì unique index đã tồn tại nên không thể có dòng trùng. Sửa nó chỉ tốn một checksum rule mà không giúp được ai.

Sản phẩm đúng cho phát hiện này là ghi chú review, không phải code. Muốn chặn lặp lại thì chỗ để viết là `backend/migrations/README.md`.

### M12. Cache subscription bị xoá trước khi commit khi đổi mã **[ĐÃ SỬA]**
`backend/internal/service/redeem_service.go:495`, `:555-569` · `subscription_service.go:218`, `:273-275`
Đường đổi mã gọi `AssignOrExtendSubscription` với `deferCacheInvalidation=false` → bỏ entry L1 ristretto **trước** commit; `invalidateRedeemCaches` sau commit không gọi `InvalidateSubCache`, cũng không `PublishSubscriptionCacheInvalidation`. Request song song nạp lại dòng tiền-commit vào L1 → hạn mới vô hình suốt TTL trên mọi instance. Rủi ro này **đã được ghi chú** tại `subscription_service.go:273-275` cho đường thanh toán nhưng chưa vá cho đường đổi mã.

### M13. Hạn mức nạp tiền hằng ngày vượt được bằng N request song song **[SỬA MỘT PHẦN]**
`backend/internal/service/payment_order.go:330` · `:246`
`checkDailyLimit` nạp toàn bộ đơn đã thanh toán trong ngày bằng `.All(ctx)` rồi cộng ở tầng Go, trong tx READ COMMITTED không khoá dòng user. `checkPendingLimit` cùng hình dạng.

### M14. Năm migration thiếu `IF NOT EXISTS`, chặn server khởi động **[ĐÃ SỬA]**
`migrations/069_add_group_messages_dispatch.sql:1` · `041_add_model_routing_enabled.sql:2` · `044b_add_group_mcp_xml_inject.sql:2` · `189_add_group_allow_live.sql:1` · `036_ops_error_logs_add_is_count_tokens.sql:8`
`ALTER TABLE ... ADD COLUMN` trần, trái quy ước idempotent của repo. Trên DB đã có cột nhưng thiếu dòng `schema_migrations` (restore loại trừ bảng tracking, clone schema-only, DDL vá tay) → lỗi `column already exists`, server từ chối boot, chỉ sửa được bằng tay.

### M15. Idempotency lưu JSON không hợp lệ cho response lớn **[BẢN RÀ SOÁT BỎP SÓT KHỎI MỌI PR — CHƯA SỬA]**
`backend/internal/service/idempotency.go:451-461`
`marshalStoredResponse` nối `"...(truncated)"` vào JSON **đã** serialize khi vượt `MaxStoredResponseLen` (mặc định 64 KiB), rồi vẫn ghi qua `MarkSucceeded`. Retry cùng `Idempotency-Key` → `json.Unmarshal` hỏng → trả `ErrIdempotencyStoreUnavail` (503) thay vì kết quả cache.

**Ghi chú — cả hai mới sửa được một nửa, cố ý dừng lại.**

M8: entropy đã xử lý (crypto/rand 130 bit, bỏ số đơn khỏi mã). **Tính nguyên tử thì chưa**: `RedeemService.Redeem` luôn tự mở transaction riêng thay vì nhận từ context, nên `CreateCode` và `Redeem` vẫn là hai giao dịch. Cửa sổ mồ côi giờ vô hại (mã 130 bit, không endpoint người dùng nào trả về, lần retry sau tự chữa) nhưng muốn đóng hẳn thì phải sửa `redeem_service.go`.

M13: tạo đơn đã được tuần tự hoá. Nhưng hạn mức ngày **chỉ tính đơn đã thanh toán**, nên vẫn mở được nhiều đơn pending cùng thấy `used=0` rồi trả hết. Hiện bị chặn bởi `MaxPendingOrders` (mặc định 3). Đóng hẳn thì hoặc tính cả đơn pending vào hạn mức (đơn bỏ dở sẽ chặn người dùng suốt `OrderTimeoutMin`), hoặc kiểm lại lúc hoàn tất đơn (tức từ chối cộng tiền đã thu). Cả hai đều là quyết định sản phẩm nên giữ nguyên ngữ nghĩa.

### M16. Hai `Stop()` thiếu `sync.Once`, panic khi tắt hai lần **[ĐÃ SỬA]**
`backend/internal/service/pricing_service.go:213-217` · `email_queue_service.go:140-144`
Cả hai gọi `close(s.stopCh)` trần. Mọi worker nền khác cùng package (`AccountExpiryService`, `UsageRecordWorkerPool`, `SubscriptionMaintenanceQueue`) đều guard bằng `sync.Once` hoặc cờ có mutex; repo còn có sẵn `TestSessionStore_Stop_Idempotent` ép quy ước này.

### M17. Open redirect ở màn đăng nhập **[ĐÃ SỬA]**
`frontend/src/views/auth/LoginView.vue:604-605`, `:645-646`, `:714-715`
`router.push(redirectTo)` nhận thẳng `query.redirect` không kiểm tra. Mọi view callback OAuth — `OAuthCallbackView.vue:242-248`, Wechat, Oidc, DingTalk, LinuxDo — đều lọc qua `sanitizeRedirectPath()` (chặn `//`, `://`, CRLF). Chỉ LoginView bỏ sót.

**Đính chính khi sửa — bản rà soát bỏ sót một đường bỏ qua.** Ngoài `//`, `://`, CRLF còn một lỗ nữa, **không liên quan gì đến backslash**: WHATWG xóa TAB/LF/CR **trước khi** parse, nên `/<TAB>/evil.com` không chứa `//` trong chuỗi thô nhưng parser vẫn thấy `//evil.com`. Luật CRLF cũ chặn LF/CR, **không chặn TAB**.

Luật mới chuẩn hoá đúng thứ parser sẽ thấy (xóa ký tự bị bỏ qua, gấp backslash thành `/`) rồi mới kiểm vị trí authority. Năm luật cũ giữ nguyên từng byte. Khoảng hở chấp nhận, đã ghim trong spec: chuỗi dạng `?to=https:/` + backslash + `evil.com` vẫn qua — vô hại ở sink này vì là query param, không phải đích điều hướng.

Cả 6 bản sao của `sanitizeRedirectPath` đã gộp về `frontend/src/utils/redirect.ts` (1 định nghĩa, 7 nơi import). Lưu ý: `DingTalkCallbackView.vue` và `DingTalkEmailCompletionView.vue` **không có spec** — `vue-tsc` là cổng duy nhất cho hai file đó.

### M18. Danh sách sửa được dùng `:key="index"` **[ĐÃ SỬA]**
`frontend/src/components/account/BulkEditAccountModal.vue:381-401` · `CreateAccountModal.vue:1178,1473,2287,3242` · `EditAccountModal.vue:278,693,919,1247,2167`
Hàng model mapping cho phép thêm/xoá nhưng khoá theo chỉ số. Xoá hàng giữa → Vue tái dùng DOM theo vị trí, state cục bộ gắn nhầm hàng. Khuôn đúng đã có: `HeaderOverrideEditor.vue` dùng `getHeaderOverrideRowKey(row)`.

**Đính chính khi sửa — số dòng đã cũ.** `EditAccountModal.vue` (cả 5 vị trí) và `CreateAccountModal.vue` (cả 4 vị trí) **đã dùng** resolver khóa ổn định từ trước — repo có sẵn `frontend/src/utils/stableObjectKey.ts`, resolver dựa trên WeakMap nên không thêm field id vào object, không rò vào payload.

Ba chỗ còn sót thật nằm ở nơi khác: `CreateAccountModal.vue:1936` (hàng Bedrock mapping-mode, **không có trong danh sách trên**) và `BulkEditAccountModal.vue:382`, `:1232`. Đã sửa ba chỗ đó. Không còn `v-for` nào trong ba modal khoá theo chỉ số.

### M19. Token lưu trong `localStorage` — đánh đổi kiến trúc, không phải regression
`frontend/src/stores/auth.ts:301-329`, `:357-395` · `frontend/src/api/tokenRefresh.ts:51`, `:145-147`
Access token, refresh token xoay vòng, object user nằm trong `localStorage` dạng rõ → mọi XSS đọc được refresh token và tự đúc access token vô hạn.
**Ghi chú:** app dùng Bearer token chứ không cookie ngay từ thiết kế, nên đây là đánh đổi có chủ ý — agent xếp CRITICAL, đã hạ xuống MEDIUM. Chuyển sang httpOnly cookie là thay đổi lớn, cân nhắc riêng. Điều đáng nói: nó làm H7 và mọi lỗ XSS khác **đắt hơn bình thường**.

---

## Low

### L1. Danh sách redact log bỏ sót các tên trường secret đang dùng thật **[ĐÃ SỬA]**
`backend/internal/util/logredact/redact.go:14-34` — chỉ phủ nhóm OAuth (`access_token`, `refresh_token`, `client_secret`, `password`), thiếu `secret_key`, `api_key`, `private_key`, `api_v3_key`, `cookie`. Chưa có call site nào rò thật; bẫy chờ lệnh log tương lai.

### L2. Client outbound của prompt-audit cố ý bỏ chặn SSRF
`backend/internal/securityaudit/prompt_outbound_security.go:58-89` — `NewSecureHTTPClient` tắt bảo vệ SSRF kèm comment giải thích đích đến là việc của quản trị viên; chỉ vô hiệu hoá kế thừa proxy từ env. Quyết định thiết kế có ghi chép, ghi nhận vì nằm trong phạm vi rà soát.

### L3. Tiền tệ đi qua interface dưới dạng `float64`, kéo theo dung sai 0.01
`backend/internal/payment/types.go:174` · `payment_fulfillment.go:112` — `PaymentNotification.Amount` và `QueryOrderResponse.Amount` là `float64`, buộc đi vòng qua `InexactFloat64()` và `strconv.ParseFloat`. Hệ quả: `amountToleranceCNY = 0.01` được chấp nhận — đơn 100.00 CNY vẫn fulfill đủ khi trả 99.99, `PAYMENT_AMOUNT_MISMATCH` không bao giờ kích hoạt.

### L4. Khoá Redis khi đổi mã không có token chủ sở hữu **[ĐÃ SỬA]**
`backend/internal/repository/redeem_cache.go:59` — `ReleaseRedeemLock` là `DEL` vô điều kiện; `AcquireRedeemLock` set giá trị `1` với TTL 10s. Tx quá 10s → khoá hết hạn, request thứ hai giành được, `defer` của request đầu xoá khoá của người thứ hai. Tác động hạn chế vì lớp bảo vệ thật là UPDATE có điều kiện ở DB.

---

---

# Phần I-B — Phát hiện mới, tìm ra trong lúc sửa

Không có trong đợt rà soát gốc. Đánh số N… để không đụng dãy cũ.

### N1. Mười hai quy tắc tương thích checksum migration chết từ lúc viết — High **[ĐÃ SỬA]**

`backend/internal/repository/migrations_runner.go` (bản đồ `migrationChecksumCompatibilityRules`)

Chín quy tắc (`109, 110, 112, 118, 123, 195, 218, 219, 220`) khai `fileChecksum` là **output của `sha256sum <file>`** — hash toàn bộ byte **kể cả newline cuối file**. Runner lại tính `sha256(strings.TrimSpace(content))`. Hai giá trị không bao giờ bằng nhau vì migration nào cũng kết thúc bằng newline.

`git log -S` xác nhận runner dùng `TrimSpace` **từ commit đầu tiên** của file (`3d617de57`, trước khi chuyển `internal/infrastructure` sang `internal/repository`). Nên **chưa DB nào từng giữ những giá trị đó**: chín quy tắc này chết ngay lúc được viết, không phải mục dần theo thời gian. Người viết đã chạy `sha256sum` ở shell thay vì lặp lại cách runner tính.

Ba quy tắc nữa (`115, 116, 120`) có `fileChecksum` đúng nhưng **thiếu phiên bản lịch sử thứ ba** trong tập chấp nhận. Khác với chín cái trên, những giá trị này **có thể đang nằm trong DB thật** — bản cài nào giữ chúng thì boot hỏng ngay bây giờ, đúng loại sự cố mà cơ chế này sinh ra để tránh.

Vì `isMigrationChecksumCompatible` đòi **cả** checksum trong DB **lẫn** checksum file đều thuộc tập, một quy tắc sai `fileChecksum` không bao giờ kích hoạt được.

**Đã sửa:** giữ nguyên mọi giá trị cũ (20 khóa không đổi, 0 checksum bị mất, tập chấp nhận 44 → 68), thêm checksum đúng và các phiên bản lịch sử còn thiếu — liệt kê bằng `git rev-list --objects --all --full-history` chứ không phải `git log` (history simplification của `git log` giấu thay đổi phía merge). Test canh: `TestMigrationChecksumCompatibilityRulesMatchEmbeddedFiles`.

**Còn hở:** `TestMigrationChecksumCompatibilityRules_CoverEditedUpgradeCompatibilityMigrations` hardcode 8 tên file và chỉ kiểm `NotEmpty` — nó pass vui vẻ trong suốt thời gian cả 8 quy tắc đó hỏng. Giờ thừa, nên xóa. Và tính chất "phủ hết lịch sử" chỉ kiểm được với lịch sử clone này thấy — 5 quy tắc khai giá trị không có blob cục bộ, bình thường với một fork.

### N2. URL nhúng trang tùy biến đính JWT phiên vào query string — High **[ĐÃ SỬA]**

`frontend/src/utils/embedded-url.ts:16-46` · `frontend/src/views/user/CustomPageView.vue:98-111`

Khác H7: H7 nói về iframe chế độ markdown (`v-html`). Đây là iframe chế độ embed, đường mã khác hẳn.

`buildEmbeddedUrl` đính `token=` **chính là access JWT thật** (`authStore.token`, `stores/auth.ts:301`) — cùng giá trị gửi làm `Authorization: Bearer` ở mọi nơi. TTL mặc định **24h** (`config.go:2262-2264`). Claim mang `Role`, nên admin mở trang `visibility:"admin"` thì rò **JWT admin**; `RoleAdmin` là tầng cao nhất, không có super-admin.

Hai bề mặt chứ không một: iframe (`:107-111`) và một link `target="_blank"` (`:98-106`) — cái sau đưa JWT lên thanh địa chỉ và vào lịch sử trình duyệt.

Ràng buộc IP+UA **tắt mặc định** (`setting_features.go:240-249`); kể cả bật cũng không chặn được kênh chính: script bất kỳ trên trang của nhà cung cấp đọc `location.search` rồi gọi từ chính trình duyệt nạn nhân — IP và UA khớp.

`menuItem.url` do admin đặt qua `PUT /api/v1/admin/settings`, chỉ validate scheme (`config.go:3778-3800`): `http://` và loopback đều qua, không allowlist host, và nhóm route settings **không có step-up 2FA** (`admin.go:555` so với `:44`, `:69`, `:80`). CSP không đỡ — nó **nới theo**: `GetFrameSrcOrigins` (`setting_public.go:761-795`) gặt mọi URL menu vào `frame-src` lúc chạy.

`docs/ADMIN_PAYMENT_INTEGRATION_API.md:102-113` **có ghi** `token` được đính kèm, nhưng không luồng tích hợp nào trong tài liệu **tiêu thụ** nó: mọi lời gọi được ghi chép đều là server-to-server dùng admin API key, định danh người dùng bằng `user_id` trong body. Backend xác nhận không bao giờ nhận JWT panel từ query param.

**Đã sửa:** thêm cờ `pass_token` **theo từng mục menu, mặc định tắt** — mọi bản cài hiện tại ngừng rò ngay, ai thật sự cần thì bật lại có chủ đích. Bật cờ thì bắt buộc `https://`. Thêm `referrerpolicy="no-referrer"`. `src_url` rút về `origin + pathname` (trước đó đưa cả query string của trang cha cho bên thứ ba). Không thêm `sandbox`: không có `allow-same-origin` thì frame thành opaque origin, gửi `Origin: null`, CORS (`middleware/cors.go:69-80`) từ chối — sẽ làm hỏng nhà cung cấp nào cần gọi ngược.

**Chưa xác định được từ repo:** trang của nhà cung cấp thật có đọc `token` hay không — mã đó không nằm ở đây. Hỏi người vận hành tích hợp thanh toán trước khi bỏ hẳn tham số.

**Phụ thu:** tài liệu nói `purchase_subscription_url` dùng cùng đường passthrough — **sai**. `buildEmbeddedUrl` chỉ có đúng một caller; route `/purchase` giờ render `PaymentView.vue` native, không iframe. Đã sửa tài liệu.

### N3. Hai mươi chín goroutine xử lý stream không có `recover` — High **[ĐÃ SỬA]**

C3 không phải trường hợp cá biệt. Quét lại: `recover()` chỉ xuất hiện ở 16 chỗ non-test trong `internal/service`, và **không chỗ nào nằm trong goroutine xử lý stream**. Cùng hình dạng lặp lại ở 29 vị trí: SSE line pump trên `resp.Body`, goroutine biến đổi qua `io.Pipe`, `bufio` chunk pump, WebSocket read pump trên frame do upstream kiểm soát. Mỗi chỗ biến một response upstream dị dạng thành **chết cả tiến trình**, vì `Recovery()` của gin chỉ bọc goroutine của request.

**Đã sửa:** `internal/service/stream_recover.go` và `internal/service/openai_ws_v2/stream_recover.go` (phải tách gói: `openai_ws_v2` được `service` import, không import ngược được).

Hai điểm dễ làm sai, ghi lại để khỏi lặp:

- `recover()` **chỉ hoạt động khi được gọi trực tiếp bởi hàm defer**. Một wrapper ủy quyền cho `recoverX()` chung sẽ trả `nil` mà không báo lỗi gì.
- `defer recoverStream()` dán đều là sai: nó biến "chết tiến trình" thành "request treo vĩnh viễn". Mỗi vị trí phải gỡ đúng thứ consumer đang chặn trên — channel thì gửi event lỗi **trước** `close`, `io.Pipe` thì `CloseWithError` (`Close` trơn cho ra EOF cụt âm thầm), gRPC stream thì phải `cancel()` vì `Recv` không nhả theo channel send.

Hai chỗ cleanup sẵn có vốn đã sai: `plugin_runtime.go:357` (`defer writer.Close()` che panic thành EOF cụt) và `plugin_runtime.go:236` (consumer kẹt ở `stream.Recv()`).

---

# Phần II — Đã kiểm tra và xác nhận an toàn

Ghi lại để không rà lại lần sau.

- **Chống double-spend khi đổi mã.** `redeem_code_repo.go:324-340` dùng `UPDATE ... WHERE id=? AND status='unused'` với `affected==0 → ErrRedeemCodeUsed`, cùng transaction với lệnh cộng tiền. Khoá Redis chỉ là đường tắt dư thừa.
- **Tính tiền theo usage nguyên tử và idempotent.** Claim `usage_billing_dedup (request_id, api_key_id)` nằm trong tx, unique index xác nhận ở migration 071/073; balance/quota/rate-limit/account-quota đều là câu SQL đơn nguyên tử; `quantizeMonetaryFields` pre-round về `NUMERIC(20,8)`.
- **Verify chữ ký webhook đúng và constant-time ở cả 5 provider.** `hmac.Equal` tại `easypay.go:602` và `airwallex.go:521`; SDK verifier cho Alipay/wxpay/Stripe; không `==` trên MAC ở đâu. Amount/currency/order-id luôn đọc lại từ đơn đã lưu, không lấy từ callback. Không `InsecureSkipVerify`, không cờ sandbox bypass.
- **Không có SQL injection.** Mọi `fmt.Sprintf` gần SQL đều dựng placeholder `$%d` hoặc chọn cột ORDER BY từ allowlist cứng (`usage_log_repo_query.go:225`, `channel_repo.go:283`, `ops_repo.go:180`, `usage_log_repo_trend.go:678`); định danh qua `pq.QuoteIdentifier`.
- **JWT chắc chắn.** `auth_service.go:1348-1387` — parser ghim HS256/384/512 qua `jwt.WithValidMethods`, type-assert `*jwt.SigningMethodHMAC` trong keyfunc, `exp`/`nbf`/`iat` validate, `TokenVersion` (sha256 email+password hash) vô hiệu hoá mọi token đang lưu hành khi đổi mật khẩu. Refresh token 256 bit, lưu SHA-256, rotate mỗi lần dùng, family-revoke khi tái sử dụng (`:1779-1862`). `config.go:2710-2717` fail startup nếu `jwt.secret` rỗng hoặc < 32 byte, cảnh báo trên denylist giá trị yếu.
- **Middleware admin không tin claim.** `middleware/admin_auth.go` re-fetch user từ DB rồi kiểm `user.IsAdmin()` (không tin `claims.Role`); admin API key so bằng `subtle.ConstantTimeCompare`. Mọi route trong `routes/admin.go` và nhóm admin của `routes/payment.go` đều nằm sau `admin.Use(adminAuth)`.
- **Step-up/TOTP fail-closed.** `middleware/step_up.go`, `service/totp_service.go` — nil settings thì chặn, chặn admin-API-key trên thao tác nhạy cảm, rate-limit 5 lần/15 phút, constant-time setup token, TOTP secret mã hoá at-rest. Ngoại lệ `audit-logs/clear` thật sự có re-verify TOTP trong handler.
- **Không có IDOR** ở API key CRUD, payment order, usage record/daily-usage, profile update (DTO không bind `role`/`balance`). Tất cả re-derive owner từ `GetAuthSubjectFromContext` phía server rồi đối chiếu `UserID`.
- **Plugin `.s2plugin` được ký và ghim.** `service/plugin_package.go` + `plugin_runtime.go` — bắt buộc chữ ký ed25519 với `AllowUnsigned` mặc định `false`, ghim SHA-256 từng file, chặn zip-slip + symlink, giới hạn kích thước; chạy qua HashiCorp go-plugin với `SecureConfig` checksum pinning, Unix socket, `SkipHostEnv`. Upload/enable cần admin + step-up.
- **Route `/plugin-ui/:token/*path`** nằm ngoài `admin.Use(adminAuth)` (`routes/admin.go:24`) nhưng token là capability token AES-GCM, có prefix mục đích, TTL ≤ 1h, chỉ mint bởi `CreateUISession` đã xác thực admin; asset resolve qua manifest whitelist + `safePluginJoin`. Không khai thác được.
- **Trình hướng dẫn cài đặt không mở lại được.** `internal/setup/{setup.go,handler.go}` — gate bằng `NeedsSetup()` (vắng cả `config.yaml` lẫn `.installed`), double-check dưới mutex trên `/setup/install`, trả 403 sau khi cài.
- **`embed_on.go` không XSS.** Settings inject qua `json.Marshal` (HTML-escape `<`, `>`, `&` mặc định) nên không breakout `</script>`; `site_logo`/`site_name` còn được validate/escape thêm.
- **Passthrough header ở các path còn lại.** Whitelist `gateway_service.go:427-449` loại `Authorization`/`Cookie`/`Host`; `account_header_override.go` chặn ghi đè header auth/hop-by-hop/session. `xai.BuildVideoURLWithValidator` (`pkg/xai/oauth.go:721-736`) path-escape và từ chối `.`/`..`/ký tự điều khiển.
- **Refresh token đa tab không đua.** `api/tokenRefresh.ts` điều phối bằng `navigator.locks` + phát hiện token của tab khác; `api/client.ts` chỉ gắn auth header cho instance axios same-origin. `utils/url.ts#sanitizeUrl` và `utils/sanitize.ts#sanitizeSvg` dùng nhất quán cho `:href`/`:src`/SVG. Cleanup `onUnmounted`/`onBeforeUnmount` đầy đủ (PaymentStatusPanel, StripePaymentView, BackupView, ChannelStatusV2View, RiskControlView, useAutoRefresh, App.vue).
- **Session binding / rate limiting / CORS.** IP+UA binding hash kiểm ở cả access lẫn refresh path (fail-open chỉ với token legacy trước tính năng); panel rate limiter key theo `UserID` đã xác thực; public-IP limiter loại loopback/private/link-local; `X-Forwarded-For`/`CF-Connecting-IP` chỉ tin khi bật `trusted_proxies`/`TrustForwardedIP`; CORS wildcard ép `allow_credentials=false` và từ chối origin lạ kể cả preflight (403).
- **Triển khai mặc định lành mạnh.** `Dockerfile`, `deploy/docker-entrypoint.sh`, `deploy/docker-compose*.yml`, `deploy/Caddyfile` — non-root runtime user, `no-new-privileges`, không mount docker socket, Redis/Postgres không publish ra host, Caddy chỉ TLS 1.2/1.3.
- **CI.** `.github/workflows/cla.yml` dùng `pull_request_target` nhưng **không checkout code PR** và gate theo `github.repository == 'Wei-Shaw/sub2api'`; `security-scan.yml`/`release.yml` không expose secret cho fork PR.
- **`go vet -tags=unit ./...` sạch** trên service/util/middleware/cmd.

---

# Phần III — Plan xử lý

## Ràng buộc phải nhớ trước khi động vào migration

`migrations_runner.go:195-204` tính SHA256 nội dung file và so với cột `schema_migrations.checksum`; **lệch là lỗi boot**. Nên sửa file migration cũ bắt buộc phải thêm entry vào `migrationChecksumCompatibilityRules` (`migrations_runner.go:75`) theo khuôn:

```go
"037_ops_alert_silences.sql": newMigrationChecksumCompatibilityRule("<checksum cũ>", "<checksum mới>"),
```

Rule chỉ pass khi **cả** checksum trong DB **và** checksum file hiện tại đều nằm trong tập đã biết — xem comment `:72-74`. Có test canh ở `migrations_runner_extra_test.go:78`, `:108`.

Đồng thời: sửa file không giúp bản cài **đã** chạy version hỏng (migration đã ghi nhận, không chạy lại) → luôn cần **thêm** migration sửa chữa riêng.

---

## PR 1 — Migration tự hoàn tác · `fix/migration-goose-down-self-revert`

**ĐÃ XONG.** Lệch so với kế hoạch dưới đây: số hiệu là 235/236/237, thứ tự đối số helper ngược lại, và phát sinh thêm N1 (12 quy tắc checksum chết). Đọc đính chính ở C1 và N1.

Ưu tiên cao nhất: lỗi duy nhất đang hỏng dữ liệu trên mọi bản cài, và rẻ nhất để sửa.

1. Xoá khối `-- +goose Down` khỏi:
   - `backend/migrations/037_ops_alert_silences.sql` (dòng 25-28)
   - `backend/migrations/019_migrate_wechat_to_attributes.sql` (dòng 57-83)
   - `backend/migrations/024_add_gemini_tier_id.sql` (dòng 22-30)
2. Tính SHA256 mới của ba file, thêm ba entry vào `migrationChecksumCompatibilityRules`.
3. Thêm ba migration sửa chữa, tất cả idempotent:
   - `221_repair_ops_alert_silences.sql` — `CREATE TABLE IF NOT EXISTS` + index, copy nguyên phần Up của 037.
   - `222_repair_wechat_attribute_migration.sql` — chép lại từ `users.wechat` sang attribute value, bỏ soft-delete trên definition, `ON CONFLICT DO NOTHING`.
   - `223_repair_gemini_tier_id_default.sql` — chạy lại đúng câu UPDATE của 024 Up.
4. Cân nhắc bỏ luôn `-- +goose Up` / `StatementBegin` / `StatementEnd` khỏi ba file để không ai hiểu nhầm repo dùng goose. Quét toàn bộ `migrations/` một lượt.

**Test chặn tái phát (phần quan trọng nhất của PR).** Thêm test trong `backend/migrations/` khẳng định không file nào chứa chuỗi `+goose Down`, kèm comment giải thích runner không parse goose. Lỗi này lọt được vì không có gì canh.

**Kiểm chứng.** `go test -tags=integration ./internal/repository/...` (cần Docker, dựng PostgreSQL qua testcontainers) — chạy migration trên DB sạch, xác nhận `ops_alert_silences` tồn tại sau boot.

---

## PR 2 — SSRF Antigravity · `fix/antigravity-upstream-ssrf`

**ĐÃ XONG.** H4 chỉ đúng một nửa — xem đính chính dưới H4. Nhánh lỗi đổi signature trả về, kéo theo thay đổi hành vi billing trên 4xx/5xx.

Một file, ba sửa đổi, không đụng ai khác.

1. `antigravity_gateway_upstream.go:36` — chèn `if err := s.validateUpstreamBaseURL(baseURL); err != nil { return nil, err }` ngay sau khi trim `base_url`. Đối chiếu chữ ký hàm ở `gateway_upstream_request.go:33` để gọi cho khớp receiver.
2. `:127` — thay `io.ReadAll(resp.Body)` bằng `ReadUpstreamResponseBody` với cùng ngưỡng `cfg.Gateway.*MaxBytes` các platform khác dùng (mẫu: `grok_media.go:706-708`).
3. `:99-101` — thay `c.Writer.Write(respBody)` ở nhánh lỗi bằng đường `sanitizeUpstreamErrorMessage` như Anthropic/OpenAI. Nhánh thành công (`:136-138`) giữ nguyên body vì đó là response hợp lệ cho client.

**Test.** Unit test dựng account với `base_url = http://169.254.169.254`, khẳng định `ForwardUpstream` trả lỗi **trước khi** phát request.

---

## PR 3 — Bedrock frame validation · `fix/bedrock-eventstream-bounds`

**ĐÃ XONG.** Ràng buộc đề xuất ở bước 1 không dùng được (tràn 32-bit) — xem đính chính dưới C3. Bước 3 (quét decoder anh em) đã làm và thành N3.

1. `bedrock_stream.go` sau dòng 266, thêm kiểm tra trước khi cắt slice:
   - `headersLength > uint32(len(data)-4)` → trả lỗi `invalid eventstream frame`.
   - Kiểm `int(headersLength)+4 <= len(data)` để chặn tràn khi ép kiểu.
2. `:74` — bọc thân goroutine bằng `defer func() { if r := recover(); r != nil { /* log + đóng stream */ } }()`.
3. Rà các decoder streaming anh em cùng họ xem có goroutine nào khác thiếu recover — cùng một khuôn lỗi.

**Test.** Table-driven test với frame CRC hợp lệ nhưng `headers_length` vượt `total_length-16`, khẳng định trả lỗi chứ không panic.

---

## PR 4 — Quản lý khoá mã hoá · `fix/secret-encryption-key`

Cần một đề xuất trong `openspec/changes/` trước: động tới schema, migration dữ liệu, và một biến môi trường đang tồn tại.

1. `payment_config_providers.go:534-540` — khôi phục `payment.Encrypt` trong `encryptConfig`. Giữ nhánh plaintext của `decryptConfig` làm shim đọc, đổi comment `TODO(deprecated-legacy-ciphertext)` cho khớp chiều mới.
2. Migration mã hoá lại các dòng `payment_provider_instances.config` đang nằm trần. **Không** làm được bằng SQL thuần vì cần khoá ứng dụng — viết thành bước khởi động một lần trong repository layer, có cờ đánh dấu đã chạy.
3. Đổi tên `totp.encryption_key` → `security.secret_encryption_key`, giữ biến cũ làm alias đọc để không phá bản cài đang chạy.
4. `config.go:1925-1937` — bỏ auto-generate. Khi trống **và** có bất kỳ tính năng lưu secret nào bật (backup S3, channel monitor, image storage, plugin, TOTP) thì fail startup với thông báo nêu rõ cần đặt biến nào.
5. Cập nhật `deploy/.env.example` **và cả ba file compose** — `.env.example` trong repo này hay ghi knob mà docker-compose không forward, kiểm cả ba.

---

## PR 5 — Ba race về tiền · `fix/billing-race-conditions`

Cùng khuôn sửa nên gộp một PR, nhưng ba commit tách bạch.

1. **H1** `affiliate_repo.go:117` — đưa kiểm tra trần vào trong transaction của `AccrueQuota`, sau `SELECT ... FROM user_affiliates WHERE user_id=$inviter FOR UPDATE`. Bỏ `GetAccruedRebateFromInvitee` khỏi `affiliate_service.go:361` hoặc chuyển thành đọc-trong-tx.
2. **H2** `payment_refund.go` — thêm cột lưu số tiền đã trừ lên order (cần migration); `prepDeduct` trừ đi phần đã lấy trước khi trừ tiếp.
3. **H3** `redeem_service.go:667` — phân giải id rồi dùng `GetByIDForUpdate` trong transaction; `:703` sửa ghi đè `Notes` thành nối thêm.

**Test.** Mỗi lỗi một integration test chạy hai goroutine đồng thời, khẳng định tổng cuối đúng. Không có test này thì không chứng minh được đã sửa.

---

## PR 6 — Đường rò API key · `fix/api-key-hashing`

Cần openspec proposal: đổi schema Ent + migration + ảnh hưởng đường xác thực nóng. **Tách hai release** vì không thể vừa drop cột vừa giữ tương thích rollback.

Release 1:
1. `ent/schema/api_key.go` — thêm cột `key_hash` unique, **giữ** `key`. Chạy `go generate ./ent`, commit output.
2. Migration backfill `key_hash = sha256(key)` cho mọi dòng.
3. `api_key_repo.go:111-167` — `GetByKeyForAuth` tra theo `key_hash`. Đây là đường nóng mọi request đi qua, cần đo lại hiệu năng.
4. Backend trả bản mask cho endpoint admin; `UserApiKeysModal.vue:17` bỏ `substring`, hiển thị thẳng giá trị đã mask.

Release 2: migration drop cột `key`.

---

## PR 7 — Gói frontend · `fix/frontend-xss-and-redirect`

**ĐÃ XONG.** H7 theo phương án allowlist + sandbox (không cắt iframe). M17 lộ thêm đường bỏ qua bằng TAB. M18 phần lớn đã đúng sẵn, ba chỗ sót thật nằm chỗ khác — xem đính chính.

1. **H7** `CustomPageView.vue:249-252` — bỏ `ADD_TAGS: ['iframe']` và `ADD_ATTR`. Nếu tính năng nhúng đang có người dùng thì thay bằng allowlist host + `sandbox`; **quyết định này cần hỏi trước khi cắt**.
2. **M17** Tách `sanitizeRedirectPath` từ `OAuthCallbackView.vue:242-248` ra `src/utils/`, gọi ở `LoginView.vue` cả ba chỗ.
3. **M18** Đổi `:key="index"` sang id ổn định ở ba modal account, theo khuôn `HeaderOverrideEditor.vue`.

**Lưu ý gate.** ESLint trong repo này bỏ qua file `.vue` — "eslint 0 lỗi" không nói gì về SFC. `vue-tsc` là cổng thật nhưng nó chỉ type-check file *được import*, nên component mồ côi không được kiểm. `pnpm` và `make` không có trên máy dev hiện tại; chạy binary trực tiếp:

```
frontend/node_modules/.bin/vue-tsc --noEmit
frontend/node_modules/.bin/vitest run
```

Không bọc `2>&1` trong PowerShell — nó luôn báo lỗi bất kể exit code thật.

---

## PR 8 — Phần còn lại

Gom theo cụm, không gấp:

- **Hạ tầng migration** — M5 (027), M6 (`_notx` INVALID index), M11 (131 xoá audit), M14 (5 file thiếu `IF NOT EXISTS`).
- **Đồng thời còn lại** — M7 (`ExtendSubscription`), M13 (hạn mức nạp), M12 (cache subscription khi đổi mã), M16 (`sync.Once`).
- **Lặt vặt** — M8 (entropy mã nạp), M9 (`promo_code_repo.go:208`, fix một dòng), M10 (EasyPay chặn `http`), M2 (bật allowlist mặc định), M3 (ownership Grok video), M4 (batch image hold), M1 nếu chưa gộp vào PR 6.
- **Low** — L1, L3, L4. L2 là design choice, không sửa.
- **M19** (`localStorage`) — quyết định kiến trúc riêng, không nhét vào PR nào.

## Đuôi còn lại từ đợt 1

Nhỏ, đã xác minh trong lúc sửa, chưa làm:

- **Nil-deref `s.cfg` ở các gateway sibling.** Anthropic/OpenAI/Gemini viết `if s.cfg != nil && …` rồi vẫn deref `s.cfg` vô điều kiện trong nhánh allowlist. Antigravity đã né bằng accessor nil-safe; các sibling thì chưa.
- **Allowlist host iframe chưa cấu hình được.** Còn thiếu 3 chỗ: setting backend, `PublicSettings` trong `frontend/src/types/index.ts`, ô nhập admin UI. Danh sách hardcode hiện tại là phỏng đoán — đối chiếu với một bản triển khai thật trước.
- **Step-up 2FA cho nhóm route settings.** `custom_menu_items` (và mọi setting khác) ghi được mà không cần step-up, khác với nhóm account/proxy/data-management. Bán kính rộng hơn một phát hiện, nên tách riêng.
- **Xoá `TestMigrationChecksumCompatibilityRules_CoverEditedUpgradeCompatibilityMigrations`** — hardcode 8 tên file, chỉ kiểm `NotEmpty`, giờ thừa hoàn toàn so với test mới.
- **Thiếu spec cho `DingTalkCallbackView.vue` và `DingTalkEmailCompletionView.vue`** — `vue-tsc` là cổng duy nhất, mà nó không type-check file không được import.
- **Ba file test chưa gofmt** (có sẵn từ trước, không ai động): `billing_cache_service_user_platform_quota_test.go`, `grok_oauth_service_test.go`, `sticky_session_test.go`.

---

# Phần IV — Ghi chú vận hành

- Nhánh hiện tại `main` sạch. **Tạo nhánh trước khi commit.**
- `gh pr create` trần sẽ mở PR vào `Wei-Shaw/sub2api` (upstream) chứ không phải fork. **Luôn truyền `--repo`.**
- `golangci-lint` mù với build tag `unit` → `unused` false-positive trên caller có tag. Gate bằng `go test -tags=unit ./...`. `go test ./...` trần gần như không compile gì: unit và integration đều tag-gated, `testutil/` cũng `//go:build unit`.
- Trên Windows có vài test luôn đỏ, **không phải regression**: file-lock của plugin installer, một test timing.
- Ent: sửa `ent/schema/*.go` → `go generate ./ent`, commit output. Sửa provider → `go generate ./cmd/server` cho `wire_gen.go`.
- Thêm method vào service interface → mọi stub trong `internal/testutil` và `*_test.go` phải có method đó, không thì hỏng build unit.
- Migration SQL và Ent schema phải đổi cùng nhau.
- Commit message và comment trong repo chủ yếu tiếng Trung — khớp ngôn ngữ file đang sửa.
- `git commit -F -` với heredoc bị từ chối trong worktree; ghi message ra file rồi `-F <path>`.
