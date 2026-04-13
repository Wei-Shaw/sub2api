package provider

import (
	"bytes"
	"context"
	"crypto/rsa"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/h5"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

// WeChat Pay constants.
const (
	wxpayCurrency = "CNY"
	wxpayH5Type   = "Wap"
)

// WeChat Pay trade states.
const (
	wxpayTradeStateSuccess  = "SUCCESS"
	wxpayTradeStateRefund   = "REFUND"
	wxpayTradeStateClosed   = "CLOSED"
	wxpayTradeStatePayError = "PAYERROR"
)

// WeChat Pay notification event types.
const (
	wxpayEventTransactionSuccess = "TRANSACTION.SUCCESS"
)

// WeChat Pay error codes.
const (
	wxpayErrNoAuth = "NO_AUTH"
)

type Wxpay struct {
	instanceID    string
	config        map[string]string
	mu            sync.Mutex
	coreClient    *core.Client
	notifyHandler *notify.Handler
}

func NewWxpay(instanceID string, config map[string]string) (*Wxpay, error) {
	required := []string{"appId", "mchId", "privateKey", "apiV3Key", "publicKey", "publicKeyId", "certSerial"}
	for _, k := range required {
		if config[k] == "" {
			return nil, fmt.Errorf("wxpay config missing required key: %s", k)
		}
	}
	if len(config["apiV3Key"]) != 32 {
		return nil, fmt.Errorf("wxpay apiV3Key must be exactly 32 bytes, got %d", len(config["apiV3Key"]))
	}
	return &Wxpay{instanceID: instanceID, config: config}, nil
}

func (w *Wxpay) Name() string        { return "Wxpay" }
func (w *Wxpay) ProviderKey() string { return payment.TypeWxpay }
func (w *Wxpay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeWxpay}
}

func formatPEM(key, keyType string) string {
	key = strings.TrimSpace(key)
	if strings.HasPrefix(key, "-----BEGIN") {
		return key
	}
	return fmt.Sprintf("-----BEGIN %s-----\n%s\n-----END %s-----", keyType, key, keyType)
}

func (w *Wxpay) ensureClient() (*core.Client, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.coreClient != nil {
		return w.coreClient, nil
	}
	privateKey, publicKey, err := w.loadKeyPair()
	if err != nil {
		return nil, err
	}
	certSerial := w.config["certSerial"]
	verifier := verifiers.NewSHA256WithRSAPubkeyVerifier(w.config["publicKeyId"], *publicKey)
	client, err := core.NewClient(context.Background(),
		option.WithMerchantCredential(w.config["mchId"], certSerial, privateKey),
		option.WithVerifier(verifier))
	if err != nil {
		return nil, fmt.Errorf("wxpay init client: %w", err)
	}
	handler, err := notify.NewRSANotifyHandler(w.config["apiV3Key"], verifier)
	if err != nil {
		return nil, fmt.Errorf("wxpay init notify handler: %w", err)
	}
	w.notifyHandler = handler
	w.coreClient = client
	return w.coreClient, nil
}

func (w *Wxpay) loadKeyPair() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privateKey, err := utils.LoadPrivateKey(formatPEM(w.config["privateKey"], "PRIVATE KEY"))
	if err != nil {
		return nil, nil, fmt.Errorf("wxpay load private key: %w", err)
	}
	publicKey, err := utils.LoadPublicKey(formatPEM(w.config["publicKey"], "PUBLIC KEY"))
	if err != nil {
		return nil, nil, fmt.Errorf("wxpay load public key: %w", err)
	}
	return privateKey, publicKey, nil
}

