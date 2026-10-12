# API Media Capability Use Key Hướng dẫn — Implementation Plan

**Goal:** Implement backend capability resolver endpoint `GET /api/v1/user/api-keys/:id/capabilities`, wire frontend Use Key modal với dynamic API tab, và deliver focused tests backing evidence-driven media capability disclosure cho từng API key.

**Architecture:** Backend resolver tái sử dụng `ListModelAvailabilityCandidates`, account/group/scheduler logic hiện có, không tạo riêng scheduler replica; trả safe unavailable khi resolver/repo/provider error. Frontend `UseKeyModal.vue` fetch capability bằng numeric key ID (không dùng secret), render dynamic API tab cho từng surface (image sync/async, Gemini native, batch, video, Seedance) chỉ khi backend xác nhận available. Response contract không để lộ account ID, upstream URL, provider credential hay internal field; reason là closed enum.

**Tech Stack:** Go 1.22+ backend, Vue 3 frontend, existing Gin routing/middleware/handler pattern, existing PostgreSQL account/group/mapping schema, existing service/repository abstractions. Không cần migration; design đọc existing groups/accounts/mappings/features. Frontend dùng composition API, TypeScript strict, i18n leaf-key parity EN/VI, accessibility (`role`/`aria` cho dynamic tab).

**Plan ID:** `api-media-capability-implementation-2026-10-10`

---

## Global Constraints

- **Bảo toàn worktree gốc `/home/ubuntu/workspaces/sub2api`:** không reset, checkout, hoặc xoá staged deletions (`~1,857` staged deletions).
- **Làm việc trong analysis worktree `/home/ubuntu/workspaces/sub2api-origin-main`** tại SHA `dee5643df26b576a5133735008d219f47714497c`; draft plan/PRD đã untracked ở đây.
- **Tái sử dụng predicate hiện có:** `ListModelAvailabilityCandidates`, `IsModelSupported`, account/provider/model mapping, scheduler eligibility, group allowlist, feature flag, endpoint capability. Không tạo keyword model heuristic riêng.
- **Phân biệt gate/account/provider/model/route:** không suy capability từ group name, group platform alone, model keyword, hay route existence mà không kiểm account/mapping/scheduler chain.
- **Safe fail-closed:** resolver/repository/provider error trả `available=false` với `account_capability_unverified` hoặc safe reason, không optimistic true.
- **Response contract:** không trả account ID, account name, provider credential, `base_url`, upstream mapped target, billing detail, raw error. Reason là closed enum: `no_group`, `group_disabled`, `platform_unsupported`, `feature_disabled`, `no_account`, `model_not_available`, `account_capability_unverified`, `route_unsupported`.
- **Không lưu credential values:** mọi credential nếu xuất hiện phải thay bằng `[REDACTED]`.
- **Frontend fetch by key ID:** không dùng API key secret trong URL/query/body capability request; JWT user route xác minh ownership.
- **Locale parity:** mọi leaf key EN phải có VI; chạy `cd frontend && pnpm run check:i18n` pass.
- **Không đổi runtime group/account/mapping/feature trong task này.**
- **Không coi plan approval là rollout approval.**

---

## File Structure

Backend:
- `backend/internal/service/api_key_capabilities.go` — resolver service, public reason enum, route-specific predicate per capability surface.
- `backend/internal/service/api_key_capabilities_test.go` — unit/table tests với fake repo/account/mapping.
- `backend/internal/handler/api_key_capabilities_handler.go` — authenticated user handler `GET /api/v1/user/api-keys/:id/capabilities`.
- `backend/internal/handler/api_key_capabilities_handler_test.go` — handler auth/ownership/serialization tests.
- `backend/internal/handler/wire.go` — thêm `NewAPIKeyCapabilitiesHandler` vào wire set.
- `backend/internal/handler/handler.go` — thêm `APIKeyCapabilities *APIKeyCapabilitiesHandler` vào `Handlers`.
- `backend/internal/server/routes/user.go` — đăng ký `GET /api/keys/:id/capabilities` trong authenticated user group.

Frontend:
- `frontend/src/api/keys.ts` — thêm `getCapabilities(id: number)` và DTO types `ApiKeyCapabilities`.
- `frontend/src/components/keys/UseKeyModal.vue` — thêm `apiKeyId` prop, capability loading/error state, dynamic API tab với sections (image/gemini_native/batch_image/video/seedance).
- `frontend/src/views/user/KeysView.vue` — pass `selectedKey.id` as `apiKeyId` prop.
- `frontend/src/i18n/locales/en.ts` — thêm leaf keys `keys.useKeyModal.api.*`.
- `frontend/src/i18n/locales/vi.ts` — thêm leaf keys `keys.useKeyModal.api.*` parity.
- Focused component/integration tests theo repo convention (nếu có `UseKeyModal.spec.ts` hoặc `keys.spec.ts` convention).

---

## Task 1: Backend capability resolver service skeleton

**Outcome:** Service `api_key_capabilities.go` với stub method `ResolveCapabilities(ctx, userID, keyID)` trả empty but well-typed response; reason enum; internal stub per-surface predicate placeholders.

**Files:**
- Create: `backend/internal/service/api_key_capabilities.go`
- Create: `backend/internal/service/api_key_capabilities_test.go`

**Interfaces:**
- Consumes: `context.Context`, user ID `int64`, API key ID `int64`, existing `APIKeyRepository.GetByID`, existing `GroupRepository`, existing `AccountRepository.ListModelAvailabilityCandidates`.
- Produces: `APIKeyCapabilities` struct với các field `object`, `key_id`, `group`, `image`, `batch_image`, `video`, `seedance`, `gemini_native`; nested available/reason per surface.
- Must preserve: không mutate scheduler state, quota, usage, task records; không bypass model allowlist.

