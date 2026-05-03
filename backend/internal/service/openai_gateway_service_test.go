package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/cespare/xxhash/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 编译期接口断言
var _ AccountRepository = (*stubOpenAIAccountRepo)(nil)
var _ GatewayCache = (*stubGatewayCache)(nil)

type stubOpenAIAccountRepo struct {
	AccountRepository
	accounts []Account
}

type snapshotUpdateAccountRepo struct {
	stubOpenAIAccountRepo
	updateExtraCalls chan map[string]any
}

type splitPoolOpenAIAccountRepo struct {
	stubOpenAIAccountRepo
	ungrouped []Account
	broad     []Account
}

func (r splitPoolOpenAIAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	result := make([]Account, 0, len(r.broad))
	for _, acc := range r.broad {
		if acc.Platform == platform {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (r splitPoolOpenAIAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	result := make([]Account, 0, len(r.ungrouped))
	for _, acc := range r.ungrouped {
		if acc.Platform == platform {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (r splitPoolOpenAIAccountRepo) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

func (r *snapshotUpdateAccountRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	if r.updateExtraCalls != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
		}
		r.updateExtraCalls <- copied
	}
	return nil
}

func (r stubOpenAIAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
		}
	}
	return nil, errors.New("account not found")
}

func (r stubOpenAIAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range r.accounts {
		if acc.Platform == platform {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (r stubOpenAIAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range r.accounts {
		if acc.Platform == platform {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (r stubOpenAIAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

func (r stubOpenAIAccountRepo) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range r.accounts {
		if acc.Platform == platform {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (r stubOpenAIAccountRepo) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	result := make([]Account, 0, len(r.accounts))
	result = append(result, r.accounts...)
	return result, nil
}

type rateLimitedFilteredOpenAIAccountRepo struct {
	stubOpenAIAccountRepo
}

func (r rateLimitedFilteredOpenAIAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	accounts, err := r.stubOpenAIAccountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, platform)
	if err != nil {
		return nil, err
	}
	return filterSchedulableAccounts(accounts), nil
}

func (r rateLimitedFilteredOpenAIAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	accounts, err := r.stubOpenAIAccountRepo.ListSchedulableByPlatform(ctx, platform)
	if err != nil {
		return nil, err
	}
	return filterSchedulableAccounts(accounts), nil
}

func (r rateLimitedFilteredOpenAIAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	accounts, err := r.stubOpenAIAccountRepo.ListSchedulableUngroupedByPlatform(ctx, platform)
	if err != nil {
		return nil, err
	}
	return filterSchedulableAccounts(accounts), nil
}

func filterSchedulableAccounts(accounts []Account) []Account {
	filtered := make([]Account, 0, len(accounts))
	for _, acc := range accounts {
		if acc.IsSchedulable() {
			filtered = append(filtered, acc)
		}
	}
	return filtered
}

func TestListSchedulableAccounts_UngroupedKeepsUngroupedPool(t *testing.T) {
	ctx := context.Background()
	repo := splitPoolOpenAIAccountRepo{
		ungrouped: []Account{{ID: 64, Platform: PlatformOpenAI, Status: StatusActive}},
		broad: []Account{
			{ID: 64, Platform: PlatformOpenAI, Status: StatusActive},
			{ID: 65, Platform: PlatformOpenAI, Status: StatusActive},
			{ID: 66, Platform: PlatformOpenAI, Status: StatusActive},
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo: repo,
	}

	accounts, err := svc.listSchedulableAccounts(ctx, nil, TargetGroupAny)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, int64(64), accounts[0].ID)
}

func TestBuildReserveCandidatePool_FillsToSixtyPercentUsingFreeActiveCapacity(t *testing.T) {
	exhaustedAccounts := []Account{{
		ID:          1,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{"plan_type": "free"},
		Extra:       map[string]any{"codex_7d_used_percent": 100.0},
	}}

	activeAccounts := []Account{
		{
			ID:          11,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{"plan_type": "free"},
			Extra:       map[string]any{"codex_7d_used_percent": 10.0},
		},
		{
			ID:          12,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{"plan_type": "free"},
			Extra:       map[string]any{"codex_7d_used_percent": 30.0},
		},
		{
			ID:          13,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 5,
			Credentials: map[string]any{"plan_type": "plus"},
		},
		{
			ID:          14,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 5,
		},
	}

	reserveAccounts := buildOpenAIReservePool(activeAccounts, exhaustedAccounts)
	require.Len(t, reserveAccounts, 1)
	require.Equal(t, int64(11), reserveAccounts[0].ID)
	require.Equal(t, 1, calculateOpenAIConcurrentCapacity(reserveAccounts))
	require.Equal(t, 3, calculateOpenAIConcurrentCapacity(exhaustedAccounts)+calculateOpenAIConcurrentCapacity(reserveAccounts))
}

func TestBuildReserveCandidatePool_PrefersHigherRemainingQuotaScore(t *testing.T) {
	activeAccounts := []Account{
		{
			ID:          21,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{"plan_type": "free"},
			Extra:       map[string]any{"codex_7d_used_percent": 80.0},
		},
		{
			ID:          22,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{"plan_type": "free"},
			Extra:       map[string]any{"codex_7d_used_percent": 10.0},
		},
	}

	reserveAccounts := buildOpenAIReservePool(activeAccounts, []Account{{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"plan_type": "free"},
		Extra:       map[string]any{"codex_7d_used_percent": 100.0},
	}})

	require.Len(t, reserveAccounts, 1)
	require.Equal(t, int64(22), reserveAccounts[0].ID)
}

func TestOpenAIRemainingQuotaScore_UsesMoreConservativeWindow(t *testing.T) {
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"plan_type": "free"},
		Extra: map[string]any{
			"codex_7d_used_percent":      20.0,
			"codex_primary_used_percent": 80.0,
		},
	}

	require.Equal(t, 20.0, account.OpenAIRemainingQuotaScore())
}

func TestOpenAIRemainingQuotaScore_InvalidPercentReturnsUnknown(t *testing.T) {
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"plan_type": "free"},
		Extra: map[string]any{
			"codex_7d_used_percent": "not-a-number",
		},
	}

	require.Equal(t, -1.0, account.OpenAIRemainingQuotaScore())
}

func TestShouldUseReserveForExhaustedOverflow_BelowThreshold(t *testing.T) {
	exhaustedAccounts := []Account{{ID: 31, Concurrency: 10}}
	reserveAccounts := []Account{{ID: 32, Concurrency: 4}}
	loadMap := map[int64]*AccountLoadInfo{
		31: {AccountID: 31, CurrentConcurrency: 6, LoadRate: 60},
	}

	require.False(t, shouldRouteExhaustedOverflowToReserve(exhaustedAccounts, reserveAccounts, loadMap))
}

func TestShouldUseReserveForExhaustedOverflow_AboveThreshold(t *testing.T) {
	exhaustedAccounts := []Account{{ID: 41, Concurrency: 10}}
	reserveAccounts := []Account{{ID: 42, Concurrency: 4}}
	loadMap := map[int64]*AccountLoadInfo{
		41: {AccountID: 41, CurrentConcurrency: 7, LoadRate: 70},
	}

	require.True(t, shouldRouteExhaustedOverflowToReserve(exhaustedAccounts, reserveAccounts, loadMap))
}

func TestShouldUseReserveForExhaustedOverflow_SparseLoadMapDoesNotUnderestimate(t *testing.T) {
	exhaustedAccounts := []Account{{ID: 51, Concurrency: 5}, {ID: 52, Concurrency: 5}}
	reserveAccounts := []Account{{ID: 53, Concurrency: 2}}
	loadMap := map[int64]*AccountLoadInfo{
		51: {AccountID: 51, CurrentConcurrency: 2, LoadRate: 40},
	}

	require.True(t, shouldRouteExhaustedOverflowToReserve(exhaustedAccounts, reserveAccounts, loadMap))
}

type stubConcurrencyCache struct {
	ConcurrencyCache
	loadBatchErr    error
	loadMap         map[int64]*AccountLoadInfo
	acquireResults  map[int64]bool
	acquireErrors   map[int64]error
	waitCounts      map[int64]int
	skipDefaultLoad bool
}

type cancelReadCloser struct{}

func (c cancelReadCloser) Read(p []byte) (int, error) { return 0, context.Canceled }
func (c cancelReadCloser) Close() error               { return nil }

type resetReadCloser struct{ err error }

func (r resetReadCloser) Read(p []byte) (int, error) { return 0, r.err }
func (r resetReadCloser) Close() error               { return nil }

type stubHTTPUpstream struct {
	lastRequest *http.Request
	lastBody    []byte
	response    *http.Response
	err         error
}

func (s *stubHTTPUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	s.lastRequest = req
	if req != nil && req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		s.lastBody = append([]byte(nil), body...)
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.response != nil {
		return s.response, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`)),
	}, nil
}

func (s *stubHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func newOpenAITestContext(t *testing.T, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	return c, rec
}

func readJSONRequestBody(t *testing.T, req *http.Request) map[string]any {
	t.Helper()
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	return parsed
}

func decodeJSONMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	return parsed
}

func newOpenAITestGatewayService(httpUpstream HTTPUpstream) *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}, httpUpstream: httpUpstream}
}

type errReadCloser struct {
	err error
}

func (r errReadCloser) Read([]byte) (int, error) { return 0, r.err }
func (r errReadCloser) Close() error             { return nil }

type failingGinWriter struct {
	gin.ResponseWriter
	failAfter int
	writes    int
}

func (w *failingGinWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("write failed")
	}
	w.writes++
	return w.ResponseWriter.Write(p)
}

func (c stubConcurrencyCache) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	if c.acquireErrors != nil {
		if err, ok := c.acquireErrors[accountID]; ok {
			return false, err
		}
	}
	if c.acquireResults != nil {
		if result, ok := c.acquireResults[accountID]; ok {
			return result, nil
		}
	}
	return true, nil
}

func (c stubConcurrencyCache) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	return nil
}

func (c stubConcurrencyCache) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	if c.loadBatchErr != nil {
		return nil, c.loadBatchErr
	}
	out := make(map[int64]*AccountLoadInfo, len(accounts))
	if c.skipDefaultLoad && c.loadMap != nil {
		for _, acc := range accounts {
			if load, ok := c.loadMap[acc.ID]; ok {
				out[acc.ID] = load
			}
		}
		return out, nil
	}
	for _, acc := range accounts {
		if c.loadMap != nil {
			if load, ok := c.loadMap[acc.ID]; ok {
				out[acc.ID] = load
				continue
			}
		}
		out[acc.ID] = &AccountLoadInfo{AccountID: acc.ID, LoadRate: 0}
	}
	return out, nil
}

func TestOpenAIGatewayService_GenerateSessionHash_Priority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	svc := &OpenAIGatewayService{}

	bodyWithKey := []byte(`{"prompt_cache_key":"ses_aaa"}`)

	// 1) session_id header wins
	c.Request.Header.Set("session_id", "sess-123")
	c.Request.Header.Set("conversation_id", "conv-456")
	h1 := svc.GenerateSessionHash(c, bodyWithKey)
	if h1 == "" {
		t.Fatalf("expected non-empty hash")
	}

	// 2) conversation_id used when session_id absent
	c.Request.Header.Del("session_id")
	h2 := svc.GenerateSessionHash(c, bodyWithKey)
	if h2 == "" {
		t.Fatalf("expected non-empty hash")
	}
	if h1 == h2 {
		t.Fatalf("expected different hashes for different keys")
	}

	// 3) prompt_cache_key used when both headers absent
	c.Request.Header.Del("conversation_id")
	h3 := svc.GenerateSessionHash(c, bodyWithKey)
	if h3 == "" {
		t.Fatalf("expected non-empty hash")
	}
	if h2 == h3 {
		t.Fatalf("expected different hashes for different keys")
	}

	// 4) empty when no signals
	h4 := svc.GenerateSessionHash(c, []byte(`{}`))
	if h4 != "" {
		t.Fatalf("expected empty hash when no signals")
	}
}

func TestStripOpenAIBuiltinToolsFieldFromBody_PreservesOriginalFormatting(t *testing.T) {
	body := []byte("{\n  \"model\": \"gpt-5.4\",\n  \"builtin_tools\": true,\n  \"tools\": {\"unexpected\": true}\n}\n")

	strippedBody, changed := stripOpenAIBuiltinToolsFieldFromBody(body)
	require.True(t, changed)
	require.NotContains(t, string(strippedBody), `"builtin_tools"`)
	require.Contains(t, string(strippedBody), "\n  \"model\": \"gpt-5.4\",")
	require.Contains(t, string(strippedBody), "\n  \"tools\": {\"unexpected\": true}\n")
}

func TestForwardResponsesRequest_ObjectBuiltinToolsAugmentsAndStripsPrivateField(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","builtin_tools":{"web_search":true},"tool_choice":"required","tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 2)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "get_weather", tools[0].(map[string]any)["name"])
	require.Equal(t, "web_search", tools[1].(map[string]any)["type"])
	require.Equal(t, "required", upstreamBody["tool_choice"])
}

func TestForwardResponsesRequest_NonOpenCodeDoesNotRehydrateGeneratedImageMarker(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	body := []byte(`{"model":"gpt-5.5","input":"sub2api-image://` + testImageID + `"}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "curl/8.0")
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = store
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	_, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.Contains(t, string(upstream.lastBody), "sub2api-image://"+testImageID)
	require.NotContains(t, string(upstream.lastBody), "data:image")
}

func TestRehydrateOpenCodeGeneratedImagesForResponsesSkipsNonOpenCodeClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "curl/8.0")
	svc := &OpenAIGatewayService{generatedImageStore: newTestStoreWithImage(t, testImageID, "png", pngBytes)}
	req := map[string]any{"input": []any{
		openCodeSub2APIImageMessageForTest(testImageID, "Generated image saved by sub2api.\nImage reference: "+openCodeSpecificImageMarkerForTest(testImageID)),
	}}

	changed, err := svc.RehydrateOpenCodeGeneratedImagesForResponses(context.Background(), c, req)

	require.NoError(t, err)
	require.False(t, changed)
	encoded := string(mustJSONBytes(t, req))
	require.NotContains(t, encoded, openCodeGeneratedImageToolName)
	require.NotContains(t, encoded, "call_sub2api_image_"+testImageID)
	require.NotContains(t, encoded, "data:image")
}

func TestRehydrateOpenCodeGeneratedImagesForResponsesThenForwardRedactsOpsUpstreamBody(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	body := mustJSONBytes(t, map[string]any{
		"model": "gpt-5.5",
		"input": []any{
			openCodeSub2APIImageMessageForTest(testImageID, strings.Join([]string{
				"Generated image: " + openCodeLegacyImageMarkerForTest(testImageID),
				"Download: " + openCodeGeneratedImageDownloadURLForTest("https://example.com", testImageID),
			}, "\n")),
		},
	})
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = store
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	reqBody := decodeJSONMap(t, body)
	changed, err := svc.RehydrateOpenCodeGeneratedImagesForResponses(context.Background(), c, reqBody)
	require.NoError(t, err)
	require.True(t, changed)
	body = mustJSONBytes(t, reqBody)
	c.Set(OpenAIParsedRequestBodyKey, reqBody)

	_, err = svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	v, ok := c.Get(OpsUpstreamRequestBodyKey)
	require.True(t, ok)
	opsBody := string(v.([]byte))
	require.NotContains(t, opsBody, "data:image")
	require.NotContains(t, opsBody, pngB64)
	require.Contains(t, opsBody, "[redacted-input-image]")
	require.Contains(t, string(upstream.lastBody), "data:image/png;base64,")
}

func TestForwardResponsesRequest_OpenCodeDoesNotRehydrateGeneratedImageMarkersInForward(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	body := mustJSONBytes(t, map[string]any{
		"model": "gpt-5.5",
		"input": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "restore " + openCodeSpecificImageMarkerForTest(testImageID)}}},
		},
	})
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = store
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	_, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotContains(t, string(upstream.lastBody), "data:image")
	require.NotContains(t, string(upstream.lastBody), openCodeGeneratedImageToolName)
	require.Contains(t, string(upstream.lastBody), openCodeSpecificImageMarkerForTest(testImageID))
}

