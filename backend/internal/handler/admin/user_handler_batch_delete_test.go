package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type batchDeleteUserAdminServiceStub struct {
	*stubAdminService
	deleted []int64
	errors  map[int64]error
}

func (s *batchDeleteUserAdminServiceStub) DeleteUser(_ context.Context, id int64) error {
	if err := s.errors[id]; err != nil {
		return err
	}
	s.deleted = append(s.deleted, id)
	return nil
}

func TestUserHandlerBatchDeleteUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminService := &batchDeleteUserAdminServiceStub{
		stubAdminService: newStubAdminService(),
		errors:           map[int64]error{13: errors.New("cannot delete admin user")},
	}
	handler := NewUserHandler(adminService, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/admin/users/batch-delete", handler.BatchDeleteUsers)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/admin/users/batch-delete", bytes.NewBufferString(`{"user_ids":[12, 13, 12]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{12}, adminService.deleted)

	var payload struct {
		Data struct {
			Affected int `json:"affected"`
			Skipped  int `json:"skipped"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, 1, payload.Data.Affected)
	require.Equal(t, 2, payload.Data.Skipped)
}

func TestUserHandlerBatchDeleteUsersRejectsInvalidRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewUserHandler(&batchDeleteUserAdminServiceStub{stubAdminService: newStubAdminService()}, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/admin/users/batch-delete", handler.BatchDeleteUsers)

	for _, body := range []string{`{}`, `{"user_ids":[]}`, `{"user_ids":[0]}`} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/admin/users/batch-delete", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
	}
}

var _ service.AdminService = (*batchDeleteUserAdminServiceStub)(nil)
