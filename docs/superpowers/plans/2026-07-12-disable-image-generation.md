# Global Image Generation Disable Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Add DISABLE_IMAGE_GENERATION=true so text-capable OpenAI /responses requests have image tools removed before the group gate while image-only and dedicated image requests remain forbidden.

**Architecture:** Add one top-level Viper flag and reuse the existing canonical raw-payload image-tool stripper. Normalize at HTTP and WebSocket ingress before image-intent checks, prevent account/channel bridge logic from re-injecting tools, and make dedicated image handlers fail closed. Preserve current behavior when the flag is false.

**Tech Stack:** Go 1.26, Gin, Viper, coder/websocket, testify, Docker Compose, PostgreSQL, Redis

## Global Constraints

- Base code and image on v0.1.151 commit deff3123ded1d14e51df1fd1286e3d43ed9ec9bd.
- DISABLE_IMAGE_GENERATION defaults to false.
- Text-model /responses requests continue after image declarations are removed.
- Image-only models, OpenAI image endpoints, and Grok generation endpoints remain forbidden.
- Cover HTTP, WebSocket first frames, and WebSocket follow-up response.create frames.
- Never log request bodies, API keys, credentials, prompts, or generated content.
- Do not add a database migration or modify account/group records.
- Deploy an immutable custom image tag; never overwrite weishaw/sub2api:latest.

---

### Task 1: Add The Global Configuration Flag

**Files:**
- Modify: backend/internal/config/config.go
- Test: backend/internal/config/config_test.go

**Interfaces:**
- Consumes: Viper AutomaticEnv and the existing dot-to-underscore replacer.
- Produces: config.Config.DisableImageGeneration bool mapped from disable_image_generation and DISABLE_IMAGE_GENERATION.

- [ ] **Step 1: Write the failing configuration test**

~~~go
func TestLoadDisableImageGeneration(t *testing.T) {
    t.Run("defaults to false", func(t *testing.T) {
        resetViperWithJWTSecret(t)
        cfg, err := Load()
        require.NoError(t, err)
        require.False(t, cfg.DisableImageGeneration)
    })

    t.Run("reads top level environment variable", func(t *testing.T) {
        resetViperWithJWTSecret(t)
        t.Setenv("DISABLE_IMAGE_GENERATION", "true")
        cfg, err := Load()
        require.NoError(t, err)
        require.True(t, cfg.DisableImageGeneration)
    })
}
~~~

- [ ] **Step 2: Verify RED**

Run:

~~~bash
cd backend
go test ./internal/config -run '^TestLoadDisableImageGeneration$' -count=1
~~~

Expected: compilation fails because Config.DisableImageGeneration is undefined.

- [ ] **Step 3: Add the field and default**

Add to Config:

~~~go
DisableImageGeneration bool `mapstructure:"disable_image_generation"`
~~~

Add in setDefaults before Gateway defaults:

~~~go
viper.SetDefault("disable_image_generation", false)
~~~

- [ ] **Step 4: Verify GREEN and package regression tests**

~~~bash
cd backend
go test ./internal/config -run '^TestLoadDisableImageGeneration$' -count=1
go test ./internal/config -count=1
~~~

Expected: both commands pass.

- [ ] **Step 5: Commit**

~~~bash
git add backend/internal/config/config.go backend/internal/config/config_test.go
git commit -m "feat: add global image generation disable config"
~~~

---

### Task 2: Expose The Canonical Payload Stripper

**Files:**
- Modify: backend/internal/service/openai_codex_transform.go
- Test: backend/internal/service/openai_ws_forwarder_ingress_test.go

**Interfaces:**
- Consumes: private stripOpenAIImageGenerationToolsFromRawPayload.
- Produces: service.StripOpenAIImageGenerationToolsFromRawPayload(payload []byte) ([]byte, bool, error).

- [ ] **Step 1: Write the failing exported-wrapper test**

~~~go
func TestStripOpenAIImageGenerationToolsFromRawPayloadExported(t *testing.T) {
    payload := []byte(`{"model":"gpt-5.5","tools":[{"type":"function","name":"shell"},{"type":"image_generation"}],"tool_choice":{"type":"image_generation"}}`)

    updated, changed, err := StripOpenAIImageGenerationToolsFromRawPayload(payload)

    require.NoError(t, err)
    require.True(t, changed)
    require.False(t, IsImageGenerationIntent(openAIResponsesEndpoint, "gpt-5.5", updated))
    require.True(t, gjson.GetBytes(updated, `tools.#(name=="shell")`).Exists())
}
~~~

