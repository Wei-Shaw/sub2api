# AI Relay Path Mappings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add route-level relative path mappings and transparent `/v1/responses` and `/v1/responses/compact` forwarding to the AI Relay plugin while preserving existing behavior for unmapped requests.

**Architecture:** Store normalized mappings on `RouteConfig`, resolve every adapter endpoint through one URL builder, and use one transparent proxy handler for models, chat, and Responses endpoints. Persist mappings as PostgreSQL `JSONB`; expose them through the existing route API; edit them as source-target rows in the existing Vue administration dialog.

**Tech Stack:** Go 1.24, `net/http`, PostgreSQL/`lib/pq`, `go-sqlmock`, Vue 3, TypeScript, Vitest.

---

## File Structure

- Modify `plugin-service/plugins/ai-relay/backend/config.go`: add mapping data to `RouteConfig`, normalize and defensively copy it.
- Create `plugin-service/plugins/ai-relay/backend/path_mapping.go`: own canonical path normalization and mapped/unmapped upstream URL resolution.
- Modify `plugin-service/plugins/ai-relay/backend/config_test.go`: cover configuration validation, copying, schema, and SQL persistence.
- Modify `plugin-service/plugins/ai-relay/backend/config_repository.go`: add the `JSONB` column and encode/decode mappings.
- Modify `plugin-service/plugins/ai-relay/backend/agnes.go`: return canonical endpoint paths through the existing adapter contract.
- Modify `plugin-service/plugins/ai-relay/backend/handler.go`: register Responses routes and centralize transparent proxying, headers, queries, and streaming.
- Modify `plugin-service/plugins/ai-relay/backend/handler_test.go`: cover endpoint URL resolution and transparent forwarding.
- Modify `frontend/src/views/admin/AIRelayView.vue`: add mapping rows to route create/edit payloads.
- Modify `frontend/src/views/admin/__tests__/AIRelayView.spec.ts`: cover the mapping editor contract.

### Task 1: Normalize Path Mapping Configuration

**Files:**
- Create: `plugin-service/plugins/ai-relay/backend/path_mapping.go`
- Modify: `plugin-service/plugins/ai-relay/backend/config.go`
- Test: `plugin-service/plugins/ai-relay/backend/config_test.go`

- [ ] **Step 1: Write failing normalization and URL-resolution tests**

Add table tests that assert:

```go
func TestNormalizeRouteConfigNormalizesPathMappings(t *testing.T) {
	config, err := NormalizeRouteConfig(RouteConfig{
		Platform: "agnes",
		Slug: "zhipu",
		BaseURL: "https://open.bigmodel.cn/v1",
		PathMappings: map[string]string{
			" /v1/responses/compact/ ": " /api/paas/v4/chat/completions/ ",
		},
	})
	if err != nil { t.Fatal(err) }
	if got := config.PathMappings["responses/compact"]; got != "api/paas/v4/chat/completions" {
		t.Fatalf("mapping = %q", got)
	}
}

func TestResolveRouteEndpointURL(t *testing.T) {
	config := RouteConfig{
		BaseURL: "https://open.bigmodel.cn/v1",
		PathMappings: map[string]string{"responses/compact": "api/paas/v4/chat/completions"},
	}
	mapped, _ := ResolveRouteEndpointURL(config, "responses/compact")
	unmapped, _ := ResolveRouteEndpointURL(config, "models")
	if mapped != "https://open.bigmodel.cn/api/paas/v4/chat/completions" { t.Fatal(mapped) }
	if unmapped != "https://open.bigmodel.cn/v1/models" { t.Fatal(unmapped) }
}
```

Add rejection cases for `https://evil.example/path`, `//evil.example/path`, `path?x=1`, `path#fragment`, empty targets, and duplicate-equivalent sources (`responses` plus `v1/responses`). Add a memory repository test that mutating a returned mapping does not mutate stored state.

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./plugins/ai-relay/backend -run 'TestNormalizeRouteConfig.*PathMappings|TestResolveRouteEndpointURL|TestMemoryRouteRepository.*Mapping' -count=1`

Expected: compilation fails because `PathMappings` and `ResolveRouteEndpointURL` do not exist.

- [ ] **Step 3: Implement normalized configuration and URL resolution**

Add to `RouteConfig`:

```go
PathMappings map[string]string `json:"path_mappings"`
```

Implement focused helpers in `path_mapping.go`:

```go
func canonicalRelayPath(value string) string {
	path := strings.Trim(strings.TrimSpace(value), "/")
	if strings.HasPrefix(path, "v1/") { path = strings.TrimPrefix(path, "v1/") }
	return path
}

