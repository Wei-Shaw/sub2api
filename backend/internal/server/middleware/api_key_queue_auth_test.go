//go:build unit

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func queueAuthTestKey(status string, limit int) *service.APIKey {
	group := &service.Group{
		ID:       42,
		Name:     "queue-group",
		Status:   service.StatusActive,
		Platform: service.PlatformOpenAI,
		Hydrated: true,
	}
	user := &service.User{ID: 7, Status: service.StatusActive, Concurrency: 3}
	apiKey := &service.APIKey{
		ID:               100,
		UserID:           user.ID,
		Key:              "queue-test-credential",
		Status:           status,
		ConcurrencyLimit: limit,
		User:             user,
		Group:            group,
	}
	apiKey.GroupID = &group.ID
	return apiKey
}

func TestNewAPIKeyQueueAuthRevalidatorRejectsDisabledKey(t *testing.T) {
	current := queueAuthTestKey(service.StatusActive, 1)
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		return current, nil
	}}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
	revalidate := newAPIKeyQueueAuthRevalidator(svc, current.Key, "", current)

	current = queueAuthTestKey(service.StatusDisabled, 1)
	_, err := revalidate(context.Background())
	require.Error(t, err)
	var rejected *service.APIKeyQueueAuthRejectedError
	require.ErrorAs(t, err, &rejected)
	require.Equal(t, http.StatusUnauthorized, infraerrors.Code(err))
	require.Equal(t, "API_KEY_DISABLED", infraerrors.Reason(err))
}

func TestNewAPIKeyQueueAuthRevalidatorRejectsDeletedKey(t *testing.T) {
	current := queueAuthTestKey(service.StatusActive, 1)
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		return nil, service.ErrAPIKeyNotFound
	}}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
	revalidate := newAPIKeyQueueAuthRevalidator(svc, current.Key, "", current)

	_, err := revalidate(context.Background())
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, infraerrors.Code(err))
	require.Equal(t, "INVALID_API_KEY", infraerrors.Reason(err))
}

func TestNewAPIKeyQueueAuthRevalidatorRejectsGroupChange(t *testing.T) {
	current := queueAuthTestKey(service.StatusActive, 1)
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		return current, nil
	}}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
	revalidate := newAPIKeyQueueAuthRevalidator(svc, current.Key, "", current)

	moved := queueAuthTestKey(service.StatusActive, 1)
	other := int64(77)
	moved.GroupID = &other
	current = moved
	_, err := revalidate(context.Background())
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(err), "a binding change is retryable, not a permission denial")
	require.Equal(t, "API_KEY_GROUP_CHANGED", infraerrors.Reason(err))
	require.Contains(t, infraerrors.Message(err), "configuration changed")
}

func TestNewAPIKeyQueueAuthRevalidatorRejectsCoreBindingChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*service.APIKey)
	}{
		{name: "platform", mutate: func(key *service.APIKey) { key.Group.Platform = service.PlatformGrok }},
		{name: "subscription type", mutate: func(key *service.APIKey) { key.Group.SubscriptionType = "subscription" }},
		{name: "key owner", mutate: func(key *service.APIKey) { key.UserID = 8 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := queueAuthTestKey(service.StatusActive, 1)
			repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
				return current, nil
			}}
			svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
			revalidate := newAPIKeyQueueAuthRevalidator(svc, current.Key, "", current)

			changed := queueAuthTestKey(service.StatusActive, 1)
			tc.mutate(changed)
			current = changed
			_, err := revalidate(context.Background())
			require.Error(t, err)
			require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(err))
			require.Equal(t, "API_KEY_GROUP_CHANGED", infraerrors.Reason(err))
		})
	}
}

// TestNewAPIKeyQueueAuthRevalidatorAllowsUnrelatedPermissionChange pins the
// request-scoped rule: flipping a group permission the request never used (Live
// for a plain text request) must not interrupt the wait.
func TestNewAPIKeyQueueAuthRevalidatorAllowsUnrelatedPermissionChange(t *testing.T) {
	current := queueAuthTestKey(service.StatusActive, 1)
	current.Group.AllowLive = true
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		return current, nil
	}}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
	revalidate := newAPIKeyQueueAuthRevalidator(svc, current.Key, "", current)

	// Same group ID; the Live permission was revoked while a text request waited.
	revoked := queueAuthTestKey(service.StatusActive, 1)
	revoked.Group.AllowLive = false
	current = revoked
	limit, err := revalidate(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, limit)
}