- [ ] **Step 2: Verify RED**

~~~bash
cd backend
go test ./internal/service -run '^TestStripOpenAIImageGenerationToolsFromRawPayloadExported$' -count=1
~~~

Expected: compilation fails because the exported function is undefined.

- [ ] **Step 3: Add the minimal wrapper**

~~~go
// StripOpenAIImageGenerationToolsFromRawPayload removes Responses image tool
// declarations while preserving unrelated request fields and tools.
func StripOpenAIImageGenerationToolsFromRawPayload(payload []byte) ([]byte, bool, error) {
    return stripOpenAIImageGenerationToolsFromRawPayload(payload)
}
~~~

- [ ] **Step 4: Verify existing and exported stripper behavior**

~~~bash
cd backend
go test ./internal/service -run '^(TestStripOpenAIImageGenerationToolsFromRawPayload|TestStripOpenAIImageGenerationToolsFromRawPayloadExported)$' -count=1
~~~

Expected: flat tools, namespaces, input.additional_tools, tool_choice, no-op, and exported wrapper cases pass.

- [ ] **Step 5: Commit**

~~~bash
git add backend/internal/service/openai_codex_transform.go backend/internal/service/openai_ws_forwarder_ingress_test.go
git commit -m "refactor: expose responses image tool stripper"
~~~

---

### Task 3: Normalize HTTP Responses Before The Group Gate

**Files:**
- Create: backend/internal/handler/image_generation_global_disable.go
- Create: backend/internal/handler/image_generation_global_disable_test.go
- Modify: backend/internal/handler/openai_gateway_handler.go
- Test: backend/internal/handler/image_concurrency_limiter_test.go

**Interfaces:**
- Consumes: Config.DisableImageGeneration and the exported service stripper.
- Produces: imageGenerationGloballyDisabled() bool and normalizeGloballyDisabledImageGeneration([]byte) ([]byte, bool, error).

- [ ] **Step 1: Write failing helper tests**

~~~go
package handler

import (
    "testing"

    "github.com/Wei-Shaw/sub2api/internal/config"
    "github.com/Wei-Shaw/sub2api/internal/service"
    "github.com/stretchr/testify/require"
    "github.com/tidwall/gjson"
)

func TestNormalizeGloballyDisabledImageGeneration(t *testing.T) {
    payload := []byte(`{"model":"gpt-5.5","input":[{"type":"message","role":"user","content":"hello"},{"type":"additional_tools","tools":[{"type":"namespace","name":"image_gen"}]}],"tools":[{"type":"function","name":"shell"},{"type":"image_generation"}],"tool_choice":{"type":"image_generation"}}`)
    h := &OpenAIGatewayHandler{cfg: &config.Config{DisableImageGeneration: true}}

    updated, changed, err := h.normalizeGloballyDisabledImageGeneration(payload)

    require.NoError(t, err)
    require.True(t, changed)
    require.False(t, service.IsImageGenerationIntent("/v1/responses", "gpt-5.5", updated))
    require.True(t, gjson.GetBytes(updated, `tools.#(name=="shell")`).Exists())
    require.Equal(t, "hello", gjson.GetBytes(updated, "input.0.content").String())
}

func TestNormalizeGloballyDisabledImageGenerationNoOp(t *testing.T) {
    payload := []byte(`{"model":"gpt-5.5","tools":[{"type":"image_generation"}]}`)
    h := &OpenAIGatewayHandler{cfg: &config.Config{}}

    updated, changed, err := h.normalizeGloballyDisabledImageGeneration(payload)

    require.NoError(t, err)
    require.False(t, changed)
    require.Equal(t, payload, updated)
}
~~~

- [ ] **Step 2: Verify RED**

~~~bash
cd backend
go test ./internal/handler -run '^TestNormalizeGloballyDisabledImageGeneration' -count=1
~~~

Expected: compilation fails because the methods are undefined.

- [ ] **Step 3: Implement the helper**

~~~go
package handler

import "github.com/Wei-Shaw/sub2api/internal/service"

func (h *OpenAIGatewayHandler) imageGenerationGloballyDisabled() bool {
    return h != nil && h.cfg != nil && h.cfg.DisableImageGeneration
}

