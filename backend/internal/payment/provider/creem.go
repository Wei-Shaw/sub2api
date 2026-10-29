package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

const (
	creemProdAPIBase        = "https://api.creem.io"
	creemTestAPIBase        = "https://test-api.creem.io"
	creemTestKeyPrefix      = "creem_test_"
	creemHTTPTimeout        = 15 * time.Second
	creemMaxResponseSize    = 1 << 20
	creemMaxErrorSummary    = 512
	creemWebhookTolerance   = 5 * time.Minute
	creemProductCacheTTL    = 10 * time.Minute
	creemMinCustomPriceCent = 100
	creemMaxCustomPriceCent = 99999999

	creemEventCheckoutCompleted = "checkout.completed"

	creemOrderStatusPaid = "paid"

	creemTransactionStatusRefunded = "refunded"

	creemRefundStatusSucceeded      = "succeeded"
	creemRefundStatusPending        = "pending"
	creemRefundStatusRequiresAction = "requiresaction"
	creemRefundStatusFailed         = "failed"
	creemRefundStatusCanceled       = "canceled"
)

// Creem 是 Merchant of Record 收款服务商。充值金额不固定，所以管理员在 Creem 后台建一个
// 一次性（onetime）商品，每笔订单用 custom_price 覆盖单价，用户跳转到 Creem 托管收银台付款。
type Creem struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

type creemProductCacheEntry struct {
	product   creemProduct
	expiresAt time.Time
}

var creemProductCache sync.Map

// 退款发起后复查交易状态的次数和间隔；测试里调小。
var (
	creemRefundRecheckAttempts = 3
	creemRefundRecheckInterval = time.Second
)

func NewCreem(instanceID string, config map[string]string) (*Creem, error) {
	for _, k := range []string{"apiKey", "webhookSecret", "productId"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("creem config missing required key: %s", k)
		}
	}
	cfg := cloneStringMap(config)
	cfg["apiKey"] = strings.TrimSpace(cfg["apiKey"])
	cfg["productId"] = strings.TrimSpace(cfg["productId"])
	apiBase, err := normalizeCreemAPIBase(cfg["apiBase"], cfg["apiKey"])
	if err != nil {
		return nil, err
	}
	cfg["apiBase"] = apiBase
	currency, err := payment.NormalizeCreemCurrency(cfg["currency"])
	if err != nil {
		return nil, fmt.Errorf("creem config currency: %w", err)
	}
	cfg["currency"] = currency
	return &Creem{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: creemHTTPTimeout},
	}, nil
}

// normalizeCreemAPIBase 未填写时按 API Key 前缀自动选择环境：creem_test_ 走测试环境。
func normalizeCreemAPIBase(raw, apiKey string) (string, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		if strings.HasPrefix(apiKey, creemTestKeyPrefix) {
			return creemTestAPIBase, nil
		}
		return creemProdAPIBase, nil
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("creem apiBase must be an HTTPS URL")
	}
	host := strings.ToLower(parsed.Host)
	if host != "api.creem.io" && host != "test-api.creem.io" {
		return "", fmt.Errorf("creem apiBase host must be api.creem.io or test-api.creem.io")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path != "" && path != "/v1" {
		return "", fmt.Errorf("creem apiBase path must be empty or /v1")
	}
	isTestKey := strings.HasPrefix(apiKey, creemTestKeyPrefix)
	if isTestKey != (host == "test-api.creem.io") {
		return "", fmt.Errorf("creem apiBase does not match the API key environment: test keys (creem_test_) use https://test-api.creem.io, live keys use https://api.creem.io")
	}
	return "https://" + host, nil
}

func (c *Creem) Name() string        { return "Creem" }
func (c *Creem) ProviderKey() string { return payment.TypeCreem }
func (c *Creem) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeCreem}
}

// RefundsLatePayments 返回 true：Creem 没有作废 checkout 的接口，订单过期后链接仍能付款，
// 这种付款由服务层自动全额退回。
func (c *Creem) RefundsLatePayments() bool { return true }

