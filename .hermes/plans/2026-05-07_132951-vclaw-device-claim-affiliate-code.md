# V-Claw Device Claim Affiliate Code Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Phân tích theo contract affiliate/invitation đang chạy trước khi chốt implementation. Kết quả kiểm tra hiện tại cho thấy hệ thống sub2api đang dùng affiliate code field canonical là `aff_code` ở register/OAuth/API type/service; V-Claw device first-claim hiện chưa truyền field affiliate nào. Nếu triển khai, bản cài đặt V-Claw chỉ được gửi affiliate/referral code theo contract sẵn có của hệ thống (`aff_code`) trong lần đăng ký thiết bị mới (`POST /api/v1/vclaw/claim`) để bind inviter cho user bootstrap được tạo từ first device claim; login/resume bằng thiết bị đã có không được bind lại hoặc đổi inviter.

**Architecture:** Không tự định nghĩa tên field mới. Tên field phải theo evidence trong code hiện có: `aff_code` là JSON/API field cho affiliate attribution; `invitation_code` là redeem/invitation/login code; `claim_code` là device-claim redeem code. Không overload `claim_code` và không gửi affiliate attribution vào `/api/v1/auth/invite-login`. Nếu thêm wiring cho V-Claw, thêm top-level `aff_code` vào request `/api/v1/vclaw/claim`, thread qua handler/service, inject `AffiliateService` vào `VClawClaimService`, và chỉ bind inviter trong nhánh `createFirstClaim(...)`. Giữ `/auth/invite-login` chỉ tiêu thụ `DLG-*` device-login code để authenticate thiết bị đã bound.

**Tech Stack:** Go backend (`gin`, service/repository pattern, Wire DI, Go tests), Electron/V-Claw app JavaScript for client request wiring, existing sub2api affiliate service.

---

## Context / current finding

### Backend hiện có

- Endpoint đăng ký/claim thiết bị: `POST /api/v1/vclaw/claim`.
  - Handler: `backend/internal/handler/vclaw_handler.go`.
  - Service: `backend/internal/service/vclaw_claim_service.go`.
  - Current request body mới có:
    - `claim_code`
    - `device.device_hash`
    - `device.fingerprint_version`
    - `device.install_id`
    - `device.platform`
    - `device.arch`
    - `device.app_version`
- `VClawClaimService.Claim(...)` hiện resume bằng `device_hash` trước:
  - Nếu device đã bound: trả lại `device_login_code`, mode `resume`.
  - Nếu chưa bound và `claim_code` rỗng: auto-first-claim, tạo bootstrap user mới.
  - Nếu chưa bound và có `claim_code`: code phải là `RedeemTypeDeviceClaim`, rồi tạo bootstrap user mới.
- `createFirstClaim(...)` hiện tạo user, bonus/default subscription, tạo `DLG-*`, tạo `user_devices` binding.
- Endpoint login bằng device code: `POST /api/v1/auth/invite-login`.
  - Handler: `backend/internal/handler/auth_handler.go`.
  - Service path: `AuthService.InviteLogin(...)` -> `completeDeviceInviteLogin(...)` khi code type là `RedeemTypeDeviceLogin`.
  - Current request body: `invitation_code`, `device_hash`, `install_id`, `client_kind`, `turnstile_token`.
  - Đây là login bằng `DLG-*`, không phải nơi bind affiliate.

### Kết luận sản phẩm

- Hiện backend đã có đăng ký thiết bị mới và login/resume bằng device binding.
- Hiện chưa có field canonical để packaged installer gửi affiliate code lên `/vclaw/claim`.
- Cần triển khai `aff_code` ở `/vclaw/claim`.
- Không dùng `claim_code` cho affiliate vì `claim_code` đang là device-claim redeem code (`DCL-*`/device claim lifecycle).
- Không dùng `/auth/invite-login` để bind affiliate vì endpoint này chạy nhiều lần và phải idempotent.

---

## Contract sau khi triển khai

### First install có affiliate code

```http
POST /api/v1/vclaw/claim
Content-Type: application/json
```

```json
{
  "aff_code": "AFF-INVITER-123",
  "device": {
    "device_hash": "<64 lowercase hex chars>",
    "fingerprint_version": 1,
    "install_id": "<stable install id>",
    "platform": "windows",
    "arch": "x64",
    "app_version": "1.0.0"
  }
}
```

