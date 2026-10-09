package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type claudeResetHandlerStub struct {
	key   string
	calls int
}

func (s *claudeResetHandlerStub) Query(context.Context, int64) (*service.ClaudeResetCredits, error) {
	return &service.ClaudeResetCredits{Credits: []service.ClaudeResetCredit{}}, nil
}

func (s *claudeResetHandlerStub) Redeem(_ context.Context, _ int64, key string) (*service.ClaudeResetOutcome, error) {
	s.calls++
	s.key = key
	if key == "" {
		return nil, service.ErrIdempotencyKeyRequired
	}
	return &service.ClaudeResetOutcome{Outcome: "reset", Cleared: []string{"five_hour"}}, nil
}

func TestClaudeResetHandlerRedeemContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &claudeResetHandlerStub{}
	h := &AccountHandler{claudeResetCredits: stub}
	r := gin.New()
	r.POST("/accounts/:id/claude/reset-credits/redeem", h.RedeemClaudeResetCredit)

	req := httptest.NewRequest(http.MethodPost, "/accounts/1/claude/reset-credits/redeem", nil)
	req.Header.Set("Idempotency-Key", "same-operation")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "same-operation", stub.key)
	require.Contains(t, w.Body.String(), `"outcome":"reset"`)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/accounts/1/claude/reset-credits/redeem", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "IDEMPOTENCY_KEY_REQUIRED")

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/accounts/abc/claude/reset-credits/redeem", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, 2, stub.calls)
}