func (c *Creem) MerchantIdentityMetadata() map[string]string {
	if c == nil {
		return nil
	}
	return map[string]string{"currency": c.currency()}
}

func (c *Creem) currency() string {
	if c == nil {
		return payment.CreemDefaultCurrency
	}
	currency, err := payment.NormalizeCreemCurrency(c.config["currency"])
	if err != nil {
		return payment.CreemDefaultCurrency
	}
	return currency
}

func (c *Creem) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	cents, err := creemAmountToCents(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("creem create payment: %w", err)
	}
	if cents < creemMinCustomPriceCent || cents > creemMaxCustomPriceCent {
		return nil, fmt.Errorf("creem create payment: amount %s is outside Creem's custom price range (1.00 - 999999.99)", req.Amount)
	}
	if strings.TrimSpace(req.OrderID) == "" {
		return nil, fmt.Errorf("creem create payment: missing order id")
	}
	if _, err := c.product(ctx); err != nil {
		return nil, fmt.Errorf("creem create payment: %w", err)
	}

	payload := creemCreateCheckoutRequest{
		RequestID:   req.OrderID,
		ProductID:   c.config["productId"],
		Units:       1,
		CustomPrice: cents,
		SuccessURL:  creemSuccessURL(req.ReturnURL),
		Metadata:    map[string]string{"order_id": req.OrderID},
	}
	var checkout creemCheckout
	if err := c.doJSON(ctx, http.MethodPost, "/v1/checkouts", payload, &checkout); err != nil {
		return nil, fmt.Errorf("creem create payment: %w", err)
	}
	if strings.TrimSpace(checkout.ID) == "" || strings.TrimSpace(checkout.CheckoutURL) == "" {
		return nil, fmt.Errorf("creem create payment: missing checkout id or checkout_url")
	}
	return &payment.CreatePaymentResponse{
		TradeNo:  checkout.ID,
		PayURL:   checkout.CheckoutURL,
		Currency: c.currency(),
	}, nil
}

// product 读取并校验配置的商品：必须是启用中的一次性商品、币种与实例一致、价格含税。
// 价外税（exclusive）会让买家实付高于订单金额，履约时的金额核对会失败，所以提前拒绝。
func (c *Creem) product(ctx context.Context) (creemProduct, error) {
	cacheKey := c.productCacheKey()
	if raw, ok := creemProductCache.Load(cacheKey); ok {
		if entry, ok := raw.(creemProductCacheEntry); ok && time.Now().Before(entry.expiresAt) {
			return entry.product, nil
		}
	}

	var product creemProduct
	if err := c.doJSON(ctx, http.MethodGet, "/v1/products/"+url.PathEscape(c.config["productId"]), nil, &product); err != nil {
		return creemProduct{}, fmt.Errorf("load product %s: %w", c.config["productId"], err)
	}
	if !strings.EqualFold(product.Status, "active") {
		return creemProduct{}, fmt.Errorf("product %s is not active (status %s)", product.ID, product.Status)
	}
	if !strings.EqualFold(product.BillingType, "onetime") {
		return creemProduct{}, fmt.Errorf("product %s must be a one-time product, got billing_type %s", product.ID, product.BillingType)
	}
	if !strings.EqualFold(product.Currency, c.currency()) {
		return creemProduct{}, fmt.Errorf("product %s currency %s does not match configured currency %s", product.ID, strings.ToUpper(product.Currency), c.currency())
	}
	if strings.EqualFold(product.TaxMode, "exclusive") {
		return creemProduct{}, fmt.Errorf("product %s uses tax_mode exclusive; set it to inclusive so the buyer pays exactly the order amount", product.ID)
	}
	creemProductCache.Store(cacheKey, creemProductCacheEntry{product: product, expiresAt: time.Now().Add(creemProductCacheTTL)})
	return product, nil
}

