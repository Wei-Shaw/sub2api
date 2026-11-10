//go:build codexapp_real_e2e

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/codexapp"
	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const handlerRealE2EToken = "fake-codex-handler-e2e-token"

type handlerRealE2EAccountRepo struct {
	service.AccountRepository
	account service.Account
}

func (r handlerRealE2EAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if id != r.account.ID {
		return nil, service.ErrNoAvailableAccounts
	}
	account := r.account
	return &account, nil
}

func (r handlerRealE2EAccountRepo) GetByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	for _, id := range ids {
		if id == r.account.ID {
			account := r.account
			return []*service.Account{&account}, nil
		}
	}
	return nil, nil
}

func (r handlerRealE2EAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, platform string) ([]service.Account, error) {
	if platform == r.account.Platform {
		return []service.Account{r.account}, nil
	}
	return nil, nil
}

func (r handlerRealE2EAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.ListSchedulableByGroupIDAndPlatform(context.Background(), 0, platform)
}

func (r handlerRealE2EAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.ListSchedulableByGroupIDAndPlatform(context.Background(), 0, platform)
}

func (r handlerRealE2EAccountRepo) List(_ context.Context, _ pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	return []service.Account{r.account}, &pagination.PaginationResult{Total: 1}, nil
}

type handlerRealE2EWorker struct {
	codexapp.Worker
	mu       sync.Mutex
	requests []codexapp.TurnRequest
	results  []codexapp.TurnResult
}

func (w *handlerRealE2EWorker) RunTurn(ctx context.Context, request codexapp.TurnRequest, sink codexapp.EventSink) (codexapp.TurnResult, error) {
	result, err := w.Worker.RunTurn(ctx, request, sink)
	w.mu.Lock()
	w.requests = append(w.requests, request)
	w.results = append(w.results, result)
	w.mu.Unlock()
	return result, err
}

func (w *handlerRealE2EWorker) snapshot() ([]codexapp.TurnRequest, []codexapp.TurnResult) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]codexapp.TurnRequest(nil), w.requests...), append([]codexapp.TurnResult(nil), w.results...)
}

type handlerCapturedResponsesRequest struct {
	Authorization string
	Body          map[string]any
	Err           error
}

func TestOpenAIResponsesViaRealAppServerUserE2E(t *testing.T) {
	codexBin := strings.TrimSpace(os.Getenv("CODEX_REAL_E2E_BIN"))
	if codexBin == "" {
		t.Skip("CODEX_REAL_E2E_BIN is not set; run backend/scripts/test-codex-appserver-blackbox.sh")
	}
	gin.SetMode(gin.TestMode)

	upstreamRequests := make(chan handlerCapturedResponsesRequest, 4)
	fakeUpstream := httptest.NewServer(handlerFakeResponses(upstreamRequests))
	defer fakeUpstream.Close()

	var stderr bytes.Buffer
	var workersMu sync.Mutex
	var workers []*handlerRealE2EWorker
	pool := codexapp.NewPool(codexapp.PoolConfig{
		MaxWorkers: 2,
		IdleTTL:    time.Minute,
		Factory: func(ctx context.Context, _ string) (codexapp.Worker, error) {
			codexHome := t.TempDir()
			provider, err := codexapp.Start(ctx, codexapp.Config{
				Command: handlerRealAppServerCommand(codexBin, fakeUpstream.URL+"/v1"),
				Env: []string{
					"PATH=" + os.Getenv("PATH"), "HOME=" + codexHome, "CODEX_HOME=" + codexHome,
					"MOCK_OPENAI_API_KEY=" + handlerRealE2EToken, "RUST_LOG=error",
				},
				ClientInfo:   codexapp.ClientInfo{Name: "sub2api_handler_e2e", Title: "Sub2API Handler E2E", Version: "test"},
				StartTimeout: 15 * time.Second,
				Stderr:       &stderr,
			})
			if err != nil {
				return nil, err
			}
			worker := &handlerRealE2EWorker{Worker: provider}
			workersMu.Lock()
			workers = append(workers, worker)
			workersMu.Unlock()
			return worker, nil
		},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, pool.Close(ctx), "app-server stderr:\n%s", stderr.String())
	})

	groupID := int64(501)
	account := service.Account{
		ID: 41, Name: "codex-app-server-e2e", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
		Concurrency: 0, Credentials: map[string]any{"access_token": "fake-not-used"},
		Extra: map[string]any{service.OpenAIExecutionBackendExtraKey: service.OpenAIExecutionBackendAppServer},
	}
	repo := handlerRealE2EAccountRepo{account: account}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	gateway := service.NewOpenAIGatewayService(
		repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCache, nil, &service.DeferredService{},
		nil, nil, nil, nil, nil, nil, nil,
	)
	gateway.SetCodexAppServerBackend(pool)
	t.Cleanup(gateway.CloseOpenAIWSPool)
	h := NewOpenAIGatewayHandler(
		gateway, service.NewConcurrencyService(nil), billingCache,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg,
	)
	apiKey := &service.APIKey{
		ID: 7, UserID: 9, GroupID: &groupID,
		User:  &service.User{ID: 9, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer fake-sub2api-user-key" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: apiKey.UserID, Concurrency: 0})
		c.Next()
	})
	router.POST("/openai/v1/responses", h.Responses)
	server := httptest.NewServer(router)
	defer server.Close()

	first := handlerPostUserResponse(t, server.URL, `{"model":"gpt-6-sol","stream":false,"prompt_cache_key":"session-alpha","input":"Reply with hello only."}`)
	second := handlerPostUserResponse(t, server.URL, `{"model":"gpt-6-sol","stream":false,"prompt_cache_key":"session-alpha","input":"Reply with hello again."}`)
	require.Equal(t, "completed", first["status"])
	require.Equal(t, "hello", handlerResponseText(t, first))
	require.Equal(t, "completed", second["status"])
	require.Equal(t, "hello", handlerResponseText(t, second))

	workersMu.Lock()
	require.Len(t, workers, 1, "two user requests for one account must reuse one app-server process")
	worker := workers[0]
	workersMu.Unlock()
	requests, results := worker.snapshot()
	require.Len(t, requests, 2)
	require.Len(t, results, 2)
	require.Empty(t, requests[0].ThreadID)
	require.NotEmpty(t, results[0].ThreadID)
	require.Equal(t, results[0].ThreadID, requests[1].ThreadID, "the second request must continue the first app-server thread")

	for range 2 {
		select {
		case captured := <-upstreamRequests:
			require.NoError(t, captured.Err)
			require.Equal(t, "Bearer "+handlerRealE2EToken, captured.Authorization)
			require.Equal(t, "gpt-6-sol", captured.Body["model"])
		case <-time.After(3 * time.Second):
			t.Fatal("fake Responses upstream did not capture both turns")
		}
	}
}

