# OpenCode Image Generation Workaround Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不修改 OpenCode 的前提下，让 sub2api 把 OpenAI Responses `image_generation_call` 转成 OpenCode 可安全保存的普通文本 marker、1 小时下载链接，并在下一轮请求中把 marker 恢复为上游 `input_image`。

**Architecture:** 新增独立的 generated image store、OpenCode 响应重写器、SSE 重写器、输入 rehydrate helper 和下载 handler。OpenCode 客户端永远看不到 raw `image_generation_call`；非 OpenCode 客户端继续保留标准 Responses 语义。

**Tech Stack:** Go, Gin, `tidwall/gjson`, `tidwall/sjson`, existing sub2api OpenAI gateway, frontend `UseKeyModal` Vitest coverage.

**Spec:** `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\backend\docs\superpowers\specs\2026-04-28-opencode-image-generation-workaround-design.md`

**Worktree:** `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\.worktrees\opencode-image-generation-workaround`

**Command Workdirs:** All Go commands in this plan run from `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\.worktrees\opencode-image-generation-workaround\backend` unless a step explicitly says otherwise. All frontend `pnpm` commands run from `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\.worktrees\opencode-image-generation-workaround\frontend`.

---

## File Structure

- Create: `backend/internal/service/openai_generated_image_store.go`
  - Owns generated image file persistence, metadata, expiration, MIME sniffing, atomic writes, cleanup, and safe lookup.
- Create: `backend/internal/service/openai_generated_image_store_test.go`
  - Unit tests for store save/load/expire/cleanup/path validation/oversize behavior.
- Create: `backend/internal/service/openai_opencode_image_rewrite.go`
  - Owns OpenCode JSON response output replacement and marker/download text construction.
- Create: `backend/internal/service/openai_opencode_image_rewrite_test.go`
  - Unit tests for non-streaming output rewrite and input rehydrate behavior.
- Create: `backend/internal/service/openai_opencode_image_sse.go`
  - Owns OpenCode SSE filtering and synthetic message SSE event generation.
- Create: `backend/internal/service/openai_opencode_image_sse_test.go`
  - Unit tests for stream event filtering, completed fallback, and no-result image item handling.
- Modify: `backend/internal/service/openai_gateway_service.go`
  - Wire rewrite/rehydrate into `/v1/responses` request and response paths; keep existing routing semantics untouched.
- Modify: `backend/internal/service/wire.go`
  - Provide one shared `OpenAIGeneratedImageStore`, inject public settings into OpenAI gateway, and run startup cleanup.
- Modify: `backend/internal/service/openai_gateway_service_test.go`
  - Replace stale auto-inject expectation with carrier-triggered behavior and OpenCode rewrite expectations.
- Modify: `backend/internal/pkg/apicompat/types.go`
  - Keep `image_generation_call` result fields for non-OpenCode and SSE capture.
- Create: `backend/internal/handler/generated_image_handler.go`
  - Gin handler for `GET /sub2api/generated-images/:filename`.
- Create: `backend/internal/handler/generated_image_handler_test.go`
  - Handler tests for download, expired, missing, invalid path, and headers.
- Modify: `backend/internal/handler/handler.go`
  - Add `GeneratedImages *GeneratedImageHandler` to root `Handlers`.
- Modify: `backend/internal/handler/wire.go`
  - Wire the generated image handler and shared generated image store.
- Modify: `backend/internal/server/routes/common.go`
  - Register unauthenticated generated image download route.
- Modify: `backend/internal/server/router.go`
  - Pass handlers into common route registration so generated image download can be registered before authenticated gateway routes.
- Modify generated DI: `backend/cmd/server/wire_gen.go`
  - Keep Wire generated initialization in sync with service/handler constructor changes.
- Modify tests: `backend/internal/service/openai_ws_protocol_forward_test.go`
  - Update direct `NewOpenAIGatewayService` constructor call for new generated image store parameter.
- Modify tests: `backend/internal/service/openai_gateway_record_usage_test.go`
  - Update direct `NewOpenAIGatewayService` constructor call for new generated image store parameter.
- Modify: `backend/internal/web/embed_on.go`
  - Bypass embedded frontend middleware for `/sub2api/generated-images/` only.
- Create: `backend/internal/web/embed_bypass.go`
  - Move `shouldBypassEmbeddedFrontend` out of `embed_on.go` so bypass predicate can be tested without `-tags embed` or prebuilt `dist`.
- Create: `backend/internal/web/embed_bypass_test.go`
  - Tests generated image bypass without requiring embedded frontend assets.
- Modify: `backend/internal/server/middleware/request_logger.go`
  - Redact generated image URL tokens from request-scoped logger path fields.
- Modify: `backend/internal/server/middleware/logger.go`
  - Redact generated image URL tokens from access log path fields.
- Modify tests: `backend/internal/server/middleware/request_access_logger_test.go`
  - Assert generated image download paths are logged with redacted opaque IDs.
- Modify: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
  - Keep existing GPT-5.5 OpenCode metadata image_generation expectation green.

---

### Task 0: Preserve Non-OpenCode Image Capture Fields

**Files:**
- Modify: `backend/internal/pkg/apicompat/types.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: Write failing non-OpenCode SSE capture test**

Add a test that uses a non-OpenCode request and an SSE body where `response.output_item.done.item` is an `image_generation_call` with `result`, while `response.completed.response.output` is empty. Assert final JSON preserves `output.0.type == "image_generation_call"` and `output.0.result`.

```go
func TestHandleSSEToJSON_NonOpenCodePreservesImageGenerationResultFromDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: response.output_item.done`,
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_123","type":"image_generation_call","status":"completed","result":"aGVsbG8=","revised_prompt":"draw a cat","output_format":"png"}}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","response":{"id":"resp_img","model":"gpt-5.5","output":[],"usage":{"input_tokens":7,"output_tokens":9,"output_tokens_details":{"image_tokens":4}}}}`,
			``,
			`data: [DONE]`,
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	require.Equal(t, "image_generation_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "aGVsbG8=", gjson.Get(rec.Body.String(), "output.0.result").String())
}
```

- [ ] **Step 2: Run test and verify RED**

Run from backend workdir:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run TestHandleSSEToJSON_NonOpenCodePreservesImageGenerationResultFromDone -count=1
```

Expected: FAIL if `ResponsesOutput` cannot carry `result`.

- [ ] **Step 3: Implement capture fields and stable key**

In `backend/internal/pkg/apicompat/types.go`, add `Result`, `RevisedPrompt`, `OutputFormat`, `Quality`, `Size`, `Background` to `ResponsesOutput` with JSON tags. In `responsesOutputStableKey`, include `image_generation_call` in the id-based stable key branch.

- [ ] **Step 4: Run test and verify GREEN**

Run command from Step 2.

Expected: PASS.

---

### Task 1: Generated Image Store

**Files:**
- Create: `backend/internal/service/openai_generated_image_store.go`
- Create: `backend/internal/service/openai_generated_image_store_test.go`

- [ ] **Step 1: Write failing store save/load test**

Add `TestOpenAIGeneratedImageStore_SaveLoadAndExpire` to `openai_generated_image_store_test.go`:

