//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type crsPoolRepo struct{ *crsLongContextAccountRepo }

func (r *crsPoolRepo) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error) {
	var accounts []Account
	for _, account := range r.accounts {
		accounts = append(accounts, *account)
	}
	return accounts, nil
}

func TestCRSImportPoolNamingAndPreservedManagement(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserve local management", true: "pool members always preserve management"}[pooled], func(t *testing.T) {
			poolID := int64(7)
			existing := &Account{ID: 1, Name: "Local name", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusDisabled, Schedulable: false, Extra: map[string]any{"crs_account_id": "existing"}}
			if pooled {
				existing.SchedulingPoolID = &poolID
			}
			repo := &crsPoolRepo{newCRSLongContextAccountRepo(existing)}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/web/auth/login" {
					_, _ = w.Write([]byte(`{"success":true,"token":"test"}`))
					return
				}
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
					"openaiOAuthAccounts": []any{
						map[string]any{"id": "existing", "name": "Remote email", "isActive": true, "schedulable": true, "credentials": map[string]any{"access_token": "updated"}},
						map[string]any{"id": "new-oauth", "name": "new@example.test", "isActive": true, "schedulable": true, "credentials": map[string]any{"access_token": "new"}},
					},
					"openaiResponsesAccounts": []any{map[string]any{"id": "new-key", "name": "key", "isActive": true, "schedulable": true, "credentials": map[string]any{"api_key": "test"}}},
				}}))
			}))
			defer server.Close()
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			svc := NewCRSSyncService(repo, nil, nil, nil, nil, cfg)
			result, err := svc.SyncFromCRS(t.Context(), SyncFromCRSInput{BaseURL: server.URL, Username: "admin", Password: "test", SchedulingPoolID: &poolID, PreserveLocalManagement: !pooled, Naming: &AccountImportNaming{Prefix: "GPT-Free-", StartNumber: 1, Padding: 2}})
			require.NoError(t, err)
			require.Equal(t, 0, result.Failed)
			require.Equal(t, 2, result.Created)
			require.Equal(t, 1, result.Updated)
			require.Equal(t, "Local name", existing.Name)
			require.Equal(t, StatusDisabled, existing.Status)
			require.False(t, existing.Schedulable)
			require.Equal(t, "updated", existing.Credentials["access_token"])
			require.Equal(t, "GPT-Free-01", repo.accounts["new-oauth"].Name)
			require.Equal(t, &poolID, repo.accounts["new-oauth"].SchedulingPoolID)
			require.Equal(t, "GPT-Free-02", repo.accounts["new-key"].Name)
			require.Nil(t, repo.accounts["new-key"].SchedulingPoolID)
		})
	}
}