func TestForwardResponsesRequest_PreinsertedOpenCodeImageToolPairNotDuplicatedInForward(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	callID := "call_sub2api_image_" + testImageID
	body := mustJSONBytes(t, map[string]any{
		"model": "gpt-5.5",
		"input": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "restore " + openCodeSpecificImageMarkerForTest(testImageID)}}},
			map[string]any{"type": "function_call", "call_id": callID, "name": openCodeGeneratedImageToolName, "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": callID, "output": []any{
				map[string]any{"type": "input_text", "text": "restored"},
				map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + pngB64},
			}},
		},
	})
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = store
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	_, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	upstreamBody := string(upstream.lastBody)
	require.Equal(t, 1, strings.Count(upstreamBody, `"name":"`+openCodeGeneratedImageToolName+`"`))
	require.Equal(t, 2, strings.Count(upstreamBody, `"call_id":"`+callID+`"`))
}

func TestOpenCodeImageToolOutputArraySurvivesCodexTransform(t *testing.T) {
	callID := "call_sub2api_image_" + testImageID
	req := map[string]any{
		"model": "gpt-5.5",
		"input": []any{
			map[string]any{"type": "function_call", "call_id": callID, "name": openCodeGeneratedImageToolName, "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": callID, "output": []any{
				map[string]any{"type": "input_text", "text": "restored"},
				map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + pngB64},
			}},
		},
	}

	result := applyCodexOAuthTransform(req, false, false)

	require.True(t, result.Modified)
	encoded := mustJSONBytes(t, req)
	require.True(t, gjson.GetBytes(encoded, "input.1.output").IsArray())
	require.Equal(t, "input_image", gjson.GetBytes(encoded, "input.1.output.1.type").String())
	require.Equal(t, gjson.GetBytes(encoded, "input.0.call_id").String(), gjson.GetBytes(encoded, "input.1.call_id").String())
}

func TestOpenCodeImageToolOutputRedactsRuntimeAndOpsErrorBodies(t *testing.T) {
	sample := "aGVsbG8="
	imageURL := "data:image/png;base64," + sample
	callID := "call_sub2api_image_" + testImageID
	rawRequestBody := mustJSONBytes(t, map[string]any{
		"model": "gpt-5.5",
		"input": []any{
			map[string]any{"type": "function_call", "call_id": callID, "name": openCodeGeneratedImageToolName, "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": callID, "output": []any{
				map[string]any{"type": "input_text", "text": "ok"},
				map[string]any{"type": "input_image", "image_url": imageURL},
			}},
		},
	})
	upstreamErrorBody := mustJSONBytes(t, map[string]any{
		"error": map[string]any{"message": "Instructions are required " + imageURL, "detail": imageURL},
	})
	assertNoImageLeak := func(label string, value any) {
		t.Helper()
		text := fmt.Sprint(value)
		require.NotContains(t, text, "data:image", label)
		require.NotContains(t, text, sample, label)
	}

	upstream := &stubHTTPUpstream{response: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(upstreamErrorBody)),
	}}
	svc := newOpenAITestGatewayService(upstream)
	svc.cfg.Gateway.LogUpstreamErrorBody = true
	svc.cfg.Gateway.LogUpstreamErrorBodyMaxBytes = 4096
	c, _ := newOpenAITestContext(t, "/v1/responses", rawRequestBody)
	logSink, restoreLogs := captureStructuredLog(t)
	t.Cleanup(restoreLogs)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	_, err := svc.Forward(context.Background(), c, account, rawRequestBody)

	require.Error(t, err)
	for _, key := range []string{OpsUpstreamRequestBodyKey, OpsUpstreamErrorDetailKey, OpsUpstreamErrorsKey} {
		value, ok := c.Get(key)
		require.True(t, ok, "expected %s in gin context", key)
		assertNoImageLeak(key, value)
	}

	logSink.mu.Lock()
	eventCount := len(logSink.events)
	foundErrorLog := false
	var leakedRuntimeLog []string
	for _, event := range logSink.events {
		if event == nil {
			continue
		}
		if strings.Contains(event.Message, "OpenAI upstream error") || strings.Contains(event.Message, "400") {
			foundErrorLog = true
		}
		if strings.Contains(event.Message, "data:image") || strings.Contains(event.Message, sample) {
			leakedRuntimeLog = append(leakedRuntimeLog, "message")
		}
		for key, field := range event.Fields {
			text := fmt.Sprint(field)
			if strings.Contains(text, "data:image") || strings.Contains(text, sample) {
				leakedRuntimeLog = append(leakedRuntimeLog, key)
			}
		}
	}
	logSink.mu.Unlock()
	require.NotZero(t, eventCount)
	require.True(t, foundErrorLog)
	require.Empty(t, leakedRuntimeLog)

	var captured []*OpsInsertErrorLogInput
	opsSvc := &OpsService{opsRepo: &opsRepoMock{
		InsertErrorLogFn: func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
			captured = append(captured, input)
			return 1, nil
		},
		BatchInsertErrorLogsFn: func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
			captured = append(captured, inputs...)
			return int64(len(inputs)), nil
		},
	}}
	detail := imageURL
	require.NoError(t, opsSvc.RecordError(context.Background(), &OpsInsertErrorLogInput{
		ErrorPhase:          "upstream",
		ErrorType:           "upstream_error",
		ErrorBody:           imageURL,
		UpstreamErrorDetail: &detail,
	}, rawRequestBody))
	require.NoError(t, opsSvc.RecordErrorBatch(context.Background(), []*OpsInsertErrorLogInput{
		{
			ErrorPhase: "upstream",
			ErrorType:  "upstream_error",
			UpstreamErrors: []*OpsUpstreamErrorEvent{
				{Kind: "http_error", Message: "bad", Detail: detail, UpstreamResponseBody: detail, UpstreamRequestBody: string(rawRequestBody)},
			},
		},
		{ErrorPhase: "upstream", ErrorType: "upstream_error", ErrorBody: imageURL},
	}))
	require.NotEmpty(t, captured)
	foundRequestBodyJSON := false
	foundUpstreamErrorsJSON := false
	for _, input := range captured {
		if input.RequestBodyJSON != nil {
			foundRequestBodyJSON = true
			assertNoImageLeak("RequestBodyJSON", *input.RequestBodyJSON)
		}
		if input.UpstreamErrorDetail != nil {
			assertNoImageLeak("UpstreamErrorDetail", *input.UpstreamErrorDetail)
		}
		if input.UpstreamErrorsJSON != nil {
			foundUpstreamErrorsJSON = true
			assertNoImageLeak("UpstreamErrorsJSON", *input.UpstreamErrorsJSON)
		}
		assertNoImageLeak("ErrorBody", input.ErrorBody)
	}
	require.True(t, foundRequestBodyJSON)
	require.True(t, foundUpstreamErrorsJSON)
}

func TestOpenCodeImageToolOutputRedactsClientErrorEnvelope(t *testing.T) {
	sample := "aGVsbG8="
	imageURL := "data:image/png;base64," + sample
	requestBody := mustJSONBytes(t, map[string]any{
		"model": "gpt-5.5",
		"input": "hello",
	})
	upstreamErrorBody := mustJSONBytes(t, map[string]any{
		"error": map[string]any{
			"type":    "invalid_request_error",
			"code":    "invalid_image",
			"param":   "input",
			"message": "invalid generated image " + imageURL,
			"detail":  imageURL,
		},
	})
	upstream := &stubHTTPUpstream{response: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(upstreamErrorBody)),
	}}
	svc := newOpenAITestGatewayService(upstream)
	c, rec := newOpenAITestContext(t, "/v1/responses", requestBody)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	_, err := svc.Forward(context.Background(), c, account, requestBody)

	require.Error(t, err)
	require.NotContains(t, err.Error(), "data:image")
	require.NotContains(t, err.Error(), sample)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := rec.Body.String()
	require.NotContains(t, body, "data:image")
	require.NotContains(t, body, sample)
	require.Contains(t, body, "upstream")
}

func TestOpenCodeImageRuntimeLogRedactsUpstreamMessageFields(t *testing.T) {
	sample := "aGVsbG8="
	imageURL := "data:image/png;base64," + sample
	assertNoImageLeak := func(t testing.TB, label string, value any) {
		t.Helper()
		text := fmt.Sprint(value)
		require.NotContains(t, text, "data:image", label)
		require.NotContains(t, text, sample, label)
	}
	assertCapturedLogs := func(t *testing.T, logSink *inMemoryLogSink, wantMessagePart string) {
		t.Helper()
		logSink.mu.Lock()
		defer logSink.mu.Unlock()
		require.NotEmpty(t, logSink.events)
		found := false
		for _, event := range logSink.events {
			if event == nil {
				continue
			}
			if strings.Contains(event.Message, wantMessagePart) || strings.Contains(event.Message, "400") {
				found = true
			}
			assertNoImageLeak(t, "runtime log message", event.Message)
			for key, field := range event.Fields {
				assertNoImageLeak(t, "runtime log field "+key, field)
			}
		}
		require.True(t, found)
	}

	t.Run("instructions required debug field", func(t *testing.T) {
		body := mustJSONBytes(t, map[string]any{"error": map[string]any{"message": "Instructions are required " + imageURL}})
		logSink, restoreLogs := captureStructuredLog(t)
		t.Cleanup(restoreLogs)

		requestBody := mustJSONBytes(t, map[string]any{"input": "ok"})
		logOpenAIInstructionsRequiredDebug(context.Background(), nil, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, http.StatusBadRequest, "Instructions are required "+imageURL, requestBody, body)

		assertCapturedLogs(t, logSink, "Instructions are required")
	})

	t.Run("transient failover field", func(t *testing.T) {
		logSink, restoreLogs := captureStructuredLog(t)
		t.Cleanup(restoreLogs)
		msg := "an error occurred while processing your request; you can retry your request; help.openai.com request id " + imageURL

		logOpenAITransientProcessingFailover(context.Background(), nil, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, http.StatusBadRequest, "rid_1", msg)

		assertCapturedLogs(t, logSink, "transient processing error")
	})
}

func TestOpenCodeImageToolOutputStringFallbackDoesNotContainImageBytes(t *testing.T) {
	output := buildOpenCodeImageToolOutputStringFallback(testImageID, openCodeGeneratedImageDownloadURLForTest("https://example.com", testImageID))

	require.NotContains(t, output, "data:image")
	require.NotContains(t, output, "base64,")
	require.NotContains(t, output, pngB64)
	require.NotContains(t, output, openCodeGeneratedImageDownloadURLForTest("https://example.com", testImageID))
	require.Contains(t, output, openCodeSpecificImageMarkerForTest(testImageID))
}

func TestOpenCodeImageToolOutputAutoRetryRewritesArrayToStringFallback(t *testing.T) {
	callID := "call_sub2api_image_" + testImageID
	body := mustJSONBytes(t, map[string]any{
		"model": "gpt-5.5",
		"input": []any{
			map[string]any{"type": "function_call", "call_id": callID, "name": openCodeGeneratedImageToolName, "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": callID, "output": []any{
				map[string]any{"type": "input_text", "text": "restored"},
				map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + pngB64},
			}},
		},
	})
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{
		{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"function_call_output output must be a string"}}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_ok","object":"response","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))},
	}}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Set(OpenAIImageToolOutputArrayKey, true)
	parsedOriginal := decodeJSONMap(t, body)
	c.Set(OpenAIParsedRequestBodyKey, parsedOriginal)

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Contains(t, string(upstream.bodies[0]), "data:image/png;base64,"+pngB64)
	require.NotContains(t, string(upstream.bodies[1]), "data:image")
	require.NotContains(t, string(upstream.bodies[1]), "base64,")
	require.NotContains(t, string(upstream.bodies[1]), pngB64)
	require.Equal(t, gjson.String, gjson.GetBytes(upstream.bodies[1], "input.1.output").Type)
	require.Contains(t, gjson.GetBytes(upstream.bodies[1], "input.1.output").String(), openCodeSpecificImageMarkerForTest(testImageID))
	cached, ok := c.Get(OpenAIParsedRequestBodyKey)
	require.True(t, ok)
	require.True(t, gjson.GetBytes(mustJSONBytes(t, cached), "input.1.output").IsArray(), "fallback retry must not mutate global parsed request cache")
}

func TestOpenCodeImageToolOutputAutoRetryHandlesCodexNormalizedCallID(t *testing.T) {
	callID := "call_sub2api_image_" + testImageID
	body := mustJSONBytes(t, map[string]any{
		"model": "gpt-5.5",
		"input": []any{
			map[string]any{"type": "function_call", "call_id": callID, "name": openCodeGeneratedImageToolName, "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": callID, "output": []any{
				map[string]any{"type": "input_text", "text": "restored"},
				map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + pngB64},
			}},
		},
	})
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{
		{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"function_call_output output must be a string"}}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_ok","object":"response","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))},
	}}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "oauth-token"}}
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Set(OpenAIImageToolOutputArrayKey, true)
	c.Set(OpenAIParsedRequestBodyKey, decodeJSONMap(t, body))

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	firstCallID := gjson.GetBytes(upstream.bodies[0], "input.0.call_id").String()
	require.NotEqual(t, callID, firstCallID)
	require.Equal(t, firstCallID, gjson.GetBytes(upstream.bodies[0], "input.1.call_id").String())
	require.True(t, gjson.GetBytes(upstream.bodies[0], "input.1.output").IsArray())
	require.Equal(t, firstCallID, gjson.GetBytes(upstream.bodies[1], "input.1.call_id").String())
	require.Equal(t, gjson.String, gjson.GetBytes(upstream.bodies[1], "input.1.output").Type)
	require.NotContains(t, string(upstream.bodies[1]), "data:image")
	require.NotContains(t, string(upstream.bodies[1]), "base64,")
	require.Contains(t, gjson.GetBytes(upstream.bodies[1], "input.1.output").String(), openCodeSpecificImageMarkerForTest(testImageID))
}

func TestOpenCodeImageToolOutputFallbackSkipsRealToolWithSyntheticLookingCallID(t *testing.T) {
	callID := "call_sub2api_image_" + testImageID
	req := map[string]any{"input": []any{
		map[string]any{"type": "function_call", "call_id": callID, "name": "real_tool", "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": callID, "output": []any{
			map[string]any{"type": "input_text", "text": "real tool output"},
			map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + pngB64},
		}},
	}}

	changed := rewriteOpenCodeImageToolOutputsToStringFallback(req)

	require.False(t, changed)
	encoded := mustJSONBytes(t, req)
	require.True(t, gjson.GetBytes(encoded, "input.1.output").IsArray())
	require.Contains(t, string(encoded), "data:image/png;base64,"+pngB64)
}

func TestOpenCodeImageToolOutputUnavailableTextFallbackKeepsUnavailableSemantics(t *testing.T) {
	callID := "call_sub2api_image_" + testImageID
	req := map[string]any{"input": []any{
		map[string]any{"type": "function_call", "call_id": callID, "name": openCodeGeneratedImageToolName, "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": callID, "output": buildOpenCodeUnavailableImageToolParts(testImageID)},
	}}

	changed := rewriteOpenCodeImageToolOutputsToStringFallback(req)

	require.True(t, changed)
	output := gjson.GetBytes(mustJSONBytes(t, req), "input.1.output").String()
	require.Contains(t, output, "no longer available")
	require.NotContains(t, strings.ToLower(output), "restored")
	require.Contains(t, output, openCodeSpecificImageMarkerForTest(testImageID))
}

