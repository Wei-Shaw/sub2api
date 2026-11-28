//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/model"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise the forwarded HTTP request without opening a network listener.
type deepseekToolsHTTPUpstream struct{ handler http.Handler }

func (u *deepseekToolsHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	rec := httptest.NewRecorder()
	if req.Body == nil {
		req.Body = http.NoBody
	}
	u.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

func (u *deepseekToolsHTTPUpstream) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, concurrency)
}

func newDeepseekToolsTestAccount(baseURL string) *Account {
	return &Account{
		ID:          901,
		Name:        "deepseek-tools",
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-dstest",
			"base_url": baseURL,
		},
	}
}

func newDeepseekToolsTestContext(method, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, path, bytes.NewReader(body))
	return c, rec
}

func forwardDeepseekToolsTest(svc *OpenAIGatewayService, c *gin.Context, account *Account, userID int64, endpoint string, body []byte, contentType string) (*DeepseekToolsForwardResult, error) {
	ctx := context.Background()
	request, err := svc.PrepareDeepseekToolsRequest(ctx, c, userID, endpoint, body, contentType)
	if err != nil {
		return nil, err
	}
	return svc.ForwardDeepseekToolsRequest(request.SchedulingContext(ctx), c, account, request)
}

func TestForwardDeepseekToolsRequest_SearchPassthrough(t *testing.T) {
	var gotAuth, gotMethod, gotPath string
	var gotBody []byte
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[{"title":"doc"}]}`))
	})

	body := []byte(`{"query":"deepseek docs"}`)
	c, rec := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/search", body)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}}
	result, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 0, "search", body, "application/json")
	require.NoError(t, err)
	require.Equal(t, "Bearer sk-dstest", gotAuth)
	require.Equal(t, http.MethodPost, gotMethod)
	require.Equal(t, "/v1/tools/search", gotPath)
	require.Equal(t, body, gotBody)
	require.Equal(t, `{"results":[{"title":"doc"}]}`, rec.Body.String())
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.Equal(t, DeepseekToolsUsage{Endpoint: "search", Units: 1}, result.Usage)
}

func TestForwardDeepseekToolsRequest_MultipartBoundaryPreserved(t *testing.T) {
	var gotContentType string
	var gotBody []byte
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pages":[{"text":"a"},{"text":"b"},{"text":"c"}],"usage":{"pages":99}}`))
	})

	rawBody := "--dstestboundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.png\"\r\nContent-Type: image/png\r\n\r\nPNGDATA\r\n--dstestboundary--\r\n"
	multipartType := "multipart/form-data; boundary=dstestboundary"
	c, _ := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/ocr", []byte(rawBody))
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}}
	result, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 0, "ocr", []byte(rawBody), multipartType)
	require.NoError(t, err)
	require.Equal(t, multipartType, gotContentType, "boundary must reach upstream intact")
	require.Equal(t, rawBody, string(gotBody))
	// 页数以 pages 数组长度为准，usage 仅参考。
	require.Equal(t, DeepseekToolsUsage{Endpoint: "ocr", Units: 3}, result.Usage)
}

func TestForwardDeepseekToolsRequest_OwnedGetDelete(t *testing.T) {
	var paths []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.RequestURI())
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Empty(t, body)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			if r.Header.Get("anthropic-beta") != "" {
				_, _ = w.Write([]byte(`{"id":"file-abc_1","type":"file_deleted"}`))
			} else {
				_, _ = w.Write([]byte(`{"id":"file-abc_1","object":"file","deleted":true}`))
			}
			return
		}
		_, _ = w.Write([]byte(`{"id":"file-abc_1"}`))
	})
	account := newDeepseekToolsTestAccount("https://relay.test")
	cache := &testDeepseekFileCache{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}, cache: cache}
	const query = "?tag=one&tag=two&q=some%20text"
	for _, format := range []string{"openai", "anthropic"} {
		require.NoError(t, cache.SaveDeepseekFile(context.Background(), 1, DeepseekFile{ID: "file-abc_1", AccountID: account.ID, Origin: "https://relay.test"}))
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			endpoint := "files_get/file-abc_1"
			if method == http.MethodDelete {
				endpoint = "files_delete/file-abc_1"
			}
			c, _ := newDeepseekToolsTestContext(method, "/v1/files/file-abc_1"+query, nil)
			path := "/v1/files/file-abc_1"
			if format == "anthropic" {
				c.Request.Header.Set("anthropic-beta", deepseekFilesBetaHeader)
				path = "/anthropic" + path
			}
			count := len(paths)
			_, err := forwardDeepseekToolsTest(svc, c, account, 2, endpoint, nil, "")
			require.ErrorIs(t, err, errDeepseekFileNotFound)
			wrong := *account
			wrong.ID++
			_, err = forwardDeepseekToolsTest(svc, c, &wrong, 1, endpoint, nil, "")
			require.ErrorIs(t, err, errDeepseekFileNotFound)
			require.Len(t, paths, count)
			result, err := forwardDeepseekToolsTest(svc, c, account, 1, endpoint, nil, "")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, result.StatusCode)
			require.Zero(t, result.Usage.Units)
			require.Equal(t, method+" "+path, paths[count])
		}
		require.Empty(t, cache.files[1])
	}
}

