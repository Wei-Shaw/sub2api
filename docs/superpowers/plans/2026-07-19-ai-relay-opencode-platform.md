# AI Relay OpenCode Platform Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add OpenCode as an AI Relay platform with automatic `/v1` URL handling and best-effort conversion of OpenCode-native chat requests to OpenAI Chat Completions.

**Architecture:** Extend platform descriptors with an optional default Base URL and add a transparent-adapter hook for effective Base URL normalization and request-body preparation. OpenCode uses that hook; OpenAI remains a no-op transparent adapter and Agnes remains unchanged. The existing proxy handler continues copying upstream responses unchanged.

**Tech Stack:** Go `net/http` and `encoding/json`, Vue 3 + TypeScript, Vitest, Go test.

---

### Task 1: Register OpenCode and implement conversion helpers

**Files:**
- Modify: `plugin-service/plugins/ai-relay/backend/adapter.go`
- Modify: `plugin-service/plugins/ai-relay/backend/openai.go`
- Create: `plugin-service/plugins/ai-relay/backend/opencode.go`
- Modify: `plugin-service/plugins/ai-relay/backend/agnes.go`
- Modify: `plugin-service/plugins/ai-relay/backend/agnes_test.go`
- Create: `plugin-service/plugins/ai-relay/backend/opencode_test.go`

- [ ] **Step 1: Write failing registry, metadata, URL, and conversion tests**

Assert the registry lists `{Key: "opencode", DisplayName: "OpenCode", Protocol: "opencode", DefaultBaseURL: "https://opencode.ai/zen"}`. Add tests for Base URL normalization (`https://opencode.ai/zen` -> `.../zen/v1`, existing `/v1` unchanged), conversion of an object model plus system/parts/stream fields, message fallback, standard OpenAI pass-through, and invalid JSON pass-through.

- [ ] **Step 2: Run focused tests and verify failure**

Run: `go test ./plugins/ai-relay/backend -run 'Test(OpenCode|DefaultAdapterRegistry)' -count=1`
Expected: FAIL because OpenCode metadata, adapter, and conversion helpers do not exist.

- [ ] **Step 3: Implement minimal adapter and conversion**

Add `DefaultBaseURL` to `PlatformDescriptor`; register OpenCode after Agnes/OpenAI. Define `OpenCodeAdapter` with transparent protocol hooks, automatic `/v1` normalization, and JSON conversion limited to `chat/completions`. Preserve only the documented OpenCode fields and return the original bytes for invalid/unconvertible input. Update Agnes descriptor metadata without changing its behavior.

- [ ] **Step 4: Run focused tests and verify green**

Run: `go test ./plugins/ai-relay/backend -run 'Test(OpenCode|DefaultAdapterRegistry)' -count=1`
Expected: PASS.

### Task 2: Integrate OpenCode hooks into transparent proxying

**Files:**
- Modify: `plugin-service/plugins/ai-relay/backend/handler.go`
- Modify: `plugin-service/plugins/ai-relay/backend/handler_test.go`
- Modify: `plugin-service/plugins/ai-relay/backend/config_test.go`

- [ ] **Step 1: Write failing handler tests**

Add an OpenCode route backed by `httptest.Server`, request `POST /plugins/ai-relay/opencode/demo/v1/chat/completions`, and assert the upstream receives `/zen/v1/chat/completions`, converted OpenCode JSON, the original query string, and Bearer header. Add a test that an explicit Base URL ending in `/v1` is not duplicated.

- [ ] **Step 2: Run focused handler tests and verify failure**

Run: `go test ./plugins/ai-relay/backend -run TestOpenCodeTransparentProxy -count=1`
Expected: FAIL because OpenCode routes and request hooks are not registered.

- [ ] **Step 3: Implement handler integration**

Expose `DefaultBaseURL` to route creation responses, add an OpenCode prefix route, and introduce a transparent adapter hook that normalizes a copied route config and replaces only the outbound request body before forwarding. Preserve headers, query strings, status codes, response headers, and streaming response bodies. Do not apply conversion to `models`, `responses`, images, or non-OpenCode routes.

- [ ] **Step 4: Run backend regression tests**

Run: `go test ./plugins/ai-relay/backend -count=1`
Expected: PASS, including Agnes and OpenAI tests.

### Task 3: Add OpenCode default URL behavior in the plugin UI

**Files:**
- Modify: `plugin-service/plugins/ai-relay/frontend/src/types.ts`
- Modify: `plugin-service/plugins/ai-relay/frontend/src/App.vue`
- Modify: `plugin-service/plugins/ai-relay/frontend/src/App.spec.ts`

- [ ] **Step 1: Write failing UI tests**

Return OpenCode with `default_base_url` from the mocked platform API. Assert the platform option is rendered, selecting it on the create form changes Base URL to `https://opencode.ai/zen`, and editing an existing route does not overwrite its Base URL.

- [ ] **Step 2: Run focused UI tests and verify failure**

Run: `npm test -- --run src/App.spec.ts`
Expected: FAIL because platform descriptors have no default URL and the form uses the Agnes URL unconditionally.

- [ ] **Step 3: Implement dynamic default URL selection**

Add optional `default_base_url` to `Platform`, derive the initial Base URL from the selected platform, and update it when changing platform only for new routes. Keep Platform disabled while editing and preserve custom Base URLs.

- [ ] **Step 4: Run frontend verification**

Run: `npm test -- --run`; `npm run typecheck`; `npm run build`
Expected: 24+ tests pass, typecheck succeeds, and generated plugin assets build.

### Task 4: Full verification and commit

**Files:**
- No additional files beyond Tasks 1-3.

- [ ] **Step 1: Run full plugin-service tests**

Run: `go test ./... -count=1`
Expected: PASS.

- [ ] **Step 2: Check the diff and commit**

Run: `git diff --check`; `git status --short`; `git add plugin-service/plugins/ai-relay`; `git commit -m "feat(ai-relay): add opencode relay platform"`
Expected: only standalone AI Relay plugin files are committed; main-site files remain unchanged.

- [ ] **Step 3: Restart the plugin service**

Restart only the process bound to port `8091`, then verify `GET http://127.0.0.1:8091/healthz` returns `{"status":"ok"}`.
