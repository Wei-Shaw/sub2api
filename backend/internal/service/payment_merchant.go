package service

// 商户支付 API（Merchant payment API）
//
// 让同一运营者的外部站点（如机场面板）通过本站的支付通道收款：
// 商户站后端用 HMAC 签名调用创建订单接口 → 拿到支付链接/二维码 → 用户付款 →
// 支付宝回调本站 → 订单进入 PAID → 商户履约（不进任何用户余额，订单直接
// COMPLETED）→ 尽力异步通知商户站 notify_url（商户站同时可用查单接口兜底）。
//
// 商户订单挂在一个专属系统用户下（每个商户一个，按需自动创建），
// order_type = payment.OrderTypeMerchant，商户上下文存于 provider_snapshot。

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	domain "github.com/Wei-Shaw/sub2api/internal/domain"
)

const (
	// merchantOrderTimeoutMin is longer than the in-site default: merchant
	// buyers land on the merchant site and may complete the scan later.
	merchantOrderTimeoutMin = 120
	// merchantMaxPendingOrders caps concurrent PENDING orders per merchant
	// (replaces the per-user site limit, which is far lower).
	merchantMaxPendingOrders = 100
	// merchantEmailSuffix scopes auto-created merchant system users.
	merchantEmailSuffix = "merchant.sub2api.local"
	// merchantNotifyTimeout is the HTTP timeout for merchant callbacks.
	merchantNotifyTimeout = 10 * time.Second
)

// merchantNotifyDelays is the best-effort retry schedule for merchant
// callbacks. Reliability comes from the merchant's polling of the query
// endpoint; these retries are an optimization.
var merchantNotifyDelays = []time.Duration{0, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}

// MerchantCreateOrderRequest is the parsed body of POST /merchant/payment/orders.
type MerchantCreateOrderRequest struct {
	OutOrderID string  `json:"out_order_id"`
	Amount     float64 `json:"amount"`
	Subject    string  `json:"subject"`
	NotifyURL  string  `json:"notify_url"`
	ReturnURL  string  `json:"return_url"`
	IsMobile   bool    `json:"is_mobile"`
	ClientIP   string  `json:"-"`
}