func TestForwardDeepseekToolsRequest_TTSStreamsBinary(t *testing.T) {
	audio := bytes.Repeat([]byte{0x01, 0x02, 0x03, 0xFF}, 512)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Content-Length", strconv.Itoa(len(audio)))
		_, _ = w.Write(audio)
	})

	body := []byte(`{"text":"你好 world"}`)
	c, rec := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/tts", body)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}}
	result, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 0, "tts", body, "application/json")
	require.NoError(t, err)
	require.Equal(t, audio, rec.Body.Bytes(), "streamed audio bytes must match upstream")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "audio/wav", rec.Header().Get("Content-Type"))
	require.Equal(t, DeepseekToolsUsage{Endpoint: "tts", Units: 10}, result.Usage)
}

func TestForwardDeepseekToolsRequest_ASRDurationToSeconds(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"hi","duration_ms":3200}`))
	})

	c, _ := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/asr", []byte(`{}`))
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}}
	result, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 0, "asr", []byte(`{}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, DeepseekToolsUsage{Endpoint: "asr", Units: 4}, result.Usage)
}

func TestForwardDeepseekToolsRequest_UpstreamClientError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad audio"}}`))
	})

	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}}

	c, _ := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/asr", []byte(`{}`))
	result, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 0, "asr", []byte(`{}`), "application/json")
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Equal(t, "bad audio", infraerrors.Message(err))
	require.False(t, c.Writer.Written())
	require.Nil(t, result)
}

func TestForwardDeepseekToolsRequest_RejectsBadInput(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{}}

	_, err := forwardDeepseekToolsTest(svc, nil, &Account{Platform: PlatformOpenAI}, 0, "search", nil, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported")

	_, err = forwardDeepseekToolsTest(svc, nil, &Account{Platform: PlatformDeepseek}, 0, "unknown", nil, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported")

	_, err = forwardDeepseekToolsTest(svc, nil, &Account{Platform: PlatformDeepseek}, 0, "files_get/../etc", nil, "")
	require.Error(t, err)
}

func TestProbeDeepseekToolsSupportClearsStaleMarker(t *testing.T) {
	for _, tc := range []struct {
		name, base, body string
		status           int
	}{
		{"legacy official", "https://api.deepseek.com", "", 0},
		{"HTML 200", "https://relay.test", "<html>login</html>", 200},
		{"generic JSON", "https://relay.test", `{"ok":true}`, 200},
		{"error with results", "https://relay.test", `{"results":[],"error":{}}`, 200},
		{"truncated JSON", "https://relay.test", `{"results":[]`, 200},
		{"inconclusive HTTP", "https://relay.test", "", 429},
		{"invalid URL", "ftp://invalid", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := newDeepseekToolsTestAccount(tc.base)
			account.Extra = map[string]any{deepseekToolsSupportedExtraKey: true}
			s, repo, up := newDeepseekToolsProbeTestService(*account, &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))})
			s.ProbeDeepseekToolsSupport(context.Background(), account.ID)
			require.Len(t, repo.updateExtraCalls, 1)
			require.Equal(t, map[string]any{deepseekToolsSupportedExtraKey: nil}, <-repo.updateExtraCalls)
			if tc.status == 0 {
				require.Nil(t, up.lastReq)
			}
		})
	}
}

func TestDeepseekToolsUpstreamPassthroughRule(t *testing.T) {
	for _, tc := range []struct {
		endpoint              string
		status, accountErrors int
	}{
		{"search", 401, 1}, {"fetch", 403, 0},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			c, _ := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/"+tc.endpoint, nil)
			rules := &ErrorPassthroughService{}
			rule := newNonFailoverPassthroughRule(tc.status, "rejected", 422, "Configured rejection")
			rule.Platforms = []string{PlatformDeepseek}
			rule.SkipMonitoring = true
			rules.setLocalCache([]*model.ErrorPassthroughRule{rule})
			BindErrorPassthroughService(c, rules)
			repo := &compatStreamSideEffectsRepo{}
			s := &OpenAIGatewayService{rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
			resp := &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rejected"}}`))}
			err := s.deepseekToolsUpstreamError(context.Background(), c, newDeepseekToolsTestAccount("https://relay.test"), resp, tc.endpoint)
			require.Equal(t, 422, infraerrors.Code(err))
			require.Equal(t, "Configured rejection", infraerrors.Message(err))
			require.True(t, c.GetBool(OpsSkipPassthroughKey))
			require.False(t, c.Writer.Written())
			require.Len(t, repo.errorCalls, tc.accountErrors)
		})
	}
}