```go
var fixedNow = time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
var pngBytes = []byte("\x89PNG\r\n\x1a\nimage-bytes")
var pngB64 = base64.StdEncoding.EncodeToString(pngBytes)

type incrementingRandReader struct{ next byte }

func (r *incrementingRandReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.next
		r.next++
	}
	return len(p), nil
}

func newTestOpenAIGeneratedImageStore(t *testing.T, now time.Time) *OpenAIGeneratedImageStore {
	t.Helper()
	store := NewOpenAIGeneratedImageStore(t.TempDir())
	store.now = func() time.Time { return now }
	store.rand = &incrementingRandReader{}
	store.maxEncodedBytes = 32 << 20
	store.maxDecodedBytes = 20 << 20
	store.maxRehydrateBytes = 20 << 20
	return store
}

func TestOpenAIGeneratedImageStore_SaveLoadAndExpire(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)

	rec, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{
		Base64:       pngB64,
		OutputFormat: "png",
		SourceItemID: "ig_123",
	})
	require.NoError(t, err)
	require.Regexp(t, `^img_[A-Za-z0-9_-]{32,}$`, rec.ID)
	require.Equal(t, "png", rec.Format)
	require.Equal(t, "image/png", rec.MIME)
	require.Equal(t, fixedNow.Add(time.Hour), rec.ExpiresAt)

	loaded, data, err := store.Load(context.Background(), rec.ID)
	require.NoError(t, err)
	require.Equal(t, rec.ID, loaded.ID)
	require.Equal(t, pngBytes, data)

	store.now = func() time.Time { return fixedNow.Add(time.Hour + time.Second) }
	_, _, err = store.Load(context.Background(), rec.ID)
	require.ErrorIs(t, err, errOpenAIGeneratedImageExpired)
}
```

- [ ] **Step 2: Run store test and verify RED**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run TestOpenAIGeneratedImageStore_SaveLoadAndExpire -count=1
```

Expected: FAIL because `OpenAIGeneratedImageStore` does not exist.

- [ ] **Step 3: Implement minimal store types and save/load**

Create `openai_generated_image_store.go` with these public test-facing shapes:

```go
type OpenAIGeneratedImageStore struct {
	root              string
	now               func() time.Time
	rand              io.Reader
	maxAge            time.Duration
	maxEncodedBytes   int64
	maxDecodedBytes   int64
	maxRehydrateBytes int64
	maxTotalBytes     int64
	cleanupLimit      int
	lastCleanup       time.Time
	cleanupInterval   time.Duration
}

type OpenAIGeneratedImageSaveInput struct {
	Base64       string
	OutputFormat string
	SourceItemID string
}