// MerchantOrderResult is the API response for order creation.
type MerchantOrderResult struct {
	OutTradeNo string    `json:"out_trade_no"`
	PayURL     string    `json:"pay_url"`
	QRCode     string    `json:"qr_code"`
	Amount     float64   `json:"amount"`
	Status     string    `json:"status"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// MerchantOrderStatus is the API response for order queries.
type MerchantOrderStatus struct {
	OutTradeNo  string     `json:"out_trade_no"`
	OutOrderID  string     `json:"out_order_id"`
	Status      string     `json:"status"`
	Amount      float64    `json:"amount"`
	PayAmount   float64    `json:"pay_amount"`
	PaidAt      *time.Time `json:"paid_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// merchantOrderSnapshot is the merchant context persisted on each order.
type merchantOrderSnapshot struct {
	MerchantID string `json:"merchant_id"`
	OutOrderID string `json:"out_order_id"`
	NotifyURL  string `json:"notify_url"`
	Subject    string `json:"subject"`
}

func merchantSnapshotOf(o *dbent.PaymentOrder) merchantOrderSnapshot {
	var snap merchantOrderSnapshot
	if o.ProviderSnapshot == nil {
		return snap
	}
	raw, _ := json.Marshal(o.ProviderSnapshot)
	_ = json.Unmarshal(raw, &snap)
	return snap
}

// GenerateMerchantSecret creates a random 32-byte hex secret for a merchant.
func GenerateMerchantSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// VerifyMerchantSignature checks X-Merchant-Id / X-Timestamp / X-Signature
// against the raw request body. Signature = hex(hmac_sha256(secret,
// timestamp + "." + rawBody)); GET requests sign the empty body.
func (s *PaymentService) VerifyMerchantSignature(ctx context.Context, merchantID, timestamp, signature string, body []byte) (*PaymentMerchant, error) {
	merchant, err := s.configService.GetPaymentMerchant(ctx, merchantID)
	if err != nil {
		return nil, err
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil || mathAbs64(time.Now().Unix()-ts) > 300 {
		return nil, infraerrors.Unauthorized("MERCHANT_AUTH_FAILED", "invalid or stale timestamp")
	}
	mac := hmac.New(sha256.New, []byte(merchant.Secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte{'.'})
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(strings.TrimSpace(signature))), []byte(expected)) != 1 {
		return nil, infraerrors.Unauthorized("MERCHANT_AUTH_FAILED", "invalid signature")
	}
	return merchant, nil
}

// SignMerchantPayload produces the X-Signature header value for an outbound
// merchant callback (same scheme as inbound requests).
func SignMerchantPayload(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte{'.'})
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func mathAbs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// getOrCreateMerchantUser returns the dedicated system user for a merchant.
func (s *PaymentService) getOrCreateMerchantUser(ctx context.Context, m *PaymentMerchant) (*User, error) {
	email := m.ID + "@" + merchantEmailSuffix
	u, err := s.userRepo.GetByEmail(ctx, email)
	if err == nil {
		if u.Status != payment.EntityStatusActive {
			return nil, infraerrors.Forbidden("MERCHANT_USER_INACTIVE", "merchant system user is disabled")
		}
		return u, nil
	}
	if !errors.Is(err, ErrUserNotFound) && !dbent.IsNotFound(err) {
		return nil, fmt.Errorf("lookup merchant user: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(GenerateMerchantSecret()), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash merchant user password: %w", err)
	}
	nu := &domain.User{
		Email:        email,
		Username:     "merchant-" + m.ID,
		PasswordHash: string(hash),
		Role:         "user",
		Status:       payment.EntityStatusActive,
	}
	if err := s.userRepo.Create(ctx, nu); err != nil {
		return nil, fmt.Errorf("create merchant user: %w", err)
	}
	slog.Info("merchant system user created", "merchant", m.ID, "user_id", nu.ID)
	return nu, nil
}

// CreateMerchantOrder validates the request, creates a merchant order and
// invokes the payment provider (alipay) for it.
func (s *PaymentService) CreateMerchantOrder(ctx context.Context, m *PaymentMerchant, req MerchantCreateOrderRequest) (*MerchantOrderResult, error) {
	if req.Amount <= 0 || isInvalidAmount(req.Amount) {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount must be a positive number")
	}
	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		subject = "Order " + req.OutOrderID
	}
	if len([]rune(subject)) > 100 {
		subject = string([]rune(subject)[:100])
	}
	notifyURL := strings.TrimSpace(req.NotifyURL)
	if notifyURL == "" {
		notifyURL = strings.TrimSpace(m.NotifyURL)
	}

	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
	}
	if !cfg.Enabled {
		return nil, infraerrors.Forbidden("PAYMENT_DISABLED", "payment system is disabled")
	}

	user, err := s.getOrCreateMerchantUser(ctx, m)
	if err != nil {
		return nil, err
	}

	pending, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.UserIDEQ(user.ID), paymentorder.StatusEQ(OrderStatusPending)).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count pending merchant orders: %w", err)
	}
	if pending >= merchantMaxPendingOrders {
		return nil, infraerrors.TooManyRequests("TOO_MANY_PENDING", "too many pending merchant orders")
	}

	sel, err := s.selectCreateOrderInstance(ctx, CreateOrderRequest{
		UserID:      user.ID,
		PaymentType: payment.TypeAlipay,
		OrderType:   payment.OrderTypeMerchant,
		Amount:      req.Amount,
	}, cfg, req.Amount)
	if err != nil {
		return nil, err
	}

	selectedCurrency := paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	payAmountStr := payment.FormatAmountForCurrency(req.Amount, selectedCurrency)
	if err := validateSelectedCreateOrderAmountCurrency(payAmountStr, sel); err != nil {
		return nil, err
	}

	snap := map[string]any{
		"merchant_id":  m.ID,
		"out_order_id": strings.TrimSpace(req.OutOrderID),
		"notify_url":   notifyURL,
		"subject":      subject,
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	outTradeNo, err := s.allocateOutTradeNo(ctx, tx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	order, err := tx.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(req.Amount).
		SetPayAmount(req.Amount).
		SetFeeRate(0).
		SetRechargeCode("").
		SetOutTradeNo(outTradeNo).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeMerchant).
		SetProviderSnapshot(snap).
		SetStatus(OrderStatusPending).
		SetExpiresAt(now.Add(merchantOrderTimeoutMin * time.Minute)).
		SetClientIP(req.ClientIP).
		SetSrcHost("merchant-api").
		SetProviderInstanceID(strings.TrimSpace(sel.InstanceID)).
		SetProviderKey(strings.TrimSpace(sel.ProviderKey)).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create merchant order: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit merchant order: %w", err)
	}

	prov, err := provider.CreateProvider(sel.ProviderKey, sel.InstanceID, sel.Config)
	if err != nil {
		slog.Error("[MerchantPayment] CreateProvider failed", "provider", sel.ProviderKey, "instance", sel.InstanceID, "error", err)
		s.markMerchantOrderFailed(ctx, order.ID, err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_PROVIDER_MISCONFIGURED", "provider_misconfigured").
			WithMetadata(map[string]string{"provider": sel.ProviderKey, "instance_id": sel.InstanceID})
	}
	pr, err := prov.CreatePayment(ctx, payment.CreatePaymentRequest{
		OrderID:     outTradeNo,
		Amount:      payAmountStr,
		PaymentType: payment.TypeAlipay,
		Subject:     subject,
		ReturnURL:   strings.TrimSpace(req.ReturnURL),
		ClientIP:    req.ClientIP,
		IsMobile:    req.IsMobile,
	})
	if err != nil {
		slog.Error("[MerchantPayment] CreatePayment failed", "provider", sel.ProviderKey, "instance", sel.InstanceID, "error", err)
		s.markMerchantOrderFailed(ctx, order.ID, err)
		if appErr := new(infraerrors.ApplicationError); errors.As(err, &appErr) {
			return nil, appErr
		}
		return nil, classifyCreatePaymentError(CreateOrderRequest{PaymentType: payment.TypeAlipay}, sel.ProviderKey, err)
	}

	_, err = s.entClient.PaymentOrder.UpdateOneID(order.ID).
		SetPaymentTradeNo(strings.TrimSpace(pr.TradeNo)).
		SetNillablePayURL(psNilIfEmpty(pr.PayURL)).
		SetNillableQrCode(psNilIfEmpty(pr.QRCode)).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("persist merchant payment details: %w", err)
	}

	s.writeAuditLog(ctx, order.ID, "MERCHANT_ORDER_CREATED", "merchant:"+m.ID, map[string]any{
		"merchantId":  m.ID,
		"outOrderId":  req.OutOrderID,
		"amount":      req.Amount,
		"outTradeNo":  outTradeNo,
		"providerKey": sel.ProviderKey,
	})

	return &MerchantOrderResult{
		OutTradeNo: outTradeNo,
		PayURL:     pr.PayURL,
		QRCode:     pr.QRCode,
		Amount:     req.Amount,
		Status:     OrderStatusPending,
		ExpiresAt:  order.ExpiresAt,
	}, nil
}