**Constraints:**
- Tái sử dụng global constraints.
- Service method signature: `func (s *APIKeyCapabilitiesService) ResolveCapabilities(ctx context.Context, userID int64, keyID int64) (*APIKeyCapabilities, error)`.
- Internal per-surface stub methods: `resolveImageCapability`, `resolveBatchImageCapability`, `resolveVideoCapability`, `resolveSeedanceCapability`, `resolveGeminiNativeImageCapability` trả empty struct với `available=false`, `reason="account_capability_unverified"` placeholder.
- Reason enum constants: `ReasonNoGroup`, `ReasonGroupDisabled`, `ReasonPlatformUnsupported`, `ReasonFeatureDisabled`, `ReasonNoAccount`, `ReasonModelNotAvailable`, `ReasonAccountCapabilityUnverified`, `ReasonRouteUnsupported`.

- [x] **Step 1 — Establish RED**
  - Tạo `backend/internal/service/api_key_capabilities_test.go` với test case:
    ```go
    func TestResolveCapabilitiesReturnsStructuredResponseForMissingKey(t *testing.T) {
        svc := &APIKeyCapabilitiesService{/* fake repos */}
        caps, err := svc.ResolveCapabilities(context.Background(), 1, 999)
        require.NoError(t, err)
        require.Equal(t, "api_key_capabilities", caps.Object)
        require.Equal(t, int64(999), caps.KeyID)
        require.False(t, caps.Image.Available)
        require.Equal(t, ReasonNoGroup, caps.Image.Sync.Reason)
    }
    ```
  - Run: `cd backend && go test ./internal/service -run TestResolveCapabilitiesReturnsStructuredResponseForMissingKey`
  - Expected: fail vì `APIKeyCapabilitiesService` chưa tồn tại.

- [x] **Step 2 — Implement minimally**
  - Tạo `backend/internal/service/api_key_capabilities.go`:
    - `type APIKeyCapabilities struct` với JSON tags match response proposal.
    - `type APIKeyCapabilitiesService struct` với repos: `apiKeyRepo APIKeyRepository`, `groupRepo GroupRepository`, `accountRepo AccountRepository`.
    - `func NewAPIKeyCapabilitiesService(...) *APIKeyCapabilitiesService`.
    - `func (s *APIKeyCapabilitiesService) ResolveCapabilities(...)` đọc key/group từ repo; nếu key missing hoặc group nil trả all-unavailable với `reason=ReasonNoGroup`; otherwise stub call internal resolver methods (chưa implement logic thật) trả placeholder `available=false` với `reason=ReasonAccountCapabilityUnverified`.
    - Internal stub placeholders: `resolveImageCapability`, `resolveBatchImageCapability`, `resolveVideoCapability`, `resolveSeedanceCapability`, `resolveGeminiNativeImageCapability` trả struct với `available=false`.
    - Reason enum constants: `const ReasonNoGroup = "no_group"` etc.

- [x] **Step 3 — Verify GREEN**
  - Run: `cd backend && go test ./internal/service -run TestResolveCapabilitiesReturnsStructuredResponseForMissingKey`
  - Expected: pass với `caps.Image.Available == false`, `caps.Image.Sync.Reason == "no_group"`.

- [x] **Step 4 — Check nearby regressions**
  - Run: `cd backend && go test ./internal/service/...`
  - Expected: không có new failure.

- [x] **Step 5 — Self-review**
  - Kiểm tra `api_key_capabilities.go` không hardcode credential, không expose account ID/upstream field, reason values là string constants match closed enum.
  - Report: `backend/internal/service/api_key_capabilities.go` (resolver skeleton), `backend/internal/service/api_key_capabilities_test.go` (missing-key test), evidence: `go test ./internal/service` pass.

---

## Task 2: Backend handler và route registration

**Outcome:** Existing authenticated user API-key handler serves `GET /api/v1/user/api-keys/:id/capabilities`; JWT middleware supplies the user subject, ownership is checked before resolution, and the public capability JSON is serialized without credential/provider internals.

**Files:**
- Modify: `backend/internal/handler/api_key_handler.go` — add the authenticated capability method and narrow read seams.
- Create: `backend/internal/handler/api_key_capabilities_handler_test.go`
- Modify: `backend/internal/handler/wire.go` — inject the resolver into the existing API-key handler.
- Modify: `backend/internal/service/wire.go` and generated `backend/cmd/server/wire_gen.go` — provide the capability service from existing repositories.
- Modify: `backend/internal/server/routes/user.go` — register the route under the existing authenticated `/user` group.

**Interfaces:**
- Consumes: `gin.Context`, authenticated user ID từ middleware, `:id` param, `APIKeyCapabilitiesService`.
- Produces: HTTP 200 JSON `{"object":"api_key_capabilities", ...}` hoặc 401/403/404 error.
- Must preserve: không expose API key secret; không expose account/provider internals; ownership check trước khi resolve.

**Constraints:**
- Tái sử dụng global constraints.
- Handler signature: `func (h *APIKeyHandler) GetCapabilities(c *gin.Context)`.
- Auth middleware: existing `middleware2.GetAuthSubjectFromContext(c)`.
- Ownership check: `key.UserID != subject.UserID` trả 404 không phải 403 để tránh key ID enumeration.
- Response serialization không chứa field `credential`, `api_key`, `token`, `password`, `secret`, `base_url`, `account_id`, `upstream_target`.

- [x] **Step 1 — Establish RED**
  - Tạo `backend/internal/handler/api_key_capabilities_handler_test.go` với contract tests cho unauthenticated request, foreign-key ownership, successful resolver invocation, and public serialization.
  - Run: `cd backend && go test ./internal/handler -run 'TestAPIKeyHandlerGetCapabilities' -count=1`
  - Expected RED đã được ghi nhận trước khi handler implementation tồn tại.

- [x] **Step 2 — Implement minimally**
  - Mở rộng `APIKeyHandler` bằng narrow ownership/resolver seams, giữ nguyên constructor hiện hữu.
  - `GetCapabilities` parse `:id`, đọc auth subject, kiểm tra ownership trước resolve, trả 404 cho foreign key, và serialize capability response.
  - `ProvideAPIKeyHandler` injects `APIKeyCapabilitiesService`; route dùng authenticated `user.GET("/api-keys/:id/capabilities", h.APIKey.GetCapabilities)`.

- [x] **Step 3 — Verify GREEN**
  - Run: `cd backend && go test ./internal/handler -run 'TestAPIKeyHandlerGetCapabilities' -count=1`.
  - Expected: pass với 401 unauthenticated, 404 foreign key, 200 public contract.

