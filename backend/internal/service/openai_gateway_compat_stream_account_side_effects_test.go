package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// compatStreamSideEffectsRepo records the account-health writes made by the rate limit service.
type compatStreamSideEffectsRepo struct {
	stubOpenAIAccountRepo
	rateLimitCalls []time.Time
	errorCalls     []string
}

func (r *compatStreamSideEffectsRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitCalls = append(r.rateLimitCalls, resetAt)
	return nil
}

func (r *compatStreamSideEffectsRepo) SetError(_ context.Context, _ int64, msg string) error {
	r.errorCalls = append(r.errorCalls, msg)
	return nil
}

func compatStreamSideEffectsAccount() Account {
	return Account{
		ID:          601,
		Name:        "openai-apikey-compat-stream",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
	}
}

// compatStreamSideEffectsSSE builds an upstream responses stream that fails with errorEvent.
// With outputFirst the error follows response.created and a text delta, so client output has
// started (in practice response.created alone already starts it on both compat paths); without,
// the error is the first event and the request can still fail over.
func compatStreamSideEffectsSSE(outputFirst bool, errorEvent string) string {
	var lines []string
	if outputFirst {
		lines = append(lines,
			`data: {"type":"response.created","response":{"id":"resp_side","model":"gpt-5.5","status":"in_progress","output":[]}}`,
			"",
			`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"partial"}`,
			"",
		)
	}
	lines = append(lines, `event: error`, `data: `+errorEvent, "")
	return strings.Join(lines, "\n")
}

const compatStreamAuthErrorEvent = `{"type":"error","status":401,"error":{"code":"invalid_api_key","type":"invalid_request_error","message":"Incorrect API key provided"}}`

func newCompatStreamSideEffectsService(sse string) (*OpenAIGatewayService, *compatStreamSideEffectsRepo, *Account) {
	account := compatStreamSideEffectsAccount()
	repo := &compatStreamSideEffectsRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
	svc := &OpenAIGatewayService{
		accountRepo:      repo,
		rateLimitService: &RateLimitService{accountRepo: repo},
		httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_side"}},
			Body:       io.NopCloser(strings.NewReader(sse)),
		}},
	}
	return svc, repo, &account
}

func newCompatStreamContext(path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

// A terminal stream error that arrives after client output started can no longer fail over, but the
// account health still has to be updated — otherwise the account stays schedulable and every new
// request routed to it fails the same way.
func TestForwardAsChatCompletions_StreamErrorAfterOutputAppliesAccountSideEffects(t *testing.T) {
	svc, repo, account := newCompatStreamSideEffectsService(compatStreamSideEffectsSSE(true, compatStreamAuthErrorEvent))
	body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	c, rec := newCompatStreamContext("/v1/chat/completions", body)

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.5")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "output already started: must not fail over")
	require.Contains(t, rec.Body.String(), "partial", "client output must have started before the error")
	require.Len(t, repo.errorCalls, 1, "auth failure after output must mark the account exactly once")
	require.Empty(t, repo.rateLimitCalls)
}

func TestForwardAsAnthropic_StreamErrorAfterOutputAppliesAccountSideEffects(t *testing.T) {
	svc, repo, account := newCompatStreamSideEffectsService(compatStreamSideEffectsSSE(true, compatStreamAuthErrorEvent))
	body := []byte(`{"model":"gpt-5.5","max_tokens":1024,"messages":[{"role":"user","content":"hi"}],"stream":true}`)
	c, rec := newCompatStreamContext("/v1/messages", body)

	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.5")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "output already started: must not fail over")
	require.Contains(t, rec.Body.String(), "partial", "client output must have started before the error")
	require.Len(t, repo.errorCalls, 1, "auth failure after output must mark the account exactly once")
	require.Empty(t, repo.rateLimitCalls)
}

// Before client output the error fails over; the failover constructor already applies the side
// effects, so the account is marked once — not once per path.
func TestForwardAsChatCompletions_StreamErrorBeforeOutputCountsOnce(t *testing.T) {
	svc, repo, account := newCompatStreamSideEffectsService(compatStreamSideEffectsSSE(false, compatStreamAuthErrorEvent))
	body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	c, _ := newCompatStreamContext("/v1/chat/completions", body)

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.5")
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "no output yet: must fail over, got %v", err)
	require.Len(t, repo.errorCalls, 1, "failover path must not double count")
}

func TestForwardAsAnthropic_StreamErrorBeforeOutputCountsOnce(t *testing.T) {
	svc, repo, account := newCompatStreamSideEffectsService(compatStreamSideEffectsSSE(false, compatStreamAuthErrorEvent))
	body := []byte(`{"model":"gpt-5.5","max_tokens":1024,"messages":[{"role":"user","content":"hi"}],"stream":true}`)
	c, _ := newCompatStreamContext("/v1/messages", body)

	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.5")
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "no output yet: must fail over, got %v", err)
	require.Len(t, repo.errorCalls, 1, "failover path must not double count")
}
