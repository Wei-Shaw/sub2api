//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func openDesignTestAccount() *Account {
	return &Account{
		ID: 702, Platform: PlatformOpenDesign, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-amr-test", "api_protocol": APIProtocolAdaptive},
	}
}

func TestOpenDesignGatewayRoutesTextToNativeEndpoints(t *testing.T) {
	for _, ingress := range routingMatrixIngresses() {
		t.Run(ingress.name, func(t *testing.T) {
			account := openDesignTestAccount()
			// 官方控制台复制的地址可能含 /v1；转发不能再追加一遍。
			account.Credentials["api_base_urls"] = map[string]any{
				APIProtocolAnthropic: "https://amr-link.open-design.ai/v1",
			}
			upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			const model = "deepseek-v4.1-flash"
			body := routingMatrixCase{ingress: ingress, model: model}.body()
			_ = ingress.forward(svc, adaptiveProtocolTestContext(ingress.path, body), account, body)

			require.NotEmpty(t, upstream.requests)
			req := upstream.requests[0]
			wantPath := ingress.path
			if ingress.responsesBody {
				wantPath = "/v1/responses"
			}
			require.Equal(t, "https://amr-link.open-design.ai"+wantPath, req.URL.String())
			if wantPath == "/v1/messages" {
				require.Equal(t, "sk-amr-test", req.Header.Get("x-api-key"))
			} else {
				require.Equal(t, "Bearer sk-amr-test", req.Header.Get("Authorization"))
			}
			require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
		})
	}
}

func TestOpenDesignAccountConnectionChecksThreeNativeEndpoints(t *testing.T) {
	account := openDesignTestAccount()
	account.Credentials["api_base_urls"] = map[string]any{
		APIProtocolAnthropic: "https://amr-link.open-design.ai/v1",
	}
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNChatTestResponse(),
		adaptiveCNAnthropicTestResponse(), adaptiveCNResponsesTestResponse())
	c, recorder := newTestContext()

	require.NoError(t, svc.TestAccountConnection(c, account.ID, "", "hi", AccountTestModeDefault))
	require.Len(t, upstream.requests, 3)
	for i, path := range []string{"/v1/chat/completions", "/v1/messages", "/v1/responses"} {
		require.Equal(t, "https://amr-link.open-design.ai"+path, upstream.requests[i].URL.String())
	}
	require.Equal(t, "deepseek-v4.1-flash", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"test_complete"`))
	require.Contains(t, recorder.Body.String(), `"success":true`)
}

func TestOpenDesignModelSyncUsesAccountEndpoint(t *testing.T) {
	for _, tc := range []struct{ name, protocol, base, wantURL string }{
		{"default", APIProtocolAdaptive, "", "https://amr-link.open-design.ai/v1/models"},
		{"custom_messages", APIProtocolAnthropic, "https://relay.example/v1", "https://relay.example/v1/models"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := openDesignTestAccount()
			account.Credentials["api_protocol"] = tc.protocol
			account.Credentials["base_url"] = tc.base
			svc, upstream := adaptiveCNAccountTestService(account, &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"deepseek-v4.1-flash"}]}`)),
			})
			models, err := svc.FetchUpstreamSupportedModels(context.Background(), account)
			require.NoError(t, err)
			require.Equal(t, []string{"deepseek-v4.1-flash"}, models)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, tc.wantURL, upstream.requests[0].URL.String())
			require.Equal(t, "Bearer sk-amr-test", upstream.requests[0].Header.Get("Authorization"))
		})
	}
}

func TestOpenDesignRejectsNonAPIKeyAccounts(t *testing.T) {
	_, err := buildAccountForCreate(&CreateAccountInput{Platform: PlatformOpenDesign, Type: AccountTypeOAuth}, nil)
	require.ErrorContains(t, err, "only support apikey")
}