type OpenAIGeneratedImageRecord struct {
	ID           string    `json:"id"`
	Filename     string    `json:"filename"`
	Format       string    `json:"format"`
	MIME         string    `json:"mime"`
	SourceItemID string    `json:"source_item_id,omitempty"`
	SHA256       string    `json:"sha256"`
	DecodedBytes int64     `json:"decoded_bytes"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}
```

Implementation requirements:
- Before decoding, reject encoded payloads whose trimmed length or `base64.StdEncoding.DecodedLen(len(encoded))` exceeds configured limits. Then decode base64 with `base64.StdEncoding.DecodeString`.
- Validate PNG/JPEG/WEBP by magic bytes.
- Generate id with 24 or 32 bytes from `crypto/rand`, URL-safe base64 without padding, prefixed by `img_`; this gives at least 128-bit entropy, with 192/256-bit preferred.
- If the target image or metadata path already exists, retry id generation; never overwrite an existing resource.
- Write image and metadata with temp files followed by `os.Rename`.
- Return `errOpenAIGeneratedImageExpired` when `now()` is after `ExpiresAt`.
- Export sentinel aliases for cross-package handler code: `ErrOpenAIGeneratedImageNotFound`, `ErrOpenAIGeneratedImageExpired`, `ErrOpenAIGeneratedImageInvalid`, `ErrOpenAIGeneratedImageTooLarge`.
- Add `LoadByFilename(ctx, filename)` for download handler use. It must call `validateOpenAIGeneratedImageFilename` before loading metadata.
- Add `Load(ctx, id)` validation with `validateOpenAIGeneratedImageID`; reject short IDs, path separators, NUL, URL encoded separators, and anything outside `^img_[A-Za-z0-9_-]{32,}$` before building metadata paths.

- [ ] **Step 4: Run store save/load test and verify GREEN**

Run the same command from Step 2.

Expected: PASS.

- [ ] **Step 5: Add failing store safety tests**

Add tests:

```go
func TestOpenAIGeneratedImageStore_RejectsMalformedAndUnsupportedImages(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	_, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: "%%%", OutputFormat: "png"})
	require.ErrorIs(t, err, errOpenAIGeneratedImageInvalid)
	gif := base64.StdEncoding.EncodeToString([]byte("GIF89a"))
	_, err = store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: gif, OutputFormat: "gif"})
	require.ErrorIs(t, err, errOpenAIGeneratedImageInvalid)
}

func TestOpenAIGeneratedImageStore_RejectsOversizedImages(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.maxDecodedBytes = 4
	_, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.ErrorIs(t, err, errOpenAIGeneratedImageTooLarge)
}

func TestOpenAIGeneratedImageStore_RejectsOversizedEncodedInputBeforeDecode(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.maxEncodedBytes = 8
	oversized := base64.StdEncoding.EncodeToString([]byte(string(pngBytes) + strings.Repeat("x", 64)))
	_, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: oversized, OutputFormat: "png"})
	require.ErrorIs(t, err, errOpenAIGeneratedImageTooLarge)
}

func TestOpenAIGeneratedImageStore_RejectsInvalidIDOnLoad(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, id := range []string{"img_short", "../img_abcdefghijklmnopqrstuvwxyzABCDEF", `img_abc\\def`, "img_abc%2fdef", "img_abc\x00def"} {
		_, _, err := store.Load(context.Background(), id)
		require.Error(t, err, id)
	}
}

func TestOpenAIGeneratedImageStore_ValidateFilenameRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../img_a.png", `img_a\\b.png`, "img_a%2f.png", "img_a\x00.png", "img_short.png"} {
		_, _, err := validateOpenAIGeneratedImageFilename(name)
		require.Error(t, err, name)
	}
}

func TestOpenAIGeneratedImageStore_CleanupDeletesOnlyExpired(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.cleanupInterval = 24 * time.Hour
	expired, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	fresh, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	store.now = func() time.Time { return fixedNow.Add(2*time.Hour + time.Minute) }
	removed, err := store.Cleanup(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, removed)
	_, _, err = store.Load(context.Background(), expired.ID)
	require.ErrorIs(t, err, errOpenAIGeneratedImageNotFound)
	_, _, err = store.Load(context.Background(), fresh.ID)
	require.NoError(t, err)
}
```

Each test should assert named errors: `errOpenAIGeneratedImageInvalid`, `errOpenAIGeneratedImageTooLarge`, `errOpenAIGeneratedImageNotFound`, `errOpenAIGeneratedImageExpired`.

- [ ] **Step 6: Run safety tests and verify RED**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestOpenAIGeneratedImageStore_(Rejects|Validate|Cleanup)" -count=1
```

Expected: FAIL on missing validation/cleanup behavior.

- [ ] **Step 7: Implement safety behavior**

Add helpers:

```go
func validateOpenAIGeneratedImageFilename(filename string) (id string, format string, err error)
func validateOpenAIGeneratedImageID(id string) error
func safeOpenAIGeneratedImagePath(root string, filename string) (string, error)
func generateOpenAIGeneratedImageID(rand io.Reader) (string, error)
func sniffOpenAIGeneratedImage(data []byte, requested string) (format string, mime string, err error)
func (s *OpenAIGeneratedImageStore) Cleanup(ctx context.Context, limit int) (int, error)
```

Do not log base64 or image bytes.

`validateOpenAIGeneratedImageID` must use anchored regex `^img_[A-Za-z0-9_-]{32,}$` and reject `/`, `\`, `..`, NUL, and URL encoded separators. `validateOpenAIGeneratedImageFilename` must use anchored regex `^img_[A-Za-z0-9_-]{32,}\.(png|jpe?g|webp)$` and apply the same lexical rejection. `safeOpenAIGeneratedImagePath` must verify `filepath.Clean(filepath.Join(root, filename))` remains under the generated image directory.

- [ ] **Step 7b: Add capacity and cleanup wiring tests**

Add tests proving save triggers bounded cleanup when throttled interval allows it, startup cleanup can be called without deleting fresh files, and directory capacity overflow returns `errOpenAIGeneratedImageTooLarge` after expired cleanup cannot free enough space. Use `store.maxTotalBytes` and `store.cleanupLimit` fields in test setup.

```go
func (s *OpenAIGeneratedImageStore) saveDecodedForTest(id string, format string, data []byte) (OpenAIGeneratedImageRecord, error) {
	if err := validateOpenAIGeneratedImageID(id); err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	format, mime, err := sniffOpenAIGeneratedImage(data, format)
	if err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	filename := id + "." + format
	imagePath, err := safeOpenAIGeneratedImagePath(s.root, filename)
	if err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	hash := sha256.Sum256(data)
	rec := OpenAIGeneratedImageRecord{ID:id, Filename:filename, Format:format, MIME:mime, SHA256:hex.EncodeToString(hash[:]), DecodedBytes:int64(len(data)), CreatedAt:s.now(), ExpiresAt:s.now().Add(time.Hour)}
	if err := os.WriteFile(imagePath, data, 0o600); err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	meta, err := json.Marshal(rec)
	if err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	if err := os.WriteFile(filepath.Join(s.root, id+".json"), meta, 0o600); err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	return rec, nil
}

func TestOpenAIGeneratedImageStore_SaveRunsThrottledCleanup(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.cleanupLimit = 10
	store.cleanupInterval = time.Minute
	expired, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	fresh, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	_, _, err = store.Load(context.Background(), expired.ID)
	require.ErrorIs(t, err, errOpenAIGeneratedImageNotFound)
	_, _, err = store.Load(context.Background(), fresh.ID)
	require.NoError(t, err)
}

func TestOpenAIGeneratedImageStore_DefaultLimitsAreSafe(t *testing.T) {
	store := NewOpenAIGeneratedImageStore(t.TempDir())
	require.Positive(t, store.maxEncodedBytes)
	require.Positive(t, store.maxDecodedBytes)
	require.Positive(t, store.maxRehydrateBytes)
	require.Positive(t, store.maxTotalBytes)
	require.Positive(t, store.cleanupLimit)
	require.Positive(t, store.cleanupInterval)
	require.GreaterOrEqual(t, store.maxEncodedBytes, store.maxDecodedBytes)
	require.Greater(t, store.maxTotalBytes, store.maxDecodedBytes)
}

func TestOpenAIGeneratedImageStore_SaveRetriesOnIDCollisionAndDoesNotOverwriteExistingResource(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	collisionBytes := bytes.Repeat([]byte{1}, 24)
	collisionID, err := generateOpenAIGeneratedImageID(bytes.NewReader(collisionBytes))
	require.NoError(t, err)
	oldBytes := []byte("\x89PNG\r\n\x1a\nold-image")
	oldRec, err := store.saveDecodedForTest(collisionID, "png", oldBytes)
	require.NoError(t, err)
	store.rand = io.MultiReader(bytes.NewReader(collisionBytes), bytes.NewReader(bytes.Repeat([]byte{2}, 24)))
	rec, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	require.NotEqual(t, oldRec.ID, rec.ID)
	loaded, data, err := store.Load(context.Background(), oldRec.ID)
	require.NoError(t, err)
	require.Equal(t, oldRec.ID, loaded.ID)
	require.Equal(t, oldBytes, data)
}

func TestOpenAIGeneratedImageStore_StartupCleanupKeepsFreshFiles(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	fresh, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	removed, err := store.Cleanup(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 0, removed)
	_, _, err = store.Load(context.Background(), fresh.ID)
	require.NoError(t, err)
}

func TestOpenAIGeneratedImageStore_RejectsDirectoryCapacityOverflowAfterCleanup(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.cleanupLimit = 10
	expired, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	store.maxTotalBytes = int64(len(pngBytes) - 1)
	_, err = store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.ErrorIs(t, err, errOpenAIGeneratedImageTooLarge)
	_, _, err = store.Load(context.Background(), expired.ID)
	require.ErrorIs(t, err, errOpenAIGeneratedImageNotFound)
}
```

Run and verify RED:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestOpenAIGeneratedImageStore_(SaveRunsThrottledCleanup|DefaultLimitsAreSafe|SaveRetriesOnIDCollisionAndDoesNotOverwriteExistingResource|StartupCleanupKeepsFreshFiles|RejectsDirectoryCapacityOverflowAfterCleanup)" -count=1
```

Expected: FAIL until capacity accounting and throttled cleanup are implemented.

- [ ] **Step 7c: Implement capacity and cleanup wiring**

Add default constants `defaultOpenAIGeneratedImageMaxEncodedBytes`, `defaultOpenAIGeneratedImageMaxDecodedBytes`, `defaultOpenAIGeneratedImageMaxRehydrateBytes`, `defaultOpenAIGeneratedImageMaxTotalBytes`, `defaultOpenAIGeneratedImageCleanupLimit`, and `defaultOpenAIGeneratedImageCleanupInterval`. `NewOpenAIGeneratedImageStore` must initialize all limit fields to positive safe defaults; `maxTotalBytes == 0` means use the default, not unlimited and not reject-all. Add `maxEncodedBytes`, `maxTotalBytes`, `cleanupLimit`, `lastCleanup`, and `cleanupInterval` to the store. `SaveBase64` should run bounded cleanup when `lastCleanup` is zero or `now.Sub(lastCleanup) >= cleanupInterval`, then refuse new saves if directory total plus candidate image/metadata would exceed `maxTotalBytes`. Service initialization should call `Cleanup(ctx, cleanupLimit)` once.

Add provider wiring in `backend/internal/service/wire.go`:

```go
func ProvideOpenAIGeneratedImageStore(cfg *config.Config) *OpenAIGeneratedImageStore {
	store := NewOpenAIGeneratedImageStore(resolveOpenAIGeneratedImageRoot(cfg))
	if _, err := store.Cleanup(context.Background(), store.cleanupLimit); err != nil {
		logger.LegacyPrintf("service.openai_generated_images", "startup cleanup failed: %v", err)
	}
	return store
}
```

Add `ProvideOpenAIGeneratedImageStore` to `ProviderSet`. `resolveOpenAIGeneratedImageRoot` must resolve to `<data-dir>/openai-generated-images`, mirroring `setup.GetDataDir()` semantics (`DATA_DIR` env, then writable `/app/data`, then current directory `.`) without importing `internal/setup` from `internal/service` to avoid an import cycle. Do not reference a non-existent `config.Config.DataDir` field and do not invent a different `./data` fallback.

- [ ] **Step 8: Run all store tests and verify GREEN**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestOpenAIGeneratedImageStore" -count=1
```

Expected: PASS.

---

### Task 2: Download Handler And Frontend Bypass

**Files:**
- Create: `backend/internal/handler/generated_image_handler.go`
- Create: `backend/internal/handler/generated_image_handler_test.go`
- Modify: `backend/internal/handler/handler.go`
- Modify: `backend/internal/handler/wire.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/internal/server/routes/common.go`
- Modify: `backend/internal/server/router.go`
- Modify generated DI: `backend/cmd/server/wire_gen.go`
- Modify tests: `backend/internal/service/openai_ws_protocol_forward_test.go`
- Modify tests: `backend/internal/service/openai_gateway_record_usage_test.go`
- Modify: `backend/internal/web/embed_on.go`
- Create: `backend/internal/web/embed_bypass.go`
- Create: `backend/internal/web/embed_bypass_test.go`

- [ ] **Step 1: Write failing handler tests**

Add tests in `generated_image_handler_test.go`:

```go
var handlerPNGBytes = []byte("\x89PNG\r\n\x1a\nhandler-image-bytes")

type stubGeneratedImageStore struct {
	rec  service.OpenAIGeneratedImageRecord
	data []byte
	err  error
}

func (s *stubGeneratedImageStore) LoadByFilename(ctx context.Context, filename string) (service.OpenAIGeneratedImageRecord, []byte, error) {
	return s.rec, s.data, s.err
}

func TestGeneratedImageHandler_DownloadsImageWithSafeHeaders(t *testing.T) {
	store := &stubGeneratedImageStore{rec: service.OpenAIGeneratedImageRecord{Filename: "img_abcdefghijklmnopqrstuvwxyzABCDEF.png", MIME: "image/png"}, data: handlerPNGBytes}
	h := NewGeneratedImageHandler(store)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "filename", Value: "img_abcdefghijklmnopqrstuvwxyzABCDEF.png"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/sub2api/generated-images/img_abcdefghijklmnopqrstuvwxyzABCDEF.png", nil)
	h.Download(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")
	require.Equal(t, handlerPNGBytes, rec.Body.Bytes())
}

func TestGeneratedImageHandler_RejectsInvalidFilename(t *testing.T) {
	h := NewGeneratedImageHandler(&stubGeneratedImageStore{err: service.ErrOpenAIGeneratedImageInvalid})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "filename", Value: "../x.png"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/sub2api/generated-images/../x.png", nil)
	h.Download(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGeneratedImageHandler_ExpiredReturnsGone(t *testing.T) {
	h := NewGeneratedImageHandler(&stubGeneratedImageStore{err: service.ErrOpenAIGeneratedImageExpired})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "filename", Value: "img_abcdefghijklmnopqrstuvwxyzABCDEF.png"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/sub2api/generated-images/img_abcdefghijklmnopqrstuvwxyzABCDEF.png", nil)
	h.Download(c)
	require.Equal(t, http.StatusGone, rec.Code)
}

func TestGeneratedImageHandler_MetadataMissingOrCorruptDoesNotLeakPath(t *testing.T) {
	h := NewGeneratedImageHandler(&stubGeneratedImageStore{err: service.ErrOpenAIGeneratedImageNotFound})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "filename", Value: "img_abcdefghijklmnopqrstuvwxyzABCDEF.png"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/sub2api/generated-images/img_abcdefghijklmnopqrstuvwxyzABCDEF.png", nil)
	h.Download(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NotContains(t, rec.Body.String(), "openai-generated-images")
}
```

Use a fake store interface:

```go
type generatedImageStore interface {
	LoadByFilename(ctx context.Context, filename string) (service.OpenAIGeneratedImageRecord, []byte, error)
}
```

- [ ] **Step 2: Run handler tests and verify RED**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/handler -run TestGeneratedImageHandler -count=1
```

Expected: FAIL because handler does not exist.

- [ ] **Step 3: Implement handler**

Create handler with:

```go
type GeneratedImageHandler struct {
	store generatedImageStore
}

func NewGeneratedImageHandler(store generatedImageStore) *GeneratedImageHandler
func (h *GeneratedImageHandler) Download(c *gin.Context)
```

Response rules:
- `200`: `Data` with record MIME and bytes.
- `Content-Disposition: attachment; filename="<record.Filename>"`.
- `X-Content-Type-Options: nosniff`.
- invalid/missing: `404`.
- expired: `410`.

- [ ] **Step 4: Run handler tests and verify GREEN**

Run command from Step 2.

Expected: PASS.

- [ ] **Step 5: Add route and frontend bypass tests**

Move `shouldBypassEmbeddedFrontend` from `embed_on.go` to a new no-build-tag file `embed_bypass.go`, then add `embed_bypass_test.go`:

```go
func TestEmbeddedFrontendBypassGeneratedImages(t *testing.T) {
	require.True(t, shouldBypassEmbeddedFrontend("/sub2api/generated-images/img_abcdefghijklmnopqrstuvwxyzABCDEF.png"))
	require.False(t, shouldBypassEmbeddedFrontend("/sub2api/other"))
	require.False(t, shouldBypassEmbeddedFrontend("/sub2api/generated-images-malicious/img_abcdefghijklmnopqrstuvwxyzABCDEF.png"))
}
```

- [ ] **Step 6: Run bypass test and verify RED**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/web -run TestEmbeddedFrontendBypassGeneratedImages -count=1
```

Expected: FAIL until bypass is added.

- [ ] **Step 7: Register route and bypass**

Modify `handler.Handlers` to add `GeneratedImages *GeneratedImageHandler`. Modify `handler/wire.go` and `service/wire.go` so one shared `OpenAIGeneratedImageStore` instance is provided to both `OpenAIGatewayService` and `GeneratedImageHandler`. Modify `OpenAIGatewayService` and `NewOpenAIGatewayService` to accept and store `generatedImageStore *OpenAIGeneratedImageStore`.

Because `generatedImageStore` is a handler-package interface, add an explicit provider so Wire can compile:

```go
func ProvideGeneratedImageHandler(store *service.OpenAIGeneratedImageStore) *GeneratedImageHandler {
	return NewGeneratedImageHandler(store)
}
```

Add `ProvideGeneratedImageHandler` to `handler.ProviderSet` and pass `generatedImageHandler *GeneratedImageHandler` through `ProvideHandlers`.

Add `newOpenAITestGatewayServiceWithGeneratedImageStore(t, upstream)` in `openai_gateway_service_test.go` or set `generatedImageStore: newTestOpenAIGeneratedImageStore(t, fixedNow)` in individual OpenCode image tests. Existing non-image tests may keep the current helper, but OpenCode image rewrite/rehydrate paths must never run with a nil store.

Search and update every direct constructor call:

```powershell
Select-String -Path "internal/**/*.go" -Pattern "NewOpenAIGatewayService\("
```

At minimum update `backend/internal/service/openai_ws_protocol_forward_test.go` and `backend/internal/service/openai_gateway_record_usage_test.go` to pass `nil` or a test store as the new final argument, depending on whether the test exercises OpenCode image behavior.

Modify routing to register:

```go
func RegisterCommonRoutes(r *gin.Engine, h *handler.Handlers) {
	r.GET("/sub2api/generated-images/:filename", h.GeneratedImages.Download)
	// existing health/setup/event routes stay unchanged
}
```

Update `backend/internal/server/router.go` from `routes.RegisterCommonRoutes(r)` to `routes.RegisterCommonRoutes(r, h)`. Avoid adding API key middleware.

Modify embedded frontend bypass to match only `/sub2api/generated-images/`.

Run Wire generation after constructor/provider changes:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" run -mod=mod github.com/google/wire/cmd/wire ./cmd/server
```

- [ ] **Step 8: Run route and web tests**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/handler -run TestGeneratedImageHandler -count=1
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/web -run TestEmbeddedFrontendBypassGeneratedImages -count=1
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/server/... ./cmd/server -run "Test.*GeneratedImage|Test.*Wire" -count=1
```

Expected: PASS.

---

### Task 3: OpenCode JSON Output Rewrite

**Files:**
- Create: `backend/internal/service/openai_opencode_image_rewrite.go`
- Create: `backend/internal/service/openai_opencode_image_rewrite_test.go`
- Modify: `backend/internal/service/openai_gateway_service.go`

- [ ] **Step 1: Write failing non-stream rewrite test**

Add:

```go
func TestRewriteOpenCodeImageGenerationOutput_ReplacesImageCallWithMessage(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}`)
	patched, changed, err := rewriteOpenCodeImageGenerationOutput(context.Background(), body, store, openCodeImageRewriteOptions{BaseURL:"https://example.com"})
	require.NoError(t, err)
	require.True(t, changed)
	require.Regexp(t, `^msg_sub2api_img_`, gjson.GetBytes(patched, "output.0.id").String())
	require.Equal(t, "message", gjson.GetBytes(patched, "output.0.type").String())
	require.Equal(t, "completed", gjson.GetBytes(patched, "output.0.status").String())
	require.Equal(t, "assistant", gjson.GetBytes(patched, "output.0.role").String())
	require.Equal(t, "output_text", gjson.GetBytes(patched, "output.0.content.0.type").String())
	require.Equal(t, int64(0), gjson.GetBytes(patched, "output.0.content.0.annotations.#").Int())
	require.Contains(t, gjson.GetBytes(patched, "output.0.content.0.text").String(), "sub2api-image://img_")
	require.Contains(t, gjson.GetBytes(patched, "output.0.content.0.text").String(), "https://example.com/sub2api/generated-images/")
	require.NotContains(t, string(patched), "image_generation_call")
	require.NotContains(t, string(patched), pngB64)
}
```

- [ ] **Step 2: Run rewrite test and verify RED**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run TestRewriteOpenCodeImageGenerationOutput_ReplacesImageCallWithMessage -count=1
```

Expected: FAIL because rewrite helper does not exist.

- [ ] **Step 3: Implement rewrite helper**

Implement:

```go
type openCodeImageRewriteOptions struct { BaseURL string }
func rewriteOpenCodeImageGenerationOutput(ctx context.Context, body []byte, store *OpenAIGeneratedImageStore, opts openCodeImageRewriteOptions) ([]byte, bool, error)
func buildOpenCodeGeneratedImageMessage(rec OpenAIGeneratedImageRecord, opts openCodeImageRewriteOptions) map[string]any
func (s *OpenAIGatewayService) resolveOpenCodeImageDownloadBaseURL(ctx context.Context, c *gin.Context) string
func resolveOpenCodeImageDownloadBaseURL(c *gin.Context, cfg *config.Config) string
```

Rules:
- Preserve non-image output items.
- Keep existing `web_search_call` filtering behavior.
- Replace image item with complete message schema.
- After a successful generated-image message, append a synthetic `bash` `function_call` whose command only echoes a continuation hint. This keeps OpenCode in the tool-call loop so the next model step can finish the user's original instruction, such as saving the image locally.
- Do not include base64 in text.
- Resolve absolute download URL with this priority: public `api_base_url` setting with trailing `/v1` removed, `cfg.Server.FrontendURL`, trusted forwarded host, trusted request Host. If Host is untrusted, omit the absolute `Download URL:` line and keep only `Server download path (not a local file):`.
- Marker text must explicitly say the server download path is not a local filesystem path, so OpenCode agents do not try to read `/sub2api/...` from their local disk. If no absolute download URL is available, tell the agent to ask for the sub2api base URL before downloading.
- Do not trust arbitrary `Host`, `X-Forwarded-Host`, or `X-Forwarded-Proto` values. If no configured public base URL is available and the request is not from configured trusted proxy context, return an empty base URL and use relative-only marker text.

- [ ] **Step 3b: Add URL builder tests**

Add tests proving the gateway resolver prefers public `api_base_url` over `cfg.Server.FrontendURL` and removes a trailing `/v1`, while the config fallback still prefers `cfg.Server.FrontendURL`, rejects untrusted Host fallback by returning empty base URL, and allows relative-only marker text when base URL is empty:

```go
func TestResolveOpenCodeImageDownloadBaseURL_PrefersConfiguredFrontendURL(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Host = "attacker.example"
	cfg := &config.Config{}
	cfg.Server.FrontendURL = "https://sub2api.example/app/"
	require.Equal(t, "https://sub2api.example/app", resolveOpenCodeImageDownloadBaseURL(c, cfg))
}

func TestOpenAIGatewayResolveOpenCodeImageDownloadBaseURL_PrefersPublicAPIBaseURL(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Host = "attacker.example"
	cfg := &config.Config{}
	cfg.Server.FrontendURL = "https://frontend.example/app/"
	svc := &OpenAIGatewayService{
		cfg: cfg,
		publicSettingsProvider: &fakeOpenCodePublicSettingsProvider{
			settings: &PublicSettings{APIBaseURL: "https://api.example.com/v1/"},
		},
	}
	require.Equal(t, "https://api.example.com", svc.resolveOpenCodeImageDownloadBaseURL(context.Background(), c))
}

func TestResolveOpenCodeImageDownloadBaseURL_RejectsUntrustedHostFallback(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Host = "attacker.example"
	require.Equal(t, "", resolveOpenCodeImageDownloadBaseURL(c, &config.Config{}))
}

func TestBuildOpenCodeGeneratedImageMessage_UsesRelativeOnlyWhenBaseURLEmpty(t *testing.T) {
	rec := OpenAIGeneratedImageRecord{ID:"img_abcdefghijklmnopqrstuvwxyzABCDEF", Filename:"img_abcdefghijklmnopqrstuvwxyzABCDEF.png", Format:"png", MIME:"image/png", ExpiresAt:fixedNow.Add(time.Hour)}
	msg := buildOpenCodeGeneratedImageMessage(rec, openCodeImageRewriteOptions{})
	content := msg["content"].([]any)[0].(map[string]any)["text"].(string)
	require.Contains(t, content, "sub2api-image://img_abcdefghijklmnopqrstuvwxyzABCDEF")
	require.Contains(t, content, "Server download path (not a local file): /sub2api/generated-images/img_abcdefghijklmnopqrstuvwxyzABCDEF.png")
	require.Contains(t, content, "Do not treat the server download path as a local filesystem path.")
	require.Contains(t, content, "If no Download URL is shown, ask for the sub2api base URL before downloading.")
	require.NotContains(t, content, "Download URL:")
}
```

- [ ] **Step 3c: Add continuation loop tests**

Add tests proving OpenCode image rewrite adds a synthetic `bash` `function_call` after successful image messages and that SSE output emits matching function-call frames. Assert the synthetic command mentions the preceding `Download URL`, tells the model to continue the user's original request, and never includes raw image base64.

- [ ] **Step 4: Run rewrite test and verify GREEN**

Run command from Step 2.

Expected: PASS.

- [ ] **Step 5: Add non-OpenCode and no-result tests**

Add tests proving:
- Image item without `result` becomes ordinary text explaining no image result.
- Non-OpenCode path still returns `image_generation_call` via Task 0 and must not call this OpenCode-only rewrite helper.

```go
func TestRewriteOpenCodeImageGenerationOutput_ImageCallWithoutResultBecomesText(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed"}],"usage":{"input_tokens":1,"output_tokens":2}}`)
	patched, changed, err := rewriteOpenCodeImageGenerationOutput(context.Background(), body, store, openCodeImageRewriteOptions{BaseURL:"https://example.com"})
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(patched, "output.0.type").String())
	require.Contains(t, gjson.GetBytes(patched, "output.0.content.0.text").String(), "no image result")
	require.NotContains(t, string(patched), "image_generation_call")
}

func TestHandleNonStreamingResponse_NonOpenCodePreservesImageGenerationJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "curl/8.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}`)),
	}
	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform:PlatformOpenAI, Type:AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	require.Equal(t, "image_generation_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, pngB64, gjson.Get(rec.Body.String(), "output.0.result").String())
}

func TestHandleSSEToJSON_OpenCodeRewritesImageFromDoneWhenCompletedOutputEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	cfg := &config.Config{}
	cfg.Server.FrontendURL = "https://example.com"
	svc := &OpenAIGatewayService{cfg: cfg, generatedImageStore: store}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))),
	}
	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform:PlatformOpenAI, Type:AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String(), "sub2api-image://img_")
	require.Contains(t, rec.Body.String(), "https://example.com/sub2api/generated-images/")
	require.NotContains(t, rec.Body.String(), "image_generation_call")
	require.NotContains(t, rec.Body.String(), pngB64)
}
```

- [ ] **Step 6: Wire non-stream OpenCode response path**

Modify `handleNonStreamingResponse` and `handleSSEToJSONForAccount` where `isOpenCodeResponsesClient(c)` currently calls `sanitizeOpenCodeResponsesOutput`. Call the new rewrite helper before writing response to client.

- [ ] **Step 7: Run focused gateway tests**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestRewriteOpenCodeImageGenerationOutput|TestHandleSSEToJSON_OpenCode|TestHandleNonStreamingResponse_NonOpenCodePreservesImageGenerationJSON" -count=1
```

