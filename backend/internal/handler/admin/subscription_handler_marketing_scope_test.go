//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type subscriptionScopeRepoStub struct {
	sub     *service.UserSubscription
	deleted []int64
}

func (r *subscriptionScopeRepoStub) Create(context.Context, *service.UserSubscription) error {
	panic("unexpected Create call")
}

func (r *subscriptionScopeRepoStub) GetByID(_ context.Context, id int64) (*service.UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, service.ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *subscriptionScopeRepoStub) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.UserSubscription, error) {
	return r.GetByID(ctx, id)
}

func (r *subscriptionScopeRepoStub) GetByUserIDAndGroupID(context.Context, int64, int64) (*service.UserSubscription, error) {
	panic("unexpected GetByUserIDAndGroupID call")
}

func (r *subscriptionScopeRepoStub) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*service.UserSubscription, error) {
	panic("unexpected GetActiveByUserIDAndGroupID call")
}

func (r *subscriptionScopeRepoStub) Update(context.Context, *service.UserSubscription) error {
	panic("unexpected Update call")
}

func (r *subscriptionScopeRepoStub) Delete(_ context.Context, id int64) error {
	r.deleted = append(r.deleted, id)
	return nil
}

func (r *subscriptionScopeRepoStub) Restore(context.Context, int64, string) (*service.UserSubscription, error) {
	panic("unexpected Restore call")
}

func (r *subscriptionScopeRepoStub) ListByUserID(context.Context, int64) ([]service.UserSubscription, error) {
	panic("unexpected ListByUserID call")
}

func (r *subscriptionScopeRepoStub) ListActiveByUserID(context.Context, int64) ([]service.UserSubscription, error) {
	panic("unexpected ListActiveByUserID call")
}

func (r *subscriptionScopeRepoStub) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}

func (r *subscriptionScopeRepoStub) List(context.Context, pagination.PaginationParams, *int64, *int64, []int64, string, string, string, string, string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (r *subscriptionScopeRepoStub) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	panic("unexpected ExistsByUserIDAndGroupID call")
}

func (r *subscriptionScopeRepoStub) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	panic("unexpected ExistsActiveByUserIDAndGroupID call")
}

func (r *subscriptionScopeRepoStub) ExtendExpiry(context.Context, int64, time.Time) error {
	panic("unexpected ExtendExpiry call")
}

func (r *subscriptionScopeRepoStub) UpdateStatus(context.Context, int64, string) error {
	panic("unexpected UpdateStatus call")
}

func (r *subscriptionScopeRepoStub) UpdateNotes(context.Context, int64, string) error {
	panic("unexpected UpdateNotes call")
}

func (r *subscriptionScopeRepoStub) ActivateWindows(context.Context, int64, time.Time) error {
	panic("unexpected ActivateWindows call")
}

func (r *subscriptionScopeRepoStub) ResetUsageWindows(context.Context, int64, bool, bool, bool, time.Time) error {
	panic("unexpected ResetUsageWindows call")
}

func (r *subscriptionScopeRepoStub) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetDailyUsage call")
}

func (r *subscriptionScopeRepoStub) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetWeeklyUsage call")
}

func (r *subscriptionScopeRepoStub) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetMonthlyUsage call")
}

func (r *subscriptionScopeRepoStub) IncrementUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementUsage call")
}

func (r *subscriptionScopeRepoStub) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	panic("unexpected BatchUpdateExpiredStatus call")
}

func TestSubscriptionHandlerMarketingRevokeAllowsAffiliateScopedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	adminSvc.users = []service.User{{ID: 7, Email: "scoped@example.com", Status: service.StatusActive}}
	subRepo := &subscriptionScopeRepoStub{sub: &service.UserSubscription{ID: 11, UserID: 7, GroupID: 2}}
	handler := NewSubscriptionHandler(service.NewSubscriptionService(nil, subRepo, nil, nil, nil), adminSvc)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "11"}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/subscriptions/11", nil)
	c.Set(string(middleware.ContextKeyUserRole), service.RoleMarketing)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 91})

	handler.Revoke(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{11}, subRepo.deleted)
}

func TestSubscriptionHandlerMarketingRevokeRejectsUserOutsideAffiliateScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	adminSvc.users = nil
	subRepo := &subscriptionScopeRepoStub{sub: &service.UserSubscription{ID: 11, UserID: 7, GroupID: 2}}
	handler := NewSubscriptionHandler(service.NewSubscriptionService(nil, subRepo, nil, nil, nil), adminSvc)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "11"}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/subscriptions/11", nil)
	c.Set(string(middleware.ContextKeyUserRole), service.RoleMarketing)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 91})

	handler.Revoke(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Empty(t, subRepo.deleted)
}
