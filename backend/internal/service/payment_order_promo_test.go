//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// TestCheckRechargePromoExpired 守护 CreateOrder 阶段对充值赠送活动到期的
// 二次拦截。该函数刻意做成纯函数（不依赖 entClient / userRepo / loadBalancer），
// 用例只覆盖判定本身——避免把 selectInstance / DB 这些下游噪音卷进来。
//
// 对每条用例的关注点：
//   - 用 reason 而不是 message 文本断言错误，让文案改动不破坏测试；
//   - 时间统一传 fixedNow，明确"服务端当前时间"这一关键参数；
//   - 凡是预期"放行"的 case，期望 err == nil（不再做下游模拟）。
func TestCheckRechargePromoExpired(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	pastValidUntil := fixedNow.Add(-1 * time.Hour)
	futureValidUntil := fixedNow.Add(24 * time.Hour)
	pastValidFrom := fixedNow.Add(-24 * time.Hour)

	// activeCfg 是"服务端此刻仍在窗口期、能正常发赠送"的标准配置：
	// multiplier=1, 单档 100/5%。用例中需要"非到期/非禁用"的场景都
	// 引用它。
	activeCfg := &PaymentConfig{
		BalanceRechargeMultiplier: 1,
		RechargePromo: &RechargePromo{
			Enabled:    true,
			ValidFrom:  &pastValidFrom,
			ValidUntil: &futureValidUntil,
			Tiers: []RechargePromoTier{
				{MinAmount: 100, BonusRate: 0.05},
			},
		},
	}

	expiredCfg := &PaymentConfig{
		BalanceRechargeMultiplier: 1,
		RechargePromo: &RechargePromo{
			Enabled:    true,
			ValidFrom:  &pastValidFrom,
			ValidUntil: &pastValidUntil, // 已经过期 1 小时
			Tiers: []RechargePromoTier{
				{MinAmount: 100, BonusRate: 0.05},
			},
		},
	}

	disabledCfg := &PaymentConfig{
		BalanceRechargeMultiplier: 1,
		RechargePromo: &RechargePromo{
			Enabled:    false, // admin 中途禁用
			ValidFrom:  &pastValidFrom,
			ValidUntil: &futureValidUntil,
			Tiers: []RechargePromoTier{
				{MinAmount: 100, BonusRate: 0.05},
			},
		},
	}

	noPromoCfg := &PaymentConfig{
		BalanceRechargeMultiplier: 1,
		RechargePromo:             nil, // admin 删除 / 从未启用
	}

	tests := []struct {
		name       string
		req        CreateOrderRequest
		cfg        *PaymentConfig
		wantReason string // 空 = 期望放行
	}{
		{
			// 正常路径：用户期待赠送、服务端确认仍能发 → 放行。是核心
			// 不变量——一旦这条不通过，会把所有现有 balance 充值流量都
			// 误锁回 409，事故烈度极高，必须先守住。
			name: "active_promo_with_expectation_passes",
			req: CreateOrderRequest{
				OrderType:           payment.OrderTypeBalance,
				Amount:              200,
				ClientExpectedBonus: 10,
			},
			cfg:        activeCfg,
			wantReason: "",
		},
		{
			// 用户停留过久 → valid_until 已过；client 拿到的是页面打开
			// 时的快照，仍展示着 $10 赠送 → 服务端必须 409 让前端弹
			// 二次确认。
			name: "expired_promo_with_expectation_returns_409",
			req: CreateOrderRequest{
				OrderType:           payment.OrderTypeBalance,
				Amount:              200,
				ClientExpectedBonus: 10,
			},
			cfg:        expiredCfg,
			wantReason: "RECHARGE_PROMO_EXPIRED",
		},
		{
			// 同上，用户在 modal 上点"继续充值" → 重发带 ack=true → 放行；
			// fulfillment 仍以服务器时间核账，不会误发赠送。
			name: "expired_promo_with_ack_passes",
			req: CreateOrderRequest{
				OrderType:                payment.OrderTypeBalance,
				Amount:                   200,
				ClientExpectedBonus:      10,
				PromoExpiredAcknowledged: true,
			},
			cfg:        expiredCfg,
			wantReason: "",
		},
		{
			// admin 中途把活动 disable —— 用户视角与"valid_until 已过"
			// 没区别：客户端仍展示赠送、服务端不再发。同样 409。
			name: "disabled_promo_with_expectation_returns_409",
			req: CreateOrderRequest{
				OrderType:           payment.OrderTypeBalance,
				Amount:              200,
				ClientExpectedBonus: 10,
			},
			cfg:        disabledCfg,
			wantReason: "RECHARGE_PROMO_EXPIRED",
		},
		{
			// 兼容路径：直接 curl 打接口 / 老前端没传 expected_bonus
			// 的客户端，没有"用户期待赠送"的语义信号 → 不打扰，即便
			// 此刻 promo 已过期也不返回 409。fulfillment 阶段会以
			// 0 赠送处理，安全已守住。
			name: "no_client_expectation_legacy_path_passes_even_when_expired",
			req: CreateOrderRequest{
				OrderType:           payment.OrderTypeBalance,
				Amount:              200,
				ClientExpectedBonus: 0,
			},
			cfg:        expiredCfg,
			wantReason: "",
		},
		{
			// 订阅订单：本拦截只对 balance 生效。订阅与 promo 完全
			// 解耦，即使误传了 expected_bonus 也不应触发——给前端
			// 兜底防呆。
			name: "subscription_order_short_circuits_even_with_expectation",
			req: CreateOrderRequest{
				OrderType:           payment.OrderTypeSubscription,
				Amount:              200,
				ClientExpectedBonus: 10,
			},
			cfg:        expiredCfg,
			wantReason: "",
		},
		{
			// 活动从未开过 / 被删除（cfg.RechargePromo == nil）但 client
			// 居然还发了 expected_bonus > 0 —— 状态自相矛盾，但语义上
			// "客户端期待 vs 服务端 0"完全成立，仍应拦截让用户确认。
			name: "no_active_promo_but_client_expects_returns_409",
			req: CreateOrderRequest{
				OrderType:           payment.OrderTypeBalance,
				Amount:              200,
				ClientExpectedBonus: 10,
			},
			cfg:        noPromoCfg,
			wantReason: "RECHARGE_PROMO_EXPIRED",
		},
		{
			// 反向防误伤：活动开着、金额没到档 → server bonus = 0、但
			// client 也已经按相同算法算出 expected = 0 → 不传 / 传 0 →
			// 不应拦截。这条用例守住"金额没到档"不会被这个闸门误锁。
			name: "active_promo_amount_below_tier_no_expectation_passes",
			req: CreateOrderRequest{
				OrderType:           payment.OrderTypeBalance,
				Amount:              50, // 低于 100 档
				ClientExpectedBonus: 0,
			},
			cfg:        activeCfg,
			wantReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := checkRechargePromoExpired(tt.req, tt.cfg, fixedNow)

			if tt.wantReason == "" {
				require.NoError(t, err, "expected pass-through, got error")
				return
			}

			require.Error(t, err, "expected gate to fire")
			appErr := new(infraerrors.ApplicationError)
			require.ErrorAs(t, err, &appErr, "expected ApplicationError")
			require.Equal(t, tt.wantReason, appErr.Reason)
			// 409 是与前端约定的状态码，与 reason 是一对一绑定关系；
			// 一并守住，避免被人随手改成 400/422 而前端 modal 不被触发。
			require.Equal(t, int32(409), appErr.Code)
		})
	}
}
