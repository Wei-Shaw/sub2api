package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func excelAccount() *Account {
	return &Account{ID: 300, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 10,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}, Extra: map[string]any{"openai_excel_bps": true}}
}

func excelBPSTestContext(body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	return c, rec
}

func TestExcelBPSAccountGating(t *testing.T) {
	account := excelAccount()
	require.True(t, account.IsExcelBPSEnabledForModel("gpt-6-sol"), "no model list routes every model")

	account.Extra[ExcelBPSModelsKey] = []any{"gpt-6-sol"}
	require.True(t, account.IsExcelBPSEnabledForModel("gpt-6-sol"))
	require.False(t, account.IsExcelBPSEnabledForModel("gpt-6.1-sol"))
	require.False(t, account.isExcelBPSAllModelsEnabled())

	free := excelAccount()
	free.Credentials["plan_type"] = " Free "
	require.False(t, free.IsExcelBPSEnabled(), "free plans never use BPS")

	apiKey := excelAccount()
	apiKey.Type = AccountTypeAPIKey
	require.False(t, apiKey.IsExcelBPSEnabled())

	off := excelAccount()
	off.Extra[ExcelBPSEnabledKey] = false
	require.False(t, off.IsExcelBPSEnabledForModel("gpt-6-sol"))
	require.False(t, off.IsExcelBPSAutoDisableOn403Enabled())

	all := excelAccount()
	all.Extra["openai_oauth_responses_websockets_v2_enabled"] = true
	require.False(t, all.IsOpenAIResponsesWebSocketV2Enabled(), "all-model BPS is HTTP/SSE only")
}

func TestExcelBPSAccountID(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "jwt-account"}})
	require.NoError(t, err)
	token := "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"

	account := excelAccount()
	require.Equal(t, "test-account", excelBPSAccountID(account, token))
	delete(account.Credentials, "chatgpt_account_id")
	require.Equal(t, "jwt-account", excelBPSAccountID(account, token))
	require.Empty(t, excelBPSAccountID(account, "not-a-jwt"))
}

func TestExcelBPSForwardContract(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint("stream=", stream), func(t *testing.T) {
			wire := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_excel\",\"status\":\"completed\",\"model\":\"gpt-5.6-sol\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"21\"}]}],\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
			svc := openAIClientToolsTestService(upstream)
			body := []byte(fmt.Sprintf(`{"model":"gpt-5.6-sol","stream":%v,"reasoning":{"effort":"max"},"input":"test","tools":[{"type":"web_search"}]}`, stream))
			c, rec := excelBPSTestContext(body)
			c.Request.Header.Set("x-codex-turn-state", "must-not-leak")
			account := excelAccount()
			account.Proxy = &Proxy{Protocol: "http", Host: "127.0.0.1", Port: 7890}

			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, account.Proxy.URL(), upstream.lastProxyURL)
			require.Equal(t, basispoints.ResponsesURL, upstream.lastReq.URL.String())
			require.Equal(t, "Bearer test-token", upstream.lastReq.Header.Get("Authorization"))
			require.Equal(t, "test-account", upstream.lastReq.Header.Get("Chatgpt-Account-Id"))
			require.Empty(t, upstream.lastReq.Header.Get("x-codex-turn-state"))
			require.Equal(t, HTTPUpstreamProfileLongStream, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
			require.True(t, HTTPUpstreamRedirectsDisabled(upstream.lastReq.Context()))
			require.False(t, gjson.GetBytes(upstream.lastBody, "tools").Exists(), "hosted tools are omitted")
			require.Equal(t, "xhigh", gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
			require.Equal(t, "xhigh", *result.ReasoningEffort)
			require.Equal(t, excelBPSUpstreamEndpoint, result.UpstreamEndpoint)
			require.Equal(t, 10, result.Usage.InputTokens)
			require.Equal(t, 2, result.Usage.OutputTokens)
			require.Contains(t, rec.Body.String(), "21")
		})
	}
}

func TestExcelBPSModelOutsideListStaysOnCodex(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_1","status":"completed","output":[]}`))}}
	svc := openAIClientToolsTestService(upstream)
	body := []byte(`{"model":"gpt-6.1-sol","stream":false,"input":"test"}`)
	c, _ := excelBPSTestContext(body)
	account := excelAccount()
	account.Extra[ExcelBPSModelsKey] = []any{"gpt-6-sol"}

	_, _ = svc.Forward(context.Background(), c, account, body)
	require.NotNil(t, upstream.lastReq)
	require.NotEqual(t, "bps.openai.com", upstream.lastReq.URL.Host)
}

func TestExcelBPS429FailsOverAndCoolsDownOnlyBPSModels(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"120"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"slow down"}}`))}}
	svc := openAIClientToolsTestService(upstream)
	body := []byte(`{"model":"gpt-6-sol","stream":true,"input":"test"}`)
	c, rec := excelBPSTestContext(body)
	account := excelAccount()
	account.Extra[ExcelBPSModelsKey] = []any{"gpt-6-sol"}

	_, err := svc.Forward(context.Background(), c, account, body)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, ExcelBPSRateLimitedReason, failoverErr.Reason)
	require.Equal(t, "120", failoverErr.ResponseHeaders.Get("Retry-After"))
	require.False(t, failoverErr.ShouldReportAccountScheduleFailure())
	require.Empty(t, rec.Body.String(), "no output before failover")

	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-6-sol"))
	require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-6.1-sol"), "native models keep scheduling")
}

type excelBPSDisableRepoStub struct {
	AccountRepository
	disabled []int64
}

func (r *excelBPSDisableRepoStub) DisableExcelBPSOn403(_ context.Context, account *Account) (bool, error) {
	r.disabled = append(r.disabled, account.ID)
	return true, nil
}

func TestExcelBPS403AutoDisable(t *testing.T) {
	for _, optIn := range []bool{false, true} {
		t.Run(fmt.Sprint("opt_in=", optIn), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"forbidden"}}`))}}
			repo := &excelBPSDisableRepoStub{}
			svc := openAIClientToolsTestService(upstream)
			svc.accountRepo = repo
			body := []byte(`{"model":"gpt-6-sol","stream":false,"input":"test"}`)
			c, rec := excelBPSTestContext(body)
			account := excelAccount()
			account.Extra[ExcelBPSAutoDisableOn403Key] = optIn

			_, err := svc.Forward(context.Background(), c, account, body)
			require.Error(t, err)
			require.Equal(t, http.StatusForbidden, rec.Code)
			if optIn {
				require.Equal(t, []int64{account.ID}, repo.disabled)
				require.Contains(t, rec.Body.String(), "automatically disabled")
			} else {
				require.Empty(t, repo.disabled)
			}
		})
	}
}

func TestExcelBPSCacheCreationAsInputDownstreamUsage(t *testing.T) {
	payload := []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":1000,"input_tokens_details":{"cached_tokens":100,"cache_write_tokens":200}}}}`)
	out, err := excelBPSDownstreamUsage(payload)
	require.NoError(t, err)
	require.Equal(t, int64(0), gjson.GetBytes(out, "response.usage.input_tokens_details.cache_write_tokens").Int())
	require.Equal(t, int64(1000), gjson.GetBytes(out, "response.usage.input_tokens").Int())
	require.Equal(t, int64(100), gjson.GetBytes(out, "response.usage.input_tokens_details.cached_tokens").Int())
}
