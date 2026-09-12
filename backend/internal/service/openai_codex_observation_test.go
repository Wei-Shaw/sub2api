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
)

func codexObservationTestHeaders() http.Header {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Codex-Primary-Used-Percent", "100")
	headers.Set("X-Codex-Primary-Window-Minutes", "300")
	headers.Set("X-Codex-Primary-Reset-After-Seconds", "600")
	return headers
}

type codexObservationDelayedBody struct {
	io.Reader
	delay time.Duration
}

func (r *codexObservationDelayedBody) Read(b []byte) (int, error) {
	if r.delay > 0 {
		time.Sleep(r.delay)
		r.delay = 0
	}
	return r.Reader.Read(b)
}

func (r *codexObservationDelayedBody) Close() error { return nil }

func waitCodexObservationWrites(t *testing.T, svc *OpenAIGatewayService) {
	t.Helper()
	require.Eventually(t, func() bool {
		svc.codexObservationGate.mu.Lock()
		defer svc.codexObservationGate.mu.Unlock()
		return len(svc.codexObservationGate.queues) == 0
	}, 2*time.Second, time.Millisecond)
}

func TestAlphaSearchCapturesQuotaOnceBeforeSlowBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 4)}
	response := &http.Response{StatusCode: http.StatusOK, Header: codexObservationTestHeaders(), Body: &codexObservationDelayedBody{
		Reader: strings.NewReader(`{"output":"result"}`), delay: 2100 * time.Millisecond,
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, accountRepo: repo, httpUpstream: &quotaHeaderUpstream{response: response}}
	t.Cleanup(svc.codexObservationGate.close)
	account := &Account{ID: 901, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"access_token": "oauth-token", "chatgpt_account_id": "account-id",
	}}
	body := []byte(`{"model":"gpt-5.5","commands":{"search_query":[{"q":"news"}]}}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", bytes.NewReader(body))
	result, err := svc.ForwardAlphaSearch(context.Background(), c, account, body)
	require.NoError(t, err)
	require.True(t, result.CodexUsageCaptured)
	waitCodexObservationWrites(t, svc)
	select {
	case update := <-repo.updateExtraCalls:
		observedAt, err := time.Parse(time.RFC3339Nano, update["codex_usage_updated_at"].(string))
		require.NoError(t, err)
		require.Greater(t, time.Since(observedAt), 2*time.Second)
		require.Equal(t, observedAt.Add(600*time.Second).UTC().Format(time.RFC3339), update["codex_5h_reset_at"])
	case <-time.After(time.Second):
		t.Fatal("missing initial header observation")
	}
	select {
	case update := <-repo.updateExtraCalls:
		t.Fatalf("body completion must not produce a second, drifted observation: %v", update)
	default:
	}
}

type captured429ObservationRepo struct {
	snapshotUpdateAccountRepo
	resetAt time.Time
}

func (r *captured429ObservationRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.resetAt = resetAt
	return nil
}

func TestCapturedHTTP429UsesOriginalObservationAndOrderedWriter(t *testing.T) {
	repo := &captured429ObservationRepo{snapshotUpdateAccountRepo: snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 4)}}
	account := &Account{ID: 902, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	headers := codexObservationTestHeaders()
	wireHeaders := headers.Clone()
	response := &http.Response{StatusCode: http.StatusTooManyRequests, Header: headers}
	svc := &OpenAIGatewayService{accountRepo: repo, httpUpstream: &quotaHeaderUpstream{response: response}}
	t.Cleanup(svc.codexObservationGate.close)
	request := httptest.NewRequest(http.MethodPost, "https://example.test/responses", nil)
	_, err := svc.doOpenAIUpstream(request, "", account)
	require.NoError(t, err)
	require.Nil(t, capturedCodexSnapshot(request.Context(), account.ID), "response metadata must not leak into another upstream attempt")
	observation := codexObservationFromResponse(response)
	require.NotNil(t, observation)
	// Replaying error handling later must use this exact captured time. Using a
	// fresh Parse would change even its nanoseconds and fail the reset assertion.
	wantReset := codexSnapshotBaseTime(observation.snapshot, time.Time{}).Add(600 * time.Second)
	svc.rateLimitService = NewRateLimitService(repo, nil, nil, nil, nil)
	svc.handleFailoverSideEffects(context.Background(), response, account, nil)
	require.Equal(t, wantReset, repo.resetAt)
	require.Equal(t, wireHeaders, response.Header, "internal metadata must not enter wire headers")
	waitCodexObservationWrites(t, svc)
	select {
	case <-repo.updateExtraCalls:
	case <-time.After(time.Second):
		t.Fatal("missing initial observation")
	}
	select {
	case update := <-repo.updateExtraCalls:
		t.Fatalf("429 handling bypassed the ordered writer: %v", update)
	default:
	}
}

func TestDirect429StillPersistsCodexObservation(t *testing.T) {
	repo := &captured429ObservationRepo{snapshotUpdateAccountRepo: snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 2)}}
	account := &Account{ID: 903, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	svc.handle429(context.Background(), account, codexObservationTestHeaders(), nil)
	require.False(t, repo.resetAt.IsZero())
	select {
	case update := <-repo.updateExtraCalls:
		require.Equal(t, 100.0, update["codex_5h_used_percent"])
	default:
		t.Fatal("standalone/native WebSocket error handling must retain snapshot persistence")
	}
}

func TestWSHTTPBridgeMarksQuotaAlreadyCaptured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 4)}
	headers := codexObservationTestHeaders()
	headers.Set("Content-Type", "text/event-stream")
	response := &http.Response{StatusCode: http.StatusOK, Header: headers, Body: io.NopCloser(strings.NewReader(
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_quota\",\"model\":\"gpt-5.5\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n",
	))}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, accountRepo: repo, httpUpstream: &quotaHeaderUpstream{response: response}}
	t.Cleanup(svc.codexObservationGate.close)
	account := &Account{ID: 904, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "account-id"}}
	payload := []byte(`{"type":"response.create","model":"gpt-5.5","stream":true,"input":"hi"}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "oauth-token", payload, len(payload), "gpt-5.5", "", "", "", "", 1, func([]byte) error { return nil })
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.CodexUsageCaptured, "the WebSocket handler must not resample an HTTP bridge response")
}
