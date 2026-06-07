package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// decodeErrorReason 把响应体里的 {code,message,reason} 解出来，专门用于断言
// 业务错误的 reason 字段。普通成功路径请走 decodeData。
func decodeErrorReason(t *testing.T, rec *httptest.ResponseRecorder) (int, string) {
	t.Helper()
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Reason  string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return rec.Code, resp.Reason
}

// TestRechargePromoHandler_AcceptsValidThreeTierSave 是 task 5.2 的“happy path”：
// 一次合法的 3 档保存（按 min_amount 升序、bonus_rate 都在 [0,1)、日期顺序合法）
// 必须以 201 + INVALID_RECHARGE_PROMO 缺席 落库。
func TestRechargePromoHandler_AcceptsValidThreeTierSave(t *testing.T) {
	router, _ := newRechargePromoTestRouter(t)
	from := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	until := from.Add(30 * 24 * time.Hour)

	body := map[string]any{
		"name":        "valid-3-tier",
		"enabled":     true,
		"valid_from":  from,
		"valid_until": until,
		"tiers": []map[string]any{
			{"min_amount": 100, "bonus_rate": 0.03},
			{"min_amount": 500, "bonus_rate": 0.05},
			{"min_amount": 1000, "bonus_rate": 0.08},
		},
	}
	rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var created adminPromoActivity
	decodeData(t, rec, &created)
	require.True(t, created.Enabled)
	require.Len(t, created.Tiers, 3)
	require.Equal(t, 100.0, created.Tiers[0].MinAmount)
	require.Equal(t, 0.08, created.Tiers[2].BonusRate)
}

// TestRechargePromoHandler_RejectsNonAscendingTiers 校验 tiers 必须按 min_amount
// 严格升序——若管理员把第二档配成更低的 min_amount，validateRechargePromo 会
// 抛 INVALID_RECHARGE_PROMO 并被 handler 转译为 4xx。
func TestRechargePromoHandler_RejectsNonAscendingTiers(t *testing.T) {
	router, _ := newRechargePromoTestRouter(t)

	body := map[string]any{
		"name":    "non-ascending",
		"enabled": true,
		"tiers": []map[string]any{
			{"min_amount": 500, "bonus_rate": 0.05},
			{"min_amount": 100, "bonus_rate": 0.03}, // ← 倒序
		},
	}
	rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", body)
	code, reason := decodeErrorReason(t, rec)
	require.GreaterOrEqual(t, code, 400, "non-ascending tiers must be rejected, body=%s", rec.Body.String())
	require.Equal(t, "INVALID_RECHARGE_PROMO", reason)
}

// TestRechargePromoHandler_RejectsRateOutOfRange 校验 bonus_rate ∈ [0, 1)：
// rate >= 1 在产品上等价于"赠送 ≥ 100%"，必须拦下；rate < 0 同理。
func TestRechargePromoHandler_RejectsRateOutOfRange(t *testing.T) {
	router, _ := newRechargePromoTestRouter(t)

	t.Run("rate equals 1", func(t *testing.T) {
		body := map[string]any{
			"name":    "rate-eq-1",
			"enabled": true,
			"tiers": []map[string]any{
				{"min_amount": 100, "bonus_rate": 1.0},
			},
		}
		rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", body)
		code, reason := decodeErrorReason(t, rec)
		require.GreaterOrEqual(t, code, 400)
		require.Equal(t, "INVALID_RECHARGE_PROMO", reason)
	})

	t.Run("rate above 1", func(t *testing.T) {
		body := map[string]any{
			"name":    "rate-gt-1",
			"enabled": true,
			"tiers": []map[string]any{
				{"min_amount": 100, "bonus_rate": 1.5},
			},
		}
		rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", body)
		code, reason := decodeErrorReason(t, rec)
		require.GreaterOrEqual(t, code, 400)
		require.Equal(t, "INVALID_RECHARGE_PROMO", reason)
	})

	t.Run("rate negative", func(t *testing.T) {
		body := map[string]any{
			"name":    "rate-negative",
			"enabled": true,
			"tiers": []map[string]any{
				{"min_amount": 100, "bonus_rate": -0.05},
			},
		}
		rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", body)
		code, reason := decodeErrorReason(t, rec)
		require.GreaterOrEqual(t, code, 400)
		require.Equal(t, "INVALID_RECHARGE_PROMO", reason)
	})
}

