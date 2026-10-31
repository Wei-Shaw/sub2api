//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type expiredMuseSessionRepository struct{ service.AccountRepository }

func (expiredMuseSessionRepository) GetByID(context.Context, int64) (*service.Account, error) {
	return nil, muse.ErrSessionExpired
}

func TestMuseAuthenticateDoesNotInvalidateAdminSessionOnAppExpiry(t *testing.T) {
	core := service.NewMuseCoreService(expiredMuseSessionRepository{}, nil, nil, &service.OpenAIGatewayService{})
	router := gin.New()
	router.POST("/admin/muse/accounts/:id/authenticate", NewMuseHandler(core).Authenticate)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/admin/muse/accounts/1/authenticate", nil))
	// The shared admin client treats 401 as Sub2API token expiry and refreshes or
	// clears the operator's login. Muse cookie expiry must remain an app error.
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "import fresh cookies") {
		t.Fatalf("app expiry must preserve the admin login: %d %s", recorder.Code, recorder.Body.String())
	}
}