func normalizePathMappings(input map[string]string) (map[string]string, error)
func ResolveRouteEndpointURL(config RouteConfig, endpoint string) (string, error)
```

`ResolveRouteEndpointURL` parses `BaseURL`; on a match it sets `URL.Path` to `"/" + target`, clears raw path/query/fragment, and on no match joins the canonical endpoint onto the complete base path. Reject mapping targets whose parsed form is absolute, has a host, query, fragment, or empty path. Update `copyRouteConfig` to clone the mapping map.

- [ ] **Step 4: Run focused tests and all backend package tests**

Run: `go test ./plugins/ai-relay/backend -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the configuration unit**

```bash
git add plugin-service/plugins/ai-relay/backend/config.go plugin-service/plugins/ai-relay/backend/path_mapping.go plugin-service/plugins/ai-relay/backend/config_test.go
git commit -m "feat(ai-relay): add route path mapping resolver"
```

### Task 2: Persist Path Mappings In PostgreSQL

**Files:**
- Modify: `plugin-service/plugins/ai-relay/backend/config_repository.go`
- Test: `plugin-service/plugins/ai-relay/backend/config_test.go`

- [ ] **Step 1: Extend SQL mock tests first**

Require schema initialization to execute:

```sql
ALTER TABLE plugin_ai_relay_routes
ADD COLUMN IF NOT EXISTS path_mappings JSONB NOT NULL DEFAULT '{}'::jsonb
```

Update the upsert expectation to receive serialized normalized mappings and return a `path_mappings` value. Add a scan test using:

```go
sqlmock.NewRows([]string{"platform", "slug", "name", "base_url", "path_mappings"}).
	AddRow("agnes", "zhipu", "Zhipu", "https://open.bigmodel.cn/v1", []byte(`{"responses/compact":"api/paas/v4/chat/completions"}`))
```

- [ ] **Step 2: Run the SQL tests and verify failure**

Run: `go test ./plugins/ai-relay/backend -run 'TestSQLRouteRepository|TestEnsureRouteSchema' -count=1`

Expected: FAIL because schema, SQL arguments, and scan columns do not include `path_mappings`.

- [ ] **Step 3: Implement JSONB persistence**

Add the column migration after table creation. Change `routeSelectSQL` to select `path_mappings`. Marshal the normalized map before upsert, pass it as `$5::jsonb`, return the column, and unmarshal it in `scanRouteConfig`. Treat SQL `NULL` as `{}` for compatibility, then return a defensive normalized map.

- [ ] **Step 4: Run repository and package tests**

Run: `go test ./plugins/ai-relay/backend -count=1`

Expected: PASS.

- [ ] **Step 5: Commit persistence**

```bash
git add plugin-service/plugins/ai-relay/backend/config_repository.go plugin-service/plugins/ai-relay/backend/config_test.go
git commit -m "feat(ai-relay): persist route path mappings"
```

### Task 3: Route All Upstream Endpoints Through The Resolver

**Files:**
- Modify: `plugin-service/plugins/ai-relay/backend/adapter.go`
- Modify: `plugin-service/plugins/ai-relay/backend/agnes.go`
- Modify: `plugin-service/plugins/ai-relay/backend/handler.go`
- Test: `plugin-service/plugins/ai-relay/backend/agnes_test.go`
- Test: `plugin-service/plugins/ai-relay/backend/handler_test.go`

- [ ] **Step 1: Write failing endpoint and transparent proxy tests**

Add mux-level tests for both new routes and table-driven mapped/unmapped cases:

```go
tests := []struct{ inbound, upstream string }{
	{"/plugins/ai-relay/agnes/zhipu/v1/responses", "/v1/responses"},
	{"/plugins/ai-relay/agnes/zhipu/v1/responses/compact", "/api/paas/v4/chat/completions"},
}
```

For the compact mapped request assert that the upstream receives the original query, authorization, content type, accept header, custom end-to-end header, and exact body. Return status `202`, a custom response header, and a multi-chunk body; assert the client receives them unchanged. Assert `X-Sub2api-Proxy-Id` selects the configured client but is not leaked upstream. Add mapping cases for `models`, `chat/completions`, and image generation to prove all existing endpoints use the resolver.

- [ ] **Step 2: Run focused handler tests and verify failure**

Run: `go test ./plugins/ai-relay/backend -run 'Test.*PathMapping|Test.*Responses' -count=1`

Expected: FAIL because Responses routes are not registered and adapter URLs bypass the resolver.

- [ ] **Step 3: Implement shared endpoint resolution and transparent forwarding**

Change the adapter endpoint contract to expose canonical relative endpoints rather than prejoined URLs:

```go
Endpoint(RouteConfig) string                // returns "images/generations"
ModelsEndpoint(RouteConfig) string          // returns "models"
ChatCompletionsEndpoint(RouteConfig) string // returns "chat/completions"
```

Resolve these immediately before creating upstream requests. Register:

```go
mux.HandleFunc("POST /plugins/ai-relay/{platform}/{slug}/v1/responses", handler.ProxyResponses)
mux.HandleFunc("POST /plugins/ai-relay/{platform}/{slug}/v1/responses/compact", handler.ProxyResponsesCompact)
```

