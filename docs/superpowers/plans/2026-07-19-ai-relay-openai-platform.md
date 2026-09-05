# AI Relay OpenAI Platform Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a generic OpenAI transparent platform with `/v1/*` forwarding and route-level path mappings while preserving Agnes behavior.

**Architecture:** Extend platform descriptors with an explicit protocol/capability marker. Register an OpenAI transparent adapter beside Agnes. Route explicit existing endpoints through current handlers, and add a wildcard endpoint handler that only accepts transparent platforms and forwards the original HTTP exchange after resolving the mapped upstream path.

**Tech Stack:** Go `net/http`, existing AI Relay adapter/repository packages, Vue 3 + TypeScript, Vitest, Go test.

---

### Task 1: Add the OpenAI platform descriptor

**Files:**
- Modify: `plugin-service/plugins/ai-relay/backend/adapter.go`
- Create: `plugin-service/plugins/ai-relay/backend/openai.go`
- Modify: `plugin-service/plugins/ai-relay/backend/agnes_test.go`
- Create: `plugin-service/plugins/ai-relay/backend/openai_test.go`

- [ ] **Step 1: Write failing registry and descriptor tests**

Add assertions that `NewDefaultAdapterRegistry().Platforms()` contains `{Key: "openai", DisplayName: "OpenAI", Protocol: "transparent"}` and that the OpenAI adapter reports transparent protocol capability.

- [ ] **Step 2: Run the focused tests and verify they fail**

Run: `go test ./plugin-service/plugins/ai-relay/backend -run 'Test(NewDefaultAdapterRegistry|OpenAI)' -count=1`
Expected: FAIL because `PlatformDescriptor` has no protocol field and no OpenAI adapter is registered.

- [ ] **Step 3: Implement the minimum adapter and descriptor changes**

Add a `Protocol` field to `PlatformDescriptor`, implement `OpenAIAdapter` with `Platform() == "openai"`, `Descriptor().Protocol == "transparent"`, and register it in `NewDefaultAdapterRegistry`. Keep image conversion methods unavailable through an explicit unsupported error so the generic platform cannot accidentally use Agnes payload conversion.

- [ ] **Step 4: Run focused tests and verify they pass**

Run: `go test ./plugin-service/plugins/ai-relay/backend -run 'Test(NewDefaultAdapterRegistry|OpenAI)' -count=1`
Expected: PASS.

### Task 2: Add wildcard transparent proxying

**Files:**
- Modify: `plugin-service/plugins/ai-relay/backend/handler.go`
- Modify: `plugin-service/plugins/ai-relay/backend/handler_test.go`

- [ ] **Step 1: Write failing wildcard forwarding tests**

Register the routes, create an OpenAI route backed by an `httptest.Server`, and request `POST /plugins/ai-relay/openai/demo/v1/embeddings?x=1`. Assert the upstream receives `/v4/embeddings` when mapping `v1/embeddings -> v4/embeddings`, the original JSON body and bearer token are present, and upstream status/body are returned unchanged. Add a second assertion for an unmapped `/v1/audio/transcriptions` path being appended unchanged.

- [ ] **Step 2: Run the focused handler tests and verify they fail**

Run: `go test ./plugin-service/plugins/ai-relay/backend -run TestOpenAIWildcard -count=1`
Expected: FAIL because the wildcard route and transparent handler do not exist.

- [ ] **Step 3: Implement transparent proxy handling**

Register `/{path...}` under the plugin route prefix after the explicit patterns. Add `ProxyOpenAIPath` to derive the endpoint from `r.PathValue("path")`, load the route, require the OpenAI transparent protocol, resolve with `ResolveRouteEndpointURL`, preserve query parameters and request/response headers, and stream the response. Return the existing OpenAI-formatted errors for auth, lookup, unsupported platform, URL resolution, and upstream failures.

- [ ] **Step 4: Run focused and regression handler tests**

Run: `go test ./plugin-service/plugins/ai-relay/backend -run 'TestOpenAIWildcard|TestPathMappingsApplyToExistingRelayEndpoints|Test(Generate|Edit)' -count=1`
Expected: PASS, including unchanged Agnes image conversion tests.

### Task 3: Expose OpenAI in the plugin UI

**Files:**
- Modify: `plugin-service/plugins/ai-relay/frontend/src/App.vue`
- Modify: `plugin-service/plugins/ai-relay/frontend/src/App.spec.ts`

- [ ] **Step 1: Write the failing UI test**

Return both `agnes` and `openai` from the mocked platform API, mount the route form, and assert the platform selector renders an `OpenAI` option and submits `platform: "openai"` with mappings unchanged.

- [ ] **Step 2: Run the focused UI test and verify it fails**

Run: `npm test -- --run src/App.spec.ts`
Expected: FAIL because the current platform fixture and selector do not expose OpenAI.

- [ ] **Step 3: Implement the minimal UI update**

Use the existing API-driven platform list without hard-coded Agnes-only assumptions; ensure the OpenAI option is selectable and the route form continues submitting the selected platform and path mappings verbatim.

- [ ] **Step 4: Run frontend tests, typecheck, and build**

Run: `npm test -- --run`; `npm run typecheck`; `npm run build`
Expected: all tests pass, typecheck succeeds, and the production build completes.

### Task 4: Full verification and commit

**Files:**
- No additional production files.

- [ ] **Step 1: Run the complete backend suite**

Run: `go test ./plugin-service/plugins/ai-relay/backend -count=1`
Expected: PASS.

- [ ] **Step 2: Run the complete plugin-service suite**

Run: `go test ./plugin-service/... -count=1`
Expected: PASS.

- [ ] **Step 3: Inspect the diff for main-site changes**

Run: `git diff --check HEAD~1`; `git status --short`
Expected: no whitespace errors and only plugin-service plus documentation changes.

- [ ] **Step 4: Commit the implementation**

Run: `git add plugin-service/plugins/ai-relay && git commit -m "feat(ai-relay): add transparent openai platform"`
Expected: one implementation commit is created.