func (s *PaymentService) markMerchantOrderFailed(ctx context.Context, oid int64, cause error) {
	_, uerr := s.entClient.PaymentOrder.UpdateOneID(oid).
		SetStatus(OrderStatusFailed).
		SetFailedAt(time.Now()).
		SetFailedReason(fmt.Sprintf("%.480s", cause.Error())).
		Save(ctx)
	if uerr != nil {
		slog.Warn("[MerchantPayment] mark order failed errored", "orderID", oid, "error", uerr)
	}
}

func isInvalidAmount(v float64) bool {
	return v != v || v > 1_000_000 // NaN / absurd magnitude
}

// GetMerchantOrder returns the status of a merchant order by out_trade_no.
func (s *PaymentService) GetMerchantOrder(ctx context.Context, m *PaymentMerchant, outTradeNo string) (*MerchantOrderStatus, error) {
	user, err := s.getOrCreateMerchantUser(ctx, m)
	if err != nil {
		return nil, err
	}
	o, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.OutTradeNo(strings.TrimSpace(outTradeNo)),
			paymentorder.UserIDEQ(user.ID),
			paymentorder.OrderTypeEQ(payment.OrderTypeMerchant),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("ORDER_NOT_FOUND", "order not found")
		}
		return nil, fmt.Errorf("query merchant order: %w", err)
	}
	snap := merchantSnapshotOf(o)
	return &MerchantOrderStatus{
		OutTradeNo:  o.OutTradeNo,
		OutOrderID:  snap.OutOrderID,
		Status:      o.Status,
		Amount:      o.Amount,
		PayAmount:   o.PayAmount,
		PaidAt:      o.PaidAt,
		CompletedAt: o.CompletedAt,
	}, nil
}