func TestOpenCodeImageToolOutputImageArrayFallbackSaysPixelsNotAttached(t *testing.T) {
	callID := "call_sub2api_image_" + testImageID
	req := map[string]any{"input": []any{
		map[string]any{"type": "function_call", "call_id": callID, "name": openCodeGeneratedImageToolName, "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": callID, "output": buildOpenCodeRehydratedInputImageParts(testImageID, "image/png", pngBytes)},
	}}

	changed := rewriteOpenCodeImageToolOutputsToStringFallback(req)

	require.True(t, changed)
	output := gjson.GetBytes(mustJSONBytes(t, req), "input.1.output").String()
	require.Contains(t, output, "not attached")
	require.NotContains(t, output, "data:image")
	require.NotContains(t, output, "base64,")
	require.NotContains(t, output, pngB64)
	require.Contains(t, output, openCodeSpecificImageMarkerForTest(testImageID))
}

func TestOpenCodeImageToolOutputAutoRetrySkipsNonOutputTypeErrors(t *testing.T) {
	callID := "call_sub2api_image_" + testImageID
	body := mustJSONBytes(t, map[string]any{
		"model": "gpt-5.5",
		"input": []any{
			map[string]any{"type": "function_call", "call_id": callID, "name": openCodeGeneratedImageToolName, "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": callID, "output": []any{
				map[string]any{"type": "input_text", "text": "restored"},
				map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + pngB64},
			}},
		},
	})
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"ordinary bad request"}}`))}}}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Set(OpenAIImageToolOutputArrayKey, true)

	_, err := svc.Forward(context.Background(), c, account, body)

	require.Error(t, err)
	require.Len(t, upstream.bodies, 1)
}

func TestShouldRetryOpenCodeImageOutputArrayCompatibilitySkipsWrittenResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set(OpenAIImageToolOutputArrayKey, true)
	_, err := c.Writer.Write([]byte("started"))
	require.NoError(t, err)

	require.False(t, shouldRetryOpenCodeImageOutputArrayCompatibility(c, http.StatusBadRequest, []byte(`{"error":{"message":"function_call_output output"}}`)))
}

func TestForwardResponsesRequest_OpenCodeUserLegacyMarkerDoesNotRehydrate(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	body := mustJSONBytes(t, map[string]any{
		"model": "gpt-5.5",
		"input": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "please discuss " + openCodeLegacyImageMarkerForTest(testImageID)}}},
		},
	})
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = store
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	_, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotContains(t, string(upstream.lastBody), "data:image")
	v, ok := c.Get(OpsUpstreamRequestBodyKey)
	require.True(t, ok)
	opsBody := string(v.([]byte))
	require.NotContains(t, opsBody, "[redacted-input-image]")
	require.NotContains(t, opsBody, "data:image")
}

func TestForwardResponsesRequest_OpenCodeImageGenerationContinuesServerSide(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"model":"gpt-5.5","stream":false,"input":[{"role":"user","content":[{"type":"input_text","text":"draw and continue"}]}],"builtin_tools":{"image_generation":true}}`)
	c, rec := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_img","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_continue","output":[{"id":"msg_continue","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"continued after image","annotations":[]}]}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}`)),
		},
	}}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = store
	svc.cfg.Server.FrontendURL = "https://example.com"
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.Equal(t, 2, upstream.callCount)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 6, result.Usage.OutputTokens)
	secondBody := string(upstream.bodies[1])
	require.Contains(t, secondBody, `"type":"function_call"`)
	require.Contains(t, secondBody, `"type":"function_call_output"`)
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, secondBody)
	require.Contains(t, secondBody, "Temporary download URL: https://example.com/sub2api/generated-images/img_")
	require.Contains(t, secondBody, "Immediately use the available shell, command-line, or network-access tool")
	require.Contains(t, secondBody, "The saved image URL expires quickly")
	require.Contains(t, secondBody, "Do not ask the user whether to download")
	require.Contains(t, secondBody, "Do not use ls or rtk ls as a preflight")
	require.Contains(t, secondBody, "Invoke-WebRequest")
	require.Contains(t, secondBody, "save it to a temporary directory")
	require.Contains(t, secondBody, "Only if no tool can access the URL")
	require.NotContains(t, secondBody, pngB64)
	clientBody := rec.Body.String()
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, clientBody)
	require.Contains(t, clientBody, "continued after image")
	require.Equal(t, int64(4), gjson.Get(clientBody, "usage.input_tokens").Int())
	require.Equal(t, int64(6), gjson.Get(clientBody, "usage.output_tokens").Int())
	require.Equal(t, int64(10), gjson.Get(clientBody, "usage.total_tokens").Int())
	require.NotContains(t, clientBody, `"name":"bash"`)
	require.NotContains(t, clientBody, "image_generation_call")
	require.NotContains(t, clientBody, pngB64)
}

func TestForwardResponsesRequest_OpenCodeNonStreamingSSEImageGenerationContinuesServerSide(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"model":"gpt-5.5","stream":false,"input":[{"role":"user","content":[{"type":"input_text","text":"draw and continue"}]}],"builtin_tools":{"image_generation":true}}`)
	c, rec := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	firstStream := strings.Join([]string{
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_img","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(firstStream)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_continue","output":[{"id":"msg_continue","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"continued after non-stream sse image","annotations":[]}]}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}`)),
		},
	}}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = store
	svc.cfg.Server.FrontendURL = "https://example.com"
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.Equal(t, 2, upstream.callCount)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 6, result.Usage.OutputTokens)
	secondBody := string(upstream.bodies[1])
	require.Contains(t, secondBody, `"type":"function_call"`)
	require.Contains(t, secondBody, `"type":"function_call_output"`)
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, secondBody)
	clientBody := rec.Body.String()
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, clientBody)
	require.Contains(t, clientBody, "continued after non-stream sse image")
	require.Equal(t, int64(4), gjson.Get(clientBody, "usage.input_tokens").Int())
	require.Equal(t, int64(6), gjson.Get(clientBody, "usage.output_tokens").Int())
	require.Equal(t, int64(10), gjson.Get(clientBody, "usage.total_tokens").Int())
	require.NotContains(t, clientBody, `"name":"bash"`)
	require.NotContains(t, clientBody, "image_generation_call")
	require.NotContains(t, clientBody, pngB64)
}

func TestForwardResponsesRequest_OpenCodeStreamingImageGenerationContinuesServerSide(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"role":"user","content":[{"type":"input_text","text":"draw and continue"}]}],"builtin_tools":{"image_generation":true}}`)
	c, rec := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	firstStream := strings.Join([]string{
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_img","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	secondStream := strings.Join([]string{
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"msg_continue","type":"message","status":"in_progress","role":"assistant","content":[]}}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","output_index":1,"content_index":0,"item_id":"msg_continue","delta":"continued stream after image"}`,
		"",
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":1,"item":{"id":"msg_continue","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"continued stream after image","annotations":[]}]}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_continue","output":[{"id":"msg_continue","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"continued stream after image","annotations":[]}]}],"usage":{"input_tokens":3,"output_tokens":4}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(firstStream))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(secondStream))},
	}}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = store
	svc.cfg.Server.FrontendURL = "https://example.com"
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.Equal(t, 2, upstream.callCount)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 6, result.Usage.OutputTokens)
	secondBody := string(upstream.bodies[1])
	require.Contains(t, secondBody, `"type":"function_call"`)
	require.Contains(t, secondBody, `"type":"function_call_output"`)
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, secondBody)
	clientBody := rec.Body.String()
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, clientBody)
	require.Contains(t, clientBody, "continued stream after image")
	require.Equal(t, 1, strings.Count(clientBody, "event: response.completed"))
	require.Equal(t, 1, strings.Count(clientBody, "data: [DONE]"))
	require.NotContains(t, clientBody, `"name":"bash"`)
	require.NotContains(t, clientBody, "image_generation_call")
	require.NotContains(t, clientBody, pngB64)
}

func TestForwardResponsesRequest_OpenCodeStreamingTerminalOnlyImageGenerationContinuesServerSide(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"role":"user","content":[{"type":"input_text","text":"draw and continue"}]}],"builtin_tools":{"image_generation":true}}`)
	c, rec := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	firstStream := strings.Join([]string{
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_img","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	secondStream := strings.Join([]string{
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"msg_continue","type":"message","status":"in_progress","role":"assistant","content":[]}}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","output_index":1,"content_index":0,"item_id":"msg_continue","delta":"continued stream after terminal-only image"}`,
		"",
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":1,"item":{"id":"msg_continue","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"continued stream after terminal-only image","annotations":[]}]}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_continue","output":[{"id":"msg_continue","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"continued stream after terminal-only image","annotations":[]}]}],"usage":{"input_tokens":3,"output_tokens":4}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(firstStream))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(secondStream))},
	}}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = store
	svc.cfg.Server.FrontendURL = "https://example.com"
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.Equal(t, 2, upstream.callCount)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 6, result.Usage.OutputTokens)
	secondBody := string(upstream.bodies[1])
	require.Contains(t, secondBody, `"type":"function_call"`)
	require.Contains(t, secondBody, `"type":"function_call_output"`)
	clientBody := rec.Body.String()
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, clientBody)
	require.Contains(t, clientBody, "continued stream after terminal-only image")
	require.Equal(t, 1, strings.Count(clientBody, "event: response.completed"))
	require.Equal(t, 1, strings.Count(clientBody, "data: [DONE]"))
	require.NotContains(t, clientBody, `"name":"bash"`)
	require.NotContains(t, clientBody, "image_generation_call")
	require.NotContains(t, clientBody, pngB64)
}

func TestForwardResponsesRequest_OpenCodeStreamingContinuationFailureFallsBackToFirstTerminal(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"role":"user","content":[{"type":"input_text","text":"draw and continue"}]}],"builtin_tools":{"image_generation":true}}`)
	c, rec := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	firstStream := strings.Join([]string{
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_img","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(firstStream))},
		{StatusCode: http.StatusBadGateway, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"continuation failed"}}`))},
	}}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = store
	svc.cfg.Server.FrontendURL = "https://example.com"
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	_, err := svc.Forward(context.Background(), c, account, body)

	require.Error(t, err)
	require.Equal(t, 2, upstream.callCount)
	clientBody := rec.Body.String()
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, clientBody)
	require.Equal(t, 1, strings.Count(clientBody, "event: response.completed"))
	require.Equal(t, 1, strings.Count(clientBody, "data: [DONE]"))
	require.NotContains(t, clientBody, `"name":"bash"`)
	require.NotContains(t, clientBody, "image_generation_call")
	require.NotContains(t, clientBody, pngB64)
}

func TestForwardResponsesRequest_MetadataBuiltinToolsAugmentsAndStripsPrivateField(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","metadata":{"builtin_tools":{"web_search":true},"trace_id":"trace-1","client":"opencode"},"tool_choice":"required","tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")
	require.NotContains(t, upstreamBody, "metadata")
	require.Equal(t, "required", upstreamBody["tool_choice"])

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 2)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "get_weather", tools[0].(map[string]any)["name"])
	require.Equal(t, "web_search", tools[1].(map[string]any)["type"])
}

func TestForwardResponsesRequest_OpenCodeMetadataImageGenerationCarrierAddsToolAndStripsMetadata(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5-Sys","metadata":{"builtin_tools":{"image_generation":{"model":"gpt-image-2","size":"1024x1024","output_format":"png"}},"client":"opencode"},"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	svc.generatedImageStore = newTestOpenAIGeneratedImageStore(t, fixedNow)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
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
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 1)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
}

func TestForwardResponsesRequest_OpenCodeWithoutCarrierDoesNotInjectImageGenerationTool(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5-Sys","tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 1)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
}

func TestForwardResponsesRequest_OpenCodeAndCodexWithoutCarrierOrToolsDoNotInjectImageGenerationTool(t *testing.T) {
	for _, tc := range []struct {
		name      string
		userAgent string
	}{
		{name: "opencode", userAgent: "opencode/1.0"},
		{name: "codex", userAgent: "Codex Desktop/1.2.3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.5-Sys","input":"draw a cat"}`)
			c, _ := newOpenAITestContext(t, "/v1/responses", body)
			c.Request.Header.Set("User-Agent", tc.userAgent)
			upstream := &stubHTTPUpstream{}
			svc := newOpenAITestGatewayService(upstream)
			account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
			_, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)

			upstreamBody := decodeJSONMap(t, upstream.lastBody)
			require.NotContains(t, upstreamBody, "tools")
		})
	}
}

func TestForwardResponsesRequest_ImageGenerationBuiltinToolsAugmentsConfiguredTool(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5-Sys","builtin_tools":{"image_generation":{"model":"gpt-image-2","size":"1024x1024","quality":"low","output_format":"png","partial_images":1,"ignored":"drop-me"}},"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 2)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	imageTool := tools[1].(map[string]any)
	require.Equal(t, "image_generation", imageTool["type"])
	require.Equal(t, "gpt-image-2", imageTool["model"])
	require.Equal(t, "1024x1024", imageTool["size"])
	require.Equal(t, "low", imageTool["quality"])
	require.Equal(t, "png", imageTool["output_format"])
	require.Equal(t, float64(1), imageTool["partial_images"])
	require.NotContains(t, imageTool, "ignored")
}

func TestForwardResponsesRequest_OAuthImageGenerationBuiltinToolChoicePreserved(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5-Sys","instructions":"draw with native image tool","input":"draw a cat","builtin_tools":{"image_generation":{"model":"gpt-image-2","output_format":"png"}},"tool_choice":{"type":"image_generation"}}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
	}

	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")
	require.NotContains(t, upstreamBody, "metadata")

	choice, ok := upstreamBody["tool_choice"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "image_generation", choice["type"])

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 1)
	imageTool := tools[0].(map[string]any)
	require.Equal(t, "image_generation", imageTool["type"])
	require.Equal(t, "gpt-image-2", imageTool["model"])
	require.Equal(t, "png", imageTool["output_format"])
}

func TestForwardResponsesRequest_BuiltinToolsArrayAddsWebSearchAndImageGeneration(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5-Sys","metadata":{"builtin_tools":["web_search","image_generation"]},"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "metadata")

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 3)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "web_search", tools[1].(map[string]any)["type"])
	imageTool := tools[2].(map[string]any)
	require.Equal(t, "image_generation", imageTool["type"])
	require.Equal(t, "gpt-image-2", imageTool["model"])
	require.Equal(t, "png", imageTool["output_format"])
}

func TestApplyOpenAIBuiltinToolsAugmentation_DoesNotDuplicateExistingWebSearch(t *testing.T) {
	reqBody := map[string]any{
		"builtin_tools": []any{"web_search", "code_interpreter"},
		"tools": []any{
			map[string]any{"type": "function", "name": "get_weather"},
			map[string]any{"type": "web_search"},
		},
	}

	changed := applyOpenAIBuiltinToolsAugmentation(reqBody)
	require.True(t, changed)
	require.NotContains(t, reqBody, "builtin_tools")

	tools := reqBody["tools"].([]any)
	require.Len(t, tools, 2)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "get_weather", tools[0].(map[string]any)["name"])
	require.Equal(t, "web_search", tools[1].(map[string]any)["type"])
}

func TestApplyOpenAIBuiltinToolsAugmentation_DoesNotDuplicateExistingImageGeneration(t *testing.T) {
	reqBody := map[string]any{
		"builtin_tools": map[string]any{"image_generation": true},
		"tools": []any{
			map[string]any{"type": "function", "name": "get_weather"},
			map[string]any{"type": "image_generation", "model": "gpt-image-2", "output_format": "webp"},
		},
	}

	changed := applyOpenAIBuiltinToolsAugmentation(reqBody)
	require.True(t, changed)
	require.NotContains(t, reqBody, "builtin_tools")

	tools := reqBody["tools"].([]any)
	require.Len(t, tools, 2)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	imageTool := tools[1].(map[string]any)
	require.Equal(t, "image_generation", imageTool["type"])
	require.Equal(t, "webp", imageTool["output_format"])
}

func TestApplyOpenAIBuiltinToolsAugmentation_InvalidToolsOnlyStripsPrivateField(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","builtin_tools":true,"tools":{"type":"function","name":"broken_tools_shape"}}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")
	require.IsType(t, map[string]any{}, upstreamBody["tools"])
	require.Equal(t, map[string]any{"type": "function", "name": "broken_tools_shape"}, upstreamBody["tools"])
}

func TestForwardResponsesRequest_PassthroughStripsBuiltinToolsWithoutAugmenting(t *testing.T) {
	rawBody := []byte(`{"model":"gpt-5.4","builtin_tools":true,"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", rawBody)
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
		Extra:       map[string]any{"openai_passthrough": true},
	}

	_, err := svc.Forward(context.Background(), c, account, rawBody)
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 1)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "get_weather", tools[0].(map[string]any)["name"])
	require.False(t, hasOpenAIBuiltinTool(tools, "web_search"))
}

