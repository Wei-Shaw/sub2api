//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type deepseekTestTransport struct {
	service.HTTPUpstream
	accounts []int64
	respond  func(*http.Request, int64) (*http.Response, error)
}

func (u *deepseekTestTransport) Do(r *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.accounts = append(u.accounts, accountID)
	return u.respond(r, accountID)
}

func deepseekTestReply(status int, body io.Reader) *http.Response {
	return &http.Response{StatusCode: status, ContentLength: -1, Body: io.NopCloser(body),
		Header: http.Header{"X-Request-Id": {"test-upstream"}, "Retry-After": {"10"}}}
}

type deepseekPolicyRepo struct {
	openAIWSFailoverHandlerAccountRepoStub
	disabled []int64
}

func (r *deepseekPolicyRepo) UpdateExtra(context.Context, int64, map[string]any) error { return nil }

func (r *deepseekPolicyRepo) SetError(_ context.Context, id int64, _ string) error {
	r.disabled = append(r.disabled, id)
	return nil
}

func (r *deepseekPolicyRepo) SetTempUnschedulable(_ context.Context, id int64, _ time.Time, _ string) error {
	r.disabled = append(r.disabled, id)
	return nil
}

type deepseekProxyFileCache struct {
	stickyWritebackCache
	service.DeepseekFileStore
}

func (*deepseekProxyFileCache) LookupDeepseekFiles(context.Context, int64, []string) (map[string]service.DeepseekFile, error) {
	return map[string]service.DeepseekFile{"owned": {AccountID: 1, Origin: service.DefaultDeepseekBaseURL}}, nil
}

func newDeepseekProxyHandler(t *testing.T, n int, up *deepseekTestTransport, slots *service.ConcurrencyService) (*OpenAIGatewayHandler, *deepseekPolicyRepo, *compatStreamUsageBillingRepo) {
	t.Helper()
	cfg := &config.Config{}
	repo := &deepseekPolicyRepo{}
	for i := range n {
		repo.accounts = append(repo.accounts, service.Account{ID: int64(i + 1), GroupIDs: []int64{163}, Platform: service.PlatformDeepseek,
			Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: i + 1, Concurrency: 1,
			Credentials: map[string]any{"api_key": "test"}})
	}
	billing := service.NewBillingCacheService(nil, &cachedBillingUserRepo{balance: 5}, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	commands := &compatStreamUsageBillingRepo{}
	rateLimit := service.NewRateLimitService(repo, nil, cfg, nil, nil)
	gateway := service.NewOpenAIGatewayService(repo, &cachedBillingUsageRepo{}, commands, nil, nil, nil, &deepseekProxyFileCache{}, cfg, nil, slots,
		service.NewBillingService(cfg, nil), rateLimit, billing, up, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	if slots == nil {
		slots = service.NewConcurrencyService(nil)
	}
	return &OpenAIGatewayHandler{gatewayService: gateway, billingCacheService: billing, apiKeyService: &service.APIKeyService{},
		concurrencyHelper: NewConcurrencyHelper(slots, SSEPingFormatNone, 0), cfg: cfg}, repo, commands
}

func requireDeepseekError(t *testing.T, w *httptest.ResponseRecorder, status int, anthropic bool) {
	t.Helper()
	require.Equal(t, status, w.Code, w.Body.String())
	require.True(t, json.Valid(w.Body.Bytes()), "expected exactly one JSON document, got %q", w.Body.String())
	require.NotEmpty(t, gjson.GetBytes(w.Body.Bytes(), "error.message").String())
	require.Equal(t, anthropic, gjson.GetBytes(w.Body.Bytes(), "type").String() == "error")
}

func TestDeepseekToolsHTTPFailover(t *testing.T) {
	for _, status := range []int{401, 402, 403, 408, 429, 500, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			up := &deepseekTestTransport{respond: func(_ *http.Request, id int64) (*http.Response, error) {
				if id == 1 {
					return deepseekTestReply(status, strings.NewReader(`{"error":{"message":"rejected"}}`)), nil
				}
				return deepseekTestReply(200, strings.NewReader(`{"results":[]}`)), nil
			}}
			h, repo, commands := newDeepseekProxyHandler(t, 2, up, nil)
			c, w := fileOwnerTestContext("POST", "/v1/tools/search", `{"query":"q"}`, service.PlatformDeepseek, "", 7)
			h.DeepseekTools(c, "search")
			require.Equal(t, []int64{1, 2}, up.accounts)
			require.Equal(t, 200, w.Code)
			require.JSONEq(t, `{"results":[]}`, w.Body.String())
			require.Empty(t, commands.commands)
			switch status {
			case 401, 402, 403:
				require.Contains(t, repo.disabled, int64(1))
			case 429:
				require.Contains(t, repo.rateLimitedIDs, int64(1))
			}
			events := c.MustGet(service.OpsUpstreamErrorsKey).([]*service.OpsUpstreamErrorEvent)
			require.Len(t, events, 1)
			require.Equal(t, status, events[0].UpstreamStatusCode)
			require.Equal(t, "test-upstream", events[0].UpstreamRequestID)
			require.Equal(t, "failover", events[0].Kind)
		})
	}
}