Expected: PASS.

---

### Task 4: OpenCode SSE Rewrite

**Files:**
- Create: `backend/internal/service/openai_opencode_image_sse.go`
- Create: `backend/internal/service/openai_opencode_image_sse_test.go`
- Modify: `backend/internal/service/openai_gateway_service.go`

- [ ] **Step 1: Write failing SSE rewrite test**

Add:

```go
func TestFilterOpenCodeResponsesSSEFrame_RewritesImageGenerationToMessage(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":2,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
		"",
	}
	out, _, _, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{BaseURL:"https://example.com"})
	require.True(t, keep)
	require.Contains(t, out, "response.output_item.added")
	require.Contains(t, out, "response.content_part.added")
	require.Contains(t, out, "response.output_text.delta")
	require.Contains(t, out, "response.output_text.done")
	require.Contains(t, out, "response.content_part.done")
	require.Contains(t, out, "response.output_item.done")
	require.Contains(t, out, `"output_index":2`)
	require.Contains(t, out, "sub2api-image://img_")
	require.NotContains(t, out, "image_generation_call")
	require.NotContains(t, out, pngB64)
}

func TestFilterOpenCodeResponsesSSEFrame_DropsImageProgressAndAdded(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, eventType := range []string{
		"response.image_generation_call.in_progress",
		"response.image_generation_call.generating",
		"response.image_generation_call.partial_image",
		"response.image_generation_call.completed",
	} {
		progressFrame := []string{
			"event: " + eventType,
			`data: {"type":"` + eventType + `","output_index":0,"partial_image_index":0,"partial_image_b64":"` + pngB64 + `"}`,
			"",
		}
		frame, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), progressFrame, store, openCodeImageRewriteOptions{})
		require.False(t, keep, eventType)
		require.True(t, hasData, eventType)
		require.Contains(t, data, eventType)
		require.Empty(t, frame)
	}

	addedFrame := []string{
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"ig_1","type":"image_generation_call","status":"in_progress"}}`,
		"",
	}
	frame, _, _, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), addedFrame, store, openCodeImageRewriteOptions{})
	require.False(t, keep)
	require.Empty(t, frame)
}

