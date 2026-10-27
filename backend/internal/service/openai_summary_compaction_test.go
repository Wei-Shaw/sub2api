//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const summaryTestResponse = `{"id":"resp_summary","status":"completed","model":"glm-5.3","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Project code COPPER-731. Next: run the tests."}]}],"usage":{"input_tokens":23,"output_tokens":12,"total_tokens":35}}`
const summaryTestRequest = `{"model":"gpt-alias","stream":true,"input":[{"type":"message","role":"user","content":"Project code COPPER-731"},{"type":"function_call","call_id":"c1","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"c1","output":"test result: success"},{"type":"compaction_trigger"}],"tools":[{"type":"function","name":"read"}]}`

func summaryTestService(response string) (*OpenAIGatewayService, *httpUpstreamRecorder) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}}
	return &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{JWT: config.JWTConfig{Secret: "summary-test-secret"}}}, upstream
}

func summaryTestAccount() *Account {
	return &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "test-only", "base_url": "https://example.com/v1", "model_mapping": map[string]any{"gpt-alias": "glm-5.3"}},
		Extra:       map[string]any{OpenAICompactStrategyKey: "summary"}}
}

func summaryTestContext(body []byte, userID int64) (*gin.Context, *httptest.ResponseRecorder, context.Context) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), ctxkey.UserID, userID)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body)).WithContext(ctx)
	return c, recorder, ctx
}

func TestSummaryCompactionAccountPolicy(t *testing.T) {
	for _, strategy := range []string{"", "inherit", "summary", "unknown"} {
		a := summaryTestAccount()
		a.Extra[OpenAICompactStrategyKey] = strategy
		require.Equal(t, strategy == "summary", a.UsesOpenAISummaryCompaction())
		if strategy != "" {
			err := ValidateOpenAICompactStrategy(a.Platform, a.Type, a.Extra)
			require.Equal(t, strategy == "unknown", err != nil)
		}
	}
	a := summaryTestAccount()
	a.Type = AccountTypeOAuth
	require.False(t, a.UsesOpenAISummaryCompaction())
	require.Error(t, ValidateOpenAICompactStrategy(a.Platform, a.Type, a.Extra))
	require.NoError(t, ValidateOpenAICompactStrategy(PlatformOpenAI, AccountTypeAPIKey, nil))
}

func TestSummaryCompactionForwardAndReplayAcrossAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mapped := range []bool{false, true} {
		t.Run(map[bool]string{false: "follow_current_model", true: "compact_override"}[mapped], func(t *testing.T) {
			svc, upstream := summaryTestService(summaryTestResponse)
			svc.cfg.Gateway.OpenAICompactModel = "global-compact-fallback"
			account := summaryTestAccount()
			wantModel := "glm-5.3"
			if mapped {
				account.Credentials["compact_model_mapping"] = map[string]any{"gpt-alias": "summary-model"}
				wantModel = "summary-model"
			}
			body := []byte(summaryTestRequest)
			c, rec, ctx := summaryTestContext(body, 42)
			MarkOpenAINativeCompactionV2(c)
			result, err := svc.Forward(ctx, c, account, body)
			require.NoError(t, err)
			require.Equal(t, wantModel, result.UpstreamModel)
			require.True(t, result.Stream)
			require.Equal(t, 23, result.Usage.InputTokens)
			require.Equal(t, wantModel, gjson.GetBytes(upstream.lastBody, "model").String())
			require.False(t, HasCompactionTriggerInInput(upstream.lastBody))
			require.False(t, gjson.GetBytes(upstream.lastBody, "tools").Exists())
			require.Contains(t, string(upstream.lastBody), "test result: success")
			require.Contains(t, rec.Body.String(), "response.output_item.done")
			final, ok := extractCodexFinalResponse(rec.Body.String())
			require.True(t, ok)
			require.Equal(t, int64(1), gjson.GetBytes(final, "output.#").Int())
			item := gjson.GetBytes(final, "output.0").Raw
			encrypted := gjson.Get(item, "encrypted_content").String()
			require.True(t, strings.HasPrefix(encrypted, summaryCompactEnvelopePrefix))
			require.NotContains(t, encrypted, "COPPER")

			// 新账号保持 inherit，仍需恢复同一用户已有的摘要；普通请求不再压缩。
			next := []byte(`{"model":"gpt-alias","stream":false,"input":[` + item + `,{"role":"user","content":"Continue"}]}`)
			account.ID = 10
			account.Extra = nil
			upstream.resp.Body = io.NopCloser(strings.NewReader(summaryTestResponse))
			c, _, ctx = summaryTestContext(next, 42)
			_, err = svc.Forward(ctx, c, account, next)
			require.NoError(t, err)
			require.Contains(t, string(upstream.lastBody), "COPPER-731")
			require.NotContains(t, string(upstream.lastBody), summaryCompactEnvelopePrefix)
			require.Equal(t, "glm-5.3", gjson.GetBytes(upstream.lastBody, "model").String())
		})
	}
}

func TestSummaryCompactionDoesNotChangeInheritOrOrdinaryRequests(t *testing.T) {
	a := summaryTestAccount()
	a.Extra = nil
	c, _, _ := summaryTestContext([]byte(summaryTestRequest), 42)
	require.False(t, shouldSynthesizeSummaryRemoteCompactionRequest(c, a, []byte(summaryTestRequest)))
	a.Extra = map[string]any{OpenAICompactStrategyKey: "summary"}
	require.False(t, shouldSynthesizeSummaryRemoteCompactionRequest(c, a, []byte(`{"model":"gpt-alias","input":"hello"}`)))
}

