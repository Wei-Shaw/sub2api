package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
)

type claudeResetHandlerStub struct{ key, selection string }

func (s *claudeResetHandlerStub) Query(context.Context, int64) (*service.ClaudeResetCredits, error) {
	return &service.ClaudeResetCredits{Credits: []service.ClaudeResetCredit{}}, nil
}
func (s *claudeResetHandlerStub) Redeem(_ context.Context, _ int64, selection, key string) (*service.ClaudeResetOutcome, error) {
	s.key = key
	s.selection = selection
	return &service.ClaudeResetOutcome{Outcome: "unknown", Reason: "claim_unconfirmed"}, nil
}
func TestClaudeResetHandlerRedeemContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &claudeResetHandlerStub{}
	h := &AccountHandler{claudeResetCredits: stub}
	r := gin.New()
	r.POST("/accounts/:id/claude/reset-credits", h.RedeemClaudeResetCredit)
	req := httptest.NewRequest("POST", "/accounts/1/claude/reset-credits", strings.NewReader(`{"selection_token":"selected-use"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "same-operation")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	require.Equal(t, "same-operation", stub.key)
	require.Equal(t, "selected-use", stub.selection)
	require.Contains(t, w.Body.String(), `"outcome":"unknown"`)
}
