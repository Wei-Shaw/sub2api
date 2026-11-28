//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// loopbackHTTPUpstream sends upstream requests over a real HTTP client to a
// local test server.
type loopbackHTTPUpstream struct{ client *http.Client }

func (u loopbackHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.client.Do(req)
}

func (u loopbackHTTPUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.client.Do(req)
}

const upstreamWaitTestResponsesStream = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_slow\",\"model\":\"gpt-5.4\"}}\n\n" +
	"data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n" +
	"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_slow\",\"model\":\"gpt-5.4\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"pong\"}]}],\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n"

// slowHeadersUpstream holds the response headers back for delay, then answers
// with status (a Responses stream for 200, a JSON error otherwise).
func slowHeadersUpstream(t *testing.T, delay time.Duration, status int) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	paths := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		if status != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"type":"server_error","message":"upstream overloaded"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(upstreamWaitTestResponsesStream))
	}))
	t.Cleanup(srv.Close)
	return srv, &paths
}

func upstreamWaitTestService() *OpenAIGatewayService {
	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway = config.GatewayConfig{StreamKeepaliveInterval: 1, MaxLineSize: defaultMaxLineSize}
	return &OpenAIGatewayService{
		cfg:                  cfg,
		httpUpstream:         loopbackHTTPUpstream{client: &http.Client{}},
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
	}
}

func upstreamWaitTestAccount(baseURL string) *Account {
	return &Account{
		ID:          201,
		Name:        "slow-relay",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": baseURL + "/v1"},
		Extra:       map[string]any{"openai_responses_supported": true},
	}
}

func upstreamWaitTestContext(path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

func messagesBody(stream bool) []byte {
	if stream {
		return []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"ping"}],"stream":true}`)
	}
	return []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"ping"}],"stream":false}`)
}

func TestForwardAsAnthropicPingsStreamingClientWhileUpstreamHeadersAreLate(t *testing.T) {
	srv, paths := slowHeadersUpstream(t, 2500*time.Millisecond, http.StatusOK)
	svc := upstreamWaitTestService()
	c, rec := upstreamWaitTestContext("/v1/messages", messagesBody(true))

	result, err := svc.ForwardAsAnthropic(context.Background(), c, upstreamWaitTestAccount(srv.URL), messagesBody(true), "", "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, []string{"/v1/responses"}, *paths)
	body := rec.Body.String()
	pings := anthropicUpstreamWaitPing + anthropicUpstreamWaitPing
	require.True(t, strings.HasPrefix(body, pings), "two pings precede the upstream answer, got %q", body)
	require.Greater(t, strings.Index(body, "message_start"), len(pings)-1)
	require.Contains(t, body, "pong")
	require.Equal(t, "text/event-stream", rec.Result().Header.Get("Content-Type"))
}

// After pings committed the response, an upstream error before any output must
// still leave the response unwritten in the failover sense, attempt after attempt.
func TestForwardAsAnthropicLateUpstreamErrorAfterPingsStillFailsOver(t *testing.T) {
	srv, _ := slowHeadersUpstream(t, 1500*time.Millisecond, http.StatusBadGateway)
	svc := upstreamWaitTestService()
	c, rec := upstreamWaitTestContext("/v1/messages", messagesBody(true))
	before := OpenAICompactKeepaliveAdjustedWrittenSize(c)

	for attempt := 1; attempt <= 2; attempt++ {
		_, err := svc.ForwardAsAnthropic(context.Background(), c, upstreamWaitTestAccount(srv.URL), messagesBody(true), "", "")
		var failoverErr *UpstreamFailoverError
		require.True(t, errors.As(err, &failoverErr), "attempt %d: want failover error, got %v", attempt, err)
		require.Equal(t, before, OpenAICompactKeepaliveAdjustedWrittenSize(c), "attempt %d: pings are not client output", attempt)
		require.Equal(t, strings.Repeat(anthropicUpstreamWaitPing, attempt), rec.Body.String())
	}
}

func TestForwardAsAnthropicNonStreamingClientGetsNoPings(t *testing.T) {
	srv, _ := slowHeadersUpstream(t, 1500*time.Millisecond, http.StatusOK)
	svc := upstreamWaitTestService()
	c, rec := upstreamWaitTestContext("/v1/messages", messagesBody(false))

	_, err := svc.ForwardAsAnthropic(context.Background(), c, upstreamWaitTestAccount(srv.URL), messagesBody(false), "", "")

	require.NoError(t, err)
	require.True(t, strings.HasPrefix(rec.Body.String(), "{"), "non-streaming body is plain JSON, got %q", rec.Body.String())
	require.Contains(t, rec.Body.String(), "pong")
}

func TestForwardResponsesPingsStreamingClientWhileUpstreamHeadersAreLate(t *testing.T) {
	srv, paths := slowHeadersUpstream(t, 1500*time.Millisecond, http.StatusOK)
	svc := upstreamWaitTestService()
	body := []byte(`{"model":"gpt-5.4","input":"ping","stream":true}`)
	c, rec := upstreamWaitTestContext("/v1/responses", body)

	result, err := svc.Forward(context.Background(), c, upstreamWaitTestAccount(srv.URL), body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, []string{"/v1/responses"}, *paths)
	out := rec.Body.String()
	require.True(t, strings.HasPrefix(out, openAIResponsesUpstreamWaitPing), "a comment ping precedes the upstream answer, got %q", out)
	require.Contains(t, out, "response.completed")
}
