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
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// checkoutInfoTestEnv is the minimum wiring needed to drive
// PaymentHandler.GetCheckoutInfo against a real sqlite ent client. It
// intentionally re-uses the production constructors (NewPaymentConfigService,
// NewRechargePromoActivityService, NewSettingRepository) so the test
// exercises the same code path the handler uses in production.
type checkoutInfoTestEnv struct {
	client      *dbent.Client
	configSvc   *service.PaymentConfigService
	activitySvc *service.RechargePromoActivityService
	handler     *PaymentHandler
}

func newCheckoutInfoTestEnv(t *testing.T, dbName string) *checkoutInfoTestEnv {
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

	settingRepo := repository.NewSettingRepository(client)
	activitySvc := service.NewRechargePromoActivityService(client)
	configSvc := service.NewPaymentConfigService(
		client,
		settingRepo,
		[]byte("0123456789abcdef0123456789abcdef"),
		activitySvc,
	)

	handler := NewPaymentHandler(nil, configSvc, nil)

	return &checkoutInfoTestEnv{
		client:      client,
		configSvc:   configSvc,
		activitySvc: activitySvc,
		handler:     handler,
	}
}

// checkoutInfoBody mirrors the JSON shape returned by GetCheckoutInfo. We
// only decode the recharge_promo field — everything else is exercised by
// other handler tests.
type checkoutInfoBody struct {
	Code int `json:"code"`
	Data struct {
		RechargePromo *struct {
			Enabled    bool       `json:"enabled"`
			ValidFrom  *time.Time `json:"valid_from"`
			ValidUntil *time.Time `json:"valid_until"`
			Tiers      []struct {
				MinAmount float64 `json:"min_amount"`
				BonusRate float64 `json:"bonus_rate"`
			} `json:"tiers"`
			Version string `json:"version"`
		} `json:"recharge_promo"`
	} `json:"data"`
}