func TestSummaryCompactionEncryptedStateBoundToUserAndPreservesForeignState(t *testing.T) {
	svc, _ := summaryTestService(summaryTestResponse)
	ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
	state, err := svc.sealSummaryCompactCheckpoint(ctx, "Remember COPPER-731")
	require.NoError(t, err)
	_, err = svc.openSummaryCompactCheckpoint(context.WithValue(ctx, ctxkey.UserID, int64(43)), state)
	require.ErrorIs(t, err, ErrSummaryCompactInvalidEncryptedContent)
	_, err = svc.openSummaryCompactCheckpoint(ctx, state[:len(state)-8]+"AAAAAAAA")
	require.ErrorIs(t, err, ErrSummaryCompactInvalidEncryptedContent)
	body := []byte(`{"input":[{"type":"compaction","encrypted_content":"foreign-opaque"}]}`)
	restored, changed, err := svc.RestoreSummaryCompactInput(ctx, body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, restored)
	_, err = summaryCompactResponsesRequest([]byte(`{"input":[{"type":"compaction","encrypted_content":"foreign-opaque"},{"type":"compaction_trigger"}]}`), "glm-5.3")
	require.Error(t, err)
}

func TestSummaryCompactionRejectsIncompleteHistory(t *testing.T) {
	for _, input := range []string{
		`[{"type":"compaction_trigger"}]`,
		`[{"type":"function_call","call_id":"c1","name":"read"},{"type":"compaction_trigger"}]`,
		`[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AA"}]},{"type":"compaction_trigger"}]`,
	} {
		_, err := summaryCompactResponsesRequest([]byte(`{"input":`+input+`}`), "glm-5.3")
		require.Error(t, err)
	}
}

func TestSummaryCompactionSSETerminalValidation(t *testing.T) {
	svc, _ := summaryTestService(summaryTestResponse)
	c, _, _ := summaryTestContext(nil, 42)
	good := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + summaryTestResponse + "}\n\n"
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"completed", good, true},
		{"volcengine_done_marker", good + "data: [DONE]\n\n", true},
		{"early_done_marker", "data: [DONE]\n\n" + good, false},
		{"duplicate_done_marker", good + "data: [DONE]\n\ndata: [DONE]\n\n", false},
		{"truncated", strings.TrimSuffix(good, "\n\n"), false},
		{"duplicate", good + good, false},
		{"no_compaction_summary", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n", false},
		{"upstream_failed", strings.ReplaceAll(good, "response.completed", "response.failed"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{Body: io.NopCloser(strings.NewReader(tc.body))}
			_, err := svc.readSummaryCompactResponsesStream(c, resp, time.Now())
			require.Equal(t, tc.valid, err == nil)
		})
	}
}

func TestSummaryCompactionFailureAfterKeepaliveIsAnSSEFailure(t *testing.T) {
	svc, _ := summaryTestService(`{"status":"incomplete","output":[]}`)
	c, rec, ctx := summaryTestContext([]byte(summaryTestRequest), 42)
	MarkOpenAICompactClientStream(c)
	stop := StartOpenAICompactSSEKeepalive(c, time.Millisecond)
	defer stop()
	require.Eventually(t, func() bool { return c.Writer.Written() }, time.Second, time.Millisecond)
	_, err := svc.Forward(ctx, c, summaryTestAccount(), []byte(summaryTestRequest))
	require.Error(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "event: response.failed")
	require.NotContains(t, rec.Body.String(), "event: response.completed")
}

func TestSummaryCompactionChatUpstream(t *testing.T) {
	cc := `{"id":"cc1","model":"glm-5.3","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Remember COPPER-731"}}],"usage":{"prompt_tokens":20,"completion_tokens":8,"total_tokens":28}}`
	svc, upstream := summaryTestService(cc)
	a := summaryTestAccount()
	a.Extra["openai_responses_mode"] = "force_chat_completions"
	c, rec, ctx := summaryTestContext([]byte(summaryTestRequest), 42)
	result, err := svc.Forward(ctx, c, a, []byte(summaryTestRequest))
	require.NoError(t, err)
	require.Equal(t, "/v1/chat/completions", result.UpstreamEndpoint)
	require.True(t, gjson.GetBytes(upstream.lastBody, "messages").IsArray())
	final, ok := extractCodexFinalResponse(rec.Body.String())
	require.True(t, ok)
	var response map[string]any
	require.NoError(t, json.Unmarshal(final, &response))
	require.Equal(t, "compaction", gjson.GetBytes(final, "output.0.type").String())
}

func TestSummaryCompactionCapabilityAndMappingAreScopedToTheRequest(t *testing.T) {
	a := summaryTestAccount()
	a.Extra["openai_responses_mode"] = "force_chat_completions"
	a.Credentials["compact_model_mapping"] = map[string]any{"gpt-alias": "summary-model"}
	ctx := context.Background()
	require.Equal(t, "capability_mismatch", openAICompatibleAccountEligibilityFailureReasonBeforeProfit(ctx, a, PlatformOpenAI, "gpt-alias", false, OpenAIEndpointCapabilityResponses))
	require.Empty(t, openAICompatibleAccountEligibilityFailureReasonBeforeProfit(WithOpenAISummaryCompactionRequest(ctx), a, PlatformOpenAI, "gpt-alias", false, OpenAIEndpointCapabilityResponses))
	require.Equal(t, "summary-model", resolveOpenAIAccountUpstreamModelForRequest(a, "gpt-alias", true))
	require.Equal(t, "glm-5.3", resolveOpenAIAccountUpstreamModelForRequest(a, "gpt-alias", false))
	a.Extra[OpenAICompactStrategyKey] = "inherit"
	require.Equal(t, "capability_mismatch", openAICompatibleAccountEligibilityFailureReasonBeforeProfit(WithOpenAISummaryCompactionRequest(ctx), a, PlatformOpenAI, "gpt-alias", false, OpenAIEndpointCapabilityResponses))
}