- [x] **Step 4 — Check nearby regressions**
  - Run: `cd backend && go test ./internal/handler/... ./internal/server/routes/...`.
  - Run: `cd backend && go vet ./internal/handler/... ./internal/server/routes/... ./internal/service/...`.
  - Run: `cd backend && go build ./cmd/server`.

- [x] **Step 5 — Self-review**
  - Serialization test marshal JSON và recursively scan forbidden fields (`credential`, `api_key`, `token`, `password`, `secret`, `base_url`, `account_id`, `upstream_target`).
  - Wire regenerated via `go generate ./cmd/server`; generated output constructs the capability service from existing repositories.
  - Report: handler method in `api_key_handler.go`, route in `user.go`, DI in `handler/wire.go`, `service/wire.go`, and `cmd/server/wire_gen.go`; all bounded checks pass.

---

## Task 3: Image sync/async capability implementation

**Outcome:** Resolver `resolveImageCapability` kiểm tra image gate (group flag/feature), platform route OpenAI/Grok, account candidate từ `ListModelAvailabilityCandidates`, account endpoint capability, model mapping; async submit/poll tách biệt dựa object storage/task service enabled state.

**Files:**
- Modify: `backend/internal/service/api_key_capabilities.go` — implement `resolveImageCapability`.
- Modify: `backend/internal/service/api_key_capabilities_test.go` — thêm test cases image gate, platform, account, async split.

**Interfaces:**
- Consumes: `APIKey`, `Group`, account repo, scheduler candidate query, existing image gate logic (nếu có feature flag hoặc group flag).
- Produces: `ImageCapability` struct với `available`, `gate_open`, `account_available`, `sync {available, reason}`, `async {submit_available, poll_available, reason}`.
- Must preserve: không mutate account state; sync/async split dựa feature flag/object storage không phải group name.

**Constraints:**
- Tái sử dụng global constraints.
- Image gate: kiểm tra group flag/feature cho image capability; không assume group name `Image` là gate.
- Platform route: OpenAI hoặc Grok; composite resolve target platform trước khi check.
- Account candidate: `ListModelAvailabilityCandidates` với platform OpenAI/Grok; check account active+schedulable.
- Endpoint capability: reuse existing `OpenAIEndpointCapability` constants hoặc account method `SupportsOpenAIEndpointCapability`.
- Model mapping: check account `IsModelSupported` cho at least one image model candidate.
- Async submit: nếu object storage/task service disabled trả `submit_available=false` với `reason="feature_disabled"`; sync có thể remain available.
- Async poll: tách biệt; task store pollable thì poll available ngay cả khi submit disabled.

- [ ] **Step 1 — Establish RED**
  - Thêm test case `TestImageCapabilityGateClosed`:
    ```go
    func TestImageCapabilityGateClosed(t *testing.T) {
        // fake key với group image gate = false
        // account có mapping image
        caps := resolveImageCapability(...)
        require.False(t, caps.Available)
        require.False(t, caps.GateOpen)
        require.Equal(t, ReasonGroupDisabled, caps.Sync.Reason)
    }
    ```
  - Run: `cd backend && go test ./internal/service -run TestImageCapabilityGateClosed`
  - Expected: fail vì `resolveImageCapability` vẫn trả placeholder.

- [ ] **Step 2 — Implement minimally**
  - Trong `api_key_capabilities.go`, implement `resolveImageCapability`:
    - Check group image gate (giả sử có `group.AllowImageGeneration` hoặc tương đương); nếu false trả `gate_open=false`, `available=false`, `reason=ReasonGroupDisabled`.
    - Resolve platform: nếu composite dùng existing composite resolution (nếu có); check platform OpenAI hoặc Grok; otherwise `reason=ReasonPlatformUnsupported`.
    - Query account candidates từ `accountRepo.ListModelAvailabilityCandidates(ctx, &groupID, []string{platform}, includeGrouped)`.
    - Nếu candidates empty trả `account_available=false`, `reason=ReasonNoAccount`.
    - Check at least one account `IsModelSupported` cho image model candidate hoặc has image endpoint capability; nếu không trả `reason=ReasonModelNotAvailable`.
    - Sync available = true nếu pass all checks.
    - Async submit: kiểm tra object storage/task service enabled (giả sử có `imageTaskService.Enabled()` hoặc config flag); nếu disabled trả `submit_available=false`, `reason=ReasonFeatureDisabled`; sync unaffected.
    - Async poll: tách biệt; nếu task store pollable (có `imageTaskService` hoặc tương đương) trả `poll_available=true`.

- [ ] **Step 3 — Verify GREEN**
  - Run: `cd backend && go test ./internal/service -run TestImageCapabilityGateClosed`
  - Expected: pass.
  - Thêm test cases: gate open + no account, gate open + account + mapping → sync available, async submit disabled by feature flag.
  - Run: `cd backend && go test ./internal/service -run TestImageCapability`
  - Expected: all pass.

- [ ] **Step 4 — Check nearby regressions**
  - Run: `cd backend && go test ./internal/service/...`
  - Expected: không có new failure.

- [ ] **Step 5 — Self-review**
  - Kiểm tra không dùng keyword model substring match; dùng `IsModelSupported` và endpoint capability.
  - Report: image capability logic implemented, tests pass, sync/async split dựa feature flag.

---

## Task 4: Batch image capability implementation

**Outcome:** Resolver `resolveBatchImageCapability` kiểm tra batch flag, group platform Gemini, provider registry/scheduler, Gemini account candidate, model mapping, pricing.

**Files:**
- Modify: `backend/internal/service/api_key_capabilities.go` — implement `resolveBatchImageCapability`.
- Modify: `backend/internal/service/api_key_capabilities_test.go` — thêm test cases batch flag, platform, account, model mapping.

**Interfaces:**
- Consumes: `APIKey`, `Group`, account repo, batch feature flag (giả sử `group.EnableBatchImage` hoặc config).
- Produces: `BatchImageCapability` struct với `available`, `reason`, và `models` route metadata (`method="GET"`, `path="/v1/images/batches/models"`).
- Must preserve: không hardcode model list; link existing discovery endpoint.