func (h *OpenAIGatewayHandler) normalizeGloballyDisabledImageGeneration(payload []byte) ([]byte, bool, error) {
    if !h.imageGenerationGloballyDisabled() {
        return payload, false, nil
    }
    return service.StripOpenAIImageGenerationToolsFromRawPayload(payload)
}
~~~

- [ ] **Step 4: Verify helper GREEN**

~~~bash
cd backend
go test ./internal/handler -run '^TestNormalizeGloballyDisabledImageGeneration' -count=1
~~~

Expected: both helper tests pass.

- [ ] **Step 5: Write failing handler regression tests**

Add this concrete fixture and the two regression tests:

~~~go
func newGlobalImageDisableResponsesTestContext(t *testing.T, body string) (*httptest.ResponseRecorder, *gin.Context, *OpenAIGatewayHandler) {
    t.Helper()
    gin.SetMode(gin.TestMode)
    rec := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rec)
    c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
    groupID := int64(1)
    c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
        ID: 10,
        GroupID: &groupID,
        Group: &service.Group{ID: groupID, AllowImageGeneration: false},
        User: &service.User{ID: 20},
    })
    c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 20, Concurrency: 1})
    cfg := &config.Config{RunMode: config.RunModeSimple, DisableImageGeneration: true}
    h := &OpenAIGatewayHandler{
        gatewayService: &service.OpenAIGatewayService{},
        billingCacheService: service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil),
        apiKeyService: &service.APIKeyService{},
        concurrencyHelper: &ConcurrencyHelper{concurrencyService: service.NewConcurrencyService(&helperConcurrencyCacheStub{userSeq: []bool{true}})},
        cfg: cfg,
        imageLimiter: &imageConcurrencyLimiter{},
    }
    return rec, c, h
}

func TestOpenAIGatewayHandlerResponsesGlobalDisableStripsBeforeGroupGate(t *testing.T) {
    body := `{"model":"gpt-5.5","input":"write code","tools":[{"type":"image_generation"}]}`
    rec, c, h := newGlobalImageDisableResponsesTestContext(t, body)
    h.Responses(c)
    require.NotEqual(t, http.StatusForbidden, rec.Code)
    require.NotContains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
}

func TestOpenAIGatewayHandlerResponsesGlobalDisableRejectsImageOnlyModel(t *testing.T) {
    body := `{"model":"gpt-image-2","input":"draw"}`
    rec, c, h := newGlobalImageDisableResponsesTestContext(t, body)
    h.Responses(c)
    require.Equal(t, http.StatusForbidden, rec.Code)
    require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
}
~~~

- [ ] **Step 6: Verify RED**

~~~bash
cd backend
go test ./internal/handler -run '^TestOpenAIGatewayHandlerResponsesGlobalDisable' -count=1
~~~

Expected: text-model case returns the current 403; image-only case returns the intended 403.

- [ ] **Step 7: Normalize in Responses before moderation and image intent**

After compact normalization and JSON validation:

~~~go
normalizedBody, stripped, stripErr := h.normalizeGloballyDisabledImageGeneration(body)
if stripErr != nil {
    h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to normalize request body")
    return
}
if stripped {
    body = normalizedBody
    reqLog.Info("openai.image_generation_tools_stripped_global")
}
sessionHashBody = body
~~~

Keep all later parsing, moderation, image-intent checks, mapping, hashing, and forwarding on the normalized body.

- [ ] **Step 8: Verify focused handler tests**

~~~bash
cd backend
go test ./internal/handler -run '^(TestNormalizeGloballyDisabledImageGeneration|TestOpenAIGatewayHandlerResponsesGlobalDisable|TestOpenAIGatewayHandlerResponses_(ImageIntentRejectedByImageConcurrency|TextOnlyNotRejectedByImageConcurrency))' -count=1
~~~

Expected: all selected tests pass.

- [ ] **Step 9: Commit**

~~~bash
git add backend/internal/handler/image_generation_global_disable.go backend/internal/handler/image_generation_global_disable_test.go backend/internal/handler/openai_gateway_handler.go backend/internal/handler/image_concurrency_limiter_test.go
git commit -m "fix: strip image tools before responses group gate"
~~~

---

### Task 4: Cover WebSocket And Prevent Tool Re-Injection

