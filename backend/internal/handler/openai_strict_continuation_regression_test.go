package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newContinuationRegressionHandler(t *testing.T, mode string) (*OpenAIGatewayHandler, *openAIWSFailoverHandlerAccountRepoStub, *openAIHTTPPassthroughAuthFailoverUpstream) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	account := service.Account{ID: 9911, Name: "existing-api-key", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "sk-test-fixture", "base_url": "https://api.example.test"},
		Extra:       map[string]any{service.OpenAIResponsesForwardModeExtraKey: mode, "openai_responses_supported": true},
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	repo := &openAIWSFailoverHandlerAccountRepoStub{accounts: []service.Account{account}}
	upstream := &openAIHTTPPassthroughAuthFailoverUpstream{}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
	return h, repo, upstream
}

func sendContinuationRegressionRequest(h *OpenAIGatewayHandler, previousID string) *httptest.ResponseRecorder {
	groupID := int64(4203)
	body := `{"model":"gpt-5.2","input":"hello","stream":false}`
	if previousID != "" {
		body = `{"model":"gpt-5.2","input":"continue","stream":false,"previous_response_id":"` + previousID + `"}`
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1803, GroupID: &groupID,
		User: &service.User{ID: 1703, Status: service.StatusActive}, Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive}})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1703, Concurrency: 0})
	h.Responses(c)
	return w
}

func TestOpenAIResponsesExistingNonStrictHTTPContinuation(t *testing.T) {
	for _, mode := range []string{"normal", "passthrough"} {
		t.Run(mode, func(t *testing.T) {
			h, _, _ := newContinuationRegressionHandler(t, mode)
			// Preserve pre-existing ownership records even when the scheduler binding
			// has expired: normal upstream continuation behavior is not strict opt-in.
			require.NoError(t, h.gatewayService.BindOpenAIHTTPResponseOwner(context.Background(), 4203, "resp_existing", 1703, 1803))
			w := sendContinuationRegressionRequest(h, "resp_existing")
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		})
	}
}

func TestOpenAIResponsesStrictContinuationKeepsItsProtocolOwner(t *testing.T) {
	for _, transition := range []string{"unchanged", "disabled", "reconfigured", "deleted"} {
		t.Run(transition, func(t *testing.T) {
			h, repo, upstream := newContinuationRegressionHandler(t, "strict_raw")
			first := sendContinuationRegressionRequest(h, "")
			require.Equal(t, http.StatusOK, first.Code, first.Body.String())
			require.Contains(t, first.Body.String(), "resp_healthy")
			require.Len(t, upstream.calls(), 1)
			fallback := repo.accounts[0]
			fallback.ID = 9912
			fallback.Extra = map[string]any{"openai_responses_supported": true}
			switch transition {
			case "disabled":
				repo.accounts[0].Status = service.StatusDisabled
			case "reconfigured":
				repo.accounts[0].Extra = map[string]any{"openai_responses_supported": true}
			case "deleted":
				repo.accounts = nil
			}
			if transition != "unchanged" {
				repo.accounts = append(repo.accounts, fallback)
			}
			next := sendContinuationRegressionRequest(h, "resp_healthy")
			if transition == "unchanged" {
				require.Equal(t, http.StatusOK, next.Code, next.Body.String())
				require.Equal(t, []int64{9911, 9911}, upstream.calls())
			} else {
				require.Equal(t, http.StatusConflict, next.Code, next.Body.String())
				require.Equal(t, []int64{9911}, upstream.calls(), "a strict continuation must not reach a replacement or transforming upstream")
			}
		})
	}
}
