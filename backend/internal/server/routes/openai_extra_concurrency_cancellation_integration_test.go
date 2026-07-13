//go:build integration

package routes

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type blockingOpenAIMessagesCancellationUpstream struct {
	arrivals chan int64
	release  <-chan struct{}
	calls    atomic.Int32
}

func (u *blockingOpenAIMessagesCancellationUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	u.arrivals <- accountID
	<-u.release
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_messages_cancel\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"gpt-5.1\",\"output\":[{\"type\":\"message\",\"id\":\"msg_cancel\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\ndata: [DONE]\n\n",
		)),
	}, nil
}

func (u *blockingOpenAIMessagesCancellationUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func (h *openAIExtraConcurrencyRoutesHarness) responsesRequestWithContext(
	ctx context.Context,
	session string,
	standardLimit int,
	extraLimit int,
) *httptest.ResponseRecorder {
	body := `{"model":"gpt-5.1","stream":false,"prompt_cache_key":"` + session + `","input":"` + session + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req = req.WithContext(ctx)
	req = req.WithContext(context.WithValue(req.Context(), openAIExtraConcurrencyRequestNameKey{}, strings.TrimPrefix(session, "request-")))
	req = req.WithContext(context.WithValue(req.Context(), openAIExtraConcurrencyRequestUserKey{}, openAIExtraConcurrencyRequestUser{
		userID:        h.userID,
		standardLimit: standardLimit,
		extraLimit:    extraLimit,
	}))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, req)
	return recorder
}

func (h *openAIExtraConcurrencyRoutesHarness) chatCompletionsRequestWithContext(
	ctx context.Context,
	session string,
	standardLimit int,
	extraLimit int,
) *httptest.ResponseRecorder {
	body := `{"model":"gpt-5.1","stream":false,"prompt_cache_key":"` + session + `","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req = req.WithContext(ctx)
	req = req.WithContext(context.WithValue(req.Context(), openAIExtraConcurrencyRequestNameKey{}, strings.TrimPrefix(session, "request-")))
	req = req.WithContext(context.WithValue(req.Context(), openAIExtraConcurrencyRequestUserKey{}, openAIExtraConcurrencyRequestUser{
		userID:        h.userID,
		standardLimit: standardLimit,
		extraLimit:    extraLimit,
	}))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, req)
	return recorder
}

func (h *openAIExtraConcurrencyRoutesHarness) messagesRequestWithContextForUser(
	ctx context.Context,
	session string,
	standardLimit int,
	extraLimit int,
) *httptest.ResponseRecorder {
	body := `{"model":"gpt-5.1","stream":false,"metadata":{"user_id":"` + session + `"},"max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req = req.WithContext(ctx)
	req = req.WithContext(context.WithValue(req.Context(), openAIExtraConcurrencyRequestNameKey{}, strings.TrimPrefix(session, "request-")))
	req = req.WithContext(context.WithValue(req.Context(), openAIExtraConcurrencyRequestUserKey{}, openAIExtraConcurrencyRequestUser{
		userID:        h.userID,
		standardLimit: standardLimit,
		extraLimit:    extraLimit,
	}))
	req = req.WithContext(context.WithValue(req.Context(), openAIExtraConcurrencyEndpointKey{}, "messages"))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, req)
	return recorder
}

func TestOpenAIResponsesCanceledDuringSameAccountRetryDoesNotWriteFallback(t *testing.T) {
	releaseUpstream := make(chan struct{}, 1)
	upstream := &retryingOpenAIExtraConcurrencyUpstream{
		arrivals: make(chan string, 2),
		release:  releaseUpstream,
	}
	accountExtra := map[string]any{
		"openai_passthrough":    false,
		"openai_responses_mode": "force_responses",
	}
	harness := newOpenAIExtraConcurrencyRoutesHarness(
		t,
		extraConcurrencySettingRepository{},
		upstream,
		accountExtra,
		[]service.Account{{
			ID:          1301,
			Name:        "openai-pool-mode-responses-cancel",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Concurrency: 1,
			Status:      service.StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"api_key":               "sk-pool-responses-cancel",
				"base_url":              "https://api.openai.com",
				"pool_mode":             true,
				"pool_mode_retry_count": 1,
			},
			Extra:    accountExtra,
			GroupIDs: []int64{2301},
		}},
	)

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() {
		responses <- harness.responsesRequestWithContext(requestCtx, "request-cancel", 1, 0)
	}()
	require.Equal(t, "CANCEL", <-upstream.arrivals)
	cancelRequest()

	canceledResponse := requireOpenAIExtraConcurrencyHTTPResponse(t, responses, "CANCEL")
	require.Empty(t, canceledResponse.Body.String())
	require.Equal(t, int32(1), upstream.calls.Load())

	go func() {
		responses <- harness.responsesRequestWithContext(context.Background(), "request-next", 1, 0)
	}()
	select {
	case arrival := <-upstream.arrivals:
		require.Equal(t, "NEXT", arrival)
	case <-time.After(2 * time.Second):
		t.Fatal("next request did not acquire the released user and target leases")
	}
	releaseUpstream <- struct{}{}
	nextResponse := requireOpenAIExtraConcurrencyHTTPResponse(t, responses, "NEXT")
	require.Equal(t, http.StatusOK, nextResponse.Code, nextResponse.Body.String())
}