// TestRechargePromoHandler_RejectsEmptyTiersWhenEnabled 是 5.2 显式列出的失败
// 模式之一：enabled=true 时不允许 tiers 为空（否则前端会渲染一个永远命中不了
// 任何档位的"假活动"）。disabled 时则不校验，作为反例保护。
func TestRechargePromoHandler_RejectsEmptyTiersWhenEnabled(t *testing.T) {
	router, _ := newRechargePromoTestRouter(t)

	t.Run("empty tiers with enabled=true is rejected", func(t *testing.T) {
		body := map[string]any{
			"name":    "empty-tiers-enabled",
			"enabled": true,
			"tiers":   []map[string]any{},
		}
		rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", body)
		code, reason := decodeErrorReason(t, rec)
		require.GreaterOrEqual(t, code, 400)
		require.Equal(t, "INVALID_RECHARGE_PROMO", reason)
	})

	t.Run("empty tiers with enabled=false is allowed (草稿态)", func(t *testing.T) {
		body := map[string]any{
			"name":    "empty-tiers-disabled",
			"enabled": false,
			"tiers":   []map[string]any{},
		}
		rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", body)
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	})
}

// TestRechargePromoHandler_RejectsInvertedDateOrder 校验 valid_from 必须严格
// 早于 valid_until。等于或反序都会被 INVALID_RECHARGE_PROMO 兜底拒绝。
func TestRechargePromoHandler_RejectsInvertedDateOrder(t *testing.T) {
	router, _ := newRechargePromoTestRouter(t)
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("valid_until before valid_from", func(t *testing.T) {
		body := map[string]any{
			"name":        "date-inverted",
			"enabled":     true,
			"valid_from":  now.Add(48 * time.Hour),
			"valid_until": now,
			"tiers": []map[string]any{
				{"min_amount": 100, "bonus_rate": 0.05},
			},
		}
		rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", body)
		code, reason := decodeErrorReason(t, rec)
		require.GreaterOrEqual(t, code, 400)
		require.Equal(t, "INVALID_RECHARGE_PROMO", reason)
	})

	t.Run("valid_until equal to valid_from", func(t *testing.T) {
		body := map[string]any{
			"name":        "date-equal",
			"enabled":     true,
			"valid_from":  now,
			"valid_until": now,
			"tiers": []map[string]any{
				{"min_amount": 100, "bonus_rate": 0.05},
			},
		}
		rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", body)
		code, reason := decodeErrorReason(t, rec)
		require.GreaterOrEqual(t, code, 400)
		require.Equal(t, "INVALID_RECHARGE_PROMO", reason)
	})
}

// TestRechargePromoHandler_RejectsZeroOrNegativeMinAmount 锁住校验里另一处
// 边界——min_amount 必须 > 0，否则 ResolveTier 的 payAmount + 1e-9 >= MinAmount
// 会把 0 元订单也错误地视作命中赠送。
func TestRechargePromoHandler_RejectsZeroOrNegativeMinAmount(t *testing.T) {
	router, _ := newRechargePromoTestRouter(t)

	t.Run("min_amount=0", func(t *testing.T) {
		body := map[string]any{
			"name":    "min-zero",
			"enabled": true,
			"tiers": []map[string]any{
				{"min_amount": 0, "bonus_rate": 0.05},
			},
		}
		rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", body)
		code, reason := decodeErrorReason(t, rec)
		require.GreaterOrEqual(t, code, 400)
		require.Equal(t, "INVALID_RECHARGE_PROMO", reason)
	})

	t.Run("min_amount<0", func(t *testing.T) {
		body := map[string]any{
			"name":    "min-negative",
			"enabled": true,
			"tiers": []map[string]any{
				{"min_amount": -100, "bonus_rate": 0.05},
			},
		}
		rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/recharge-promos", body)
		code, reason := decodeErrorReason(t, rec)
		require.GreaterOrEqual(t, code, 400)
		require.Equal(t, "INVALID_RECHARGE_PROMO", reason)
	})
}
