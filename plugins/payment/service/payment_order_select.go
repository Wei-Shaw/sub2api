package service

import (
	"context"

	"github.com/shopspring/decimal"

	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// selectCreateOrderInstance picks the provider instance the order will be
// routed to. The caller has already passed validation; this layer only
// translates an empty selection into a 503 / 429 ApplicationError.
//
// The plugin does not yet support the WeChat OAuth flow (the official MP
// app credential lives in core settings, not in plugin SDK reach), so we
// route every supported method through the same load-balancer call. If a
// caller hits the in-WeChat path with a JSAPI provider, the load balancer
// will simply return whatever instance matches.
func (s *PaymentService) selectCreateOrderInstance(ctx context.Context, req CreateOrderRequest, cfg *PaymentConfig, payAmount decimal.Decimal) (*payment.InstanceSelection, error) {
	sel, err := s.loadBalancer.SelectInstance(ctx, "", req.PaymentType, payment.Strategy(cfg.LoadBalanceStrategy), payAmount)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "method_not_configured").
			WithMetadata(map[string]string{"payment_type": req.PaymentType})
	}
	if sel == nil {
		return nil, infraerrors.TooManyRequests("NO_AVAILABLE_INSTANCE", "no_available_instance")
	}
	return sel, nil
}
