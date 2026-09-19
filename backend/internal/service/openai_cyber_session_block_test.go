package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCyberBlockTestCtx(headers map[string]string, body string) (*gin.Context, []byte) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/openai/v1/responses", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Request = req
	return c, []byte(body)
}

// --- fakes ---

type fakeCyberBlockStore struct {
	blocked    map[string]bool
	scopes     map[string]bool
	findCalls  int
	findErr    error
	scopeCalls int
}

var _ CyberSessionBlockStore = (*fakeCyberBlockStore)(nil)

func (f *fakeCyberBlockStore) SetCyberSessionBlocked(_ context.Context, scopeKey string, keys []string, _ time.Duration) error {
	if f.blocked == nil {
		f.blocked = map[string]bool{}
	}
	for _, key := range keys {
		f.blocked[key] = true
	}
	if scopeKey != "" {
		if f.scopes == nil {
			f.scopes = map[string]bool{}
		}
		f.scopes[scopeKey] = true
	}
	return nil
}

func (f *fakeCyberBlockStore) IsCyberSessionScopeActive(_ context.Context, scopeKey string) (bool, error) {
	f.scopeCalls++
	return f.scopes[scopeKey], nil
}

func (f *fakeCyberBlockStore) FindCyberSessionBlocked(_ context.Context, keys []string) (string, error) {
	f.findCalls++
	if f.findErr != nil {
		return "", f.findErr
	}
	for _, key := range keys {
		if f.blocked[key] {
			return key, nil
		}
	}
	return "", nil
}

// fakeSettingRepo is a minimal SettingRepository stub for unit tests.
// Only GetValue is exercised by GetCyberSessionBlockRuntime; all other methods
// panic so accidental calls are caught immediately.
type fakeSettingRepo struct {
	vals map[string]string
}