func TestOpenAIChatCompletionsCanceledDuringSameAccountRetryDoesNotWriteFallback(t *testing.T) {
	releaseUpstream := make(chan struct{}, 1)
	upstream := &retryingOpenAIExtraConcurrencyUpstream{
		arrivals: make(chan string, 2),
		release:  releaseUpstream,
	}
	accountExtra := map[string]any{
		"openai_passthrough":    false,
		"openai_responses_mode": "force_chat_completions",
	}
	harness := newOpenAIExtraConcurrencyRoutesHarness(
		t,
		extraConcurrencySettingRepository{},
		upstream,
		accountExtra,
		[]service.Account{{
			ID:          1301,
			Name:        "openai-pool-mode-chat-cancel",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Concurrency: 1,
			Status:      service.StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"api_key":               "sk-pool-chat-cancel",
				"base_url":              "https://api.openai.com",
				"pool_mode":             true,
				"pool_mode_retry_count": 1,
			},
			Extra:    accountExtra,
			GroupIDs: []int64{2301},
		}},
	)

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() {
		responses <- harness.chatCompletionsRequestWithContext(requestCtx, "request-cancel", 1, 0)
	}()
	require.Equal(t, "CANCEL", <-upstream.arrivals)
	cancelRequest()

	canceledResponse := requireOpenAIExtraConcurrencyHTTPResponse(t, responses, "CANCEL")
	require.Empty(t, canceledResponse.Body.String())
	require.Equal(t, int32(1), upstream.calls.Load())

	go func() {
		responses <- harness.chatCompletionsRequestWithContext(context.Background(), "request-next", 1, 0)
	}()
	select {
	case arrival := <-upstream.arrivals:
		require.Equal(t, "NEXT", arrival)
	case <-time.After(2 * time.Second):
		t.Fatal("next request did not acquire the released user and target leases")
	}
	releaseUpstream <- struct{}{}
	nextResponse := requireOpenAIExtraConcurrencyHTTPResponse(t, responses, "NEXT")
	require.Equal(t, http.StatusOK, nextResponse.Code, nextResponse.Body.String())
}