func TestDeepseekToolsBillableUnits(t *testing.T) {
	for _, tc := range []struct {
		name, endpoint string
		status         int
		request, reply string
		units          int
	}{
		{"search counts a call", "search", 200, "", `{"results":[{},{}]}`, 1},
		{"empty results", "search", 200, "", `{"results":[]}`, 0},
		{"missing results", "search", 200, "", `{}`, 0},
		{"non-array results", "search", 200, "", `{"results":{"0":{},"#":1}}`, 0},
		{"incomplete result", "search", 200, "", `{"results":[{}]`, 0},
		{"accepted search", "search", 202, "", `{"results":[{}]}`, 0},
		{"fetch page", "fetch", 200, "", `{"content":" text "}`, 1},
		{"fetch title only", "fetch", 200, "", `{"title":"Title"}`, 0},
		{"fetch empty page", "fetch", 200, "", `{"content":" \n "}`, 0},
		{"non-text content", "fetch", 200, "", `{"content":{}}`, 0},
		{"partial page", "fetch", 206, "", `{"content":"text"}`, 0},
		{"OCR result", "ocr", 200, "", "\n" + `{"object":"ocr.result","page_count":3}`, 3},
		{"OCR pages fallback", "ocr", 200, "", `{"pages":[{},{}]}`, 2},
		{"OCR empty result", "ocr", 200, "", `{"object":"ocr.result","pages":[]}`, 0},
		{"OCR count is authoritative", "ocr", 200, "", `{"object":"ocr.result","page_count":0,"pages":[{}]}`, 0},
		{"OCR error after keepalive", "ocr", 200, "", "\n\n" + `{"error":{"code":"gateway_timeout_error"},"page_count":5,"pages":[{}]}`, 0},
		{"wrong OCR object", "ocr", 200, "", `{"object":"file","page_count":3}`, 0},
		{"unrecognized envelope", "ocr", 200, "", `{"status":"failed","page_count":3}`, 0},
		{"negative OCR pages", "ocr", 200, "", `{"object":"ocr.result","page_count":-1}`, 0},
		{"accepted OCR", "ocr", 202, "", `{"object":"ocr.result","page_count":3}`, 0},
		{"fraction of a second", "asr", 200, "", `{"duration_ms":1}`, 1},
		{"whole seconds", "asr", 200, "", `{"duration_ms":3000}`, 3},
		{"rounded seconds", "asr", 200, "", `{"duration_ms":3200}`, 4},
		{"zero duration", "asr", 200, "", `{"duration_ms":0}`, 0},
		{"missing duration", "asr", 200, "", `{}`, 0},
		{"redirect with duration", "asr", 307, "", `{"duration_ms":1000}`, 0},
		{"TTS sample", "tts", 200, `{"text":"你好，世界。Hello DeepSeek"}`, "", 24},
		{"TTS preserves whitespace", "tts", 200, `{"text":" \t你\n "}`, "", 6},
		{"TTS whitespace only", "tts", 200, `{"text":"  "}`, "", 2},
		{"TTS empty", "tts", 200, `{"text":""}`, "", 0},
		{"TTS redirect", "tts", 302, `{"text":"hi"}`, "", 0},
		{"TTS no content", "tts", 204, `{"text":"hi"}`, "", 0},
		{"TTS upstream error", "tts", 503, `{"text":"hi"}`, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := extractDeepseekToolsUsage(tc.endpoint, []byte(tc.request), []byte(tc.reply), tc.status)
			require.Equal(t, tc.units, usage.Units)
			if tc.units > 0 {
				require.Equal(t, tc.endpoint, usage.Endpoint)
			}
		})
	}
}

func TestCountTTSBillableChars(t *testing.T) {
	for text, want := range map[string]int{
		"": 0, "A9 ": 3, "你好": 4, "𠀀": 2, "こんにちは": 5, "한글": 2,
		"。！，": 3, "Ａ１": 2, "🙂": 1, "你好，世界。Hello DeepSeek": 24,
	} {
		require.Equal(t, want, countTTSBillableChars(text), "%q", text)
	}
}