func TestDeepseekToolsRequestRefusals(t *testing.T) {
	for _, tc := range []struct {
		endpoint string
		status   int
	}{
		{"search", 400}, {"fetch", 403}, {"fetch", 404}, {"ocr", 413},
		{"asr", 400}, {"tts", 422}, {"files_get/owned", 404},
	} {
		t.Run(tc.endpoint+"/"+strconv.Itoa(tc.status), func(t *testing.T) {
			up := &deepseekTestTransport{respond: func(*http.Request, int64) (*http.Response, error) {
				return deepseekTestReply(tc.status, strings.NewReader(`{"error":{"message":"rejected https://example.test/?key=private"}}`)), nil
			}}
			h, repo, commands := newDeepseekProxyHandler(t, 2, up, nil)
			c, w := fileOwnerTestContext("POST", "/v1/tools/"+tc.endpoint, `{"query":"q"}`, service.PlatformDeepseek, "", 7)
			if tc.endpoint == "tts" {
				// Account error-code preferences must not mask a request-level refusal.
				repo.accounts[0].Credentials["custom_error_codes_enabled"] = true
				repo.accounts[0].Credentials["custom_error_codes"] = []any{float64(500)}
			}
			if tc.endpoint == "files_get/owned" {
				c, w = fileOwnerTestContext("GET", "/v1/files/owned", "", service.PlatformDeepseek, "", 7)
			}
			h.DeepseekTools(c, tc.endpoint)
			requireDeepseekError(t, w, tc.status, false)
			require.Equal(t, "rejected https://example.test/?key=***", gjson.GetBytes(w.Body.Bytes(), "error.message").String())
			require.Equal(t, []int64{1}, up.accounts)
			require.Empty(t, repo.disabled)
			require.Empty(t, repo.rateLimitedIDs)
			require.Empty(t, commands.commands)
			events := c.MustGet(service.OpsUpstreamErrorsKey).([]*service.OpsUpstreamErrorEvent)
			require.Len(t, events, 1)
			require.Equal(t, "http_error", events[0].Kind)
		})
	}
}