// TestNewAPIKeyQueueAuthRevalidatorAllowsBenignGroupEdits covers metadata,
// pricing, routing and nil-vs-empty collection shape changes that a full Group
// comparison used to reject.
func TestNewAPIKeyQueueAuthRevalidatorAllowsBenignGroupEdits(t *testing.T) {
	current := queueAuthTestKey(service.StatusActive, 1)
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		return current, nil
	}}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
	revalidate := newAPIKeyQueueAuthRevalidator(svc, current.Key, "", current)

	benign := queueAuthTestKey(service.StatusActive, 1)
	benign.Group.Name = "renamed-group"
	benign.Group.Description = "new description"
	benign.Group.RateMultiplier = 2.5
	benign.Group.ImagePrice2K = new(float64)
	*benign.Group.ImagePrice2K = 1.25
	benign.Group.ModelRouting = map[string][]int64{}                                              // initial nil
	benign.Group.ModelPricing = []service.ChannelModelPricing{}                                   // initial nil
	benign.Group.SupportedModelScopes = []string{}                                                // initial nil
	benign.Group.ModelAllowlist = service.GroupModelAllowlist{Enabled: false, Models: []string{}} // initial zero
	current = benign
	limit, err := revalidate(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, limit)
}

func TestNewAPIKeyQueueAuthRevalidatorRejectsRemovedRequestedModel(t *testing.T) {
	current := queueAuthTestKey(service.StatusActive, 1)
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		return current, nil
	}}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
	revalidate := newAPIKeyQueueAuthRevalidator(svc, current.Key, "", current)
	ctx := service.WithAPIKeyQueueRequestPermissions(context.Background(), service.APIKeyQueueRequestPermissions{
		Models: []string{"gpt-5.4", "gpt-5.4-mini"},
	})

	// The initial group had the allowlist disabled; enabling it must still apply
	// to the already captured candidates.
	enabled := queueAuthTestKey(service.StatusActive, 1)
	enabled.Group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4", "gpt-5.4-nano"}}
	current = enabled
	_, err := revalidate(ctx)
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, infraerrors.Code(err))
	require.Equal(t, "MODEL_NOT_ALLOWED", infraerrors.Reason(err))
	require.Contains(t, infraerrors.Message(err), "gpt-5.4-mini")

	// Adding unrelated entries and keeping the requested model stays admitted.
	expanded := queueAuthTestKey(service.StatusActive, 1)
	expanded.Group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano"}}
	current = expanded
	limit, err := revalidate(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, limit)

	// Removing an unrelated entry must not reject the still-allowed request.
	shrunk := queueAuthTestKey(service.StatusActive, 1)
	shrunk.Group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4", "gpt-5.4-mini"}}
	current = shrunk
	limit, err = revalidate(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, limit)
}

func TestNewAPIKeyQueueAuthRevalidatorRejectsRevokedCapability(t *testing.T) {
	for _, tc := range []struct {
		name       string
		capability service.APIKeyQueueCapability
		mutate     func(*service.APIKey)
		reason     string
	}{
		{
			name:       "live",
			capability: service.APIKeyQueueCapabilityLive,
			mutate:     func(key *service.APIKey) { key.Group.AllowLive = false },
			reason:     "LIVE_NOT_ALLOWED",
		},
		{
			name:       "image generation",
			capability: service.APIKeyQueueCapabilityImageGeneration,
			mutate:     func(key *service.APIKey) { key.Group.AllowImageGeneration = false },
			reason:     "IMAGE_GENERATION_NOT_ALLOWED",
		},
		{
			name:       "messages dispatch",
			capability: service.APIKeyQueueCapabilityMessagesDispatch,
			mutate:     func(key *service.APIKey) { key.Group.AllowMessagesDispatch = false },
			reason:     "MESSAGES_DISPATCH_NOT_ALLOWED",
		},
		{
			name:       "claude code only",
			capability: service.APIKeyQueueCapabilityForbiddenWhenClaudeCodeOnly,
			mutate:     func(key *service.APIKey) { key.Group.ClaudeCodeOnly = true },
			reason:     "CLAUDE_CODE_ONLY",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := queueAuthTestKey(service.StatusActive, 1)
			current.Group.AllowLive = true
			current.Group.AllowImageGeneration = true
			current.Group.AllowMessagesDispatch = true
			repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
				return current, nil
			}}
			svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
			revalidate := newAPIKeyQueueAuthRevalidator(svc, current.Key, "", current)
			ctx := service.WithAPIKeyQueueCapability(context.Background(), tc.capability)

			revoked := queueAuthTestKey(service.StatusActive, 1)
			tc.mutate(revoked)
			current = revoked
			_, err := revalidate(ctx)
			require.Error(t, err)
			require.Equal(t, http.StatusForbidden, infraerrors.Code(err))
			require.Equal(t, tc.reason, infraerrors.Reason(err))

			// An unrelated capability flip still passes.
			unrelated := queueAuthTestKey(service.StatusActive, 1)
			unrelated.Group.AllowLive = true
			unrelated.Group.AllowImageGeneration = true
			unrelated.Group.AllowMessagesDispatch = false
			if tc.capability == service.APIKeyQueueCapabilityMessagesDispatch {
				unrelated.Group.AllowMessagesDispatch = true
				unrelated.Group.AllowLive = false
			}
			current = unrelated
			limit, err := revalidate(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, limit)
		})
	}
}

