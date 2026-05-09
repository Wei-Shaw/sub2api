# V-Claw Client Device Activation Handoff

Tài liệu này mô tả **contract chính xác từ backend sub2api hiện tại** cho client V-Claw/OpenClaw sau thay đổi device activation.

Client chỉ cần xử lý 2 API:

1. `POST /api/v1/vclaw/claim`
2. `POST /api/v1/auth/invite-login`

DLG/device login code được backend trả từ API `POST /api/v1/vclaw/claim` ở field:

```text
data.device_login_code
```

API `POST /api/v1/auth/invite-login` **không sinh DLG mới**. API này nhận DLG qua request field:

```text
invitation_code
```

---

## 1. Contract chung của response envelope

Backend dùng response envelope chung:

### Success

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

### Error

```json
{
  "code": 403,
  "message": "device activation is pending admin approval",
  "reason": "DEVICE_ACTIVATION_PENDING"
}
```

Client phải đọc:

- `code === 0` và HTTP `200` cho success.
- `reason` cho lỗi business.
- Không parse trạng thái device từ text `message`.

---

## 2. API claim device

```http
POST /api/v1/vclaw/claim
Content-Type: application/json
```

Backend handler: `backend/internal/handler/vclaw_handler.go`

Service: `backend/internal/service/vclaw_claim_service.go`

### 2.1 Request body chính xác

```json
{
  "claim_code": "DCL-AAAA-BBBB-CCCC",
  "aff_code": "AFF_CODE_OR_EMPTY",
  "device": {
    "device_hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "fingerprint_version": 1,
    "install_id": "optional-install-id",
    "platform": "windows",
    "arch": "x64",
    "app_version": "1.0.0"
  }
}
```

### 2.2 Field rules từ backend

`claim_code`:

- JSON field: `claim_code`
- Backend normalize bằng `NormalizeRedeemCode`: trim + uppercase.
- Nếu gửi DCL code thì code phải tồn tại, type phải là `device_claim`.
- Nếu code đã dùng bởi device hiện tại, backend resume binding cũ và trả lại DLG cũ.

`aff_code`:

- JSON field: `aff_code`
- Optional từ góc nhìn client.
- Client chỉ gửi nếu app có affiliate/campaign code cần gửi kèm.

`device.device_hash`:

- Required.
- Phải là chuỗi hex 64 ký tự sau normalize lowercase/trim.
- Đây là khóa chính để backend resume binding device cũ.

`device.fingerprint_version`:

- Required.
- Phải lớn hơn `0`.

`device.install_id`:

- Optional.
- Client gửi nếu có.
- Không dùng làm khóa chính để resume device.

`device.platform`:

- Required.
- Không được empty.

`device.arch`:

- Required.
- Không được empty.

`device.app_version`:

- Optional.

### 2.3 Success response chính xác

Backend trả envelope:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "ok",
    "mode": "first_claim",
    "user_id": 123,
    "device_login_code": "DLG-AAAA-BBBB-CCCC",
    "device_binding_id": 456,
    "device_activation_status": "pending_activation",
    "claimed_at": "2026-05-09T08:00:00Z"
  }
}
```

Field trong `data`:

| Field | Type | Giá trị |
|---|---:|---|
| `status` | string | Backend đang trả cố định `"ok"` khi claim success. Không dùng field này để quyết định active/pending. |
| `mode` | string | `"first_claim"` hoặc `"resume"`. |
| `user_id` | number | User ID được bind với device. |
| `device_login_code` | string | DLG code. Đây là mã user gửi admin/marketing khi cần active device. |
| `device_binding_id` | number | ID binding device trong backend. |
| `device_activation_status` | string | `"active"` hoặc `"pending_activation"`. Đây là field client phải dùng để quyết định flow. |
| `claimed_at` | string | Timestamp claim/resume. |

### 2.4 Claim response khi device active

Ví dụ exact shape:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "ok",
    "mode": "first_claim",
    "user_id": 123,
    "device_login_code": "DLG-AAAA-BBBB-CCCC",
    "device_binding_id": 456,
    "device_activation_status": "active",
    "claimed_at": "2026-05-09T08:00:00Z"
  }
}
```