func TestForwardResponsesRequest_CompactPathDoesNotAugmentBuiltinTools(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","builtin_tools":true,"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses/compact", body)
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 1)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "get_weather", tools[0].(map[string]any)["name"])
	require.False(t, hasOpenAIBuiltinTool(tools, "web_search"))
}

func TestForwardResponsesRequest_DualCarrierBuiltinToolsPrefersTopLevelAndStripsBoth(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","builtin_tools":false,"metadata":{"builtin_tools":{"web_search":true},"trace_id":"trace-2"},"tool_choice":"required","tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")
	require.Equal(t, "required", upstreamBody["tool_choice"])
	require.NotContains(t, upstreamBody, "metadata")

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 1)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.False(t, hasOpenAIBuiltinTool(tools, "web_search"))
}

func TestForwardResponsesRequest_PassthroughStripsBuiltinToolsWithoutAugmentingFromMetadata(t *testing.T) {
	rawBody := []byte(`{"model":"gpt-5.4","metadata":{"builtin_tools":{"web_search":true},"trace_id":"trace-pass"},"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", rawBody)
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
		Extra:       map[string]any{"openai_passthrough": true},
	}

	_, err := svc.Forward(context.Background(), c, account, rawBody)
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")
	require.NotContains(t, upstreamBody, "metadata")

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 1)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "get_weather", tools[0].(map[string]any)["name"])
	require.False(t, hasOpenAIBuiltinTool(tools, "web_search"))
}

func TestForwardResponsesRequest_CompactPathDoesNotAugmentBuiltinToolsFromMetadata(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","metadata":{"builtin_tools":{"web_search":true},"trace_id":"trace-compact"},"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses/compact", body)
	upstream := &stubHTTPUpstream{}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")
	require.NotContains(t, upstreamBody, "metadata")

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 1)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "get_weather", tools[0].(map[string]any)["name"])
	require.False(t, hasOpenAIBuiltinTool(tools, "web_search"))
}

func TestChatCompletionsBuiltinTools_AugmentsWebSearchWithoutChangingToolChoice(t *testing.T) {
	req := &apicompat.ChatCompletionsRequest{
		Model:        "gpt-5.4",
		Messages:     []apicompat.ChatMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}},
		BuiltinTools: true,
		ToolChoice:   json.RawMessage(`"auto"`),
	}

	changed := applyOpenAICompatBuiltinToolsAugmentation(req)
	require.True(t, changed)
	require.Nil(t, req.BuiltinTools)
	require.Len(t, req.Tools, 1)
	require.Equal(t, "web_search", req.Tools[0].Type)
	require.JSONEq(t, `"auto"`, string(req.ToolChoice))
}

func TestChatCompletionsBuiltinTools_DoesNotDuplicateExistingWebSearch(t *testing.T) {
	req := &apicompat.ChatCompletionsRequest{
		BuiltinTools: []any{"web_search", "code_interpreter"},
		Tools: []apicompat.ChatTool{
			{Type: "function", Function: &apicompat.ChatFunction{Name: "get_weather"}},
			{Type: "web_search"},
		},
	}

	changed := applyOpenAICompatBuiltinToolsAugmentation(req)
	require.True(t, changed)
	require.Nil(t, req.BuiltinTools)
	require.Len(t, req.Tools, 2)
	require.Equal(t, "function", req.Tools[0].Type)
	require.Equal(t, "web_search", req.Tools[1].Type)
}

func TestChatCompletionsBuiltinTools_ImageGenerationOnlyDoesNotAddWebSearch(t *testing.T) {
	req := &apicompat.ChatCompletionsRequest{
		BuiltinTools: map[string]any{"image_generation": true},
		Tools: []apicompat.ChatTool{
			{Type: "function", Function: &apicompat.ChatFunction{Name: "get_weather"}},
		},
	}

	changed := applyOpenAICompatBuiltinToolsAugmentation(req)
	require.True(t, changed)
	require.Nil(t, req.BuiltinTools)
	require.Len(t, req.Tools, 1)
	require.Equal(t, "function", req.Tools[0].Type)
}

func TestToolChoiceWhenObjectBuiltinToolsAdded_ForwardAsChatCompletionsPreservesRawChoice(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"builtin_tools":{"web_search":true},"tool_choice":"auto"}`)
	c, rec := newOpenAITestContext(t, "/v1/chat/completions", body)
	upstream := &stubHTTPUpstream{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(strings.Join([]string{
				`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
				`data: [DONE]`,
			}, "\n") + "\n")),
		},
	}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")
	require.Equal(t, "auto", upstreamBody["tool_choice"])

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 1)
	require.Equal(t, "web_search", tools[0].(map[string]any)["type"])
}

func TestToolChoiceWhenMetadataBuiltinToolsAdded_ForwardAsChatCompletionsPreservesRawChoice(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"metadata":{"builtin_tools":{"web_search":true},"trace_id":"trace-chat","client":"opencode"},"tool_choice":"auto"}`)
	c, rec := newOpenAITestContext(t, "/v1/chat/completions", body)
	upstream := &stubHTTPUpstream{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(strings.Join([]string{
				`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
				`data: [DONE]`,
			}, "\n") + "\n")),
		},
	}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")
	require.Equal(t, "auto", upstreamBody["tool_choice"])
	require.NotContains(t, upstreamBody, "metadata")

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 1)
	require.Equal(t, "web_search", tools[0].(map[string]any)["type"])
}

func TestResponsesShapeChatCompletionsBuiltinToolsAugmentsAndStripsPrivateField(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5-Sys","input":[{"role":"user","content":"draw a cat"}],"builtin_tools":{"image_generation":true},"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}],"tool_choice":{"type":"image_generation"}}`)
	c, rec := newOpenAITestContext(t, "/v1/chat/completions", body)
	upstream := &stubHTTPUpstream{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(strings.Join([]string{
				`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.5","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
				`data: [DONE]`,
			}, "\n") + "\n")),
		},
	}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")

	choice, ok := upstreamBody["tool_choice"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "image_generation", choice["type"])

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 2)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "get_weather", tools[0].(map[string]any)["name"])
	imageTool := tools[1].(map[string]any)
	require.Equal(t, "image_generation", imageTool["type"])
	require.Equal(t, "gpt-image-2", imageTool["model"])
	require.Equal(t, "png", imageTool["output_format"])
}

func TestResponsesShapeChatCompletionsBuiltinToolsMatchesExplicitToolPromptCacheKey(t *testing.T) {
	forward := func(t *testing.T, body []byte) map[string]any {
		t.Helper()
		c, rec := newOpenAITestContext(t, "/v1/chat/completions", body)
		upstream := &stubHTTPUpstream{
			response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(strings.Join([]string{
					`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
					`data: [DONE]`,
				}, "\n") + "\n")),
			},
		}
		svc := newOpenAITestGatewayService(upstream)
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
		}

		_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rec.Code)
		return decodeJSONMap(t, upstream.lastBody)
	}

	explicitToolBody := []byte(`{"model":"gpt-5.4","instructions":"draw with native image tool","input":[{"role":"user","content":"draw a cat"}],"tools":[{"type":"image_generation","model":"gpt-image-2","output_format":"png"}],"tool_choice":{"type":"image_generation"}}`)
	builtinToolBody := []byte(`{"model":"gpt-5.4","instructions":"draw with native image tool","input":[{"role":"user","content":"draw a cat"}],"builtin_tools":{"image_generation":true},"tool_choice":{"type":"image_generation"}}`)

	explicitUpstream := forward(t, explicitToolBody)
	builtinUpstream := forward(t, builtinToolBody)

	require.Equal(t, explicitUpstream["prompt_cache_key"], builtinUpstream["prompt_cache_key"])
	require.NotEmpty(t, builtinUpstream["prompt_cache_key"])
	require.NotContains(t, builtinUpstream, "builtin_tools")
	require.Equal(t, explicitUpstream["tools"], builtinUpstream["tools"])
}

func TestChatCompletionsBuiltinTools_AugmentsWebSearchAndPreservesFunctionTool(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"builtin_tools":["web_search"],"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object"}}}],"tool_choice":"auto"}`)
	c, _ := newOpenAITestContext(t, "/v1/chat/completions", body)
	upstream := &stubHTTPUpstream{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(strings.Join([]string{
				`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
				`data: [DONE]`,
			}, "\n") + "\n")),
		},
	}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)

	upstreamBody := decodeJSONMap(t, upstream.lastBody)
	require.NotContains(t, upstreamBody, "builtin_tools")
	require.Equal(t, "auto", upstreamBody["tool_choice"])

	tools := upstreamBody["tools"].([]any)
	require.Len(t, tools, 2)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "get_weather", tools[0].(map[string]any)["name"])
	require.Equal(t, "web_search", tools[1].(map[string]any)["type"])
}

func TestOpenAIGatewayService_GenerateSessionHash_UsesXSessionAffinityBeforePromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("x-session-affinity", "affinity-123")

	svc := &OpenAIGatewayService{}

	h1 := svc.GenerateSessionHash(c, []byte(`{"prompt_cache_key":"cache-key-1"}`))
	h2 := svc.GenerateSessionHash(c, []byte(`{"prompt_cache_key":"cache-key-2"}`))

	require.NotEmpty(t, h1)
	require.Equal(t, h1, h2)
	require.Equal(t, fmt.Sprintf("%016x", xxhash.Sum64String("affinity-123")), h1)
}

func TestOpenAIGatewayService_ExtractSessionID_PrefersXSessionAffinityOverPromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("x-session-affinity", "affinity-456")

	svc := &OpenAIGatewayService{}
	got := svc.ExtractSessionID(c, []byte(`{"prompt_cache_key":"cache-key-body"}`))

	require.Equal(t, "affinity-456", got)
}

func TestOpenAIGatewayService_GenerateSessionHash_UsesXXHash64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	c.Request.Header.Set("session_id", "sess-fixed-value")
	svc := &OpenAIGatewayService{}

	got := svc.GenerateSessionHash(c, nil)
	want := fmt.Sprintf("%016x", xxhash.Sum64String("sess-fixed-value"))
	require.Equal(t, want, got)
}

func TestOpenAIGatewayService_ListSchedulableAccounts_ExhaustedIncludesRateLimitedAccountWhenSourceIsSchedulableOnly(t *testing.T) {
	ctx := context.Background()
	groupID := int64(42)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)

	exhaustedRateLimited := Account{
		ID:               5001,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		RateLimitResetAt: &rateLimitedUntil,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
		},
	}
	active := Account{
		ID:          5002,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(10),
		},
	}

	repo := rateLimitedFilteredOpenAIAccountRepo{stubOpenAIAccountRepo{accounts: []Account{exhaustedRateLimited, active}}}

	t.Run("repository path", func(t *testing.T) {
		svc := &OpenAIGatewayService{accountRepo: repo, cfg: &config.Config{}}

		exhaustedAccounts, err := svc.listSchedulableAccounts(ctx, &groupID, TargetGroupExhausted)
		require.NoError(t, err)
		require.Len(t, exhaustedAccounts, 1)
		require.Equal(t, exhaustedRateLimited.ID, exhaustedAccounts[0].ID)

		activeAccounts, err := svc.listSchedulableAccounts(ctx, &groupID, TargetGroupActive)
		require.NoError(t, err)
		require.Len(t, activeAccounts, 1)
		require.Equal(t, active.ID, activeAccounts[0].ID)

		anyAccounts, err := svc.listSchedulableAccounts(ctx, &groupID, TargetGroupAny)
		require.NoError(t, err)
		require.Len(t, anyAccounts, 1)
		require.Equal(t, active.ID, anyAccounts[0].ID)
	})

	t.Run("snapshot path", func(t *testing.T) {
		snapshotCache := &openAISnapshotCacheStub{
			snapshotAccounts: []*Account{&active},
		}
		svc := &OpenAIGatewayService{
			accountRepo:       repo,
			cfg:               &config.Config{},
			schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache},
		}

		exhaustedAccounts, err := svc.listSchedulableAccounts(ctx, &groupID, TargetGroupExhausted)
		require.NoError(t, err)
		require.Len(t, exhaustedAccounts, 1)
		require.Equal(t, exhaustedRateLimited.ID, exhaustedAccounts[0].ID)

		activeAccounts, err := svc.listSchedulableAccounts(ctx, &groupID, TargetGroupActive)
		require.NoError(t, err)
		require.Len(t, activeAccounts, 1)
		require.Equal(t, active.ID, activeAccounts[0].ID)

		anyAccounts, err := svc.listSchedulableAccounts(ctx, &groupID, TargetGroupAny)
		require.NoError(t, err)
		require.Len(t, anyAccounts, 1)
		require.Equal(t, active.ID, anyAccounts[0].ID)
	})
}

func TestOpenAIGatewayService_ListSchedulableAccounts_ExhaustedSnapshotSameIDUsesBroadLatestState(t *testing.T) {
	ctx := context.Background()
	groupID := int64(420)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)

	staleSnapshotActive := Account{
		ID:          6101,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(10),
		},
	}
	broadLatestExhausted := Account{
		ID:               6101,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		RateLimitResetAt: &rateLimitedUntil,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
		},
	}

	repo := stubOpenAIAccountRepo{accounts: []Account{broadLatestExhausted}}
	snapshotCache := &openAISnapshotCacheStub{snapshotAccounts: []*Account{&staleSnapshotActive}}
	svc := &OpenAIGatewayService{
		accountRepo:       repo,
		cfg:               &config.Config{},
		schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache},
	}

	exhaustedAccounts, err := svc.listSchedulableAccounts(ctx, &groupID, TargetGroupExhausted)
	require.NoError(t, err)
	require.Len(t, exhaustedAccounts, 1)
	require.Equal(t, broadLatestExhausted.ID, exhaustedAccounts[0].ID)
	require.True(t, exhaustedAccounts[0].IsExhausted())
	require.True(t, exhaustedAccounts[0].IsRateLimited())
}

func TestOpenAIGatewayService_ListSchedulableAccounts_ExhaustedSnapshotStaleSameIDDropsWhenBroadLatestActive(t *testing.T) {
	ctx := context.Background()
	groupID := int64(421)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)

	staleSnapshotExhausted := Account{
		ID:               6201,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		RateLimitResetAt: &rateLimitedUntil,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
		},
	}
	broadLatestActive := Account{
		ID:          6201,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(10),
		},
	}

	repo := stubOpenAIAccountRepo{accounts: []Account{broadLatestActive}}
	snapshotCache := &openAISnapshotCacheStub{snapshotAccounts: []*Account{&staleSnapshotExhausted}}
	svc := &OpenAIGatewayService{
		accountRepo:       repo,
		cfg:               &config.Config{},
		schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache},
	}

	exhaustedAccounts, err := svc.listSchedulableAccounts(ctx, &groupID, TargetGroupExhausted)
	require.NoError(t, err)
	require.Empty(t, exhaustedAccounts)
}

