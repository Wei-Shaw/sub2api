package upstream

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry(NewClaudeAdapter(), NewOpenAIAdapter(), NewAntigravityAdapter())
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return r
}

func TestRegistryRejectsDuplicateAndNil(t *testing.T) {
	if _, err := NewRegistry(NewClaudeAdapter(), NewClaudeAdapter()); err == nil {
		t.Fatal("expected duplicate platform error")
	}
	if _, err := NewRegistry(nil); err == nil {
		t.Fatal("expected nil adapter error")
	}
	r := testRegistry(t)
	if got := len(r.Platforms()); got != 3 {
		t.Fatalf("platforms = %d, want 3", got)
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("unknown platform must not resolve")
	}
}

func TestClaudeAuthHeaders(t *testing.T) {
	a := NewClaudeAdapter()

	key, err := a.AuthHeaders(Credentials{Kind: AuthAPIKey, APIKey: "sk-ant-1"})
	if err != nil {
		t.Fatalf("api key: %v", err)
	}
	if key.Get("x-api-key") != "sk-ant-1" {
		t.Errorf("x-api-key = %q", key.Get("x-api-key"))
	}
	if key.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("anthropic-version = %q", key.Get("anthropic-version"))
	}
	if key.Get("authorization") != "" {
		t.Errorf("api-key account must not send authorization header, got %q", key.Get("authorization"))
	}
	if key.Get("anthropic-beta") != "" {
		t.Errorf("api-key account must not send oauth beta, got %q", key.Get("anthropic-beta"))
	}

	oauth, err := a.AuthHeaders(Credentials{Kind: AuthOAuth, AccessToken: "tok"})
	if err != nil {
		t.Fatalf("oauth: %v", err)
	}
	if oauth.Get("authorization") != "Bearer tok" {
		t.Errorf("authorization = %q", oauth.Get("authorization"))
	}
	if got := oauth.Get("anthropic-beta"); got != "oauth-2025-04-20" {
		t.Errorf("anthropic-beta = %q", got)
	}
	if oauth.Get("x-api-key") != "" {
		t.Errorf("oauth account must not send x-api-key")
	}

	// Account-level betas are merged, deduplicated, and order-stable.
	merged, _ := a.AuthHeaders(Credentials{
		Kind:        AuthOAuth,
		AccessToken: "tok",
		ExtraBeta:   "context-1m-2025-08-07, oauth-2025-04-20,  context-1m-2025-08-07 ",
	})
	if got, want := merged.Get("anthropic-beta"), "oauth-2025-04-20, context-1m-2025-08-07"; got != want {
		t.Errorf("merged beta = %q, want %q", got, want)
	}

	// Anthropic-compatible relays that only accept "Authorization: Bearer <key>"
	// for API keys (service/anthropic_apikey_auth.go).
	bearerKey, err := a.AuthHeaders(Credentials{
		Kind:         AuthAPIKey,
		APIKey:       "sk-ant-2",
		APIKeyScheme: APIKeySchemeAuthorizationBearer,
	})
	if err != nil {
		t.Fatalf("api key bearer scheme: %v", err)
	}
	if bearerKey.Get("authorization") != "Bearer sk-ant-2" {
		t.Errorf("bearer scheme authorization = %q", bearerKey.Get("authorization"))
	}
	if bearerKey.Get("x-api-key") != "" {
		t.Errorf("bearer scheme must not send x-api-key, got %q", bearerKey.Get("x-api-key"))
	}
	// The explicit scheme and the default one must stay mutually exclusive.
	if key.Get("authorization") != "" {
		t.Errorf("default scheme must not send authorization header")
	}

	if _, err := a.AuthHeaders(Credentials{Kind: AuthOAuth}); err == nil {
		t.Fatal("expected error for empty oauth token")
	}
	if _, err := a.AuthHeaders(Credentials{Kind: "bogus", APIKey: "x"}); err == nil {
		t.Fatal("expected error for unknown auth kind")
	}
}

func TestClaudeBaseURLResolvesEmptySentinel(t *testing.T) {
	a := NewClaudeAdapter()
	// service.Account.GetBaseURL() returns "" for non-API-key accounts; the adapter
	// must translate that sentinel into the platform default.
	if got := a.BaseURL(Credentials{Kind: AuthOAuth}); got != "https://api.anthropic.com" {
		t.Errorf("empty override = %q", got)
	}
	if got := a.BaseURL(Credentials{BaseURL: "https://relay.example.com/"}); got != "https://relay.example.com" {
		t.Errorf("trailing slash not trimmed: %q", got)
	}
}