Client behavior:

1. Đọc `data.device_activation_status`.
2. Nếu là `"active"`, lấy `data.device_login_code`.
3. Gọi tiếp `POST /api/v1/auth/invite-login` với:

```json
{
  "invitation_code": "DLG-AAAA-BBBB-CCCC",
  "device_hash": "same 64-char device_hash",
  "install_id": "same install_id if available",
  "client_kind": "desktop"
}
```

4. Chỉ mở OpenClaw sau khi `/auth/invite-login` trả token thành công.

### 2.5 Claim response khi device pending activation

Ví dụ exact shape:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "ok",
    "mode": "first_claim",
    "user_id": 123,
    "device_login_code": "DLG-AAAA-BBBB-CCCC",
    "device_binding_id": 456,
    "device_activation_status": "pending_activation",
    "claimed_at": "2026-05-09T08:00:00Z"
  }
}
```

Client behavior:

1. Đọc `data.device_activation_status`.
2. Nếu là `"pending_activation"`, lấy `data.device_login_code`.
3. Lưu lại local pending state gồm tối thiểu:
   - `device_hash`
   - `install_id` nếu có
   - `device_login_code`
   - `device_activation_status = "pending_activation"`
4. Hiển thị màn hình chờ active.
5. Hiển thị rõ DLG code cho user copy/gửi admin/marketing.
6. Không gọi tiếp `/auth/invite-login` ngay trong path này.
7. Không mở OpenClaw.
8. Không xóa local state.

Suggested UI copy:

```text
Thiết bị của bạn đang chờ admin/marketing kích hoạt.
Vui lòng gửi mã sau cho admin/marketing để được kích hoạt:
DLG-AAAA-BBBB-CCCC

Sau khi được kích hoạt, bấm Thử lại.
```

### 2.6 Claim resume behavior

Khi client gọi lại `/vclaw/claim` với cùng `device.device_hash`, backend tìm binding cũ bằng `device_hash` và trả lại DLG đang bind với device đó.

Response có:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "ok",
    "mode": "resume",
    "user_id": 123,
    "device_login_code": "DLG-AAAA-BBBB-CCCC",
    "device_binding_id": 456,
    "device_activation_status": "pending_activation",
    "claimed_at": "2026-05-09T08:05:00Z"
  }
}
```

Client behavior khi resume:

- Nếu `device_activation_status === "pending_activation"`: tiếp tục show pending + DLG cũ.
- Nếu `device_activation_status === "active"`: gọi `/auth/invite-login` bằng DLG đó.

---

## 3. API device login bằng DLG

```http
POST /api/v1/auth/invite-login
Content-Type: application/json
```

Backend handler: `backend/internal/handler/auth_handler.go`

Service: `backend/internal/service/auth_service.go`

Device helper: `backend/internal/service/auth_invite_device_helper.go`

### 3.1 Request body chính xác

```json
{
  "invitation_code": "DLG-AAAA-BBBB-CCCC",
  "device_hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "install_id": "optional-install-id",
  "client_kind": "desktop",
  "turnstile_token": "optional-turnstile-token"
}
```

Field rules:

`invitation_code`:

- Required.
- Với V-Claw device flow, đây chính là DLG lấy từ `data.device_login_code` của `/vclaw/claim`.
- Backend normalize trim + uppercase.

`device_hash`:

- Required cho desktop/device client.
- Phải match đúng `device_hash` đã bind với DLG.
- Nếu `client_kind === "web"`, backend có nhánh cho phép thiếu `device_hash`; V-Claw desktop không dùng nhánh này.

`install_id`:

- Optional.
- Nếu khác install ID đã bind, backend hiện log cảnh báo nhưng không block nếu `device_hash` match.