func TestNewAPIKeyQueueAuthRevalidatorRejectsIPBlacklistChange(t *testing.T) {
	current := queueAuthTestKey(service.StatusActive, 1)
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		return current, nil
	}}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
	const trustedIP = "203.0.113.9"
	revalidate := newAPIKeyQueueAuthRevalidator(svc, current.Key, trustedIP, current)

	blocked := queueAuthTestKey(service.StatusActive, 1)
	blocked.IPBlacklist = []string{trustedIP}
	current = blocked
	_, err := revalidate(context.Background())
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, infraerrors.Code(err))
	require.Equal(t, "ACCESS_DENIED", infraerrors.Reason(err))
}

func TestNewAPIKeyQueueAuthRevalidatorReturnsFreshLimitAndCaches(t *testing.T) {
	key := queueAuthTestKey(service.StatusActive, 4)
	var repoCalls atomic.Int32
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		repoCalls.Add(1)
		return key, nil
	}}
	cfg := &config.Config{}
	cfg.APIKeyAuth.L1Size = 64
	cfg.APIKeyAuth.L1TTLSeconds = 60
	cfg.APIKeyAuth.L2TTLSeconds = 60
	authCache := &stubAPIKeyAuthCache{entries: map[string]*service.APIKeyAuthCacheEntry{}}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, authCache, cfg)
	revalidate := newAPIKeyQueueAuthRevalidator(svc, key.Key, "", key)

	for i := 0; i < 3; i++ {
		limit, err := revalidate(context.Background())
		require.NoError(t, err)
		require.Equal(t, 4, limit)
	}
	require.Equal(t, int32(1), repoCalls.Load(), "revalidation reuses the existing auth cache instead of polling the database")
}

// stubAPIKeyAuthCache is a minimal in-memory L2 auth cache.
type stubAPIKeyAuthCache struct {
	mu      sync.Mutex
	entries map[string]*service.APIKeyAuthCacheEntry
}

func (c *stubAPIKeyAuthCache) GetCreateAttemptCount(context.Context, int64) (int, error) {
	return 0, nil
}
func (c *stubAPIKeyAuthCache) IncrementCreateAttemptCount(context.Context, int64) error { return nil }
func (c *stubAPIKeyAuthCache) DeleteCreateAttemptCount(context.Context, int64) error    { return nil }
func (c *stubAPIKeyAuthCache) IncrementDailyUsage(context.Context, string) error        { return nil }
func (c *stubAPIKeyAuthCache) SetDailyUsageExpiry(context.Context, string, time.Duration) error {
	return nil
}
func (c *stubAPIKeyAuthCache) GetAuthCache(_ context.Context, key string) (*service.APIKeyAuthCacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[key]
	if entry == nil {
		return nil, errors.New("cache miss")
	}
	return entry, nil
}
func (c *stubAPIKeyAuthCache) SetAuthCache(_ context.Context, key string, entry *service.APIKeyAuthCacheEntry, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = entry
	return nil
}
func (c *stubAPIKeyAuthCache) DeleteAuthCache(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
	return nil
}
func (c *stubAPIKeyAuthCache) PublishAuthCacheInvalidation(context.Context, string) error { return nil }
func (c *stubAPIKeyAuthCache) SubscribeAuthCacheInvalidation(context.Context, func(string)) error {
	return nil
}

