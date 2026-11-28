package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type renameAdminStub struct {
	service.AdminService
	preview *service.BulkAccountRenameInput
	items   []service.AccountRenameItem
	err     error
}

func (s *renameAdminStub) PreviewAccountRenames(_ context.Context, input *service.BulkAccountRenameInput) (*service.BulkAccountRenamePreview, error) {
	s.preview = input
	return &service.BulkAccountRenamePreview{Items: []service.AccountRenameItem{{AccountID: 2, OldName: "email@example.test", NewName: "GPT-Free-01"}}}, s.err
}
func (s *renameAdminStub) ApplyAccountRenames(_ context.Context, items []service.AccountRenameItem) (*service.BulkAccountRenameResult, error) {
	s.items = items
	return &service.BulkAccountRenameResult{Applied: len(items)}, s.err
}

func TestAccountRenamePreviewHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &renameAdminStub{}
	h := &AccountHandler{adminService: svc}
	router := gin.New()
	router.POST("/preview", h.PreviewAccountRenames)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/preview", bytes.NewBufferString(`{"account_ids":[2],"prefix":"GPT-Free-","start_number":1,"padding":2}`)))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "GPT-Free-01")
	require.Equal(t, []int64{2}, svc.preview.AccountIDs)
	require.Empty(t, svc.items)
	svc.err = service.ErrAccountRenameConflict
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/preview", bytes.NewBufferString(`{}`)))
	require.Equal(t, http.StatusConflict, recorder.Code)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/preview", bytes.NewBufferString(`{"padding":1.5}`)))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestAccountRenameApplyHandlerUsesReviewedNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &renameAdminStub{}
	h := &AccountHandler{adminService: svc}
	router := gin.New()
	router.POST("/rename", h.ApplyAccountRenames)
	body := `{"items":[{"account_id":2,"old_name":"email@example.test","new_name":"GPT-Free-01"}]}`
	req := httptest.NewRequest(http.MethodPost, "/rename", bytes.NewBufferString(body))
	req.Header.Set("Idempotency-Key", "test-bulk-rename-handler")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []service.AccountRenameItem{{AccountID: 2, OldName: "email@example.test", NewName: "GPT-Free-01"}}, svc.items)
	require.Contains(t, recorder.Body.String(), `"applied":1`)
}