**Constraints:**
- Tái sử dụng global constraints.
- Batch flag: kiểm tra group/feature flag cho batch image.
- Platform: Gemini; không dùng group name; check `group.Platform == service.PlatformGemini`.
- Account candidate: `ListModelAvailabilityCandidates` với platform Gemini.
- Model mapping: check account `IsModelSupported` cho batch model candidate; không keyword match.
- Pricing: nếu có pricing check (giả sử batch service kiểm tra pricing availability) reuse existing logic; otherwise skip.

- [x] **Step 1 — Establish RED**
  - Thêm test case `TestBatchImageCapabilityFlagFalse`:
    ```go
    func TestBatchImageCapabilityFlagFalse(t *testing.T) {
        // group batch flag = false
        caps := resolveBatchImageCapability(...)
        require.False(t, caps.Available)
        require.Equal(t, ReasonGroupDisabled, caps.Reason)
    }
    ```
  - Run: `cd backend && go test ./internal/service -run TestBatchImageCapabilityFlagFalse`
  - Expected: fail.

- [x] **Step 2 — Implement minimally**
  - Implement `resolveBatchImageCapability`:
    - Check batch flag; nếu false trả `reason=ReasonGroupDisabled`.
    - Check platform Gemini; nếu không trả `reason=ReasonPlatformUnsupported`.
    - Query Gemini account candidates.
    - Nếu candidates empty trả `reason=ReasonNoAccount`.
    - Check at least one account `IsModelSupported` cho batch model candidate.
    - Nếu pass trả `available=true`, `models.available=true`, `models.method="GET"`, `models.path="/v1/images/batches/models"`.

- [x] **Step 3 — Verify GREEN**
  - Run: `cd backend && go test ./internal/service -run TestBatchImageCapabilityFlagFalse`
  - Expected: pass.
  - Thêm test cases: flag true + non-Gemini platform, flag true + Gemini + no account, flag true + Gemini + account + mapping → available.
  - Run: `cd backend && go test ./internal/service -run TestBatchImageCapability`
  - Expected: all pass.

- [x] **Step 4 — Check nearby regressions**
  - Run: `cd backend && go test ./internal/service/...`
  - Expected: không có new failure.

- [x] **Step 5 — Self-review**
  - Kiểm tra không hardcode model list; `models` route metadata link discovery endpoint.
  - Report: batch capability logic, tests pass.

---

## Task 5: Video capability implementation

**Outcome:** Resolver `resolveVideoCapability` kiểm tra Grok/composite video eligibility, generation/status/content available, edit/extension tách biệt (composite current route rejects edit/extension).

**Files:**
- Modify: `backend/internal/service/api_key_capabilities.go` — implement `resolveVideoCapability`.
- Modify: `backend/internal/service/api_key_capabilities_test.go` — thêm test cases Grok video, composite generation-only, edit/extension asymmetry.

**Interfaces:**
- Consumes: `APIKey`, `Group`, account repo, existing `SelectMediaVideoRequestAccount` hoặc `SelectAccountWithSchedulerForCapability`, Grok media eligibility/endpoint capability.
- Produces: `VideoCapability` struct với `generation`, `status`, `content`, `edit`, `extension` nested `{available, reason, path}`.
- Must preserve: không claim composite edit/extension available khi current route rejects it.

**Constraints:**
- Tái sử dụng global constraints.
- Platform: Grok hoặc composite resolve Grok; check Grok media gate/feature.
- Account candidate: reuse existing Grok media account selection logic.
- Endpoint capability: `OpenAIEndpointCapabilityGrokMediaGeneration` hoặc tương đương.
- Edit/extension asymmetry: composite route handler current code rejects edit/extension; nếu composite trả generation/status/content available nhưng edit/extension `available=false`, `reason=ReasonRouteUnsupported`.

- [ ] **Step 1 — Establish RED**
  - Thêm test case `TestVideoCapabilityCompositeGenerationOnly`:
    ```go
    func TestVideoCapabilityCompositeGenerationOnly(t *testing.T) {
        // composite group với Grok account eligible
        caps := resolveVideoCapability(...)
        require.True(t, caps.Generation.Available)
        require.False(t, caps.Edit.Available)
        require.Equal(t, ReasonRouteUnsupported, caps.Edit.Reason)
    }
    ```
  - Run: `cd backend && go test ./internal/service -run TestVideoCapabilityCompositeGenerationOnly`
  - Expected: fail.

- [ ] **Step 2 — Implement minimally**
  - Implement `resolveVideoCapability`:
    - Check Grok/composite platform resolution.
    - Query Grok media account candidate (reuse `SelectMediaVideoRequestAccount` predicate hoặc tương đương).
    - Nếu eligible: generation/status/content available.
    - Edit/extension: nếu platform composite trả `available=false`, `reason=ReasonRouteUnsupported`; nếu Grok native check Grok route support (hiện có) và trả available nếu pass.

- [ ] **Step 3 — Verify GREEN**
  - Run: `cd backend && go test ./internal/service -run TestVideoCapabilityCompositeGenerationOnly`
  - Expected: pass.
  - Thêm test cases: Grok native all available, no Grok account → all unavailable.
  - Run: `cd backend && go test ./internal/service -run TestVideoCapability`
  - Expected: all pass.

- [ ] **Step 4 — Check nearby regressions**
  - Run: `cd backend && go test ./internal/service/...`
  - Expected: không có new failure.

- [ ] **Step 5 — Self-review**
  - Kiểm tra edit/extension asymmetry được test coverage.
  - Report: video capability logic, composite generation-only behavior, tests pass.

---

## Task 6: Seedance capability implementation

**Outcome:** Resolver `resolveSeedanceCapability` kiểm tra OpenAI target, API-key account, custom base URL, Seedance endpoint capability, model mapping.

**Files:**
- Modify: `backend/internal/service/api_key_capabilities.go` — implement `resolveSeedanceCapability`.
- Modify: `backend/internal/service/api_key_capabilities_test.go` — thêm test cases Seedance OpenAI account, endpoint capability.

**Interfaces:**
- Consumes: `APIKey`, `Group`, account repo, existing `OpenAIEndpointCapabilitySeedance`.
- Produces: `SeedanceCapability` struct với `available`, `reason`.
- Must preserve: không claim available khi account lacks endpoint capability.

