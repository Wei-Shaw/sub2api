package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userStoreUnavailableRepoStub struct{}

func (userStoreUnavailableRepoStub) CreateProcessing(context.Context, *service.IdempotencyRecord) (bool, error) {
	return false, errors.New("store unavailable")
}
func (userStoreUnavailableRepoStub) GetByScopeAndKeyHash(context.Context, string, string) (*service.IdempotencyRecord, error) {
	return nil, errors.New("store unavailable")
}
func (userStoreUnavailableRepoStub) TryReclaim(context.Context, int64, string, time.Time, time.Time, time.Time) (bool, error) {
	return false, errors.New("store unavailable")
}
func (userStoreUnavailableRepoStub) ExtendProcessingLock(context.Context, int64, string, time.Time, time.Time) (bool, error) {
	return false, errors.New("store unavailable")
}
func (userStoreUnavailableRepoStub) MarkSucceeded(context.Context, int64, int, string, time.Time) error {
	return errors.New("store unavailable")
}
func (userStoreUnavailableRepoStub) MarkFailedRetryable(context.Context, int64, string, time.Time, time.Time) error {
	return errors.New("store unavailable")
}
func (userStoreUnavailableRepoStub) DeleteExpired(context.Context, time.Time, int) (int64, error) {
	return 0, errors.New("store unavailable")
}

type userMemoryIdempotencyRepoStub struct {
	mu     sync.Mutex
	nextID int64
	data   map[string]*service.IdempotencyRecord
}

func newUserMemoryIdempotencyRepoStub() *userMemoryIdempotencyRepoStub {
	return &userMemoryIdempotencyRepoStub{
		nextID: 1,
		data:   make(map[string]*service.IdempotencyRecord),
	}
}

func (r *userMemoryIdempotencyRepoStub) key(scope, keyHash string) string {
	return scope + "|" + keyHash
}

func (r *userMemoryIdempotencyRepoStub) clone(in *service.IdempotencyRecord) *service.IdempotencyRecord {
	if in == nil {
		return nil
	}
	out := *in
	if in.LockedUntil != nil {
		v := *in.LockedUntil
		out.LockedUntil = &v
	}
	if in.ResponseBody != nil {
		v := *in.ResponseBody
		out.ResponseBody = &v
	}
	if in.ResponseStatus != nil {
		v := *in.ResponseStatus
		out.ResponseStatus = &v
	}
	if in.ErrorReason != nil {
		v := *in.ErrorReason
		out.ErrorReason = &v
	}
	return &out
}

func (r *userMemoryIdempotencyRepoStub) CreateProcessing(_ context.Context, record *service.IdempotencyRecord) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(record.Scope, record.IdempotencyKeyHash)
	if _, ok := r.data[k]; ok {
		return false, nil
	}
	cp := r.clone(record)
	cp.ID = r.nextID
	r.nextID++
	r.data[k] = cp
	record.ID = cp.ID
	return true, nil
}

func (r *userMemoryIdempotencyRepoStub) GetByScopeAndKeyHash(_ context.Context, scope, keyHash string) (*service.IdempotencyRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.clone(r.data[r.key(scope, keyHash)]), nil
}

func (r *userMemoryIdempotencyRepoStub) TryReclaim(_ context.Context, id int64, fromStatus string, now, newLockedUntil, newExpiresAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID != id {
			continue
		}
		if rec.Status != fromStatus {
			return false, nil
		}
		if rec.LockedUntil != nil && rec.LockedUntil.After(now) {
			return false, nil
		}
		rec.Status = service.IdempotencyStatusProcessing
		rec.LockedUntil = &newLockedUntil
		rec.ExpiresAt = newExpiresAt
		rec.ErrorReason = nil
		return true, nil
	}
	return false, nil
}

func (r *userMemoryIdempotencyRepoStub) ExtendProcessingLock(_ context.Context, id int64, requestFingerprint string, newLockedUntil, newExpiresAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID != id {
			continue
		}
		if rec.Status != service.IdempotencyStatusProcessing || rec.RequestFingerprint != requestFingerprint {
			return false, nil
		}
		rec.LockedUntil = &newLockedUntil
		rec.ExpiresAt = newExpiresAt
		return true, nil
	}
	return false, nil
}

