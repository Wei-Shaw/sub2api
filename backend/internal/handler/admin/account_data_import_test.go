//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func importDataBody(t *testing.T, groupIDs []int64) []byte {
	t.Helper()
	payload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{{
				"name":        "acc",
				"platform":    service.PlatformOpenAI,
				"type":        service.AccountTypeAPIKey,
				"credentials": map[string]any{"api_key": "sk-test"},
			}},
		},
		"skip_default_group_bind": true,
	}
	if groupIDs != nil {
		payload["group_ids"] = groupIDs
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return body
}

func postImportData(router *gin.Engine, ctx context.Context, body []byte, idempotencyKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func useIdempotencyCoordinator(t *testing.T, coordinator *service.IdempotencyCoordinator) {
	t.Helper()
	previous := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(coordinator)
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previous) })
}

func TestImportDataBindsSelectedGroups(t *testing.T) {
	useIdempotencyCoordinator(t, nil)
	router, adminSvc := setupAccountDataRouter()

	rec := postImportData(router, context.Background(), importDataBody(t, []int64{7, 9}), "")
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 1)
	require.Equal(t, []int64{7, 9}, adminSvc.createdAccounts[0].GroupIDs)
}

func TestImportDataSameIdempotencyKeyCreatesAccountOnce(t *testing.T) {
	useIdempotencyCoordinator(t, service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), service.DefaultIdempotencyConfig()))
	router, adminSvc := setupAccountDataRouter()
	body := importDataBody(t, nil)

	first := postImportData(router, context.Background(), body, "import-retry-key")
	require.Equal(t, http.StatusOK, first.Code)
	second := postImportData(router, context.Background(), body, "import-retry-key")
	require.Equal(t, http.StatusOK, second.Code)

	require.Len(t, adminSvc.createdAccounts, 1, "a retried import with the same key must not create accounts again")
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Replayed"))
	require.JSONEq(t, first.Body.String(), second.Body.String())
}

type ctxRecordingAdminService struct {
	*stubAdminService
	createCtxErrs []error
}

func (s *ctxRecordingAdminService) CreateAccount(ctx context.Context, input *service.CreateAccountInput) (*service.Account, error) {
	s.createCtxErrs = append(s.createCtxErrs, ctx.Err())
	return s.stubAdminService.CreateAccount(ctx, input)
}

func TestImportDataKeepsRunningAfterClientDisconnect(t *testing.T) {
	useIdempotencyCoordinator(t, nil)
	gin.SetMode(gin.TestMode)
	svc := &ctxRecordingAdminService{stubAdminService: newStubAdminService()}
	h := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/accounts/data", h.ImportData)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the admin's browser gave up before the import finished

	rec := postImportData(router, ctx, importDataBody(t, nil), "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []error{nil}, svc.createCtxErrs, "account creation must not inherit the client's cancellation")
}