func TestFilterOpenCodeResponsesSSEFrame_ImageDoneWithoutResultEmitsText(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":2,"item":{"id":"ig_1","type":"image_generation_call","status":"completed"}}`,
		"",
	}
	out, _, _, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{})
	require.True(t, keep)
	require.Contains(t, out, "response.output_text.delta")
	require.Contains(t, out, "no image result")
	require.NotContains(t, out, "image_generation_call")
}
```

- [ ] **Step 2: Run SSE test and verify RED**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run TestFilterOpenCodeResponsesSSEFrame_RewritesImageGenerationToMessage -count=1
```

Expected: FAIL because helper does not exist.

- [ ] **Step 3: Implement SSE rewrite helper**

Implement:

```go
func filterOpenCodeResponsesSSEFrameWithImages(ctx context.Context, lines []string, store *OpenAIGeneratedImageStore, opts openCodeImageRewriteOptions) (frame string, data string, hasData bool, keep bool)
func buildOpenCodeGeneratedImageSSEFrames(rec OpenAIGeneratedImageRecord, outputIndex int, opts openCodeImageRewriteOptions) string
```

Rules:
- Drop all `response.image_generation_call.*` frames.
- Drop image `output_item.added`.
- Rewrite image `output_item.done` with result into synthetic message frames using original output index.
- Rewrite completed output image fallback before forwarding completed frame.
- If image done has no result, emit ordinary text explanation or drop safely.
- When the input frame is terminal `response.completed` / `response.done`, returned `frame` may contain synthetic frames plus patched terminal frame, but returned `data` must be the single patched terminal JSON payload used for `sawTerminalEvent`, usage extraction, and terminal response tracking. Do not return multi-frame concatenated data as `data`.

