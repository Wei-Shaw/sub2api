package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/web"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type keyGroupRouteAPIKeyRepo struct {
	service.APIKeyRepository
	current *service.APIKey
	cached  *service.APIKey
	reads   int
}

func cloneKeyGroupRouteKey(source *service.APIKey) *service.APIKey {
	clone := *source
	if source.User != nil {
		user := *source.User
		clone.User = &user
	}
	if source.Group != nil {
		group := *source.Group
		clone.Group = &group
	}
	return &clone
}

func (r *keyGroupRouteAPIKeyRepo) GetByKey(_ context.Context, key string) (*service.APIKey, error) {
	r.reads++
	if r.current == nil || key != r.current.Key {
		return nil, service.ErrAPIKeyNotFound
	}
	return cloneKeyGroupRouteKey(r.current), nil
}

func (r *keyGroupRouteAPIKeyRepo) GetByKeyForAuth(_ context.Context, key string) (*service.APIKey, error) {
	if key != r.cached.Key {
		return nil, service.ErrAPIKeyNotFound
	}
	return cloneKeyGroupRouteKey(r.cached), nil
}

type keyGroupRouteAccountRepo struct {
	service.AccountRepository
	groups  map[int64][]service.Account
	lookups []int64
}

func (r *keyGroupRouteAccountRepo) ListSchedulableByGroupID(_ context.Context, groupID int64) ([]service.Account, error) {
	r.lookups = append(r.lookups, groupID)
	return r.groups[groupID], nil
}

func newKeyGroupRouteTestRouter() (*gin.Engine, *keyGroupRouteAPIKeyRepo, *keyGroupRouteAccountRepo) {
	gin.SetMode(gin.TestMode)
	groupID := int64(8)
	group := &service.Group{ID: groupID, Name: "key-group", Status: service.StatusActive, Hydrated: true, Platform: service.PlatformOpenAI, SubscriptionType: service.SubscriptionTypeStandard}
	key := &service.APIKey{ID: 100, UserID: 7, Key: "key-group-test-secret", Status: service.StatusActive, GroupID: &groupID, Group: group, User: &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 0}}
	keys := &keyGroupRouteAPIKeyRepo{current: key, cached: cloneKeyGroupRouteKey(key)}
	accounts := &keyGroupRouteAccountRepo{groups: map[int64][]service.Account{8: {{ID: 5, Platform: service.PlatformOpenAI, Type: "apikey", Status: service.StatusActive, Credentials: map[string]any{"api_key": "upstream-test-secret", "model_mapping": map[string]any{"public-a": "actual-a"}}}}}}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	keyService := service.NewAPIKeyService(keys, nil, nil, nil, nil, nil, cfg)
	gatewayService := service.NewGatewayService(accounts, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	gatewayHandler := handler.NewGatewayHandler(gatewayService, nil, nil, nil, nil, nil, nil, nil, keyService, nil, nil, nil, nil, cfg, nil)
	router := gin.New()
	if web.HasEmbeddedFrontend() {
		router.Use(web.ServeEmbeddedFrontend())
	}
	RegisterGatewayRoutes(router, &handler.Handlers{Gateway: gatewayHandler, OpenAIGateway: &handler.OpenAIGatewayHandler{}}, servermiddleware.NewAPIKeyAuthMiddleware(keyService, nil, cfg), keyService, nil, nil, nil, nil, cfg)
	return router, keys, accounts
}

func requestKeyGroupInfo(router *gin.Engine, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/sub2api/key-group", nil)
	req.RemoteAddr = "192.0.2.1:43210"
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeKeyGroupInfo(t *testing.T, w *httptest.ResponseRecorder) struct {
	Group struct {
		ID   int64
		Name string
	}
	Models   []service.KeyGroupModel
	Revision string
} {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Group struct {
			ID   int64
			Name string
		}
		Models   []service.KeyGroupModel
		Revision string
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Models, 1)
	return body
}

func TestGatewayRoutesKeyGroupInfoRequiresAPIKeyAndHasNoCredentialLeak(t *testing.T) {
	router, keys, accounts := newKeyGroupRouteTestRouter()
	w := requestKeyGroupInfo(router, "")
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Empty(t, accounts.lookups)

	w = requestKeyGroupInfo(router, keys.current.Key)
	body := decodeKeyGroupInfo(t, w)
	require.Equal(t, int64(8), body.Group.ID)
	require.Equal(t, "actual-a", body.Models[0].UpstreamModel)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.NotContains(t, w.Body.String(), keys.current.Key)
	require.NotContains(t, w.Body.String(), "upstream-test-secret")
	require.NotContains(t, w.Body.String(), "credentials")
	require.NotContains(t, w.Body.String(), "account_id")
}