Replace `proxyOpenAIEndpoint` with a transparent helper that accepts a canonical endpoint. Copy all request and response headers except hop-by-hop headers, exclude the internal proxy-selection header, clone incoming query values onto the resolved upstream URL, preserve status, and stream with `io.Copy`. Keep authentication, route lookup, adapter validation, and client selection common to all transparent endpoints.

- [ ] **Step 4: Run backend tests**

Run: `go test ./plugins/ai-relay/backend -count=1`

Expected: PASS.

Run: `go test ./... -count=1` from `plugin-service`.

Expected: PASS.

- [ ] **Step 5: Commit proxy support**

```bash
git add plugin-service/plugins/ai-relay/backend/adapter.go plugin-service/plugins/ai-relay/backend/agnes.go plugin-service/plugins/ai-relay/backend/agnes_test.go plugin-service/plugins/ai-relay/backend/handler.go plugin-service/plugins/ai-relay/backend/handler_test.go
git commit -m "feat(ai-relay): proxy mapped responses paths"
```

### Task 4: Add The Path Mapping Editor

**Files:**
- Modify: `frontend/src/views/admin/AIRelayView.vue`
- Modify: `frontend/src/views/admin/__tests__/AIRelayView.spec.ts`

- [ ] **Step 1: Replace the source-inspection test with mounted behavior coverage**

Mock `apiClient` and mount `AIRelayView`. Cover:

- opening create adds and removes mapping rows;
- editing converts `path_mappings` into rows;
- saving normalizes `/v1/responses/compact/` to `responses/compact` and `/api/paas/v4/chat/completions/` to `api/paas/v4/chat/completions`;
- blank row pairs are omitted;
- an empty editor sends `path_mappings: {}`.

Use stable selectors:

```html
data-testid="path-mapping-add"
data-testid="path-mapping-source"
data-testid="path-mapping-target"
data-testid="path-mapping-remove"
```

- [ ] **Step 2: Run the frontend test and verify failure**

Run: `npm run test:run -- src/views/admin/__tests__/AIRelayView.spec.ts`

Expected: FAIL because the editor and `path_mappings` payload do not exist.

- [ ] **Step 3: Implement the mapping rows**

Extend the route type and form state:

```ts
type MappingRow = { source: string; target: string }
interface RelayRoute { /* existing fields */ path_mappings: Record<string, string> }
const pathMappingRows = ref<MappingRow[]>([])
```

Add `mappingRowsFromRecord`, `canonicalPath`, and `mappingRecordFromRows` helpers based on the reference project. Reset rows on create, populate them on edit, and submit the generated record. Add a full-width unframed section under Base URL with two inputs per row, an arrow separator, a trash icon button, and an “Add mapping” command. Ensure the dialog remains usable on mobile by stacking the two inputs below the small breakpoint.

- [ ] **Step 4: Run frontend tests, typecheck, and build**

Run: `npm run test:run -- src/views/admin/__tests__/AIRelayView.spec.ts`

Expected: PASS.

Run: `npm run typecheck`

Expected: PASS.

Run: `npm run build`

Expected: PASS.

- [ ] **Step 5: Commit the UI**

```bash
git add frontend/src/views/admin/AIRelayView.vue frontend/src/views/admin/__tests__/AIRelayView.spec.ts
git commit -m "feat(ai-relay): add path mapping editor"
```

### Task 5: End-To-End Verification

**Files:**
- Verify only; modify prior files only if a regression is found.

- [ ] **Step 1: Format and lint changed code**

Run: `gofmt -w plugins/ai-relay/backend/config.go plugins/ai-relay/backend/path_mapping.go plugins/ai-relay/backend/config_repository.go plugins/ai-relay/backend/adapter.go plugins/ai-relay/backend/agnes.go plugins/ai-relay/backend/handler.go plugins/ai-relay/backend/config_test.go plugins/ai-relay/backend/agnes_test.go plugins/ai-relay/backend/handler_test.go` from `plugin-service`.

Run: `npm run lint:check -- src/views/admin/AIRelayView.vue src/views/admin/__tests__/AIRelayView.spec.ts` from `frontend`.

Expected: no errors.

- [ ] **Step 2: Run full relevant test suites**

Run: `go test ./... -count=1` from `plugin-service`.

Run: `npm run test:run -- src/views/admin/__tests__/AIRelayView.spec.ts` from `frontend`.

Run: `npm run typecheck` from `frontend`.

Run: `npm run build` from `frontend`.

Expected: all commands pass.

- [ ] **Step 3: Inspect the final diff**

Run: `git diff --check` and `git status --short`.

Expected: no whitespace errors; only intended implementation and test files are changed.

- [ ] **Step 4: Commit any verification fixes**

If verification required edits:

```bash
git add plugin-service/plugins/ai-relay/backend frontend/src/views/admin/AIRelayView.vue frontend/src/views/admin/__tests__/AIRelayView.spec.ts
git commit -m "fix(ai-relay): finalize path mapping support"
```