func TestDeepseekToolsExhaustedClientStatus(t *testing.T) {
	for _, tc := range []struct {
		status        int
		body, message string
	}{
		{408, `{"error":{"message":"upload timed out https://example.test/?key=private"}}`, "upload timed out https://example.test/?key=***"},
		{408, "internal diagnostics", http.StatusText(408)},
		{429, `{"error":{"message":"upload quota reached"}}`, "upload quota reached"},
	} {
		up := &deepseekTestTransport{respond: func(*http.Request, int64) (*http.Response, error) {
			return deepseekTestReply(tc.status, strings.NewReader(tc.body)), nil
		}}
		h, _, _ := newDeepseekProxyHandler(t, 2, up, nil)
		c, w := fileOwnerTestContext("POST", "/v1/tools/ocr", "upload", service.PlatformDeepseek, "", 7)
		h.DeepseekTools(c, "ocr")
		requireDeepseekError(t, w, tc.status, false)
		require.Equal(t, []int64{1, 2}, up.accounts)
		require.Equal(t, tc.message, gjson.GetBytes(w.Body.Bytes(), "error.message").String())
		require.Equal(t, "10", w.Header().Get("Retry-After"))
	}
}

func TestDeepseekToolsSingleErrorResponse(t *testing.T) {
	for _, tc := range []struct {
		name             string
		accounts, status int
	}{
		{"exhausted", 5, 502}, {"token", 1, 502}, {"URL", 1, 502}, {"body read", 1, 502}, {"upstream 400", 1, 400}, {"invalid file ID", 0, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up := &deepseekTestTransport{respond: func(_ *http.Request, _ int64) (*http.Response, error) {
				switch tc.name {
				case "exhausted":
					return deepseekTestReply(503, strings.NewReader("private upstream diagnostics")), nil
				case "body read":
					return deepseekTestReply(200, iotest.ErrReader(io.ErrUnexpectedEOF)), nil
				default:
					return deepseekTestReply(400, strings.NewReader(`{"error":{"message":"bad url https://test/?key=secret"}}`)), nil
				}
			}}
			h, repo, _ := newDeepseekProxyHandler(t, tc.accounts, up, nil)
			endpoint := "search"
			switch tc.name {
			case "token":
				delete(repo.accounts[0].Credentials, "api_key")
			case "URL":
				repo.accounts[0].Credentials["base_url"] = "ftp://invalid"
				repo.accounts[0].Extra = map[string]any{"deepseek_tools_mode": "force_supported"}
			case "invalid file ID":
				endpoint = "files_get/bad?id"
			}
			c, w := fileOwnerTestContext("POST", "/v1/tools/search", `{"query":"q"}`, service.PlatformDeepseek, "", 7)

			c.Request.Header.Set("anthropic-beta", "files-api-2025-04-14")
			h.DeepseekTools(c, endpoint)
			requireDeepseekError(t, w, tc.status, tc.name == "invalid file ID")
			require.NotContains(t, w.Body.String(), "private upstream")
			require.NotContains(t, w.Body.String(), "secret")
			if tc.name == "exhausted" {
				require.Len(t, up.accounts, 4)
			}

		})
	}
}

type deepseekAdmissionCache struct {
	deepseekToolsSlotCache
	userIDs      []int64
	userLimits   []int
	userReleased int
	denyUser     bool
	failAccount  int64
}

func (s *deepseekAdmissionCache) AcquireUserSlot(_ context.Context, id int64, limit int, _ string) (bool, error) {
	s.userIDs = append(s.userIDs, id)
	s.userLimits = append(s.userLimits, limit)
	if s.denyUser {
		return false, errors.New("user slot unavailable")
	}
	return true, nil
}
func (s *deepseekAdmissionCache) ReleaseUserSlot(context.Context, int64, string) error {
	s.userReleased++
	return nil
}
func (s *deepseekAdmissionCache) AcquireAccountSlot(ctx context.Context, id int64, limit int, req string) (bool, error) {
	if id == s.failAccount {
		return false, errors.New("slot unavailable")
	}
	return s.deepseekToolsSlotCache.AcquireAccountSlot(ctx, id, limit, req)
}