- [ ] **Step 4: Add completed-only fallback RED test**

Add test where only `response.completed.response.output[]` contains image result and assert synthetic message frames are emitted before completed, completed output no longer contains `image_generation_call`, returned `data` is patched terminal JSON, and usage extraction still sees terminal usage.

```go
func TestFilterOpenCodeResponsesSSEFrame_RewritesCompletedOutputImageAndKeepsTerminalData(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":3,"output_tokens":5}}}`,
		"",
	}
	out, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{BaseURL:"https://example.com"})
	require.True(t, keep)
	require.True(t, hasData)
	require.Contains(t, out, "response.output_item.added")
	require.Contains(t, out, "response.completed")
	require.NotContains(t, out, "image_generation_call")
	require.NotContains(t, out, pngB64)
	require.Equal(t, "response.completed", gjson.Get(data, "type").String())
	require.Equal(t, int64(3), gjson.Get(data, "response.usage.input_tokens").Int())
	require.Equal(t, "message", gjson.Get(data, "response.output.0.type").String())
}

func TestFilterOpenCodeResponsesSSEFrame_RewritesDoneOutputImageAndKeepsTerminalData(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		"event: response.done",
		`data: {"type":"response.done","response":{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":3,"output_tokens":5}}}`,
		"",
	}
	out, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{BaseURL:"https://example.com"})
	require.True(t, keep)
	require.True(t, hasData)
	require.Contains(t, out, "response.output_item.added")
	require.Contains(t, out, "response.done")
	require.NotContains(t, out, "image_generation_call")
	require.NotContains(t, out, pngB64)
	require.Equal(t, "response.done", gjson.Get(data, "type").String())
	require.Equal(t, int64(3), gjson.Get(data, "response.usage.input_tokens").Int())
	require.Equal(t, "message", gjson.Get(data, "response.output.0.type").String())
}
```

- [ ] **Step 5: Implement completed fallback and terminal patch**

Update helper so completed frame can return multiple SSE frames: synthetic message frames followed by patched completed frame.

- [ ] **Step 6: Wire streaming OpenCode response path**

Replace existing `filterOpenCodeResponsesSSEFrame` usage with image-aware helper for OpenCode clients. Preserve existing web_search filtering semantics.

When `filterOpenCodeResponsesSSEFrameWithImages` returns multi-frame synthetic output, preserve that returned `frameBody` for downstream writes. Later model replacement or tool correction may update the single `data` payload used for terminal/usage tracking, but must not rebuild `frameBody` from the original `frameLines` and accidentally discard synthetic frames. If terminal model replacement is needed, patch the terminal JSON before building the multi-frame output or replace only inside the terminal sub-frame while leaving synthetic frames intact.

Add full streaming integration tests in `openai_opencode_image_sse_test.go` or `openai_gateway_service_test.go`:

```go
func TestHandleStreamingResponse_OpenCodeRewritesImageGenerationDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg:&config.Config{}, generatedImageStore:store}
	resp := &http.Response{StatusCode:http.StatusOK, Header:http.Header{"Content-Type":[]string{"text/event-stream"}}, Body:io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":1,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		"",
		"data: [DONE]",
	}, "\n")))}
	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID:1, Platform:PlatformOpenAI, Type:AccountTypeOAuth}, time.Now(), "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	body := rec.Body.String()
	require.Contains(t, body, "response.output_item.added")
	require.Contains(t, body, "response.output_text.delta")
	require.Contains(t, body, "sub2api-image://img_")
	require.NotContains(t, body, "image_generation_call")
	require.NotContains(t, body, pngB64)
}

func TestHandleStreamingResponse_OpenCodeRewritesCompletedOnlyImageFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg:&config.Config{}, generatedImageStore:store}
	resp := &http.Response{StatusCode:http.StatusOK, Header:http.Header{"Content-Type":[]string{"text/event-stream"}}, Body:io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		"",
		"data: [DONE]",
	}, "\n")))}
	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID:1, Platform:PlatformOpenAI, Type:AccountTypeOAuth}, time.Now(), "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	body := rec.Body.String()
	require.Contains(t, body, "response.output_item.added")
	require.Contains(t, body, "response.completed")
	require.Contains(t, body, "sub2api-image://img_")
	require.NotContains(t, body, "image_generation_call")
	require.NotContains(t, body, pngB64)
}

