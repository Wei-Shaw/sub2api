package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type clashPreviewEnvelope struct {
	Code int                       `json:"code"`
	Data ClashProxyPreviewResponse `json:"data"`
}

func setupClashPreviewRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/admin/proxies/clash/preview", NewProxyHandler(newStubAdminService()).PreviewClashImport)
	return router
}

func TestPreviewClashImportParsesSupportedProxies(t *testing.T) {
	router := setupClashPreviewRouter()
	body := []byte(`{
		"content": "proxies:\n  - name: basic-http\n    type: http\n    server: 127.0.0.1\n    port: 8080\n    username: user\n    password: pass\n  - name: secure-http\n    type: http\n    server: proxy.example.com\n    port: 443\n    tls: true\n  - name: disabled-socks\n    type: socks5\n    server: socks.example.com\n    port: 1080\n    enabled: false\n"
	}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies/clash/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp clashPreviewEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 3, resp.Data.Summary.Total)
	require.Equal(t, 3, resp.Data.Summary.Valid)
	require.Len(t, resp.Data.DataPayload.Proxies, 3)
	require.Equal(t, "https", resp.Data.DataPayload.Proxies[1].Protocol)
	require.Equal(t, "inactive", resp.Data.DataPayload.Proxies[2].Status)
	require.Len(t, resp.Data.BatchPayload.Proxies, 3)
}

func TestPreviewClashImportMarksInvalidAndDuplicateRows(t *testing.T) {
	router := setupClashPreviewRouter()
	body := []byte(`{
		"content": "proxies:\n  - name: first\n    type: http\n    server: 127.0.0.1\n    port: 8080\n  - name: duplicate\n    type: http\n    server: 127.0.0.1\n    port: 8080\n  - name: bad\n    type: vmess\n    server: vmess.example.com\n    port: 443\n  - name: missing-port\n    type: socks5\n    server: socks.example.com\n"
	}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies/clash/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp clashPreviewEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 4, resp.Data.Summary.Total)
	require.Equal(t, 1, resp.Data.Summary.Valid)
	require.Equal(t, 3, resp.Data.Summary.Invalid)
	require.Equal(t, 1, resp.Data.Summary.Duplicates)
	require.True(t, resp.Data.Rows[1].Duplicate)
	require.Contains(t, resp.Data.Rows[2].Errors[0], "unsupported proxy type")
	require.Contains(t, resp.Data.Rows[3].Errors[0], "port is invalid")
	require.Len(t, resp.Data.DataPayload.Proxies, 1)
}

func TestPreviewClashImportRejectsNonHTTPURL(t *testing.T) {
	router := setupClashPreviewRouter()
	body := []byte(`{"url":"file:///tmp/sub.yaml"}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies/clash/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