func (w *Wxpay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	client, err := w.ensureClient()
	if err != nil {
		return nil, err
	}
	// Request-first, config-fallback (consistent with EasyPay/Alipay)
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = w.config["notifyUrl"]
	}
	if notifyURL == "" {
		return nil, fmt.Errorf("wxpay notifyUrl is required")
	}
	totalFen, err := payment.YuanToFen(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("wxpay create payment: %w", err)
	}
	if req.IsMobile && req.ClientIP != "" {
		resp, err := w.createOrder(ctx, client, req, notifyURL, totalFen, true)
		if err == nil {
			return resp, nil
		}
		if !strings.Contains(err.Error(), wxpayErrNoAuth) {
			return nil, err
		}
		slog.Warn("wxpay H5 payment not authorized, falling back to native", "order", req.OrderID)
	}
	return w.createOrder(ctx, client, req, notifyURL, totalFen, false)
}

func (w *Wxpay) createOrder(ctx context.Context, c *core.Client, req payment.CreatePaymentRequest, notifyURL string, totalFen int64, useH5 bool) (*payment.CreatePaymentResponse, error) {
	if useH5 {
		return w.prepayH5(ctx, c, req, notifyURL, totalFen)
	}
	return w.prepayNative(ctx, c, req, notifyURL, totalFen)
}

func (w *Wxpay) prepayNative(ctx context.Context, c *core.Client, req payment.CreatePaymentRequest, notifyURL string, totalFen int64) (*payment.CreatePaymentResponse, error) {
	svc := native.NativeApiService{Client: c}
	cur := wxpayCurrency
	resp, _, err := svc.Prepay(ctx, native.PrepayRequest{
		Appid: core.String(w.config["appId"]), Mchid: core.String(w.config["mchId"]),
		Description: core.String(req.Subject), OutTradeNo: core.String(req.OrderID),
		NotifyUrl: core.String(notifyURL),
		Amount:    &native.Amount{Total: core.Int64(totalFen), Currency: &cur},
	})
	if err != nil {
		return nil, fmt.Errorf("wxpay native prepay: %w", err)
	}
	codeURL := ""
	if resp.CodeUrl != nil {
		codeURL = *resp.CodeUrl
	}
	return &payment.CreatePaymentResponse{TradeNo: req.OrderID, QRCode: codeURL}, nil
}

func (w *Wxpay) prepayH5(ctx context.Context, c *core.Client, req payment.CreatePaymentRequest, notifyURL string, totalFen int64) (*payment.CreatePaymentResponse, error) {
	svc := h5.H5ApiService{Client: c}
	cur := wxpayCurrency
	tp := wxpayH5Type
	resp, _, err := svc.Prepay(ctx, h5.PrepayRequest{
		Appid: core.String(w.config["appId"]), Mchid: core.String(w.config["mchId"]),
		Description: core.String(req.Subject), OutTradeNo: core.String(req.OrderID),
		NotifyUrl: core.String(notifyURL),
		Amount:    &h5.Amount{Total: core.Int64(totalFen), Currency: &cur},
		SceneInfo: &h5.SceneInfo{PayerClientIp: core.String(req.ClientIP), H5Info: &h5.H5Info{Type: &tp}},
	})
	if err != nil {
		return nil, fmt.Errorf("wxpay h5 prepay: %w", err)
	}
	h5URL := ""
	if resp.H5Url != nil {
		h5URL = *resp.H5Url
	}
	return &payment.CreatePaymentResponse{TradeNo: req.OrderID, PayURL: h5URL}, nil
}

func wxSV(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

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
	var amt float64
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
	return &payment.QueryOrderResponse{TradeNo: id, Status: mapWxState(wxSV(tx.TradeState)), Amount: amt, PaidAt: pa}, nil
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
	var amt float64
	if tx.Amount != nil && tx.Amount.Total != nil {
		amt = payment.FenToYuan(*tx.Amount.Total)
	}
	st := payment.ProviderStatusFailed
	if wxSV(tx.TradeState) == wxpayTradeStateSuccess {
		st = payment.ProviderStatusSuccess
	}
	return &payment.PaymentNotification{
		TradeNo: wxSV(tx.TransactionId), OrderID: wxSV(tx.OutTradeNo),
		Amount: amt, Status: st, RawData: rawBody,
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