Expected:
- Tạo user bootstrap mới.
- Tạo/bảo đảm affiliate profile cho user mới.
- Bind user mới với inviter tương ứng `aff_code`, nếu code hợp lệ và affiliate feature enabled.
- Trả `device_login_code` dạng `DLG-*`.

### First install có cả device claim code + affiliate code

```json
{
  "claim_code": "DCL-....",
  "aff_code": "AFF-INVITER-123",
  "device": { "...": "..." }
}
```

Expected:
- `claim_code` vẫn chỉ xử lý device claim redeem code.
- `aff_code` chỉ xử lý affiliate attribution.

### Login/resume sau đó

```http
POST /api/v1/auth/invite-login
Content-Type: application/json
```

```json
{
  "invitation_code": "DLG-....",
  "device_hash": "<same 64 lowercase hex chars>",
  "install_id": "<same install id>",
  "client_kind": "desktop"
}
```

Expected:
- Chỉ authenticate device/user và cấp token/bootstrap API keys.
- Không bind affiliate, không đổi inviter, không credit thêm invite count.

---

## Files likely to change

Backend:
- Modify: `backend/internal/handler/vclaw_handler.go`
- Modify: `backend/internal/service/vclaw_claim_service.go`
- Modify: `backend/internal/service/wire.go`
- Generated update likely needed if repo uses Wire output file:
  - Search/inspect for `wire_gen.go` before coding.
- Modify tests:
  - `backend/internal/service/vclaw_claim_service_test.go`
  - Maybe handler-level tests if existing nearby handler tests cover request DTOs.

V-Claw app/client:
- Modify likely request builder(s), exact file to confirm during implementation:
  - `/root/projects/V-Claw/v-claw-app/src/main/services/invite-flow.js`
  - `/root/projects/V-Claw/v-claw-app/src/main/services/onboarding/auth/invite-flow.js`
  - Relevant tests: `/root/projects/V-Claw/v-claw-app/test/invite-flow.test.js`

Docs/handoff if needed:
- Optional: add/update internal docs or release notes describing `aff_code` contract.

---

## Step-by-step plan

### Task 1: Confirm repo conventions and Wire shape

**Objective:** Make sure implementation follows local structure and identifies generated DI files.

**Files:**
- Read/search only.

**Steps:**
1. Check backend repo instructions/configs:
   - Search for `AGENTS.md`, `CLAUDE.md`, `HERMES.md`, `.hermes.md`, `.editorconfig`.
   - Current discovery: none at `/root/projects/sub2api` root, but re-check before implementation.
2. Search Wire generated files:
   - `search_files(pattern="*wire*.go", target="files", path="/root/projects/sub2api/backend")`.
3. Read Makefile targets:
   - `/root/projects/sub2api/Makefile`
   - `/root/projects/sub2api/backend/Makefile`
4. Verify current branch/dirty state:
   - `git status --short --branch`

**Verification:**
- No code changes yet.
- Know exact command for backend tests and whether Wire generation is required.

---

### Task 2: Add RED service test for first claim with valid `aff_code`

**Objective:** Prove first device claim should bind affiliate inviter once.

**Files:**
- Modify test: `backend/internal/service/vclaw_claim_service_test.go`

**Test intent:**
- Arrange:
  - Mock/create inviter affiliate profile with `aff_code`.
  - New device hash not bound.
  - `VClawClaimRequest{AffCode: "...", Device: ...}`.
  - Inject mock `AffiliateService` dependency or repository-backed service depending current test structure.
- Act:
  - `svc.Claim(ctx, req)`.
- Assert:
  - result mode is `first_claim`.
  - affiliate bind path called for newly created user and provided `aff_code`.
  - no error.

**Expected RED:**
- Test fails because `VClawClaimRequest` has no `AffCode`, constructor has no affiliate dependency, and claim path does not bind affiliate.

**Command:**
```bash
cd /root/projects/sub2api/backend
go test ./internal/service -run 'TestVClawClaimService.*Affiliate|TestVClawClaimServiceFirstClaim' -count=1
```

---

### Task 3: Add RED service test for resume with different `aff_code` does not rebind

**Objective:** Preserve idempotency: existing device resume must not change inviter or count again.

**Files:**
- Modify test: `backend/internal/service/vclaw_claim_service_test.go`

**Test intent:**
- Arrange existing active `user_devices` binding for `device_hash` with `login_redeem_code_id`.
- Call `Claim(...)` with same `device_hash` and a different `AffCode`.
- Assert:
  - result mode is `resume`.
  - no affiliate bind call.
  - returned `device_login_code` is existing DLG code.

