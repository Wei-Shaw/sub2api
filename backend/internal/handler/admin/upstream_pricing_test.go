package admin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpstreamPricingRestrictedAdminDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &UpstreamPricingHandler{}
	for _, action := range []func(*gin.Context){h.Get, h.Groups, h.Preview, h.Apply, h.Sync, h.Schedule, h.RollbackPreview, h.Comparison, h.StartBrowserImport, h.BrowserImportStatus, h.Activity} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/admin/upstream-pricing/accounts/7", nil)
		c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{KeyID: "restricted", Policy: adminaccess.Policy{}}))
		c.Params = gin.Params{{Key: "id", Value: "7"}}
		action(c)
		require.Equal(t, 403, w.Code)
	}
}

func TestUpstreamPricingErrorAndAuditDoNotExposeCredential(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	pricingError(c, errors.New("database failure containing synthetic-panel-credential"))
	require.Equal(t, 503, w.Code)
	require.NotContains(t, w.Body.String(), "synthetic-panel-credential")
	redacted := service.RedactAuditBody([]byte(`{"revision":1,"config":{"base_url":"https://example.com","token":"synthetic-panel-credential"}}`), "application/json")
	require.NotContains(t, redacted, "synthetic-panel-credential")
	require.Contains(t, redacted, "https://example.com")
}