func (r *userMemoryIdempotencyRepoStub) MarkSucceeded(_ context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID != id {
			continue
		}
		rec.Status = service.IdempotencyStatusSucceeded
		rec.LockedUntil = nil
		rec.ExpiresAt = expiresAt
		rec.ResponseStatus = &responseStatus
		rec.ResponseBody = &responseBody
		rec.ErrorReason = nil
		return nil
	}
	return nil
}

func (r *userMemoryIdempotencyRepoStub) MarkFailedRetryable(_ context.Context, id int64, errorReason string, lockedUntil, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID != id {
			continue
		}
		rec.Status = service.IdempotencyStatusFailedRetryable
		rec.LockedUntil = &lockedUntil
		rec.ExpiresAt = expiresAt
		rec.ErrorReason = &errorReason
		return nil
	}
	return nil
}

func (r *userMemoryIdempotencyRepoStub) DeleteExpired(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func withUserSubject(userID int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID})
		c.Next()
	}
}

func TestExecuteUserIdempotentJSONFallbackWithoutCoordinator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.SetDefaultIdempotencyCoordinator(nil)

	var executed int
	router := gin.New()
	router.Use(withUserSubject(1))
	router.POST("/idempotent", func(c *gin.Context) {
		executeUserIdempotentJSON(c, "user.test.scope", map[string]any{"a": 1}, time.Minute, func(ctx context.Context) (any, error) {
			executed++
			return gin.H{"ok": true}, nil
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/idempotent", bytes.NewBufferString(`{"a":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, executed)
}

func TestExecuteUserIdempotentJSONFailCloseOnStoreUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(userStoreUnavailableRepoStub{}, service.DefaultIdempotencyConfig()))
	t.Cleanup(func() {
		service.SetDefaultIdempotencyCoordinator(nil)
	})

	var executed int
	router := gin.New()
	router.Use(withUserSubject(2))
	router.POST("/idempotent", func(c *gin.Context) {
		executeUserIdempotentJSON(c, "user.test.scope", map[string]any{"a": 1}, time.Minute, func(ctx context.Context) (any, error) {
			executed++
			return gin.H{"ok": true}, nil
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/idempotent", bytes.NewBufferString(`{"a":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "k1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, 0, executed)
}

func TestExecuteUserIdempotentJSONConcurrentRetrySingleSideEffectAndReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newUserMemoryIdempotencyRepoStub()
	cfg := service.DefaultIdempotencyConfig()
	cfg.ProcessingTimeout = 2 * time.Second
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(repo, cfg))
	t.Cleanup(func() {
		service.SetDefaultIdempotencyCoordinator(nil)
	})

	var executed atomic.Int32
	router := gin.New()
	router.Use(withUserSubject(3))
	router.POST("/idempotent", func(c *gin.Context) {
		executeUserIdempotentJSON(c, "user.test.scope", map[string]any{"a": 1}, time.Minute, func(ctx context.Context) (any, error) {
			executed.Add(1)
			time.Sleep(80 * time.Millisecond)
			return gin.H{"ok": true}, nil
		})
	})

	call := func() (int, http.Header) {
		req := httptest.NewRequest(http.MethodPost, "/idempotent", bytes.NewBufferString(`{"a":1}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "same-user-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code, rec.Header()
	}

	var status1, status2 int
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); status1, _ = call() }()
	go func() { defer wg.Done(); status2, _ = call() }()
	wg.Wait()

	require.Contains(t, []int{http.StatusOK, http.StatusConflict}, status1)
	require.Contains(t, []int{http.StatusOK, http.StatusConflict}, status2)
	require.Equal(t, int32(1), executed.Load())

	status3, headers3 := call()
	require.Equal(t, http.StatusOK, status3)
	require.Equal(t, "true", headers3.Get("X-Idempotency-Replayed"))
	require.Equal(t, int32(1), executed.Load())
}