**Constraints:**
- Tái sử dụng global constraints.
- Platform: OpenAI target.
- Account type: API-key style OpenAI account với custom base URL.
- Endpoint capability: `account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilitySeedance)`.
- Model mapping: check `IsModelSupported` cho Seedance model candidate.

- [ ] **Step 1 — Establish RED**
  - Thêm test case `TestSeedanceCapabilityNoEndpointCapability`:
    ```go
    func TestSeedanceCapabilityNoEndpointCapability(t *testing.T) {
        // OpenAI account lacking endpoint capability
        caps := resolveSeedanceCapability(...)
        require.False(t, caps.Available)
        require.Equal(t, ReasonAccountCapabilityUnverified, caps.Reason)
    }
    ```
  - Run: `cd backend && go test ./internal/service -run TestSeedanceCapabilityNoEndpointCapability`
  - Expected: fail.

- [ ] **Step 2 — Implement minimally**
  - Implement `resolveSeedanceCapability`:
    - Check platform OpenAI.
    - Query API-key OpenAI account candidates với custom base URL.
    - Check endpoint capability `SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilitySeedance)`.
    - Check model mapping.
    - Nếu pass trả `available=true`.

- [ ] **Step 3 — Verify GREEN**
  - Run: `cd backend && go test ./internal/service -run TestSeedanceCapabilityNoEndpointCapability`
  - Expected: pass.
  - Thêm test case: OpenAI account with endpoint capability + mapping → available.
  - Run: `cd backend && go test ./internal/service -run TestSeedanceCapability`
  - Expected: all pass.

- [ ] **Step 4 — Check nearby regressions**
  - Run: `cd backend && go test ./internal/service/...`
  - Expected: không có new failure.

- [ ] **Step 5 — Self-review**
  - Report: Seedance capability logic, endpoint capability check, tests pass.

---

## Task 7: Gemini native image capability implementation

**Outcome:** Resolver `resolveGeminiNativeImageCapability` kiểm tra Gemini/Antigravity native route, group/force-platform guard, allowlist, active+schedulable native account, mapping/`IsModelSupported`, provider lane, image request/model contract, native `generateContent`/`streamGenerateContent`.

**Files:**
- Modify: `backend/internal/service/api_key_capabilities.go` — implement `resolveGeminiNativeImageCapability`.
- Modify: `backend/internal/service/api_key_capabilities_test.go` — thêm test cases Gemini native image allowlist, account, provider lane, model contract, text-only model unaffected.

**Interfaces:**
- Consumes: `APIKey`, `Group`, account repo, group allowlist, native Gemini route predicate, provider lane support, image model/request contract.
- Produces: `GeminiNativeImageCapability` struct với `available`, `reason`, `generate_content {available, path}`, `stream_generate_content {available, path}`.
- Must preserve: không claim available chỉ từ model mapping alone; text-only model không làm mất ordinary native text capability.

**Constraints:**
- Tái sử dụng global constraints.
- Route: `/v1beta/models/{model}:generateContent` và `streamGenerateContent`; Gemini/Antigravity native.
- Platform guard: Gemini target hoặc composite→Gemini resolution, hoặc forced Antigravity với group guard.
- Allowlist: group model allowlist pass.
- Account candidate: `ListModelAvailabilityCandidates` Gemini/Antigravity platform.
- Mapping/model support: `IsModelSupported` cho image model candidate; không keyword match.
- Provider lane: Gemini/Antigravity provider lane support; không claim available cho unsupported lane.
- Image request/model contract: check image-capable model hoặc `generationConfig.imageConfig` type evidence.
- Text-only model: nếu model text-only trả native image unavailable nhưng không làm mất native text capability (không test ở resolver này).

- [ ] **Step 1 — Establish RED**
  - Thêm test case `TestGeminiNativeImageCapabilityMappingAlone`:
    ```go
    func TestGeminiNativeImageCapabilityMappingAlone(t *testing.T) {
        // Gemini account with image model mapping alone
        // group/platform/allowlist/account pass
        // but provider lane hoặc image contract unverified
        caps := resolveGeminiNativeImageCapability(...)
        require.False(t, caps.Available)
        require.Equal(t, ReasonAccountCapabilityUnverified, caps.Reason)
    }
    ```
  - Run: `cd backend && go test ./internal/service -run TestGeminiNativeImageCapabilityMappingAlone`
  - Expected: fail.

- [ ] **Step 2 — Implement minimally**
  - Implement `resolveGeminiNativeImageCapability`:
    - Check Gemini/Antigravity native route guard (group platform Gemini hoặc composite→Gemini, hoặc forced Antigravity với group).
    - Check group allowlist.
    - Query Gemini/Antigravity account candidates.
    - Check `IsModelSupported` cho image model candidate.
    - Check provider lane support (giả sử có predicate hoặc account method); nếu unsupported trả `reason=ReasonAccountCapabilityUnverified`.
    - Check image request/model contract (giả sử có type evidence hoặc model family); nếu không trả `reason=ReasonAccountCapabilityUnverified`.
    - Nếu pass trả `available=true`, `generate_content.available=true`, `stream_generate_content.available=true` với paths.

- [ ] **Step 3 — Verify GREEN**
  - Run: `cd backend && go test ./internal/service -run TestGeminiNativeImageCapabilityMappingAlone`
  - Expected: pass.
  - Thêm test cases: all conditions pass → available; text-only model → native image unavailable (không test native text ở đây).
  - Run: `cd backend && go test ./internal/service -run TestGeminiNativeImageCapability`
  - Expected: all pass.

- [ ] **Step 4 — Check nearby regressions**
  - Run: `cd backend && go test ./internal/service/...`
  - Expected: không có new failure.

- [ ] **Step 5 — Self-review**
  - Kiểm tra không claim available chỉ từ mapping alone; provider lane và image contract được test.
  - Report: Gemini native image capability logic, tests pass.

---

## Task 8: Frontend capability API client

**Outcome:** `frontend/src/api/keys.ts` thêm `getCapabilities(id: number)` method và DTO types `ApiKeyCapabilities` match backend response contract.