func TestDeepseekToolsSlotErrorAfterFailover(t *testing.T) {
	cache := &deepseekAdmissionCache{deepseekToolsSlotCache: deepseekToolsSlotCache{held: map[string]struct{}{}}, failAccount: 2}
	up := &deepseekTestTransport{respond: func(*http.Request, int64) (*http.Response, error) {
		return deepseekTestReply(503, strings.NewReader("{}")), nil
	}}
	h, _, _ := newDeepseekProxyHandler(t, 2, up, service.NewConcurrencyService(cache))
	c, w := fileOwnerTestContext("POST", "/v1/tools/search", `{}`, service.PlatformDeepseek, "", 7)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7, Concurrency: 3})
	h.DeepseekTools(c, "search")
	requireDeepseekError(t, w, 503, false)
	require.Equal(t, []int64{1}, up.accounts)
	require.Equal(t, []int64{7}, cache.userIDs)
	require.Equal(t, []int{3}, cache.userLimits)
	require.Equal(t, 1, cache.userReleased)
	require.Zero(t, cache.outstanding())

	cache.denyUser = true
	c, w = fileOwnerTestContext("GET", "/v1/files", "", "deepseek", "", 7)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7, Concurrency: 3})
	c.Request.Header.Set("anthropic-beta", "files-api-2025-04-14")
	h.DeepseekTools(c, "files_list")
	requireDeepseekError(t, w, 503, true)
	require.Equal(t, []int64{1}, up.accounts)
}

func TestDeepseekToolsCommittedResponse(t *testing.T) {
	h, _, _ := newDeepseekProxyHandler(t, 0, nil, nil)
	c, w := fileOwnerTestContext("POST", "/v1/tools/search", "{}", "deepseek", "", 1)
	c.String(200, "already committed")
	h.DeepseekTools(c, "search")
	require.Equal(t, "already committed", w.Body.String())
}

type deepseekVetoSlots struct {
	deepseekToolsSlotCache
	rates map[int64]*float64
}

func (s *deepseekVetoSlots) AcquireAccountSlot(ctx context.Context, id int64, limit int, request string) (bool, error) {
	ok, err := s.deepseekToolsSlotCache.AcquireAccountSlot(ctx, id, limit, request)
	// Simulate a price refresh between selection and the post-slot profit check.
	if ok {
		*s.rates[id] = 2
	}
	return ok, err
}

func TestDeepseekToolsExhaustedSlotVetoes(t *testing.T) {
	slots := &deepseekVetoSlots{deepseekToolsSlotCache: deepseekToolsSlotCache{held: map[string]struct{}{}}, rates: map[int64]*float64{}}
	up := &deepseekTestTransport{}
	h, repo, _ := newDeepseekProxyHandler(t, 5, up, service.NewConcurrencyService(slots))
	for i := range repo.accounts {
		acc := &repo.accounts[i]
		acc.RateMultiplier = new(0.5)
		slots.rates[acc.ID] = acc.RateMultiplier
	}
	c, w := fileOwnerTestContext("POST", "/v1/tools/search", "{}", "deepseek", "", 7)
	key, _ := middleware.GetAPIKeyFromContext(c)
	// Install the shared scheduler gate while keeping the normal DeepSeek
	// handler, candidate accounts, slot cache, and post-acquisition checks.
	gateGroup := &service.Group{ID: *key.GroupID, Hydrated: true, Platform: service.PlatformOpenAI, Status: service.StatusActive, RateMultiplier: 1, ProfitControlEnabled: true}
	ctx, _ := h.gatewayService.WithOpenAIRequestPricingContext(context.WithValue(c.Request.Context(), ctxkey.Group, gateGroup), key.GroupID)
	c.Request = c.Request.WithContext(ctx)
	h.DeepseekTools(c, "search")
	requireDeepseekError(t, w, 503, false)
	require.Equal(t, 4, slots.acquires)
	require.Equal(t, 4, slots.releaseCount())
	require.Empty(t, up.accounts)
}