// ExecuteMerchantFulfillment completes a paid merchant order (no balance
// credit) and fires the best-effort merchant callback.
func (s *PaymentService) ExecuteMerchantFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return fmt.Errorf("get merchant order: %w", err)
	}
	if o.OrderType != payment.OrderTypeMerchant {
		return infraerrors.BadRequest("INVALID_STATUS", "not a merchant order")
	}
	if o.Status == OrderStatusCompleted {
		// Idempotent re-entry (e.g. webhook retry after restart): re-notify only.
		go s.dispatchMerchantNotify(oid)
		return nil
	}
	if o.Status != OrderStatusPaid && o.Status != OrderStatusRecharging && o.Status != OrderStatusFailed {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	now := time.Now()
	if _, err := s.entClient.PaymentOrder.UpdateOneID(oid).
		SetStatus(OrderStatusCompleted).
		SetCompletedAt(now).
		ClearFailedAt().
		ClearFailedReason().
		Save(ctx); err != nil {
		return fmt.Errorf("complete merchant order: %w", err)
	}
	snap := merchantSnapshotOf(o)
	s.writeAuditLog(ctx, oid, "MERCHANT_ORDER_COMPLETED", "merchant:"+snap.MerchantID, map[string]any{
		"merchantId": snap.MerchantID,
		"outOrderId": snap.OutOrderID,
		"amount":     o.PayAmount,
	})
	go s.dispatchMerchantNotify(oid)
	return nil
}

// dispatchMerchantNotify POSTs the signed payment.succeeded payload to the
// merchant notify URL with bounded retries. Best effort: merchants must also
// poll the query endpoint.
func (s *PaymentService) dispatchMerchantNotify(oid int64) {
	ctx, cancel := context.WithTimeout(context.Background(), merchantOrderTimeoutMin*time.Minute)
	defer cancel()

	if s.configService == nil {
		return
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil || o.Status != OrderStatusCompleted {
		return
	}
	snap := merchantSnapshotOf(o)
	notifyURL := snap.NotifyURL
	merchant, merr := s.configService.GetPaymentMerchant(ctx, snap.MerchantID)
	if merr != nil {
		slog.Warn("[MerchantPayment] notify skipped: merchant missing", "orderID", oid, "merchant", snap.MerchantID)
		return
	}
	if notifyURL == "" {
		notifyURL = merchant.NotifyURL
	}
	if notifyURL == "" {
		return // merchant relies on polling
	}

	payload, err := json.Marshal(map[string]any{
		"event":        "payment.succeeded",
		"merchant_id":  snap.MerchantID,
		"out_order_id": snap.OutOrderID,
		"out_trade_no": o.OutTradeNo,
		"status":       OrderStatusCompleted,
		"amount":       o.PayAmount,
		"paid_at":      o.PaidAt,
		"completed_at": o.CompletedAt,
		"ts":           time.Now().Unix(),
	})
	if err != nil {
		return
	}

	client := &http.Client{Timeout: merchantNotifyTimeout}
	for _, delay := range merchantNotifyDelays {
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
		}
		ts := fmt.Sprintf("%d", time.Now().Unix())
		r, rerr := http.NewRequestWithContext(ctx, http.MethodPost, notifyURL, strings.NewReader(string(payload)))
		if rerr != nil {
			break
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Merchant-Id", snap.MerchantID)
		r.Header.Set("X-Timestamp", ts)
		r.Header.Set("X-Signature", SignMerchantPayload(merchant.Secret, ts, payload))

		resp, err := client.Do(r)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				s.writeAuditLog(ctx, oid, "MERCHANT_NOTIFY_OK", "merchant:"+snap.MerchantID, map[string]any{
					"url": notifyURL, "httpStatus": resp.StatusCode,
				})
				return
			}
		}
	}
	s.writeAuditLog(ctx, oid, "MERCHANT_NOTIFY_FAILED", "merchant:"+snap.MerchantID, map[string]any{
		"url": notifyURL, "reason": "exhausted retries",
	})
}