`client_kind`:

- V-Claw desktop nên gửi `"desktop"` hoặc non-`web` value hiện có của client.
- Không gửi `"web"` cho desktop để tránh đi nhánh web-login đặc biệt.

`turnstile_token`:

- Handler luôn gọi `VerifyTurnstile`.
- Handler luôn gọi `VerifyTurnstile` trước khi vào service.
- Client giữ nguyên cách gửi `turnstile_token` đang dùng trong app hiện tại; nếu môi trường backend yêu cầu Turnstile thì token phải hợp lệ.

### 3.2 Success response khi device active

Exact envelope:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "eyJ...",
    "refresh_token": "rt_...",
    "expires_in": 86400,
    "token_type": "Bearer",
    "user": {
      "id": 123,
      "email": "...",
      "username": "...",
      "role": "user",
      "status": "active"
    },
    "bootstrap_api_keys": [
      {
        "id": 1,
        "name": "...",
        "key": "...",
        "group_id": 1,
        "platform": "..."
      }
    ]
  }
}
```

Client behavior:

1. Save `access_token`, `refresh_token`, `expires_in`, `token_type`.
2. Save/use `bootstrap_api_keys` nếu OpenClaw cần.
3. Mark local device state active.
4. Mở OpenClaw.

### 3.3 Error response khi device pending activation

Khi DLG đúng nhưng device status trong backend là `pending_activation`, helper trả `ErrDeviceActivationPending`.

Exact response envelope:

```json
{
  "code": 403,
  "message": "device activation is pending admin approval",
  "reason": "DEVICE_ACTIVATION_PENDING"
}
```

Important:

- `/auth/invite-login` không trả `device_login_code` trong response lỗi này.
- Client phải hiển thị lại chính DLG đang submit trong request `invitation_code`.

Client behavior:

1. Nếu HTTP status là `403` và `reason === "DEVICE_ACTIVATION_PENDING"`:
   - Không mở OpenClaw.
   - Không xóa DLG.
   - Không báo sai mã.
   - Show pending activation screen.
   - Show DLG đang dùng: request `invitation_code`.
2. User gửi DLG đó cho admin/marketing.
3. Sau khi admin/marketing active device, user bấm retry.
4. Retry gọi lại `/auth/invite-login` với cùng DLG + cùng `device_hash`.

### 3.4 Other relevant invite-login errors

Các lỗi này phải tách khỏi pending activation:

#### Missing invitation code

```json
{
  "code": 400,
  "message": "invitation code is required",
  "reason": "INVITATION_CODE_REQUIRED"
}
```

Client behavior: báo thiếu DLG/code.

#### Invalid DLG / wrong type / code not found

```json
{
  "code": 400,
  "message": "invalid or used invitation code",
  "reason": "INVITATION_CODE_INVALID"
}
```

Client behavior: báo mã không hợp lệ, không show pending activation.

#### Missing device hash

```json
{
  "code": 400,
  "message": "device_hash is required",
  "reason": "DEVICE_HASH_REQUIRED"
}
```

Client behavior: regenerate/repair fingerprint state hoặc yêu cầu restart repair flow.

#### Invalid device hash

```json
{
  "code": 400,
  "message": "device_hash must be a 64-character hex string",
  "reason": "DEVICE_HASH_INVALID"
}
```

Client behavior: fingerprint bug; không gọi retry loop vô hạn.

#### Device mismatch

```json
{
  "code": 403,
  "message": "device does not match bound login code",
  "reason": "DEVICE_MISMATCH"
}
```

Client behavior: DLG này không thuộc máy hiện tại. Không show pending activation. Yêu cầu claim lại hoặc liên hệ support.

#### Device revoked/blocked

```json
{
  "code": 403,
  "message": "device binding has been revoked",
  "reason": "DEVICE_REVOKED"
}
```

Client behavior: thiết bị đã bị revoke/blocked. Không show pending activation. Yêu cầu liên hệ support/admin.

---

## 4. Flow client phải implement

### 4.1 First claim flow

```ts
type ClaimResponseData = {
  status: 'ok'
  mode: 'first_claim' | 'resume'
  user_id?: number
  device_login_code?: string
  device_binding_id?: number
  device_activation_status?: 'active' | 'pending_activation'
  claimed_at?: string
}