func (c *Creem) productCacheKey() string {
	sum := sha256.Sum256([]byte(c.config["apiKey"]))
	return c.config["apiBase"] + "|" + c.config["productId"] + "|" + c.currency() + "|" + hex.EncodeToString(sum[:8])
}

func (c *Creem) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	checkout, err := c.getCheckout(ctx, tradeNo)
	if err != nil {
		return nil, fmt.Errorf("creem query order: %w", err)
	}
	resp := &payment.QueryOrderResponse{
		TradeNo:  checkout.ID,
		Status:   payment.ProviderStatusPending,
		Metadata: c.checkoutMetadata(checkout),
	}
	if order := checkout.Order; order != nil {
		resp.Amount = order.paidAmount()
		if strings.EqualFold(order.Status, creemOrderStatusPaid) {
			resp.Status = payment.ProviderStatusPaid
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(order.UpdatedAt)); err == nil {
				resp.PaidAt = t.Format(time.RFC3339)
			}
		}
	}
	return resp, nil
}

func (c *Creem) getCheckout(ctx context.Context, checkoutID string) (*creemCheckout, error) {
	checkoutID = strings.TrimSpace(checkoutID)
	if checkoutID == "" {
		return nil, fmt.Errorf("missing checkout id")
	}
	var checkout creemCheckout
	if err := c.doJSON(ctx, http.MethodGet, "/v1/checkouts?checkout_id="+url.QueryEscape(checkoutID), nil, &checkout); err != nil {
		return nil, err
	}
	if strings.TrimSpace(checkout.ID) == "" {
		return nil, fmt.Errorf("checkout %s not found", checkoutID)
	}
	return &checkout, nil
}

func (c *Creem) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if err := verifyCreemWebhookSignature(rawBody, headers, c.config["webhookSecret"], time.Now()); err != nil {
		return nil, err
	}

	var event creemWebhookEvent
	if err := json.Unmarshal([]byte(rawBody), &event); err != nil {
		return nil, fmt.Errorf("creem parse webhook: %w", err)
	}
	if event.eventType() != creemEventCheckoutCompleted {
		return nil, nil
	}

	var checkout creemCheckout
	if err := json.Unmarshal(event.object(), &checkout); err != nil {
		return nil, fmt.Errorf("creem parse checkout: %w", err)
	}
	orderID := checkout.merchantOrderID()
	if strings.TrimSpace(checkout.ID) == "" || orderID == "" {
		return nil, fmt.Errorf("creem webhook missing checkout id or request_id")
	}
	if checkout.Order == nil {
		return nil, fmt.Errorf("creem checkout.completed webhook missing order")
	}
	status := payment.ProviderStatusFailed
	if strings.EqualFold(checkout.Order.Status, creemOrderStatusPaid) {
		status = payment.NotificationStatusSuccess
	}

	return &payment.PaymentNotification{
		TradeNo:  checkout.ID,
		OrderID:  orderID,
		Amount:   checkout.Order.paidAmount(),
		Status:   status,
		RawData:  rawBody,
		Metadata: c.checkoutMetadata(&checkout),
	}, nil
}

