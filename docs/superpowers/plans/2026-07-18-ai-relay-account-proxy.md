# AI Relay Account Proxy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make an account's configured primary proxy control AI Relay's platform-upstream connection without exposing proxy credentials or proxying the internal main-site-to-plugin hop.

**Architecture:** The main backend writes a reserved proxy ID header from the selected account and its shared HTTP upstream layer detects AI Relay URLs, removes the account proxy from the internal hop, and strips the header from all non-relay traffic. The plugin resolves the ID from the shared PostgreSQL `proxies` table and chooses a cached direct or proxy-aware HTTP client for every relay operation.

**Tech Stack:** Go, Gin, `net/http`, PostgreSQL/libpq, existing Sub2API HTTP upstream service, Go `httptest` and `sqlmock`-style repository tests.

---

### Task 1: Define the trusted main-site proxy context

**Files:**
- Create: `backend/internal/pluginrelay/proxy_context.go`
- Create: `backend/internal/pluginrelay/proxy_context_test.go`
- Modify: `backend/internal/service/account_header_override.go`
- Modify: `backend/internal/service/account_header_override_test.go`

- [ ] **Step 1: Write failing proxy-context tests**

Cover the reserved header contract, AI Relay URL detection, internal-hop proxy bypass, and non-relay header stripping:

```go
func TestPrepareUpstreamRequestBypassesProxyForAIRelay(t *testing.T) {
    req := httptest.NewRequest(http.MethodPost, "http://plugin-server:8091/plugins/ai-relay/agnes/1/v1/images/generations", nil)
    req.Header.Set(ProxyIDHeader, "42")
    require.Empty(t, PrepareUpstreamRequest(req, "http://proxy.example:8080"))
    require.Equal(t, "42", req.Header.Get(ProxyIDHeader))
}

func TestPrepareUpstreamRequestStripsReservedHeaderForOtherUpstreams(t *testing.T) {
    req := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/images/generations", nil)
    req.Header.Set(ProxyIDHeader, "42")
    require.Equal(t, "http://proxy.example:8080", PrepareUpstreamRequest(req, "http://proxy.example:8080"))
    require.Empty(t, req.Header.Get(ProxyIDHeader))
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `go test ./internal/pluginrelay ./internal/service -run 'TestPrepareUpstreamRequest|TestApplyHeaderOverrides'`

Expected: FAIL because `pluginrelay` and the reserved proxy context do not exist.

- [ ] **Step 3: Implement the proxy-context package**

Add a focused package with this public contract:

```go
const ProxyIDHeader = "X-Sub2api-Proxy-Id"

