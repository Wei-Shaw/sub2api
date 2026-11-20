package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPluginCatalogRejectsInvalidScopeBeforeDirectoryAccess(t *testing.T) {
	handler := NewPluginHandler(nil)
	router := gin.New()
	router.GET("/plugins/:id/catalog", handler.CatalogInventory)
	for _, path := range []string{"/plugins/0/catalog", "/plugins/1/catalog?group_id=0", "/plugins/1/catalog?group_id=-1", "/plugins/1/catalog?group_id=abc"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusBadRequest, response.Code, path)
	}
}