// Refund 只支持全额退款：Creem 的退款接口只接收 transaction_id，没有金额参数。
func (c *Creem) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	checkout, err := c.getCheckout(ctx, req.TradeNo)
	if err != nil {
		return nil, fmt.Errorf("creem refund: %w", err)
	}
	if checkout.Order == nil || strings.TrimSpace(checkout.Order.Transaction) == "" {
		return nil, fmt.Errorf("creem refund: checkout %s has no paid transaction", checkout.ID)
	}
	requested, err := creemAmountToCents(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("creem refund: %w", err)
	}
	if paid := checkout.Order.paidCents(); requested != paid {
		return nil, fmt.Errorf("creem refund: only full refunds are supported (requested %s, paid %s)",
			decimal.NewFromInt(requested).Shift(-2).StringFixed(2), decimal.NewFromInt(paid).Shift(-2).StringFixed(2))
	}

	transactionID := checkout.Order.Transaction

	// 幂等：Creem 对已全额退款的交易再发退款会返回 400（no remaining amount）。上一次退款若在
	// Creem 侧已成功、本地却因超时等原因回滚，重试时必须识别出来，而不是再失败一次。
	var transaction creemTransaction
	if err := c.doJSON(ctx, http.MethodGet, "/v1/transactions?transaction_id="+url.QueryEscape(transactionID), nil, &transaction); err != nil {
		return nil, fmt.Errorf("creem refund: load transaction %s: %w", transactionID, err)
	}
	if strings.EqualFold(transaction.Status, creemTransactionStatusRefunded) {
		return &payment.RefundResponse{RefundID: transactionID, Status: payment.ProviderStatusSuccess}, nil
	}

	// 退款接口的响应只有 status 一个字段，没有退款单 id，用交易 id 作为退款标识。
	var refund creemRefund
	payload := creemCreateRefundRequest{TransactionID: transactionID}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/refunds", payload, &refund); err != nil {
		return nil, fmt.Errorf("creem refund: %w", err)
	}
	status := creemRefundProviderStatus(refund.Status)
	if status == payment.ProviderStatusPending && c.waitTransactionRefunded(ctx, transactionID) {
		status = payment.ProviderStatusSuccess
	}
	return &payment.RefundResponse{
		RefundID: transactionID,
		Status:   status,
	}, nil
}

// waitTransactionRefunded 在发起退款后短暂复查交易状态。Creem 的退款接口总是先回 pending，
// 交易通常几秒内变成 refunded（实测 < 6s）；复查几次可避免每笔退款都停在 REFUND_PENDING
// 等管理员手动查询。复查失败不影响已发起的退款，订单仍可通过 QueryRefund 收敛。
func (c *Creem) waitTransactionRefunded(ctx context.Context, transactionID string) bool {
	for i := 0; i < creemRefundRecheckAttempts; i++ {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(creemRefundRecheckInterval):
		}
		var tx creemTransaction
		if err := c.doJSON(ctx, http.MethodGet, "/v1/transactions?transaction_id="+url.QueryEscape(transactionID), nil, &tx); err == nil &&
			strings.EqualFold(tx.Status, creemTransactionStatusRefunded) {
			return true
		}
	}
	return false
}

// QueryRefund 让 REFUND_PENDING 订单收敛：Creem 退款没有退款单 id，按订单交易的状态判断，
// 交易变为 refunded 即退款完成，其余状态仍视为处理中。
func (c *Creem) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	checkout, err := c.getCheckout(ctx, req.TradeNo)
	if err != nil {
		return nil, fmt.Errorf("creem query refund: %w", err)
	}
	if checkout.Order == nil || strings.TrimSpace(checkout.Order.Transaction) == "" {
		return nil, fmt.Errorf("creem query refund: checkout %s has no paid transaction", checkout.ID)
	}
	transactionID := checkout.Order.Transaction
	var transaction creemTransaction
	if err := c.doJSON(ctx, http.MethodGet, "/v1/transactions?transaction_id="+url.QueryEscape(transactionID), nil, &transaction); err != nil {
		return nil, fmt.Errorf("creem query refund: load transaction %s: %w", transactionID, err)
	}
	status := payment.ProviderStatusPending
	if strings.EqualFold(transaction.Status, creemTransactionStatusRefunded) {
		status = payment.ProviderStatusSuccess
	}
	return &payment.RefundResponse{RefundID: transactionID, Status: status}, nil
}