**Expected RED:**
- Initially may not compile until `AffCode` exists; after Task 4 implementation it must pass.

**Command:**
```bash
cd /root/projects/sub2api/backend
go test ./internal/service -run 'TestVClawClaimService.*Affiliate|TestVClawClaimServiceResumes' -count=1
```

---

### Task 4: Implement backend service contract minimally

**Objective:** Add `AffCode` to claim request and bind affiliate only during first claim.

**Files:**
- Modify: `backend/internal/service/vclaw_claim_service.go`

**Implementation details:**
1. Extend `VClawClaimService` struct:
   - Add `affiliateService *AffiliateService`.
2. Extend constructor `NewVClawClaimService(...)`:
   - Add `affiliateService *AffiliateService` parameter.
   - Store it.
3. Extend `VClawClaimRequest`:
   - Add `AffCode string`.
4. In `createFirstClaim(...)`, after user is successfully created and before return:
   - Call helper like `s.bindDeviceClaimAffiliate(runCtx, user.ID, req.AffCode)`.
5. Add helper with behavior matching existing registration/OAuth convention:
   - If `s.affiliateService == nil` or `userID <= 0`: no-op.
   - Always attempt `EnsureUserAffiliate(ctx, userID)`; log error, do not block.
   - If trimmed `aff_code` is non-empty: call `BindInviterByCode(ctx, userID, code)`; log error, do not block.
   - Consider checking `s.affiliateService.IsEnabled(ctx)` if `AffiliateService` semantics require it; mirror existing registration behavior as much as possible.
6. Do not touch `resumeExistingClaim(...)`.
7. Do not touch `/auth/invite-login` affiliate behavior.

**Expected GREEN:**
- Service tests from Tasks 2-3 pass.

**Command:**
```bash
cd /root/projects/sub2api/backend
go test ./internal/service -run 'TestVClawClaimService.*Affiliate|TestVClawClaimServiceResumes|TestVClawClaimServiceFirstClaim' -count=1
```

---

### Task 5: Add handler DTO support for top-level `aff_code`

**Objective:** Public API accepts installer affiliate code on `/api/v1/vclaw/claim`.

**Files:**
- Modify: `backend/internal/handler/vclaw_handler.go`
- Test if existing handler tests are present; otherwise service coverage plus compile is acceptable, but prefer adding handler test if simple.

**Implementation details:**
1. Add to handler request struct:
   - `AffCode string `json:"aff_code"``
2. Pass into service request:
   - `AffCode: req.AffCode,`
3. Do not add aliases like `invite_code` unless explicitly requested; keep canonical `aff_code`.

**Verification:**
```bash
cd /root/projects/sub2api/backend
go test ./internal/handler -run 'VClaw|InviteLogin|Auth' -count=1
```

If no matching tests exist, run package compile tests:
```bash
cd /root/projects/sub2api/backend
go test ./internal/handler -count=1
```

---

### Task 6: Update Wire/DI and constructor callers

**Objective:** Ensure production service gets `AffiliateService`, not only tests.

**Files:**
- Modify: `backend/internal/service/wire.go`
- Modify generated Wire output if present, or run Wire generation per repo pattern.
- Modify any direct test constructor calls in `backend/internal/service/vclaw_claim_service_test.go`.

**Implementation details:**
1. Since `ProviderSet` already includes `NewAffiliateService` before `NewVClawClaimService`, adding `*AffiliateService` constructor param should be resolvable.
2. Run search:
   - `NewVClawClaimService(` across backend.
3. Update all call sites with either a mock/nil affiliate service for tests.
4. If generated wire file exists, regenerate using repo command (likely `wire` target or `go generate`). Do not hand-edit generated file unless repo already does so.

**Verification:**
```bash
cd /root/projects/sub2api/backend
go test ./internal/service -run 'TestVClawClaimService' -count=1
```

---

### Task 7: Add RED/GREEN backend test for invalid `aff_code` non-blocking

**Objective:** Packaged install should not fail completely if affiliate code is stale/invalid, matching existing registration behavior where affiliate bind failures are logged only.

**Files:**
- Modify: `backend/internal/service/vclaw_claim_service_test.go`

**Test intent:**
- Arrange affiliate service mock returning error from `BindInviterByCode`.
- Call first claim with `AffCode`.
- Assert claim still succeeds and device binding/DLG are created.

**Command:**
```bash
cd /root/projects/sub2api/backend
go test ./internal/service -run 'TestVClawClaimService.*Affiliate' -count=1
```

---