func TestOpenAIGatewayService_ListOpenAIExhaustedWithReserveOverlay_UsesProjectionInsteadOfLiveBuckets(t *testing.T) {
	ctx := context.Background()
	groupID := int64(422)
	projectionExhausted := Account{
		ID:          6301,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}},
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(10),
		},
	}
	projectionReserve := Account{
		ID:          6302,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}},
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(10),
		},
	}
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&Account{
			ID:          6309,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}},
		}},
		openAIState: newOpenAIBucketStateForTest([]Account{projectionExhausted, projectionReserve}, 7, map[string]OpenAIModelRoleView{
			"gpt-5.6": {
				CanonicalModel:     "gpt-5.6",
				ExhaustedBaseIDs:   []int64{projectionExhausted.ID},
				ReserveOverflowIDs: []int64{projectionReserve.ID},
			},
		}),
	}
	svc := &OpenAIGatewayService{
		cfg:               &config.Config{},
		schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache},
	}

	exhaustedAccounts, reserveAccounts, err := svc.listOpenAIExhaustedWithReserveOverlay(ctx, &groupID, "gpt-5.6")
	require.NoError(t, err)
	require.Len(t, exhaustedAccounts, 1)
	require.Len(t, reserveAccounts, 1)
	require.Equal(t, projectionExhausted.ID, exhaustedAccounts[0].ID)
	require.Equal(t, projectionReserve.ID, reserveAccounts[0].ID)
}

func TestOpenAIGatewayService_ListOpenAIExhaustedWithReserveOverlay_GPT55SubsetPromotesReserve(t *testing.T) {
	ctx := context.Background()
	groupID := int64(4225)
	activeTeam := newOpenAIProjectionPaidTierAccount(6310, 1, "team", []string{"gpt-5.5"})
	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: groupID, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.5"},
		AccountsAll:      []Account{activeTeam},
	})
	snapshotCache := &openAISnapshotCacheStub{
		openAIState: newOpenAIBucketStateForTest([]Account{activeTeam}, 11, projection.Models),
	}
	svc := &OpenAIGatewayService{
		cfg:               &config.Config{},
		schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache},
	}

	exhaustedAccounts, reserveAccounts, err := svc.listOpenAIExhaustedWithReserveOverlay(ctx, &groupID, "gpt-5.5")
	require.NoError(t, err)
	require.Empty(t, exhaustedAccounts)
	require.Len(t, reserveAccounts, 1)
	require.Equal(t, activeTeam.ID, reserveAccounts[0].ID)
}

func TestOpenAIGatewayService_IsCurrentOpenAIReserveOverlayAccount_ProjectionMissDoesNotUseLegacyOverlay(t *testing.T) {
	ctx := context.Background()
	groupID := int64(4226)
	exhaustedBase := Account{
		ID:          6311,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
		},
	}
	overlayReserve := newOpenAIReserveCandidateAccountForTest(6312, 1, 80)
	activeReserve := newOpenAIReserveCandidateAccountForTest(6313, 1, 20)
	activeAccount := Account{ID: 6314, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	snapshotCache := &openAISnapshotCacheStub{accountsByID: map[int64]*Account{6311: &exhaustedBase, 6312: &overlayReserve, 6313: &activeReserve, 6314: &activeAccount}, openAIStateMiss: true}
	svc := &OpenAIGatewayService{
		accountRepo:       stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}},
		cfg:               &config.Config{},
		schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache},
	}

	overlayFromSnapshot, err := svc.getSchedulableAccount(ctx, overlayReserve.ID)
	require.NoError(t, err)
	require.NotNil(t, overlayFromSnapshot)
	activeReserveFromSnapshot, err := svc.getSchedulableAccount(ctx, activeReserve.ID)
	require.NoError(t, err)
	require.NotNil(t, activeReserveFromSnapshot)

	require.False(t, svc.isCurrentOpenAIReserveOverlayAccount(ctx, &groupID, "gpt-5.1", overlayFromSnapshot))
	require.False(t, svc.isCurrentOpenAIReserveOverlayAccount(ctx, &groupID, "gpt-5.1", activeReserveFromSnapshot))
}

func TestOpenAIGatewayService_IsCurrentOpenAIReserveOverlayAccount_CacheNotReadyDoesNotUseLegacyOverlay(t *testing.T) {
	ctx := context.Background()
	groupID := int64(4227)
	exhaustedBase := Account{
		ID:          6321,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
		},
	}
	reserveAccount := newOpenAIReserveCandidateAccountForTest(6322, 1, 20)
	snapshotCache := &openAISnapshotCacheStub{
		accountsByID: map[int64]*Account{6321: &exhaustedBase, 6322: &reserveAccount},
	}
	svc := &OpenAIGatewayService{
		accountRepo:       stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, reserveAccount}},
		cfg:               &config.Config{},
		schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache},
	}

	reserveFromSnapshot, err := svc.getSchedulableAccount(ctx, reserveAccount.ID)
	require.NoError(t, err)
	require.NotNil(t, reserveFromSnapshot)
	require.False(t, svc.isCurrentOpenAIReserveOverlayAccount(ctx, &groupID, "gpt-5.1", reserveFromSnapshot))
}

func TestOpenAIGatewayService_ProjectionMissFailsClosedWithoutLiveReserveFallback(t *testing.T) {
	ctx := context.Background()
	groupID := int64(423)
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{
			&Account{
				ID:          6401,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}},
				Extra: map[string]any{
					"quota_limit": float64(100),
					"quota_used":  float64(100),
				},
			},
			&Account{
				ID:          6402,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Credentials: map[string]any{"plan_type": "free", "model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}},
				Extra: map[string]any{
					"codex_7d_used_percent": 20.0,
				},
			},
		},
		openAIStateMiss: true,
	}
	svc := &OpenAIGatewayService{
		cfg:               &config.Config{},
		schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache},
	}

	_, _, err := svc.listOpenAIExhaustedWithReserveOverlay(ctx, &groupID, "gpt-5.6")
	require.ErrorIs(t, err, ErrSchedulerCacheNotReady)
}

func TestOpenAIGatewayService_LoadOpenAIProjectionAccounts_UsesBundleWithoutGetAccountFallback(t *testing.T) {
	ctx := context.Background()
	state := newOpenAIBucketStateForTest([]Account{
		{
			ID:          6501,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}},
		},
		{
			ID:          6502,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{"plan_type": "free", "model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}},
			Extra:       map[string]any{"codex_7d_used_percent": 20.0},
		},
	}, 7, map[string]OpenAIModelRoleView{
		"gpt-5.6": {
			CanonicalModel:     "gpt-5.6",
			ExhaustedBaseIDs:   []int64{6501},
			ReserveOverflowIDs: []int64{6502},
		},
	})
	state.Accounts = []*Account{state.Accounts[0]}
	snapshotCache := &openAISnapshotCacheStub{
		accountsByID: map[int64]*Account{
			6501: state.ProjectionAccounts[0],
			6502: state.ProjectionAccounts[1],
		},
		openAIState: state,
	}
	svc := &OpenAIGatewayService{schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache}}

	exhaustedAccounts, reserveAccounts, err := svc.listOpenAIExhaustedWithReserveOverlay(ctx, nil, "gpt-5.6")
	require.NoError(t, err)
	require.Len(t, exhaustedAccounts, 1)
	require.Len(t, reserveAccounts, 1)
	require.Equal(t, int64(6501), exhaustedAccounts[0].ID)
	require.Equal(t, int64(6502), reserveAccounts[0].ID)
	require.Zero(t, snapshotCache.getAccountCalls)
}

func TestOpenAIGatewayService_LoadOpenAIProjectionAccounts_FailsClosedWhenBundleMissingProjectionParticipants(t *testing.T) {
	ctx := context.Background()
	state := newOpenAIBucketStateForTest([]Account{
		{
			ID:          6601,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}},
		},
		{
			ID:          6602,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{"plan_type": "free", "model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}},
			Extra:       map[string]any{"codex_7d_used_percent": 20.0},
		},
	}, 7, map[string]OpenAIModelRoleView{
		"gpt-5.6": {
			CanonicalModel:     "gpt-5.6",
			ExhaustedBaseIDs:   []int64{6601},
			ReserveOverflowIDs: []int64{6602},
		},
	})
	state.Accounts = []*Account{state.Accounts[0]}
	state.ProjectionAccounts = []*Account{state.ProjectionAccounts[0]}
	snapshotCache := &openAISnapshotCacheStub{
		accountsByID: map[int64]*Account{
			6601: state.Accounts[0],
			6602: {ID: 6602, Platform: PlatformOpenAI},
		},
		openAIState: state,
	}
	svc := &OpenAIGatewayService{schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache}}

	_, _, err := svc.listOpenAIExhaustedWithReserveOverlay(ctx, nil, "gpt-5.6")
	require.ErrorIs(t, err, ErrSchedulerCacheNotReady)
	require.Zero(t, snapshotCache.getAccountCalls)
}

func TestOpenAIGatewayService_GenerateSessionHash_AttachesLegacyHashToContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	c.Request.Header.Set("session_id", "sess-legacy-check")
	svc := &OpenAIGatewayService{}

	sessionHash := svc.GenerateSessionHash(c, nil)
	require.NotEmpty(t, sessionHash)
	require.NotNil(t, c.Request)
	require.NotNil(t, c.Request.Context())
	require.NotEmpty(t, openAILegacySessionHashFromContext(c.Request.Context()))
}

func TestOpenAIGatewayService_GenerateSessionHashWithFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	svc := &OpenAIGatewayService{}
	seed := "openai_ws_ingress:9:100:200"

	got := svc.GenerateSessionHashWithFallback(c, []byte(`{}`), seed)
	want := fmt.Sprintf("%016x", xxhash.Sum64String(seed))
	require.Equal(t, want, got)
	require.NotEmpty(t, openAILegacySessionHashFromContext(c.Request.Context()))

	empty := svc.GenerateSessionHashWithFallback(c, []byte(`{}`), "   ")
	require.Equal(t, "", empty)
}

func TestOpenAIGatewayService_GenerateSessionHash_ContentFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)

	svc := &OpenAIGatewayService{}

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"Hello"}]}`)

	hash := svc.GenerateSessionHash(c, body)
	require.NotEmpty(t, hash, "content-based fallback should produce a hash")

	hash2 := svc.GenerateSessionHash(c, body)
	require.Equal(t, hash, hash2, "same content should produce same hash")

	bodyExtended := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"Hello"},{"role":"assistant","content":"Hi!"},{"role":"user","content":"How are you?"}]}`)
	hashExtended := svc.GenerateSessionHash(c, bodyExtended)
	require.Equal(t, hash, hashExtended, "hash should be stable across later turns")

	bodyDifferent := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Different question"}]}`)
	hashDifferent := svc.GenerateSessionHash(c, bodyDifferent)
	require.NotEqual(t, hash, hashDifferent, "different content should produce different hash")
}

func TestOpenAIGatewayService_GenerateSessionHash_ExplicitSignalWinsOverContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)

	svc := &OpenAIGatewayService{}
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Hello"}]}`)

	contentHash := svc.GenerateSessionHash(c, body)
	require.NotEmpty(t, contentHash)

	c.Request.Header.Set("session_id", "explicit-session")
	explicitHash := svc.GenerateSessionHash(c, body)
	require.NotEmpty(t, explicitHash)
	require.NotEqual(t, contentHash, explicitHash, "explicit session_id should override content fallback")
}

func TestOpenAIGatewayService_GenerateSessionHash_EmptyBodyStillEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)

	svc := &OpenAIGatewayService{}
	require.Empty(t, svc.GenerateSessionHash(c, []byte(`{}`)))
	require.Empty(t, svc.GenerateSessionHash(c, nil))
}

func (c stubConcurrencyCache) GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error) {
	if c.waitCounts != nil {
		if count, ok := c.waitCounts[accountID]; ok {
			return count, nil
		}
	}
	return 0, nil
}

type stubGatewayCache struct {
	sessionBindings map[string]int64
	deletedSessions map[string]int
}

func (c *stubGatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	if id, ok := c.sessionBindings[sessionHash]; ok {
		return id, nil
	}
	return 0, errors.New("not found")
}

func (c *stubGatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	if c.sessionBindings == nil {
		c.sessionBindings = make(map[string]int64)
	}
	c.sessionBindings[sessionHash] = accountID
	return nil
}

func (c *stubGatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	return nil
}

func (c *stubGatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	if c.sessionBindings == nil {
		return nil
	}
	if c.deletedSessions == nil {
		c.deletedSessions = make(map[string]int)
	}
	c.deletedSessions[sessionHash]++
	delete(c.sessionBindings, sessionHash)
	return nil
}

func TestOpenAISelectAccountWithLoadAwareness_FiltersUnschedulable(t *testing.T) {
	now := time.Now()
	resetAt := now.Add(10 * time.Minute)
	groupID := int64(1)

	rateLimited := Account{
		ID:               1,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		Priority:         0,
		RateLimitResetAt: &resetAt,
	}
	available := Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
	}

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{rateLimited, available}},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-5.2", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection with account")
	}
	if selection.Account.ID != available.ID {
		t.Fatalf("expected account %d, got %d", available.ID, selection.Account.ID)
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestWriteChatCompletionsError_IncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-Id", "req-cc-789")
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)

	writeChatCompletionsError(c, http.StatusBadGateway, "server_error", "boom")

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "req-cc-789", errorObj["request_id"])
}

func TestOpenAISelectAccountWithLoadAwareness_FiltersUnschedulableWhenNoConcurrencyService(t *testing.T) {
	now := time.Now()
	resetAt := now.Add(10 * time.Minute)
	groupID := int64(1)

	rateLimited := Account{
		ID:               1,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		Priority:         0,
		RateLimitResetAt: &resetAt,
	}
	available := Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
	}

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{rateLimited, available}},
		// concurrencyService is nil, forcing the non-load-batch selection path.
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-5.2", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection with account")
	}
	if selection.Account.ID != available.ID {
		t.Fatalf("expected account %d, got %d", available.ID, selection.Account.ID)
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAISelectAccountForModelWithExclusions_StickyUnschedulableClearsSession(t *testing.T) {
	sessionHash := "session-1"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusDisabled, Schedulable: true, Concurrency: 1},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1},
	}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
	}
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2, got %+v", acc)
	}
	if cache.deletedSessions["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session to be deleted")
	}
	if cache.sessionBindings["openai:"+sessionHash] != 2 {
		t.Fatalf("expected sticky session to bind to account 2")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_StickyUnschedulableClearsSession(t *testing.T) {
	sessionHash := "session-2"
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusDisabled, Schedulable: true, Concurrency: 1},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2, got %+v", selection)
	}
	if cache.deletedSessions["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session to be deleted")
	}
	if cache.sessionBindings["openai:"+sessionHash] != 2 {
		t.Fatalf("expected sticky session to bind to account 2")
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAISelectAccountForModelWithExclusions_NoModelSupport(t *testing.T) {
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Credentials: map[string]any{"model_mapping": map[string]any{"gpt-3.5-turbo": "gpt-3.5-turbo"}},
			},
		},
	}
	cache := &stubGatewayCache{}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "gpt-4", nil)
	if err == nil {
		t.Fatalf("expected error for unsupported model")
	}
	if acc != nil {
		t.Fatalf("expected nil account for unsupported model")
	}
	if !strings.Contains(err.Error(), "supporting model") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAISelectAccountWithLoadAwareness_LoadBatchErrorFallback(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		loadBatchErr: errors.New("load batch failed"),
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "fallback", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection")
	}
	if selection.Account.ID != 2 {
		t.Fatalf("expected account 2, got %d", selection.Account.ID)
	}
	if cache.sessionBindings["openai:fallback"] != 2 {
		t.Fatalf("expected sticky session updated")
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAISelectAccountWithLoadAwareness_NoSlotFallbackWait(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		acquireResults: map[int64]bool{1: false},
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 10},
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected wait plan fallback")
	}
	if selection.Account == nil || selection.Account.ID != 1 {
		t.Fatalf("expected account 1")
	}
}

func TestOpenAISelectAccountForModelWithExclusions_SetsStickyBinding(t *testing.T) {
	sessionHash := "bind"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
	}
	if acc == nil || acc.ID != 1 {
		t.Fatalf("expected account 1")
	}
	if cache.sessionBindings["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session binding")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_StickyWaitPlan(t *testing.T) {
	sessionHash := "sticky-wait"
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1},
	}
	concurrencyCache := stubConcurrencyCache{
		acquireResults: map[int64]bool{1: false},
		waitCounts:     map[int64]int{1: 0},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected sticky wait plan")
	}
	if selection.Account == nil || selection.Account.ID != 1 {
		t.Fatalf("expected account 1")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_PrefersLowerLoad(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 80},
			2: {AccountID: 2, LoadRate: 10},
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "load", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2")
	}
	if cache.sessionBindings["openai:load"] != 2 {
		t.Fatalf("expected sticky session updated")
	}
}

