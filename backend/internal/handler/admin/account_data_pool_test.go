package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestImportDataPoolAndNaming(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()
	poolID := int64(7)
	payload := DataImportRequest{
		SchedulingPoolID: &poolID,
		Naming:           &service.AccountImportNaming{Prefix: "GPT-Free-", StartNumber: 4, Padding: 2},
		Data: DataPayload{Proxies: []DataProxy{}, Accounts: []DataAccount{
			{Name: "mail@example.test", Platform: "openai", Type: "oauth", Credentials: map[string]any{"access_token": "test"}, Concurrency: 1},
			{Name: "key", Platform: "openai", Type: "apikey", Credentials: map[string]any{"api_key": "test"}, Concurrency: 1},
		}},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Len(t, adminSvc.createdAccounts, 2)
	require.Equal(t, "GPT-Free-04", adminSvc.createdAccounts[0].Name)
	require.Equal(t, &poolID, adminSvc.createdAccounts[0].SchedulingPoolID)
	require.Equal(t, "GPT-Free-05", adminSvc.createdAccounts[1].Name)
	require.Nil(t, adminSvc.createdAccounts[1].SchedulingPoolID)
}

func TestImportDataRejectsNameConflictsBeforeCreatingAnything(t *testing.T) {
	_, adminSvc := setupAccountDataRouter()
	adminSvc.accounts = []service.Account{{ID: 1, Name: "GPT-Free-01"}}
	h := &AccountHandler{adminService: adminSvc}
	_, err := h.importData(t.Context(), DataImportRequest{
		Naming: &service.AccountImportNaming{Prefix: "GPT-Free-", StartNumber: 1, Padding: 2},
		Data:   DataPayload{Accounts: []DataAccount{{Name: "email", Platform: "openai", Type: "oauth"}}},
	})
	require.ErrorContains(t, err, "already exists")
	require.Empty(t, adminSvc.createdAccounts)
}
