package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
)

func mapWxState(s string) string {
	switch s {
	case wxpayTradeStateSuccess:
		return payment.ProviderStatusPaid
	case wxpayTradeStateRefund:
		return payment.ProviderStatusRefunded
	case wxpayTradeStateClosed, wxpayTradeStatePayError:
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
	}
}

func buildWxpayTransactionMetadata(tx *payments.Transaction) map[string]string {
	if tx == nil {
		return nil
	}

	metadata := map[string]string{}
	if appID := wxSV(tx.Appid); appID != "" {
		metadata[wxpayMetadataAppID] = appID
	}
	if merchantID := wxSV(tx.Mchid); merchantID != "" {
		metadata[wxpayMetadataMerchantID] = merchantID
	}
	if tradeState := wxSV(tx.TradeState); tradeState != "" {
		metadata[wxpayMetadataTradeState] = tradeState
	}
	if tx.Amount != nil {
		if currency := wxSV(tx.Amount.Currency); currency != "" {
			metadata[wxpayMetadataCurrency] = currency
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func (w *Wxpay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	c, err := w.ensureClient()
	if err != nil {
		return nil, err
	}
	svc := native.NativeApiService{Client: c}
	tx, _, err := svc.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(tradeNo), Mchid: core.String(w.config["mchId"]),
	})
	if err != nil {
		return nil, fmt.Errorf("wxpay query order: %w", err)
	}
	amt := decimal.Zero
	if tx.Amount != nil && tx.Amount.Total != nil {
		amt = payment.FenToYuan(*tx.Amount.Total)
	}
	id := tradeNo
	if tx.TransactionId != nil {
		id = *tx.TransactionId
	}
	pa := ""
	if tx.SuccessTime != nil {
		pa = *tx.SuccessTime
	}
	return &payment.QueryOrderResponse{
		TradeNo:  id,
		Status:   mapWxState(wxSV(tx.TradeState)),
		Amount:   amt,
		PaidAt:   pa,
		Metadata: buildWxpayTransactionMetadata(tx),
	}, nil
}

func (w *Wxpay) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if _, err := w.ensureClient(); err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "/", io.NopCloser(bytes.NewBufferString(rawBody)))
	if err != nil {
		return nil, fmt.Errorf("wxpay construct request: %w", err)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	var tx payments.Transaction
	nr, err := w.notifyHandler.ParseNotifyRequest(ctx, r, &tx)
	if err != nil {
		return nil, fmt.Errorf("wxpay verify notification: %w", err)
	}
	if nr.EventType != wxpayEventTransactionSuccess {
		return nil, nil
	}
	amt := decimal.Zero
	if tx.Amount != nil && tx.Amount.Total != nil {
		amt = payment.FenToYuan(*tx.Amount.Total)
	}
	st := payment.ProviderStatusFailed
	if wxSV(tx.TradeState) == wxpayTradeStateSuccess {
		st = payment.ProviderStatusSuccess
	}
	return &payment.PaymentNotification{
		TradeNo: wxSV(tx.TransactionId), OrderID: wxSV(tx.OutTradeNo),
		Amount: amt, Status: st, RawData: rawBody, Metadata: buildWxpayTransactionMetadata(&tx),
	}, nil
}

func (w *Wxpay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	c, err := w.ensureClient()
	if err != nil {
		return nil, err
	}
	rf, err := payment.YuanToFen(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("wxpay refund amount: %w", err)
	}
	tf, err := w.queryOrderTotalFen(ctx, c, req.OrderID)
	if err != nil {
		return nil, err
	}
	rs := refunddomestic.RefundsApiService{Client: c}
	cur := wxpayCurrency
	res, _, err := rs.Create(ctx, refunddomestic.CreateRequest{
		OutTradeNo:  core.String(req.OrderID),
		OutRefundNo: core.String(fmt.Sprintf("%s-refund-%d", req.OrderID, time.Now().UnixNano())),
		Reason:      core.String(req.Reason),
		Amount:      &refunddomestic.AmountReq{Refund: core.Int64(rf), Total: core.Int64(tf), Currency: &cur},
	})
	if err != nil {
		return nil, fmt.Errorf("wxpay refund: %w", err)
	}
	rid := wxSV(res.RefundId)
	if rid == "" {
		rid = fmt.Sprintf("%s-refund", req.OrderID)
	}
	st := payment.ProviderStatusPending
	if res.Status != nil && *res.Status == refunddomestic.STATUS_SUCCESS {
		st = payment.ProviderStatusSuccess
	}
	return &payment.RefundResponse{RefundID: rid, Status: st}, nil
}

func (w *Wxpay) queryOrderTotalFen(ctx context.Context, c *core.Client, orderID string) (int64, error) {
	svc := native.NativeApiService{Client: c}
	tx, _, err := svc.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(orderID), Mchid: core.String(w.config["mchId"]),
	})
	if err != nil {
		return 0, fmt.Errorf("wxpay refund query order: %w", err)
	}
	var tf int64
	if tx.Amount != nil && tx.Amount.Total != nil {
		tf = *tx.Amount.Total
	}
	return tf, nil
}

func (w *Wxpay) CancelPayment(ctx context.Context, tradeNo string) error {
	c, err := w.ensureClient()
	if err != nil {
		return err
	}
	svc := native.NativeApiService{Client: c}
	_, err = svc.CloseOrder(ctx, native.CloseOrderRequest{
		OutTradeNo: core.String(tradeNo), Mchid: core.String(w.config["mchId"]),
	})
	if err != nil {
		return fmt.Errorf("wxpay cancel payment: %w", err)
	}
	return nil
}

var (
	_ payment.Provider           = (*Wxpay)(nil)
	_ payment.CancelableProvider = (*Wxpay)(nil)
)