func TestOpenAISelectAccountForModelWithExclusions_StickyExcludedFallback(t *testing.T) {
	sessionHash := "excluded"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1},
	}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	excluded := map[int64]struct{}{1: {}}
	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", excluded)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
	}
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2")
	}
}

func TestOpenAISelectAccountForModelWithExclusions_StickyNonOpenAI(t *testing.T) {
	sessionHash := "non-openai"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1},
	}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
	}
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2")
	}
}

func TestOpenAISelectAccountForModelWithExclusions_NoAccounts(t *testing.T) {
	repo := stubOpenAIAccountRepo{accounts: []Account{}}
	cache := &stubGatewayCache{}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "", nil)
	if err == nil {
		t.Fatalf("expected error for no accounts")
	}
	if acc != nil {
		t.Fatalf("expected nil account")
	}
	if !errors.Is(err, ErrNoAvailableAccounts) {
		t.Fatalf("expected ErrNoAvailableAccounts, got: %v", err)
	}
	if !strings.Contains(err.Error(), ErrNoAvailableAccounts.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAISelectAccountWithLoadAwareness_NoCandidates(t *testing.T) {
	groupID := int64(1)
	resetAt := time.Now().Add(1 * time.Hour)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, RateLimitResetAt: &resetAt},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err == nil {
		t.Fatalf("expected error for no candidates")
	}
	if selection != nil {
		t.Fatalf("expected nil selection")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_AllFullWaitPlan(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 100},
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected wait plan")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_LoadBatchErrorNoAcquire(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		loadBatchErr:   errors.New("load batch failed"),
		acquireResults: map[int64]bool{1: false},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected wait plan")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_MissingLoadInfo(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 50},
		},
		skipDefaultLoad: true,
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2")
	}
}

func TestOpenAISelectAccountForModelWithExclusions_LeastRecentlyUsed(t *testing.T) {
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Priority: 1, LastUsedAt: &newTime},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Priority: 1, LastUsedAt: &oldTime},
		},
	}
	cache := &stubGatewayCache{}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
	}
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_PreferNeverUsed(t *testing.T) {
	groupID := int64(1)
	lastUsed := time.Now().Add(-1 * time.Hour)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, LastUsedAt: &lastUsed},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 10},
			2: {AccountID: 2, LoadRate: 10},
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2")
	}
}

func TestOpenAIStreamingTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 1,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	start := time.Now()
	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, start, "model", "model")
	_ = pw.Close()
	_ = pr.Close()

	if err == nil || !strings.Contains(err.Error(), "stream data interval timeout") {
		t.Fatalf("expected stream timeout error, got %v", err)
	}
	if !strings.Contains(rec.Body.String(), "\"type\":\"error\"") || !strings.Contains(rec.Body.String(), "stream_timeout") {
		t.Fatalf("expected OpenAI-compatible error SSE event, got %q", rec.Body.String())
	}
}

func TestOpenAIStreamingContextCanceledReturnsIncompleteErrorWithoutInjectingErrorEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       cancelReadCloser{},
		Header:     http.Header{},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "model", "model")
	if err == nil || !strings.Contains(err.Error(), "stream usage incomplete") {
		t.Fatalf("expected incomplete stream error, got %v", err)
	}
	if strings.Contains(rec.Body.String(), "event: error") || strings.Contains(rec.Body.String(), "stream_read_error") {
		t.Fatalf("expected no injected SSE error event, got %q", rec.Body.String())
	}
}

func TestOpenAIStreamingReadErrorBeforeOutputReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       errReadCloser{err: io.ErrUnexpectedEOF},
		Header:     http.Header{"X-Request-Id": []string{"rid-disconnect"}},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestOpenAIStreamingResponseFailedBeforeOutputReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			"event: response.in_progress",
			`data: {"type":"response.in_progress","response":{"id":"resp_1"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"message":"An error occurred while processing your request."}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-failed"}},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "An error occurred while processing your request")
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestOpenAIStreamingPreambleOnlyMissingTerminalReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			"event: response.in_progress",
			`data: {"type":"response.in_progress","response":{"id":"resp_1"}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-missing-terminal"}},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestOpenAIStreamingPreambleKeepaliveUsesDownstreamIdle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   1,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n"))
		for i := 0; i < 6; i++ {
			time.Sleep(250 * time.Millisecond)
			_, _ = pw.Write([]byte("data: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_1\"}}\n\n"))
		}
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n"))
	}()

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), ":\n\n")
	require.Contains(t, rec.Body.String(), "response.completed")
}

func TestOpenAIStreamingPolicyResponseFailedBeforeOutputPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"type":"safety_error","message":"This request has been flagged for potentially high-risk cyber activity."}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-policy-failed"}},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.True(t, c.Writer.Written())
	require.Contains(t, rec.Body.String(), "response.failed")
	require.Contains(t, rec.Body.String(), "high-risk cyber activity")
}

func TestOpenAIStreamingClientDisconnectDrainsUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Writer = &failingGinWriter{ResponseWriter: c.Writer, failAfter: 0}

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.in_progress\",\"response\":{}}\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n"))
	}()

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "model", "model")
	_ = pr.Close()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result == nil || result.usage == nil {
		t.Fatalf("expected usage result")
	}
	if result.usage.InputTokens != 3 || result.usage.OutputTokens != 5 || result.usage.CacheReadInputTokens != 1 {
		t.Fatalf("unexpected usage: %+v", *result.usage)
	}
	if strings.Contains(rec.Body.String(), "event: error") || strings.Contains(rec.Body.String(), "write_failed") {
		t.Fatalf("expected no injected SSE error event, got %q", rec.Body.String())
	}
}

func TestOpenAIStreamingReadErrorBeforeFirstTokenReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       resetReadCloser{err: errors.New("stream error: stream ID 1; INTERNAL_ERROR; received from peer")},
		Header:     http.Header{},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "gpt-5.4", "gpt-5.4")
	var failoverErr *UpstreamFailoverError
	if !errors.As(err, &failoverErr) {
		t.Fatalf("expected UpstreamFailoverError, got %v", err)
	}
	if strings.Contains(rec.Body.String(), "stream_read_error") {
		t.Fatalf("expected no injected stream error event, got %q", rec.Body.String())
	}
}

func TestOpenAIStreamingTransientErrorEventBeforeFirstChunkReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 0, StreamKeepaliveInterval: 0, MaxLineSize: defaultMaxLineSize}}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{"x-request-id": []string{"rid-prestream"}},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"error\",\"sequence_number\":2,\"error\":{\"type\":\"server_error\",\"code\":\"server_error\",\"message\":\"An error occurred while processing your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists. Please include the request ID test-prestream in your message.\"}}\n\n"))
	}()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "gpt-5.4", "gpt-5.4")
	_ = pr.Close()
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	require.Empty(t, rec.Body.String())
	require.Empty(t, rec.Header().Get("Content-Type"))
}

func TestOpenAIStreamingCreatedThenTransientErrorStillReturnsFailoverBeforeFirstChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 0, StreamKeepaliveInterval: 0, MaxLineSize: defaultMaxLineSize}}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{"x-request-id": []string{"rid-created-then-error"}}}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.created\",\"sequence_number\":1,\"response\":{\"id\":\"resp_1\"}}\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"error\",\"sequence_number\":2,\"error\":{\"type\":\"server_error\",\"code\":\"server_error\",\"message\":\"An error occurred while processing your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists. Please include the request ID created-then-error in your message.\"}}\n\n"))
	}()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "gpt-5.4", "gpt-5.4")
	_ = pr.Close()
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, rec.Body.String())
	require.Empty(t, rec.Header().Get("Content-Type"))
}

func TestOpenAIStreamingCreatedThenOverloadedErrorStillReturnsFailoverBeforeFirstChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 0, StreamKeepaliveInterval: 0, MaxLineSize: defaultMaxLineSize}}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{"x-request-id": []string{"rid-created-then-overloaded"}}}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.created\",\"sequence_number\":1,\"response\":{\"id\":\"resp_1\"}}\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"error\",\"sequence_number\":2,\"error\":{\"type\":\"service_unavailable_error\",\"code\":\"server_is_overloaded\",\"message\":\"Our servers are currently overloaded. Please try again later.\",\"param\":null}}\n\n"))
	}()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "gpt-5.4", "gpt-5.4")
	_ = pr.Close()
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, rec.Body.String())
	require.Empty(t, rec.Header().Get("Content-Type"))
}

func TestOpenAIStreamingMissingTerminalEventReturnsIncompleteError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"message\"},\"output_index\":0}\n\n"))
	}()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "model", "model")
	_ = pr.Close()
	if err == nil || !strings.Contains(err.Error(), "missing terminal event") {
		t.Fatalf("expected missing terminal event error, got %v", err)
	}
}

func TestOpenAIStreamingPassthroughMissingTerminalEventReturnsIncompleteError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"message\"},\"output_index\":0}\n\n"))
	}()

	_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "", "")
	_ = pr.Close()
	if err == nil || !strings.Contains(err.Error(), "missing terminal event") {
		t.Fatalf("expected missing terminal event error, got %v", err)
	}
}

func TestOpenAIStreamingPassthroughTransientErrorEventBeforeFirstChunkReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{"x-request-id": []string{"rid-pass-prestream"}},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"error\",\"sequence_number\":2,\"error\":{\"type\":\"server_error\",\"code\":\"server_error\",\"message\":\"An error occurred while processing your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists. Please include the request ID test-pass-prestream in your message.\"}}\n\n"))
	}()

	_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "", "")
	_ = pr.Close()
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	require.Empty(t, rec.Body.String())
	require.Empty(t, rec.Header().Get("Content-Type"))
}

func TestOpenAIStreamingPassthroughCreatedThenTransientErrorStillReturnsFailoverBeforeFirstChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{"x-request-id": []string{"rid-pass-created-then-error"}}}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.created\",\"sequence_number\":1,\"response\":{\"id\":\"resp_1\"}}\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"error\",\"sequence_number\":2,\"error\":{\"type\":\"server_error\",\"code\":\"server_error\",\"message\":\"An error occurred while processing your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists. Please include the request ID pass-created-then-error in your message.\"}}\n\n"))
	}()

	_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "", "")
	_ = pr.Close()
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, rec.Body.String())
	require.Empty(t, rec.Header().Get("Content-Type"))
}

func TestOpenAIStreamingPassthroughResponseFailedBeforeOutputReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"message":"upstream processing failed"}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-passthrough-failed"}},
	}

	_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "", "")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "upstream processing failed")
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestOpenAIStreamingPassthroughResponseDoneWithoutDoneMarkerStillSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.done\",\"response\":{\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n"))
	}()

	result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "", "")
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 2, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	require.Equal(t, 1, result.usage.CacheReadInputTokens)
}

func TestOpenAIStreamingPassthroughResponseIncompleteWithoutDoneMarkerStillSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.incomplete\",\"response\":{\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n"))
	}()

	result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "", "")
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 2, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	require.Equal(t, 1, result.usage.CacheReadInputTokens)
}

func TestOpenAIStreamingTooLong(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               64 * 1024,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		// 写入超过 MaxLineSize 的单行数据，触发 ErrTooLong
		payload := "data: " + strings.Repeat("a", 128*1024) + "\n"
		_, _ = pw.Write([]byte(payload))
	}()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 2}, time.Now(), "model", "model")
	_ = pr.Close()

	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("expected ErrTooLong, got %v", err)
	}
	if !strings.Contains(rec.Body.String(), "\"type\":\"error\"") || !strings.Contains(rec.Body.String(), "response_too_large") {
		t.Fatalf("expected OpenAI-compatible error SSE event, got %q", rec.Body.String())
	}
}

func TestOpenAINonStreamingContentTypePassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: false},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body := []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/vnd.test+json"}},
	}

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{}, "model", "model")
	if err != nil {
		t.Fatalf("handleNonStreamingResponse error: %v", err)
	}

	if !strings.Contains(rec.Header().Get("Content-Type"), "application/vnd.test+json") {
		t.Fatalf("expected Content-Type passthrough, got %q", rec.Header().Get("Content-Type"))
	}
}

func TestOpenAINonStreamingContentTypeDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: false},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body := []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{},
	}

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{}, "model", "model")
	if err != nil {
		t.Fatalf("handleNonStreamingResponse error: %v", err)
	}

	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected default Content-Type, got %q", rec.Header().Get("Content-Type"))
	}
}

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

func TestOpenAIStreamingHeadersOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: false},
		},
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header: http.Header{
			"Cache-Control": []string{"upstream"},
			"X-Request-Id":  []string{"req-123"},
			"Content-Type":  []string{"application/custom"},
		},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{}}\n\n"))
	}()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "model", "model")
	_ = pr.Close()
	if err != nil {
		t.Fatalf("handleStreamingResponse error: %v", err)
	}

	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("expected Cache-Control override, got %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected Content-Type override, got %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("X-Request-Id") != "req-123" {
		t.Fatalf("expected X-Request-Id passthrough, got %q", rec.Header().Get("X-Request-Id"))
	}
}

func TestOpenAIStreamingReuseScannerBufferAndStillWorks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"input_tokens_details\":{\"cached_tokens\":3}}}}\n\n"))
	}()

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "model", "model")
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 1, result.usage.InputTokens)
	require.Equal(t, 2, result.usage.OutputTokens)
	require.Equal(t, 3, result.usage.CacheReadInputTokens)
}

func TestOpenAIInvalidBaseURLWhenAllowlistDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "://invalid-url"},
	}

	_, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte("{}"), "token", false, "", false)
	if err == nil {
		t.Fatalf("expected error for invalid base_url when allowlist disabled")
	}
}

func TestOpenAIValidateUpstreamBaseURLDisabledRequiresHTTPS(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	if _, err := svc.validateUpstreamBaseURL("http://not-https.example.com"); err == nil {
		t.Fatalf("expected http to be rejected when allow_insecure_http is false")
	}
	normalized, err := svc.validateUpstreamBaseURL("https://example.com")
	if err != nil {
		t.Fatalf("expected https to be allowed when allowlist disabled, got %v", err)
	}
	if normalized != "https://example.com" {
		t.Fatalf("expected raw url passthrough, got %q", normalized)
	}
}

func TestOpenAIValidateUpstreamBaseURLDisabledAllowsHTTP(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
			},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	normalized, err := svc.validateUpstreamBaseURL("http://not-https.example.com")
	if err != nil {
		t.Fatalf("expected http allowed when allow_insecure_http is true, got %v", err)
	}
	if normalized != "http://not-https.example.com" {
		t.Fatalf("expected raw url passthrough, got %q", normalized)
	}
}

func TestOpenAIValidateUpstreamBaseURLEnabledEnforcesAllowlist(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:       true,
				UpstreamHosts: []string{"example.com"},
			},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	if _, err := svc.validateUpstreamBaseURL("https://example.com"); err != nil {
		t.Fatalf("expected allowlisted host to pass, got %v", err)
	}
	if _, err := svc.validateUpstreamBaseURL("https://evil.com"); err == nil {
		t.Fatalf("expected non-allowlisted host to fail")
	}
}

