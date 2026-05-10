# V-Claw Client Handoff — Device Activation Status Fix

Tài liệu này là handoff sửa client V-Claw/OpenClaw sau khi backend sub2api đã refactor app/device activation sang **status-only contract**.

Mục tiêu của client fix:

- Không đọc `device_activation_status` nữa.
- Không đọc/ghi `device_active` nữa.
- Không dựa vào `has_device_binding` để quyết định mở OpenClaw.
- Dùng duy nhất `data.status` từ `POST /api/v1/vclaw/claim` để quyết định `active` hay `pending_activation`.
- Chỉ mở OpenClaw sau khi `POST /api/v1/auth/invite-login` trả token thành công.

Nguồn backend hiện tại:

- `backend/internal/handler/vclaw_handler.go`
- `backend/internal/service/vclaw_claim_service.go`
- `backend/internal/handler/auth_handler.go`
- `backend/internal/service/auth_service.go`
- `backend/internal/service/auth_invite_device_helper.go`
- `backend/internal/service/user_device.go`

---

## 1. Breaking change so với handoff cũ

### Bỏ contract cũ

Client phải xóa mọi logic dạng:

```ts
claim.data.device_activation_status
claim.data.deviceActive
claim.data.device_active
claim.data.deviceActivation
claim.data.status === 'ok'
```

Các field trên không còn là source of truth cho client.

### Contract mới

`POST /api/v1/vclaw/claim` trả:

```ts
type VClawClaimData = {
  status: 'active' | 'pending_activation'
  mode: 'first_claim' | 'resume'
  user_id?: number
  device_login_code?: string
  device_binding_id?: number
  claimed_at?: string
}
```

Client đọc trạng thái activation tại:

```ts
claimResponse.data.status
```

Giá trị client cần xử lý:

| `data.status` | Ý nghĩa với client | Hành động |
|---|---|---|
| `active` | Device/app đã được active | Gọi tiếp `/api/v1/auth/invite-login` bằng DLG |
| `pending_activation` | Device/app đang chờ admin/marketing active | Show pending screen + DLG, không mở OpenClaw |

---

## 2. API 1 — Claim/resume device

```http
POST /api/v1/vclaw/claim
Content-Type: application/json
```

### Request body

Client giữ request nested `device` như hiện tại:

```json
{
  "claim_code": "DCL-AAAA-BBBB-CCCC",
  "aff_code": "AFF_CODE_IF_ANY",
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

Field rules client cần giữ:

- `device.device_hash` là required, 64-char lowercase/uppercase hex đều được backend normalize.
- `device.fingerprint_version` phải lớn hơn `0`.
- `device.platform` required.
- `device.arch` required.
- `device.install_id` optional; gửi nếu có, nhưng client không dùng nó thay thế `device_hash`.
- `claim_code` optional theo backend; nếu có DCL thì gửi `claim_code`.
- `aff_code` optional; gửi nếu installer/app có campaign/affiliate code.

### Active response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "active",
    "mode": "first_claim",
    "user_id": 123,
    "device_login_code": "DLG-AAAA-BBBB-CCCC",
    "device_binding_id": 456,
    "claimed_at": "2026-05-09T08:00:00Z"
  }
}
```

Client behavior:

1. Read `data.status`.
2. Read DLG from `data.device_login_code`.
3. If `data.status === 'active'`, call `/api/v1/auth/invite-login` with that DLG.
4. Do **not** open OpenClaw yet; wait for invite-login token success.

### Pending response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "pending_activation",
    "mode": "first_claim",
    "user_id": 123,
    "device_login_code": "DLG-AAAA-BBBB-CCCC",
    "device_binding_id": 456,
    "claimed_at": "2026-05-09T08:00:00Z"
  }
}
```

Client behavior:

1. Read `data.status`.
2. Read DLG from `data.device_login_code`.
3. Persist pending state locally:
   - `device_hash`
   - `install_id` if available
   - `device_login_code`
   - `status: 'pending_activation'`
4. Show pending activation screen.
5. Show/copy DLG code for user to send admin/marketing.
6. Do **not** call invite-login automatically in this branch.
7. Do **not** open OpenClaw.

Suggested UI copy:

```text
Thiết bị của bạn đang chờ admin/marketing kích hoạt.
Vui lòng gửi mã này cho admin/marketing:
DLG-AAAA-BBBB-CCCC

