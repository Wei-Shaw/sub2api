package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// CheckoutRedirectPath 是自动提交收银台页面的路径。
const CheckoutRedirectPath = "/api/v1/payment/checkout"

// CheckoutForm 描述一次表单式跳转所需的全部内容。
type CheckoutForm struct {
	OrderID    int64
	OutTradeNo string
	Action     string
	Fields     map[string]string
	ExpiresAt  time.Time
}

// BuildCheckoutRedirectPath 生成带 resume token 的收银台页面地址。
// 用相对路径而不是绝对地址：站点可能挂在任意域名和反向代理之后。
func BuildCheckoutRedirectPath(resumeToken string) string {
	return CheckoutRedirectPath + "?token=" + url.QueryEscape(resumeToken)
}

// GetCheckoutForm 用 resume token 还原一份可提交的收银台表单。
//
// 表单字段是就地重算的，不落库：SePay 的 checkout 初始化只是本地 HMAC 签名，
// 没有任何上游副作用，所以用户刷新或重开支付链接都会得到一份同样有效的表单。
func (s *PaymentService) GetCheckoutForm(ctx context.Context, token string) (*CheckoutForm, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "checkout token is required")
	}
	claims, err := s.paymentResume().ParseToken(token)
	if err != nil {
		return nil, err
	}

	order, err := s.entClient.PaymentOrder.Get(ctx, claims.OrderID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
		}
		return nil, fmt.Errorf("get checkout order: %w", err)
	}
	if claims.UserID > 0 && order.UserID != claims.UserID {
		return nil, invalidResumeTokenMatchError()
	}
	if order.Status != OrderStatusPending {
		return nil, infraerrors.Conflict("ORDER_NOT_PAYABLE", "order is no longer awaiting payment").
			WithMetadata(map[string]string{"status": order.Status})
	}
	if !order.ExpiresAt.IsZero() && time.Now().After(order.ExpiresAt) {
		return nil, infraerrors.Conflict("ORDER_EXPIRED", "order has expired")
	}

	inst, err := s.getOrderProviderInstance(ctx, order)
	if err != nil {
		return nil, fmt.Errorf("load checkout provider instance: %w", err)
	}
	if inst == nil {
		return nil, fmt.Errorf("order %d provider instance is unresolved", order.ID)
	}
	prov, err := s.createProviderFromInstance(ctx, inst)
	if err != nil {
		return nil, err
	}

	instanceConfig, err := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
	if err != nil {
		return nil, fmt.Errorf("load checkout provider config: %w", err)
	}
	currency := paymentProviderConfigCurrency(inst.ProviderKey, instanceConfig)

	returnURL, err := buildPaymentReturnURL(claims.CanonicalReturnURL, order.ID, order.OutTradeNo, token)
	if err != nil {
		return nil, err
	}

	result, err := prov.CreatePayment(ctx, payment.CreatePaymentRequest{
		OrderID:     order.OutTradeNo,
		Amount:      payment.FormatAmountForCurrency(order.PayAmount, currency),
		PaymentType: order.PaymentType,
		Subject:     s.buildCheckoutSubject(ctx, order, currency),
		ReturnURL:   returnURL,
	})
	if err != nil {
		return nil, err
	}
	if result.ResultType != payment.CreatePaymentResultFormPost || strings.TrimSpace(result.FormAction) == "" {
		return nil, fmt.Errorf("order %d provider does not use a form checkout", order.ID)
	}

	return &CheckoutForm{
		OrderID:    order.ID,
		OutTradeNo: order.OutTradeNo,
		Action:     result.FormAction,
		Fields:     result.FormFields,
		ExpiresAt:  order.ExpiresAt,
	}, nil
}

// buildCheckoutSubject 重建订单描述。描述只对账单展示有意义，不参与对账，
// 因此按订单已有信息重算即可，无需与建单那一刻逐字一致。
func (s *PaymentService) buildCheckoutSubject(ctx context.Context, order *dbent.PaymentOrder, currency string) string {
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		cfg = nil
	}
	if order.PlanID != nil && *order.PlanID > 0 {
		if plan, planErr := s.configService.GetPlan(ctx, *order.PlanID); planErr == nil {
			return s.buildPaymentSubject(plan, order.Amount, cfg, nil)
		}
	}
	amountStr := payment.FormatAmountForCurrency(order.PayAmount, currency)
	if hasPaymentProductNameAffix(cfg) {
		return applyPaymentProductNameAffix(amountStr, cfg)
	}
	return "Sub2API " + amountStr + " " + currency
}
