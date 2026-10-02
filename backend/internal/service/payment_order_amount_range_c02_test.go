//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type c02SettingRepoStub struct {
	SettingRepository
	values map[string]string
}

func (s *c02SettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := s.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

type c02UserRepoStub struct {
	UserRepository
}

func (s *c02UserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	return &User{ID: id, Email: "c02@example.com", Status: payment.EntityStatusActive}, nil
}

// c02LoadBalancerStub 记录选实例时拿到的网关金额，然后直接报错，让 CreateOrder 在建单前停下。
type c02LoadBalancerStub struct {
	called    bool
	payAmount float64
}

func (s *c02LoadBalancerStub) GetInstanceConfig(context.Context, int64) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func (s *c02LoadBalancerStub) SelectInstance(_ context.Context, _ string, _ payment.PaymentType, _ payment.Strategy, orderAmount float64) (*payment.InstanceSelection, error) {
	s.called = true
	s.payAmount = orderAmount
	return nil, errors.New("c02: stop before creating the order")
}

func c02NewPaymentService(lb payment.LoadBalancer) *PaymentService {
	settings := &c02SettingRepoStub{values: map[string]string{
		SettingPaymentEnabled: "true",
		// 管理员按网关币种（VND）配置的单笔上下限。
		SettingMinRechargeAmount: "10000",
		SettingMaxRechargeAmount: "50000000",
	}}
	svc := &PaymentService{
		configService: NewPaymentConfigService(nil, settings, nil),
		userRepo:      &c02UserRepoStub{},
		loadBalancer:  lb,
	}
	rateSvc := NewExchangeRateService(nil)
	rateSvc.cached = &ExchangeRateSnapshot{Rate: decimal.RequireFromString("26260"), FetchedAt: time.Now()}
	svc.SetExchangeRateService(rateSvc)
	return svc
}

func TestC02CreateOrderAmountRangeUsesGatewayCurrency(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		amount      float64
		currency    string
		wantInRange bool
		wantCharge  float64
	}{
		// $10 = 262,600 VND，在 [10,000, 50,000,000] 内，不应被 min=10000 挡住。
		{name: "usd within gateway range", amount: 10, currency: "USD", wantInRange: true, wantCharge: 262600},
		// $100,000 = 2,626,000,000 VND，超过 max，必须拦下。
		{name: "usd above gateway max", amount: 100000, currency: "USD", wantInRange: false},
		// 按 VND 填写的行为不变。
		{name: "vnd below min", amount: 5000, currency: "VND", wantInRange: false},
		{name: "vnd within range", amount: 262600, currency: "VND", wantInRange: true, wantCharge: 262600},
		{name: "empty currency treated as gateway", amount: 5000, currency: "", wantInRange: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lb := &c02LoadBalancerStub{}
			svc := c02NewPaymentService(lb)

			_, err := svc.CreateOrder(context.Background(), CreateOrderRequest{
				UserID:         1,
				Amount:         tc.amount,
				AmountCurrency: tc.currency,
				PaymentType:    payment.TypeGPMPay,
				OrderType:      payment.OrderTypeBalance,
			})
			require.Error(t, err)
			if tc.wantInRange {
				require.NotEqual(t, "INVALID_AMOUNT", infraerrors.Reason(err), "err=%v", err)
				require.True(t, lb.called, "in-range order should reach instance selection")
				require.InDelta(t, tc.wantCharge, lb.payAmount, 0.001)
				return
			}
			require.Equal(t, "INVALID_AMOUNT", infraerrors.Reason(err), "err=%v", err)
			require.False(t, lb.called, "out-of-range order must be rejected before instance selection")
		})
	}
}
