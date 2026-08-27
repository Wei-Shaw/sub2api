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

type batchBanAdminServiceStub struct {
	*stubAdminService
	updated []int64
}

func (s *batchBanAdminServiceStub) UpdateUser(_ context.Context, id int64, input *service.UpdateUserInput) (*service.User, error) {
	s.updated = append(s.updated, id)
	return &service.User{ID: id, Status: input.Status}, nil
}

func TestUserHandlerBatchBanUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminService := &batchBanAdminServiceStub{stubAdminService: newStubAdminService()}
	handler := NewUserHandler(adminService, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/admin/users/batch-ban", handler.BatchBanUsers)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/admin/users/batch-ban", bytes.NewBufferString(`{"user_ids":[12, 13, 12]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{12, 13}, adminService.updated)

	var response struct {
		Data struct {
			Affected int `json:"affected"`
			Skipped  int `json:"skipped"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, 2, response.Data.Affected)
	require.Equal(t, 1, response.Data.Skipped)
}

func TestUserHandlerBatchBanUsersRejectsInvalidRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminService := &batchBanAdminServiceStub{stubAdminService: newStubAdminService()}
	handler := NewUserHandler(adminService, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/admin/users/batch-ban", handler.BatchBanUsers)

	for _, body := range []string{`{}`, `{"user_ids":[]}`, `{"user_ids":[0]}`} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/admin/users/batch-ban", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
	}
}
