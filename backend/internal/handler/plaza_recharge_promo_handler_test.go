//go:build unit

package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// plazaPromoTestEnv 把 *PlazaHandler 接到一个真实的 ent sqlite 上，使用
// 与 production 一致的 RechargePromoActivityService —— GetCurrent / 时间窗
// / version 行为不再需要 mock。
type plazaPromoTestEnv struct {
	client      *dbent.Client
	activitySvc *service.RechargePromoActivityService
	handler     *PlazaHandler
}

func newPlazaPromoTestEnv(t *testing.T, dbName string) *plazaPromoTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", "file:"+dbName+"?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	activitySvc := service.NewRechargePromoActivityService(client)

	// PlazaHandler 仅需 promoActivity 注入；plazaService 留 nil（GetPublicRechargePromo
	// 不会触达 ListModelRows / ListPlanCards）。
	handler := &PlazaHandler{
		promoActivity: activitySvc,
	}

	return &plazaPromoTestEnv{
		client:      client,
		activitySvc: activitySvc,
		handler:     handler,
	}
}

// callPlazaPromo 调用 GET /api/v1/plaza/recharge-promo 并返回原始 JSON map
// （以便断言"字段不存在"而不是仅 zero-value）。第二个返回值是 status code。
func callPlazaPromo(t *testing.T, h *PlazaHandler, headers map[string]string) (map[string]any, int) {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plaza/recharge-promo", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	ctx.Request = req

	h.GetPublicRechargePromo(ctx)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	return raw, rec.Code
}

// extractPromo 从 {"code":0,"data":{"promo":...}} 顶层结构中拿出 promo 字段。
// promo 可能是 nil（对应 JSON null）或 map[string]any。
func extractPromo(t *testing.T, body map[string]any) any {
	t.Helper()
	require.Equal(t, float64(0), body["code"], "expected code=0, got body=%v", body)
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "data must be an object, got %v", body["data"])
	// 注意：DTO 没有 omitempty，所以 promo 字段一定存在；nil 时为 JSON null
	// （json.Unmarshal 会把 null 解析成 Go 的 nil interface）。
	val, present := data["promo"]
	require.True(t, present, "promo key must always be present in response")
	return val
}

// =====================================================================
// 任务 2.6 (a)：有活动 → 返回 promo
// =====================================================================

