package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type bulkActionHandlerSubscriptionRepo struct {
	service.UserSubscriptionRepository
	sub         *service.UserSubscription
	extendCalls int
}

type cancellationAwareBulkActionRepo struct {
	*bulkActionHandlerSubscriptionRepo
	cancelRequest context.CancelFunc
}

func (r *cancellationAwareBulkActionRepo) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.bulkActionHandlerSubscriptionRepo.GetByID(ctx, id)
}

func (r *cancellationAwareBulkActionRepo) ExtendExpiry(ctx context.Context, id int64, expiry time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := r.bulkActionHandlerSubscriptionRepo.ExtendExpiry(ctx, id, expiry)
	r.cancelRequest()
	return err
}

type cancellationAwareAdminIdempotencyRepo struct {
	*memoryIdempotencyRepoStub
	saves int
}

func (r *cancellationAwareAdminIdempotencyRepo) MarkSucceeded(ctx context.Context, id int64, status int, body string, expiresAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.saves++
	return r.memoryIdempotencyRepoStub.MarkSucceeded(ctx, id, status, body, expiresAt)
}

func (r *bulkActionHandlerSubscriptionRepo) GetByID(_ context.Context, id int64) (*service.UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, service.ErrSubscriptionNotFound
	}
	copy := *r.sub
	return &copy, nil
}

func (r *bulkActionHandlerSubscriptionRepo) GetByIDForUpdate(ctx context.Context, id int64) (*service.UserSubscription, error) {
	return r.GetByID(ctx, id)
}

func (r *bulkActionHandlerSubscriptionRepo) ExtendExpiry(_ context.Context, _ int64, expiry time.Time) error {
	r.extendCalls++
	r.sub.ExpiresAt = expiry
	return nil
}

func bulkActionHandlerRequest(router *gin.Engine, path, body, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestSubscriptionBulkAction_ReplaysPartialResultWithoutRepeatingExtension(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := service.DefaultIdempotencyConfig()
	cfg.ObserveOnly = false
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), cfg))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(nil) })
	expiresAt := time.Now().AddDate(0, 0, 30)
	repo := &bulkActionHandlerSubscriptionRepo{sub: &service.UserSubscription{ID: 1, UserID: 1, GroupID: 10, ExpiresAt: expiresAt}}
	svc := service.NewSubscriptionService(nil, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)
	h := NewSubscriptionHandler(svc)
	router := gin.New()
	path := "/api/v1/admin/subscriptions/bulk-action"
	router.POST(path, h.BulkAction)
	body := `{"subscription_ids":[1,404,1],"action":"extend","days":7}`

	missingKey := bulkActionHandlerRequest(router, path, body, "")
	require.Equal(t, http.StatusBadRequest, missingKey.Code)
	require.Zero(t, repo.extendCalls)

	first := bulkActionHandlerRequest(router, path, body, "bulk-extend-once")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var result struct {
		Data service.BulkSubscriptionActionResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &result))
	require.Equal(t, 1, result.Data.SuccessCount)
	require.Equal(t, 1, result.Data.FailedCount)
	require.Len(t, result.Data.Results, 2)

	replayed := bulkActionHandlerRequest(router, path, body, "bulk-extend-once")
	require.Equal(t, http.StatusOK, replayed.Code, replayed.Body.String())
	require.Equal(t, "true", replayed.Header().Get("X-Idempotency-Replayed"))
	require.JSONEq(t, first.Body.String(), replayed.Body.String())
	require.Equal(t, 1, repo.extendCalls)
	require.Equal(t, expiresAt.AddDate(0, 0, 7), repo.sub.ExpiresAt)

	conflict := bulkActionHandlerRequest(router, path, `{"subscription_ids":[1,404,1],"action":"extend","days":14}`, "bulk-extend-once")
	require.Equal(t, http.StatusConflict, conflict.Code)
	require.Equal(t, 1, repo.extendCalls)
}

