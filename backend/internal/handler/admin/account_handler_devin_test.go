package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDevinAccountAvailableModels(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mapping map[string]any
		want    []string
	}{
		{"defaults", nil, []string{"swe-1-6", "swe-1-6-fast"}},
		{"configured-request-keys", map[string]any{"z-alias": "swe-1-6", "a-alias": "swe-1-6-fast"}, []string{"a-alias", "z-alias"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &availableModelsAdminService{
				stubAdminService: newStubAdminService(),
				account: service.Account{ID: 44, Platform: service.PlatformDevin, Type: service.AccountTypeOAuth,
					Credentials: map[string]any{"model_mapping": tc.mapping}},
			}
			router := setupAvailableModelsRouter(svc)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/44/models", nil))
			require.Equal(t, http.StatusOK, rec.Code)
			var response struct {
				Data []openai.Model `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
			ids := make([]string, 0, len(response.Data))
			for _, model := range response.Data {
				ids = append(ids, model.ID)
				require.Equal(t, "model", model.Object)
				require.Equal(t, "model", model.Type)
				require.Equal(t, model.ID, model.DisplayName)
			}
			require.Equal(t, tc.want, ids)
		})
	}
}

func TestDevinRefreshDoesNotFallThroughToClaude(t *testing.T) {
	// Nil OAuth services intentionally panic if this falls through to any provider.
	handler := &AccountHandler{}
	updated, warning, err := handler.refreshSingleAccount(context.Background(), &service.Account{
		Platform: service.PlatformDevin, Type: service.AccountTypeOAuth,
	})
	require.Nil(t, updated)
	require.Empty(t, warning)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.ErrorContains(t, err, "sign in again")
	require.ErrorContains(t, err, "CLI session token")
}
