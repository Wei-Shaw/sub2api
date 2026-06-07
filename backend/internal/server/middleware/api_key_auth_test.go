//go:build unit

package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthRejectsOversizedCredentialsBeforeLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var calls atomic.Int32
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		calls.Add(1)
		return nil, service.ErrAPIKeyNotFound
	}}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)

	for _, headers := range []map[string]string{
		{"x-api-key": strings.Repeat("x", service.MaxAPIKeyCredentialBytes+1)},
		{"Authorization": "Bearer " + strings.Repeat("x", service.MaxAPIKeyCredentialBytes+1)},
		{"Authorization": strings.Repeat("x", maxAPIKeyAuthorizationHeaderBytes+1)},
	} {
		r := gin.New()
		r.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)))
		r.GET("/t", func(c *gin.Context) { c.Status(http.StatusOK) })
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		for name, value := range headers {
			req.Header.Set(name, value)
		}
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	}
	require.Zero(t, calls.Load())
}

func TestProviderIDForAutoSwitchPlatformKeepsProviderPlatformsDistinct(t *testing.T) {
	tests := []struct {
		platform string
		want     string
	}{
		{platform: "", want: ""},
		{platform: service.PlatformOpenAI, want: "v-claw-openai"},
		{platform: service.PlatformAnthropic, want: "v-claw-anthropic"},
		{platform: service.PlatformKiro, want: "v-claw-kiro"},
		{platform: service.PlatformGemini, want: "v-claw-gemini"},
		{platform: service.PlatformAntigravity, want: "v-claw-antigravity"},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			require.Equal(t, tt.want, providerIDForAutoSwitchPlatform(tt.platform))
		})
	}
}

func TestSimpleModeBypassesQuotaCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	limit := 1.0
	group := &service.Group{
		ID:               42,
		Name:             "sub",
		Status:           service.StatusActive,
		Hydrated:         true,
		SubscriptionType: service.SubscriptionTypeSubscription,
		DailyLimitUSD:    &limit,
	}
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "test-key",
		Status: service.StatusActive,
		User:   user,
		Group:  group,
	}
	apiKey.GroupID = &group.ID

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	t.Run("standard_mode_completes_maintenance_before_request", func(t *testing.T) {
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.SubscriptionMaintenance.WorkerCount = 1
		cfg.SubscriptionMaintenance.QueueSize = 1

		apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)

		past := time.Now().Add(-48 * time.Hour)
		sub := &service.UserSubscription{
			ID:                 55,
			UserID:             user.ID,
			GroupID:            group.ID,
			Status:             service.SubscriptionStatusActive,
			ExpiresAt:          time.Now().Add(24 * time.Hour),
			DailyWindowStart:   &past,
			WeeklyWindowStart:  &past,
			MonthlyWindowStart: &past,
			DailyUsageUSD:      0,
		}
		maintenanceCalled := make(chan struct{}, 1)
		subscriptionRepo := &stubUserSubscriptionRepo{
			getByID: func(ctx context.Context, id int64) (*service.UserSubscription, error) {
				clone := *sub
				return &clone, nil
			},
			getActive: func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
				clone := *sub
				return &clone, nil
			},
			updateStatus:   func(ctx context.Context, subscriptionID int64, status string) error { return nil },
			activateWindow: func(ctx context.Context, id int64, start time.Time) error { return nil },
			resetDaily: func(ctx context.Context, id int64, start time.Time) error {
				sub.DailyWindowStart = &start
				sub.DailyUsageUSD = 0
				maintenanceCalled <- struct{}{}
				return nil
			},
			resetWeekly: func(ctx context.Context, id int64, start time.Time) error {
				sub.WeeklyWindowStart = &start
				return nil
			},
			resetMonthly: func(ctx context.Context, id int64, start time.Time) error {
				sub.MonthlyWindowStart = &start
				return nil
			},
		}
		subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, cfg)
		t.Cleanup(subscriptionService.Stop)

		router := newAuthTestRouter(apiKeyService, subscriptionService, cfg)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("x-api-key", apiKey.Key)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		select {
		case <-maintenanceCalled:
			// ok
		case <-time.After(time.Second):
			t.Fatalf("expected maintenance to complete before response")
		}
	})

	t.Run("standard_mode_revalidates_cas_loser_from_database", func(t *testing.T) {
		cfg := &config.Config{RunMode: config.RunModeStandard}
		apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)

		past := time.Now().Add(-48 * time.Hour)
		current := time.Now()
		stale := &service.UserSubscription{
			ID:                 56,
			UserID:             user.ID,
			GroupID:            group.ID,
			Status:             service.SubscriptionStatusActive,
			ExpiresAt:          current.Add(24 * time.Hour),
			DailyWindowStart:   &past,
			WeeklyWindowStart:  &past,
			MonthlyWindowStart: &past,
			DailyUsageUSD:      10,
		}
		fresh := *stale
		fresh.DailyWindowStart = &current
		fresh.WeeklyWindowStart = &current
		fresh.MonthlyWindowStart = &current
		fresh.DailyUsageUSD = 2

		subscriptionRepo := &stubUserSubscriptionRepo{
			getActive: func(context.Context, int64, int64) (*service.UserSubscription, error) {
				clone := *stale
				return &clone, nil
			},
			getByID: func(context.Context, int64) (*service.UserSubscription, error) {
				clone := fresh
				return &clone, nil
			},
			resetDaily:   func(context.Context, int64, time.Time) error { return nil },
			resetWeekly:  func(context.Context, int64, time.Time) error { return nil },
			resetMonthly: func(context.Context, int64, time.Time) error { return nil },
		}
		subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, cfg)
		router := newAuthTestRouter(apiKeyService, subscriptionService, cfg)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("x-api-key", apiKey.Key)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusTooManyRequests, w.Code)
	})

	t.Run("simple_mode_bypasses_quota_check", func(t *testing.T) {
		cfg := &config.Config{RunMode: config.RunModeSimple}
		apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
		subscriptionService := service.NewSubscriptionService(nil, &stubUserSubscriptionRepo{}, nil, nil, cfg)
		router := newAuthTestRouter(apiKeyService, subscriptionService, cfg)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("x-api-key", apiKey.Key)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("simple_mode_accepts_lowercase_bearer", func(t *testing.T) {
		cfg := &config.Config{RunMode: config.RunModeSimple}
		apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
		subscriptionService := service.NewSubscriptionService(nil, &stubUserSubscriptionRepo{}, nil, nil, cfg)
		router := newAuthTestRouter(apiKeyService, subscriptionService, cfg)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("Authorization", "bearer "+apiKey.Key)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("standard_mode_enforces_quota_check", func(t *testing.T) {
		cfg := &config.Config{RunMode: config.RunModeStandard}
		apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)

		now := time.Now()
		sub := &service.UserSubscription{
			ID:               55,
			UserID:           user.ID,
			GroupID:          group.ID,
			Status:           service.SubscriptionStatusActive,
			ExpiresAt:        now.Add(24 * time.Hour),
			DailyWindowStart: &now,
			DailyUsageUSD:    10,
		}
		subscriptionRepo := &stubUserSubscriptionRepo{
			getActive: func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
				if userID != sub.UserID || groupID != sub.GroupID {
					return nil, service.ErrSubscriptionNotFound
				}
				clone := *sub
				return &clone, nil
			},
			updateStatus:   func(ctx context.Context, subscriptionID int64, status string) error { return nil },
			activateWindow: func(ctx context.Context, id int64, start time.Time) error { return nil },
			resetDaily:     func(ctx context.Context, id int64, start time.Time) error { return nil },
			resetWeekly:    func(ctx context.Context, id int64, start time.Time) error { return nil },
			resetMonthly:   func(ctx context.Context, id int64, start time.Time) error { return nil },
		}
		subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, cfg)
		router := newAuthTestRouter(apiKeyService, subscriptionService, cfg)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("x-api-key", apiKey.Key)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusTooManyRequests, w.Code)
		require.Contains(t, w.Body.String(), "USAGE_LIMIT_EXCEEDED")
		require.Equal(t, "subscription_limit_exceeded", w.Header().Get("X-Sub2API-Billing-Code"))
		require.Equal(t, "true", w.Header().Get("X-Sub2API-Auto-Switchable"))
		var body ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, "subscription_limit_exceeded", body.Metadata["billing_code"])
		require.Equal(t, true, body.Metadata["auto_switchable"])
	})
}

func TestAPIKeyAuthSetsGroupContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:       101,
		Name:     "g1",
		Status:   service.StatusActive,
		Platform: service.PlatformAnthropic,
		Hydrated: true,
	}
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "test-key",
		Status: service.StatusActive,
		User:   user,
		Group:  group,
	}
	apiKey.GroupID = &group.ID

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.GET("/t", func(c *gin.Context) {
		groupFromCtx, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group)
		if !ok || groupFromCtx == nil || groupFromCtx.ID != group.ID {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false})
			return
		}
		userIDFromCtx, ok := c.Request.Context().Value(ctxkey.UserID).(int64)
		if !ok || userIDFromCtx != user.ID {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthRejectsExclusiveGroupWhenUserNoLongerAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:          202,
		Name:        "exclusive",
		Status:      service.StatusActive,
		IsExclusive: true,
		Hydrated:    true,
	}
	user := &service.User{
		ID:            7,
		Role:          service.RoleUser,
		Status:        service.StatusActive,
		Balance:       10,
		Concurrency:   3,
		AllowedGroups: []int64{},
	}
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "test-key",
		Status: service.StatusActive,
		User:   user,
		Group:  group,
	}
	apiKey.GroupID = &group.ID

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, nil, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "GROUP_NOT_ALLOWED")
}

func TestAPIKeyAuthOverwritesInvalidContextGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:       101,
		Name:     "g1",
		Status:   service.StatusActive,
		Platform: service.PlatformAnthropic,
		Hydrated: true,
	}
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "test-key",
		Status: service.StatusActive,
		User:   user,
		Group:  group,
	}
	apiKey.GroupID = &group.ID

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))

	invalidGroup := &service.Group{
		ID:       group.ID,
		Platform: group.Platform,
		Status:   group.Status,
	}
	router.GET("/t", func(c *gin.Context) {
		groupFromCtx, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group)
		if !ok || groupFromCtx == nil || groupFromCtx.ID != group.ID || !groupFromCtx.Hydrated || groupFromCtx == invalidGroup {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, invalidGroup))
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthRejectsUnavailableGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(101)
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}

	tests := []struct {
		name       string
		group      *service.Group
		wantStatus int
		wantCode   string
		wantMarked bool
		wantReject IngressRejectReason
	}{
		{
			name: "active group passes",
			group: &service.Group{
				ID:       groupID,
				Name:     "active",
				Status:   service.StatusActive,
				Platform: service.PlatformAnthropic,
				Hydrated: true,
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "disabled group is forbidden",
			group: &service.Group{
				ID:       groupID,
				Name:     "disabled",
				Status:   service.StatusDisabled,
				Platform: service.PlatformAnthropic,
				Hydrated: true,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "GROUP_DISABLED",
			wantMarked: true,
			wantReject: IngressRejectGroupDisabled,
		},
		{
			name: "deleted status group is forbidden",
			group: &service.Group{
				ID:       groupID,
				Name:     "deleted",
				Status:   "deleted",
				Platform: service.PlatformAnthropic,
				Hydrated: true,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "GROUP_DELETED",
			wantMarked: true,
			wantReject: IngressRejectGroupDeleted,
		},
		{
			name:       "missing group edge is forbidden",
			group:      nil,
			wantStatus: http.StatusForbidden,
			wantCode:   "GROUP_DELETED",
			wantMarked: true,
			wantReject: IngressRejectGroupDeleted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiKey := &service.APIKey{
				ID:      100,
				UserID:  user.ID,
				GroupID: &groupID,
				Key:     "test-key",
				Status:  service.StatusActive,
				User:    user,
				Group:   tt.group,
			}
			apiKeyRepo := &stubApiKeyRepo{
				getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
					if key != apiKey.Key {
						return nil, service.ErrAPIKeyNotFound
					}
					clone := *apiKey
					return &clone, nil
				},
			}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
			router := gin.New()
			var markedBusinessLimited bool
			var businessLimitedReason string
			var rejectReason IngressRejectReason
			var rejected bool
			router.Use(func(c *gin.Context) {
				c.Next()
				markedBusinessLimited = service.HasOpsClientBusinessLimited(c)
				rejectReason, rejected = GetIngressRejectReason(c)
				if v, ok := c.Get(service.OpsClientBusinessLimitedReasonKey); ok {
					businessLimitedReason, _ = v.(string)
				}
			})
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
			router.GET("/t", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/t", nil)
			req.Header.Set("x-api-key", apiKey.Key)
			router.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			if tt.wantCode != "" {
				require.Contains(t, w.Body.String(), tt.wantCode)
			}
			require.Equal(t, tt.wantMarked, markedBusinessLimited)
			require.Equal(t, tt.wantReject != "", rejected)
			require.Equal(t, tt.wantReject, rejectReason)
			if tt.wantMarked {
				require.Equal(t, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnavailable, businessLimitedReason)
			}
		})
	}
}

func TestAPIKeyAuthMarksOnlyExpectedIngressRejections(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		path       string
		key        string
		authHeader string
		repoErr    error
		wantStatus int
		wantCode   string
		wantReason IngressRejectReason
	}{
		{
			name:       "query key deprecated",
			path:       "/t?key=legacy",
			wantStatus: http.StatusBadRequest,
			wantCode:   "api_key_in_query_deprecated",
			wantReason: IngressRejectQueryAPIKeyDeprecated,
		},
		{
			name:       "missing key",
			path:       "/t",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "API_KEY_REQUIRED",
			wantReason: IngressRejectAPIKeyRequired,
		},
		{
			name:       "malformed authorization",
			path:       "/t",
			authHeader: "Basic not-a-bearer-key",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "API_KEY_REQUIRED",
			wantReason: IngressRejectInvalidAPIKey,
		},
		{
			name:       "oversized key",
			path:       "/t",
			key:        strings.Repeat("x", service.MaxAPIKeyCredentialBytes+1),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "INVALID_API_KEY",
			wantReason: IngressRejectInvalidAPIKey,
		},
		{
			name:       "invalid key",
			path:       "/t",
			key:        "invalid",
			repoErr:    service.ErrAPIKeyNotFound,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "INVALID_API_KEY",
			wantReason: IngressRejectInvalidAPIKey,
		},
		{
			name:       "repository failure remains operational error",
			path:       "/t",
			key:        "valid-shape",
			repoErr:    errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
		{
			name:       "auth lookup bulkhead rejection is an admission rejection",
			path:       "/t",
			key:        "valid-shape",
			repoErr:    service.ErrAPIKeyAuthOverloaded,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "API_KEY_AUTH_OVERLOADED",
			wantReason: IngressRejectAPIKeyAuthOverloaded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
				return nil, tt.repoErr
			}}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			apiKeyService := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
			router := gin.New()
			var reason IngressRejectReason
			var rejected bool
			router.Use(func(c *gin.Context) {
				c.Next()
				reason, rejected = GetIngressRejectReason(c)
			})
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
			router.GET("/t", func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.key != "" {
				req.Header.Set("x-api-key", tt.key)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			router.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			require.Contains(t, w.Body.String(), tt.wantCode)
			require.Equal(t, tt.wantReason != "", rejected)
			require.Equal(t, tt.wantReason, reason)
		})
	}
}

func TestAPIKeyAuthSetsOpsFallbackKeyOnEarlyAbort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(101)
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:      100,
		UserID:  user.ID,
		GroupID: &groupID,
		Key:     "test-key",
		Status:  service.StatusActive,
		User:    user,
		Group: &service.Group{
			ID:       groupID,
			Name:     "disabled",
			Status:   service.StatusDisabled,
			Platform: service.PlatformAnthropic,
			Hydrated: true,
		},
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)

	router := gin.New()
	var fallback *service.APIKey
	var fallbackOK bool
	router.Use(func(c *gin.Context) {
		c.Next()
		fallback, fallbackOK = GetOpsFallbackAPIKey(c)
	})
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	// 分组停用 → 早退中断，但 ops fallback key 仍应写入，含 user/group/platform。
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "GROUP_DISABLED")
	require.True(t, fallbackOK, "鉴权早退时也应写入 ops fallback api key")
	require.NotNil(t, fallback)
	require.Equal(t, apiKey.ID, fallback.ID)
	require.NotNil(t, fallback.User)
	require.Equal(t, user.ID, fallback.User.ID)
	require.NotNil(t, fallback.GroupID)
	require.Equal(t, groupID, *fallback.GroupID)
	require.NotNil(t, fallback.Group)
	require.Equal(t, service.PlatformAnthropic, fallback.Group.Platform)
}