// TestAPIKeyQueueMiddlewareInstallsRevalidator proves the installed callback is
// the actual path used by a queued request: the holder owns the only slot, the
// key is disabled while the request waits, and the wait ends with the same
// authentication error instead of forwarding.
func TestAPIKeyQueueMiddlewareInstallsRevalidator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := testutil.NewRedisConcurrencyCache(t)
	queueCache, ok := cache.(service.APIKeySlotQueueCache)
	require.True(t, ok)

	key := queueAuthTestKey(service.StatusActive, 1)
	current := key
	var currentMu sync.Mutex
	loadCurrent := func() *service.APIKey {
		currentMu.Lock()
		defer currentMu.Unlock()
		return current
	}
	setCurrent := func(next *service.APIKey) {
		currentMu.Lock()
		current = next
		currentMu.Unlock()
	}
	var repoCalls atomic.Int32
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		repoCalls.Add(1)
		return loadCurrent(), nil
	}}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)

	concurrencyService := service.NewConcurrencyService(cache)
	concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 1, Timeout: 3 * time.Second})

	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, key.ID, 1)
	require.NoError(t, err)
	require.NotNil(t, holder)
	defer holder.Release()

	var upstreamCalls atomic.Int32
	handlerResult := make(chan error, 1)
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.POST("/v1/messages", func(c *gin.Context) {
		reservation, reserveErr := concurrencyService.ReserveAPIKeySlotWithWait(c.Request.Context(), key.ID, 1)
		if reserveErr == nil && reservation != nil {
			upstreamCalls.Add(1)
			reservation.Release()
			c.Status(http.StatusOK)
			handlerResult <- nil
			return
		}
		c.Status(http.StatusUnauthorized)
		handlerResult <- reserveErr
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	request.Header.Set("x-api-key", key.Key)
	response := httptest.NewRecorder()
	serveDone := make(chan struct{})
	started := time.Now()
	go func() {
		router.ServeHTTP(response, request)
		close(serveDone)
	}()

	require.Eventually(t, func() bool {
		_, waiting, statsErr := queueCache.GetAPIKeyQueueStats(context.Background(), key.ID)
		return statsErr == nil && waiting == 1
	}, 2*time.Second, 5*time.Millisecond, "request must be waiting before the key changes")

	setCurrent(queueAuthTestKey(service.StatusDisabled, 1))
	select {
	case reserveErr := <-handlerResult:
		require.True(t, service.IsAPIKeyQueueErrorKind(reserveErr, service.APIKeyQueueErrorAuthRejected))
		require.Equal(t, http.StatusUnauthorized, infraerrors.Code(reserveErr))
		require.Equal(t, "API_KEY_DISABLED", infraerrors.Reason(reserveErr))
	case <-time.After(3 * time.Second):
		t.Fatal("queued request did not stop after the key was disabled")
	}
	require.Less(t, time.Since(started), 2*time.Second, "revalidation must end the wait promptly")
	<-serveDone
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Zero(t, upstreamCalls.Load(), "a rejected request must not forward")
	require.Greater(t, repoCalls.Load(), int32(1), "queued wait re-read the key through the auth path")
	require.Eventually(t, func() bool {
		_, waiting, statsErr := queueCache.GetAPIKeyQueueStats(context.Background(), key.ID)
		return statsErr == nil && waiting == 0
	}, time.Second, 5*time.Millisecond, "rejected waiter cleans its ticket")
}

// TestAPIKeyQueueMiddlewareLimitZeroTracksOnly verifies the real middleware
// path when the key's limit becomes 0 while waiting: the queued attempt is
// closed first, then the existing unlimited tracking path owns the request.
func TestAPIKeyQueueMiddlewareLimitZeroTracksOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := testutil.NewRedisConcurrencyCache(t)
	queueCache, ok := cache.(service.APIKeySlotQueueCache)
	require.True(t, ok)

	key := queueAuthTestKey(service.StatusActive, 1)
	current := key
	var currentMu sync.Mutex
	loadCurrent := func() *service.APIKey {
		currentMu.Lock()
		defer currentMu.Unlock()
		return current
	}
	setCurrent := func(next *service.APIKey) {
		currentMu.Lock()
		current = next
		currentMu.Unlock()
	}
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		return loadCurrent(), nil
	}}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)

	concurrencyService := service.NewConcurrencyService(cache)
	concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 1, Timeout: 3 * time.Second})

	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, key.ID, 1)
	require.NoError(t, err)
	require.NotNil(t, holder)
	defer holder.Release()

	tracked := make(chan *service.APIKeySlotReservation, 1)
	handlerResult := make(chan error, 1)
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.POST("/v1/messages", func(c *gin.Context) {
		reservation, reserveErr := concurrencyService.ReserveAPIKeySlotWithWait(c.Request.Context(), key.ID, 1)
		if reserveErr != nil {
			handlerResult <- reserveErr
			c.Status(http.StatusInternalServerError)
			return
		}
		tracked <- reservation
		handlerResult <- nil
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	request.Header.Set("x-api-key", key.Key)
	response := httptest.NewRecorder()
	serveDone := make(chan struct{})
	go func() {
		router.ServeHTTP(response, request)
		close(serveDone)
	}()

	require.Eventually(t, func() bool {
		_, waiting, statsErr := queueCache.GetAPIKeyQueueStats(context.Background(), key.ID)
		return statsErr == nil && waiting == 1
	}, 2*time.Second, 5*time.Millisecond, "request must be waiting before the key changes")

	setCurrent(queueAuthTestKey(service.StatusActive, 0))
	select {
	case reserveErr := <-handlerResult:
		require.NoError(t, reserveErr, "limit 0 continues on the unlimited tracking path")
	case <-time.After(3 * time.Second):
		t.Fatal("queued request did not switch to tracking after the limit changed")
	}
	<-serveDone
	require.Equal(t, http.StatusOK, response.Code)
	reservation := <-tracked
	require.True(t, reservation.StatsOnly(), "limit 0 has no enforced Key capacity")
	require.NotEmpty(t, reservation.RequestID(), "stats handle keeps an exact transfer identity")
	_, waiting, statsErr := queueCache.GetAPIKeyQueueStats(context.Background(), key.ID)
	require.NoError(t, statsErr)
	require.Zero(t, waiting, "queue ticket is closed before the tracking path starts")

	holder.Release()
	reservation.Release()
	_, active, statsErr := queueCache.GetAPIKeyQueueStats(context.Background(), key.ID)
	require.NoError(t, statsErr)
	require.Zero(t, active, "tracking member is released by its owner")
}

func TestAPIKeyQueueAuthRevalidatorFallbackWithoutServiceOrCredential(t *testing.T) {
	revalidate := newAPIKeyQueueAuthRevalidator(nil, "", "", nil)
	_, err := revalidate(context.Background())
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(err))

	revalidate = newAPIKeyQueueAuthRevalidator(nil, "cred", "", queueAuthTestKey(service.StatusActive, 1))
	_, err = revalidate(context.Background())
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(err))
}