async function claimAndMaybeLogin(input: ClaimInput) {
  const claim = await postVClawClaim({
    claim_code: input.claimCode,
    aff_code: input.affCode,
    device: {
      device_hash: input.deviceHash,
      fingerprint_version: input.fingerprintVersion,
      install_id: input.installId,
      platform: input.platform,
      arch: input.arch,
      app_version: input.appVersion,
    },
  })

  const data = claim.data
  const dlg = data.device_login_code
  const activationStatus = data.device_activation_status

  if (!dlg) {
    showFatalError('Server không trả device login code.')
    return
  }

  saveDeviceLoginCode(dlg)

  if (activationStatus === 'pending_activation') {
    savePendingActivationState({
      deviceHash: input.deviceHash,
      installId: input.installId,
      deviceLoginCode: dlg,
    })
    showPendingActivationScreen(dlg)
    return
  }

  if (activationStatus === 'active') {
    await loginWithDlg({
      dlg,
      deviceHash: input.deviceHash,
      installId: input.installId,
    })
    return
  }

  showFatalError('Server trả trạng thái kích hoạt thiết bị không hợp lệ.')
}
```

### 4.2 Login with existing DLG

```ts
async function loginWithDlg(input: {
  dlg: string
  deviceHash: string
  installId?: string
}) {
  try {
    const login = await postInviteLogin({
      invitation_code: input.dlg,
      device_hash: input.deviceHash,
      install_id: input.installId,
      client_kind: 'desktop',
    })

    saveAuth(login.data)
    markDeviceActive()
    openOpenClaw()
  } catch (err) {
    const apiErr = normalizeApiError(err)

    if (apiErr.reason === 'DEVICE_ACTIVATION_PENDING') {
      savePendingActivationState({
        deviceHash: input.deviceHash,
        installId: input.installId,
        deviceLoginCode: input.dlg,
      })
      showPendingActivationScreen(input.dlg)
      return
    }

    if (apiErr.reason === 'DEVICE_MISMATCH') {
      showDeviceMismatchError()
      return
    }

    if (apiErr.reason === 'DEVICE_REVOKED') {
      showDeviceRevokedError()
      return
    }

    if (apiErr.reason === 'INVITATION_CODE_INVALID') {
      showInvalidDlgError()
      return
    }

    showGenericLoginError(apiErr.message)
  }
}
```

### 4.3 Retry after admin/marketing active

Khi user bấm retry trên pending screen:

1. Lấy DLG đã lưu local.
2. Gọi lại `/api/v1/auth/invite-login` với cùng DLG.
3. Nếu success: save token và mở OpenClaw.
4. Nếu vẫn `DEVICE_ACTIVATION_PENDING`: giữ pending screen và show cùng DLG.
5. Nếu `DEVICE_REVOKED`, `DEVICE_MISMATCH`, `INVITATION_CODE_INVALID`: chuyển sang error tương ứng.

---

## 5. Khi nào user lấy DLG gửi admin/marketing?

User lấy DLG trong các case sau:

### Case A: Sau `/vclaw/claim` trả pending

Source DLG:

```text
/vclaw/claim response: data.device_login_code
```

Client show DLG đó ngay trên pending screen.

### Case B: App restart khi đang pending

Source DLG:

```text
local saved device_login_code
```

Nếu local mất DLG nhưng còn `device_hash`, client gọi lại `/vclaw/claim` với cùng `device_hash` để backend resume và trả lại:

```text
data.device_login_code
```

### Case C: `/auth/invite-login` trả `DEVICE_ACTIVATION_PENDING`

Source DLG:

```text
request invitation_code đang submit
```

Không đọc DLG từ response của `/auth/invite-login` vì API này không trả DLG trong lỗi pending.

---

## 6. Những điều client không được làm

- Không mở OpenClaw khi `device_activation_status === "pending_activation"`.
- Không mở OpenClaw khi `/auth/invite-login` trả `DEVICE_ACTIVATION_PENDING`.
- Không coi `DEVICE_ACTIVATION_PENDING` là sai mã.
- Không xóa DLG/local pending state khi pending.
- Không tạo DLG ở client.
- Không dùng DCL làm code gửi admin/marketing active device.
- Không dùng `data.status` của `/vclaw/claim` để quyết định active/pending vì field đó đang là `"ok"` khi success.
- Không gửi request flat device fields cho `/vclaw/claim`; backend yêu cầu object `device` nested.

---

## 7. Acceptance criteria cho client V-Claw

1. Client gọi `/api/v1/vclaw/claim` với JSON có nested `device` object.
2. Client lấy DLG từ `/vclaw/claim` response field `data.device_login_code`.
3. Client lấy trạng thái activation từ `/vclaw/claim` response field `data.device_activation_status`.
4. Nếu `device_activation_status === "active"`, client gọi `/api/v1/auth/invite-login` với `invitation_code = DLG`.
5. Nếu `device_activation_status === "pending_activation"`, client không login tiếp và show DLG cho user gửi admin/marketing.
6. Nếu `/auth/invite-login` success, client lưu token và mở OpenClaw.
7. Nếu `/auth/invite-login` trả `reason === "DEVICE_ACTIVATION_PENDING"`, client show pending screen bằng DLG đang submit trong request.
8. Client phân biệt rõ pending với `DEVICE_REVOKED`, `DEVICE_MISMATCH`, `INVITATION_CODE_INVALID`.
9. Retry sau khi admin/marketing active dùng lại cùng DLG và cùng `device_hash`.

---

## 8. Quick examples

### Pending ngay sau claim

Request:

```json
{
  "claim_code": "DCL-AAAA-BBBB-CCCC",
  "aff_code": "ABC",
  "device": {
    "device_hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "fingerprint_version": 1,
    "install_id": "install-1",
    "platform": "windows",
    "arch": "x64",
    "app_version": "1.0.0"
  }
}
```

Response:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "ok",
    "mode": "first_claim",
    "user_id": 123,
    "device_login_code": "DLG-WXYZ-2345-ABCD",
    "device_binding_id": 456,
    "device_activation_status": "pending_activation",
    "claimed_at": "2026-05-09T08:00:00Z"
  }
}
```

Client result:

```text
Show pending screen and DLG-WXYZ-2345-ABCD.
Do not open OpenClaw.
```

### Active after admin/marketing active, retry login

Request:

```json
{
  "invitation_code": "DLG-WXYZ-2345-ABCD",
  "device_hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "install_id": "install-1",
  "client_kind": "desktop"
}
```

Response:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "eyJ...",
    "refresh_token": "rt_...",
    "expires_in": 86400,
    "token_type": "Bearer",
    "user": {
      "id": 123,
      "email": "invite-example@invite-login.invalid",
      "role": "user",
      "status": "active"
    },
    "bootstrap_api_keys": []
  }
}
```

Client result:

```text
Save auth data and open OpenClaw.
```

### Still pending on invite-login

Request:

```json
{
  "invitation_code": "DLG-WXYZ-2345-ABCD",
  "device_hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "install_id": "install-1",
  "client_kind": "desktop"
}
```

Response:

```json
{
  "code": 403,
  "message": "device activation is pending admin approval",
  "reason": "DEVICE_ACTIVATION_PENDING"
}
```

Client result:

```text
Show pending screen and DLG-WXYZ-2345-ABCD from request invitation_code.
Do not open OpenClaw.
```