func TestAPIKeyAuthGoogleSetsOpsFallbackKeyOnEarlyAbort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(202)
	user := &service.User{
		ID:          9,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:      200,
		UserID:  user.ID,
		GroupID: &groupID,
		Key:     "g-key",
		Status:  service.StatusActive,
		User:    user,
		Group: &service.Group{
			ID:       groupID,
			Name:     "disabled",
			Status:   service.StatusDisabled,
			Platform: service.PlatformGemini,
			Hydrated: true,
		},
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)

	router := gin.New()
	var fallback *service.APIKey
	var fallbackOK bool
	router.Use(func(c *gin.Context) {
		c.Next()
		fallback, fallbackOK = GetOpsFallbackAPIKey(c)
	})
	router.Use(gin.HandlerFunc(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-goog-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.True(t, fallbackOK, "Google 鉴权早退时也应写入 ops fallback api key")
	require.NotNil(t, fallback)
	require.Equal(t, apiKey.ID, fallback.ID)
	require.NotNil(t, fallback.User)
	require.Equal(t, user.ID, fallback.User.ID)
}

func TestRequireGroupAssignmentMarksUngroupedKeyBusinessLimited(t *testing.T) {
	gin.SetMode(gin.TestMode)

	settingService := service.NewSettingService(fakeSettingRepo{
		values: map[string]string{
			service.SettingKeyAllowUngroupedKeyScheduling: "false",
		},
	}, &config.Config{})
	apiKey := &service.APIKey{
		ID:     100,
		Key:    "ungrouped-key",
		Status: service.StatusActive,
	}

	router := gin.New()
	var markedBusinessLimited bool
	var businessLimitedReason string
	var rejectReason IngressRejectReason
	var rejected bool
	router.Use(func(c *gin.Context) {
		c.Next()
		markedBusinessLimited = service.HasOpsClientBusinessLimited(c)
		rejectReason, rejected = GetIngressRejectReason(c)
		if v, ok := c.Get(service.OpsClientBusinessLimitedReasonKey); ok {
			businessLimitedReason, _ = v.(string)
		}
	})
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), apiKey)
		c.Next()
	})
	router.Use(RequireGroupAssignment(settingService, AnthropicErrorWriter))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "not assigned to any group")
	require.True(t, rejected)
	require.Equal(t, IngressRejectGroupUnassigned, rejectReason)
	require.True(t, markedBusinessLimited)
	require.Equal(t, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnassigned, businessLimitedReason)
}

func TestAPIKeyAuthIPRestrictionUsesTrustedPathWhenSwitchDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:          100,
		UserID:      user.ID,
		Key:         "test-key",
		Status:      service.StatusActive,
		User:        user,
		IPWhitelist: []string{"1.2.3.4"},
	}

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.SetTrustForwardedIPForAPIKeyACL(false)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	var markedBusinessLimited bool
	var businessLimitedReason string
	router.Use(func(c *gin.Context) {
		c.Next()
		markedBusinessLimited = service.HasOpsClientBusinessLimited(c)
		if v, ok := c.Get(service.OpsClientBusinessLimitedReasonKey); ok {
			businessLimitedReason, _ = v.(string)
		}
	})
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.RemoteAddr = "9.9.9.9:12345"
	req.Header.Set("x-api-key", apiKey.Key)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "1.2.3.4")
	req.Header.Set("CF-Connecting-IP", "1.2.3.4")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	requireAPIKeyAuthError(t, w, "ACCESS_DENIED", "Access denied. Your IP is 9.9.9.9")
	require.True(t, markedBusinessLimited)
	require.Equal(t, service.OpsClientBusinessLimitedReasonIPRestriction, businessLimitedReason)
}

func TestAPIKeyAuthIPRestrictionIncludesClientIPForBlacklistDenial(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:          100,
		UserID:      user.ID,
		Key:         "test-key",
		Status:      service.StatusActive,
		User:        user,
		IPBlacklist: []string{"9.9.9.9"},
	}

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.RemoteAddr = "9.9.9.9:12345"
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	requireAPIKeyAuthError(t, w, "ACCESS_DENIED", "Access denied. Your IP is 9.9.9.9")
}

func TestAPIKeyAuthIPRestrictionUsesConfiguredTrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:          100,
		UserID:      user.ID,
		Key:         "test-key",
		Status:      service.StatusActive,
		User:        user,
		IPWhitelist: []string{"1.2.3.4"},
	}

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.SetTrustForwardedIPForAPIKeyACL(false)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies([]string{"9.9.9.9"}))
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.RemoteAddr = "9.9.9.9:12345"
	req.Header.Set("x-api-key", apiKey.Key)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "1.2.3.4")
	req.Header.Set("CF-Connecting-IP", "1.2.3.4")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthIPRestrictionUsesForwardedClientIPInDenialWhenTrusted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:          100,
		UserID:      user.ID,
		Key:         "test-key",
		Status:      service.StatusActive,
		User:        user,
		IPWhitelist: []string{"9.9.9.9"},
	}

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.SetTrustForwardedIPForAPIKeyACL(false)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies([]string{"9.9.9.9"}))
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.RemoteAddr = "9.9.9.9:12345"
	req.Header.Set("x-api-key", apiKey.Key)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "1.2.3.4")
	req.Header.Set("CF-Connecting-IP", "1.2.3.4")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	requireAPIKeyAuthError(t, w, "ACCESS_DENIED", "Access denied. Your IP is 1.2.3.4")
}

func TestAPIKeyAuthTouchesLastUsedOnSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "touch-ok",
		Status: service.StatusActive,
		User:   user,
	}

	var touchedID int64
	var touchedAt time.Time
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			touchedID = id
			touchedAt = usedAt
			return nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, nil, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, apiKey.ID, touchedID)
	require.False(t, touchedAt.IsZero(), "expected touch timestamp")
}

func TestAPIKeyAuthTouchLastUsedFailureDoesNotBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          8,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     101,
		UserID: user.ID,
		Key:    "touch-fail",
		Status: service.StatusActive,
		User:   user,
	}

	touchCalls := 0
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			touchCalls++
			return errors.New("db unavailable")
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, nil, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "touch failure should not block request")
	require.Equal(t, 1, touchCalls)
}

func TestAPIKeyAuthTouchesLastUsedInStandardMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          9,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     102,
		UserID: user.ID,
		Key:    "touch-standard",
		Status: service.StatusActive,
		User:   user,
	}

	touchCalls := 0
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			touchCalls++
			return nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, nil, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, touchCalls)
}

func TestAPIKeyAuthBillingInfoSkipsBillingAndSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:               42,
		Name:             "subscription",
		Status:           service.StatusActive,
		Hydrated:         true,
		SubscriptionType: service.SubscriptionTypeSubscription,
	}
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     0,
		Concurrency: 3,
	}
	expiredAt := time.Now().Add(-time.Hour)
	apiKey := &service.APIKey{
		ID:        100,
		UserID:    user.ID,
		Key:       "billing-info-auth-only",
		Status:    service.StatusAPIKeyQuotaExhausted,
		User:      user,
		GroupID:   &group.ID,
		Group:     group,
		Quota:     1,
		QuotaUsed: 1,
		ExpiresAt: &expiredAt,
	}

	touchCalls := 0
	subscriptionCalls := 0
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(context.Context, string) (*service.APIKey, error) {
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(context.Context, int64, time.Time) error {
			touchCalls++
			return nil
		},
	}
	subscriptionRepo := &stubUserSubscriptionRepo{
		getActive: func(context.Context, int64, int64) (*service.UserSubscription, error) {
			subscriptionCalls++
			return nil, service.ErrSubscriptionNotFound
		},
	}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, cfg)
	t.Cleanup(subscriptionService.Stop)
	router := newAuthTestRouter(apiKeyService, subscriptionService, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/sub2api/billing", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Zero(t, subscriptionCalls)
	require.Zero(t, touchCalls)
}

func TestAPIKeyAuthBillingInfoSkipsLastUsedInSimpleMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive}
	apiKey := &service.APIKey{ID: 100, UserID: user.ID, Key: "billing-info-simple", Status: service.StatusActive, User: user}
	touchCalls := 0
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(context.Context, string) (*service.APIKey, error) {
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(context.Context, int64, time.Time) error {
			touchCalls++
			return nil
		},
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, nil, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/sub2api/billing", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Zero(t, touchCalls)
}

func TestAPIKeyAuthUsageStillTouchesLastUsed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 10}
	apiKey := &service.APIKey{ID: 100, UserID: user.ID, Key: "usage-touch", Status: service.StatusActive, User: user}
	touchCalls := 0
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(context.Context, string) (*service.APIKey, error) {
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(context.Context, int64, time.Time) error {
			touchCalls++
			return nil
		},
	}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, nil, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, touchCalls)
}