func (r *fakeSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	v, ok := r.vals[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return v, nil
}
func (r *fakeSettingRepo) Get(_ context.Context, _ string) (*Setting, error) {
	panic("fakeSettingRepo.Get not implemented")
}
func (r *fakeSettingRepo) Set(_ context.Context, _, _ string) error {
	panic("fakeSettingRepo.Set not implemented")
}
func (r *fakeSettingRepo) GetMultiple(_ context.Context, _ []string) (map[string]string, error) {
	panic("fakeSettingRepo.GetMultiple not implemented")
}
func (r *fakeSettingRepo) SetMultiple(_ context.Context, _ map[string]string) error {
	panic("fakeSettingRepo.SetMultiple not implemented")
}
func (r *fakeSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	panic("fakeSettingRepo.GetAll not implemented")
}
func (r *fakeSettingRepo) Delete(_ context.Context, _ string) error {
	panic("fakeSettingRepo.Delete not implemented")
}

var _ SettingRepository = (*fakeSettingRepo)(nil)

// comboCacheAndStore implements both GatewayCache (no-op stubs) and
// CyberSessionBlockStore (delegates to fakeCyberBlockStore) so it can be
// injected as s.cache and successfully type-asserted to CyberSessionBlockStore.
type comboCacheAndStore struct {
	store fakeCyberBlockStore
}

var _ GatewayCache = (*comboCacheAndStore)(nil)
var _ CyberSessionBlockStore = (*comboCacheAndStore)(nil)

func (c *comboCacheAndStore) GetSessionAccountID(_ context.Context, _ int64, _ string) (int64, error) {
	return 0, errors.New("stub")
}
func (c *comboCacheAndStore) SetSessionAccountID(_ context.Context, _ int64, _ string, _ int64, _ time.Duration) error {
	return nil
}
func (c *comboCacheAndStore) RefreshSessionTTL(_ context.Context, _ int64, _ string, _ time.Duration) error {
	return nil
}
func (c *comboCacheAndStore) DeleteSessionAccountID(_ context.Context, _ int64, _ string) error {
	return nil
}

func (c *comboCacheAndStore) SetGrokVideoPendingBilling(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}
func (c *comboCacheAndStore) GetGrokVideoPendingBilling(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (c *comboCacheAndStore) ClaimGrokVideoBilled(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (c *comboCacheAndStore) ReleaseGrokVideoBilled(_ context.Context, _ string) error {
	return nil
}

func (c *comboCacheAndStore) SetReasoningContent(_ context.Context, _ string, _ string, _ time.Duration) error {
	return nil
}
func (c *comboCacheAndStore) GetReasoningContent(_ context.Context, _ string) (string, error) {
	return "", ErrReasoningContentNotFound
}

func (c *comboCacheAndStore) SetCyberSessionBlocked(ctx context.Context, scopeKey string, keys []string, ttl time.Duration) error {
	return c.store.SetCyberSessionBlocked(ctx, scopeKey, keys, ttl)
}
func (c *comboCacheAndStore) IsCyberSessionScopeActive(ctx context.Context, scopeKey string) (bool, error) {
	return c.store.IsCyberSessionScopeActive(ctx, scopeKey)
}
func (c *comboCacheAndStore) FindCyberSessionBlocked(ctx context.Context, keys []string) (string, error) {
	return c.store.FindCyberSessionBlocked(ctx, keys)
}

// --- tests ---

// TestIsCyberSessionBlocked_EmptyKeyAndNilService covers the fail-open paths:
// empty key, nil service, store missing → always false / no panic.
func TestFindCyberSessionBlocked_EmptyAndNilService(t *testing.T) {
	var nilSvc *OpenAIGatewayService
	require.Empty(t, nilSvc.FindCyberSessionBlockedForRequest(context.Background(), 1, nil, nil, "", ""))
	require.NotPanics(t, func() { nilSvc.MarkCyberSessionBlocked(context.Background(), "", []string{"k"}) })

	svc := &OpenAIGatewayService{}
	require.Empty(t, svc.FindCyberSessionBlockedForRequest(context.Background(), 1, nil, nil, "", ""))
}

// TestCyberSessionBlock_RoundTrip exercises the type-assertion success path:
// mark a session blocked via a combo cache+store, then confirm IsCyberSessionBlocked
// returns true, and an unrelated key returns false.
func TestCyberSessionBlock_RoundTrip(t *testing.T) {
	// SettingService with only settingRepo set — GetCyberSessionBlockRuntime needs
	// nothing else (cfg/proxyRepo/etc. are not touched by this code path).
	settingSvc := &SettingService{
		settingRepo: &fakeSettingRepo{
			vals: map[string]string{
				SettingKeyCyberSessionBlockEnabled:    "true",
				SettingKeyCyberSessionBlockTTLSeconds: "60",
			},
		},
	}

	combo := &comboCacheAndStore{}
	svc := &OpenAIGatewayService{
		cache:          combo,
		settingService: settingSvc,
	}

	ctx := context.Background()
	const testKey = "deadbeef1234"

	c, body := newCyberBlockTestCtx(map[string]string{"session_id": "sess-roundtrip"}, `{}`)
	explicitKey := CyberSessionExplicitBlockKey(1, c, body)
	require.Empty(t, svc.FindCyberSessionBlockedForRequest(ctx, 1, c, body, "203.0.113.1", "client/1.0"))

	svc.MarkCyberSessionBlocked(ctx, "", []string{explicitKey, testKey})

	require.Equal(t, explicitKey, svc.FindCyberSessionBlockedForRequest(ctx, 1, c, body, "203.0.113.1", "client/1.0"))
}

func TestCyberSessionExplicitBlockKeyUsesSessionHeadersOnly(t *testing.T) {
	var expected string
	for _, header := range []string{"session-id", "session_id", "conversation_id", "X-Session-Id", "X-OpenCode-Session", "X-Conversation-ID"} {
		c, body := newCyberBlockTestCtx(map[string]string{header: " conversation-a "}, `{"prompt_cache_key":"shared-cache"}`)
		key := CyberSessionExplicitBlockKey(7, c, body)
		require.NotEmpty(t, key, header)
		if expected == "" {
			expected = key
		}
		require.Equal(t, expected, key, "header aliases must identify the same conversation")
		require.NotEqual(t, key, CyberSessionExplicitBlockKey(8, c, body))
		require.Empty(t, CyberSessionExplicitBlockKey(0, c, body))
		require.Empty(t, CyberSessionExplicitBlockKey(-1, c, body))
	}
	for _, body := range []string{`{}`, `{"prompt_cache_key":"conversation-a"}`, `{"type":"response.create","response":{"prompt_cache_key":"conversation-a","input":"hello"}}`} {
		c, b := newCyberBlockTestCtx(map[string]string{"X-Session-Affinity": "conversation-a"}, body)
		require.Empty(t, CyberSessionExplicitBlockKey(7, c, b))
	}
	require.Empty(t, CyberSessionExplicitBlockKey(7, nil, nil))
	require.Empty(t, CyberSessionExplicitBlockKey(7, &gin.Context{}, nil))
}

func TestCyberSessionIsolationAcrossSharedKeyAndLongHistory(t *testing.T) {
	ctx := context.Background()
	combo := &comboCacheAndStore{}
	svc := &OpenAIGatewayService{cache: combo, settingService: &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{
		SettingKeyCyberSessionBlockEnabled: "true", SettingKeyCyberSessionBlockTTLSeconds: "60",
	}}}}
	// Same input, cache hint and source cannot merge distinct session IDs.
	items := make([]map[string]string, 600)
	for i := range items {
		items[i] = map[string]string{"type": "function_call_output", "call_id": "call-" + strconv.Itoa(i), "output": "ordinary tool result"}
	}
	payload, err := json.Marshal(map[string]any{"input": items, "prompt_cache_key": "shared-cache"})
	require.NoError(t, err)
	a, _ := newCyberBlockTestCtx(map[string]string{"session_id": "conversation-a"}, string(payload))
	aKey := CyberSessionExplicitBlockKey(7, a, payload)
	legacyScope := sha256.Sum256([]byte("cyber-scope:v1|api_key=7|ip=203.0.113.20|ua=" + NormalizeSessionUserAgent("Codex CLI 1.2.3")))
	svc.MarkCyberSessionBlocked(ctx, hex.EncodeToString(legacyScope[:]), []string{aKey})

	for _, tc := range []struct {
		name, session string
		apiKey        int64
		blocked       bool
	}{
		{"original session remains blocked", "conversation-a", 7, true},
		{"another session shares key and history", "conversation-b", 7, false},
		{"another authenticated key reuses session name", "conversation-a", 8, false},
		{"no explicit identity with shared cache", "", 7, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newCyberBlockTestCtx(map[string]string{"session_id": tc.session}, string(payload))
			got := svc.FindCyberSessionBlockedForRequest(ctx, tc.apiKey, c, payload, "203.0.113.20", "Codex CLI 1.2.3")
			if tc.blocked {
				require.Equal(t, aKey, got)
			} else {
				require.Empty(t, got)
			}
		})
	}
	// A reconnect/model change and a different network must not evade an exact block.
	require.Equal(t, aKey, svc.FindCyberSessionBlockedForRequest(ctx, 7, a, []byte(`{"model":"another-model","input":"next turn"}`), "203.0.113.99", "AnotherClient/2"))
	require.Zero(t, combo.store.scopeCalls, "a shared source is never a blocking identity")
}

