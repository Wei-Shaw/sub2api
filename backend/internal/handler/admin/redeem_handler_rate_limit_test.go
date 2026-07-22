//go:build unit

package admin

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type createAndRedeemRepoStub struct {
	code        *service.RedeemCode
	createCalls int
}

func (r *createAndRedeemRepoStub) Create(_ context.Context, code *service.RedeemCode) error {
	r.createCalls++
	cloned := *code
	cloned.ID = int64(r.createCalls)
	code.ID = cloned.ID
	r.code = &cloned
	return nil
}

func (r *createAndRedeemRepoStub) CreateBatch(context.Context, []service.RedeemCode) error {
	panic("unexpected CreateBatch call")
}

func (r *createAndRedeemRepoStub) GetByID(_ context.Context, id int64) (*service.RedeemCode, error) {
	if r.code == nil || r.code.ID != id {
		return nil, service.ErrRedeemCodeNotFound
	}
	cloned := *r.code
	return &cloned, nil
}

func (r *createAndRedeemRepoStub) GetByCode(_ context.Context, code string) (*service.RedeemCode, error) {
	if r.code == nil || r.code.Code != code {
		return nil, service.ErrRedeemCodeNotFound
	}
	cloned := *r.code
	return &cloned, nil
}

func (r *createAndRedeemRepoStub) Update(context.Context, *service.RedeemCode) error {
	panic("unexpected Update call")
}

func (r *createAndRedeemRepoStub) BatchUpdate(context.Context, []int64, service.RedeemCodeBatchUpdateFields) (int64, error) {
	panic("unexpected BatchUpdate call")
}

func (r *createAndRedeemRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (r *createAndRedeemRepoStub) Use(context.Context, int64, int64) error {
	panic("unexpected Use call")
}

func (r *createAndRedeemRepoStub) List(context.Context, pagination.PaginationParams) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (r *createAndRedeemRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (r *createAndRedeemRepoStub) ListByUser(context.Context, int64, int) ([]service.RedeemCode, error) {
	panic("unexpected ListByUser call")
}

func (r *createAndRedeemRepoStub) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
}

func (r *createAndRedeemRepoStub) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
}

type createAndRedeemCacheStub struct {
	getCalls       int
	incrementCalls int
	acquireCalls   int
	releaseCalls   int
}

func (c *createAndRedeemCacheStub) GetRedeemAttemptCount(context.Context, int64) (int, error) {
	c.getCalls++
	return 30, nil
}

func (c *createAndRedeemCacheStub) IncrementRedeemAttemptCount(context.Context, int64) error {
	c.incrementCalls++
	return nil
}

func (c *createAndRedeemCacheStub) AcquireRedeemLock(context.Context, string, time.Duration) (bool, error) {
	c.acquireCalls++
	return true, nil
}

func (c *createAndRedeemCacheStub) ReleaseRedeemLock(context.Context, string) error {
	c.releaseCalls++
	return nil
}

func newCreateAndRedeemRateLimitHandler(repo *createAndRedeemRepoStub, cache *createAndRedeemCacheStub) *RedeemHandler {
	return &RedeemHandler{
		adminService: newStubAdminService(),
		redeemService: service.NewRedeemService(
			repo, nil, nil, cache, nil, nil, nil, nil,
		),
	}
}

func TestCreateAndRedeemNewCodeUsesTrustedAdminPath(t *testing.T) {
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) })

	repo := &createAndRedeemRepoStub{}
	cache := &createAndRedeemCacheStub{}
	handler := newCreateAndRedeemRateLimitHandler(repo, cache)

	status := postCreateAndRedeemValidation(t, handler, map[string]any{
		"code":    "ADMIN-NEW",
		"type":    service.RedeemTypeInvitation,
		"value":   1,
		"user_id": 42,
	})

	require.Equal(t, 400, status)
	require.Equal(t, 1, repo.createCalls)
	require.Zero(t, cache.getCalls, "admin fulfillment must not read the public failure counter")
	require.Zero(t, cache.incrementCalls)
	require.Equal(t, 1, cache.acquireCalls)
	require.Equal(t, 1, cache.releaseCalls)
}

func TestCreateAndRedeemExistingUnusedCodeUsesTrustedAdminPath(t *testing.T) {
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) })

	repo := &createAndRedeemRepoStub{code: &service.RedeemCode{
		ID: 1, Code: "ADMIN-EXISTING", Type: service.RedeemTypeInvitation, Value: 1, Status: service.StatusUnused,
	}}
	cache := &createAndRedeemCacheStub{}
	handler := newCreateAndRedeemRateLimitHandler(repo, cache)

	status := postCreateAndRedeemValidation(t, handler, map[string]any{
		"code":    "ADMIN-EXISTING",
		"type":    service.RedeemTypeInvitation,
		"value":   1,
		"user_id": 42,
	})

	require.Equal(t, 400, status)
	require.Zero(t, repo.createCalls)
	require.Zero(t, cache.getCalls, "admin fulfillment retry must not read the public failure counter")
	require.Zero(t, cache.incrementCalls)
	require.Equal(t, 1, cache.acquireCalls)
	require.Equal(t, 1, cache.releaseCalls)
}

var _ service.RedeemCodeRepository = (*createAndRedeemRepoStub)(nil)
var _ service.RedeemCache = (*createAndRedeemCacheStub)(nil)
