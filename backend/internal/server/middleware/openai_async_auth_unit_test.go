//go:build unit

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAsyncManagementAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name         string
		method       string
		path         string
		keyStatus    string
		userStatus   string
		lookupErr    error
		cancelLookup bool
		disabled     bool
		want         int
	}{
		{name: "list files without balance", method: http.MethodGet, path: "/v1/files", want: http.StatusOK},
		{name: "upload file without balance", method: http.MethodPost, path: "/v1/files", want: http.StatusOK},
		{name: "read file without balance", method: http.MethodGet, path: "/v1/files/file-local-123/content", want: http.StatusOK},
		{name: "delete file without balance", method: http.MethodDelete, path: "/v1/files/file-local-123", want: http.StatusOK},
		{name: "list batches without balance", method: http.MethodGet, path: "/v1/batches", want: http.StatusOK},
		{name: "cancel batch without balance", method: http.MethodPost, path: "/v1/batches/batch_123/cancel", want: http.StatusOK},
		{name: "create batch requires balance", method: http.MethodPost, path: "/v1/batches", want: http.StatusForbidden},
		{name: "disabled feature enforces billing", method: http.MethodGet, path: "/v1/files", disabled: true, want: http.StatusForbidden},
		{name: "exhausted key may retrieve results", method: http.MethodGet, path: "/v1/files", keyStatus: service.StatusAPIKeyQuotaExhausted, want: http.StatusOK},
		{name: "disabled key is rejected", method: http.MethodGet, path: "/v1/files", keyStatus: service.StatusDisabled, want: http.StatusUnauthorized},
		{name: "disabled user is rejected", method: http.MethodGet, path: "/v1/files", userStatus: service.StatusDisabled, want: http.StatusUnauthorized},
		{name: "unknown key is rejected", method: http.MethodGet, path: "/v1/files", lookupErr: service.ErrAPIKeyNotFound, want: http.StatusUnauthorized},
		{name: "client disconnect during lookup", method: http.MethodGet, path: "/v1/files", cancelLookup: true, want: statusClientClosedRequest},
		{name: "lookup failure remains server error", method: http.MethodGet, path: "/v1/files", lookupErr: errors.New("database unavailable"), want: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			user := &service.User{ID: 10, Role: service.RoleUser, Status: service.StatusActive}
			key := &service.APIKey{ID: 100, UserID: user.ID, Key: "sk-async-auth-test", Status: service.StatusActive, User: user}
			if tc.keyStatus != "" {
				key.Status = tc.keyStatus
			}
			if tc.userStatus != "" {
				user.Status = tc.userStatus
			}
			repo := &stubApiKeyRepo{getByKey: func(lookupCtx context.Context, credential string) (*service.APIKey, error) {
				require.Equal(t, key.Key, credential)
				if tc.cancelLookup {
					cancel()
					return nil, lookupCtx.Err()
				}
				if tc.lookupErr != nil {
					return nil, tc.lookupErr
				}
				return key, nil
			}}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			cfg.OpenAIAsync.Enabled = !tc.disabled
			svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
			router := gin.New()
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)))
			called := false
			router.Handle(tc.method, tc.path, func(c *gin.Context) {
				called = true
				authenticated, ok := GetAPIKeyFromContext(c)
				require.True(t, ok)
				require.Equal(t, key.UserID, authenticated.UserID)
				c.Status(http.StatusOK)
			})
			req := httptest.NewRequest(tc.method, tc.path, nil).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer "+key.Key)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tc.want, w.Code, w.Body.String())
			require.Equal(t, tc.want == http.StatusOK, called)
		})
	}
}