func (c *Creem) checkoutMetadata(checkout *creemCheckout) map[string]string {
	metadata := map[string]string{}
	if checkout == nil {
		return metadata
	}
	if productID := checkout.productID(); productID != "" {
		metadata["product_id"] = productID
	}
	if order := checkout.Order; order != nil {
		metadata["currency"] = strings.ToUpper(strings.TrimSpace(order.Currency))
		metadata["status"] = strings.ToLower(strings.TrimSpace(order.Status))
		if order.ID != "" {
			metadata["order_id"] = order.ID
		}
		if order.Transaction != "" {
			metadata["transaction_id"] = order.Transaction
		}
	}
	return metadata
}

func (c *Creem) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var bodyReader io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.config["apiBase"]+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("x-api-key", c.config["apiKey"])

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: creemHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, creemMaxResponseSize))
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("HTTP %d: %s; Creem rejected the API key, check that it matches the environment (creem_test_ keys use test mode)", resp.StatusCode, summarizeCreemResponse(body))
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, summarizeCreemResponse(body))
	}
	if out == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

func creemRefundProviderStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case creemRefundStatusSucceeded:
		return payment.ProviderStatusSuccess
	case creemRefundStatusFailed, creemRefundStatusCanceled:
		return payment.ProviderStatusFailed
	case creemRefundStatusPending, creemRefundStatusRequiresAction:
		return payment.ProviderStatusPending
	default:
		return payment.ProviderStatusPending
	}
}

// creemSuccessURL 过滤 Creem 不接受的回跳地址：实测 Creem 会以 "URL must be valid" 拒绝主机名是
// IP 的地址（如 http://127.0.0.1:8080/...），整笔下单随之失败。这类地址不传，付款完成后用户停留在
// Creem 的完成页，订单仍由 webhook 入账。
func creemSuccessURL(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || net.ParseIP(parsed.Hostname()) != nil {
		return ""
	}
	return raw
}

func creemAmountToCents(raw string) (int64, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return 0, fmt.Errorf("invalid amount %s", raw)
	}
	cents := amount.Shift(2)
	if !cents.Equal(cents.Truncate(0)) {
		return 0, fmt.Errorf("amount %s has more than 2 decimal places", raw)
	}
	return cents.IntPart(), nil
}

// verifyCreemWebhookSignature 与官方 SDK 的 verifyWebhookSignature 一致：优先校验 Standard Webhooks
// 头（webhook-id / webhook-timestamp / webhook-signature），没有这些头时回落到旧版 creem-signature
// （原始请求体的 HMAC-SHA256 十六进制摘要）。
func verifyCreemWebhookSignature(rawBody string, headers map[string]string, secret string, now time.Time) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return fmt.Errorf("creem webhookSecret not configured")
	}
	if now.IsZero() {
		now = time.Now()
	}

	id := strings.TrimSpace(headers["webhook-id"])
	timestamp := strings.TrimSpace(headers["webhook-timestamp"])
	signatures := strings.TrimSpace(headers["webhook-signature"])
	if id != "" && timestamp != "" && signatures != "" {
		return verifyCreemStandardWebhook(rawBody, id, timestamp, signatures, secret, now)
	}

	signature := strings.TrimSpace(headers["creem-signature"])
	if signature == "" {
		signature = strings.TrimSpace(headers["x-creem-signature"])
	}
	if signature == "" {
		return fmt.Errorf("creem notification missing creem-signature header")
	}
	signature = strings.ToLower(strings.TrimPrefix(signature, "sha256="))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(rawBody))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("creem invalid signature")
	}
	return nil
}