func TestAPIKeyAuthAllowsBalanceBelowMinimumReserve(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          10,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     0.005,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     103,
		UserID: user.ID,
		Key:    "held-balance-low",
		Status: service.StatusActive,
		User:   user,
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			userClone := *user
			clone.User = &userClone
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Billing.MinimumBalanceReserve = 0.01
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, nil, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	// 鉴权层保持历史语义：MinimumBalanceReserve 只用于 billing-cache 预检，
	// 0 < balance < reserve 不得被鉴权中间件硬 403（存量部署静默行为变更）。
	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthRejectsExhaustedBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          10,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     0,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     104,
		UserID: user.ID,
		Key:    "held-balance-zero",
		Status: service.StatusActive,
		User:   user,
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			userClone := *user
			clone.User = &userClone
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, nil, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	requireAPIKeyAuthError(t, w, "INSUFFICIENT_BALANCE", "Insufficient account balance")
}

func TestAPIKeyAuthOpenAIQuotaErrorFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{ID: 11, Role: service.RoleUser, Status: service.StatusActive, Balance: 10}
	group := &service.Group{ID: 8, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	apiKey := &service.APIKey{
		ID: 105, UserID: user.ID, Key: "openai-quota-exhausted", Status: service.StatusAPIKeyQuotaExhausted,
		User: user, Group: group, GroupID: &group.ID,
	}
	apiKeyRepo := &stubApiKeyRepo{getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
		if key != apiKey.Key {
			return nil, service.ErrAPIKeyNotFound
		}
		clone := *apiKey
		userClone := *user
		clone.User = &userClone
		return &clone, nil
	}}

	cfg := &config.Config{RunMode: config.RunModeStandard}
	router := newAuthTestRouter(service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg), nil, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	var response struct {
		Error struct {
			Message string  `json:"message"`
			Type    string  `json:"type"`
			Param   *string `json:"param"`
			Code    string  `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, "API key 额度已用完", response.Error.Message)
	require.Equal(t, "insufficient_quota", response.Error.Type)
	require.Nil(t, response.Error.Param)
	require.Equal(t, "insufficient_quota", response.Error.Code)
}

func TestAPIKeyAuthQuotaErrorKeepsLegacyFormatOutsideResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{ID: 11, Role: service.RoleUser, Status: service.StatusActive, Balance: 10}
	group := &service.Group{ID: 8, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	apiKey := &service.APIKey{
		ID: 105, UserID: user.ID, Key: "openai-quota-exhausted", Status: service.StatusAPIKeyQuotaExhausted,
		User: user, Group: group, GroupID: &group.ID,
	}
	apiKeyRepo := &stubApiKeyRepo{getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
		if key != apiKey.Key {
			return nil, service.ErrAPIKeyNotFound
		}
		clone := *apiKey
		userClone := *user
		clone.User = &userClone
		return &clone, nil
	}}

	cfg := &config.Config{RunMode: config.RunModeStandard}
	router := newAuthTestRouter(service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg), nil, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	requireAPIKeyAuthError(t, w, "API_KEY_QUOTA_EXHAUSTED", "API key 额度已用完")
}

func TestAPIKeyAuthAutoSwitchesExpiredBootstrapSubscriptionKeyToNewSubscription(t *testing.T) {
	gin.SetMode(gin.TestMode)

	monthlyLimit := 50.0
	oldGroup := &service.Group{ID: 41, Name: "free-trial", Status: service.StatusActive, Platform: service.PlatformOpenAI, Hydrated: true, SubscriptionType: service.SubscriptionTypeSubscription, MonthlyLimitUSD: &monthlyLimit}
	newGroup := &service.Group{ID: 42, Name: "paid-subscription", Status: service.StatusActive, Platform: service.PlatformOpenAI, Hydrated: true, SubscriptionType: service.SubscriptionTypeSubscription, MonthlyLimitUSD: &monthlyLimit}
	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 10, Concurrency: 3}
	keyStore := map[int64]*service.APIKey{}
	currentKey := &service.APIKey{ID: 100, UserID: user.ID, Key: "bootstrap-key", Status: service.StatusActive, User: user, GroupID: &oldGroup.ID, Group: oldGroup}
	keyStore[currentKey.ID] = currentKey

	cloneStoredKey := func(key *service.APIKey) *service.APIKey {
		if key == nil {
			return nil
		}
		clone := *key
		if key.GroupID != nil {
			groupID := *key.GroupID
			clone.GroupID = &groupID
		}
		return &clone
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != currentKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(keyStore[currentKey.ID]), nil
		},
		getByID: func(ctx context.Context, id int64) (*service.APIKey, error) {
			key := keyStore[id]
			if key == nil {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(key), nil
		},
		listByUserID: func(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
			require.Equal(t, user.ID, userID)
			clone := cloneStoredKey(keyStore[currentKey.ID])
			return []service.APIKey{*clone}, &pagination.PaginationResult{Total: 1, Page: 1, PageSize: 1000}, nil
		},
	}
	now := time.Now()
	activeSub := service.UserSubscription{ID: 55, UserID: user.ID, GroupID: newGroup.ID, Status: service.SubscriptionStatusActive, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(90 * 24 * time.Hour), MonthlyWindowStart: &now, MonthlyUsageUSD: 10}
	subscriptionRepo := &stubUserSubscriptionRepo{
		getActive: func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
			if userID == activeSub.UserID && groupID == activeSub.GroupID {
				clone := activeSub
				return &clone, nil
			}
			return nil, service.ErrSubscriptionNotFound
		},
		listByUserID: func(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
			require.Equal(t, user.ID, userID)
			return []service.UserSubscription{
				{ID: 54, UserID: user.ID, GroupID: oldGroup.ID, Status: service.SubscriptionStatusExpired, StartsAt: now.Add(-96 * time.Hour), ExpiresAt: now.Add(-24 * time.Hour)},
				activeSub,
			}, nil
		},
		updateStatus:   func(ctx context.Context, subscriptionID int64, status string) error { return nil },
		activateWindow: func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetDaily:     func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetWeekly:    func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetMonthly:   func(ctx context.Context, id int64, start time.Time) error { return nil },
	}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeStandard})
	subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, &config.Config{RunMode: config.RunModeStandard})
	groups := map[int64]*service.Group{oldGroup.ID: oldGroup, newGroup.ID: newGroup}
	entitlementService := service.NewEntitlementService(
		&stubEntitlementUserRepo{user: user},
		&stubEntitlementGroupRepo{groups: groups},
		&stubEntitlementAPIKeyUpdater{update: func(ctx context.Context, id, userID int64, req service.UpdateAPIKeyRequest) (*service.APIKey, error) {
			require.Equal(t, currentKey.ID, id)
			require.Equal(t, user.ID, userID)
			require.NotNil(t, req.GroupID)
			require.Equal(t, newGroup.ID, *req.GroupID)
			stored := keyStore[id]
			stored.GroupID = req.GroupID
			stored.Group = newGroup
			return cloneStoredKey(stored), nil
		}},
		apiKeyRepo,
		subscriptionRepo,
	)

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddlewareWithEntitlements(apiKeyService, subscriptionService, entitlementService, &config.Config{RunMode: config.RunModeStandard})))
	router.GET("/t", func(c *gin.Context) {
		keyFromCtx, ok := GetAPIKeyFromContext(c)
		require.True(t, ok)
		require.NotNil(t, keyFromCtx.GroupID)
		require.Equal(t, newGroup.ID, *keyFromCtx.GroupID)
		subFromCtx, hasSubscription := GetSubscriptionFromContext(c)
		require.True(t, hasSubscription)
		require.Equal(t, activeSub.ID, subFromCtx.ID)
		c.JSON(http.StatusOK, gin.H{"api_key_id": keyFromCtx.ID, "group_id": *keyFromCtx.GroupID, "subscription_id": subFromCtx.ID})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", currentKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "true", w.Header().Get("X-Sub2API-Auto-Switched"))
	require.Equal(t, newGroup.Name, w.Header().Get("X-Sub2API-Auto-Switch-Target-Group"))
	require.Contains(t, w.Body.String(), `"group_id":42`)
	require.Contains(t, w.Body.String(), `"subscription_id":55`)
}

func TestAPIKeyAuthAutoSwitchesExpiredBootstrapSubscriptionKeyToBalanceGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	monthlyLimit := 50.0
	oldGroup := &service.Group{ID: 41, Name: "free-trial", Status: service.StatusActive, Platform: service.PlatformOpenAI, Hydrated: true, SubscriptionType: service.SubscriptionTypeSubscription, MonthlyLimitUSD: &monthlyLimit}
	balanceGroup := &service.Group{ID: 42, Name: "balance-credit", Status: service.StatusActive, Platform: service.PlatformOpenAI, Hydrated: true, SubscriptionType: service.SubscriptionTypeStandard}
	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 10, Concurrency: 3}
	keyStore := map[int64]*service.APIKey{}
	currentKey := &service.APIKey{ID: 100, UserID: user.ID, Key: "bootstrap-key", Status: service.StatusActive, User: user, GroupID: &oldGroup.ID, Group: oldGroup}
	otherBalanceKey := &service.APIKey{ID: 101, UserID: user.ID, Key: "other-balance-key", Status: service.StatusActive, User: user, GroupID: &balanceGroup.ID, Group: balanceGroup}
	keyStore[currentKey.ID] = currentKey
	keyStore[otherBalanceKey.ID] = otherBalanceKey

	cloneStoredKey := func(key *service.APIKey) *service.APIKey {
		if key == nil {
			return nil
		}
		clone := *key
		if key.GroupID != nil {
			groupID := *key.GroupID
			clone.GroupID = &groupID
		}
		return &clone
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != currentKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(keyStore[currentKey.ID]), nil
		},
		getByID: func(ctx context.Context, id int64) (*service.APIKey, error) {
			key := keyStore[id]
			if key == nil {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(key), nil
		},
		listByUserID: func(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
			require.Equal(t, user.ID, userID)
			current := cloneStoredKey(keyStore[currentKey.ID])
			other := cloneStoredKey(keyStore[otherBalanceKey.ID])
			return []service.APIKey{*current, *other}, &pagination.PaginationResult{Total: 2, Page: 1, PageSize: 1000}, nil
		},
	}
	now := time.Now()
	subscriptionRepo := &stubUserSubscriptionRepo{
		getActive: func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
			return nil, service.ErrSubscriptionNotFound
		},
		listByUserID: func(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
			require.Equal(t, user.ID, userID)
			return []service.UserSubscription{
				{ID: 54, UserID: user.ID, GroupID: oldGroup.ID, Status: service.SubscriptionStatusExpired, StartsAt: now.Add(-96 * time.Hour), ExpiresAt: now.Add(-24 * time.Hour)},
			}, nil
		},
		updateStatus:   func(ctx context.Context, subscriptionID int64, status string) error { return nil },
		activateWindow: func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetDaily:     func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetWeekly:    func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetMonthly:   func(ctx context.Context, id int64, start time.Time) error { return nil },
	}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeStandard})
	subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, &config.Config{RunMode: config.RunModeStandard})
	groups := map[int64]*service.Group{oldGroup.ID: oldGroup, balanceGroup.ID: balanceGroup}
	entitlementService := service.NewEntitlementService(
		&stubEntitlementUserRepo{user: user},
		&stubEntitlementGroupRepo{groups: groups},
		&stubEntitlementAPIKeyUpdater{update: func(ctx context.Context, id, userID int64, req service.UpdateAPIKeyRequest) (*service.APIKey, error) {
			require.Equal(t, currentKey.ID, id, "gateway continuity must rebind the request key, not switch to another balance key")
			require.Equal(t, user.ID, userID)
			require.NotNil(t, req.GroupID)
			require.Equal(t, balanceGroup.ID, *req.GroupID)
			stored := keyStore[id]
			stored.GroupID = req.GroupID
			stored.Group = balanceGroup
			return cloneStoredKey(stored), nil
		}},
		apiKeyRepo,
		subscriptionRepo,
	)

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddlewareWithEntitlements(apiKeyService, subscriptionService, entitlementService, &config.Config{RunMode: config.RunModeStandard})))
	router.GET("/t", func(c *gin.Context) {
		keyFromCtx, ok := GetAPIKeyFromContext(c)
		require.True(t, ok)
		require.Equal(t, currentKey.ID, keyFromCtx.ID)
		require.NotNil(t, keyFromCtx.GroupID)
		require.Equal(t, balanceGroup.ID, *keyFromCtx.GroupID)
		require.Equal(t, balanceGroup.ID, c.Request.Context().Value(ctxkey.Group).(*service.Group).ID)
		_, hasSubscription := GetSubscriptionFromContext(c)
		require.False(t, hasSubscription)
		c.JSON(http.StatusOK, gin.H{"api_key_id": keyFromCtx.ID, "group_id": *keyFromCtx.GroupID})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", currentKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "true", w.Header().Get("X-Sub2API-Auto-Switched"))
	require.Equal(t, balanceGroup.Name, w.Header().Get("X-Sub2API-Auto-Switch-Target-Group"))
	require.Contains(t, w.Body.String(), `"api_key_id":100`)
	require.Contains(t, w.Body.String(), `"group_id":42`)
}

func TestAPIKeyAuthDoesNotRebindAcrossProviderPlatforms(t *testing.T) {
	gin.SetMode(gin.TestMode)

	monthlyLimit := 50.0
	openAIGroup := &service.Group{ID: 41, Name: "openai-expired", Status: service.StatusActive, Platform: service.PlatformOpenAI, Hydrated: true, SubscriptionType: service.SubscriptionTypeSubscription, MonthlyLimitUSD: &monthlyLimit}
	geminiSubscriptionGroup := &service.Group{ID: 42, Name: "gemini-subscription", Status: service.StatusActive, Platform: service.PlatformGemini, Hydrated: true, SubscriptionType: service.SubscriptionTypeSubscription, MonthlyLimitUSD: &monthlyLimit}
	geminiBalanceGroup := &service.Group{ID: 43, Name: "gemini-credit", Status: service.StatusActive, Platform: service.PlatformGemini, Hydrated: true, SubscriptionType: service.SubscriptionTypeStandard}
	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 10, Concurrency: 3}
	currentKey := &service.APIKey{ID: 100, UserID: user.ID, Key: "openai-key", Status: service.StatusActive, User: user, GroupID: &openAIGroup.ID, Group: openAIGroup}

	cloneStoredKey := func(key *service.APIKey) *service.APIKey {
		if key == nil {
			return nil
		}
		clone := *key
		if key.GroupID != nil {
			groupID := *key.GroupID
			clone.GroupID = &groupID
		}
		return &clone
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != currentKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(currentKey), nil
		},
		getByID: func(ctx context.Context, id int64) (*service.APIKey, error) {
			if id != currentKey.ID {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(currentKey), nil
		},
		listByUserID: func(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
			require.Equal(t, user.ID, userID)
			clone := cloneStoredKey(currentKey)
			return []service.APIKey{*clone}, &pagination.PaginationResult{Total: 1, Page: 1, PageSize: 1000}, nil
		},
	}
	now := time.Now()
	subscriptionRepo := &stubUserSubscriptionRepo{
		getActive: func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
			return nil, service.ErrSubscriptionNotFound
		},
		listByUserID: func(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
			require.Equal(t, user.ID, userID)
			return []service.UserSubscription{
				{ID: 54, UserID: user.ID, GroupID: openAIGroup.ID, Status: service.SubscriptionStatusExpired, StartsAt: now.Add(-96 * time.Hour), ExpiresAt: now.Add(-24 * time.Hour)},
				{ID: 55, UserID: user.ID, GroupID: geminiSubscriptionGroup.ID, Status: service.SubscriptionStatusActive, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(30 * 24 * time.Hour)},
			}, nil
		},
		updateStatus:   func(ctx context.Context, subscriptionID int64, status string) error { return nil },
		activateWindow: func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetDaily:     func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetWeekly:    func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetMonthly:   func(ctx context.Context, id int64, start time.Time) error { return nil },
	}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeStandard})
	subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, &config.Config{RunMode: config.RunModeStandard})
	entitlementService := service.NewEntitlementService(
		&stubEntitlementUserRepo{user: user},
		&stubEntitlementGroupRepo{groups: map[int64]*service.Group{openAIGroup.ID: openAIGroup, geminiSubscriptionGroup.ID: geminiSubscriptionGroup, geminiBalanceGroup.ID: geminiBalanceGroup}},
		&stubEntitlementAPIKeyUpdater{update: func(ctx context.Context, id, userID int64, req service.UpdateAPIKeyRequest) (*service.APIKey, error) {
			t.Fatalf("OpenAI request key must not be rebound to Gemini subscription/balance groups")
			return nil, nil
		}},
		apiKeyRepo,
		subscriptionRepo,
	)

	router := gin.New()
	reachedHandler := false
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddlewareWithEntitlements(apiKeyService, subscriptionService, entitlementService, &config.Config{RunMode: config.RunModeStandard})))
	router.GET("/t", func(c *gin.Context) {
		reachedHandler = true
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", currentKey.Key)
	router.ServeHTTP(w, req)

	require.False(t, reachedHandler)
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "SUBSCRIPTION_NOT_FOUND")
	require.Empty(t, w.Header().Get("X-Sub2API-Auto-Switched"))
	require.Empty(t, w.Header().Get("X-Sub2API-Auto-Switch-Action"))
}

func TestAPIKeyAuthGoogleDoesNotRebindAcrossProviderPlatforms(t *testing.T) {
	gin.SetMode(gin.TestMode)

	monthlyLimit := 50.0
	openAIGroup := &service.Group{ID: 51, Name: "openai-expired", Status: service.StatusActive, Platform: service.PlatformOpenAI, Hydrated: true, SubscriptionType: service.SubscriptionTypeSubscription, MonthlyLimitUSD: &monthlyLimit}
	geminiSubscriptionGroup := &service.Group{ID: 52, Name: "gemini-subscription", Status: service.StatusActive, Platform: service.PlatformGemini, Hydrated: true, SubscriptionType: service.SubscriptionTypeSubscription, MonthlyLimitUSD: &monthlyLimit}
	geminiBalanceGroup := &service.Group{ID: 53, Name: "gemini-credit", Status: service.StatusActive, Platform: service.PlatformGemini, Hydrated: true, SubscriptionType: service.SubscriptionTypeStandard}
	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 10, Concurrency: 3}
	currentKey := &service.APIKey{ID: 110, UserID: user.ID, Key: "openai-google-style-key", Status: service.StatusActive, User: user, GroupID: &openAIGroup.ID, Group: openAIGroup}

	cloneStoredKey := func(key *service.APIKey) *service.APIKey {
		if key == nil {
			return nil
		}
		clone := *key
		if key.GroupID != nil {
			groupID := *key.GroupID
			clone.GroupID = &groupID
		}
		return &clone
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != currentKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(currentKey), nil
		},
		getByID: func(ctx context.Context, id int64) (*service.APIKey, error) {
			if id != currentKey.ID {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(currentKey), nil
		},
		listByUserID: func(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
			require.Equal(t, user.ID, userID)
			clone := cloneStoredKey(currentKey)
			return []service.APIKey{*clone}, &pagination.PaginationResult{Total: 1, Page: 1, PageSize: 1000}, nil
		},
	}
	now := time.Now()
	subscriptionRepo := &stubUserSubscriptionRepo{
		getActive: func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
			return nil, service.ErrSubscriptionNotFound
		},
		listByUserID: func(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
			require.Equal(t, user.ID, userID)
			return []service.UserSubscription{
				{ID: 64, UserID: user.ID, GroupID: openAIGroup.ID, Status: service.SubscriptionStatusExpired, StartsAt: now.Add(-96 * time.Hour), ExpiresAt: now.Add(-24 * time.Hour)},
				{ID: 65, UserID: user.ID, GroupID: geminiSubscriptionGroup.ID, Status: service.SubscriptionStatusActive, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(30 * 24 * time.Hour)},
			}, nil
		},
		updateStatus:   func(ctx context.Context, subscriptionID int64, status string) error { return nil },
		activateWindow: func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetDaily:     func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetWeekly:    func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetMonthly:   func(ctx context.Context, id int64, start time.Time) error { return nil },
	}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeStandard})
	subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, &config.Config{RunMode: config.RunModeStandard})
	entitlementService := service.NewEntitlementService(
		&stubEntitlementUserRepo{user: user},
		&stubEntitlementGroupRepo{groups: map[int64]*service.Group{openAIGroup.ID: openAIGroup, geminiSubscriptionGroup.ID: geminiSubscriptionGroup, geminiBalanceGroup.ID: geminiBalanceGroup}},
		&stubEntitlementAPIKeyUpdater{update: func(ctx context.Context, id, userID int64, req service.UpdateAPIKeyRequest) (*service.APIKey, error) {
			t.Fatalf("OpenAI request key from Google-style auth must not be rebound to Gemini subscription/balance groups")
			return nil, nil
		}},
		apiKeyRepo,
		subscriptionRepo,
	)

	router := gin.New()
	reachedHandler := false
	router.Use(APIKeyAuthWithSubscriptionGoogleAndEntitlements(apiKeyService, subscriptionService, entitlementService, &config.Config{RunMode: config.RunModeStandard}))
	router.GET("/v1beta/test", func(c *gin.Context) {
		reachedHandler = true
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("x-goog-api-key", currentKey.Key)
	router.ServeHTTP(w, req)

	require.False(t, reachedHandler)
	require.Equal(t, http.StatusForbidden, w.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "No active subscription found for this group", resp.Error.Message)
	require.Equal(t, "PERMISSION_DENIED", resp.Error.Status)
	require.Empty(t, w.Header().Get("X-Sub2API-Auto-Switched"))
	require.Empty(t, w.Header().Get("X-Sub2API-Auto-Switch-Action"))
}

func TestAPIKeyAuthDoesNotSwitchToAlternateAPIKeyForGatewayHiddenKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	balanceGroup := &service.Group{ID: 42, Name: "balance-credit", Status: service.StatusActive, Platform: service.PlatformOpenAI, Hydrated: true, SubscriptionType: service.SubscriptionTypeStandard}
	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 10, Concurrency: 3}
	keyStore := map[int64]*service.APIKey{}
	currentKey := &service.APIKey{ID: 100, UserID: user.ID, Key: "bootstrap-key", Status: service.StatusAPIKeyQuotaExhausted, User: user, GroupID: &balanceGroup.ID, Group: balanceGroup, Quota: 1, QuotaUsed: 1}
	otherKey := &service.APIKey{ID: 101, UserID: user.ID, Key: "other-balance-key", Status: service.StatusActive, User: user, GroupID: &balanceGroup.ID, Group: balanceGroup}
	keyStore[currentKey.ID] = currentKey
	keyStore[otherKey.ID] = otherKey

	cloneStoredKey := func(key *service.APIKey) *service.APIKey {
		if key == nil {
			return nil
		}
		clone := *key
		if key.GroupID != nil {
			groupID := *key.GroupID
			clone.GroupID = &groupID
		}
		return &clone
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != currentKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(keyStore[currentKey.ID]), nil
		},
		getByID: func(ctx context.Context, id int64) (*service.APIKey, error) {
			key := keyStore[id]
			if key == nil {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(key), nil
		},
		listByUserID: func(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
			require.Equal(t, user.ID, userID)
			current := cloneStoredKey(keyStore[currentKey.ID])
			other := cloneStoredKey(keyStore[otherKey.ID])
			return []service.APIKey{*current, *other}, &pagination.PaginationResult{Total: 2, Page: 1, PageSize: 1000}, nil
		},
	}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeStandard})
	entitlementService := service.NewEntitlementService(
		&stubEntitlementUserRepo{user: user},
		&stubEntitlementGroupRepo{groups: map[int64]*service.Group{balanceGroup.ID: balanceGroup}},
		&stubEntitlementAPIKeyUpdater{update: func(ctx context.Context, id, userID int64, req service.UpdateAPIKeyRequest) (*service.APIKey, error) {
			t.Fatalf("gateway continuity must not update or switch a different API key; client can only keep using the original bootstrap key")
			return nil, nil
		}},
		apiKeyRepo,
		&stubUserSubscriptionRepo{listByUserID: func(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
			require.Equal(t, user.ID, userID)
			return nil, nil
		}},
	)

	router := gin.New()
	reachedHandler := false
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddlewareWithEntitlements(apiKeyService, nil, entitlementService, &config.Config{RunMode: config.RunModeStandard})))
	router.GET("/t", func(c *gin.Context) {
		reachedHandler = true
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", currentKey.Key)
	router.ServeHTTP(w, req)

	require.False(t, reachedHandler)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Contains(t, w.Body.String(), "API_KEY_QUOTA_EXHAUSTED")
	require.Empty(t, w.Header().Get("X-Sub2API-Auto-Switched"))
	require.Empty(t, w.Header().Get("X-Sub2API-Auto-Switch-Action"))
}

func TestAPIKeyAuthAutoSwitchesBalanceKeyToActiveSubscriptionWhenBalanceEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)

	monthlyLimit := 50.0
	balanceGroup := &service.Group{
		ID:               41,
		Name:             "balance-credit",
		Status:           service.StatusActive,
		Platform:         service.PlatformOpenAI,
		Hydrated:         true,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	subscriptionGroup := &service.Group{
		ID:               42,
		Name:             "sub-plan",
		Status:           service.StatusActive,
		Platform:         service.PlatformOpenAI,
		Hydrated:         true,
		SubscriptionType: service.SubscriptionTypeSubscription,
		MonthlyLimitUSD:  &monthlyLimit,
	}
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     0,
		Concurrency: 3,
	}
	keyStore := map[int64]*service.APIKey{}
	currentKey := &service.APIKey{
		ID:      100,
		UserID:  user.ID,
		Key:     "balance-key",
		Status:  service.StatusActive,
		User:    user,
		GroupID: &balanceGroup.ID,
		Group:   balanceGroup,
	}
	keyStore[currentKey.ID] = currentKey

	cloneStoredKey := func(key *service.APIKey) *service.APIKey {
		if key == nil {
			return nil
		}
		clone := *key
		if key.GroupID != nil {
			groupID := *key.GroupID
			clone.GroupID = &groupID
		}
		return &clone
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != currentKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(keyStore[currentKey.ID]), nil
		},
		getByID: func(ctx context.Context, id int64) (*service.APIKey, error) {
			key := keyStore[id]
			if key == nil {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(key), nil
		},
		listByUserID: func(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
			require.Equal(t, user.ID, userID)
			clone := cloneStoredKey(keyStore[currentKey.ID])
			return []service.APIKey{*clone}, &pagination.PaginationResult{Total: 1, Page: 1, PageSize: 1000}, nil
		},
	}
	now := time.Now()
	subscription := service.UserSubscription{
		ID:                 55,
		UserID:             user.ID,
		GroupID:            subscriptionGroup.ID,
		Status:             service.SubscriptionStatusActive,
		StartsAt:           now.Add(-24 * time.Hour),
		ExpiresAt:          now.Add(24 * time.Hour),
		MonthlyWindowStart: &now,
		MonthlyUsageUSD:    10,
	}
	subscriptionRepo := &stubUserSubscriptionRepo{
		getActive: func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
			if userID != subscription.UserID || groupID != subscription.GroupID {
				return nil, service.ErrSubscriptionNotFound
			}
			clone := subscription
			return &clone, nil
		},
		listByUserID: func(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
			require.Equal(t, user.ID, userID)
			return []service.UserSubscription{subscription}, nil
		},
		updateStatus:   func(ctx context.Context, subscriptionID int64, status string) error { return nil },
		activateWindow: func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetDaily:     func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetWeekly:    func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetMonthly:   func(ctx context.Context, id int64, start time.Time) error { return nil },
	}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeStandard})
	subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, &config.Config{RunMode: config.RunModeStandard})

	groups := map[int64]*service.Group{
		balanceGroup.ID:      balanceGroup,
		subscriptionGroup.ID: subscriptionGroup,
	}
	entitlementService := service.NewEntitlementService(
		&stubEntitlementUserRepo{user: user},
		&stubEntitlementGroupRepo{groups: groups},
		&stubEntitlementAPIKeyUpdater{update: func(ctx context.Context, id, userID int64, req service.UpdateAPIKeyRequest) (*service.APIKey, error) {
			require.Equal(t, currentKey.ID, id)
			require.Equal(t, user.ID, userID)
			require.NotNil(t, req.GroupID)
			require.Equal(t, subscriptionGroup.ID, *req.GroupID)
			stored := keyStore[id]
			stored.GroupID = req.GroupID
			stored.Group = subscriptionGroup
			return cloneStoredKey(stored), nil
		}},
		apiKeyRepo,
		subscriptionRepo,
	)

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddlewareWithEntitlements(apiKeyService, subscriptionService, entitlementService, &config.Config{RunMode: config.RunModeStandard})))
	router.GET("/t", func(c *gin.Context) {
		keyFromCtx, ok := GetAPIKeyFromContext(c)
		require.True(t, ok)
		require.NotNil(t, keyFromCtx.GroupID)
		require.Equal(t, subscriptionGroup.ID, *keyFromCtx.GroupID)
		require.Equal(t, subscriptionGroup.ID, c.Request.Context().Value(ctxkey.Group).(*service.Group).ID)
		subFromCtx, hasSubscription := GetSubscriptionFromContext(c)
		require.True(t, hasSubscription, "switched subscription group must set subscription context for billing")
		require.Equal(t, subscription.ID, subFromCtx.ID)
		c.JSON(http.StatusOK, gin.H{"api_key_id": keyFromCtx.ID, "group_id": *keyFromCtx.GroupID, "subscription_id": subFromCtx.ID})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", currentKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "true", w.Header().Get("X-Sub2API-Auto-Switched"))
	require.Equal(t, service.EntitlementSwitchActionGroup, w.Header().Get("X-Sub2API-Auto-Switch-Action"))
	require.Equal(t, subscriptionGroup.Name, w.Header().Get("X-Sub2API-Auto-Switch-Target-Group"))
	require.Contains(t, w.Body.String(), `"group_id":42`)
	require.Contains(t, w.Body.String(), `"subscription_id":55`)
}

func TestAPIKeyAuthAutoSwitchesSubscriptionLimitToFallbackGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dailyLimit := 1.0
	fallbackGroupID := int64(43)
	subscriptionGroup := &service.Group{
		ID:               42,
		Name:             "sub-plan",
		Status:           service.StatusActive,
		Platform:         service.PlatformAnthropic,
		Hydrated:         true,
		SubscriptionType: service.SubscriptionTypeSubscription,
		DailyLimitUSD:    &dailyLimit,
		FallbackGroupID:  &fallbackGroupID,
	}
	creditGroup := &service.Group{
		ID:               fallbackGroupID,
		Name:             "credit-fallback",
		Status:           service.StatusActive,
		Platform:         service.PlatformAnthropic,
		Hydrated:         true,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	keyStore := map[int64]*service.APIKey{}
	currentKey := &service.APIKey{
		ID:      100,
		UserID:  user.ID,
		Key:     "sub-key",
		Status:  service.StatusActive,
		User:    user,
		GroupID: &subscriptionGroup.ID,
		Group:   subscriptionGroup,
	}
	keyStore[currentKey.ID] = currentKey

	cloneStoredKey := func(key *service.APIKey) *service.APIKey {
		if key == nil {
			return nil
		}
		clone := *key
		if key.GroupID != nil {
			groupID := *key.GroupID
			clone.GroupID = &groupID
		}
		return &clone
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != currentKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(keyStore[currentKey.ID]), nil
		},
		getByID: func(ctx context.Context, id int64) (*service.APIKey, error) {
			key := keyStore[id]
			if key == nil {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneStoredKey(key), nil
		},
		listByUserID: func(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
			require.Equal(t, user.ID, userID)
			clone := cloneStoredKey(keyStore[currentKey.ID])
			return []service.APIKey{*clone}, &pagination.PaginationResult{Total: 1, Page: 1, PageSize: 1000}, nil
		},
	}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeStandard})

	now := time.Now()
	subscription := service.UserSubscription{
		ID:               55,
		UserID:           user.ID,
		GroupID:          subscriptionGroup.ID,
		Status:           service.SubscriptionStatusActive,
		StartsAt:         now.Add(-24 * time.Hour),
		ExpiresAt:        now.Add(24 * time.Hour),
		DailyWindowStart: &now,
		DailyUsageUSD:    10,
	}
	subscriptionRepo := &stubUserSubscriptionRepo{
		getActive: func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
			if userID != subscription.UserID || groupID != subscription.GroupID {
				return nil, service.ErrSubscriptionNotFound
			}
			clone := subscription
			return &clone, nil
		},
		listByUserID: func(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
			require.Equal(t, user.ID, userID)
			return []service.UserSubscription{subscription}, nil
		},
		updateStatus:   func(ctx context.Context, subscriptionID int64, status string) error { return nil },
		activateWindow: func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetDaily:     func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetWeekly:    func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetMonthly:   func(ctx context.Context, id int64, start time.Time) error { return nil },
	}
	subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, &config.Config{RunMode: config.RunModeStandard})

	groups := map[int64]*service.Group{
		subscriptionGroup.ID: subscriptionGroup,
		creditGroup.ID:       creditGroup,
	}
	entitlementService := service.NewEntitlementService(
		&stubEntitlementUserRepo{user: user},
		&stubEntitlementGroupRepo{groups: groups},
		&stubEntitlementAPIKeyUpdater{update: func(ctx context.Context, id, userID int64, req service.UpdateAPIKeyRequest) (*service.APIKey, error) {
			require.Equal(t, currentKey.ID, id)
			require.Equal(t, user.ID, userID)
			require.NotNil(t, req.GroupID)
			require.Equal(t, creditGroup.ID, *req.GroupID)
			stored := keyStore[id]
			stored.GroupID = req.GroupID
			stored.Group = creditGroup
			return cloneStoredKey(stored), nil
		}},
		apiKeyRepo,
		subscriptionRepo,
	)

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddlewareWithEntitlements(apiKeyService, subscriptionService, entitlementService, &config.Config{RunMode: config.RunModeStandard})))
	router.GET("/t", func(c *gin.Context) {
		keyFromCtx, ok := GetAPIKeyFromContext(c)
		require.True(t, ok)
		require.NotNil(t, keyFromCtx.GroupID)
		require.Equal(t, creditGroup.ID, *keyFromCtx.GroupID)
		require.Equal(t, creditGroup.ID, c.Request.Context().Value(ctxkey.Group).(*service.Group).ID)
		_, hasSubscription := GetSubscriptionFromContext(c)
		require.False(t, hasSubscription, "fallback credit group must not keep exhausted subscription context")
		c.JSON(http.StatusOK, gin.H{"api_key_id": keyFromCtx.ID, "group_id": *keyFromCtx.GroupID})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", currentKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "true", w.Header().Get("X-Sub2API-Auto-Switched"))
	require.Equal(t, service.EntitlementSwitchActionGroup, w.Header().Get("X-Sub2API-Auto-Switch-Action"))
	require.Equal(t, creditGroup.Name, w.Header().Get("X-Sub2API-Auto-Switch-Target-Group"))
	require.Contains(t, w.Body.String(), `"group_id":43`)
}

func newAuthTestRouter(apiKeyService *service.APIKeyService, subscriptionService *service.SubscriptionService, cfg *config.Config) *gin.Engine {
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, subscriptionService, cfg)))
	ok := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
	router.GET("/t", ok)
	router.POST("/v1/responses", ok)
	router.POST("/v1/messages", ok)
	router.GET("/v1/usage", ok)
	router.GET("/v1/sub2api/billing", ok)
	return router
}

func requireAPIKeyAuthError(t *testing.T, w *httptest.ResponseRecorder, code, message string) {
	t.Helper()

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, code, resp.Code)
	require.Equal(t, message, resp.Message)
}

type stubApiKeyRepo struct {
	getByID        func(ctx context.Context, id int64) (*service.APIKey, error)
	getByKey       func(ctx context.Context, key string) (*service.APIKey, error)
	update         func(ctx context.Context, key *service.APIKey) error
	listByUserID   func(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error)
	updateLastUsed func(ctx context.Context, id int64, usedAt time.Time) error
}

func (r *stubApiKeyRepo) Create(ctx context.Context, key *service.APIKey) error {
	return errors.New("not implemented")
}

func (r *stubApiKeyRepo) GetByID(ctx context.Context, id int64) (*service.APIKey, error) {
	if r.getByID != nil {
		return r.getByID(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error) {
	return "", 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) GetByKey(ctx context.Context, key string) (*service.APIKey, error) {
	if r.getByKey != nil {
		return r.getByKey(ctx, key)
	}
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) GetByKeyForAuth(ctx context.Context, key string) (*service.APIKey, error) {
	return r.GetByKey(ctx, key)
}

func (r *stubApiKeyRepo) Update(ctx context.Context, key *service.APIKey, _ service.APIKeyUpdateFields) error {
	if r.update != nil {
		return r.update(ctx, key)
	}
	return errors.New("not implemented")
}

func (r *stubApiKeyRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (r *stubApiKeyRepo) DeleteWithAudit(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (r *stubApiKeyRepo) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	if r.listByUserID != nil {
		return r.listByUserID(ctx, userID, params, filters)
	}
	return nil, nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ExistsByKey(ctx context.Context, key string) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.APIKey, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.APIKey, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) UpdateGroupIDByUserAndGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ListKeysByUserID(ctx context.Context, userID int64) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ListKeysByGroupID(ctx context.Context, groupID int64) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) (float64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) UpdateLastUsed(ctx context.Context, id int64, usedAt time.Time) error {
	if r.updateLastUsed != nil {
		return r.updateLastUsed(ctx, id, usedAt)
	}
	return nil
}

func (r *stubApiKeyRepo) IncrementRateLimitUsage(ctx context.Context, id int64, cost float64) error {
	return nil
}
func (r *stubApiKeyRepo) ResetRateLimitWindows(ctx context.Context, id int64) error {
	return nil
}
func (r *stubApiKeyRepo) GetRateLimitData(ctx context.Context, id int64) (*service.APIKeyRateLimitData, error) {
	return nil, nil
}

type stubUserSubscriptionRepo struct {
	getByID        func(ctx context.Context, id int64) (*service.UserSubscription, error)
	getActive      func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error)
	listByUserID   func(ctx context.Context, userID int64) ([]service.UserSubscription, error)
	updateStatus   func(ctx context.Context, subscriptionID int64, status string) error
	activateWindow func(ctx context.Context, id int64, start time.Time) error
	resetDaily     func(ctx context.Context, id int64, start time.Time) error
	resetWeekly    func(ctx context.Context, id int64, start time.Time) error
	resetMonthly   func(ctx context.Context, id int64, start time.Time) error
}

type fakeSettingRepo struct {
	values map[string]string
}

func (r fakeSettingRepo) Get(ctx context.Context, key string) (*service.Setting, error) {
	return nil, errors.New("not implemented")
}

func (r fakeSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	if v, ok := r.values[key]; ok {
		return v, nil
	}
	return "", service.ErrSettingNotFound
}

func (r fakeSettingRepo) Set(ctx context.Context, key, value string) error {
	return errors.New("not implemented")
}

func (r fakeSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func (r fakeSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	return errors.New("not implemented")
}

func (r fakeSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func (r fakeSettingRepo) Delete(ctx context.Context, key string) error {
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) Create(ctx context.Context, sub *service.UserSubscription) error {
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	if r.getByID != nil {
		return r.getByID(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	if r.getActive != nil {
		return r.getActive(ctx, userID, groupID)
	}
	return nil, errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) Update(ctx context.Context, sub *service.UserSubscription) error {
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) Restore(ctx context.Context, subscriptionID int64, restoredStatus string) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) ListByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	if r.listByUserID != nil {
		return r.listByUserID(ctx, userID)
	}
	return nil, errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) ListActiveByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, scopedUserIDs []int64, status, platform, deviceCode, sortBy, sortOrder string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) ExistsActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	if r.updateStatus != nil {
		return r.updateStatus(ctx, subscriptionID, status)
	}
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error {
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) ActivateWindows(ctx context.Context, id int64, start time.Time) error {
	if r.activateWindow != nil {
		return r.activateWindow(ctx, id, start)
	}
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) ResetUsageWindows(context.Context, int64, bool, bool, bool, time.Time) error {
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) ResetDailyUsage(ctx context.Context, id int64, _ *time.Time, newWindowStart time.Time) error {
	if r.resetDaily != nil {
		return r.resetDaily(ctx, id, newWindowStart)
	}
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) ResetWeeklyUsage(ctx context.Context, id int64, _ *time.Time, newWindowStart time.Time) error {
	if r.resetWeekly != nil {
		return r.resetWeekly(ctx, id, newWindowStart)
	}
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) ResetMonthlyUsage(ctx context.Context, id int64, _ *time.Time, newWindowStart time.Time) error {
	if r.resetMonthly != nil {
		return r.resetMonthly(ctx, id, newWindowStart)
	}
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	return errors.New("not implemented")
}

func (r *stubUserSubscriptionRepo) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	return 0, errors.New("not implemented")
}

type stubEntitlementUserRepo struct {
	user *service.User
}

func (r *stubEntitlementUserRepo) GetByID(ctx context.Context, id int64) (*service.User, error) {
	if r.user != nil && r.user.ID == id {
		clone := *r.user
		return &clone, nil
	}
	return nil, service.ErrUserNotFound
}

type stubEntitlementGroupRepo struct {
	groups map[int64]*service.Group
}

func (r *stubEntitlementGroupRepo) GetByID(_ context.Context, id int64) (*service.Group, error) {
	group := r.groups[id]
	if group == nil {
		return nil, service.ErrGroupNotFound
	}
	clone := *group
	if group.FallbackGroupID != nil {
		fallbackGroupID := *group.FallbackGroupID
		clone.FallbackGroupID = &fallbackGroupID
	}
	if group.DailyLimitUSD != nil {
		dailyLimit := *group.DailyLimitUSD
		clone.DailyLimitUSD = &dailyLimit
	}
	return &clone, nil
}

func (r *stubEntitlementGroupRepo) ListActiveByPlatform(_ context.Context, platform string) ([]service.Group, error) {
	groups := make([]service.Group, 0, len(r.groups))
	for _, group := range r.groups {
		if group == nil || group.Platform != platform || !group.IsActive() {
			continue
		}
		clone := *group
		groups = append(groups, clone)
	}
	return groups, nil
}

type stubEntitlementAPIKeyUpdater struct {
	update func(ctx context.Context, id, userID int64, req service.UpdateAPIKeyRequest) (*service.APIKey, error)
}

func (u *stubEntitlementAPIKeyUpdater) Update(ctx context.Context, id, userID int64, req service.UpdateAPIKeyRequest) (*service.APIKey, error) {
	if u.update != nil {
		return u.update(ctx, id, userID, req)
	}
	return nil, errors.New("not implemented")
}