// TestAPIKeyQueueMiddlewareAllowsBenignGroupEdits proves through the installed
// middleware callback that metadata, pricing and nil-vs-empty collection edits
// never abort either the immediate path or a real queued wait.
func TestAPIKeyQueueMiddlewareAllowsBenignGroupEdits(t *testing.T) {
	for _, tc := range []struct {
		name string
		wait bool
	}{
		{name: "immediate path", wait: false},
		{name: "blocked path", wait: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			cache := testutil.NewRedisConcurrencyCache(t)
			queueCache, ok := cache.(service.APIKeySlotQueueCache)
			require.True(t, ok)

			key := queueAuthTestKey(service.StatusActive, 1)
			current := key
			var currentMu sync.Mutex
			setCurrent := func(next *service.APIKey) {
				currentMu.Lock()
				current = next
				currentMu.Unlock()
			}
			repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
				currentMu.Lock()
				defer currentMu.Unlock()
				return current, nil
			}}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			apiKeyService := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
			concurrencyService := service.NewConcurrencyService(cache)
			concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 1, Timeout: 3 * time.Second})

			var holder *service.APIKeySlotReservation
			if tc.wait {
				holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
				defer cancelHolder()
				var err error
				holder, err = concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, key.ID, 1)
				require.NoError(t, err)
				require.NotNil(t, holder)
			}

			var upstreamCalls atomic.Int32
			handlerResult := make(chan error, 1)
			router := gin.New()
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
			router.POST("/v1/messages", func(c *gin.Context) {
				reservation, reserveErr := concurrencyService.ReserveAPIKeySlotWithWait(c.Request.Context(), key.ID, 1)
				if reserveErr == nil && reservation != nil {
					upstreamCalls.Add(1)
					reservation.Release()
					c.Status(http.StatusOK)
					handlerResult <- nil
					return
				}
				c.Status(http.StatusServiceUnavailable)
				handlerResult <- reserveErr
			})

			benign := queueAuthTestKey(service.StatusActive, 1)
			benign.Group.Name = "renamed-group"
			benign.Group.RateMultiplier = 3
			benign.Group.ModelRouting = map[string][]int64{} // initial nil
			setCurrent(benign)

			request := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			request.Header.Set("x-api-key", key.Key)
			response := httptest.NewRecorder()
			serveDone := make(chan struct{})
			go func() {
				router.ServeHTTP(response, request)
				close(serveDone)
			}()

			if tc.wait {
				require.Eventually(t, func() bool {
					_, waiting, statsErr := queueCache.GetAPIKeyQueueStats(context.Background(), key.ID)
					return statsErr == nil && waiting == 1
				}, 2*time.Second, 5*time.Millisecond, "request must be waiting before the second edit")

				renamedAgain := queueAuthTestKey(service.StatusActive, 1)
				renamedAgain.Group.Name = "renamed-again"
				renamedAgain.Group.ModelRouting = map[string][]int64{}
				setCurrent(renamedAgain)
				holder.Release()
			}

			select {
			case reserveErr := <-handlerResult:
				require.NoError(t, reserveErr, "benign group edits must not reject the wait")
			case <-time.After(3 * time.Second):
				t.Fatal("request did not finish after benign group edits")
			}
			<-serveDone
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, int32(1), upstreamCalls.Load(), "the admitted request forwards exactly once")
		})
	}
}

