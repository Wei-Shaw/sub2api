package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The channel config response contains the webhook signing secret in the clear,
// so it must never be cached by a browser or an intermediary proxy.
func TestGetNotificationChannelsSetsNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &settingHandlerRepoStub{values: map[string]string{}}
	handler := &SettingHandler{}
	handler.SetNotificationEmailService(service.NewNotificationEmailService(repo, nil))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/admin/settings/notification-channels", nil)

	handler.GetNotificationChannels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
}
