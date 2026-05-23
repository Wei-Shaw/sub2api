package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type billingPoolManagerStub struct {
	createInput  *service.CreateBillingPoolInput
	createResult *service.BillingPool
	createErr    error
	lookupItems  []service.BillingPoolLookup
	lookupErr    error
}

func (s *billingPoolManagerStub) Create(_ context.Context, input *service.CreateBillingPoolInput) (*service.BillingPool, error) {
	if input != nil {
		copied := *input
		copied.Members = append([]service.BillingPoolMemberInput(nil), input.Members...)
		s.createInput = &copied
	}
	if s.createErr != nil {
		return nil, s.createErr
	}
	if s.createResult != nil {
		return s.createResult, nil
	}
	return &service.BillingPool{ID: 1, Name: input.Name, Code: input.Code, Status: service.StatusActive}, nil

}

func (s *billingPoolManagerStub) GetByID(context.Context, int64) (*service.BillingPool, error) {
	panic("unexpected GetByID call")
}

func (s *billingPoolManagerStub) List(context.Context, int, int, service.BillingPoolListFilters, string, string) ([]service.BillingPool, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *billingPoolManagerStub) ListLookup(context.Context) ([]service.BillingPoolLookup, error) {
	return append([]service.BillingPoolLookup(nil), s.lookupItems...), s.lookupErr
}

func (s *billingPoolManagerStub) Update(context.Context, int64, *service.UpdateBillingPoolInput) (*service.BillingPool, error) {
	panic("unexpected Update call")
}

func (s *billingPoolManagerStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (s *billingPoolManagerStub) ReplaceMembers(context.Context, int64, []service.BillingPoolMemberInput) (*service.BillingPool, error) {
	panic("unexpected ReplaceMembers call")
}

func (s *billingPoolManagerStub) GetByGroupID(context.Context, int64) (*service.BillingPool, error) {
	panic("unexpected GetByGroupID call")
}

func newBillingPoolHandlerRouter(manager service.BillingPoolManager) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewBillingPoolHandler(manager)
	router.GET("/api/v1/admin/billing-pools/lookup", handler.Lookup)
	router.POST("/api/v1/admin/billing-pools", handler.Create)
	return router
}

func TestBillingPoolHandlerLookup_ReturnsLookupItems(t *testing.T) {
	router := newBillingPoolHandlerRouter(&billingPoolManagerStub{lookupItems: []service.BillingPoolLookup{
		{ID: 1, Name: "Alpha Pool", Status: service.StatusActive, PlatformScope: string(service.BillingPoolPlatformScopeSamePlatform)},
		{ID: 2, Name: "Beta Pool", Status: "inactive", PlatformScope: string(service.BillingPoolPlatformScopeMixedPlatform)},
	}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/billing-pools/lookup", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	items, ok := resp.Data.([]any)
	require.True(t, ok)
	require.Len(t, items, 2)

	first, ok := items[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "Alpha Pool", first["name"])
	require.Equal(t, string(service.BillingPoolPlatformScopeSamePlatform), first["platform_scope"])
}

func TestBillingPoolHandlerCreate_DefaultsOptionalFlagsToTrue(t *testing.T) {
	stub := &billingPoolManagerStub{createResult: &service.BillingPool{
		ID:                         9,
		Name:                       "Primary Pool",
		Code:                       "primary-pool",
		Status:                     service.StatusActive,
		PlatformScope:              string(service.BillingPoolPlatformScopeSamePlatform),
		RequirePrimarySubscription: true,
		AllowBalanceFallback:       true,
	}}
	router := newBillingPoolHandlerRouter(stub)

	body := `{"name":"Primary Pool","code":"primary-pool","groups":[{"group_id":101,"chain_order":2,"can_be_primary":true,"can_be_fallback":true}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/billing-pools", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, stub.createInput)
	require.True(t, stub.createInput.RequirePrimarySubscription)
	require.True(t, stub.createInput.AllowBalanceFallback)
	require.Len(t, stub.createInput.Members, 1)
	require.Equal(t, int64(101), stub.createInput.Members[0].GroupID)
	require.Equal(t, 2, stub.createInput.Members[0].ChainOrder)
	require.True(t, stub.createInput.Members[0].CanBePrimary)
	require.True(t, stub.createInput.Members[0].CanBeFallback)

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(9), data["id"])
	require.Equal(t, "primary-pool", data["code"])
}