func TestSubscriptionBulkAction_ClientCancellationStillPersistsReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	idempotencyRepo := &cancellationAwareAdminIdempotencyRepo{memoryIdempotencyRepoStub: newMemoryIdempotencyRepoStub()}
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(idempotencyRepo, service.DefaultIdempotencyConfig()))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(nil) })
	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	expiresAt := time.Now().AddDate(0, 0, 30)
	repo := &cancellationAwareBulkActionRepo{
		bulkActionHandlerSubscriptionRepo: &bulkActionHandlerSubscriptionRepo{sub: &service.UserSubscription{ID: 1, UserID: 1, GroupID: 10, ExpiresAt: expiresAt}},
		cancelRequest:                     cancel,
	}
	svc := service.NewSubscriptionService(nil, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)
	router := gin.New()
	path := "/api/v1/admin/subscriptions/bulk-action"
	router.POST(path, NewSubscriptionHandler(svc).BulkAction)
	body := `{"subscription_ids":[1],"action":"extend","days":7}`
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)).WithContext(requestCtx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "canceled-request")
	first := httptest.NewRecorder()
	router.ServeHTTP(first, request)
	require.ErrorIs(t, requestCtx.Err(), context.Canceled)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Equal(t, 1, idempotencyRepo.saves)

	replayed := bulkActionHandlerRequest(router, path, body, "canceled-request")
	require.Equal(t, http.StatusOK, replayed.Code, replayed.Body.String())
	require.Equal(t, "true", replayed.Header().Get("X-Idempotency-Replayed"))
	require.JSONEq(t, first.Body.String(), replayed.Body.String())
	require.Equal(t, 1, repo.extendCalls)
	require.Equal(t, expiresAt.AddDate(0, 0, 7), repo.sub.ExpiresAt)
}

func TestSubscriptionBulkAction_ValidatesBeforeIdempotencyAndExecution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(storeUnavailableRepoStub{}, service.DefaultIdempotencyConfig()))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(nil) })
	router := gin.New()
	path := "/api/v1/admin/subscriptions/bulk-action"
	router.POST(path, NewSubscriptionHandler(nil).BulkAction)
	tooManyIDs := make([]int64, 101)
	for i := range tooManyIDs {
		tooManyIDs[i] = 1
	}
	tooManyBody, err := json.Marshal(service.BulkSubscriptionActionInput{SubscriptionIDs: tooManyIDs, Action: "revoke"})
	require.NoError(t, err)
	for _, body := range []string{
		`{"subscription_ids":[1],"action":"delete"}`,
		`{"subscription_ids":[1,0],"action":"revoke"}`,
		`{"subscription_ids":[],"action":"restore"}`,
		`{"subscription_ids":[1],"action":"extend","days":0}`,
		`{"subscription_ids":[1],"action":"extend","days":36501}`,
		`{"subscription_ids":[1],"action":"reset_quota"}`,
		`{"subscription_ids":[1],"action":"reset_quota","daily":"yes"}`,
		`{"subscription_ids":[1],"action":"revoke"`,
		string(tooManyBody),
	} {
		t.Run(body, func(t *testing.T) {
			response := bulkActionHandlerRequest(router, path, body, "bulk-invalid")
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		})
	}

	// A valid request must still fail closed when replay protection is unavailable.
	response := bulkActionHandlerRequest(router, path, `{"subscription_ids":[1],"action":"revoke"}`, "bulk-valid")
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
}

func TestSubscriptionBulkAssign_RejectsInvalidUserIDsBeforeExecution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	path := "/api/v1/admin/subscriptions/bulk-assign"
	router.POST(path, NewSubscriptionHandler(nil).BulkAssign)
	for _, ids := range [][]int64{{}, {1, 0}, {1, -1}, make([]int64, 101)} {
		t.Run(fmt.Sprint(len(ids), ids), func(t *testing.T) {
			body, err := json.Marshal(BulkAssignSubscriptionRequest{UserIDs: ids, GroupID: 1, ValidityDays: 30})
			require.NoError(t, err)
			response := bulkActionHandlerRequest(router, path, string(body), "")
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		})
	}
}