func TestCyberSessionIgnoresLegacyCacheBlocks(t *testing.T) {
	ctx := context.Background()
	combo := &comboCacheAndStore{}
	svc := &OpenAIGatewayService{cache: combo, settingService: &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{SettingKeyCyberSessionBlockEnabled: "true"}}}}
	// Before v2, a prompt_cache_key used the same digest as a session header.
	legacy := sha256.Sum256([]byte(isolateOpenAISessionID(7, "shared-value")))
	svc.MarkCyberSessionBlocked(ctx, "legacy-scope", []string{hex.EncodeToString(legacy[:])})
	c, body := newCyberBlockTestCtx(map[string]string{"session_id": "shared-value"}, `{}`)
	require.Empty(t, svc.FindCyberSessionBlockedForRequest(ctx, 7, c, body, "", ""))
}

func TestCyberSessionLookupFailOpen(t *testing.T) {
	c, body := newCyberBlockTestCtx(map[string]string{"session_id": "session-a"}, `{}`)
	for _, enabled := range []string{"false", "true"} {
		t.Run(enabled, func(t *testing.T) {
			combo := &comboCacheAndStore{store: fakeCyberBlockStore{findErr: errors.New("storage unavailable")}}
			svc := &OpenAIGatewayService{cache: combo, settingService: &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{SettingKeyCyberSessionBlockEnabled: enabled}}}}
			require.Empty(t, svc.FindCyberSessionBlockedForRequest(context.Background(), 7, c, body, "", ""))
			if enabled == "false" {
				require.Zero(t, combo.store.findCalls)
			} else {
				require.Equal(t, 1, combo.store.findCalls)
			}
		})
	}
}