func TestHandleStreamingResponse_OpenCodePreservesSyntheticFramesWhenReplacingModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg:&config.Config{}, generatedImageStore:store}
	resp := &http.Response{StatusCode:http.StatusOK, Header:http.Header{"Content-Type":[]string{"text/event-stream"}}, Body:io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		"",
		"data: [DONE]",
	}, "\n")))}
	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID:1, Platform:PlatformOpenAI, Type:AccountTypeOAuth}, time.Now(), "gpt-5.5", "gpt-5.1")
	require.NoError(t, err)
	body := rec.Body.String()
	require.Contains(t, body, "response.output_item.added")
	require.Contains(t, body, "sub2api-image://img_")
	require.Contains(t, body, "gpt-5.5")
	require.NotContains(t, body, "image_generation_call")
	require.NotContains(t, body, pngB64)
}
```

Add a non-OpenCode streaming test proving `image_generation_call` is not filtered or rewritten when `isOpenCodeResponsesClient(c)` is false.

```go
func TestHandleStreamingResponse_NonOpenCodeKeepsImageGenerationCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "curl/8.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
			"data: [DONE]",
		}, "\n"))),
	}
	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID:1, Platform:PlatformOpenAI, Type:AccountTypeOAuth}, time.Now(), "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	require.Contains(t, rec.Body.String(), "image_generation_call")
	require.Contains(t, rec.Body.String(), pngB64)
}
```

- [ ] **Step 7: Run SSE tests**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestFilterOpenCodeResponsesSSEFrame|TestHandleStreamingResponse_OpenCode|TestHandleStreamingResponse_NonOpenCodeKeepsImageGenerationCall" -count=1
```

Expected: PASS.

---

### Task 5: Input Marker Rehydrate And Ops Redaction

**Files:**
- Create: `backend/internal/service/openai_opencode_image_rehydrate.go`
- Extend: `backend/internal/service/openai_opencode_image_rewrite_test.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/server/middleware/request_logger.go`
- Modify: `backend/internal/server/middleware/logger.go`
- Modify tests: `backend/internal/server/middleware/request_access_logger_test.go`

- [ ] **Step 1: Write failing rehydrate test**

Add:

```go
const testImageID = "img_abcdefghijklmnopqrstuvwxyzABCDEF"

func newTestStoreWithImage(t *testing.T, id string, format string, data []byte) *OpenAIGeneratedImageStore {
	t.Helper()
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	// Test-only helper in openai_generated_image_store_test.go that writes a
	// deterministic record+file without weakening production ID generation.
	_, err := store.saveDecodedForTest(id, format, data)
	require.NoError(t, err)
	return store
}

func TestRehydrateOpenCodeGeneratedImageMarkers_AddsSyntheticInputImage(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	req := map[string]any{"input": []any{map[string]any{"role":"assistant", "content": []any{map[string]any{"type":"output_text", "text":"Generated image: sub2api-image://" + testImageID}}}}}
	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages:3})
	require.NoError(t, err)
	require.True(t, changed)
	input := req["input"].([]any)
	last := input[len(input)-1].(map[string]any)
	require.Equal(t, "user", last["role"])
	content := last["content"].([]any)
	require.Equal(t, "input_image", content[1].(map[string]any)["type"])
	require.Contains(t, content[1].(map[string]any)["image_url"], "data:image/png;base64,")
}
```

- [ ] **Step 2: Run rehydrate test and verify RED**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run TestRehydrateOpenCodeGeneratedImageMarkers_AddsSyntheticInputImage -count=1
```

Expected: FAIL because rehydrate helper does not exist.

- [ ] **Step 3: Implement marker scanner and rehydrate**

Implement:

```go
type openCodeImageRehydrateOptions struct { MaxImages int; MaxRehydrateBytes int64 }
func rehydrateOpenCodeGeneratedImageMarkers(ctx context.Context, reqBody map[string]any, store *OpenAIGeneratedImageStore, opts openCodeImageRehydrateOptions) (bool, error)
```

Scanner must cover string content, `input_text`, and `output_text`. Only insert synthetic `role:"user"` messages. Do not generate `item_reference`.

- [ ] **Step 4: Add stale/dedupe/cap RED tests**

Add tests:
- Expired marker inserts ordinary unavailable text and no `input_image`.
- Duplicate marker injects once.
- Four markers inject only the most recent three.
- Non-OpenCode caller does not call rehydrate from gateway path.
- Resource over `MaxRehydrateBytes` keeps the download reference text but does not inject `input_image`; it inserts ordinary text saying image bytes were not attached because the image is too large.
- Relative `/sub2api/generated-images/img_<id>.<ext>` download paths and absolute download URLs are also scanned and deduped.
- Non-OpenCode callers do not run rehydrate from the gateway path.

```go
func TestRehydrateOpenCodeGeneratedImageMarkers_ExpiredMarkerAddsUnavailableText(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	req := map[string]any{"input": []any{"sub2api-image://" + testImageID}}
	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages:3})
	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Contains(t, string(encoded), "image bytes unavailable")
	require.NotContains(t, string(encoded), "input_image")
}

func TestRehydrateOpenCodeGeneratedImageMarkers_DedupesAndCapsMostRecent(t *testing.T) {
	ids := []string{
		"img_abcdefghijklmnopqrstuvwxyzABCDEF",
		"img_bcdefghijklmnopqrstuvwxyzABCDEFG",
		"img_cdefghijklmnopqrstuvwxyzABCDEFGH",
		"img_defghijklmnopqrstuvwxyzABCDEFGHI",
	}
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, id := range ids {
		_, err := store.saveDecodedForTest(id, "png", pngBytes)
		require.NoError(t, err)
	}
	req := map[string]any{"input": []any{strings.Join([]string{"sub2api-image://" + ids[0], "sub2api-image://" + ids[1], "sub2api-image://" + ids[1], "sub2api-image://" + ids[2], "sub2api-image://" + ids[3]}, " ")}}
	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages:3})
	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Equal(t, 3, strings.Count(string(encoded), `"type":"input_image"`))
}

func TestRehydrateOpenCodeGeneratedImageMarkers_TooLargeSkipsInputImage(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	req := map[string]any{"input": []any{"sub2api-image://" + testImageID}}
	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages:3, MaxRehydrateBytes:4})
	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Contains(t, string(encoded), "image bytes were not attached because the image is too large")
	require.NotContains(t, string(encoded), "input_image")
}

func TestRehydrateOpenCodeGeneratedImageMarkers_ScansDownloadPathsAndAbsoluteURLs(t *testing.T) {
	ids := []string{"img_abcdefghijklmnopqrstuvwxyzABCDEF", "img_bcdefghijklmnopqrstuvwxyzABCDEFG"}
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, id := range ids {
		_, err := store.saveDecodedForTest(id, "png", pngBytes)
		require.NoError(t, err)
	}
	req := map[string]any{"input": []any{map[string]any{"role":"assistant", "content": []any{map[string]any{"type":"output_text", "text":"Download path: /sub2api/generated-images/" + ids[0] + ".png\nDownload: https://example.com/sub2api/generated-images/" + ids[1] + ".png"}}}}}
	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages:3})
	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Equal(t, 2, strings.Count(string(encoded), `"type":"input_image"`))
}