func verifyCreemStandardWebhook(rawBody, id, timestamp, signatures, secret string, now time.Time) error {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || seconds <= 0 {
		return fmt.Errorf("creem invalid webhook timestamp")
	}
	if diff := now.Sub(time.Unix(seconds, 0)).Abs(); diff > creemWebhookTolerance {
		return fmt.Errorf("creem webhook timestamp outside tolerance")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return fmt.Errorf("creem webhookSecret is not a valid Standard Webhooks secret: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(id + "." + timestamp + "." + rawBody))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	for _, versioned := range strings.Fields(signatures) {
		version, signature, ok := strings.Cut(versioned, ",")
		if ok && version == "v1" && hmac.Equal([]byte(signature), []byte(expected)) {
			return nil
		}
	}
	return fmt.Errorf("creem invalid signature")
}

func summarizeCreemResponse(body []byte) string {
	summary := strings.Join(strings.Fields(string(body)), " ")
	if summary == "" {
		return "<empty>"
	}
	if len(summary) > creemMaxErrorSummary {
		return summary[:creemMaxErrorSummary] + "..."
	}
	return summary
}

type creemCreateCheckoutRequest struct {
	RequestID   string            `json:"request_id"`
	ProductID   string            `json:"product_id"`
	Units       int               `json:"units"`
	CustomPrice int64             `json:"custom_price"`
	SuccessURL  string            `json:"success_url,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type creemCreateRefundRequest struct {
	TransactionID string `json:"transaction_id"`
}

type creemProduct struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	BillingType string `json:"billing_type"`
	Currency    string `json:"currency"`
	TaxMode     string `json:"tax_mode"`
}

type creemCheckout struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	RequestID   string          `json:"request_id"`
	Product     json.RawMessage `json:"product"`
	CheckoutURL string          `json:"checkout_url"`
	Order       *creemOrder     `json:"order"`
	Metadata    map[string]any  `json:"metadata"`
}

// merchantOrderID 返回下单时写入的 out_trade_no：request_id 是主来源，metadata.order_id 兜底。
func (c *creemCheckout) merchantOrderID() string {
	if id := strings.TrimSpace(c.RequestID); id != "" {
		return id
	}
	if raw, ok := c.Metadata["order_id"].(string); ok {
		return strings.TrimSpace(raw)
	}
	return ""
}

// productID 兼容 product 字段为裸 id 或展开对象两种形状。
func (c *creemCheckout) productID() string {
	return creemRelatedID(c.Product)
}

func creemRelatedID(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var id string
	if err := json.Unmarshal(raw, &id); err == nil {
		return strings.TrimSpace(id)
	}
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return strings.TrimSpace(obj.ID)
	}
	return ""
}

type creemOrder struct {
	ID          string `json:"id"`
	Transaction string `json:"transaction"`
	Amount      int64  `json:"amount"`
	AmountPaid  *int64 `json:"amount_paid"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
	UpdatedAt   string `json:"updated_at"`
}

// paidCents 优先取实付金额：含税价商品的实付等于标价，用了折扣码则会低于订单金额，
// 让履约时的金额核对拦下来，而不是按原价入账。
func (o *creemOrder) paidCents() int64 {
	if o.AmountPaid != nil && *o.AmountPaid > 0 {
		return *o.AmountPaid
	}
	return o.Amount
}

func (o *creemOrder) paidAmount() float64 {
	return decimal.NewFromInt(o.paidCents()).Shift(-2).InexactFloat64()
}

type creemRefund struct {
	Status string `json:"status"`
}

type creemTransaction struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// creemWebhookEvent 兼容两种事件信封：{eventType, object} 和 {type, data}。
type creemWebhookEvent struct {
	ID        string          `json:"id"`
	EventType string          `json:"eventType"`
	Type      string          `json:"type"`
	Object    json.RawMessage `json:"object"`
	Data      json.RawMessage `json:"data"`
}

func (e creemWebhookEvent) eventType() string {
	if t := strings.TrimSpace(e.EventType); t != "" {
		return t
	}
	return strings.TrimSpace(e.Type)
}

func (e creemWebhookEvent) object() json.RawMessage {
	if len(bytes.TrimSpace(e.Object)) > 0 {
		return e.Object
	}
	return e.Data
}

var (
	_ payment.Provider                 = (*Creem)(nil)
	_ payment.MerchantIdentityProvider = (*Creem)(nil)
	_ payment.LatePaymentRefunder      = (*Creem)(nil)
	_ payment.RefundQueryProvider      = (*Creem)(nil)
)
