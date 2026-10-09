//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeUnsupportedModelsRejectMappedAliasesBeforeProtocolSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, ingress := range routingMatrixIngresses() {
		for _, protocol := range []string{APIProtocolAdaptive, APIProtocolResponses, APIProtocolChatCompletions, APIProtocolAnthropic} {
			for _, mode := range []string{AccountModeGo, AccountModeZen} {
				t.Run(ingress.name+"/"+protocol+"/"+mode, func(t *testing.T) {
					account := &Account{ID: 105, Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey,
						Credentials: map[string]any{
							"api_key": "test-key", "api_protocol": protocol, "account_mode": mode,
							"model_mapping":  map[string]any{"public-model": "gemini-3.8-flash"},
							"protocol_rules": []any{map[string]any{"pattern": "*", "protocol": APIProtocolChatCompletions}},
						}}
					body := (routingMatrixCase{ingress: ingress, model: "public-model"}).body()
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, ingress.path, bytes.NewReader(body))
					upstream := &httpUpstreamRecorder{err: errors.New("unexpected upstream request")}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

					err := ingress.forward(svc, c, account, body)

					require.ErrorContains(t, err, "opencode unsupported model: gemini-3.8-flash")
					require.Equal(t, http.StatusBadRequest, recorder.Code)
					require.Empty(t, upstream.requests)
				})
			}
		}
	}
}

func TestCommandCodeGeminiModelReachesUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, ingress := range routingMatrixIngresses() {
		t.Run(ingress.name, func(t *testing.T) {
			account := commandCodeTestAccount(106)
			account.Credentials["api_protocol"] = APIProtocolChatCompletions
			body := (routingMatrixCase{ingress: ingress, model: "gemini-3.8-flash"}).body()
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, ingress.path, bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			err := ingress.forward(svc, c, account, body)

			require.Error(t, err)
			require.NotContains(t, err.Error(), "opencode unsupported model")
			require.Len(t, upstream.requests, 1)
			require.Equal(t, "https://api.commandcode.ai/provider/v1/chat/completions", upstream.requests[0].URL.String())
		})
	}
	account := commandCodeTestAccount(107)
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNChatTestResponse())
	c, _ := newTestContext()
	require.NoError(t, svc.testModelRoutedAccountConnection(c, account, "gemini-3.8-flash", "hi"))
	require.Len(t, upstream.requests, 1)
}

func TestOpenCodeConnectionTestRejectsMappedUnsupportedModel(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_protocol":  APIProtocolChatCompletions,
			"model_mapping": map[string]any{"public-model": "jev-1.13"},
		}}
	c, _ := newTestContext()
	c.Request = httptest.NewRequest(http.MethodPost, "/test", nil).WithContext(context.Background())
	err := (&AccountTestService{}).testModelRoutedAccountConnection(c, account, "public-model", "hi")
	require.ErrorContains(t, err, "not supported on OpenCode standard gateway")
	require.ErrorContains(t, err, "jev-1.13")
}
