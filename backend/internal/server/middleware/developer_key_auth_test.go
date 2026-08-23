//go:build unit

package middleware

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type middlewareDeveloperKeyUsers struct {
	service.UserRepository
	user *service.User
}

func (r *middlewareDeveloperKeyUsers) GetByID(context.Context, int64) (*service.User, error) {
	return r.user, nil
}

func TestDeveloperKeyAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	validPlaintext := "dev_" + strings.Repeat("A", base64.RawURLEncoding.EncodedLen(32))
	recording := &recordingDeveloperKeyRepo{row: &service.DeveloperKey{ID: 3, UserID: 7, Name: "test", KeyPrefix: "dev_AAAAAAAA"}}
	svc := service.NewDeveloperKeyService(recording, &middlewareDeveloperKeyUsers{user: &service.User{ID: 7, Status: service.StatusActive}})

	router := gin.New()
	router.Use(gin.HandlerFunc(NewDeveloperKeyAuthMiddleware(svc)))
	router.GET("/file", func(c *gin.Context) {
		subject, _ := GetAuthSubjectFromContext(c)
		key, _ := GetDeveloperKeyFromContext(c)
		c.JSON(http.StatusOK, gin.H{"user_id": subject.UserID, "key_id": key.ID})
	})

	for _, tc := range []struct {
		name   string
		header string
		want   int
	}{
		{name: "missing", want: http.StatusUnauthorized},
		{name: "wrong scheme", header: "Basic abc", want: http.StatusUnauthorized},
		{name: "valid", header: "Bearer " + validPlaintext, want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/file", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			router.ServeHTTP(w, req)
			require.Equal(t, tc.want, w.Code)
		})
	}
}

type recordingDeveloperKeyRepo struct {
	row *service.DeveloperKey
}

func (r *recordingDeveloperKeyRepo) Create(context.Context, *service.DeveloperKey, string) (*service.DeveloperKey, error) {
	return nil, service.ErrDeveloperKeyNotFound
}
func (r *recordingDeveloperKeyRepo) ListByUserID(context.Context, int64) ([]*service.DeveloperKey, error) {
	return nil, nil
}
func (r *recordingDeveloperKeyRepo) DeleteByUserID(context.Context, int64, int64) error { return nil }
func (r *recordingDeveloperKeyRepo) GetByHash(context.Context, string) (*service.DeveloperKey, error) {
	clone := *r.row
	return &clone, nil
}
func (r *recordingDeveloperKeyRepo) TouchLastUsed(context.Context, int64, time.Time) error {
	return nil
}