func handlerPostUserResponse(t *testing.T, baseURL, body string) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/openai/v1/responses", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer fake-sub2api-user-key")
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, "response body: %s", payload)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	return decoded
}

func handlerResponseText(t *testing.T, response map[string]any) string {
	t.Helper()
	return response["output"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
}

func handlerRealAppServerCommand(codexBin, baseURL string) []string {
	return []string{
		codexBin, "app-server", "--listen", "stdio://",
		"-c", `model="gpt-6-sol"`,
		"-c", `model_provider="sub2api-handler-e2e"`,
		"-c", `web_search="disabled"`,
		"-c", `model_providers.sub2api-handler-e2e.name="Sub2API Handler E2E Fake Responses"`,
		"-c", fmt.Sprintf(`model_providers.sub2api-handler-e2e.base_url=%q`, baseURL),
		"-c", `model_providers.sub2api-handler-e2e.env_key="MOCK_OPENAI_API_KEY"`,
		"-c", `model_providers.sub2api-handler-e2e.wire_api="responses"`,
		"-c", `model_providers.sub2api-handler-e2e.requires_openai_auth=false`,
		"-c", `model_providers.sub2api-handler-e2e.supports_websockets=false`,
	}
}

func handlerFakeResponses(requests chan<- handlerCapturedResponsesRequest) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"gpt-6-sol","object":"model","created":0,"owned_by":"sub2api-e2e"}]}`)
			return
		}
		captured := handlerCapturedResponsesRequest{Authorization: r.Header.Get("Authorization")}
		body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			captured.Err = err
		} else {
			captured.Err = json.Unmarshal(body, &captured.Body)
		}
		requests <- captured

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, event := range []map[string]any{
			{"type": "response.created", "response": map[string]any{"id": "resp-handler-e2e"}},
			{"type": "response.output_item.added", "item": map[string]any{
				"type": "message", "role": "assistant", "id": "msg-handler-e2e",
				"content": []any{map[string]any{"type": "output_text", "text": ""}},
			}},
			{"type": "response.output_text.delta", "delta": "hello"},
			{"type": "response.output_item.done", "item": map[string]any{
				"type": "message", "role": "assistant", "id": "msg-handler-e2e",
				"content": []any{map[string]any{"type": "output_text", "text": "hello"}},
			}},
			{"type": "response.completed", "response": map[string]any{
				"id": "resp-handler-e2e", "usage": map[string]any{
					"input_tokens": 4, "output_tokens": 2, "total_tokens": 6,
					"input_tokens_details":  map[string]any{"cached_tokens": 1},
					"output_tokens_details": map[string]any{"reasoning_tokens": 0},
				},
			}},
		} {
			kind, _ := event["type"].(string)
			payload, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
	})
}