func callCheckoutInfo(t *testing.T, h *PaymentHandler) checkoutInfoBody {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/checkout-info", nil)

	h.GetCheckoutInfo(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var body checkoutInfoBody
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	return body
}

// TestGetCheckoutInfo_NoActivity_OmitsRechargePromo ensures the response keeps
// `recharge_promo` absent (omitempty) when no campaign row exists at all.
func TestGetCheckoutInfo_NoActivity_OmitsRechargePromo(t *testing.T) {
	env := newCheckoutInfoTestEnv(t, "checkout_info_no_activity")

	body := callCheckoutInfo(t, env.handler)
	require.Nil(t, body.Data.RechargePromo, "recharge_promo must be omitted when no activity exists")
}

// TestGetCheckoutInfo_ActiveAndInWindow_ReturnsPromo covers the happy path:
// an enabled activity with a window straddling now must be surfaced as
// `recharge_promo` with tiers preserved verbatim and a non-empty version.
func TestGetCheckoutInfo_ActiveAndInWindow_ReturnsPromo(t *testing.T) {
	ctx := context.Background()
	env := newCheckoutInfoTestEnv(t, "checkout_info_active")

	now := time.Now()
	_, err := env.client.RechargePromoActivity.Create().
		SetName("active").
		SetEnabled(true).
		SetValidFrom(now.Add(-1 * time.Hour)).
		SetValidUntil(now.Add(24 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{
			{MinAmount: 100, BonusRate: 0.03},
			{MinAmount: 500, BonusRate: 0.05},
			{MinAmount: 1000, BonusRate: 0.08},
		}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	body := callCheckoutInfo(t, env.handler)
	require.NotNil(t, body.Data.RechargePromo, "recharge_promo must be present when active and in window")
	require.True(t, body.Data.RechargePromo.Enabled)
	require.NotEmpty(t, body.Data.RechargePromo.Version)
	require.Len(t, body.Data.RechargePromo.Tiers, 3)
	require.Equal(t, 100.0, body.Data.RechargePromo.Tiers[0].MinAmount)
	require.Equal(t, 0.03, body.Data.RechargePromo.Tiers[0].BonusRate)
	require.Equal(t, 1000.0, body.Data.RechargePromo.Tiers[2].MinAmount)
	require.Equal(t, 0.08, body.Data.RechargePromo.Tiers[2].BonusRate)
}

// TestGetCheckoutInfo_DisabledActivity_OmitsPromo: even if a row exists it
// must stay hidden when enabled = FALSE — matches GetCurrent's filter
// (`enabled = TRUE LIMIT 1`).
func TestGetCheckoutInfo_DisabledActivity_OmitsPromo(t *testing.T) {
	ctx := context.Background()
	env := newCheckoutInfoTestEnv(t, "checkout_info_disabled")

	now := time.Now()
	_, err := env.client.RechargePromoActivity.Create().
		SetName("disabled").
		SetEnabled(false).
		SetValidFrom(now.Add(-1 * time.Hour)).
		SetValidUntil(now.Add(24 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{
			{MinAmount: 100, BonusRate: 0.05},
		}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	body := callCheckoutInfo(t, env.handler)
	require.Nil(t, body.Data.RechargePromo, "recharge_promo must be omitted when activity is disabled")
}

// TestGetCheckoutInfo_FutureValidFrom_OmitsPromo verifies the time-window
// guard: an enabled activity whose ValidFrom is still in the future must
// stay hidden — handler-level IsActiveAt check.
func TestGetCheckoutInfo_FutureValidFrom_OmitsPromo(t *testing.T) {
	ctx := context.Background()
	env := newCheckoutInfoTestEnv(t, "checkout_info_future")

	now := time.Now()
	_, err := env.client.RechargePromoActivity.Create().
		SetName("future").
		SetEnabled(true).
		SetValidFrom(now.Add(24 * time.Hour)).
		SetValidUntil(now.Add(48 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{
			{MinAmount: 100, BonusRate: 0.05},
		}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	body := callCheckoutInfo(t, env.handler)
	require.Nil(t, body.Data.RechargePromo, "recharge_promo must be omitted before valid_from")
}

// TestGetCheckoutInfo_PastValidUntil_OmitsPromo: the inverse — an expired
// window also yields no promo on the wire.
func TestGetCheckoutInfo_PastValidUntil_OmitsPromo(t *testing.T) {
	ctx := context.Background()
	env := newCheckoutInfoTestEnv(t, "checkout_info_expired")

	now := time.Now()
	_, err := env.client.RechargePromoActivity.Create().
		SetName("expired").
		SetEnabled(true).
		SetValidFrom(now.Add(-48 * time.Hour)).
		SetValidUntil(now.Add(-1 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{
			{MinAmount: 100, BonusRate: 0.05},
		}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	body := callCheckoutInfo(t, env.handler)
	require.Nil(t, body.Data.RechargePromo, "recharge_promo must be omitted after valid_until")
}

// TestGetCheckoutInfo_VersionStable_AcrossReadsAndIdempotentToggle pins the
// red-dot dismissal contract: the version string must not change unless the
// row is materially mutated. Specifically:
//  1. Two consecutive GetCheckoutInfo reads return the same version.
//  2. SetEnabled(true) on an already-enabled row is a no-op (idempotent —
//     does not bump updated_at), so the version stays stable across it.
//
// If either invariant breaks, the frontend's
// `recharge-promo-seen:<userId>:<version>` key would silently invalidate
// after every page load and red dots would never stay dismissed.
func TestGetCheckoutInfo_VersionStable_AcrossReadsAndIdempotentToggle(t *testing.T) {
	ctx := context.Background()
	env := newCheckoutInfoTestEnv(t, "checkout_info_version_stable")

	now := time.Now()
	row, err := env.client.RechargePromoActivity.Create().
		SetName("version-stable").
		SetEnabled(true).
		SetValidFrom(now.Add(-1 * time.Hour)).
		SetValidUntil(now.Add(24 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{
			{MinAmount: 100, BonusRate: 0.05},
		}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	first := callCheckoutInfo(t, env.handler)
	require.NotNil(t, first.Data.RechargePromo)
	v1 := first.Data.RechargePromo.Version
	require.NotEmpty(t, v1)

	second := callCheckoutInfo(t, env.handler)
	require.NotNil(t, second.Data.RechargePromo)
	require.Equal(t, v1, second.Data.RechargePromo.Version,
		"version must be stable across consecutive read-only calls")

	// Idempotent SetEnabled(true) — already enabled, must not bump updated_at.
	_, err = env.activitySvc.SetEnabled(ctx, row.ID, true)
	require.NoError(t, err)

	third := callCheckoutInfo(t, env.handler)
	require.NotNil(t, third.Data.RechargePromo)
	require.Equal(t, v1, third.Data.RechargePromo.Version,
		"version must be stable across an idempotent SetEnabled(true) save")
}