// TestAPIKeyQueueMiddlewareRejectsCoreIdentityChange proves the retryable 503
// end to end: a platform change while queued must not forward under the old
// route and must not surface as a 403 permission denial.
func TestAPIKeyQueueMiddlewareRejectsCoreIdentityChange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := testutil.NewRedisConcurrencyCache(t)
	queueCache, ok := cache.(service.APIKeySlotQueueCache)
	require.True(t, ok)

	key := queueAuthTestKey(service.StatusActive, 1)
	current := key
	var currentMu sync.Mutex
	setCurrent := func(next *service.APIKey) {
		currentMu.Lock()
		current = next
		currentMu.Unlock()
	}
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		currentMu.Lock()
		defer currentMu.Unlock()
		return current, nil
	}}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
	concurrencyService := service.NewConcurrencyService(cache)
	concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 1, Timeout: 3 * time.Second})

	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, key.ID, 1)
	require.NoError(t, err)
	defer holder.Release()

	var upstreamCalls atomic.Int32
	handlerResult := make(chan error, 1)
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.POST("/v1/messages", func(c *gin.Context) {
		reservation, reserveErr := concurrencyService.ReserveAPIKeySlotWithWait(c.Request.Context(), key.ID, 1)
		if reserveErr == nil && reservation != nil {
			upstreamCalls.Add(1)
			reservation.Release()
			c.Status(http.StatusOK)
			handlerResult <- nil
			return
		}
		c.Status(http.StatusServiceUnavailable)
		handlerResult <- reserveErr
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	request.Header.Set("x-api-key", key.Key)
	response := httptest.NewRecorder()
	serveDone := make(chan struct{})
	go func() {
		router.ServeHTTP(response, request)
		close(serveDone)
	}()

	require.Eventually(t, func() bool {
		_, waiting, statsErr := queueCache.GetAPIKeyQueueStats(context.Background(), key.ID)
		return statsErr == nil && waiting == 1
	}, 2*time.Second, 5*time.Millisecond, "request must be waiting before the platform changes")

	changed := queueAuthTestKey(service.StatusActive, 1)
	changed.Group.Platform = service.PlatformGrok
	setCurrent(changed)

	select {
	case reserveErr := <-handlerResult:
		require.True(t, service.IsAPIKeyQueueErrorKind(reserveErr, service.APIKeyQueueErrorAuthRejected))
		require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(reserveErr), "a core change is retryable, not a 403")
		require.Equal(t, "API_KEY_GROUP_CHANGED", infraerrors.Reason(reserveErr))
		require.Contains(t, infraerrors.Message(reserveErr), "configuration changed")
	case <-time.After(3 * time.Second):
		t.Fatal("request did not stop after the platform changed")
	}
	<-serveDone
	require.Zero(t, upstreamCalls.Load(), "a stale route must not forward")
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Eventually(t, func() bool {
		_, waiting, statsErr := queueCache.GetAPIKeyQueueStats(context.Background(), key.ID)
		return statsErr == nil && waiting == 0
	}, time.Second, 5*time.Millisecond, "rejected waiter cleans its ticket")
}

