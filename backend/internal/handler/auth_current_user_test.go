//go:build unit

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAuthHandlerGetCurrentUserReturnsProfileCompatibilityFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	verifiedAt := time.Date(2026, 4, 20, 8, 30, 0, 0, time.UTC)
	repo := &userHandlerRepoStub{
		user: &service.User{
			ID:           31,
			Email:        "me@example.com",
			Username:     "linuxdo-handle",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
			AvatarURL:    "https://cdn.example.com/linuxdo.png",
			AvatarSource: "remote_url",
		},
			identities: []service.UserAuthIdentityRecord{
				{
					ProviderType:    "linuxdo",
					ProviderKey:     "linuxdo",
					ProviderSubject: "linuxdo-subject-31",
					VerifiedAt:      &verifiedAt,
					Metadata: map[string]any{
						"username":   "linuxdo-handle",
						"avatar_url": "https://cdn.example.com/linuxdo.png",
					},
				},
			},
		}

	handler := &AuthHandler{
		userService: service.NewUserService(repo, nil, nil, nil),
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 31})

	handler.GetCurrentUser(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, true, resp.Data["email_bound"])
	require.Equal(t, true, resp.Data["linuxdo_bound"])
	require.Equal(t, "https://cdn.example.com/linuxdo.png", resp.Data["avatar_url"])

	authBindings, ok := resp.Data["auth_bindings"].(map[string]any)
	require.True(t, ok)
	linuxdoBinding, ok := authBindings["linuxdo"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, linuxdoBinding["bound"])

	avatarSource, ok := resp.Data["avatar_source"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "linuxdo", avatarSource["provider"])
	require.Equal(t, "linuxdo", avatarSource["source"])

	profileSources, ok := resp.Data["profile_sources"].(map[string]any)
	require.True(t, ok)
	usernameSource, ok := profileSources["username"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "linuxdo", usernameSource["provider"])
	require.Equal(t, "linuxdo", usernameSource["source"])
}
