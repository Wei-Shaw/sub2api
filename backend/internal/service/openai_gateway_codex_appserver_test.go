//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/codexapp"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type recordedCodexAppServerCall struct {
	WorkerKey  string
	SessionKey string
	Request    codexapp.TurnRequest
}

type fakeCodexAppServerBackend struct {
	mu    sync.Mutex
	calls []recordedCodexAppServerCall
}

func (b *fakeCodexAppServerBackend) RunTurn(
	_ context.Context,
	workerKey string,
	sessionKey string,
	request codexapp.TurnRequest,
	sink codexapp.EventSink,
) (codexapp.TurnResult, error) {
	b.mu.Lock()
	b.calls = append(b.calls, recordedCodexAppServerCall{WorkerKey: workerKey, SessionKey: sessionKey, Request: request})
	b.mu.Unlock()
	threadID := request.ThreadID
	if threadID == "" {
		threadID = "thread-user-e2e"
	}
	if sink != nil {
		if err := sink(codexapp.Event{
			Type: codexapp.EventAgentMessageDelta, ThreadID: threadID, TurnID: "turn", Delta: "hello",
		}); err != nil {
			return codexapp.TurnResult{}, err
		}
	}
	return codexapp.TurnResult{
		ThreadID: threadID, TurnID: "turn", Status: "completed",
		Usage: codexapp.TokenUsage{InputTokens: 4, CachedInputTokens: 1, OutputTokens: 2, TotalTokens: 6},
	}, nil
}

func (b *fakeCodexAppServerBackend) snapshot() []recordedCodexAppServerCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]recordedCodexAppServerCall(nil), b.calls...)
}

func TestOpenAIGatewayForwardCodexAppServerUserResponseAndContinuation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	backend := &fakeCodexAppServerBackend{}
	service := &OpenAIGatewayService{}
	service.SetCodexAppServerBackend(backend)
	t.Cleanup(service.CloseOpenAIWSPool)
	account := &Account{
		ID:    41,
		Extra: map[string]any{OpenAIExecutionBackendExtraKey: OpenAIExecutionBackendAppServer},
	}

	firstBody := []byte(`{"model":"gpt-test","stream":false,"input":"Reply with hello."}`)
	firstRecorder, firstContext := codexAppServerForwardTestContext(firstBody, 7)
	firstResult, err := service.Forward(context.Background(), firstContext, account, firstBody)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, firstRecorder.Code)
	require.Equal(t, OpenAIUsage{InputTokens: 4, CacheReadInputTokens: 1, OutputTokens: 2}, firstResult.Usage)

	var firstResponse map[string]any
	require.NoError(t, json.Unmarshal(firstRecorder.Body.Bytes(), &firstResponse))
	responseID, _ := firstResponse["id"].(string)
	require.NotEmpty(t, responseID)
	require.Equal(t, "completed", firstResponse["status"])
	require.Equal(t, "hello", firstResponse["output"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"])

	secondBody := []byte(`{"model":"gpt-test","stream":false,"previous_response_id":"` + responseID + `","input":"Continue."}`)
	secondRecorder, secondContext := codexAppServerForwardTestContext(secondBody, 7)
	_, err = service.Forward(context.Background(), secondContext, account, secondBody)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, secondRecorder.Code)

	calls := backend.snapshot()
	require.Len(t, calls, 2)
	require.Equal(t, "account:41", calls[0].WorkerKey)
	require.Equal(t, calls[0].SessionKey, calls[1].SessionKey, "previous_response_id must retain thread affinity")
	require.Equal(t, []codexapp.InputItem{{Type: "text", Text: "Reply with hello."}}, calls[0].Request.Input)
}

func TestOpenAIGatewayForwardCodexAppServerStreamsResponsesEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	backend := &fakeCodexAppServerBackend{}
	service := &OpenAIGatewayService{}
	service.SetCodexAppServerBackend(backend)
	account := &Account{ID: 42, Extra: map[string]any{
		OpenAIExecutionBackendExtraKey: OpenAIExecutionBackendAppServer,
	}}
	body := []byte(`{"model":"gpt-test","stream":true,"input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)
	recorder, c := codexAppServerForwardTestContext(body, 8)

	result, err := service.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.True(t, result.Stream)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Contains(t, recorder.Body.String(), "event: response.created")
	require.Contains(t, recorder.Body.String(), "event: response.output_text.delta")
	require.Contains(t, recorder.Body.String(), `"delta":"hello"`)
	require.Contains(t, recorder.Body.String(), "event: response.completed")
}

func TestOpenAIGatewayForwardCodexAppServerRejectsCrossAccountSessionMove(t *testing.T) {
	gin.SetMode(gin.TestMode)
	backend := &fakeCodexAppServerBackend{}
	service := &OpenAIGatewayService{}
	service.SetCodexAppServerBackend(backend)
	t.Cleanup(service.CloseOpenAIWSPool)
	accountA := &Account{ID: 41, Extra: map[string]any{
		OpenAIExecutionBackendExtraKey: OpenAIExecutionBackendAppServer,
	}}
	accountB := &Account{ID: 42, Extra: map[string]any{
		OpenAIExecutionBackendExtraKey: OpenAIExecutionBackendAppServer,
	}}

	firstBody := []byte(`{"model":"gpt-test","stream":false,"prompt_cache_key":"session-alpha","input":"hello"}`)
	firstRecorder, firstContext := codexAppServerForwardTestContext(firstBody, 7)
	_, err := service.Forward(context.Background(), firstContext, accountA, firstBody)
	require.NoError(t, err)
	var firstResponse map[string]any
	require.NoError(t, json.Unmarshal(firstRecorder.Body.Bytes(), &firstResponse))
	responseID, _ := firstResponse["id"].(string)
	require.NotEmpty(t, responseID)

	for _, body := range [][]byte{
		[]byte(`{"model":"gpt-test","stream":false,"prompt_cache_key":"session-alpha","input":"continue"}`),
		[]byte(`{"model":"gpt-test","stream":false,"previous_response_id":"` + responseID + `","input":"continue"}`),
	} {
		recorder, c := codexAppServerForwardTestContext(body, 7)
		_, err := service.Forward(context.Background(), c, accountB, body)
		require.Error(t, err)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Contains(t, recorder.Body.String(), "session_pinned_account_unavailable")
	}

	require.Len(t, backend.snapshot(), 1, "a cross-account continuation must not start a new app-server turn")
}

func TestOpenAIAppServerSchedulingConcurrencyIsOne(t *testing.T) {
	account := &Account{
		Concurrency: 12,
		LoadFactor:  func() *int { value := 8; return &value }(),
		Extra: map[string]any{
			OpenAIExecutionBackendExtraKey: OpenAIExecutionBackendAppServer,
		},
	}
	require.Equal(t, 1, openAIAccountSchedulingConcurrency(account))
	require.Equal(t, 1, openAIAccountSchedulingLoadFactor(account))

	direct := &Account{Concurrency: 12, LoadFactor: func() *int { value := 8; return &value }()}
	require.Equal(t, 12, openAIAccountSchedulingConcurrency(direct))
	require.Equal(t, 8, openAIAccountSchedulingLoadFactor(direct))
}

func codexAppServerForwardTestContext(body []byte, apiKeyID int64) (*httptest.ResponseRecorder, *gin.Context) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("api_key", &APIKey{ID: apiKeyID})
	return recorder, c
}
