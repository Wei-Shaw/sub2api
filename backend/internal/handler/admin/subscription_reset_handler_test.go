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

type resetHandlerAdminStub struct {
	groupID int64
	limit   int
}

func (s *resetHandlerAdminStub) GetPolicy(_ context.Context, id int64) (*service.SubscriptionResetPolicy, error) {
	s.groupID = id
	return service.DefaultSubscriptionResetPolicy(id), nil
}
func (s *resetHandlerAdminStub) SavePolicy(_ context.Context, p *service.SubscriptionResetPolicy) (*service.SubscriptionResetPolicy, error) {
	s.groupID = p.GroupID
	return p, nil
}
func (s *resetHandlerAdminStub) GetStatus(_ context.Context, id int64, limit int) (*service.SubscriptionResetStatus, error) {
	s.groupID = id
	s.limit = limit
	return &service.SubscriptionResetStatus{ObservationOnly: true}, nil
}

func TestSubscriptionResetHandlerRouteOwnsGroupID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &resetHandlerAdminStub{}
	h := &SubscriptionHandler{resetObserver: s}
	r := gin.New()
	r.PUT("/groups/:id/subscription-reset-policy", h.SaveResetPolicy)
	req := httptest.NewRequest(http.MethodPut, "/groups/7/subscription-reset-policy", strings.NewReader(`{"group_id":99,"mode":"observe","version":0}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(7), s.groupID)
}

func TestSubscriptionResetHandlerRejectsInvalidIDsAndLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &resetHandlerAdminStub{}
	h := &SubscriptionHandler{resetObserver: s}
	r := gin.New()
	r.GET("/groups/:id/subscription-reset-status", h.GetResetStatus)
	for _, url := range []string{"/groups/0/subscription-reset-status", "/groups/no/subscription-reset-status", "/groups/1/subscription-reset-status?limit=101", "/groups/1/subscription-reset-status?limit=NaN"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		require.Equal(t, http.StatusBadRequest, w.Code, url)
	}
	require.Zero(t, s.groupID)
}
