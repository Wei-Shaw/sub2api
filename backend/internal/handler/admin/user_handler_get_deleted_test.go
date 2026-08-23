package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type getByIDAdminStub struct {
	service.AdminService
}

func (s *getByIDAdminStub) GetUser(_ context.Context, _ int64) (*service.User, error) {
	return nil, service.ErrUserNotFound
}

func (s *getByIDAdminStub) GetUserIncludeDeleted(_ context.Context, id int64) (*service.User, error) {
	return &service.User{ID: id, Email: "del@test.com"}, nil
}

func setupGetByIDRouter(svc service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewUserHandler(svc, nil, nil, nil, nil, nil, nil)
	r.GET("/admin/users/:id", h.GetByID)
	return r
}

func TestAdminUserGetByID_IncludeDeleted(t *testing.T) {
	svc := &getByIDAdminStub{AdminService: newStubAdminService()}
	router := setupGetByIDRouter(svc)

	t.Run("normal path returns 404 for deleted user", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin/users/7", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("include_deleted=true returns 200", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin/users/7?include_deleted=true", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}