**Files:**
- Modify: `frontend/src/api/keys.ts`

**Interfaces:**
- Consumes: API key ID `number`, existing `apiClient` base.
- Produces: `Promise<ApiKeyCapabilities>`.
- Must preserve: không dùng API key secret trong request; fetch by numeric ID.

**Constraints:**
- Tái sử dụng global constraints.
- Method signature: `export async function getCapabilities(id: number): Promise<ApiKeyCapabilities>`.
- Request: `GET /user/api-keys/{id}/capabilities` (relative path cho `apiClient`).
- DTO types match backend JSON response: `object`, `key_id`, `group`, `image`, `batch_image`, `video`, `seedance`, `gemini_native`.

- [ ] **Step 1 — Establish RED**
  - Mock test hoặc type-only verification: thêm DTO types `ApiKeyCapabilities`, `ImageCapability`, `BatchImageCapability`, `VideoCapability`, `SeedanceCapability`, `GeminiNativeImageCapability`.
  - Tạo stub method `getCapabilities(id: number)` trả mock data.
  - Run: `cd frontend && pnpm run typecheck`
  - Expected: pass nếu types đúng; fail nếu chưa có types.

- [ ] **Step 2 — Implement minimally**
  - Trong `keys.ts`:
    - Thêm DTO types:
      ```typescript
      export interface ApiKeyCapabilities {
        object: string
        key_id: number
        group?: { id: number; platform: string }
        image: ImageCapability
        batch_image: BatchImageCapability
        video: VideoCapability
        seedance: SeedanceCapability
        gemini_native: GeminiNativeImageCapability
      }
      export interface ImageCapability {
        available: boolean
        gate_open?: boolean
        account_available?: boolean
        sync: CapabilityRoute
        async: { generation: CapabilityRoute; edit: CapabilityRoute; poll: CapabilityRoute }
      }
      export interface CapabilityRoute { available: boolean; reason?: string; method?: string; path?: string }
      export interface BatchImageCapability {
        available: boolean
        reason?: string
        models: CapabilityRoute
      }
      export interface VideoCapability {
        generation: CapabilityRoute
        status: CapabilityRoute
        content: CapabilityRoute
        edit: CapabilityRoute
        extension: CapabilityRoute
      }
      export interface SeedanceCapability {
        available: boolean
        reason?: string
        create: CapabilityRoute
        status: CapabilityRoute
        delete: CapabilityRoute
      }
      export interface GeminiNativeImageCapability {
        image_generation: {
          available: boolean
          reason?: string
          generate_content: CapabilityRoute
          stream_generate_content: CapabilityRoute
        }
      }
      ```
    - Thêm method:
      ```typescript
      export async function getCapabilities(id: number): Promise<ApiKeyCapabilities> {
        const { data } = await apiClient.get<ApiKeyCapabilities>(`/user/api-keys/${id}/capabilities`)
        return data
      }
      ```

- [ ] **Step 3 — Verify GREEN**
  - Run: `cd frontend && pnpm run typecheck`
  - Expected: pass.
  - Nếu có test convention cho API client, thêm mock test case.
  - Run: `cd frontend && pnpm run test:run -- src/api/keys.spec.ts` (nếu có).
  - Expected: pass.

- [ ] **Step 4 — Check nearby regressions**
  - Run: `cd frontend && pnpm run lint:check`
  - Expected: pass.

- [ ] **Step 5 — Self-review**
  - Kiểm tra DTO types match backend response; method không dùng API key secret.
  - Report: `keys.ts` capability API client, types, typecheck pass.

---

## Task 9: Frontend Use Key modal dynamic API tab

**Outcome:** `UseKeyModal.vue` thêm `apiKeyId` prop, fetch capability khi opened/key ID changes, loading/error state preserve existing tabs, dynamic API tab render only when at least one capability available, sections (image/gemini_native/batch_image/video/seedance) render independently với `baseUrl` prop.

**Files:**
- Modify: `frontend/src/components/keys/UseKeyModal.vue`
- Modify: `frontend/src/views/user/KeysView.vue` — pass `selectedKey.id` as `apiKeyId` prop.

**Interfaces:**
- Consumes: `apiKeyId: number`, `baseUrl: string`, existing modal props.
- Produces: dynamic API tab với sections; loading/error state.
- Must preserve: existing Claude/Codex/Gemini/OpenCode tabs usable during loading/error; không dùng API key secret.

**Constraints:**
- Tái sử dụng global constraints.
- Props: thêm `apiKeyId?: number` prop.
- Fetch: `watch(() => props.apiKeyId, (id) => { if (id) fetchCapabilities(id) })`.
- Loading/error: không remove existing tabs; API tab xuất hiện chỉ khi capability loaded và at least one available.
- Sections: image sync/async, Gemini native, batch, video (generation/status/content, edit/extension tách biệt), Seedance.
- Example snippets: dùng `props.baseUrl`, không hardcode domain.
- Accessibility: `role="tab"`, `aria-selected`, `aria-controls`, `role="tabpanel"`.

- [ ] **Step 1 — Establish RED**
  - Thêm unit/component test (nếu có convention):
    ```typescript
    test('UseKeyModal does not crash when apiKeyId prop changes', () => {
      const wrapper = mount(UseKeyModal, { props: { show: true, apiKey: 'sk-test', baseUrl: 'https://test', platform: 'openai', apiKeyId: 1 } })
      expect(wrapper.exists()).toBe(true)
    })
    ```
  - Run: `cd frontend && pnpm run test:run -- UseKeyModal.spec.ts` (nếu có).
  - Expected: fail nếu `apiKeyId` prop chưa có.