**Files:**
- Modify: backend/internal/handler/openai_gateway_handler.go
- Modify: backend/internal/service/openai_ws_forwarder_ingress.go
- Modify: backend/internal/service/openai_gateway_forward.go
- Test: backend/internal/handler/openai_gateway_handler_test.go
- Test: backend/internal/service/openai_ws_forwarder_ingress_session_test.go

**Interfaces:**
- Consumes: handler normalizer and OpenAIGatewayService.cfg.DisableImageGeneration.
- Produces: OpenAIGatewayService.normalizeGloballyDisabledImageGeneration([]byte) plus normalized first/follow-up frames and bridge-injection guards.

- [ ] **Step 1: Write a failing service normalization test**

Add this table test to openai_ws_forwarder_ingress_test.go:

~~~go
func TestOpenAIGatewayServiceNormalizeGloballyDisabledImageGeneration(t *testing.T) {
    svc := &OpenAIGatewayService{cfg: &config.Config{DisableImageGeneration: true}}
    frames := [][]byte{
        []byte(`{"type":"response.create","model":"gpt-5.5","tools":[{"type":"function","name":"shell"},{"type":"image_generation"}],"tool_choice":{"type":"image_generation"}}`),
        []byte(`{"type":"response.create","tools":[{"type":"function","name":"shell"},{"type":"namespace","name":"image_gen"}],"tool_choice":{"type":"namespace","name":"image_gen"}}`),
    }
    for _, frame := range frames {
        updated, changed, err := svc.normalizeGloballyDisabledImageGeneration(frame)
        require.NoError(t, err)
        require.True(t, changed)
        require.False(t, IsImageGenerationIntent(openAIResponsesEndpoint, "gpt-5.5", updated))
        require.True(t, gjson.GetBytes(updated, `tools.#(name=="shell")`).Exists())
        require.False(t, gjson.GetBytes(updated, "tool_choice").Exists())
    }
}
~~~

- [ ] **Step 2: Verify RED**

~~~bash
cd backend
go test ./internal/service -run '^TestOpenAIGatewayServiceNormalizeGloballyDisabledImageGeneration$' -count=1
~~~

Expected: compilation fails because the service method is undefined.

- [ ] **Step 3: Normalize each service ingress turn**

Add the method near the ingress code:

~~~go
func (s *OpenAIGatewayService) normalizeGloballyDisabledImageGeneration(payload []byte) ([]byte, bool, error) {
    if s == nil || s.cfg == nil || !s.cfg.DisableImageGeneration {
        return payload, false, nil
    }
    return StripOpenAIImageGenerationToolsFromRawPayload(payload)
}
~~~

Before codexBridgeEnabled in openai_ws_forwarder_ingress.go:

~~~go
globalImageDisable := s != nil && s.cfg != nil && s.cfg.DisableImageGeneration
if globalImageDisable {
    strippedPayload, changed, stripErr := s.normalizeGloballyDisabledImageGeneration(normalized)
    if stripErr != nil {
        return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", stripErr)
    }
    if changed {
        normalized = strippedPayload
        logOpenAIWSModeInfo("ingress_ws_image_tool_stripped_global account_id=%d", account.ID)
    }
}
~~~

Require !globalImageDisable in codexBridgeEnabled.

- [ ] **Step 4: Prevent HTTP forwarding from re-injecting**

In openai_gateway_forward.go calculate:

~~~go
globalImageDisable := s != nil && s.cfg != nil && s.cfg.DisableImageGeneration
~~~

Require !globalImageDisable in codexImageGenerationBridgeEnabled.

- [ ] **Step 5: Normalize the WebSocket first frame before its handler gate**

After firstMessage JSON validation, insert:

~~~go
normalizedFirstMessage, stripped, stripErr := h.normalizeGloballyDisabledImageGeneration(firstMessage)
if stripErr != nil {
    closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "invalid websocket request payload")
    return
}
if stripped {
    firstMessage = normalizedFirstMessage
    reqLog.Info("openai.websocket_image_generation_tools_stripped_global")
}
~~~

Derive model, moderation input, session data, and image intent from the normalized firstMessage.

- [ ] **Step 6: Verify WebSocket and bridge tests**

~~~bash
cd backend
go test ./internal/service -run '^(TestOpenAIGatewayServiceNormalizeGloballyDisabledImageGeneration|TestStripOpenAIImageGenerationToolsFromRawPayload|TestOpenAIWSForwarderIngress)' -count=1
go test ./internal/handler -run '^TestOpenAIResponsesWebSocket' -count=1
~~~