Sau khi được kích hoạt, bấm Thử lại.
```

### Resume response

When the same `device_hash` calls `/vclaw/claim` again, backend resumes the existing device binding and returns the same DLG.

Pending resume:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "pending_activation",
    "mode": "resume",
    "user_id": 123,
    "device_login_code": "DLG-AAAA-BBBB-CCCC",
    "device_binding_id": 456,
    "claimed_at": "2026-05-09T08:05:00Z"
  }
}
```

Active resume:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "active",
    "mode": "resume",
    "user_id": 123,
    "device_login_code": "DLG-AAAA-BBBB-CCCC",
    "device_binding_id": 456,
    "claimed_at": "2026-05-09T08:05:00Z"
  }
}
```

Client behavior on resume:

- `pending_activation` -> keep pending screen and show DLG.
- `active` -> call invite-login using returned DLG.

---

## 3. API 2 — Login with DLG

```http
POST /api/v1/auth/invite-login
Content-Type: application/json
```

### Request body

```json
{
  "invitation_code": "DLG-AAAA-BBBB-CCCC",
  "device_hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "install_id": "optional-install-id",
  "client_kind": "desktop",
  "turnstile_token": "optional-turnstile-token"
}
```

Rules:

- `invitation_code` is the DLG from `/vclaw/claim` `data.device_login_code`.
- `device_hash` is required for desktop client.
- `device_hash` must match the binding behind the DLG.
- `install_id` is optional; backend does not block a matching `device_hash` only because install ID rotated/missing.
- `client_kind` for V-Claw desktop should be `desktop` or the existing non-`web` desktop value.
- `turnstile_token` should be passed exactly like current client behavior when required by environment.

### Success response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "***",
    "refresh_token": "***",
    "expires_in": 86400,
    "token_type": "Bearer",
    "user": {
      "id": 123,
      "email": "invite-example@invite-login.invalid",
      "username": "invite-example",
      "role": "user",
      "status": "active"
    },
    "bootstrap_api_keys": []
  }
}
```

Client behavior:

1. Save `access_token`, `refresh_token`, `expires_in`, `token_type`.
2. Save/use `bootstrap_api_keys` if OpenClaw needs them.
3. Mark local device state active.
4. Open OpenClaw.

### Pending error response

If DLG exists but the app/device is still waiting for admin/marketing activation:

```json
{
  "code": 403,
  "message": "device activation is pending admin approval",
  "reason": "DEVICE_ACTIVATION_PENDING"
}
```

Important:

- `/auth/invite-login` does not return a new DLG in this error response.
- Client must reuse the DLG it submitted in request `invitation_code`.

Client behavior:

1. If `reason === 'DEVICE_ACTIVATION_PENDING'`:
   - Do not open OpenClaw.
   - Do not delete DLG.
   - Do not show “invalid code”.
   - Save/keep pending state.
   - Show pending screen using request `invitation_code` as the displayed DLG.
2. User sends that DLG to admin/marketing.
3. After admin/marketing updates user `status` to `active`, user clicks Retry.
4. Retry calls `/auth/invite-login` again with the same DLG + same `device_hash`.

---

## 4. Error mapping client must keep separate

These errors are not pending activation and must not show the pending screen.

| `reason` | Meaning | Client behavior |
|---|---|---|
| `INVITATION_CODE_REQUIRED` | Missing DLG/invitation code | Ask client/user to provide code or repair local state |
| `INVITATION_CODE_INVALID` | DLG invalid/wrong type/not found | Show invalid code / restart claim flow |
| `DEVICE_HASH_REQUIRED` | Desktop request missing `device_hash` | Repair fingerprint/device state |
| `DEVICE_HASH_INVALID` | `device_hash` is not 64-char hex | Treat as fingerprint bug; no infinite retry |
| `DEVICE_MISMATCH` | DLG belongs to another device hash | Show device mismatch; do not show pending |
| `DEVICE_REVOKED` | Binding is revoked/blocked/invalid state | Show revoked/blocked/support message; do not show pending |
| `DEVICE_ACTIVATION_PENDING` | Correct DLG/device but not active yet | Show pending + DLG |