### Task 8: Wire V-Claw client to send packaged `aff_code`

**Objective:** The desktop app sends installer/referral code to backend on initial `/vclaw/claim` request.

**Files:**
- Inspect and likely modify:
  - `/root/projects/V-Claw/v-claw-app/src/main/services/invite-flow.js`
  - `/root/projects/V-Claw/v-claw-app/src/main/services/onboarding/auth/invite-flow.js`
  - `/root/projects/V-Claw/v-claw-app/test/invite-flow.test.js`

**Implementation design:**
1. Identify current place where app builds body for `POST /api/v1/vclaw/claim`.
2. Identify current stored installer invite/referral value name; likely existing `inviteCode` / saved state from onboarding. Confirm before editing.
3. Add only to claim request body:
   - top-level `aff_code: <packaged affiliate code>` when present.
4. Do not send affiliate code to `/auth/invite-login`.
5. Ensure existing `claim_code` logic remains for device claim code only.

**Tests:**
- Add/adjust test that claim request body contains `aff_code` on first device claim when installer code exists.
- Add/adjust test that invite-login request body does not include `aff_code` and still uses `invitation_code: DLG-*`.

**Commands:**
```bash
cd /root/projects/V-Claw/v-claw-app
npm test -- invite-flow.test.js
```

Adapt command to repo package manager after reading `package.json`.

---

### Task 9: Run focused backend verification

**Objective:** Prove backend contract and regression-sensitive auth/device flows still work.

**Commands:**
```bash
cd /root/projects/sub2api/backend
go test ./internal/service -run 'TestVClawClaimService|TestAuthService.*InviteLogin|Test.*InviteLogin' -count=1
go test ./internal/handler -run 'VClaw|InviteLogin|Auth' -count=1
```

If package regex misses tests, run the full packages:
```bash
cd /root/projects/sub2api/backend
go test ./internal/service ./internal/handler -count=1
```

---

### Task 10: Run broader verification

**Objective:** Catch compile/DI/API regressions beyond focused tests.

**Commands:**
```bash
cd /root/projects/sub2api/backend
go test ./... -count=1
```

If repo uses unit tag per Makefile, also run:
```bash
cd /root/projects/sub2api/backend
go test -tags=unit ./... -count=1
```

For V-Claw app after client changes:
```bash
cd /root/projects/V-Claw/v-claw-app
npm test -- invite-flow.test.js
```

If package scripts define lint/typecheck/build, run focused equivalent:
```bash
cd /root/projects/V-Claw/v-claw-app
npm run lint
npm run test
```

Only run heavier build if dependencies are installed and repo instructions allow.

---

### Task 11: Document usage / handoff

**Objective:** Give V-Claw integration guidance after code lands.

**Content to report:**
- New request field: top-level `aff_code` on `POST /api/v1/vclaw/claim`.
- `claim_code` remains device-claim code only.
- `/api/v1/auth/invite-login` still sends `invitation_code: DLG-*`, `device_hash`, `install_id`, `client_kind: desktop`; no `aff_code` there.
- Affiliate bind is first-claim only; resume/login idempotent.

---

## Risks / tradeoffs

1. **Affiliate failure behavior:** Existing registration logs affiliate bind failures and does not block signup. Plan follows that. If product wants invalid installer affiliate code to hard-fail installation, that must be explicitly changed.
2. **Naming ambiguity:** User says “invite code” but backend has several code types. Contract should use `aff_code` for affiliate attribution, `claim_code` for device claim redeem, `invitation_code` for login/redeem endpoints.
3. **Idempotency:** Must never bind/change inviter on resume or `/auth/invite-login`, otherwise repeated app starts could corrupt attribution/counts.
4. **Wire generation:** Constructor signature change may require generated Wire output refresh.
5. **V-Claw client source of installer code:** Need confirm exact variable/config source for packaged code before editing app side.

---

## Acceptance criteria

- `POST /api/v1/vclaw/claim` accepts top-level `aff_code`.
- First device claim with valid `aff_code` binds invitee user to inviter.
- First device claim without `aff_code` still works exactly as before.
- Existing device resume ignores any supplied `aff_code` and returns existing `DLG-*`.
- `/api/v1/auth/invite-login` behavior unchanged and does not bind affiliate.
- Tests cover affiliate bind, resume non-rebind, invalid aff non-blocking, and unchanged claim-code semantics.
- Focused backend tests pass; broader backend compile/tests pass as practical.
- V-Claw request body sends `aff_code` only on claim and not on invite-login.
