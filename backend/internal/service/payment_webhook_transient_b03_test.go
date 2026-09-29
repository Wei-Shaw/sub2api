//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

var b03SvcErrTransientDB = errors.New("pq: server closed the connection unexpectedly")

func b03SvcFailWhen(armed *atomic.Bool) dbent.Interceptor {
	return dbent.TraverseFunc(func(context.Context, dbent.Query) error {
		if armed.Load() {
			return b03SvcErrTransientDB
		}
		return nil
	})
}

// 订单查询的瞬时错误不能被当成「订单不存在」静默落到 registry 兜底。
func TestB03GetWebhookProvidersSurfacesOrderLookupError(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	var armed atomic.Bool
	client.PaymentOrder.Intercept(b03SvcFailWhen(&armed))
	armed.Store(true)

	svc := &PaymentService{entClient: client, registry: payment.NewRegistry(), providersLoaded: true}
	_, err := svc.GetWebhookProviders(ctx, payment.TypeNowPayments, "sub2_b03_svc")
	require.Error(t, err)
	require.ErrorIs(t, err, b03SvcErrTransientDB)
	require.False(t, errors.Is(err, payment.ErrProviderNotFound))
	require.NotContains(t, strings.ToLower(err.Error()), "ambiguous")
}

// 兜底实例计数失败必须返回原始错误，而不是伪装成 ambiguous。
func TestB03GetWebhookProvidersSurfacesFallbackCountError(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	var armed atomic.Bool
	client.PaymentProviderInstance.Intercept(b03SvcFailWhen(&armed))
	armed.Store(true)

	svc := &PaymentService{entClient: client, registry: payment.NewRegistry(), providersLoaded: true}
	for _, outTradeNo := range []string{"", "sub2_b03_missing"} {
		_, err := svc.GetWebhookProviders(ctx, payment.TypeNowPayments, outTradeNo)
		require.Error(t, err, "outTradeNo=%q", outTradeNo)
		require.ErrorIs(t, err, b03SvcErrTransientDB, "outTradeNo=%q", outTradeNo)
		require.NotContains(t, strings.ToLower(err.Error()), "ambiguous", "outTradeNo=%q", outTradeNo)
	}
}