---

## 5. Client implementation sketch

```ts
type ClaimStatus = 'active' | 'pending_activation'
type ClaimMode = 'first_claim' | 'resume'

type VClawClaimData = {
  status: ClaimStatus
  mode: ClaimMode
  user_id?: number
  device_login_code?: string
  device_binding_id?: number
  claimed_at?: string
}

async function claimAndContinue(input: {
  claimCode?: string
  affCode?: string
  deviceHash: string
  fingerprintVersion: number
  installId?: string
  platform: string
  arch: string
  appVersion?: string
}) {
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

  const data = claim.data as VClawClaimData
  const dlg = data.device_login_code

  if (!dlg) {
    showFatalError('Server did not return device login code.')
    return
  }

  saveDeviceLoginCode(dlg)

  switch (data.status) {
    case 'pending_activation':
      savePendingActivationState({
        deviceHash: input.deviceHash,
        installId: input.installId,
        deviceLoginCode: dlg,
      })
      showPendingActivationScreen(dlg)
      return

    case 'active':
      await loginWithDlg({
        dlg,
        deviceHash: input.deviceHash,
        installId: input.installId,
      })
      return

    default:
      showFatalError(`Unsupported activation status: ${String(data.status)}`)
  }
}

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

---

## 6. Migration checklist for client PR

- [ ] Remove reads of `data.device_activation_status`.
- [ ] Remove reads of `device_active` / `deviceActive` / `deviceActivation`.
- [ ] Remove logic that treats `data.status === 'ok'` as claim success state.
- [ ] Type claim response `status` as `'active' | 'pending_activation'`.
- [ ] Read DLG only from `/vclaw/claim` `data.device_login_code` or local saved pending state.
- [ ] Pending claim must show DLG and must not call invite-login automatically.
- [ ] Active claim must call invite-login and must not open OpenClaw before token success.
- [ ] Invite-login pending error must reuse request `invitation_code` as displayed DLG.
- [ ] `DEVICE_MISMATCH`, `DEVICE_REVOKED`, `INVITATION_CODE_INVALID` must not be shown as pending activation.
- [ ] App restart while pending should restore local pending DLG or call `/vclaw/claim` again with same `device_hash` to resume and recover DLG.
- [ ] Retry after admin/marketing activation should call `/auth/invite-login` with the same DLG + same `device_hash`.

---

## 7. Acceptance criteria

Client fix is complete when these scenarios work:

1. **First claim returns active**
   - Client reads `data.status === 'active'`.
   - Client reads `data.device_login_code`.
   - Client calls `/auth/invite-login`.
   - Client opens OpenClaw only after token success.

2. **First claim returns pending**
   - Client reads `data.status === 'pending_activation'`.
   - Client shows pending screen with DLG from `data.device_login_code`.
   - Client does not call invite-login automatically.
   - Client does not open OpenClaw.

3. **App restart while pending**
   - Client shows saved DLG if local pending state exists.
   - If DLG is missing but `device_hash` exists, client calls `/vclaw/claim` to resume and recover `data.device_login_code`.

4. **Retry before activation**
   - `/auth/invite-login` returns `DEVICE_ACTIVATION_PENDING`.
   - Client keeps pending screen and displays request `invitation_code`.

5. **Retry after admin/marketing activation**
   - Same DLG + same `device_hash` succeeds on `/auth/invite-login`.
   - Client saves auth data and opens OpenClaw.

6. **Invalid/revoked/mismatch cases**
   - Client shows dedicated error state.
   - Client does not show pending activation for those cases.

---

## 8. Notes for handoff consumers

- The DLG code is the user-shareable activation reference.
- The client does not need any backend setting name to implement this flow.
- The client should not infer activation from admin UI fields.
- The client should not depend on account-management fields such as `has_device_binding`.
- The only client decision field from claim is `data.status`.
