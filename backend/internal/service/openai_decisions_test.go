package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardDecisionsPreservesMultimodalRequestAndAnswers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"decision-alias","input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="},{"type":"input_json","data":{"total":42}}]}],"questions":[{"type":"predicate","name":"ok","instructions":[{"type":"input_text","text":"Is it OK?"},{"type":"input_image","image_url":"https://example.com/reference.png"}]}],"prompt_cache_key":"reference","prompt_cache_options":{"ttl":"30m"}}`)
	response := `{"model":"gpt-6-luna","answers":[{"type":"predicate","name":"ok","probability":0.9}],"usage":{"input_tokens":1000,"output_tokens":7,"input_tokens_details":{"cached_tokens":200,"cache_write_tokens":300}}}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/decisions", bytes.NewReader(body))
	c.Request.Header.Set("Authorization", "Bearer downstream-key")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}, "X-Request-Id": {"decision-1"}},
		Body: io.NopCloser(strings.NewReader(response)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "upstream-key", "base_url": "https://api.openai.com/v1",
		"openai_capabilities": []string{"decisions"}, "model_mapping": map[string]any{"decision-alias": "gpt-6-luna"},
	}}
	result, err := svc.ForwardDecisions(context.Background(), c, account, body, "")
	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/decisions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer upstream-key", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "gpt-6-luna", gjson.GetBytes(upstream.lastBody, "model").String())
	for _, field := range []string{"input", "questions", "prompt_cache_key", "prompt_cache_options"} {
		require.JSONEq(t, gjson.GetBytes(body, field).Raw, gjson.GetBytes(upstream.lastBody, field).Raw)
	}
	require.Equal(t, response, recorder.Body.String())
	require.Equal(t, "decision-1", result.RequestID)
	require.Equal(t, "/v1/decisions", result.UpstreamEndpoint)
	require.Equal(t, "/v1/decisions", GetActualOpenAIUpstreamEndpoint(c))
	require.Equal(t, 1000, result.Usage.InputTokens)
	require.Equal(t, 200, result.Usage.CacheReadInputTokens)
	require.Equal(t, 300, result.Usage.CacheCreationInputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
}

func TestDecisionsAccountCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name, platform, accountType string
		capabilities                any
		want                        bool
	}{
		{"unrestricted API key", PlatformOpenAI, AccountTypeAPIKey, nil, true},
		{"explicit decisions", PlatformOpenAI, AccountTypeAPIKey, []string{"decisions"}, true},
		{"explicit text only", PlatformOpenAI, AccountTypeAPIKey, []string{"chat_completions"}, false},
		{"OAuth", PlatformOpenAI, AccountTypeOAuth, []string{"decisions"}, false},
		{"other platform", PlatformGrok, AccountTypeAPIKey, []string{"decisions"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{Platform: tc.platform, Type: tc.accountType, Credentials: map[string]any{"openai_capabilities": tc.capabilities}}
			require.Equal(t, tc.want, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDecisions))
			if !tc.want {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				svc := &OpenAIGatewayService{}
				_, err := svc.ForwardDecisions(context.Background(), c, account, nil, "")
				require.Error(t, err) // Must reject before attempting any upstream call.
			}
		})
	}
}

func TestForwardDecisionsZeroInputDoesNotUseTotalTokens(t *testing.T) {
	response := `{"answers":[],"usage":{"input_tokens":0,"output_tokens":7,"total_tokens":7}}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/decisions", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response)),
	}}}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	result, err := svc.ForwardDecisions(context.Background(), c, account, []byte(`{"model":"gpt-6-luna","input":"","questions":[{"type":"predicate","instructions":"OK?"}]}`), "")
	require.NoError(t, err)
	require.Zero(t, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
}

func TestDecisionsSchedulerExcludesOAuthAndTextOnlyAccounts(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Priority: 0, Concurrency: 1},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Priority: 1, Concurrency: 1,
			Credentials: map[string]any{"openai_capabilities": []string{"chat_completions"}}},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Priority: 2, Concurrency: 1,
			Credentials: map[string]any{"openai_capabilities": []string{"decisions"}}},
	}
	svc := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, cache: &schedulerTestGatewayCache{},
		cfg: &config.Config{}, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	groupID := int64(1)
	selection, _, err := svc.SelectAccountWithSchedulerForCapability(context.Background(), &groupID, "", "", "gpt-6-luna", nil,
		OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityDecisions, false, false, true)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(3), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestDecisionsBillingKeepsInputAndCacheRatesButOutputIsFree(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)
	svc := &OpenAIGatewayService{billingService: billing}
	for _, tc := range []struct {
		endpoint      string
		output, write float64
	}{
		{"/v1/decisions", 0, 0.125e-6 * 300},
		{"/v1/responses", 0.5e-6 * 7, 0.125e-6 * 300},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			result := &OpenAIForwardResult{UpstreamEndpoint: tc.endpoint}
			cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), result, &APIKey{}, []string{"gpt-6-luna"},
				2, 1, 1, 1, UsageTokens{InputTokens: 500, CacheReadTokens: 200, CacheCreationTokens: 300, OutputTokens: 7}, "", nil, false, time.Time{})
			require.NoError(t, err)
			require.InDelta(t, 0.1e-6*500, cost.InputCost, 1e-12)
			require.InDelta(t, 0.01e-6*200, cost.CacheReadCost, 1e-12)
			require.InDelta(t, tc.write, cost.CacheCreationCost, 1e-12)
			require.InDelta(t, tc.output, cost.OutputCost, 1e-12)
			require.InDelta(t, 2*cost.TotalCost, cost.ActualCost, 1e-12)
		})
	}
}

func TestForwardDecisionsUpstreamErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		body := `{"model":"gpt-6-luna","input":"x","questions":[{"type":"predicate","name":"ok","instructions":"OK?"}]}`
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(body))
		response := `{"error":{"message":"upstream error"}}`
		upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
		_, err := svc.ForwardDecisions(context.Background(), c, account, []byte(body), "")
		require.Error(t, err)
		if status == http.StatusBadRequest {
			require.Equal(t, status, recorder.Code)
			require.Equal(t, response, recorder.Body.String())
		} else {
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.Equal(t, status, failover.StatusCode)
			require.False(t, c.Writer.Written())
		}
	}
}

func TestRecordDecisionsUsageSplitsCacheAndPreservesReportedOutput(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
	result := &OpenAIForwardResult{
		RequestID: "decision-billing", Model: "gpt-6-luna", UpstreamEndpoint: "/v1/decisions",
		Usage: OpenAIUsage{InputTokens: 1000, CacheReadInputTokens: 200, CacheCreationInputTokens: 300, OutputTokens: 7},
	}
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: result, APIKey: &APIKey{ID: 2}, User: &User{ID: 1}, Account: &Account{ID: 3, Type: AccountTypeAPIKey},
		InboundEndpoint: "/v1/decisions", UpstreamEndpoint: "/v1/decisions",
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 500, usageRepo.lastLog.InputTokens)
	require.Equal(t, 200, usageRepo.lastLog.CacheReadTokens)
	require.Equal(t, 300, usageRepo.lastLog.CacheCreationTokens)
	require.Equal(t, 7, usageRepo.lastLog.OutputTokens)
	require.Zero(t, usageRepo.lastLog.OutputCost)
	expected := 500*0.1e-6 + 200*0.01e-6 + 300*0.125e-6
	require.InDelta(t, expected, usageRepo.lastLog.TotalCost, 1e-12)
	require.InDelta(t, expected*1.1, userRepo.lastAmount, 1e-12)
	require.Equal(t, 1, userRepo.deductCalls)
}