func TestDeepseekToolsIncompleteTTSBody(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		length     int64
	}{
		{"truncated content length", "audio", 50}, {"empty audio", "", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up := &deepseekTestTransport{respond: func(*http.Request, int64) (*http.Response, error) {
				r := deepseekTestReply(200, strings.NewReader(tc.body))
				r.Header.Set("Content-Type", "audio/wav")
				r.ContentLength = tc.length
				return r, nil
			}}
			h, _, commands := newDeepseekProxyHandler(t, 2, up, nil)
			c, w := fileOwnerTestContext("POST", "/v1/tools/tts", `{"text":"test"}`, "deepseek", "", 7)
			require.PanicsWithValue(t, http.ErrAbortHandler, func() { h.DeepseekTools(c, "tts") })
			require.Equal(t, tc.body, w.Body.String())
			require.Empty(t, commands.commands)
			require.Len(t, up.accounts, 1)
		})
	}
}

func TestDeepseekToolsOwnedFileErrors(t *testing.T) {
	for _, beta := range []bool{false, true} {
		for _, status := range []int{200, 400, 429} {
			up := &deepseekTestTransport{respond: func(*http.Request, int64) (*http.Response, error) {
				if status == 200 {
					return deepseekTestReply(200, iotest.ErrReader(io.ErrUnexpectedEOF)), nil
				}
				return deepseekTestReply(status, strings.NewReader(`{"error":{"message":"test"}}`)), nil
			}}
			h, _, _ := newDeepseekProxyHandler(t, 2, up, nil)
			c, w := fileOwnerTestContext("GET", "/v1/files/owned", "", "deepseek", "", 7)
			if beta {
				c.Request.Header.Set("anthropic-beta", "files-api-2025-04-14")
			}
			h.DeepseekTools(c, "files_get/owned")
			want := status
			if status == 200 {
				want = 502
			}
			requireDeepseekError(t, w, want, beta)
			require.Equal(t, []int64{1}, up.accounts, "file failover must stay on the owner")
			if status == 429 {
				require.Equal(t, "10", w.Header().Get("Retry-After"))
			}
		}
	}
}

func TestDeepseekToolsAuditPrompt(t *testing.T) {
	require.Equal(t, "你好", extractDeepseekToolsPromptText([]byte(`{"query":" \u4f60\u597d ","text":"unused"}`)))
	require.Equal(t, "example", extractDeepseekToolsPromptText([]byte(`{"query":false,"url":" example "}`)))
	require.Empty(t, extractDeepseekToolsPromptText([]byte(`{"text":"unterminated"`)))
}

type deepseekDisconnectWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
	writes int
}

func (w *deepseekDisconnectWriter) Write([]byte) (int, error) {
	w.writes++
	w.cancel()
	return 0, io.ErrClosedPipe
}

