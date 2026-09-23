package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type simpleModeAccountService struct {
	*stubAdminService
	account     service.Account
	createCalls int
	updateCalls int
	bulkCalls   int
}

func (s *simpleModeAccountService) GetAccount(context.Context, int64) (*service.Account, error) {
	return &s.account, nil
}

func (s *simpleModeAccountService) CreateAccount(ctx context.Context, input *service.CreateAccountInput) (*service.Account, error) {
	if err := s.ValidateAccountGroupBindings(ctx, input.GroupIDs); err != nil {
		return nil, err
	}
	s.createCalls++
	return &s.account, nil
}

func (s *simpleModeAccountService) UpdateAccount(ctx context.Context, _ int64, input *service.UpdateAccountInput) (*service.Account, error) {
	if input.GroupIDs != nil {
		if err := s.ValidateAccountGroupBindings(ctx, *input.GroupIDs); err != nil {
			return nil, err
		}
	}
	s.updateCalls++
	return &s.account, nil
}

func (s *simpleModeAccountService) BulkUpdateAccounts(ctx context.Context, input *service.BulkUpdateAccountsInput) (*service.BulkUpdateAccountsResult, error) {
	if input.GroupIDs != nil {
		if err := s.ValidateAccountGroupBindings(ctx, *input.GroupIDs); err != nil {
			return nil, err
		}
	}
	s.bulkCalls++
	return &service.BulkUpdateAccountsResult{}, nil
}

func (s *simpleModeAccountService) ValidateAccountGroupBindings(_ context.Context, groupIDs []int64) error {
	for _, id := range groupIDs {
		for i := range s.groups {
			if s.groups[i].ID == id && s.groups[i].Platform == service.PlatformComposite {
				return infraerrors.BadRequest("SIMPLE_MODE_GROUP_NOT_BINDABLE", "composite groups cannot be bound in simple mode")
			}
		}
	}
	return nil
}

func TestAccountHandlerSimpleModeUsesMinimalGroupReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	richGroup := &service.Group{ID: 7, Name: "basic", Platform: service.PlatformAnthropic, Status: service.StatusActive, RateMultiplier: 9, IsExclusive: true, SubscriptionType: service.SubscriptionTypeSubscription, RPMLimit: 99}
	historicalComposite := &service.Group{ID: 9, Name: "historical composite", Platform: service.PlatformComposite, Status: service.StatusActive}
	account := service.Account{
		ID: 3, Name: "account", Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey, Status: service.StatusActive,
		GroupIDs: []int64{7, 9},
		Groups:   []*service.Group{richGroup, historicalComposite},
		AccountGroups: []service.AccountGroup{
			{AccountID: 3, GroupID: 7, Priority: 2, Group: richGroup, Account: &service.Account{ID: 3, Extra: map[string]any{"secret": true}}},
			{AccountID: 3, GroupID: 9, Priority: 3, Group: historicalComposite},
		},
	}
	svc := &simpleModeAccountService{stubAdminService: newStubAdminService(), account: account}
	svc.accounts = []service.Account{account}
	h := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.cfg = &config.Config{RunMode: config.RunModeSimple}
	r := gin.New()
	r.GET("/accounts", h.List)
	r.GET("/accounts/:id", h.GetByID)
	r.POST("/accounts", h.Create)
	r.PUT("/accounts/:id", h.Update)

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/accounts", ""},
		{http.MethodGet, "/accounts/3", ""},
		{http.MethodPost, "/accounts", `{"name":"account","platform":"anthropic","type":"apikey","credentials":{"key":"x"}}`},
		{http.MethodPut, "/accounts/3", `{"name":"account"}`},
	}
	for _, tt := range tests {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			r.ServeHTTP(res, req)
			require.Equal(t, http.StatusOK, res.Code, res.Body.String())
			var payload map[string]any
			require.NoError(t, json.Unmarshal(res.Body.Bytes(), &payload))
			data, ok := payload["data"].(map[string]any)
			require.True(t, ok)
			if items, ok := data["items"].([]any); ok {
				require.Len(t, items, 1)
				data, ok = items[0].(map[string]any)
				require.True(t, ok)
			}
			groups, ok := data["groups"].([]any)
			require.True(t, ok)
			require.Len(t, groups, 1)
			require.Equal(t, []any{float64(7)}, data["group_ids"])
			group, ok := groups[0].(map[string]any)
			require.True(t, ok)
			require.ElementsMatch(t, []string{"id", "name", "platform", "status"}, mapKeys(group))
			accountGroups, ok := data["account_groups"].([]any)
			require.True(t, ok)
			require.Len(t, accountGroups, 1)
			accountGroup, ok := accountGroups[0].(map[string]any)
			require.True(t, ok)
			require.ElementsMatch(t, []string{"account_id", "group_id", "priority", "created_at", "group"}, mapKeys(accountGroup))
			nestedGroup, ok := accountGroup["group"].(map[string]any)
			require.True(t, ok)
			require.ElementsMatch(t, []string{"id", "name", "platform", "status"}, mapKeys(nestedGroup))
		})
	}
}