Expected: all selected tests pass and no global-disable payload contains an image tool upstream.

- [ ] **Step 7: Commit**

~~~bash
git add backend/internal/handler/openai_gateway_handler.go backend/internal/handler/openai_gateway_handler_test.go backend/internal/service/openai_ws_forwarder_ingress.go backend/internal/service/openai_gateway_forward.go backend/internal/service/openai_ws_forwarder_ingress_session_test.go
git commit -m "fix: enforce global image disable on websocket turns"
~~~

---

### Task 5: Fail Closed On Dedicated Generation Endpoints

**Files:**
- Modify: backend/internal/handler/openai_images.go
- Modify: backend/internal/handler/grok_media.go
- Test: backend/internal/handler/openai_images_controls_test.go
- Test: backend/internal/handler/grok_media_test.go

**Interfaces:**
- Consumes: imageGenerationGloballyDisabled() and service.GroupAllowsImageGeneration().
- Produces: imageGenerationAllowedForGroup(*service.Group) bool and global 403 enforcement independent of group permission.

- [ ] **Step 1: Write the failing shared policy test**

Add to image_generation_global_disable_test.go:

~~~go
func TestImageGenerationAllowedForGroup(t *testing.T) {
    allowedGroup := &service.Group{AllowImageGeneration: true}
    tests := []struct {
        name string
        disabled bool
        want bool
    }{
        {name: "existing group permission retained", disabled: false, want: true},
        {name: "global disable overrides allowed group", disabled: true, want: false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            h := &OpenAIGatewayHandler{cfg: &config.Config{DisableImageGeneration: tt.disabled}}
            require.Equal(t, tt.want, h.imageGenerationAllowedForGroup(allowedGroup))
        })
    }
}

func newGlobalDisableMediaContext(t *testing.T, path string, body string) (*httptest.ResponseRecorder, *gin.Context, *OpenAIGatewayHandler) {
    t.Helper()
    gin.SetMode(gin.TestMode)
    rec := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rec)
    c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
    c.Request.Header.Set("Content-Type", "application/json")
    groupID := int64(111)
    c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
        ID: 222,
        GroupID: &groupID,
        Group: &service.Group{ID: groupID, AllowImageGeneration: true},
        User: &service.User{ID: 333},
    })
    c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 333, Concurrency: 1})
    cfg := &config.Config{RunMode: config.RunModeSimple, DisableImageGeneration: true}
    h := &OpenAIGatewayHandler{
        gatewayService: &service.OpenAIGatewayService{},
        billingCacheService: service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil),
        apiKeyService: &service.APIKeyService{},
        concurrencyHelper: &ConcurrencyHelper{concurrencyService: &service.ConcurrencyService{}},
        cfg: cfg,
    }
    return rec, c, h
}

func TestOpenAIGatewayHandlerImagesGlobalDisableRejectsAllowedGroup(t *testing.T) {
    rec, c, h := newGlobalDisableMediaContext(t, "/v1/images/generations", `{"model":"gpt-image-2","prompt":"draw"}`)
    h.Images(c)
    require.Equal(t, http.StatusForbidden, rec.Code)
    require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
}

func TestGrokMediaGlobalDisableRejectsAllowedGroup(t *testing.T) {
    rec, c, h := newGlobalDisableMediaContext(t, "/v1/images/generations", `{"model":"grok-imagine","prompt":"draw"}`)
    h.GrokImages(c)
    require.Equal(t, http.StatusForbidden, rec.Code)
    require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
}
~~~

- [ ] **Step 2: Verify RED**

~~~bash
cd backend
go test ./internal/handler -run '^(TestImageGenerationAllowedForGroup|TestOpenAIGatewayHandlerImagesGlobalDisableRejectsAllowedGroup|TestGrokMediaGlobalDisableRejectsAllowedGroup)$' -count=1
~~~

Expected: compilation fails because imageGenerationAllowedForGroup is undefined; after adding only the helper, both direct handler tests still fail because the handlers have not called it.

- [ ] **Step 3: Add global checks**

Add the shared helper:

~~~go
func (h *OpenAIGatewayHandler) imageGenerationAllowedForGroup(group *service.Group) bool {
    return !h.imageGenerationGloballyDisabled() && service.GroupAllowsImageGeneration(group)
}
~~~

Use it in OpenAI Images:

~~~go
if !h.imageGenerationAllowedForGroup(apiKey.Group) {
    h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
    return
}
~~~

Use the exact same helper call inside Grok endpoint.IsGenerationRequest(). Do not block video-status polling.

- [ ] **Step 4: Verify endpoint regressions**

~~~bash
cd backend
go test ./internal/handler -run '^(TestImageGenerationAllowedForGroup|TestOpenAIGatewayHandlerImages|TestGrokMedia)' -count=1
~~~

Expected: global and group permission tests pass.

- [ ] **Step 5: Commit**

~~~bash
git add backend/internal/handler/openai_images.go backend/internal/handler/grok_media.go backend/internal/handler/openai_images_controls_test.go backend/internal/handler/grok_media_test.go
git commit -m "fix: block dedicated image endpoints globally"
~~~

---

### Task 6: Expose The Flag In Docker Deployment Files

**Files:**
- Modify: deploy/.env.example
- Modify: deploy/docker-compose.yml
- Modify: deploy/docker-compose.standalone.yml

**Interfaces:**
- Consumes: deployment environment DISABLE_IMAGE_GENERATION.
- Produces: explicit Compose propagation with false default.

- [ ] **Step 1: Add the environment example**

~~~dotenv
# Remove image tools from text-capable /responses requests and reject dedicated
# image generation endpoints. Defaults to false for backward compatibility.
DISABLE_IMAGE_GENERATION=false
~~~

- [ ] **Step 2: Add Compose propagation under each image-generation section**

~~~yaml
- DISABLE_IMAGE_GENERATION=${DISABLE_IMAGE_GENERATION:-false}
~~~

- [ ] **Step 3: Validate both Compose files**

~~~bash
cd deploy
POSTGRES_PASSWORD=test DISABLE_IMAGE_GENERATION=true docker compose config | rg 'DISABLE_IMAGE_GENERATION: "true"'
DATABASE_HOST=db DATABASE_PASSWORD=test REDIS_HOST=redis DISABLE_IMAGE_GENERATION=true docker compose -f docker-compose.standalone.yml config | rg 'DISABLE_IMAGE_GENERATION: "true"'
~~~

Expected: each command prints one entry.

- [ ] **Step 4: Commit**

~~~bash
git add deploy/.env.example deploy/docker-compose.yml deploy/docker-compose.standalone.yml
git commit -m "docs: expose global image disable in compose"
~~~

---

### Task 7: Full Verification And Immutable Image Build

**Files:**
- Verify all files changed in Tasks 1-6.
- No new source files.

**Interfaces:**
- Consumes: completed feature commits.
- Produces: tested source commit and sub2api-custom:0.1.151-disable-image-generation.1.

- [ ] **Step 1: Format and inspect**

~~~bash
gofmt -w backend/internal/config/config.go backend/internal/config/config_test.go backend/internal/service/openai_codex_transform.go backend/internal/service/openai_ws_forwarder_ingress_test.go backend/internal/service/openai_ws_forwarder_ingress.go backend/internal/service/openai_ws_forwarder_ingress_session_test.go backend/internal/service/openai_gateway_forward.go backend/internal/handler/image_generation_global_disable.go backend/internal/handler/image_generation_global_disable_test.go backend/internal/handler/openai_gateway_handler.go backend/internal/handler/openai_gateway_handler_test.go backend/internal/handler/image_concurrency_limiter_test.go backend/internal/handler/openai_images.go backend/internal/handler/openai_images_controls_test.go backend/internal/handler/grok_media.go backend/internal/handler/grok_media_test.go
git diff --check
git status --short
~~~

Expected: no formatting or whitespace errors and only intended files differ.

- [ ] **Step 2: Run focused packages**

~~~bash
cd backend
go test ./internal/config ./internal/service ./internal/handler -count=1
~~~

Expected: all selected packages pass.

- [ ] **Step 3: Run the complete backend suite**

~~~bash
cd backend
go test ./... -count=1
~~~

Expected: zero failing packages.

- [ ] **Step 4: Transfer committed source and build natively on production amd64**