func TestDeepseekToolsStreamSettlement(t *testing.T) {
	for _, tc := range []struct {
		name, endpoint, first, rest string
		disconnect, truncated       bool
		cost                        float64
	}{
		{"OCR keepalive and pages", "ocr", "\n", `{"pages":[{},{}]}`, false, false, 2 * .225 / 1000},
		{"OCR error in 200", "ocr", "\n", `{"error":{"message":"timeout"},"pages":[{}],"page_count":5}`, false, false, 0},
		{"OCR client disconnect", "ocr", "\n", `{"object":"ocr.result","page_count":3}`, true, false, 3 * .225 / 1000},
		{"TTS client disconnect", "tts", "PCM", "more audio", true, false, 4 * .45 / 10000},
		{"TTS upstream abort", "tts", "PCM", "", false, true, 0},
		{"TTS JSON-looking PCM abort", "tts", "{", "", false, true, 0},
		{"TTS both disconnect", "tts", "PCM", "", true, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, w := fileOwnerTestContext("POST", "/v1/tools/"+tc.endpoint, `{"text":"abcd"}`, "deepseek", "", 7)
			key, _ := middleware.GetAPIKeyFromContext(c)
			key.Group.RateMultiplier = 1
			ctx, cancel := context.WithCancel(c.Request.Context())
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			if tc.disconnect {
				c.Writer = &deepseekDisconnectWriter{ResponseWriter: c.Writer, cancel: cancel}
			}
			slots := &deepseekAdmissionCache{deepseekToolsSlotCache: deepseekToolsSlotCache{held: map[string]struct{}{}}}
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7, Concurrency: 1})
			var reachedEnd bool
			up := &deepseekTestTransport{respond: func(r *http.Request, _ int64) (*http.Response, error) {
				body := io.MultiReader(deepseekReadFunc(func([]byte) (int, error) {
					if tc.endpoint == "ocr" {
						require.True(t, w.Flushed, "OCR keepalives must not wait for a result")
					}
					return 0, io.EOF
				}), strings.NewReader(tc.first), deepseekReadFunc(func([]byte) (int, error) {
					require.NoError(t, r.Context().Err())
					if tc.disconnect {
						require.ErrorIs(t, ctx.Err(), context.Canceled)
						require.Equal(t, 1, slots.outstanding())
						require.Zero(t, slots.userReleased)
					} else if tc.first != "{" {
						require.Equal(t, tc.first, w.Body.String())
					}
					if tc.truncated {
						return 0, io.ErrUnexpectedEOF
					}
					return 0, io.EOF
				}), strings.NewReader(tc.rest), deepseekReadFunc(func([]byte) (int, error) { reachedEnd = true; return 0, io.EOF }))
				response := deepseekTestReply(200, body)
				if tc.endpoint == "tts" {
					response.Header.Set("Content-Type", "audio/pcm")
				}
				return response, nil
			}}
			h, _, commands := newDeepseekProxyHandler(t, 2, up, service.NewConcurrencyService(slots))
			if tc.truncated {
				require.PanicsWithValue(t, http.ErrAbortHandler, func() { h.DeepseekTools(c, tc.endpoint) })
			} else {
				h.DeepseekTools(c, tc.endpoint)
				require.True(t, reachedEnd)
			}
			require.Len(t, up.accounts, 1)
			require.Zero(t, slots.outstanding())
			require.Equal(t, 1, slots.userReleased)
			if tc.disconnect {
				require.Equal(t, 1, c.Writer.(*deepseekDisconnectWriter).writes)
			} else {
				require.Equal(t, tc.first+tc.rest, w.Body.String())
			}
			if tc.cost == 0 {
				require.Empty(t, commands.commands)
			} else {
				require.Len(t, commands.commands, 1)
				require.InDelta(t, tc.cost, commands.commands[0].BalanceCost, 1e-12)
			}
		})
	}
}

type deepseekReadFunc func([]byte) (int, error)

func (f deepseekReadFunc) Read(p []byte) (int, error) { return f(p) }

func TestDeepseekTTSRejectsNonAudioResults(t *testing.T) {
	for _, tc := range []struct {
		status      int
		media, body string
	}{
		{202, "audio/wav", ""}, {204, "", ""}, {200, "application/json", `{"error":{"message":"synthesis rejected"}}`},
		{200, "audio/wav", ` {"error":{"message":"synthesis rejected"}}`},
	} {
		up := &deepseekTestTransport{respond: func(*http.Request, int64) (*http.Response, error) {
			r := deepseekTestReply(tc.status, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.media)
			return r, nil
		}}
		h, _, commands := newDeepseekProxyHandler(t, 1, up, nil)
		c, w := fileOwnerTestContext("POST", "/v1/tools/tts", `{"text":"hello"}`, "deepseek", "", 7)
		h.DeepseekTools(c, "tts")
		requireDeepseekError(t, w, 502, false)
		require.Empty(t, commands.commands)
		if tc.body != "" {
			require.Contains(t, w.Body.String(), "synthesis rejected")
		}
	}
}