func TestOpenAIResponsesCanceledWhileWaitingForTargetDoesNotWriteFallback(t *testing.T) {
	releaseUpstream := make(chan struct{})
	t.Cleanup(func() { close(releaseUpstream) })
	upstream := &blockingOpenAIExtraConcurrencyUpstream{
		arrivals: make(chan int64, 1),
		release:  releaseUpstream,
	}
	accountExtra := map[string]any{
		"openai_passthrough":    false,
		"openai_responses_mode": "force_responses",
	}
	harness := newOpenAIExtraConcurrencyRoutesHarnessWithLoadBatch(
		t,
		extraConcurrencySettingRepository{waitTimeoutSeconds: 10},
		upstream,
		accountExtra,
		false,
		[]service.Account{{
			ID:          1301,
			Name:        "openai-responses-target-wait-cancel",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Concurrency: 1,
			Status:      service.StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"api_key":  "sk-responses-target-wait-cancel",
				"base_url": "https://api.openai.com",
			},
			Extra:    accountExtra,
			GroupIDs: []int64{2301},
		}},
	)

	blockerRequest := service.TargetLeaseRequest{
		RequestID:        "openai-responses-target-wait-blocker",
		Platform:         service.PlatformOpenAI,
		AccountID:        1301,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
		WaitTimeout:      30 * time.Second,
	}
	blocker, err := harness.store.TryAcquireTargetLease(t.Context(), blockerRequest)
	require.NoError(t, err)
	require.True(t, blocker.Acquired)
	t.Cleanup(func() {
		_ = harness.store.ReleaseTargetLease(context.Background(), blockerRequest.Platform, blockerRequest.AccountID, blockerRequest.RequestID)
	})

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() {
		responses <- harness.responsesRequestWithContext(requestCtx, "request-cancel", 1, 0)
	}()
	requireObservedOpenAIUserLeaseAttempt(t, harness.observer.userAttempts, "CANCEL", true)
	requireObservedOpenAITargetLeaseAttempt(t, harness.observer.targetAttempts, "CANCEL", 1301, false)
	select {
	case response := <-responses:
		t.Fatalf("request returned before target wait was canceled: status=%d body=%s", response.Code, response.Body.String())
	case accountID := <-upstream.arrivals:
		t.Fatalf("request unexpectedly reached blocked account %d", accountID)
	default:
	}
	cancelRequest()

	canceledResponse := requireOpenAIExtraConcurrencyHTTPResponse(t, responses, "CANCEL")
	require.Empty(t, canceledResponse.Body.String())
	require.Zero(t, upstream.calls.Load())

	require.NoError(t, harness.store.ReleaseTargetLease(
		t.Context(),
		blockerRequest.Platform,
		blockerRequest.AccountID,
		blockerRequest.RequestID,
	))
	go func() {
		responses <- harness.responsesRequestWithContext(context.Background(), "request-next", 1, 0)
	}()
	select {
	case accountID := <-upstream.arrivals:
		require.Equal(t, int64(1301), accountID)
	case <-time.After(2 * time.Second):
		t.Fatal("next request did not acquire the released user and target leases")
	}
	releaseUpstream <- struct{}{}
	nextResponse := requireOpenAIExtraConcurrencyHTTPResponse(t, responses, "NEXT")
	require.Equal(t, http.StatusOK, nextResponse.Code, nextResponse.Body.String())
}

func TestOpenAIChatCompletionsCanceledWhileWaitingForTargetDoesNotWriteFallback(t *testing.T) {
	releaseUpstream := make(chan struct{})
	t.Cleanup(func() { close(releaseUpstream) })
	upstream := &blockingOpenAIExtraConcurrencyUpstream{
		arrivals: make(chan int64, 1),
		release:  releaseUpstream,
	}
	accountExtra := map[string]any{
		"openai_passthrough":    false,
		"openai_responses_mode": "force_chat_completions",
	}
	harness := newOpenAIExtraConcurrencyRoutesHarnessWithLoadBatch(
		t,
		extraConcurrencySettingRepository{waitTimeoutSeconds: 10},
		upstream,
		accountExtra,
		false,
		[]service.Account{{
			ID:          1301,
			Name:        "openai-chat-target-wait-cancel",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Concurrency: 1,
			Status:      service.StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"api_key":  "sk-chat-target-wait-cancel",
				"base_url": "https://api.openai.com",
			},
			Extra:    accountExtra,
			GroupIDs: []int64{2301},
		}},
	)

	blockerRequest := service.TargetLeaseRequest{
		RequestID:        "openai-chat-target-wait-blocker",
		Platform:         service.PlatformOpenAI,
		AccountID:        1301,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
		WaitTimeout:      30 * time.Second,
	}
	blocker, err := harness.store.TryAcquireTargetLease(t.Context(), blockerRequest)
	require.NoError(t, err)
	require.True(t, blocker.Acquired)
	t.Cleanup(func() {
		_ = harness.store.ReleaseTargetLease(context.Background(), blockerRequest.Platform, blockerRequest.AccountID, blockerRequest.RequestID)
	})

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() {
		responses <- harness.chatCompletionsRequestWithContext(requestCtx, "request-cancel", 1, 0)
	}()
	requireObservedOpenAIUserLeaseAttempt(t, harness.observer.userAttempts, "CANCEL", true)
	requireObservedOpenAITargetLeaseAttempt(t, harness.observer.targetAttempts, "CANCEL", 1301, false)
	select {
	case response := <-responses:
		t.Fatalf("request returned before target wait was canceled: status=%d body=%s", response.Code, response.Body.String())
	case accountID := <-upstream.arrivals:
		t.Fatalf("request unexpectedly reached blocked account %d", accountID)
	default:
	}
	cancelRequest()

	canceledResponse := requireOpenAIExtraConcurrencyHTTPResponse(t, responses, "CANCEL")
	require.Empty(t, canceledResponse.Body.String())
	require.Zero(t, upstream.calls.Load())

	require.NoError(t, harness.store.ReleaseTargetLease(
		t.Context(),
		blockerRequest.Platform,
		blockerRequest.AccountID,
		blockerRequest.RequestID,
	))
	go func() {
		responses <- harness.chatCompletionsRequestWithContext(context.Background(), "request-next", 1, 0)
	}()
	select {
	case accountID := <-upstream.arrivals:
		require.Equal(t, int64(1301), accountID)
	case <-time.After(2 * time.Second):
		t.Fatal("next request did not acquire the released user and target leases")
	}
	releaseUpstream <- struct{}{}
	nextResponse := requireOpenAIExtraConcurrencyHTTPResponse(t, responses, "NEXT")
	require.Equal(t, http.StatusOK, nextResponse.Code, nextResponse.Body.String())
}