func TestOpenAIAuthHeaders(t *testing.T) {
	a := NewOpenAIAdapter()

	plain, err := a.AuthHeaders(Credentials{Kind: AuthAPIKey, APIKey: "sk-1"})
	if err != nil {
		t.Fatalf("api key: %v", err)
	}
	if plain.Get("authorization") != "Bearer sk-1" {
		t.Errorf("authorization = %q", plain.Get("authorization"))
	}
	// A plain API call must not impersonate the Codex CLI.
	if plain.Get("version") != "" || plain.Get("originator") != "" {
		t.Errorf("non-codex call leaked fingerprint headers: version=%q originator=%q",
			plain.Get("version"), plain.Get("originator"))
	}

	codex, err := a.AuthHeaders(Credentials{
		Kind: AuthOAuth, AccessToken: "tok", AccountID: "acct-9", ImpersonateClient: "codex",
	})
	if err != nil {
		t.Fatalf("codex: %v", err)
	}
	if codex.Get("version") != "0.125.0" {
		t.Errorf("version = %q", codex.Get("version"))
	}
	if codex.Get("originator") != "codex_cli_rs" {
		t.Errorf("originator = %q", codex.Get("originator"))
	}
	if codex.Get("chatgpt-account-id") != "acct-9" {
		t.Errorf("chatgpt-account-id = %q", codex.Get("chatgpt-account-id"))
	}

	if _, err := a.AuthHeaders(Credentials{Kind: AuthOAuth}); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestAntigravityBaseURLFallbacks(t *testing.T) {
	a := NewAntigravityAdapter()

	fallbacks := a.BaseURLFallbacks(Credentials{Kind: AuthOAuth})
	want := []string{"https://daily-cloudcode-pa.sandbox.googleapis.com", "https://cloudcode-pa.googleapis.com"}
	if strings.Join(fallbacks, "|") != strings.Join(want, "|") {
		t.Fatalf("fallbacks = %v, want %v", fallbacks, want)
	}

	// A custom base_url on an API-key account gets the /antigravity suffix
	// (service/account.go:871-873).
	custom := a.BaseURLFallbacks(Credentials{Kind: AuthAPIKey, BaseURL: "https://relay.example.com/"})
	if len(custom) != 1 || custom[0] != "https://relay.example.com/antigravity" {
		t.Fatalf("custom = %v", custom)
	}

	// OAuth accounts keep the override verbatim.
	oauth := a.BaseURLFallbacks(Credentials{Kind: AuthOAuth, BaseURL: "https://relay.example.com"})
	if len(oauth) != 1 || oauth[0] != "https://relay.example.com" {
		t.Fatalf("oauth custom = %v", oauth)
	}
}

// testNow is the fixed clock every classification assertion uses. Classification
// takes now as an argument precisely so tests do not depend on wall clock.
var testNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func TestClassify(t *testing.T) {
	now := testNow
	h := func(kv ...string) http.Header {
		hd := make(http.Header, len(kv)/2)
		for i := 0; i < len(kv); i += 2 {
			hd.Set(kv[i], kv[i+1])
		}
		return hd
	}
	cases := []struct {
		name   string
		ad     Adapter
		status int
		header http.Header
		body   string
		want   FailureKind
	}{
		{"claude ok", NewClaudeAdapter(), 200, nil, `{}`, FailureNone},
		{"claude 401", NewClaudeAdapter(), 401, nil, `{}`, FailureAuth},
		{"claude 403", NewClaudeAdapter(), 403, nil, `{}`, FailureAuth},
		{"claude 400 credit", NewClaudeAdapter(), 400, nil, `{"type":"error","error":{"type":"invalid_request_error","message":"Your credit balance is too low"}}`, FailureAuth},
		{"claude 400 bad model", NewClaudeAdapter(), 400, nil, `{"error":{"type":"invalid_request_error"}}`, FailureClient},
		{"claude 529", NewClaudeAdapter(), 529, nil, `{}`, FailureOverload},
		{"claude overloaded body", NewClaudeAdapter(), 500, nil, `{"type":"overloaded_error"}`, FailureOverload},
		{"claude 429 retry-after", NewClaudeAdapter(), 429, h("retry-after", "30"), `{}`, FailureRateLimit},
		{"openai ok", NewOpenAIAdapter(), 200, nil, `{}`, FailureNone},
		{"openai 401", NewOpenAIAdapter(), 401, nil, `{}`, FailureAuth},
		{"openai insufficient_quota", NewOpenAIAdapter(), 429, nil, `{"error":{"code":"insufficient_quota"}}`, FailureAuth},
		{"openai real 429", NewOpenAIAdapter(), 429, nil, `{"error":{"code":"rate_limit_exceeded"}}`, FailureRateLimit},
		{"openai region", NewOpenAIAdapter(), 403, nil, `{"error":{"code":"unsupported_country"}}`, FailureAuth},
		{"openai 503", NewOpenAIAdapter(), 503, nil, `{}`, FailureOverload},
		{"antigravity 429 capacity", NewAntigravityAdapter(), 429, nil, `{"error":{"details":[{"reason":"MODEL_CAPACITY_EXHAUSTED"}]}}`, FailureOverload},
		{"antigravity 429 quota", NewAntigravityAdapter(), 429, nil, `{}`, FailureRateLimit},
		{"antigravity 403", NewAntigravityAdapter(), 403, nil, `{"error":{"status":"PERMISSION_DENIED"}}`, FailureAuth},
		{"antigravity 400", NewAntigravityAdapter(), 400, nil, `{}`, FailureClient},
		{"antigravity 418", NewAntigravityAdapter(), 418, nil, `{}`, FailureTransient},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.ad.Classify(testNow, tc.status, tc.header, []byte(tc.body))
			if got.Kind != tc.want {
				t.Fatalf("kind = %v, want %v (reason=%q)", got.Kind, tc.want, got.Reason)
			}
		})
	}

	// A 429 without hints must still produce a usable cooldown window.
	f := NewClaudeAdapter().Classify(testNow, 429, nil, nil)
	if !f.RetryAt.After(now) {
		t.Fatalf("429 must schedule a cooldown, RetryAt=%v", f.RetryAt)
	}
	if f.Schedulable(now) {
		t.Fatal("rate-limited account must not be schedulable immediately")
	}
	if !f.Schedulable(f.RetryAt.Add(time.Second)) {
		t.Fatal("account must become schedulable after RetryAt")
	}

	// Retry-After must be honoured, and an HTTP-date form must parse too.
	until := now.Add(45 * time.Second)
	if got := NewClaudeAdapter().Classify(testNow, 429, h("retry-after", "45"), nil).RetryAt; got.Before(now.Add(40 * time.Second)) {
		t.Fatalf("retry-after seconds ignored: %v", got)
	}
	dateHeader := h("retry-after", until.UTC().Format(http.TimeFormat))
	if got := NewClaudeAdapter().Classify(testNow, 429, dateHeader, nil).RetryAt; got.Before(now.Add(30 * time.Second)) {
		t.Fatalf("retry-after http-date ignored: %v", got)
	}
}

func TestStickyKeyIsStableAndPlatformScoped(t *testing.T) {
	a := NewOpenAIAdapter()
	b := NewClaudeAdapter()

	k1 := a.StickyKey(RequestMeta{SessionKey: "sess-1"})
	k2 := a.StickyKey(RequestMeta{SessionKey: "sess-1"})
	if k1 != k2 {
		t.Fatalf("sticky key not deterministic: %q vs %q", k1, k2)
	}
	if k1 == b.StickyKey(RequestMeta{SessionKey: "sess-1"}) {
		t.Fatal("sticky keys must be platform-scoped so a session cannot collide across platforms")
	}
	if a.StickyKey(RequestMeta{SessionKey: "sess-2"}) == k1 {
		t.Fatal("different sessions must map to different keys")
	}
	if got := a.StickyKey(RequestMeta{}); got != "" {
		t.Fatalf("empty session must disable affinity, got %q", got)
	}
}

func TestFailureKindString(t *testing.T) {
	for kind, want := range map[FailureKind]string{
		FailureNone: "none", FailureAuth: "auth", FailureRateLimit: "rate_limit",
		FailureOverload: "overload", FailureTransient: "transient", FailureClient: "client",
	} {
		if got := kind.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", kind, got, want)
		}
	}
}
