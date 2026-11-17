package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/upstreampricing"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBrowserConnectorInstallerRequiresLoopbackReceiver(t *testing.T) {
	h := &UpstreamPricingHandler{}
	for _, origin := range []string{"http://localhost:8080", "https://remote.example", "http://localhost:8081"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "http://localhost:8080/script?site=https%3A%2F%2Fstation.example&origin="+url.QueryEscape(origin), nil)
		h.BrowserConnector(c)
		if origin == "http://localhost:8080" {
			require.Equal(t, 200, w.Code)
			require.Contains(t, w.Body.String(), "// @match        https://station.example/*")
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		} else {
			require.Equal(t, 400, w.Code)
			require.NotContains(t, w.Body.String(), "// ==UserScript==")
		}
	}
}

func TestBrowserConnectorHeaderCannotReplaceCapability(t *testing.T) {
	m := upstreampricing.NewManager(nil, nil, nil)
	defer m.Stop()
	h := NewUpstreamPricingHandler(&service.UpstreamPricingService{Manager: m})
	for _, action := range []string{"connector-info", "connector-complete", "connector-fail"} {
		for _, origin := range []string{"", "https://remote.example", "http://localhost:8080"} {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/v1/upstream-pricing/browser-import/"+action,
				strings.NewReader(`{"ticket":"`+strings.Repeat("a", 48)+`","token":"synthetic-never-return"}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("X-Sub2API-Connector-Origin", origin)
			c.Params = gin.Params{{Key: "action", Value: action}}
			h.BrowserImportReceive(c)
			require.Equal(t, 400, w.Code)
			require.NotContains(t, w.Body.String(), "synthetic-never-return")
		}
	}
}