- [ ] **Step 2 — Implement minimally**
  - Trong `UseKeyModal.vue`:
    - Thêm prop `apiKeyId?: number`.
    - Thêm reactive state: `const capabilities = ref<ApiKeyCapabilities | null>(null)`, `const capabilitiesLoading = ref(false)`, `const capabilitiesError = ref<string | null>(null)`.
    - Thêm `watch(() => props.apiKeyId, async (id) => { if (!id) return; capabilitiesLoading.value = true; try { capabilities.value = await getCapabilities(id); } catch (e) { capabilitiesError.value = extractApiErrorMessage(e); } finally { capabilitiesLoading.value = false; } }, { immediate: true })`.
    - Computed `showApiTab = computed(() => capabilities.value && (capabilities.value.image.available || capabilities.value.batch_image.available || capabilities.value.video.generation.available || capabilities.value.seedance.available || capabilities.value.gemini_native.image_generation.available))`.
    - Template: thêm tab `v-if="showApiTab"` với icon, label `t('keys.useKeyModal.api.tab')`.
    - Tab panel: `v-show="activeTab === 'api'"` với sections:
      - `v-if="capabilities.image.sync.available"` → image sync example.
      - `v-if="capabilities.image.async.generation.available"` / `edit` / `poll` → independent async image examples.
      - `v-if="capabilities.gemini_native.image_generation.available"` → Gemini native section với `generateContent`/`streamGenerateContent` examples, paths từ capability.
      - `v-if="capabilities.batch_image.available"` → batch discovery example from `capabilities.batch_image.models.method/path`; do not invent submit/status/content routes.
      - `v-if="capabilities.video.generation.available"` → video generation example; `v-if="capabilities.video.edit.available"` → edit example.
      - `v-if="capabilities.seedance.available"` → Seedance example.
    - Examples dùng `props.baseUrl` placeholder `{base_url}`, `{api_key}`, `{model}`.
  - Trong `KeysView.vue`:
    - Pass `selectedKey.id` as `:apiKeyId="selectedKey.id"` prop.

- [ ] **Step 3 — Verify GREEN**
  - Run: `cd frontend && pnpm run typecheck`
  - Expected: pass.
  - Nếu có component test, run: `cd frontend && pnpm run test:run -- UseKeyModal.spec.ts`.
  - Expected: pass.

- [ ] **Step 4 — Check nearby regressions**
  - Run: `cd frontend && pnpm run lint:check`
  - Expected: pass.

- [ ] **Step 5 — Self-review**
  - Kiểm tra `baseUrl` prop được dùng, không hardcode domain; `apiKeyId` prop numeric, không dùng secret.
  - Report: `UseKeyModal.vue` dynamic API tab, `KeysView.vue` pass `apiKeyId`, typecheck/lint pass.

---

## Task 10: Frontend i18n leaf keys EN/VI parity

**Outcome:** Thêm leaf keys `keys.useKeyModal.api.*` vào `en.ts` và `vi.ts` với complete parity; chạy `pnpm run check:i18n` pass.

