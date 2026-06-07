package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// newRechargePromoTestRouter 用 SQLite + enttest 拉起一个最小路由，
// 路由结构必须与生产 RegisterPaymentRoutes 一致，否则 admin 端调用会 404。
func newRechargePromoTestRouter(t *testing.T) (*gin.Engine, *dbent.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", "file:recharge_promo_handler?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	svc := service.NewRechargePromoActivityService(client)
	h := NewRechargePromoHandler(svc)

	router := gin.New()
	v1 := router.Group("/api/v1")
	promo := v1.Group("/admin/recharge-promos")
	{
		promo.GET("", h.List)
		promo.POST("", h.Create)
		promo.GET("/:id", h.Get)
		promo.PUT("/:id", h.Update)
		promo.DELETE("/:id", h.Delete)
		promo.POST("/:id/toggle", h.Toggle)
	}
	return router, client
}

func doJSONRequest(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

// decodeData 把 {code,message,data} 中的 data 字段反序列化进 out。
func decodeData(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	var resp struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equalf(t, 0, resp.Code, "non-zero biz code: %s, body=%s", resp.Message, rec.Body.String())
	if out != nil {
		require.NoError(t, json.Unmarshal(resp.Data, out))
	}
}

// TestRechargePromoHandler_RouteRegistered 是回归测试：保证
// `/api/v1/admin/recharge-promos` 这套路径在 wire/handler 改动后仍可达，
// 不会被 gin 当作未注册路径返回 404。
func TestRechargePromoHandler_RouteRegistered(t *testing.T) {
	router, _ := newRechargePromoTestRouter(t)

	rec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/recharge-promos?page=1&page_size=20", nil)
	require.NotEqualf(t, http.StatusNotFound, rec.Code,
		"GET /api/v1/admin/recharge-promos should not 404; got body=%s", rec.Body.String())
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestRechargePromoHandler_CRUDFlow 走完一遍 Create -> List -> Get -> Update -> Delete，
// 用来兜住 handler / DTO / service 串联回归。
func TestRechargePromoHandler_CRUDFlow(t *testing.T) {
	router, _ := newRechargePromoTestRouter(t)
	from := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	until := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Second)

	createReq := map[string]any{
		"name":        "Spring Promo",
		"enabled":     false,
		"valid_from":  from,
		"valid_until": until,
		"tiers": []map[string]any{
			{"min_amount": 100, "bonus_rate": 0.05},
			{"min_amount": 500, "bonus_rate": 0.10},
		},
		"note": "init",
	}
	rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", createReq)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created adminPromoActivity
	decodeData(t, rec, &created)
	require.Greater(t, created.ID, int64(0))
	require.Equal(t, "Spring Promo", created.Name)
	require.False(t, created.Enabled)
	require.Len(t, created.Tiers, 2)

	rec = doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/recharge-promos?page=1&page_size=20", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var listResp struct {
		Items    []adminPromoActivity `json:"items"`
		Total    int64                `json:"total"`
		Page     int                  `json:"page"`
		PageSize int                  `json:"page_size"`
	}
	decodeData(t, rec, &listResp)
	require.EqualValues(t, 1, listResp.Total)
	require.Len(t, listResp.Items, 1)

	rec = doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/recharge-promos/"+itoa(created.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var got adminPromoActivity
	decodeData(t, rec, &got)
	require.Equal(t, created.ID, got.ID)

	updateReq := map[string]any{
		"name":    "Spring Promo v2",
		"enabled": false,
		"tiers": []map[string]any{
			{"min_amount": 200, "bonus_rate": 0.08},
		},
		"note": "renamed",
	}
	rec = doJSONRequest(t, router, http.MethodPut, "/api/v1/admin/recharge-promos/"+itoa(created.ID), updateReq)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var updated adminPromoActivity
	decodeData(t, rec, &updated)
	require.Equal(t, "Spring Promo v2", updated.Name)
	require.Len(t, updated.Tiers, 1)

	rec = doJSONRequest(t, router, http.MethodDelete, "/api/v1/admin/recharge-promos/"+itoa(created.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/recharge-promos/"+itoa(created.ID), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TestRechargePromoHandler_ToggleSingleEnabled 验证业务约束：
// 同一时间最多一条 enabled=TRUE，启用 B 时应自动关闭 A。
func TestRechargePromoHandler_ToggleSingleEnabled(t *testing.T) {
	router, client := newRechargePromoTestRouter(t)
	ctx := context.Background()

	makeBody := func(name string, enabled bool) map[string]any {
		return map[string]any{
			"name":    name,
			"enabled": enabled,
			"tiers": []map[string]any{
				{"min_amount": 100, "bonus_rate": 0.05},
			},
		}
	}

	rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", makeBody("A", true))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var a adminPromoActivity
	decodeData(t, rec, &a)
	require.True(t, a.Enabled)

	rec = doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", makeBody("B", false))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var b adminPromoActivity
	decodeData(t, rec, &b)
	require.False(t, b.Enabled)

	// Toggle B on -> A 应被服务层自动置 false
	rec = doJSONRequest(t, router, http.MethodPost,
		"/api/v1/admin/recharge-promos/"+itoa(b.ID)+"/toggle",
		map[string]any{"enabled": true})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var b2 adminPromoActivity
	decodeData(t, rec, &b2)
	require.True(t, b2.Enabled)

	// 验证 DB 上确实只有一条 enabled=TRUE
	count, err := client.RechargePromoActivity.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	rec = doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/recharge-promos/"+itoa(a.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var aReloaded adminPromoActivity
	decodeData(t, rec, &aReloaded)
	require.False(t, aReloaded.Enabled, "previously enabled activity must be flipped off")
}

// TestRechargePromoHandler_CreateRejectsBadRequest 校验后端参数兜底，避免前端绕过 UI 校验时入库脏数据。
func TestRechargePromoHandler_CreateRejectsBadRequest(t *testing.T) {
	router, _ := newRechargePromoTestRouter(t)

	// missing name
	rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", map[string]any{
		"enabled": false,
		"tiers":   []map[string]any{{"min_amount": 100, "bonus_rate": 0.05}},
	})
	require.GreaterOrEqual(t, rec.Code, 400)

	// enabled=true 但 tiers 为空
	rec = doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", map[string]any{
		"name":    "empty-tiers",
		"enabled": true,
		"tiers":   []map[string]any{},
	})
	require.GreaterOrEqual(t, rec.Code, 400)
}

// itoa 避免在 test 文件里直接 import strconv 仅为格式化整数。
func itoa(i int64) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	buf := [20]byte{}
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
