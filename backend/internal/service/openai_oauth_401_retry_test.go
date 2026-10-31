//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIOAuth401HTTPRecovery(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		name := "forward"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			account := &Account{ID: 5542, Name: "synthetic-oauth", Platform: PlatformOpenAI,
				Type: AccountTypeOAuth, Status: StatusActive, Concurrency: 1,
				Credentials: map[string]any{"access_token": "rejected-at", "refresh_token": "old-rt",
					"expires_at": time.Now().Add(2 * time.Hour).Format(time.RFC3339)}}
			repo := &refreshAPIAccountRepo{account: account}
			cache := newOpenAITokenCacheStub()
			executor := &refreshAPIExecutorStub{needsRefresh: false, credentials: map[string]any{
				"access_token": "fresh-at", "refresh_token": "rotated-rt",
				"expires_at": time.Now().Add(2 * time.Hour).Format(time.RFC3339)}}
			provider := NewOpenAITokenProvider(repo, cache, nil)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				newOpenAIRejectedFieldTestResponse(401, `{"error":{"code":"invalid_api_key","message":"token rejected"}}`),
				newOpenAIRejectedFieldTestResponse(200, `{"id":"resp_ok","usage":{"input_tokens":1,"output_tokens":2}}`),
			}}
			if passthrough {
				upstream.responses[1].Header.Set("Content-Type", "text/event-stream")
				upstream.responses[1].Body = io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_ok\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n"))
			}
			svc := newOpenAIRejectedFieldTestService(upstream)
			svc.openAITokenProvider = provider
			body := []byte(`{"model":"gpt-5.6-sol","instructions":"synthetic test","stream":false,"input":[{"role":"user","content":"synthetic"}]}`)
			c := newOpenAIRejectedFieldTestContext(body)
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			var result *OpenAIForwardResult
			var err error
			if passthrough {
				result, err = svc.forwardOpenAIPassthrough(context.Background(), c, account, body, body, "gpt-5.6-sol", false, nil, false, time.Now())
			} else {
				result, err = svc.Forward(context.Background(), c, account, body)
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "Bearer rejected-at", upstream.requests[0].Header.Get("Authorization"))
			require.Equal(t, "Bearer fresh-at", upstream.requests[1].Header.Get("Authorization"))
			require.Equal(t, upstream.bodies[0], upstream.bodies[1], "auth recovery must not change the request payload")
			require.Equal(t, 1, executor.refreshCalls)
			require.Equal(t, "rotated-rt", repo.account.GetOpenAIRefreshToken())
		})
	}
}