func TestForwardResponsesRequest_NonOpenCodeDoesNotRehydrateGeneratedImageMarker(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	body := []byte(`{"model":"gpt-5.5","input":"sub2api-image://` + testImageID + `"}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "curl/8.0")
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = store
	account := &Account{Platform:PlatformOpenAI, Type:AccountTypeAPIKey, Credentials:map[string]any{"api_key":"test-key"}}
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Contains(t, string(upstream.lastBody), "sub2api-image://"+testImageID)
	require.NotContains(t, string(upstream.lastBody), "data:image")
}
```

- [ ] **Step 5: Implement stale/dedupe/cap behavior**

Use stable marker regexes for `sub2api-image://(img_[A-Za-z0-9_-]{32,})`, `/sub2api/generated-images/(img_[A-Za-z0-9_-]{32,})\.(png|jpe?g|webp)`, and absolute HTTP(S) URLs whose path matches that download route. Preserve input order after dedupe. For stale marker text, use the exact phrase `image bytes unavailable` so tests can assert it.

- [ ] **Step 6: Add ops redaction RED test**

Add or extend a gateway test proving that after rehydrate, `OpsUpstreamRequestBodyKey` / request detail capture does not contain `data:image` or the original base64.

Add a download log redaction test proving access/request/ops logging does not persist the full `img_<opaque>` token; it should record a route template or redacted path such as `/sub2api/generated-images/[redacted]`.

```go
func TestRedactOpenCodeGeneratedImagesForOps_RemovesDataURLsAndResults(t *testing.T) {
	body := []byte(`{"input":[{"content":[{"type":"input_image","image_url":"data:image/png;base64,` + pngB64 + `"}]}],"output":[{"type":"image_generation_call","result":"` + pngB64 + `"}]}`)
	redacted := redactOpenCodeGeneratedImagesForOps(body)
	require.NotContains(t, string(redacted), "data:image")
	require.NotContains(t, string(redacted), pngB64)
	require.Contains(t, string(redacted), "[redacted-input-image]")
	require.Contains(t, string(redacted), "[redacted-image-result]")
}

func TestForwardResponsesRequest_OpenCodeRehydrateRedactsOpsUpstreamBody(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	body := []byte(`{"model":"gpt-5.5","input":"sub2api-image://` + testImageID + `"}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = store
	account := &Account{Platform:PlatformOpenAI, Type:AccountTypeAPIKey, Credentials:map[string]any{"api_key":"test-key"}}
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	v, ok := c.Get(OpsUpstreamRequestBodyKey)
	require.True(t, ok)
	opsBody := string(v.([]byte))
	require.NotContains(t, opsBody, "data:image")
	require.NotContains(t, opsBody, pngB64)
	require.Contains(t, opsBody, "[redacted-input-image]")
	require.Contains(t, string(upstream.lastBody), "data:image/png;base64,")
}
```

Add this middleware-package test in `backend/internal/server/middleware/request_access_logger_test.go`:

```go
func TestRedactGeneratedImagePathForLogs(t *testing.T) {
	require.Equal(t, "/sub2api/generated-images/[redacted]", redactGeneratedImagePathForLogs("/sub2api/generated-images/"+testGeneratedImageLogID+".png"))
	require.Equal(t, "/v1/responses", redactGeneratedImagePathForLogs("/v1/responses"))
}
```

Use a local test constant in the middleware package:

```go
const testGeneratedImageLogID = "img_abcdefghijklmnopqrstuvwxyzABCDEF"
```


- [ ] **Step 7: Implement redacted upstream body capture**

Before `setOpsUpstreamRequestBody`, create a redacted copy of the upstream body. Replace `input_image.image_url` values that start with `data:` with `[redacted-input-image]`; replace any `image_generation_call.result` with `[redacted-image-result]`; redact generated image URL tokens in request/log payloads. Apply `redactGeneratedImagePathForLogs` in `internal/server/middleware/request_logger.go` and `internal/server/middleware/logger.go` before adding the `path` zap field.

- [ ] **Step 8: Wire request path**

In `OpenAIGatewayService.Forward`, call rehydrate only for OpenCode Responses clients after local builtin carriers are consumed, before `applyCodexOAuthTransform`, before `buildUpstreamRequest`, before `validateCodexSparkInput`, and before `setOpsUpstreamRequestBody`. Remove or rewrite the stale unconditional `TestForwardResponsesRequest_CodexClientAutoInjectsImageGenerationTool`; image_generation enablement should be carrier-driven in first version.

- [ ] **Step 9: Run rehydrate tests**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestRehydrateOpenCodeGeneratedImageMarkers|TestForwardResponsesRequest|TestRedactOpenCodeGeneratedImagesForOps" -count=1
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/server/middleware -run TestRedactGeneratedImagePathForLogs -count=1
```

Expected: PASS.

---

### Task 6: Frontend Config Guard And Integration Verification

**Files:**
- Keep: `frontend/src/components/keys/UseKeyModal.vue`
- Keep or modify tests: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
- Modify if needed: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: Run frontend focused test baseline**

Run from frontend workdir:

```powershell
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
```

Expected: PASS with GPT-5.5 OpenCode config containing `metadata.builtin_tools.image_generation`.

- [ ] **Step 2: Add backend integration test for carrier-triggered image_generation**

Remove the unconditional Codex/OpenCode auto-injection block from `OpenAIGatewayService.Forward`:

```go
// remove this first-version behavior; image_generation is carrier-driven
if isCodexCLI && ensureOpenAIResponsesImageGenerationTool(reqBody) {
	// delete this entire block, including bodyModified/disablePatch/log side effects
}
```

Replace any stale unconditional auto-inject test with carrier-driven tests:

```go
func TestForwardResponsesRequest_OpenCodeMetadataImageGenerationCarrierAddsToolAndStripsMetadata(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5-Sys","metadata":{"builtin_tools":{"image_generation":{"model":"gpt-image-2","size":"1024x1024","output_format":"png"}},"client":"opencode"},"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = newTestOpenAIGeneratedImageStore(t, fixedNow)
	account := &Account{Platform:PlatformOpenAI, Type:AccountTypeAPIKey, Credentials:map[string]any{"api_key":"test-key"}}
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "metadata")
	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 2)
	imageTool := tools[1].(map[string]any)
	require.Equal(t, "image_generation", imageTool["type"])
	require.Equal(t, "gpt-image-2", imageTool["model"])
	require.Equal(t, "1024x1024", imageTool["size"])
	require.Equal(t, "png", imageTool["output_format"])
}

func TestForwardResponsesRequest_CodexWithoutCarrierDoesNotInjectImageGenerationTool(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5-Sys","tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "Codex Desktop/1.2.3")
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{Platform:PlatformOpenAI, Type:AccountTypeAPIKey, Credentials:map[string]any{"api_key":"test-key"}}
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 1)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
}
```

The OpenCode response rewrite protection itself is covered by `TestHandleSSEToJSON_OpenCodeRewritesImageFromDoneWhenCompletedOutputEmpty` and `TestRewriteOpenCodeImageGenerationOutput_ReplacesImageCallWithMessage`.

- [ ] **Step 3: Run backend focused integration tests**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestForwardResponsesRequest_.*ImageGeneration|TestHandleSSEToJSON_.*Image|TestOpenCode" -count=1
```

Expected: PASS.

- [ ] **Step 4: Run full required verification**

Run from backend workdir first, then frontend workdir for `pnpm` commands, then backend workdir again for embed build:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service ./internal/pkg/apicompat -count=1
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/server/... ./internal/handler/... -count=1
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" build ./cmd/server
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
pnpm typecheck
pnpm build
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags embed ./internal/web ./cmd/server -count=1
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" build -tags embed ./cmd/server
rtk git diff --check
```

Expected: all commands exit 0. Existing Vite warnings are acceptable if build exits 0.

---

## Plan Self-Review Checklist

- Spec coverage: tasks cover store, download, frontend bypass, JSON rewrite, SSE rewrite, rehydrate, stale marker, ops redaction, frontend config, non-OpenCode preservation.
- Placeholder scan: no unfinished placeholder markers or deferred behavior is present.
- Type consistency: store record/input/helper names are consistent across tasks.
- TDD: each production behavior has a failing test step before implementation.
- Commits: this plan intentionally does not include `git commit` steps because commits require explicit user instruction in this environment.
