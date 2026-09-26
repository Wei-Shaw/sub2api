package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type evaluationHandlerAccounts struct {
	service.AccountRepository
	calls int
}

func (s *evaluationHandlerAccounts) GetByID(context.Context, int64) (*service.Account, error) {
	s.calls++
	return nil, nil
}

func TestModelEvaluationHandlerValidationAndDirectRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accounts := &evaluationHandlerAccounts{}
	h := NewModelEvaluationHandler(service.NewModelEvaluationHistoryService(service.NewModelEvaluationService(accounts, nil, nil, nil), nil))
	r := gin.New()
	r.POST("/reports", h.Create)
	request := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/reports", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rec, req)
		return rec
	}
	for _, body := range []string{`{`, `{}`, `{"target_id":"invalid"}`, `{"model":"` + strings.Repeat("a", 5000) + `"}`} {
		require.Equal(t, http.StatusBadRequest, request(body).Code)
	}
	require.Zero(t, accounts.calls)
	rec := request(`{"target_type":"account","target_id":42,"model":"gpt-5.4","effort":"high","rounds":5}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "EVALUATION_ACCOUNT_UNAVAILABLE")
	require.Equal(t, 1, accounts.calls)
}
