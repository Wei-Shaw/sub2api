package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxMediaTaskReadsStayAvailableAfterBilling(t *testing.T) {
	for _, path := range []string{"/v1/media/tasks", "/v1/media/tasks/mm_x", "/v2/query/video_generation", "/v2/query/video_generation/mm_x"} {
		require.True(t, isMiniMaxMediaTaskRead(http.MethodGet, path))
		require.False(t, isMiniMaxMediaTaskRead(http.MethodPost, path))
	}
	for _, path := range []string{"/v1/t2a_v2", "/v2/video_generation", "/v1/chat/completions", "/v1/media/taskspoof"} {
		require.False(t, isMiniMaxMediaTaskRead(http.MethodGet, path))
	}
}

type miniMaxAuthReplayRepo struct {
	service.MiniMaxMediaRepository
	task  *service.MiniMaxMediaTask
	calls int
}

func (r *miniMaxAuthReplayRepo) FindByIdempotencyKey(context.Context, int64, string) (*service.MiniMaxMediaTask, error) {
	r.calls++
	if r.task == nil {
		return nil, service.ErrMiniMaxMediaNotFound
	}
	return r.task, nil
}

func TestMiniMaxReplayAuthenticationAndAllowlist(t *testing.T) {
	body := `{"model":"old-alias","text":"hello"}`
	for _, tc := range []struct {
		name, keyStatus, userStatus, path, body string
		known, otherOwner, restrictedIP         bool
		status                                  int
	}{
		{"exhausted key and wallet", service.StatusAPIKeyQuotaExhausted, service.StatusActive, "/v1/t2a_v2", body, true, false, false, http.StatusOK},
		{"expired key", service.StatusAPIKeyExpired, service.StatusActive, "/v1/t2a_v2", body, true, false, false, http.StatusOK},
		{"unknown idempotency key", service.StatusAPIKeyQuotaExhausted, service.StatusActive, "/v1/t2a_v2", body, false, false, false, http.StatusTooManyRequests},
		{"different payload", service.StatusAPIKeyQuotaExhausted, service.StatusActive, "/v1/t2a_v2", `{"model":"other"}`, true, false, false, http.StatusConflict},
		{"different owner", service.StatusAPIKeyQuotaExhausted, service.StatusActive, "/v1/t2a_v2", body, true, true, false, http.StatusConflict},
		{"disabled key", service.StatusDisabled, service.StatusActive, "/v1/t2a_v2", body, true, false, false, http.StatusUnauthorized},
		{"disabled user", service.StatusActive, service.StatusDisabled, "/v1/t2a_v2", body, true, false, false, http.StatusUnauthorized},
		{"restricted IP", service.StatusActive, service.StatusActive, "/v1/t2a_v2", body, true, false, true, http.StatusForbidden},
		{"unrelated route", service.StatusAPIKeyExpired, service.StatusActive, "/v1/chat/completions", body, true, false, false, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			group := &service.Group{ID: 3, Name: "composite", Platform: service.PlatformComposite, Status: service.StatusActive, Hydrated: true, ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"new-alias"}}}
			user := &service.User{ID: 7, Status: tc.userStatus, Role: service.RoleUser, Balance: 0, Concurrency: 2}
			key := &service.APIKey{ID: 9, Key: "test-minimax-replay-key", UserID: user.ID, User: user, GroupID: &group.ID, Group: group, Status: tc.keyStatus, Quota: 1, QuotaUsed: 1}
			if tc.restrictedIP {
				key.IPWhitelist = []string{"192.0.2.10"}
			}
			if tc.keyStatus == service.StatusAPIKeyExpired {
				past := time.Now().Add(-time.Hour)
				key.ExpiresAt = &past
			}
			keys := &miniMaxAuthKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { copy := *key; return &copy, nil }}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			keyService := service.NewAPIKeyService(keys, nil, nil, nil, nil, nil, cfg)
			repo := &miniMaxAuthReplayRepo{}
			if tc.known {
				repo.task = &service.MiniMaxMediaTask{ID: "mm_original", Kind: "speech", UserID: user.ID, APIKeyID: key.ID, RequestHash: service.HashUsageRequestPayload([]byte(body))}
			}
			if tc.otherOwner {
				repo.task.APIKeyID++
			}
			media := service.NewMiniMaxMediaService(repo, nil, nil)
			router := gin.New()
			router.Use(gin.HandlerFunc(ProvideAPIKeyAuthMiddleware(keyService, nil, cfg, media)), GroupModelAllowlist())
			called := false
			router.POST(tc.path, func(c *gin.Context) {
				called = true
				require.True(t, IsMiniMaxMediaReplay(c))
				got, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				require.Equal(t, tc.body, string(got))
				c.Status(http.StatusOK)
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.RemoteAddr = "198.51.100.5:1234"
			req.Header.Set("Authorization", "Bearer "+key.Key)
			req.Header.Set("Idempotency-Key", "old-request")
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.Equal(t, tc.status == http.StatusOK, called)
			if tc.keyStatus == service.StatusDisabled || tc.userStatus == service.StatusDisabled || tc.restrictedIP || tc.path == "/v1/chat/completions" {
				require.Zero(t, repo.calls)
			}
		})
	}
}

type miniMaxAuthKeyRepo struct {
	service.APIKeyRepository
	getByKey func(context.Context, string) (*service.APIKey, error)
}

func (r *miniMaxAuthKeyRepo) GetByKey(ctx context.Context, key string) (*service.APIKey, error) {
	return r.getByKey(ctx, key)
}
func (r *miniMaxAuthKeyRepo) GetByKeyForAuth(ctx context.Context, key string) (*service.APIKey, error) {
	return r.getByKey(ctx, key)
}
func (r *miniMaxAuthKeyRepo) UpdateLastUsed(context.Context, int64, time.Time) error { return nil }