func TestOpenAIUpdateCodexUsageSnapshotFromHeaders(t *testing.T) {
	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 1)}
	svc := &OpenAIGatewayService{accountRepo: repo}
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "12")
	headers.Set("x-codex-secondary-used-percent", "34")
	headers.Set("x-codex-primary-window-minutes", "300")
	headers.Set("x-codex-secondary-window-minutes", "10080")
	headers.Set("x-codex-primary-reset-after-seconds", "600")
	headers.Set("x-codex-secondary-reset-after-seconds", "86400")

	svc.UpdateCodexUsageSnapshotFromHeaders(context.Background(), 123, headers)

	select {
	case updates := <-repo.updateExtraCalls:
		require.Equal(t, 12.0, updates["codex_5h_used_percent"])
		require.Equal(t, 34.0, updates["codex_7d_used_percent"])
		require.Equal(t, 600, updates["codex_5h_reset_after_seconds"])
		require.Equal(t, 86400, updates["codex_7d_reset_after_seconds"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected UpdateExtra to be called")
	}
}

func TestOpenAIResponsesRequestPathSuffix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "exact v1 responses", path: "/v1/responses", want: ""},
		{name: "compact v1 responses", path: "/v1/responses/compact", want: "/compact"},
		{name: "compact alias responses", path: "/responses/compact/", want: "/compact"},
		{name: "nested suffix", path: "/openai/v1/responses/compact/detail", want: "/compact/detail"},
		{name: "unrelated path", path: "/v1/chat/completions", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)
			require.Equal(t, tt.want, openAIResponsesRequestPathSuffix(c))
		})
	}
}

func TestOpenAIBuildUpstreamRequestOpenAIPassthroughPreservesCompactPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))

	svc := &OpenAIGatewayService{}
	account := &Account{Type: AccountTypeOAuth}

	req, err := svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token")
	require.NoError(t, err)
	require.Equal(t, chatgptCodexURL+"/compact", req.URL.String())
	require.Equal(t, "application/json", req.Header.Get("Accept"))
	require.Equal(t, codexCLIVersion, req.Header.Get("Version"))
	require.NotEmpty(t, req.Header.Get("Session_Id"))
}

func TestOpenAIBuildUpstreamRequestCompactForcesJSONAcceptForOAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))

	svc := &OpenAIGatewayService{}
	account := &Account{
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-acc"},
	}

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token", false, "", true)
	require.NoError(t, err)
	require.Equal(t, chatgptCodexURL+"/compact", req.URL.String())
	require.Equal(t, "application/json", req.Header.Get("Accept"))
	require.Equal(t, codexCLIVersion, req.Header.Get("Version"))
	require.NotEmpty(t, req.Header.Get("Session_Id"))
}

func TestOpenAIBuildUpstreamRequestOAuthNonStreamSourcesIncludeForcesJSONAccept(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","include":["reasoning.encrypted_content","web_search_call.action.sources"]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	svc := &OpenAIGatewayService{}
	account := &Account{
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-acc"},
	}

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, body, "token", false, "", false)
	require.NoError(t, err)
	require.Equal(t, "application/json", req.Header.Get("Accept"))
}

func TestOpenAIBuildUpstreamRequestOAuthNonStreamWithoutSourcesIncludeKeepsSSEAccept(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","include":["reasoning.encrypted_content"]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	svc := &OpenAIGatewayService{}
	account := &Account{
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-acc"},
	}

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, body, "token", false, "", false)
	require.NoError(t, err)
	require.Equal(t, "text/event-stream", req.Header.Get("Accept"))
}

func TestOpenAIBuildUpstreamRequestPreservesCompactPathForAPIKeyBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))

	svc := &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}}
	account := &Account{
		Type:        AccountTypeAPIKey,
		Platform:    PlatformOpenAI,
		Credentials: map[string]any{"base_url": "https://example.com/v1"},
	}

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token", false, "", false)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/v1/responses/compact", req.URL.String())
}

func TestOpenAIBuildUpstreamRequestOAuthOfficialClientOriginatorCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userAgent      string
		originator     string
		wantOriginator string
	}{
		{name: "desktop originator preserved", originator: "Codex Desktop", wantOriginator: "Codex Desktop"},
		{name: "vscode originator preserved", originator: "codex_vscode", wantOriginator: "codex_vscode"},
		{name: "official ua fallback to codex_cli_rs", userAgent: "Codex Desktop/1.2.3", wantOriginator: "codex_cli_rs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))
			if tt.userAgent != "" {
				c.Request.Header.Set("User-Agent", tt.userAgent)
			}
			if tt.originator != "" {
				c.Request.Header.Set("originator", tt.originator)
			}

			svc := &OpenAIGatewayService{}
			account := &Account{
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"chatgpt_account_id": "chatgpt-acc"},
			}

			isCodexCLI := openai.IsCodexOfficialClientByHeaders(c.GetHeader("User-Agent"), c.GetHeader("originator"))
			req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token", false, "", isCodexCLI)
			require.NoError(t, err)
			require.Equal(t, tt.wantOriginator, req.Header.Get("originator"))
		})
	}
}

func TestOpenAIGatewayService_BuildUpstreamRequest_UsesXSessionAffinityOverPromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5","prompt_cache_key":"body-cache"}`)))
	c.Request.Header.Set("x-session-affinity", "affinity-upstream")

	svc := &OpenAIGatewayService{}
	account := &Account{
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-acc"},
	}

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5","prompt_cache_key":"body-cache"}`), "token", true, "body-cache", false)
	require.NoError(t, err)

	expected := isolateOpenAISessionID(0, "affinity-upstream")
	require.Equal(t, expected, req.Header.Get("Session_Id"))
	require.Equal(t, expected, req.Header.Get("Conversation_Id"))
}

func TestOpenAIGatewayService_BuildUpstreamRequest_UsesXSessionAffinityOverPromptCacheKey_Passthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5","prompt_cache_key":"body-cache"}`)))
	c.Request.Header.Set("x-session-affinity", "affinity-passthrough")

	svc := &OpenAIGatewayService{}
	account := &Account{
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-acc"},
	}

	req, err := svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, []byte(`{"model":"gpt-5","prompt_cache_key":"body-cache"}`), "token")
	require.NoError(t, err)

	expected := isolateOpenAISessionID(0, "affinity-passthrough")
	require.Equal(t, expected, req.Header.Get("Session_Id"))
	require.Equal(t, expected, req.Header.Get("Conversation_Id"))
}

func TestNormalizeOpenAIPassthroughOAuthBody_PromotesSystemMessagesToInstructions(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":[{"role":"system","content":"You are a coding assistant."},{"role":"user","content":"Write a function."}]}`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "You are a coding assistant.", gjson.GetBytes(normalized, "instructions").String())
	require.Len(t, gjson.GetBytes(normalized, "input").Array(), 1)
	require.Equal(t, "user", gjson.GetBytes(normalized, "input.0.role").String())
}

func TestBuildOpenAIUpstreamErrorEnvelope(t *testing.T) {
	body := []byte(`{"error":{"code":"invalid_value","message":"The image data you provided does not represent a valid image.","param":"input","type":"invalid_request_error"}}`)

	status, envelope := buildOpenAIUpstreamErrorEnvelope(http.StatusBadRequest, body, "Upstream request failed")
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "upstream_error", envelope.Error.Type)
	require.Equal(t, "The image data you provided does not represent a valid image.", envelope.Error.Message)
	require.NotNil(t, envelope.Error.Upstream)
	require.Equal(t, http.StatusBadRequest, envelope.Error.Upstream["status"])
	require.Equal(t, "invalid_value", envelope.Error.Upstream["code"])
	require.Equal(t, "invalid_request_error", envelope.Error.Upstream["type"])
	require.Equal(t, "input", envelope.Error.Upstream["param"])
	require.Equal(t, "The image data you provided does not represent a valid image.", envelope.Error.Upstream["message"])

	raw, ok := envelope.Error.Upstream["raw"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, raw, "error")
}

// ==================== P1-08 修复：model 替换性能优化测试 ====================

// ==================== P1-08 修复：model 替换性能优化测试 =============
func TestReplaceModelInSSELine(t *testing.T) {
	svc := &OpenAIGatewayService{}

	tests := []struct {
		name     string
		line     string
		from     string
		to       string
		expected string
	}{
		{
			name:     "顶层 model 字段替换",
			line:     `data: {"id":"chatcmpl-123","model":"gpt-4o","choices":[]}`,
			from:     "gpt-4o",
			to:       "my-custom-model",
			expected: `data: {"id":"chatcmpl-123","model":"my-custom-model","choices":[]}`,
		},
		{
			name:     "嵌套 response.model 替换",
			line:     `data: {"type":"response","response":{"id":"resp-1","model":"gpt-4o","output":[]}}`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {"type":"response","response":{"id":"resp-1","model":"my-model","output":[]}}`,
		},
		{
			name:     "model 不匹配时不替换",
			line:     `data: {"id":"chatcmpl-123","model":"gpt-3.5-turbo","choices":[]}`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {"id":"chatcmpl-123","model":"gpt-3.5-turbo","choices":[]}`,
		},
		{
			name:     "无 model 字段时不替换",
			line:     `data: {"id":"chatcmpl-123","choices":[]}`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {"id":"chatcmpl-123","choices":[]}`,
		},
		{
			name:     "空 data 行",
			line:     `data: `,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: `,
		},
		{
			name:     "[DONE] 行",
			line:     `data: [DONE]`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: [DONE]`,
		},
		{
			name:     "非 data: 前缀行",
			line:     `event: message`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `event: message`,
		},
		{
			name:     "非法 JSON 不替换",
			line:     `data: {invalid json}`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {invalid json}`,
		},
		{
			name:     "无空格 data: 格式",
			line:     `data:{"id":"x","model":"gpt-4o"}`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {"id":"x","model":"my-model"}`,
		},
		{
			name:     "model 名含特殊字符",
			line:     `data: {"model":"org/model-v2.1-beta"}`,
			from:     "org/model-v2.1-beta",
			to:       "custom/alias",
			expected: `data: {"model":"custom/alias"}`,
		},
		{
			name:     "空行",
			line:     "",
			from:     "gpt-4o",
			to:       "my-model",
			expected: "",
		},
		{
			name:     "保持其他字段不变",
			line:     `data: {"id":"abc","object":"chat.completion.chunk","model":"gpt-4o","created":1234567890,"choices":[{"index":0,"delta":{"content":"hi"}}]}`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `data: {"id":"abc","object":"chat.completion.chunk","model":"alias","created":1234567890,"choices":[{"index":0,"delta":{"content":"hi"}}]}`,
		},
		{
			name:     "顶层优先于嵌套：同时存在两个 model",
			line:     `data: {"model":"gpt-4o","response":{"model":"gpt-4o"}}`,
			from:     "gpt-4o",
			to:       "replaced",
			expected: `data: {"model":"replaced","response":{"model":"gpt-4o"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.replaceModelInSSELine(tt.line, tt.from, tt.to)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestReplaceModelInSSEBody(t *testing.T) {
	svc := &OpenAIGatewayService{}

	tests := []struct {
		name     string
		body     string
		from     string
		to       string
		expected string
	}{
		{
			name:     "多行 SSE body 替换",
			body:     "data: {\"model\":\"gpt-4o\",\"choices\":[]}\n\ndata: {\"model\":\"gpt-4o\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n",
			from:     "gpt-4o",
			to:       "alias",
			expected: "data: {\"model\":\"alias\",\"choices\":[]}\n\ndata: {\"model\":\"alias\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n",
		},
		{
			name:     "无需替换的 body",
			body:     "data: {\"model\":\"gpt-3.5-turbo\"}\n\ndata: [DONE]\n",
			from:     "gpt-4o",
			to:       "alias",
			expected: "data: {\"model\":\"gpt-3.5-turbo\"}\n\ndata: [DONE]\n",
		},
		{
			name:     "混合 event 和 data 行",
			body:     "event: message\ndata: {\"model\":\"gpt-4o\"}\n\n",
			from:     "gpt-4o",
			to:       "alias",
			expected: "event: message\ndata: {\"model\":\"alias\"}\n\n",
		},
		{
			name:     "空 body",
			body:     "",
			from:     "gpt-4o",
			to:       "alias",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.replaceModelInSSEBody(tt.body, tt.from, tt.to)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestReplaceModelInResponseBody(t *testing.T) {
	svc := &OpenAIGatewayService{}

	tests := []struct {
		name     string
		body     string
		from     string
		to       string
		expected string
	}{
		{
			name:     "替换顶层 model",
			body:     `{"id":"chatcmpl-123","model":"gpt-4o","choices":[]}`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `{"id":"chatcmpl-123","model":"alias","choices":[]}`,
		},
		{
			name:     "model 不匹配不替换",
			body:     `{"id":"chatcmpl-123","model":"gpt-3.5-turbo","choices":[]}`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `{"id":"chatcmpl-123","model":"gpt-3.5-turbo","choices":[]}`,
		},
		{
			name:     "无 model 字段不替换",
			body:     `{"id":"chatcmpl-123","choices":[]}`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `{"id":"chatcmpl-123","choices":[]}`,
		},
		{
			name:     "非法 JSON 返回原值",
			body:     `not json`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `not json`,
		},
		{
			name:     "空 body 返回原值",
			body:     ``,
			from:     "gpt-4o",
			to:       "alias",
			expected: ``,
		},
		{
			name:     "保持嵌套结构不变",
			body:     `{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":20},"choices":[{"message":{"role":"assistant","content":"hello"}}]}`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `{"model":"alias","usage":{"prompt_tokens":10,"completion_tokens":20},"choices":[{"message":{"role":"assistant","content":"hello"}}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.replaceModelInResponseBody([]byte(tt.body), tt.from, tt.to)
			require.Equal(t, tt.expected, string(got))
		})
	}
}

func TestExtractOpenAISSEDataLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantData string
		wantOK   bool
	}{
		{name: "标准格式", line: `data: {"type":"x"}`, wantData: `{"type":"x"}`, wantOK: true},
		{name: "无空格格式", line: `data:{"type":"x"}`, wantData: `{"type":"x"}`, wantOK: true},
		{name: "纯空数据", line: `data:   `, wantData: ``, wantOK: true},
		{name: "非 data 行", line: `event: message`, wantData: ``, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractOpenAISSEDataLine(tt.line)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantData, got)
		})
	}
}

func TestParseSSEUsage_SelectiveParsing(t *testing.T) {
	svc := &OpenAIGatewayService{}
	usage := &OpenAIUsage{InputTokens: 9, OutputTokens: 8, CacheReadInputTokens: 7}

	// 非 completed 事件，不应覆盖 usage
	svc.parseSSEUsage(`{"type":"response.in_progress","response":{"usage":{"input_tokens":1,"output_tokens":2}}}`, usage)
	require.Equal(t, 9, usage.InputTokens)
	require.Equal(t, 8, usage.OutputTokens)
	require.Equal(t, 7, usage.CacheReadInputTokens)

	// completed 事件，应提取 usage
	svc.parseSSEUsage(`{"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":5,"input_tokens_details":{"cached_tokens":2}}}}`, usage)
	require.Equal(t, 3, usage.InputTokens)
	require.Equal(t, 5, usage.OutputTokens)
	require.Equal(t, 2, usage.CacheReadInputTokens)

	// done 事件同样可能携带最终 usage
	svc.parseSSEUsage(`{"type":"response.done","response":{"usage":{"input_tokens":13,"output_tokens":15,"input_tokens_details":{"cached_tokens":4}}}}`, usage)
	require.Equal(t, 13, usage.InputTokens)
	require.Equal(t, 15, usage.OutputTokens)
	require.Equal(t, 4, usage.CacheReadInputTokens)
}

func TestExtractCodexFinalResponse_SampleReplay(t *testing.T) {
	body := strings.Join([]string{
		`event: message`,
		`data: {"type":"response.in_progress","response":{"id":"resp_1"}}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","usage":{"input_tokens":11,"output_tokens":22,"input_tokens_details":{"cached_tokens":3}}}}`,
		`data: [DONE]`,
	}, "\n")

	finalResp, ok := extractCodexFinalResponse(body)
	require.True(t, ok)
	require.Contains(t, string(finalResp), `"id":"resp_1"`)
	require.Contains(t, string(finalResp), `"input_tokens":11`)
}

func TestHandleSSEToJSON_CompletedEventReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	body := []byte(strings.Join([]string{
		`data: {"type":"response.in_progress","response":{"id":"resp_2"}}`,
		`data: {"type":"response.completed","response":{"id":"resp_2","model":"gpt-4o","usage":{"input_tokens":7,"output_tokens":9,"input_tokens_details":{"cached_tokens":1}}}}`,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-4o", "gpt-4o")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 7, usage.InputTokens)
	require.Equal(t, 9, usage.OutputTokens)
	require.Equal(t, 1, usage.CacheReadInputTokens)
	// Header 可能由上游 Content-Type 透传；关键是 body 已转换为最终 JSON 响应。
	require.NotContains(t, rec.Body.String(), "event:")
	require.Contains(t, rec.Body.String(), `"id":"resp_2"`)
	require.NotContains(t, rec.Body.String(), "data:")
}

func TestHandleSSEToJSON_OAuthNonCompactSupplementsWebSearchCallAndToolUsage(t *testing.T) {
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
			`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"openai pricing","sources":[{"type":"url_citation","title":"Reuters","url":"https://www.reuters.com/example"}]}}}`,
			``,
			`event: response.reasoning_summary_text.delta`,
			`data: {"type":"response.reasoning_summary_text.delta","output_index":1,"summary_index":0,"delta":"search strategy"}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","output_index":2,"content_index":0,"delta":"pricing result"}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","response":{"id":"resp_oauth_1","model":"gpt-5.4","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]}],"tool_usage":{"web_search":{"num_requests":0,"debug":"keep-me"},"other_tool":{"count":7}},"usage":{"input_tokens":1,"output_tokens":2}}}`,
			``,
			`data: [DONE]`,
		}, "\n"))),
	}

	usage, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 1, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)

	bodyText := rec.Body.String()
	require.True(t, gjson.Get(bodyText, `output.#(type=="web_search_call")`).Exists())
	require.Equal(t, "Reuters", gjson.Get(bodyText, `output.#(type=="web_search_call").action.sources.0.title`).String())
	require.Equal(t, "https://www.reuters.com/example", gjson.Get(bodyText, `output.#(type=="web_search_call").action.sources.0.url`).String())
	require.Equal(t, int64(1), gjson.Get(bodyText, `tool_usage.web_search.num_requests`).Int())
	require.Equal(t, "keep-me", gjson.Get(bodyText, `tool_usage.web_search.debug`).String())
	require.Equal(t, int64(7), gjson.Get(bodyText, `tool_usage.other_tool.count`).Int())
	require.True(t, strings.HasPrefix(gjson.Get(bodyText, `output.#(type=="message").id`).String(), "msg_"))
	require.Equal(t, "[]", gjson.Get(bodyText, `output.#(type=="message").content.0.annotations`).Raw)
}

func TestHandleSSEToJSON_OAuthNonCompactSupplementsWebSearchCallSourcesWhenDoneIsWeaker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: response.output_item.added`,
			`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"web_search_call","id":"ws_weak_done","status":"in_progress","action":{"type":"search","query":"openai pricing","sources":[{"type":"url_citation","title":"Reuters","url":"https://www.reuters.com/example"}]}}}`,
			``,
			`event: response.output_item.done`,
			`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_weak_done","status":"completed","action":{"type":"search","query":"openai pricing"}}}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"pricing result"}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","response":{"id":"resp_oauth_sources_weak_done","model":"gpt-5.4","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]}],"tool_usage":{"web_search":{"num_requests":0},"other_tool":{"count":7}},"usage":{"input_tokens":1,"output_tokens":2}}}`,
			``,
			`data: [DONE]`,
		}, "\n"))),
	}

	usage, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 1, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)

	bodyText := rec.Body.String()
	require.True(t, gjson.Get(bodyText, `output.#(type=="web_search_call")`).Exists())
	require.Equal(t, "Reuters", gjson.Get(bodyText, `output.#(type=="web_search_call").action.sources.0.title`).String())
	require.Equal(t, "https://www.reuters.com/example", gjson.Get(bodyText, `output.#(type=="web_search_call").action.sources.0.url`).String())
	require.Equal(t, int64(1), gjson.Get(bodyText, `tool_usage.web_search.num_requests`).Int())
	require.Equal(t, int64(7), gjson.Get(bodyText, `tool_usage.other_tool.count`).Int())
}