func TestOpenAIMessagesCanceledWhileWaitingForTargetDoesNotWriteFallback(t *testing.T) {
	releaseUpstream := make(chan struct{})
	t.Cleanup(func() { close(releaseUpstream) })
	upstream := &blockingOpenAIMessagesCancellationUpstream{
		arrivals: make(chan int64, 1),
		release:  releaseUpstream,
	}
	accountExtra := map[string]any{
		"openai_passthrough":    false,
		"openai_responses_mode": "force_responses",
	}
	harness := newOpenAIExtraConcurrencyRoutesHarnessWithLoadBatch(
		t,
		extraConcurrencySettingRepository{waitTimeoutSeconds: 10},
		upstream,
		accountExtra,
		false,
		[]service.Account{{
			ID:          1301,
			Name:        "openai-messages-target-wait-cancel",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Concurrency: 1,
			Status:      service.StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"api_key":  "sk-messages-target-wait-cancel",
				"base_url": "https://api.openai.com",
			},
			Extra:    accountExtra,
			GroupIDs: []int64{2301},
		}},
	)

	blockerRequest := service.TargetLeaseRequest{
		RequestID:        "openai-messages-target-wait-blocker",
		Platform:         service.PlatformOpenAI,
		AccountID:        1301,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
		WaitTimeout:      30 * time.Second,
	}
	blocker, err := harness.store.TryAcquireTargetLease(t.Context(), blockerRequest)
	require.NoError(t, err)
	require.True(t, blocker.Acquired)
	t.Cleanup(func() {
		_ = harness.store.ReleaseTargetLease(context.Background(), blockerRequest.Platform, blockerRequest.AccountID, blockerRequest.RequestID)
	})

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() {
		responses <- harness.messagesRequestWithContextForUser(requestCtx, "request-cancel", 1, 0)
	}()
	requireObservedOpenAIUserLeaseAttempt(t, harness.observer.userAttempts, "CANCEL", true)
	requireObservedOpenAITargetLeaseAttempt(t, harness.observer.targetAttempts, "CANCEL", 1301, false)
	select {
	case response := <-responses:
		t.Fatalf("request returned before target wait was canceled: status=%d body=%s", response.Code, response.Body.String())
	case accountID := <-upstream.arrivals:
		t.Fatalf("request unexpectedly reached blocked account %d", accountID)
	default:
	}
	cancelRequest()

	canceledResponse := requireOpenAIExtraConcurrencyHTTPResponse(t, responses, "CANCEL")
	require.Empty(t, canceledResponse.Body.String())
	require.Zero(t, upstream.calls.Load())

	require.NoError(t, harness.store.ReleaseTargetLease(
		t.Context(),
		blockerRequest.Platform,
		blockerRequest.AccountID,
		blockerRequest.RequestID,
	))
	go func() {
		responses <- harness.messagesRequestWithContextForUser(context.Background(), "request-next", 1, 0)
	}()
	select {
	case accountID := <-upstream.arrivals:
		require.Equal(t, int64(1301), accountID)
	case <-time.After(2 * time.Second):
		t.Fatal("next request did not acquire the released user and target leases")
	}
	releaseUpstream <- struct{}{}
	nextResponse := requireOpenAIExtraConcurrencyHTTPResponse(t, responses, "NEXT")
	require.Equal(t, http.StatusOK, nextResponse.Code, nextResponse.Body.String())
}
