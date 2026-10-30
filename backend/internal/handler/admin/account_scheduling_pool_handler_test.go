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

type poolAdminStub struct {
	accountSchedulingPoolAdminService
	poolID *int64
	ids    []int64
	policy service.AccountSchedulingPolicy
}

func (s *poolAdminStub) SetMembers(_ context.Context, id *int64, ids []int64) error {
	s.poolID = id
	s.ids = ids
	return nil
}
func (s *poolAdminStub) UpdatePolicy(_ context.Context, policy service.AccountSchedulingPolicy) error {
	s.policy = policy
	return nil
}

func TestAccountPoolMemberAssignmentValidatesAndPreservesOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		body    string
		status  int
		wantIDs []int64
	}{
		{`{"pool_id":7,"account_ids":[9,3,5]}`, 200, []int64{9, 3, 5}},
		{`{"pool_id":null,"account_ids":[9]}`, 200, []int64{9}},
		{`{"pool_id":0,"account_ids":[9]}`, 400, nil},
		{`{"pool_id":7,"account_ids":[0]}`, 400, nil},
		{`{"pool_id":7,"account_ids":[]}`, 400, nil},
	} {
		t.Run(tc.body, func(t *testing.T) {
			stub := &poolAdminStub{}
			h := &AccountSchedulingPoolHandler{service: stub}
			router := gin.New()
			router.PUT("/members", h.SetMembers)
			req := httptest.NewRequest(http.MethodPut, "/members", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			require.Equal(t, tc.wantIDs, stub.ids)
		})
	}
}

func TestAccountPoolFilterContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		raw    string
		status int
		want   int64
	}{{"0", 200, 0}, {"8", 200, 8}, {"-1", 400, 0}, {"bad", 400, 0}} {
		t.Run(tc.raw, func(t *testing.T) {
			router := gin.New()
			router.GET("/accounts", func(c *gin.Context) {
				if !applyAccountSchedulingPoolFilter(c) {
					return
				}
				id := service.AccountSchedulingPoolFilter(c.Request.Context())
				require.NotNil(t, id)
				require.Equal(t, tc.want, *id)
				c.Status(http.StatusOK)
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/accounts?pool_id="+tc.raw, nil))
			require.Equal(t, tc.status, response.Code)
		})
	}
}