func TestGatewayRoutesKeyGroupInfoReadsGroupChangesImmediately(t *testing.T) {
	router, keys, accounts := newKeyGroupRouteTestRouter()
	before := decodeKeyGroupInfo(t, requestKeyGroupInfo(router, keys.current.Key))
	groupID := int64(9)
	keys.current.GroupID = &groupID
	keys.current.Group = &service.Group{ID: groupID, Name: "new-group", Status: service.StatusActive, Hydrated: true, Platform: service.PlatformOpenAI, SubscriptionType: service.SubscriptionTypeStandard}
	accounts.groups[9] = []service.Account{{ID: 6, Platform: service.PlatformOpenAI, Type: "apikey", Status: service.StatusActive, Credentials: map[string]any{"model_mapping": map[string]any{"public-b": "actual-b"}}}}

	after := decodeKeyGroupInfo(t, requestKeyGroupInfo(router, keys.current.Key))

	require.Equal(t, int64(9), after.Group.ID)
	require.Equal(t, "actual-b", after.Models[0].UpstreamModel)
	require.NotEqual(t, before.Revision, after.Revision)
	require.Equal(t, []int64{8, 9}, accounts.lookups)
	require.Equal(t, 2, keys.reads)
}

func TestGatewayRoutesKeyGroupInfoReadsAccountMappingsImmediately(t *testing.T) {
	router, keys, accounts := newKeyGroupRouteTestRouter()
	before := decodeKeyGroupInfo(t, requestKeyGroupInfo(router, keys.current.Key))
	accounts.groups[8][0].Credentials = map[string]any{"model_mapping": map[string]any{"public-b": "actual-b"}}

	after := decodeKeyGroupInfo(t, requestKeyGroupInfo(router, keys.current.Key))

	require.Equal(t, "actual-b", after.Models[0].UpstreamModel)
	require.NotEqual(t, before.Revision, after.Revision)
	require.Equal(t, []int64{8, 8}, accounts.lookups)
}

func TestGatewayRoutesKeyGroupInfoReadsGroupAllowlistChangesImmediately(t *testing.T) {
	router, keys, accounts := newKeyGroupRouteTestRouter()
	accounts.groups[8][0].Credentials = map[string]any{"model_mapping": map[string]any{"public-a": "actual-a", "public-b": "actual-b"}}
	keys.current.Group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"public-a"}}
	before := decodeKeyGroupInfo(t, requestKeyGroupInfo(router, keys.current.Key))
	keys.current.Group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"public-b"}}

	after := decodeKeyGroupInfo(t, requestKeyGroupInfo(router, keys.current.Key))

	require.Equal(t, "actual-a", before.Models[0].UpstreamModel)
	require.Equal(t, "actual-b", after.Models[0].UpstreamModel)
	require.NotEqual(t, before.Revision, after.Revision)
}

func TestGatewayRoutesKeyGroupInfoStillEnforcesCurrentAccessRestrictions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*service.APIKey)
		status int
	}{
		{"revoked key", func(k *service.APIKey) { k.Status = service.StatusDisabled }, http.StatusUnauthorized},
		{"expired key time", func(k *service.APIKey) { expired := time.Now().Add(-time.Minute); k.ExpiresAt = &expired }, http.StatusForbidden},
		{"expired key status", func(k *service.APIKey) { k.Status = service.StatusAPIKeyExpired }, http.StatusForbidden},
		{"disabled user", func(k *service.APIKey) { k.User.Status = service.StatusDisabled }, http.StatusUnauthorized},
		{"disabled group", func(k *service.APIKey) { k.Group.Status = service.StatusDisabled }, http.StatusForbidden},
		{"IP whitelist", func(k *service.APIKey) { k.IPWhitelist = []string{"192.168.50.5"} }, http.StatusForbidden},
		{"exclusive group access removed", func(k *service.APIKey) { k.Group.IsExclusive = true }, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, keys, accounts := newKeyGroupRouteTestRouter()
			tt.mutate(keys.current)
			w := requestKeyGroupInfo(router, keys.current.Key)
			require.Equal(t, tt.status, w.Code, w.Body.String())
			require.Empty(t, accounts.lookups)
			require.NotContains(t, w.Body.String(), "actual-a")
		})
	}
}