func TestHandleSSEToJSON_OAuthNonCompactStrengthensExistingWebSearchCallWithoutDuplicate(t *testing.T) {
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
			`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_same","status":"completed","action":{"type":"search","query":"openai pricing","sources":[{"type":"url_citation","title":"Reuters","url":"https://www.reuters.com/example"}]}}}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","response":{"id":"resp_oauth_2","model":"gpt-5.4","output":[{"type":"web_search_call","id":"ws_same","status":"completed","action":{"type":"search","query":"openai pricing"}},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]}],"tool_usage":{"web_search":{"num_requests":0}},"usage":{"input_tokens":1,"output_tokens":2}}}`,
			``,
			`data: [DONE]`,
		}, "\n"))),
	}

	usage, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)

	bodyText := rec.Body.String()
	output := gjson.Get(bodyText, "output").Array()
	wsCount := 0
	for _, item := range output {
		if item.Get("id").String() != "ws_same" {
			continue
		}
		wsCount++
		require.Equal(t, "https://www.reuters.com/example", item.Get("action.sources.0.url").String())
	}
	require.Equal(t, 1, wsCount)
	require.Equal(t, int64(1), gjson.Get(bodyText, `tool_usage.web_search.num_requests`).Int())
}

func TestHandleSSEToJSON_UniqueDeltaOnlyMessageSlotMergesWithTerminalMessageID(t *testing.T) {
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
			`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"openai pricing"}}}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"pricing result"}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","response":{"id":"resp_oauth_unique_1","model":"gpt-5.4","output":[{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"openai pricing"}},{"type":"message","id":"msg_term_1","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]}],"tool_usage":{"web_search":{"num_requests":1}},"usage":{"input_tokens":1,"output_tokens":2}}}`,
			``,
			`data: [DONE]`,
		}, "\n"))),
	}

	usage, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)

	bodyText := rec.Body.String()
	messageCount := 0
	for _, item := range gjson.Get(bodyText, "output").Array() {
		if item.Get("type").String() == "message" {
			messageCount++
			require.Equal(t, "msg_term_1", item.Get("id").String())
		}
	}
	require.Equal(t, 1, messageCount)
	require.Equal(t, int64(1), gjson.Get(bodyText, `tool_usage.web_search.num_requests`).Int())
}

func TestHandleSSEToJSON_OpenCodeStillFiltersSupplementedWebSearchCallAndSkipsToolUsageFix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.4.3")

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	body := []byte(strings.Join([]string{
		`event: response.output_item.done`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"openai pricing","sources":[{"type":"url_citation","title":"Reuters","url":"https://www.reuters.com/example"}]}}}`,
		``,
		`event: response.reasoning_summary_text.delta`,
		`data: {"type":"response.reasoning_summary_text.delta","output_index":1,"summary_index":0,"delta":"search strategy"}`,
		``,
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","output_index":2,"content_index":0,"delta":"pricing result"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_opencode_1","model":"gpt-5.4","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]}],"tool_usage":{"web_search":{"num_requests":0}},"usage":{"input_tokens":1,"output_tokens":2}}}`,
		``,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSONForAccount(resp, c, body, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)

	bodyText := rec.Body.String()
	require.NotContains(t, bodyText, `"type":"web_search_call"`)
	require.Contains(t, bodyText, `pricing result`)
	require.Contains(t, bodyText, `search strategy`)
	require.Equal(t, int64(0), gjson.Get(bodyText, `tool_usage.web_search.num_requests`).Int())
}

func TestHandleNonStreamingResponse_APIKeySSEFallbackDoesNotSupplementOAuthOnlySemantics(t *testing.T) {
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
			`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"openai pricing","sources":[{"type":"url_citation","title":"Reuters","url":"https://www.reuters.com/example"}]}}}`,
			``,
			`event: response.reasoning_summary_text.delta`,
			`data: {"type":"response.reasoning_summary_text.delta","output_index":1,"summary_index":0,"delta":"search strategy"}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","output_index":2,"content_index":0,"delta":"pricing result"}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","response":{"id":"resp_api_key_oauth_only_1","model":"gpt-5.4","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]}],"tool_usage":{"web_search":{"num_requests":0}},"usage":{"input_tokens":1,"output_tokens":2}}}`,
			``,
			`data: [DONE]`,
		}, "\n"))),
	}

	usage, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)

	bodyText := rec.Body.String()
	require.NotContains(t, bodyText, `"type":"web_search_call"`)
	require.Equal(t, int64(0), gjson.Get(bodyText, `tool_usage.web_search.num_requests`).Int())
}

func TestHandleSSEToJSON_OpenCodeFiltersWebSearchCallFromFinalResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.4.3")

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	body := []byte(strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.4","output":[{"type":"web_search_call","id":"ws_1","action":{"type":"search","query":"openai pricing"}},{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"search strategy"}]},{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		``,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 1, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)
	require.NotContains(t, rec.Body.String(), `web_search_call`)
	require.Contains(t, rec.Body.String(), `pricing result`)
	require.Contains(t, rec.Body.String(), `search strategy`)
	require.NotContains(t, rec.Body.String(), `"summary":[]`)
}

func TestHandleSSEToJSON_ReconstructsImageGenerationOutputItemDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	body := []byte(strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"id":"ig_123","type":"image_generation_call","result":"aGVsbG8=","revised_prompt":"draw a cat","output_format":"png"}}`,
		`data: {"type":"response.completed","response":{"id":"resp_img","model":"gpt-5.4","output":[],"usage":{"input_tokens":7,"output_tokens":9,"output_tokens_details":{"image_tokens":4}}}}`,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 4, usage.ImageOutputTokens)
	require.NotContains(t, rec.Body.String(), "data:")
	require.Equal(t, "image_generation_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "aGVsbG8=", gjson.Get(rec.Body.String(), "output.0.result").String())
	require.Equal(t, "draw a cat", gjson.Get(rec.Body.String(), "output.0.revised_prompt").String())
}

func TestHandleSSEToJSON_NoFinalResponseKeepsSSEBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	body := []byte(strings.Join([]string{
		`data: {"type":"response.in_progress","response":{"id":"resp_3"}}`,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-4o", "gpt-4o")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.InputTokens)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, rec.Body.String(), `data: {"type":"response.in_progress"`)
}

func TestHandleSSEToJSON_ResponseFailedReturnsProtocolError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	body := []byte(strings.Join([]string{
		`data: {"type":"response.failed","error":{"message":"upstream rejected request"}}`,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-4o", "gpt-4o")
	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "upstream rejected request")
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}

func TestHandleOAuthSSEToJSON_ReconstructsEmptyOutputFromDelta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	body := []byte(strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"hello "}`,
		`data: {"type":"response.output_text.delta","delta":"world"}`,
		`data: {"type":"response.done","response":{"id":"resp_reconstructed","model":"gpt-4o","output":[],"usage":{"input_tokens":2,"output_tokens":3}}}`,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleOAuthSSEToJSON(resp, c, body, "gpt-4o", "gpt-4o")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 2, usage.InputTokens)
	require.Equal(t, 3, usage.OutputTokens)
	require.Contains(t, rec.Body.String(), `"hello world"`)
	require.NotContains(t, rec.Body.String(), `"output":[]`)
}

func TestHandleNonStreamingResponse_EventStreamAppliesToAPIKeyAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"hello "}`,
			`data: {"type":"response.output_text.delta","delta":"world"}`,
			`data: {"type":"response.done","response":{"id":"resp_api_key_sse","model":"gpt-4o","output":[],"usage":{"input_tokens":2,"output_tokens":3}}}`,
			`data: [DONE]`,
		}, "\n"))),
	}

	usage, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Type: AccountTypeAPIKey}, "gpt-4o", "gpt-4o")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 2, usage.InputTokens)
	require.Equal(t, 3, usage.OutputTokens)
	require.Contains(t, rec.Body.String(), `"hello world"`)
	require.NotContains(t, rec.Body.String(), `"output":[]`)
}

func TestHandleNonStreamingResponse_OpenCodeFiltersWebSearchCallOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.4.3")

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_1","model":"gpt-5.4","output":[{"type":"web_search_call","id":"ws_1","action":{"type":"search","query":"openai pricing"}},{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"search strategy"}]},{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]}],"usage":{"input_tokens":1,"output_tokens":2}}`)),
	}

	usage, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Type: AccountTypeAPIKey}, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 1, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)
	require.NotContains(t, rec.Body.String(), `web_search_call`)
	require.Contains(t, rec.Body.String(), `pricing result`)
	require.Contains(t, rec.Body.String(), `search strategy`)
	require.NotContains(t, rec.Body.String(), `"summary":[]`)
}

func TestHandleNonStreamingResponse_NormalizesResponsesJSONForAISDK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_sdk_1","model":"gpt-5.4","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]},{"type":"web_search_call","id":"ws_1","action":{"type":"search","query":"openai pricing"}}],"usage":{"input_tokens":1,"output_tokens":2}}`)),
	}

	usage, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Type: AccountTypeAPIKey}, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 1, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)

	body := rec.Body.String()
	require.Contains(t, body, `"id":"msg_`)
	require.Contains(t, body, `"annotations":[]`)
	require.Contains(t, body, `"type":"web_search_call"`)
	require.Contains(t, body, `pricing result`)
}

func TestHandleChatBufferedStreamingResponse_ReconstructsEmptyOutputFromDelta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid-buffered"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"hello "}`,
			`data: {"type":"response.output_text.delta","delta":"world"}`,
			`data: {"type":"response.done","response":{"id":"resp_buffered","output":[],"usage":{"input_tokens":2,"output_tokens":3}}}`,
			`data: [DONE]`,
		}, "\n"))),
	}

	result, err := svc.handleChatBufferedStreamingResponse(resp, c, "gpt-4o", "gpt-4o", "gpt-4o", time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 2, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), `"hello world"`)
}

func TestHandleStreamingResponse_OpenCodeSuppressesWebSearchCallFramesButPreservesReasoningSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.4.3")

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created","response":{"id":"resp_stream_1"}}`,
			``,
			`event: response.output_item.added`,
			`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"web_search_call","id":"ws_1","action":{"type":"search","query":"openai pricing"}}}`,
			``,
			`event: response.reasoning_summary_text.delta`,
			`data: {"type":"response.reasoning_summary_text.delta","output_index":1,"summary_index":0,"delta":"search strategy"}`,
			``,
			`event: response.output_item.done`,
			`data: {"type":"response.output_item.done","output_index":1,"item":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"search strategy"}]}}`,
			``,
			`event: response.output_item.added`,
			`data: {"type":"response.output_item.added","output_index":2,"item":{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]}}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","output_index":2,"content_index":0,"delta":"pricing result"}`,
			``,
			`event: response.web_search_call.searching`,
			`data: {"type":"response.web_search_call.searching","output_index":0}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","response":{"id":"resp_stream_1","model":"gpt-5.4","output":[{"type":"web_search_call","id":"ws_1","action":{"type":"search","query":"openai pricing"}},{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"search strategy"}]},{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n"))),
	}

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotContains(t, rec.Body.String(), `web_search_call`)
	require.NotContains(t, rec.Body.String(), `response.web_search_call.searching`)
	require.Contains(t, rec.Body.String(), `response.reasoning_summary_text.delta`)
	require.Contains(t, rec.Body.String(), `search strategy`)
	require.Contains(t, rec.Body.String(), `pricing result`)
	require.NotContains(t, rec.Body.String(), `"summary":[]`)
	require.NotContains(t, rec.Body.String(), "event: response.web_search_call")
}

func TestHandleSSEToJSON_NormalizesCompletedResponsesJSONForAISDK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	body := []byte(strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.4","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]},{"type":"web_search_call","id":"ws_1","action":{"type":"search","query":"openai pricing"}}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		``,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 1, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)

	finalBody := rec.Body.String()
	require.Contains(t, finalBody, `"id":"msg_`)
	require.Contains(t, finalBody, `"annotations":[]`)
	require.Contains(t, finalBody, `"type":"web_search_call"`)
	require.Contains(t, finalBody, `pricing result`)
}