func SetProxyID(header http.Header, proxyID *int64)
func StripProxyID(header http.Header)
func IsAIRelayURL(target *url.URL) bool
func PrepareUpstreamRequest(req *http.Request, proxyURL string) string
```

`IsAIRelayURL` must normalize the path and accept `/plugins/ai-relay/` relay targets regardless of the internal host. `PrepareUpstreamRequest` must return an empty proxy URL only for AI Relay targets; for every other target it must remove the reserved header and return the original proxy URL.

- [ ] **Step 4: Attach the account proxy ID after header overrides**

Update `Account.ApplyHeaderOverrides` so the trusted proxy ID is always written or removed, even when custom header overrides are disabled:

```go
func (a *Account) ApplyHeaderOverrides(h http.Header) {
    if h == nil {
        return
    }
    pluginrelay.SetProxyID(h, a.ProxyID)
    // existing custom override behavior follows
}
```

Add assertions that an account proxy overwrites a spoofed value and that a nil proxy removes it.

- [ ] **Step 5: Run focused tests**

Run: `go test ./internal/pluginrelay ./internal/service -run 'TestPrepareUpstreamRequest|TestApplyHeaderOverrides'`

Expected: PASS.

- [ ] **Step 6: Commit the trusted context**

```bash
git add backend/internal/pluginrelay backend/internal/service/account_header_override.go backend/internal/service/account_header_override_test.go
git commit -m "feat(ai-relay): attach account proxy context"
```

### Task 2: Apply bypass and ingress sanitization centrally

**Files:**
- Modify: `backend/internal/repository/http_upstream.go`
- Modify: `backend/internal/repository/http_upstream_test.go`
- Modify: `backend/internal/server/routes/plugin_proxy.go`
- Modify: `backend/internal/server/routes/plugin_proxy_test.go`

- [ ] **Step 1: Write failing transport and reverse-proxy tests**

Add a repository test proving that an AI Relay request containing proxy ID `42` acquires a direct client even when `Do` receives an account proxy URL. Add a plugin reverse-proxy test that sends a spoofed `X-Sub2api-Proxy-Id: 999` and asserts that the plugin upstream receives no such header.

- [ ] **Step 2: Run tests and verify they fail**

Run: `go test ./internal/repository ./internal/server/routes -run 'AIRelay|ReservedProxyHeader'`

Expected: FAIL because the account proxy is still applied and the public reverse proxy still forwards the reserved header.

- [ ] **Step 3: Normalize relay requests before client acquisition**

At the beginning of `httpUpstreamService.Do`, before `acquireClientWithProfile`, add:

```go
proxyURL = pluginrelay.PrepareUpstreamRequest(req, proxyURL)
```

This single location must govern image generation, image editing, chat, models tests, and future OpenAI relay paths.

- [ ] **Step 4: Strip the header at public plugin ingress**

In the plugin reverse proxy director, remove the reserved header before forwarding browser-originated `/plugins/*` requests:

```go
req.Header.Del(pluginrelay.ProxyIDHeader)
```

Do not remove trusted principal headers generated later by the main backend authentication flow.

- [ ] **Step 5: Run focused tests**

Run: `go test ./internal/repository ./internal/server/routes -run 'AIRelay|ReservedProxyHeader'`

Expected: PASS.

- [ ] **Step 6: Commit central routing behavior**

```bash
git add backend/internal/repository/http_upstream.go backend/internal/repository/http_upstream_test.go backend/internal/server/routes/plugin_proxy.go backend/internal/server/routes/plugin_proxy_test.go
git commit -m "feat(ai-relay): bypass proxy on internal plugin hop"
```

### Task 3: Resolve main-site proxies inside the plugin service

**Files:**
- Create: `plugin-service/plugins/ai-relay/backend/proxy_repository.go`
- Create: `plugin-service/plugins/ai-relay/backend/proxy_repository_test.go`

- [ ] **Step 1: Write failing repository tests**

Test active proxy resolution, missing rows, disabled status, expired timestamps, deleted rows, and database-disabled behavior. The resolver contract is:

```go
type ProxyConfig struct {
    ID        int64
    Protocol  string
    Host      string
    Port      int
    Username  string
    Password  string
    UpdatedAt time.Time
}

type ProxyResolver interface {
    Resolve(context.Context, int64) (ProxyConfig, error)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `go test ./plugins/ai-relay/backend -run 'ProxyRepository|ProxyResolver'`

Expected: FAIL because the resolver types and repository do not exist.

- [ ] **Step 3: Implement SQL resolution**

Query only a live proxy:

```sql
SELECT id, protocol, host, port,
       COALESCE(username, ''), COALESCE(password, ''), updated_at
FROM proxies
WHERE id = $1
  AND deleted_at IS NULL
  AND status = 'active'
  AND (expires_at IS NULL OR expires_at > NOW())
```

Return typed sentinel errors for malformed IDs, unavailable proxy storage, and missing/inactive/expired proxies. Do not include credentials in error strings.

- [ ] **Step 4: Support database-disabled direct mode**

Provide a resolver constructed from `config.DatabaseConfig`. Database-disabled development mode must return an unavailable resolver without opening PostgreSQL; direct relay requests will not call it, while a later proxy-ID lookup will return the typed unavailable error.

- [ ] **Step 5: Run repository tests**

Run: `go test ./plugins/ai-relay/backend -run 'ProxyRepository|ProxyResolver'`

Expected: PASS.

- [ ] **Step 6: Commit proxy storage support**

```bash
git add plugin-service/plugins/ai-relay/backend/proxy_repository.go plugin-service/plugins/ai-relay/backend/proxy_repository_test.go
git commit -m "feat(ai-relay): resolve account proxies"
```

### Task 4: Build and cache proxy-aware plugin clients

**Files:**
- Create: `plugin-service/plugins/ai-relay/backend/proxy_client.go`
- Create: `plugin-service/plugins/ai-relay/backend/proxy_client_test.go`

- [ ] **Step 1: Write failing client-provider tests**

Cover direct requests, malformed header values, HTTP/HTTPS/SOCKS5/SOCKS5H proxy URLs, client reuse for an unchanged proxy, client replacement after `UpdatedAt` changes, and no direct fallback after resolver or proxy connection errors.

Use the contract:

```go
type RelayClientProvider interface {
    ClientFor(context.Context, string) (*http.Client, error)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `go test ./plugins/ai-relay/backend -run 'RelayClientProvider|ProxyClient'`

Expected: FAIL because the provider does not exist.

- [ ] **Step 3: Implement client selection and caching**

Parse the header with `strconv.ParseInt`, reject values below 1, resolve the proxy, create a credential-bearing `url.URL`, clone `http.DefaultTransport`, and set `Transport.Proxy = http.ProxyURL(proxyURL)`. Cache by proxy ID plus stable configuration identity using `UpdatedAt`; keep the direct client separate.

The cache must not use a credential-containing URL as a key or log proxy credentials.

- [ ] **Step 4: Run client-provider tests**

Run: `go test ./plugins/ai-relay/backend -run 'RelayClientProvider|ProxyClient'`

Expected: PASS.

- [ ] **Step 5: Commit client selection**

```bash
git add plugin-service/plugins/ai-relay/backend/proxy_client.go plugin-service/plugins/ai-relay/backend/proxy_client_test.go
git commit -m "feat(ai-relay): add proxy-aware upstream clients"
```

### Task 5: Use the selected client for every relay operation

**Files:**
- Modify: `plugin-service/plugins/ai-relay/backend/handler.go`
- Modify: `plugin-service/plugins/ai-relay/backend/handler_test.go`
- Modify: `plugin-service/plugins/ai-relay/plugin.go`

- [ ] **Step 1: Write failing handler tests**

Inject a recording `RelayClientProvider`, send requests with `X-Sub2api-Proxy-Id: 42`, and assert client selection for:

```text
POST /plugins/ai-relay/agnes/1
POST /plugins/ai-relay/agnes/1/v1/images/generations
POST /plugins/ai-relay/agnes/1/v1/images/edits
GET  /plugins/ai-relay/agnes/1/v1/models
POST /plugins/ai-relay/agnes/1/v1/chat/completions
```

Also assert malformed, missing, inactive, and expired proxy IDs return an OpenAI-compatible error without contacting Agnes.

- [ ] **Step 2: Run tests and verify they fail**

Run: `go test ./plugins/ai-relay/backend -run 'RelayUsesAccountProxy|RelayRejectsInvalidProxy'`

Expected: FAIL because handlers still use one static `http.Client`.

- [ ] **Step 3: Refactor handler client lookup**

Replace the static handler client dependency with `RelayClientProvider`. Resolve the client once per incoming request from `X-Sub2api-Proxy-Id`, then pass it through shared execution helpers. Adapters remain unchanged.

Map provider failures to OpenAI-compatible responses without proxy credentials:

```go
writeOpenAIError(w, http.StatusBadGateway, "upstream_error", safeProxyError(err))
```

Malformed header input remains `400 invalid_request_error`.

- [ ] **Step 4: Wire production provider**

In `plugin.go`, build the SQL resolver and cached provider, then inject it into `NewRelayHandler`. Tests may inject a static provider wrapping `httptest.Server.Client()`.

- [ ] **Step 5: Run all AI Relay tests**

Run: `go test ./plugins/ai-relay/...`

Expected: PASS.

- [ ] **Step 6: Commit handler integration**

```bash
git add plugin-service/plugins/ai-relay/backend/handler.go plugin-service/plugins/ai-relay/backend/handler_test.go plugin-service/plugins/ai-relay/plugin.go
git commit -m "feat(ai-relay): route upstream through account proxy"
```

### Task 6: Update operational documentation and verify end to end

**Files:**
- Modify: `plugin-service/README.md`
- Modify: `docs/superpowers/specs/2026-07-18-ai-relay-account-proxy-design.md` only if implementation reveals a necessary clarification

- [ ] **Step 1: Document deployment and behavior**

Document that the plugin service must share the main PostgreSQL configuration, `8091` remains internal, account proxy credentials never enter browser traffic, and configured proxy failures do not fall back to direct access.

- [ ] **Step 2: Run backend verification**

Run: `go test ./internal/pluginrelay ./internal/repository ./internal/server/routes ./internal/service`

Expected: PASS.

- [ ] **Step 3: Run plugin verification**

Run: `go test ./...` from `plugin-service`.

Expected: PASS.

- [ ] **Step 4: Run repository checks**

Run: `git diff --check`

Expected: no errors.

- [ ] **Step 5: Commit documentation or final integration fixes**

```bash
git add plugin-service/README.md docs/superpowers/specs/2026-07-18-ai-relay-account-proxy-design.md
git commit -m "docs(ai-relay): document account proxy routing"
```

- [ ] **Step 6: Restart and smoke-test the plugin service**

Restart `plugin-service`, then use an account with a reachable test proxy and verify `/v1/images/generations` succeeds. Disable that proxy and verify the same request returns an OpenAI-compatible upstream error instead of connecting directly.
