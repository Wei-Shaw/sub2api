package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func TestVideoAndDramaTaskRoutesRequireAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	v1 := router.Group("/api/v1")
	jwtAuth := middleware.JWTAuthMiddleware(func(c *gin.Context) {
		response.Unauthorized(c, "unauthorized")
		c.Abort()
	})
	RegisterUserRoutes(v1, &handler.Handlers{}, jwtAuth, nil)

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "list video tasks", method: http.MethodGet, path: "/api/v1/video/tasks", want: http.StatusUnauthorized},
		{name: "create video task", method: http.MethodPost, path: "/api/v1/video/tasks", want: http.StatusUnauthorized},
		{name: "list drama tasks", method: http.MethodGet, path: "/api/v1/drama/tasks", want: http.StatusUnauthorized},
		{name: "create drama task", method: http.MethodPost, path: "/api/v1/drama/tasks", want: http.StatusUnauthorized},
		{name: "unsupported video task method", method: http.MethodPut, path: "/api/v1/video/tasks", want: http.StatusNotFound},
		{name: "unsupported drama task method", method: http.MethodPut, path: "/api/v1/drama/tasks", want: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("%s %s status = %d, want %d; body=%s", tt.method, tt.path, rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}