**Files:**
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/i18n/locales/vi.ts`

**Interfaces:**
- Consumes: existing locale structure.
- Produces: leaf keys `keys.useKeyModal.api.tab`, `keys.useKeyModal.api.loading`, `keys.useKeyModal.api.error`, `keys.useKeyModal.api.image.*`, `keys.useKeyModal.api.geminiNative.*`, `keys.useKeyModal.api.batchImage.*`, `keys.useKeyModal.api.video.*`, `keys.useKeyModal.api.seedance.*`, `keys.useKeyModal.api.reason.*`.
- Must preserve: EN/VI parity; no placeholder mismatch.

**Constraints:**
- Tái sử dụng global constraints.
- Required keys (minimum):
  - `keys.useKeyModal.api.tab` → EN: "API", VI: "API".
  - `keys.useKeyModal.api.loading` → EN: "Loading capabilities...", VI: "Đang tải khả năng...".
  - `keys.useKeyModal.api.error` → EN: "Failed to load capabilities", VI: "Tải khả năng thất bại".
  - `keys.useKeyModal.api.image.sync` → EN: "Image Sync", VI: "Ảnh đồng bộ".
  - `keys.useKeyModal.api.image.async` → EN: "Image Async", VI: "Ảnh bất đồng bộ".
  - `keys.useKeyModal.api.geminiNative.title` → EN: "Gemini Native Image", VI: "Ảnh Gemini native".
  - `keys.useKeyModal.api.batchImage.title` → EN: "Batch Image", VI: "Ảnh batch".
  - `keys.useKeyModal.api.video.generation` → EN: "Video Generation", VI: "Tạo video".
  - `keys.useKeyModal.api.video.edit` → EN: "Video Edit", VI: "Chỉnh sửa video".
  - `keys.useKeyModal.api.seedance.title` → EN: "Seedance", VI: "Seedance".
  - `keys.useKeyModal.api.reason.noGroup` → EN: "No group assigned", VI: "Chưa gán nhóm".
  - `keys.useKeyModal.api.reason.groupDisabled` → EN: "Group disabled", VI: "Nhóm đã tắt".
  - `keys.useKeyModal.api.reason.platformUnsupported` → EN: "Platform unsupported", VI: "Nền tảng không hỗ trợ".
  - `keys.useKeyModal.api.reason.featureDisabled` → EN: "Feature disabled", VI: "Tính năng đã tắt".
  - `keys.useKeyModal.api.reason.noAccount` → EN: "No account available", VI: "Không có tài khoản khả dụng".
  - `keys.useKeyModal.api.reason.modelNotAvailable` → EN: "Model not available", VI: "Model không khả dụng".
  - `keys.useKeyModal.api.reason.accountCapabilityUnverified` → EN: "Account capability unverified", VI: "Khả năng tài khoản chưa xác minh".
  - `keys.useKeyModal.api.reason.routeUnsupported` → EN: "Route unsupported", VI: "Route không hỗ trợ".

- [ ] **Step 1 — Establish RED**
  - Run: `cd frontend && pnpm run check:i18n`
  - Expected: fail vì keys chưa có.

- [ ] **Step 2 — Implement minimally**
  - Trong `en.ts`:
    - Thêm nested keys under `keys.useKeyModal.api.*` với EN translations.
  - Trong `vi.ts`:
    - Thêm nested keys under `keys.useKeyModal.api.*` với VI translations parity.

- [ ] **Step 3 — Verify GREEN**
  - Run: `cd frontend && pnpm run check:i18n`
  - Expected: pass với `0 missing` EN→VI, `0 placeholder mismatches`.

- [ ] **Step 4 — Check nearby regressions**
  - Run: `cd frontend && pnpm run lint:check`
  - Expected: pass.

- [ ] **Step 5 — Self-review**
  - Kiểm tra không có leaf key mismatch; placeholder `{}` parity.
  - Report: i18n keys EN/VI parity, `check:i18n` pass.

---

## Task 11: Verification và final checks

**Outcome:** Chạy full backend tests, frontend tests, lint, typecheck, build; kiểm tra `git status`, `git diff --check`, không có trailing whitespace, no credential leak; report final status.

**Files:**
- No new files created; verification only.

**Interfaces:**
- Consumes: all changed files.
- Produces: verification evidence.
- Must preserve: worktree gốc unchanged; analysis worktree có staged/untracked changes only.

**Constraints:**
- Tái sử dụng global constraints.
- Commands (từ PRD/TDD):
  - Backend: `cd backend && go test ./... && go vet ./... && golangci-lint run ./...`.
  - Frontend: `cd frontend && pnpm run check:i18n && pnpm run typecheck && pnpm run lint:check && pnpm run test:run && pnpm run build`.
  - Git: `git diff --check`, `git status --short`.

- [ ] **Step 1 — Backend verification**
  - Run: `cd /home/ubuntu/workspaces/sub2api-origin-main/backend && go test ./...`
  - Expected: all tests pass.
  - Run: `cd /home/ubuntu/workspaces/sub2api-origin-main/backend && go vet ./...`
  - Expected: no issues.
  - Run: `cd /home/ubuntu/workspaces/sub2api-origin-main/backend && golangci-lint run ./...` (nếu có).
  - Expected: no new issues hoặc existing issues only.

- [ ] **Step 2 — Frontend verification**
  - Run: `cd /home/ubuntu/workspaces/sub2api-origin-main/frontend && pnpm run check:i18n`
  - Expected: pass.
  - Run: `cd /home/ubuntu/workspaces/sub2api-origin-main/frontend && pnpm run typecheck`
  - Expected: pass.
  - Run: `cd /home/ubuntu/workspaces/sub2api-origin-main/frontend && pnpm run lint:check`
  - Expected: pass.
  - Run: `cd /home/ubuntu/workspaces/sub2api-origin-main/frontend && pnpm run test:run`
  - Expected: pass (hoặc existing test count không giảm).
  - Run: `cd /home/ubuntu/workspaces/sub2api-origin-main/frontend && pnpm run build`
  - Expected: build success.

- [ ] **Step 3 — Git whitespace và status check**
  - Run: `cd /home/ubuntu/workspaces/sub2api-origin-main && git diff --check`
  - Expected: no whitespace errors.
  - Run: `cd /home/ubuntu/workspaces/sub2api-origin-main && git status --short`
  - Expected: liệt kê changed/untracked files; không có unexpected changes.

- [ ] **Step 4 — Worktree gốc preservation check**
  - Run: `cd /home/ubuntu/workspaces/sub2api && git status --short --porcelain | wc -l`
  - Expected: still `~1,857` lines (staged deletions unchanged).

- [ ] **Step 5 — Self-review**
  - Inspect changed files list; confirm không có credential values, account IDs, upstream URLs trong code/tests/response examples.
  - Report: verification commands pass, worktree gốc unchanged, analysis worktree ready for review.

---

## Expected Evidence sau khi implementation hoàn thành

Backend:
- `backend/internal/service/api_key_capabilities.go` — resolver service với per-surface capability logic.
- `backend/internal/service/api_key_capabilities_test.go` — table tests cover gate/account/platform/model/provider/route predicates.
- `backend/internal/handler/api_key_capabilities_handler.go` — authenticated user handler.
- `backend/internal/handler/api_key_capabilities_handler_test.go` — auth/ownership/serialization tests.
- `backend/internal/handler/wire.go`, `handler.go`, `backend/internal/server/routes/user.go` — handler wired và route registered.
- `go test ./...` pass, `go vet ./...` clean, `golangci-lint` no new issues.

Frontend:
- `frontend/src/api/keys.ts` — `getCapabilities(id)` method và DTO types.
- `frontend/src/components/keys/UseKeyModal.vue` — `apiKeyId` prop, capability fetch, dynamic API tab với sections.
- `frontend/src/views/user/KeysView.vue` — pass `selectedKey.id`.
- `frontend/src/i18n/locales/en.ts`, `vi.ts` — leaf keys EN/VI parity.
- `pnpm run check:i18n`, `typecheck`, `lint:check`, `test:run`, `build` pass.

Git:
- `git diff --check` pass.
- `git status --short` liệt kê changed files; worktree gốc `/home/ubuntu/workspaces/sub2api` unchanged.

---

## Risks and Mitigations

| Risk | Mitigation |
|---|---|
| Backend resolver drift từ scheduler logic | Tái sử dụng `ListModelAvailabilityCandidates`, `IsModelSupported`, account endpoint capability; add route-contract tests. |
| Group gate mistaken for model availability | Separate gate/account/model/route fields và tests. |
| Model keyword produces false positive | Require `IsModelSupported` plus endpoint/provider capability; không keyword substring match. |
| Composite route asymmetry hidden | Separate video generation/status/content/edit/extension fields; test composite generation-only. |
| Async feature advertised without object storage | Read task service enabled/pollable state; sync/async split dựa feature flag. |
| Capability endpoint leaks topology/secrets | Closed reason enum; no account/provider/upstream fields; serialization tests scan for forbidden keys. |
| UI API request causes secret exposure | Fetch by numeric key ID through JWT user route; never send API key secret to resolver. |
| Frontend dynamic tab crashes on null group/platform | Defensive checks; loading/error state preserve existing tabs. |
| i18n parity drift | Run `pnpm run check:i18n` in verification; block merge if fail. |

---

## Next Steps after Plan Approval

1. Confirm response contract namespace và structure với user.
2. Confirm whether API tab hidden when all media unavailable or shows localized explanation.
3. Confirm whether public model IDs in response hoặc link to discovery endpoint.
4. Confirm composite video generation-only behavior expectation.
5. Implement tasks 1-11 sequentially; run focused tests per task.
6. After all tasks pass, commit với message referencing plan ID và PRD.
7. Push to feature branch; open PR with CI evidence và self-review summary.
8. Independent review required before merge; no production rollout without separate approval.