// TestAPIKeyQueueMiddlewareRevalidatesAllowlistEnabledWhileWaiting drives the
// real queue with the allowlist middleware mounted while the initial allowlist
// is disabled: capturing candidates must still apply an allowlist enabled later.
func TestAPIKeyQueueMiddlewareRevalidatesAllowlistEnabledWhileWaiting(t *testing.T) {
	for _, tc := range []struct {
		name        string
		models      []string
		expectError bool
	}{
		{name: "requested model removed", models: []string{"gpt-4"}, expectError: true},
		{name: "requested model kept with unrelated additions", models: []string{"gpt-4", "gpt-5.4", "gpt-5.4-nano"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			cache := testutil.NewRedisConcurrencyCache(t)
			queueCache, ok := cache.(service.APIKeySlotQueueCache)
			require.True(t, ok)

			key := queueAuthTestKey(service.StatusActive, 1)
			current := key
			var currentMu sync.Mutex
			setCurrent := func(next *service.APIKey) {
				currentMu.Lock()
				current = next
				currentMu.Unlock()
			}
			repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
				currentMu.Lock()
				defer currentMu.Unlock()
				return current, nil
			}}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			apiKeyService := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
			concurrencyService := service.NewConcurrencyService(cache)
			concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 1, Timeout: 3 * time.Second})

			holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
			defer cancelHolder()
			holder, err := concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, key.ID, 1)
			require.NoError(t, err)
			defer holder.Release()

			var upstreamCalls atomic.Int32
			handlerResult := make(chan error, 1)
			router := gin.New()
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
			router.Use(GroupModelAllowlist())
			router.POST("/v1/responses", func(c *gin.Context) {
				reservation, reserveErr := concurrencyService.ReserveAPIKeySlotWithWait(c.Request.Context(), key.ID, 1)
				if reserveErr == nil && reservation != nil {
					upstreamCalls.Add(1)
					reservation.Release()
					c.Status(http.StatusOK)
					handlerResult <- nil
					return
				}
				c.Status(http.StatusServiceUnavailable)
				handlerResult <- reserveErr
			})

			request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4"}`))
			request.Header.Set("x-api-key", key.Key)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			serveDone := make(chan struct{})
			go func() {
				router.ServeHTTP(response, request)
				close(serveDone)
			}()

			require.Eventually(t, func() bool {
				_, waiting, statsErr := queueCache.GetAPIKeyQueueStats(context.Background(), key.ID)
				return statsErr == nil && waiting == 1
			}, 2*time.Second, 5*time.Millisecond, "request must be waiting before the allowlist is enabled")

			enabled := queueAuthTestKey(service.StatusActive, 1)
			enabled.Group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: tc.models}
			setCurrent(enabled)
			holder.Release()

			select {
			case reserveErr := <-handlerResult:
				if !tc.expectError {
					require.NoError(t, reserveErr)
					break
				}
				require.True(t, service.IsAPIKeyQueueErrorKind(reserveErr, service.APIKeyQueueErrorAuthRejected))
				require.Equal(t, http.StatusNotFound, infraerrors.Code(reserveErr))
				require.Equal(t, "MODEL_NOT_ALLOWED", infraerrors.Reason(reserveErr))
			case <-time.After(3 * time.Second):
				t.Fatal("request did not finish after the allowlist changed")
			}
			<-serveDone
			if tc.expectError {
				require.Zero(t, upstreamCalls.Load(), "a revoked model must not forward")
				require.Equal(t, http.StatusServiceUnavailable, response.Code)
			} else {
				require.Equal(t, int32(1), upstreamCalls.Load())
				require.Equal(t, http.StatusOK, response.Code)
			}
			require.Eventually(t, func() bool {
				_, waiting, statsErr := queueCache.GetAPIKeyQueueStats(context.Background(), key.ID)
				return statsErr == nil && waiting == 0
			}, time.Second, 5*time.Millisecond, "the wait ticket is cleaned in both outcomes")
		})
	}
}