func TestAccountHandlerSimpleModeLitePreservesCompactShapeAndETag(t *testing.T) {
	richGroup := &service.Group{ID: 7, Name: "basic", Platform: service.PlatformAnthropic, Status: service.StatusActive, RateMultiplier: 9}
	historicalComposite := &service.Group{ID: 9, Name: "historical composite", Platform: service.PlatformComposite, Status: service.StatusActive}
	account := service.Account{
		ID: 3, Name: "account", Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey, Status: service.StatusActive,
		GroupIDs: []int64{7, 9}, Groups: []*service.Group{richGroup, historicalComposite},
		AccountGroups: []service.AccountGroup{{AccountID: 3, GroupID: 7, Group: richGroup}, {AccountID: 3, GroupID: 9, Group: historicalComposite}},
	}
	svc := &simpleModeAccountService{stubAdminService: newStubAdminService(), account: account}
	svc.accounts = []service.Account{account}
	h := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.cfg = &config.Config{RunMode: config.RunModeSimple}
	r := gin.New()
	r.GET("/accounts", h.List)

	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/accounts?lite=1", nil))
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	require.NotEmpty(t, res.Header().Get("ETag"))
	var payload struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, []any{float64(7)}, payload.Data.Items[0]["group_ids"])
	require.NotContains(t, payload.Data.Items[0], "groups")
	require.NotContains(t, payload.Data.Items[0], "account_groups")

	notModified := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/accounts?lite=1", nil)
	req.Header.Set("If-None-Match", res.Header().Get("ETag"))
	r.ServeHTTP(notModified, req)
	require.Equal(t, http.StatusNotModified, notModified.Code)
}

func TestAccountHandlerSimpleModeRejectsCompositeGroupBindingsBeforeWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct{ name, method, path, body string }{
		{"create", http.MethodPost, "/accounts", `{"name":"account","platform":"anthropic","type":"apikey","credentials":{"key":"x"},"group_ids":[9]}`},
		{"update", http.MethodPut, "/accounts/3", `{"group_ids":[9]}`},
		{"bulk", http.MethodPost, "/accounts/bulk-update", `{"account_ids":[3],"group_ids":[9]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &simpleModeAccountService{stubAdminService: newStubAdminService()}
			svc.groups = []service.Group{{ID: 7, Platform: service.PlatformAnthropic}, {ID: 9, Platform: service.PlatformComposite}}
			h := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			h.cfg = &config.Config{RunMode: config.RunModeSimple}
			r := gin.New()
			r.POST("/accounts", h.Create)
			r.PUT("/accounts/:id", h.Update)
			r.POST("/accounts/bulk-update", h.BulkUpdate)
			res := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(res, req)
			require.Equal(t, http.StatusBadRequest, res.Code, res.Body.String())
			require.Zero(t, svc.createCalls)
			require.Zero(t, svc.updateCalls)
			require.Zero(t, svc.bulkCalls)
		})
	}
}

func TestAccountHandlerSimpleModePreservesBasicGroupBinding(t *testing.T) {
	svc := &simpleModeAccountService{stubAdminService: newStubAdminService(), account: service.Account{ID: 3}}
	svc.groups = []service.Group{{ID: 7, Platform: service.PlatformAnthropic}}
	h := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.cfg = &config.Config{RunMode: config.RunModeSimple}
	r := gin.New()
	r.POST("/accounts", h.Create)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewBufferString(`{"name":"account","platform":"anthropic","type":"apikey","credentials":{"key":"x"},"group_ids":[7]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	require.Equal(t, 1, svc.createCalls)
}

func TestAccountHandlerSimpleModeBatchPrevalidatesAllGroupsAtomically(t *testing.T) {
	svc := &simpleModeAccountService{stubAdminService: newStubAdminService()}
	svc.groups = []service.Group{{ID: 7, Platform: service.PlatformAnthropic}, {ID: 9, Platform: service.PlatformComposite}}
	h := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.cfg = &config.Config{RunMode: config.RunModeSimple}
	r := gin.New()
	r.POST("/accounts/batch", h.BatchCreate)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/accounts/batch", bytes.NewBufferString(`{"accounts":[{"name":"first","platform":"anthropic","type":"apikey","credentials":{"key":"x"},"group_ids":[7]},{"name":"second","platform":"anthropic","type":"apikey","credentials":{"key":"y"},"group_ids":[9]}]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code, res.Body.String())
	require.Zero(t, svc.createCalls)
}

func TestAccountHandlerAdvancedModeKeepsFullGroupReferences(t *testing.T) {
	group := &service.Group{ID: 7, Name: "advanced", RateMultiplier: 9, SubscriptionType: service.SubscriptionTypeSubscription}
	h := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	raw, err := json.Marshal(h.buildAccountResponseWithRuntime(context.Background(), &service.Account{Groups: []*service.Group{group}}))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"rate_multiplier":9`)
	require.Contains(t, string(raw), `"subscription_type":"subscription"`)
}