func TestPlazaPromo_ActiveAndInWindow_ReturnsPromo(t *testing.T) {
	ctx := context.Background()
	env := newPlazaPromoTestEnv(t, "plaza_promo_active")

	now := time.Now()
	validUntil := now.Add(24 * time.Hour)
	_, err := env.client.RechargePromoActivity.Create().
		SetName("夏日充值送 8%").
		SetEnabled(true).
		SetValidFrom(now.Add(-1 * time.Hour)).
		SetValidUntil(validUntil).
		SetTiers([]domain.RechargePromoTier{
			{MinAmount: 100, BonusRate: 0.03},
			{MinAmount: 500, BonusRate: 0.05},
			{MinAmount: 1000, BonusRate: 0.08},
		}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	body, code := callPlazaPromo(t, env.handler, nil)
	require.Equal(t, http.StatusOK, code)

	promo, ok := extractPromo(t, body).(map[string]any)
	require.True(t, ok, "promo must be a non-null object when an active activity exists")

	require.Equal(t, "夏日充值送 8%", promo["name"])
	require.NotEmpty(t, promo["version"])

	tiers, ok := promo["tiers"].([]any)
	require.True(t, ok)
	require.Len(t, tiers, 3)
	first, _ := tiers[0].(map[string]any)
	require.Equal(t, 100.0, first["min_amount"])
	require.InDelta(t, 0.03, first["bonus_rate"], 1e-9)
}

// =====================================================================
// 任务 2.6 (b)：无 enabled 行 → null
// =====================================================================

func TestPlazaPromo_NoEnabledRow_ReturnsNull(t *testing.T) {
	env := newPlazaPromoTestEnv(t, "plaza_promo_no_enabled")

	body, code := callPlazaPromo(t, env.handler, nil)
	require.Equal(t, http.StatusOK, code)
	require.Nil(t, extractPromo(t, body), "promo must be null when no enabled row exists")
}

func TestPlazaPromo_DisabledRow_ReturnsNull(t *testing.T) {
	ctx := context.Background()
	env := newPlazaPromoTestEnv(t, "plaza_promo_disabled")

	now := time.Now()
	_, err := env.client.RechargePromoActivity.Create().
		SetName("disabled").
		SetEnabled(false).
		SetValidFrom(now.Add(-1 * time.Hour)).
		SetValidUntil(now.Add(24 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{{MinAmount: 100, BonusRate: 0.05}}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	body, code := callPlazaPromo(t, env.handler, nil)
	require.Equal(t, http.StatusOK, code)
	require.Nil(t, extractPromo(t, body))
}

// =====================================================================
// 任务 2.6 (c)：时间窗外 → null（valid_from 未到 / valid_until 已过）
// =====================================================================

func TestPlazaPromo_FutureValidFrom_ReturnsNull(t *testing.T) {
	ctx := context.Background()
	env := newPlazaPromoTestEnv(t, "plaza_promo_future")

	now := time.Now()
	_, err := env.client.RechargePromoActivity.Create().
		SetName("future").
		SetEnabled(true).
		SetValidFrom(now.Add(24 * time.Hour)).
		SetValidUntil(now.Add(48 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{{MinAmount: 100, BonusRate: 0.05}}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	body, code := callPlazaPromo(t, env.handler, nil)
	require.Equal(t, http.StatusOK, code)
	require.Nil(t, extractPromo(t, body), "promo must be null before valid_from")
}

func TestPlazaPromo_PastValidUntil_ReturnsNull(t *testing.T) {
	ctx := context.Background()
	env := newPlazaPromoTestEnv(t, "plaza_promo_expired")

	now := time.Now()
	_, err := env.client.RechargePromoActivity.Create().
		SetName("expired").
		SetEnabled(true).
		SetValidFrom(now.Add(-48 * time.Hour)).
		SetValidUntil(now.Add(-1 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{{MinAmount: 100, BonusRate: 0.05}}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	body, code := callPlazaPromo(t, env.handler, nil)
	require.Equal(t, http.StatusOK, code)
	require.Nil(t, extractPromo(t, body), "promo must be null after valid_until")
}

// =====================================================================
// 任务 2.6 (d)：tiers 空 → null
// =====================================================================

func TestPlazaPromo_EmptyTiers_ReturnsNull(t *testing.T) {
	ctx := context.Background()
	env := newPlazaPromoTestEnv(t, "plaza_promo_empty_tiers")

	now := time.Now()
	_, err := env.client.RechargePromoActivity.Create().
		SetName("no-tiers").
		SetEnabled(true).
		SetValidFrom(now.Add(-1 * time.Hour)).
		SetValidUntil(now.Add(24 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{}). // 故意写一个空 tiers 列
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	body, code := callPlazaPromo(t, env.handler, nil)
	require.Equal(t, http.StatusOK, code)
	require.Nil(t, extractPromo(t, body), "promo must be null when tiers is empty")
}

// =====================================================================
// 任务 2.6 (e)：任意 Authorization header 被忽略
// =====================================================================

func TestPlazaPromo_IgnoresAuthorizationHeader(t *testing.T) {
	ctx := context.Background()
	env := newPlazaPromoTestEnv(t, "plaza_promo_ignore_auth")

	now := time.Now()
	_, err := env.client.RechargePromoActivity.Create().
		SetName("auth-ignored").
		SetEnabled(true).
		SetValidFrom(now.Add(-1 * time.Hour)).
		SetValidUntil(now.Add(24 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{{MinAmount: 100, BonusRate: 0.05}}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	// 不带头：返回 promo
	body1, code1 := callPlazaPromo(t, env.handler, nil)
	require.Equal(t, http.StatusOK, code1)
	promo1, ok := extractPromo(t, body1).(map[string]any)
	require.True(t, ok)

	// 带一个完全无效的 Bearer token：handler 必须忽略它，行为完全一致
	body2, code2 := callPlazaPromo(t, env.handler, map[string]string{
		"Authorization": "Bearer this-is-an-invalid-token-payload",
	})
	require.Equal(t, http.StatusOK, code2)
	promo2, ok := extractPromo(t, body2).(map[string]any)
	require.True(t, ok)

	require.Equal(t, promo1["name"], promo2["name"])
	require.Equal(t, promo1["version"], promo2["version"])
}

// =====================================================================
// 任务 2.6 (f)：响应 JSON 不包含 enabled / activity_id 字段
// =====================================================================

func TestPlazaPromo_ResponseOmitsEnabledAndActivityID(t *testing.T) {
	ctx := context.Background()
	env := newPlazaPromoTestEnv(t, "plaza_promo_omit_internal")

	now := time.Now()
	_, err := env.client.RechargePromoActivity.Create().
		SetName("omit-internal").
		SetEnabled(true).
		SetValidFrom(now.Add(-1 * time.Hour)).
		SetValidUntil(now.Add(24 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{{MinAmount: 100, BonusRate: 0.05}}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	body, code := callPlazaPromo(t, env.handler, nil)
	require.Equal(t, http.StatusOK, code)

	promo, ok := extractPromo(t, body).(map[string]any)
	require.True(t, ok)

	_, hasEnabled := promo["enabled"]
	require.False(t, hasEnabled, "public DTO must NOT expose `enabled` (presence implies enabled)")
	_, hasActivityID := promo["activity_id"]
	require.False(t, hasActivityID, "public DTO must NOT expose `activity_id` (internal audit field)")

	// 对照：合法字段必须存在
	for _, key := range []string{"name", "tiers", "version"} {
		_, ok := promo[key]
		require.True(t, ok, "public DTO must contain %q", key)
	}
}

// =====================================================================
// 任务 3.1：plaza/recharge-promo.version 与 checkout-info.recharge_promo.version
//
//	在同一 admin-config 状态下严格相等
//
// =====================================================================

func TestPlazaPromo_VersionMatches_CheckoutInfo(t *testing.T) {
	ctx := context.Background()

	// 使用与 checkout-info 测试一致的 wiring（同一进程、同一 sqlite 内存库
	// 通过 cache=shared 共享），并用同一 dbName 让两个 env 命中同一份数据。
	dbName := "plaza_promo_version_match"
	checkoutEnv := newCheckoutInfoTestEnv(t, dbName)
	plazaEnv := newPlazaPromoTestEnv(t, dbName)

	now := time.Now()
	_, err := checkoutEnv.client.RechargePromoActivity.Create().
		SetName("version-match").
		SetEnabled(true).
		SetValidFrom(now.Add(-1 * time.Hour)).
		SetValidUntil(now.Add(24 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{{MinAmount: 100, BonusRate: 0.05}}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	// 走 checkout-info 拿一次
	checkoutBody := callCheckoutInfo(t, checkoutEnv.handler)
	require.NotNil(t, checkoutBody.Data.RechargePromo)
	checkoutVersion := checkoutBody.Data.RechargePromo.Version
	require.NotEmpty(t, checkoutVersion)

	// 走 plaza 公共端点拿一次
	plazaBody, code := callPlazaPromo(t, plazaEnv.handler, nil)
	require.Equal(t, http.StatusOK, code)
	plazaPromo, ok := extractPromo(t, plazaBody).(map[string]any)
	require.True(t, ok)
	plazaVersion, _ := plazaPromo["version"].(string)
	require.NotEmpty(t, plazaVersion)

	require.Equal(t, checkoutVersion, plazaVersion,
		"checkout-info and plaza/recharge-promo MUST emit identical version strings"+
			" — they are the dismiss-key contract for the red-dot")
}

// =====================================================================
// 任务 3.2：admin 仅修改 name 字段保存后，version 必须随之变化
// =====================================================================

func TestPlazaPromo_VersionChangesWhenNameOnlyUpdated(t *testing.T) {
	ctx := context.Background()
	env := newPlazaPromoTestEnv(t, "plaza_promo_version_name_change")

	now := time.Now()
	row, err := env.client.RechargePromoActivity.Create().
		SetName("初始名称").
		SetEnabled(true).
		SetValidFrom(now.Add(-1 * time.Hour)).
		SetValidUntil(now.Add(24 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{{MinAmount: 100, BonusRate: 0.05}}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	body1, _ := callPlazaPromo(t, env.handler, nil)
	promo1, ok := extractPromo(t, body1).(map[string]any)
	require.True(t, ok)
	v1, _ := promo1["version"].(string)
	require.NotEmpty(t, v1)

	// 至少等 1 秒，确保 updated_at 的 unix 秒级时间戳一定会变化
	// （Version = "{id}:{updated_at_unix}"）。
	time.Sleep(1100 * time.Millisecond)

	// 仅改 name 字段
	newName := "升级后的名称"
	_, err = env.activitySvc.Update(ctx, row.ID, service.UpdateActivityInput{
		Name:     &newName,
		Operator: "admin",
	})
	require.NoError(t, err)

	body2, _ := callPlazaPromo(t, env.handler, nil)
	promo2, ok := extractPromo(t, body2).(map[string]any)
	require.True(t, ok)
	v2, _ := promo2["version"].(string)
	require.NotEmpty(t, v2)

	require.Equal(t, "升级后的名称", promo2["name"])
	require.NotEqual(t, v1, v2,
		"version MUST change after admin updates only the name field — "+
			"otherwise frontend dismiss-key would falsely match and the new "+
			"campaign title would never trigger a fresh red dot")
}