func TestDeepseekToolsPricing(t *testing.T) {
	key := &APIKey{User: &User{ID: 1}, GroupID: groupIDPtr(), Group: &Group{
		ID: *groupIDPtr(), Platform: PlatformDeepseek, RateMultiplier: 2, SearchPricePer1k: testPtrFloat64(999),
	}}
	billing := newTestBillingServiceForResolver()
	for _, tc := range []struct {
		endpoint       string
		units          int
		list, override float64
	}{
		{"search", 1, 0.75 / 1000, 0.03},
		{"fetch", 1, 0.75 / 1000, 0.04},
		{"ocr", 3, 0.225 / 1000, 0.05},
		{"asr", 4, 0.116 / 3600, 0.06},
		{"tts", 24, 0.45 / 10000, 0.07},
	} {
		u := DeepseekToolsUsage{Endpoint: tc.endpoint, Units: tc.units}
		for _, pricing := range []struct {
			name    string
			channel *float64
			units   int
			want    float64
		}{
			{"USD list", nil, u.Units, float64(u.Units) * tc.list},
			{"channel", &tc.override, u.Units, float64(u.Units) * tc.override},
			{"free channel", testPtrFloat64(0), u.Units, 0},
			{"zero units at list", nil, 0, 0},
			{"zero units with channel", &tc.override, 0, 0},
		} {
			t.Run(tc.endpoint+"/"+pricing.name, func(t *testing.T) {
				usage := DeepseekToolsUsage{Endpoint: tc.endpoint, Units: pricing.units}
				svc := &OpenAIGatewayService{billingService: billing}
				if pricing.channel != nil {
					svc.resolver = newResolverWithChannelForPlatform(t, PlatformDeepseek, []ChannelModelPricing{{
						Platform: PlatformDeepseek, Models: []string{usage.Model()},
						BillingMode: BillingModePerRequest, PerRequestPrice: pricing.channel,
					}})
				}
				result := &OpenAIForwardResult{Model: usage.Model(), DeepseekTool: &usage}
				cost, err := svc.calculateOpenAIRecordUsageCost(t.Context(), result, key, []string{result.Model},
					2, 2, 2, 2, UsageTokens{}, "", nil, false, time.Time{})
				require.NoError(t, err)
				require.Equal(t, string(BillingModePerRequest), cost.BillingMode)
				require.InDelta(t, pricing.want, cost.TotalCost, 1e-12)
				require.InDelta(t, 2*pricing.want, cost.ActualCost, 1e-12)
				amount, priced := svc.EstimateInflightReservation(t.Context(), key, InflightEstimateRequest{DeepseekTool: &usage})
				require.True(t, priced)
				require.InDelta(t, cost.ActualCost, amount, 1e-12)
			})
		}
	}
}

func TestGrokZeroTokenSearchUsesModelPricing(t *testing.T) {
	key := &APIKey{GroupID: groupIDPtr(), Group: &Group{ID: *groupIDPtr(), Platform: PlatformGrok}}
	result := &OpenAIForwardResult{Model: "grok-custom-search", SearchCount: 2}
	svc := &OpenAIGatewayService{billingService: newTestBillingServiceForResolver()}
	charge := func() (*CostBreakdown, error) {
		return svc.calculateOpenAIRecordUsageCost(t.Context(), result, key, []string{result.Model},
			1, 1, 1, 1, UsageTokens{}, "", nil, false, time.Time{})
	}
	svc.resolver = newResolverWithChannelForPlatform(t, PlatformGrok, []ChannelModelPricing{{
		Platform: PlatformGrok, Models: []string{result.Model}, BillingMode: BillingModePerRequest, PerRequestPrice: testPtrFloat64(0.25),
	}})
	cost, err := charge()
	require.NoError(t, err)
	require.InDelta(t, 0.25+2*5.0/1000, cost.ActualCost, 1e-12)
	result.Model = "unknown-unpriced-model"
	_, err = charge()
	require.ErrorIs(t, err, ErrModelPricingUnavailable)
}

