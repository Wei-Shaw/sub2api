package service

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	paymentModeManual        = "manual"
	manualQRCodeImgConfigKey = "manualQrCodeImg"
)

func isManualPaymentMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), paymentModeManual)
}

func supportsManualPaymentMode(providerKey string) bool {
	switch strings.TrimSpace(providerKey) {
	case payment.TypeAlipay, payment.TypeWxpay:
		return true
	default:
		return false
	}
}

func isManualPaymentSelection(sel *payment.InstanceSelection) bool {
	return sel != nil && supportsManualPaymentMode(sel.ProviderKey) && isManualPaymentMode(sel.PaymentMode)
}

func manualQRCodeImageFromConfig(config map[string]string) string {
	if len(config) == 0 {
		return ""
	}
	for _, key := range []string{manualQRCodeImgConfigKey, "manual_qr_code_img", "manualQrImage", "qrCodeImg"} {
		if value := strings.TrimSpace(config[key]); value != "" {
			return value
		}
	}
	return ""
}

func buildManualCreatePaymentResponse(sel *payment.InstanceSelection) (*payment.CreatePaymentResponse, error) {
	qrCodeImg := manualQRCodeImageFromConfig(sel.Config)
	if qrCodeImg == "" {
		return nil, infraerrors.ServiceUnavailable(
			"MANUAL_QR_CODE_REQUIRED",
			"manual payment mode requires an uploaded QR code image",
		).WithMetadata(map[string]string{
			"provider":    strings.TrimSpace(sel.ProviderKey),
			"instance_id": strings.TrimSpace(sel.InstanceID),
		})
	}
	return &payment.CreatePaymentResponse{
		QRCode:     qrCodeImg,
		QRCodeImg:  qrCodeImg,
		ResultType: payment.CreatePaymentResultOrderCreated,
	}, nil
}

func paymentOrderIsManualMode(order *dbent.PaymentOrder) bool {
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil && isManualPaymentMode(snapshot.PaymentMode) {
		return true
	}
	return false
}

// AdminCompleteManualOrder marks a pending manual QR order as paid and runs the
// normal fulfillment pipeline. It is intentionally limited to orders whose
// original provider snapshot was created with payment_mode=manual.
func (s *PaymentService) AdminCompleteManualOrder(ctx context.Context, orderID int64, adminUserID int64, tradeNo string, note string) error {
	order, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if !paymentOrderIsManualMode(order) {
		return infraerrors.BadRequest("MANUAL_PAYMENT_REQUIRED", "order is not a manual payment order")
	}
	if order.Status != OrderStatusPending {
		if order.Status == OrderStatusCompleted {
			return nil
		}
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot be manually completed in status "+order.Status)
	}

	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		tradeNo = "manual:" + strings.TrimSpace(order.OutTradeNo)
	}
	paidAmount := order.PayAmount
	if paidAmount <= 0 {
		paidAmount = order.PaymentAmount
	}
	if paidAmount <= 0 {
		paidAmount = order.Amount
	}
	operator := "admin"
	if adminUserID > 0 {
		operator = fmt.Sprintf("admin:%d", adminUserID)
	}
	s.writeAuditLog(ctx, order.ID, "ORDER_MANUAL_CONFIRMED", operator, map[string]any{
		"tradeNo":    tradeNo,
		"paidAmount": paidAmount,
		"note":       strings.TrimSpace(note),
	})
	return s.toPaid(ctx, order, tradeNo, paidAmount, "manual")
}
