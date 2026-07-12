package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newOidcClientAdminTestRouter(t *testing.T) (*gin.Engine, *dbent.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	name := strings.ReplaceAll(t.Name(), "/", "_")
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	h := NewOidcClientHandler(service.NewOidcClientService(client))

	router := gin.New()
	g := router.Group("/api/v1/admin/oidc/clients")
	{
		g.GET("", h.List)
		g.POST("", h.Create)
		g.GET("/:id", h.Get)
		g.PATCH("/:id", h.Update)
		g.DELETE("/:id", h.Delete)
		g.POST("/:id/reset-secret", h.ResetSecret)
	}
	return router, client
}

func oidcAdminRequest(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// envelope 解析 {code,message,data}，data 保留为原始 JSON 以便逐字段断言。
type oidcAdminEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) oidcAdminEnvelope {
	t.Helper()
	var env oidcAdminEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	return env
}

// TestOidcClientHandler_Create_ReturnsSecretOnce 验证 Create 响应里包含明文 secret，
// 而后续 Get / List 都不暴露 secret (任务 4.10)。
func TestOidcClientHandler_Create_ReturnsSecretOnce(t *testing.T) {
	router, _ := newOidcClientAdminTestRouter(t)

	createBody := map[string]any{
		"client_name":    "Acme RP",
		"redirect_uris":  []string{"https://acme.example.com/cb"},
		"allowed_scopes": []string{"openid", "profile"},
		"enabled":        true,
	}
	w := oidcAdminRequest(t, router, http.MethodPost, "/api/v1/admin/oidc/clients", createBody)
	require.Equal(t, http.StatusCreated, w.Code)

	env := decodeEnvelope(t, w)
	var created map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &created))

	secret, ok := created["client_secret"].(string)
	require.True(t, ok, "Create 响应必须包含 client_secret")
	require.NotEmpty(t, secret)

	idFloat, ok := created["id"].(float64)
	require.True(t, ok, "Create 响应必须包含数值类型 id")
	id := int64(idFloat)
	require.NotZero(t, id)

	// Get 不得包含 client_secret 字段
	gw := oidcAdminRequest(t, router, http.MethodGet, fmt.Sprintf("/api/v1/admin/oidc/clients/%d", id), nil)
	require.Equal(t, http.StatusOK, gw.Code)
	genv := decodeEnvelope(t, gw)
	var got map[string]any
	require.NoError(t, json.Unmarshal(genv.Data, &got))
	_, hasSecret := got["client_secret"]
	require.False(t, hasSecret, "Get 响应不得暴露 client_secret")
	require.Equal(t, "Acme RP", got["client_name"])

	// List 同样不得包含 secret
	lw := oidcAdminRequest(t, router, http.MethodGet, "/api/v1/admin/oidc/clients", nil)
	require.Equal(t, http.StatusOK, lw.Code)
	require.NotContains(t, lw.Body.String(), "client_secret")
}

// TestOidcClientHandler_ResetSecret_ReturnsNewSecret 验证 reset-secret 返回新明文 secret。
func TestOidcClientHandler_ResetSecret_ReturnsNewSecret(t *testing.T) {
	router, _ := newOidcClientAdminTestRouter(t)

	w := oidcAdminRequest(t, router, http.MethodPost, "/api/v1/admin/oidc/clients", map[string]any{
		"client_name":    "RP",
		"redirect_uris":  []string{"https://rp.example.com/cb"},
		"allowed_scopes": []string{"openid"},
		"enabled":        true,
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(decodeEnvelope(t, w).Data, &created))
	idFloat, ok := created["id"].(float64)
	require.True(t, ok)
	id := int64(idFloat)
	originalSecret, ok := created["client_secret"].(string)
	require.True(t, ok)

	rw := oidcAdminRequest(t, router, http.MethodPost, fmt.Sprintf("/api/v1/admin/oidc/clients/%d/reset-secret", id), nil)
	require.Equal(t, http.StatusOK, rw.Code)
	var reset map[string]any
	require.NoError(t, json.Unmarshal(decodeEnvelope(t, rw).Data, &reset))
	newSecret, ok := reset["client_secret"].(string)
	require.True(t, ok)
	require.NotEmpty(t, newSecret)
	require.NotEqual(t, originalSecret, newSecret, "reset 后必须是新的 secret")
}

// TestOidcClientHandler_Create_RejectsInvalidScope 验证非法 scope → 400。
func TestOidcClientHandler_Create_RejectsInvalidScope(t *testing.T) {
	router, _ := newOidcClientAdminTestRouter(t)
	w := oidcAdminRequest(t, router, http.MethodPost, "/api/v1/admin/oidc/clients", map[string]any{
		"client_name":    "RP",
		"redirect_uris":  []string{"https://rp.example.com/cb"},
		"allowed_scopes": []string{"openid", "not-a-scope"},
		"enabled":        true,
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestOidcClientHandler_Get_NotFound 验证未知 id → 404。
func TestOidcClientHandler_Get_NotFound(t *testing.T) {
	router, _ := newOidcClientAdminTestRouter(t)
	w := oidcAdminRequest(t, router, http.MethodGet, "/api/v1/admin/oidc/clients/99999", nil)
	require.Equal(t, http.StatusNotFound, w.Code)
}