func TestDeepseekToolsCompositeChannelPricing(t *testing.T) {
	usage := DeepseekToolsUsage{Endpoint: "tts", Units: 24}
	model := usage.Model()
	resolver := newResolverWithChannelForPlatform(t, PlatformComposite, []ChannelModelPricing{
		{Platform: PlatformOpenAI, Models: []string{model}, BillingMode: BillingModePerRequest, PerRequestPrice: testPtrFloat64(1)},
		{Platform: PlatformDeepseek, Models: []string{model}, BillingMode: BillingModePerRequest, PerRequestPrice: testPtrFloat64(0.002)},
	})
	key := &APIKey{User: &User{ID: 1}, GroupID: groupIDPtr(), Group: &Group{ID: *groupIDPtr(), Platform: PlatformComposite, RateMultiplier: 1}}
	gateway := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
	result := &OpenAIForwardResult{Model: model, DeepseekTool: &usage}
	cost, err := gateway.calculateOpenAIRecordUsageCost(t.Context(), result, key, []string{model}, 1, 1, 1, 1,
		UsageTokens{}, "", nil, false, time.Time{})
	require.NoError(t, err)
	require.InDelta(t, 0.048, cost.ActualCost, 1e-12)
	estimate, priced := gateway.EstimateInflightReservation(t.Context(), key, InflightEstimateRequest{DeepseekTool: &usage})
	require.True(t, priced)
	require.InDelta(t, cost.ActualCost, estimate, 1e-12)
}

func newDeepseekToolsAnthropicFilesAccount(baseURL string) *Account {
	return &Account{
		ID:          902,
		Name:        "deepseek-anthropic-files",
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-dstest",
			"api_protocol": "adaptive",
			"base_url":     "https://api.deepseek.com",
			"api_base_urls": map[string]any{
				"anthropic": baseURL,
			},
		},
	}
}

func TestForwardDeepseekToolsRequest_AnthropicFilesPassthrough(t *testing.T) {
	var gotPath, gotBeta, gotVersion, gotAuth string
	var gotBody []byte
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBeta = r.Header.Get("anthropic-beta")
		gotVersion = r.Header.Get("anthropic-version")
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"file-api-1","type":"file","size_bytes":70,"created_at":"2026-11-17T04:20:28.705+00:00","filename":"a.png","mime_type":"image/png"}`))
	})

	body := []byte("--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.png\"\r\nContent-Type: image/png\r\n\r\nPNG\r\n--b--\r\n")
	c, rec := newDeepseekToolsTestContext(http.MethodPost, "/v1/files", body)
	c.Request.Header.Set("anthropic-beta", "files-api-2025-04-14")
	c.Request.Header.Set("anthropic-version", "2023-06-01")
	cache := &testDeepseekFileCache{}
	writtenBytesAtBind := -1
	cache.onSave = func() { writtenBytesAtBind = rec.Body.Len() }
	account := newDeepseekToolsAnthropicFilesAccount("https://relay.test/anthropic")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}, cache: cache}
	result, err := forwardDeepseekToolsTest(svc, c, account, 7, "files_create", body, "multipart/form-data; boundary=b")
	require.NoError(t, err)
	require.Equal(t, "/anthropic/v1/files", gotPath, "anthropic-beta requests must go to the Anthropic compatible Files endpoint")
	require.Equal(t, "files-api-2025-04-14", gotBeta)
	require.Equal(t, "2023-06-01", gotVersion)
	require.Equal(t, "Bearer sk-dstest", gotAuth)
	require.Equal(t, body, gotBody)
	require.Equal(t, `{"id":"file-api-1","type":"file","size_bytes":70,"created_at":"2026-11-17T04:20:28.705+00:00","filename":"a.png","mime_type":"image/png"}`, rec.Body.String(), "Anthropic shaped response must pass through unchanged")
	require.Zero(t, result.Usage)

	require.Zero(t, writtenBytesAtBind)
	file := cache.files[7]["file-api-1"]
	require.Equal(t, account.ID, file.AccountID)
	require.Equal(t, "user_data", file.Purpose)
	require.Equal(t, "image/png", file.MIMEType)
	require.Equal(t, "anthropic", file.Format)
	require.Equal(t, int64(70), file.Bytes)
	require.Zero(t, file.ExpiresAt)
}

func TestForwardDeepseekToolsRequest_FilesWithoutBetaStaysOpenAI(t *testing.T) {
	var gotPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"file-api-2","object":"file","bytes":70,"created_at":1700000000,"expires_at":1700086400,"purpose":"user_data"}`))
	})

	body := []byte("--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.png\"\r\n\r\nPNG\r\n--b--\r\n")
	c, _ := newDeepseekToolsTestContext(http.MethodPost, "/v1/files", body)
	cache := &testDeepseekFileCache{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}, cache: cache}
	_, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 7, "files_create", body, "multipart/form-data; boundary=b")
	require.NoError(t, err)
	require.Equal(t, "/v1/files", gotPath, "requests without the Files beta header keep the OpenAI compatible path")
	file := cache.files[7]["file-api-2"]
	require.Equal(t, int64(901), file.AccountID)
	require.Equal(t, int64(1700086400), file.ExpiresAt)
	require.Equal(t, "openai", file.Format)
}