~~~bash
BUILD_TAG=0.1.151-disable-image-generation.1
COMMIT=$(git rev-parse --short HEAD)
git archive --format=tar HEAD | ssh 101.43.35.235 "mkdir -p /opt/sub2api-builds/$BUILD_TAG && tar -xf - -C /opt/sub2api-builds/$BUILD_TAG"
ssh 101.43.35.235 "cd /opt/sub2api-builds/$BUILD_TAG && docker build --build-arg VERSION=$BUILD_TAG --build-arg COMMIT=$COMMIT -t sub2api-custom:$BUILD_TAG ."
ssh 101.43.35.235 "docker image inspect sub2api-custom:$BUILD_TAG --format 'id={{.Id}} created={{.Created}}'"
~~~

Expected: build succeeds and image inspection prints a non-empty ID.

---

### Task 8: Production Deployment, Verification, And Rollback

**Files:**
- Remote backup: /opt/sub2api/backups/docker-compose.override.yml.<timestamp>
- Remote deployment: /opt/sub2api/docker-compose.override.yml
- Local staging: ../production/docker-compose.override.yml

**Interfaces:**
- Consumes: sub2api-custom:0.1.151-disable-image-generation.1.
- Produces: healthy production service with the global switch enabled and an exact rollback path.

- [ ] **Step 1: Record rollback state**

~~~bash
ssh 101.43.35.235 'docker inspect sub2api --format "image={{.Config.Image}} image_id={{.Image}} started={{.State.StartedAt}}"; cd /opt/sub2api && docker compose config --images'
~~~

Expected: current upstream image and image ID are recorded before mutation.

- [ ] **Step 2: Download and preserve the current override**

~~~bash
mkdir -p ../production
scp 101.43.35.235:/opt/sub2api/docker-compose.override.yml ../production/docker-compose.override.yml
cp ../production/docker-compose.override.yml ../production/docker-compose.override.yml.before-image-disable
~~~

Use apply_patch on the local copy, preserving existing DGC network settings, to add under services.sub2api:

~~~yaml
image: sub2api-custom:0.1.151-disable-image-generation.1
environment:
  DISABLE_IMAGE_GENERATION: "true"
~~~

- [ ] **Step 3: Back up, upload, and validate**

~~~bash
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
ssh 101.43.35.235 "mkdir -p /opt/sub2api/backups && cp /opt/sub2api/docker-compose.override.yml /opt/sub2api/backups/docker-compose.override.yml.$TIMESTAMP"
scp ../production/docker-compose.override.yml 101.43.35.235:/opt/sub2api/docker-compose.override.yml
ssh 101.43.35.235 'cd /opt/sub2api && docker compose config | grep -E "sub2api-custom:0.1.151-disable-image-generation.1|DISABLE_IMAGE_GENERATION"'
~~~

Expected: custom image and true flag appear in rendered config.

- [ ] **Step 4: Recreate only Sub2API**

~~~bash
ssh 101.43.35.235 'cd /opt/sub2api && docker compose up -d --no-deps --force-recreate sub2api'
~~~

Poll docker inspect every two seconds for at most 60 seconds. Expected: running|healthy.

- [ ] **Step 5: Verify runtime state**

~~~bash
ssh 101.43.35.235 'docker inspect sub2api --format "image={{.Config.Image}} image_id={{.Image}} health={{.State.Health.Status}}"; docker inspect sub2api --format "{{range .Config.Env}}{{println .}}{{end}}" | grep "^DISABLE_IMAGE_GENERATION=true$"; docker logs --since 5m sub2api 2>&1 | grep -E "ERROR|FATAL|panic" || true'
curl -fsS -o /dev/null -w 'site_http=%{http_code}
' https://sub2api.weihub.cloud/
~~~

Expected: custom image, true flag, healthy container, no startup errors, site HTTP 200.

- [ ] **Step 6: Verify behavior end to end**

Send a fresh Codex gpt-5.5 text request through https://sub2api.weihub.cloud/responses and record its new request ID. Expected: log event openai.image_generation_tools_stripped_global, no image-permission 403, and a normal upstream response.

Send an authenticated /images/generations request. Expected: HTTP 403 with Image generation is not enabled for this group and no upstream account selection.

- [ ] **Step 7: Roll back on any failure**

~~~bash
ssh 101.43.35.235 "cp /opt/sub2api/backups/docker-compose.override.yml.$TIMESTAMP /opt/sub2api/docker-compose.override.yml && cd /opt/sub2api && docker compose up -d --no-deps --force-recreate sub2api"
~~~

Poll to running|healthy and verify the recorded upstream image ID is restored. Keep failed-deployment logs for diagnosis.
