package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"entgo.io/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDingTalkUsageRevokesAllAccountAccess(t *testing.T) {
	for _, path := range []string{"/v1/usage", "/antigravity/v1/usage"} {
		t.Run(path, func(t *testing.T) {
			ctx := context.Background()
			directoryError := atomic.Value{}
			directoryError.Store("")
			auth, client := newDingTalkLoginTestHandler(t, "none", "", &directoryError)
			callDingTalkCallback(t, auth)
			identity, err := client.AuthIdentity.Query().Where(authidentity.ProviderTypeEQ("dingtalk")).Only(ctx)
			require.NoError(t, err)
			owner, err := auth.userService.GetByID(ctx, identity.UserID)
			require.NoError(t, err)
			tokens, err := auth.authService.GenerateTokenPair(ctx, owner, "")
			require.NoError(t, err)
			for _, key := range []string{"balance-check-key", "other-access-key"} {
				_, err := client.APIKey.Create().SetUserID(owner.ID).SetKey(key).SetName(key).Save(ctx)
				require.NoError(t, err)
			}
			cache := &dingTalkKeyInvalidationCache{}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			apiKeys := service.NewAPIKeyService(repository.NewAPIKeyRepository(client, nil), nil, nil, nil, nil, cache, cfg)
			organization := NewDingTalkOrganizationHandler(nil, auth.settingSvc, auth.userService, auth, apiKeys)
			gateway := &GatewayHandler{dingTalkOrganization: organization, userService: auth.userService}
			router := gin.New()
			keyAuth := gin.HandlerFunc(middleware.NewAPIKeyAuthMiddleware(apiKeys, nil, cfg))
			router.GET(path, keyAuth, gateway.Usage)
			router.POST("/v1/messages", keyAuth, func(c *gin.Context) { c.Status(http.StatusOK) })
			router.GET("/profile", gin.HandlerFunc(middleware.NewJWTAuthMiddleware(auth.authService, auth.userService, auth.settingSvc, nil)), func(c *gin.Context) { c.Status(http.StatusOK) })
			request := func(method, path, key string) *httptest.ResponseRecorder {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(method, path, nil)
				req.Header.Set("Authorization", "Bearer "+key)
				router.ServeHTTP(rec, req)
				return rec
			}
			active := request(http.MethodGet, path, "balance-check-key")
			require.Equal(t, http.StatusOK, active.Code, active.Body.String())
			require.Contains(t, active.Body.String(), `"balance"`)
			directoryError.Store(`{"errcode":60121,"errmsg":"departed"}`)
			// Normal inference authentication must not call the directory API.
			require.Equal(t, http.StatusOK, request(http.MethodPost, "/v1/messages", "other-access-key").Code)
			departed := request(http.MethodGet, path, "balance-check-key")
			require.Equal(t, http.StatusForbidden, departed.Code, departed.Body.String())
			require.Contains(t, departed.Body.String(), "DINGTALK_ACCOUNT_DEPARTED")
			require.NotContains(t, departed.Body.String(), `"balance"`)
			stored, err := client.User.Get(ctx, owner.ID)
			require.NoError(t, err)
			require.Equal(t, service.StatusDisabled, stored.Status)
			require.Equal(t, 2, cache.deleted)
			require.Equal(t, 2, cache.published)
			require.Equal(t, http.StatusUnauthorized, request(http.MethodPost, "/v1/messages", "other-access-key").Code)
			require.Equal(t, http.StatusUnauthorized, request(http.MethodGet, "/profile", tokens.AccessToken).Code)
			_, err = auth.authService.RefreshTokenPair(ctx, tokens.RefreshToken)
			require.ErrorIs(t, err, service.ErrRefreshTokenInvalid)
			directoryError.Store("")
			require.Equal(t, http.StatusUnauthorized, request(http.MethodGet, "/profile", tokens.AccessToken).Code)
			require.Contains(t, callDingTalkCallback(t, auth).Header().Get("Location"), "account_unavailable")
		})
	}
}

func TestDingTalkUsageMembershipOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, upstream string
		linked         bool
		want           int
	}{
		{"active", "", true, http.StatusOK},
		{"missing employee", `{"errcode":60111,"errmsg":"not found"}`, true, http.StatusForbidden},
		{"rate limited", `{"errcode":88,"errmsg":"rate limited"}`, true, http.StatusServiceUnavailable},
		{"permission denied", `{"errcode":60011,"errmsg":"permission denied"}`, true, http.StatusServiceUnavailable},
		{"malformed response", "invalid json", true, http.StatusServiceUnavailable},
		{"not linked", `{"errcode":60121,"errmsg":"must not query"}`, false, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			directoryError := atomic.Value{}
			directoryError.Store("")
			auth, client := newDingTalkLoginTestHandler(t, "none", "", &directoryError)
			callDingTalkCallback(t, auth)
			identity, err := client.AuthIdentity.Query().Where(authidentity.ProviderTypeEQ("dingtalk")).Only(ctx)
			require.NoError(t, err)
			if !tc.linked {
				require.NoError(t, client.AuthIdentity.DeleteOneID(identity.ID).Exec(ctx))
			}
			key, err := client.APIKey.Create().SetUserID(identity.UserID).SetKey("usage-outcome-key").SetName("key").SetQuota(100).Save(ctx)
			require.NoError(t, err)
			apiKeys := service.NewAPIKeyService(repository.NewAPIKeyRepository(client, nil), nil, nil, nil, nil, nil, nil)
			organization := NewDingTalkOrganizationHandler(nil, auth.settingSvc, auth.userService, auth, apiKeys)
			gateway := &GatewayHandler{dingTalkOrganization: organization, userService: auth.userService}
			directoryError.Store(tc.upstream)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: key.ID, UserID: key.UserID, Status: key.Status, Quota: key.Quota})
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.UserID})
			gateway.Usage(c)
			require.Equal(t, tc.want, rec.Code, rec.Body.String())
			stored, err := client.User.Get(ctx, identity.UserID)
			require.NoError(t, err)
			storedKey, err := client.APIKey.Get(ctx, key.ID)
			require.NoError(t, err)
			wantStatus := service.StatusActive
			if tc.want == http.StatusForbidden {
				wantStatus = service.StatusDisabled
			}
			require.Equal(t, wantStatus, stored.Status)
			require.Equal(t, wantStatus, storedKey.Status)
			if tc.want != http.StatusOK {
				require.NotContains(t, rec.Body.String(), `"remaining"`)
			}
		})
	}
}

func TestDingTalkAccessRevocationRollsBackTogether(t *testing.T) {
	ctx := context.Background()
	directoryError := atomic.Value{}
	directoryError.Store("")
	auth, client := newDingTalkLoginTestHandler(t, "none", "", &directoryError)
	callDingTalkCallback(t, auth)
	identity, err := client.AuthIdentity.Query().Where(authidentity.ProviderTypeEQ("dingtalk")).Only(ctx)
	require.NoError(t, err)
	key, err := client.APIKey.Create().SetUserID(identity.UserID).SetKey("rollback-key").SetName("key").Save(ctx)
	require.NoError(t, err)
	cache := &dingTalkKeyInvalidationCache{}
	apiKeys := service.NewAPIKeyService(repository.NewAPIKeyRepository(client, nil), nil, nil, nil, nil, cache, nil)
	organization := NewDingTalkOrganizationHandler(nil, auth.settingSvc, auth.userService, auth, apiKeys)
	client.APIKey.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			return nil, errors.New("key update unavailable")
		})
	})
	directoryError.Store(`{"errcode":60121,"errmsg":"departed"}`)
	require.ErrorIs(t, organization.validateUsageMembership(ctx, identity.UserID), errDingTalkMembershipUnavailable)
	user, err := client.User.Get(ctx, identity.UserID)
	require.NoError(t, err)
	storedKey, err := client.APIKey.Get(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, user.Status)
	require.Equal(t, service.StatusAPIKeyActive, storedKey.Status)
	require.Zero(t, cache.deleted)
	require.Zero(t, cache.published)
}
