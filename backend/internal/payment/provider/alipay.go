package provider

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/smartwalle/alipay/v3"
)

// Alipay product codes.
const (
	alipayProductCodeWapPay  = "QUICK_WAP_WAY"
	alipayProductCodePagePay = "FAST_INSTANT_TRADE_PAY"
)

// Alipay response constants.
const (
	alipayFundChangeYes    = "Y"
	alipayErrTradeNotExist = "ACQ.TRADE_NOT_EXIST"
	alipayRefundSuffix     = "-refund"
)

var (
	alipayTradeWapPay = func(client *alipay.Client, param alipay.TradeWapPay) (*url.URL, error) {
		return client.TradeWapPay(param)
REDACTED
	alipayTradePagePay = func(client *alipay.Client, param alipay.TradePagePay) (*url.URL, error) {
		return client.TradePagePay(param)
REDACTED
)

// Alipay implements payment.Provider and payment.CancelableProvider using the smartwalle/alipay SDK.
type Alipay struct {
	instanceID string
	config     map[string]string // appId, privateKey, publicKey (or alipayPublicKey), notifyUrl, returnUrl

	mu     sync.Mutex
	client *alipay.Client
REDACTED

// NewAlipay creates a new Alipay provider instance.
func NewAlipay(instanceID string, config map[string]string) (*Alipay, error) {
	required := []string{"appId", "privateKey"REDACTED
	for _, k := range required {
		if config[k] == "" {
			return nil, fmt.Errorf("alipay config missing required key: %s", k)
	REDACTED
REDACTED
	return &Alipay{
		instanceID: instanceID,
		config:     config,
REDACTED, nil
REDACTED

func (a *Alipay) getClient() (*alipay.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client != nil {
		return a.client, nil
REDACTED
	client, err := alipay.New(a.config["appId"], a.config["privateKey"], true)
	if err != nil {
		return nil, fmt.Errorf("alipay init client: %w", err)
REDACTED
	pubKey := a.config["publicKey"]
	if pubKey == "" {
		pubKey = a.config["alipayPublicKey"]
REDACTED
	if pubKey == "" {
		return nil, fmt.Errorf("alipay config missing required key: publicKey (or alipayPublicKey)")
REDACTED
	if err := client.LoadAliPayPublicKey(pubKey); err != nil {
		return nil, fmt.Errorf("alipay load public key: %w", err)
REDACTED
	a.client = client
	return a.client, nil
REDACTED

func (a *Alipay) Name() string        { return "Alipay" REDACTED
func (a *Alipay) ProviderKey() string { return payment.TypeAlipay REDACTED
func (a *Alipay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAlipayREDACTED
REDACTED

func (a *Alipay) MerchantIdentityMetadata() map[string]string {
	if a == nil {
		return nil
REDACTED
	appID := strings.TrimSpace(a.config["appId"])
	if appID == "" {
		return nil
REDACTED
	return map[string]string{"app_id": appIDREDACTED
REDACTED

// CreatePayment creates an Alipay payment using redirect-only flow:
//   - Mobile (H5): alipay.trade.wap.pay — returns a URL the browser jumps to.
//   - PC: alipay.trade.page.pay — returns a gateway URL the browser opens in a
//     new window; Alipay's own page then shows login/QR. We intentionally do
//     NOT encode the URL into a QR on the client (it isn't a scannable payload
//     and would produce an invalid scan result).
func (a *Alipay) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	client, err := a.getClient()
	if err != nil {
		return nil, err
REDACTED

	notifyURL := a.config["notifyUrl"]
	if req.NotifyURL != "" {
		notifyURL = req.NotifyURL
REDACTED
	returnURL := a.config["returnUrl"]
	if req.ReturnURL != "" {
		returnURL = req.ReturnURL
REDACTED

	if req.IsMobile {
		return a.createWapTrade(client, req, notifyURL, returnURL)
REDACTED
	return a.createPagePayTrade(client, req, notifyURL, returnURL)
REDACTED

func (a *Alipay) createWapTrade(client *alipay.Client, req payment.CreatePaymentRequest, notifyURL, returnURL string) (*payment.CreatePaymentResponse, error) {
	param := alipay.TradeWapPay{REDACTED
	param.OutTradeNo = req.OrderID
	param.TotalAmount = req.Amount
	param.Subject = req.Subject
	param.ProductCode = alipayProductCodeWapPay
	param.NotifyURL = notifyURL
	param.ReturnURL = returnURL

	payURL, err := alipayTradeWapPay(client, param)
	if err != nil {
		return nil, fmt.Errorf("alipay TradeWapPay: %w", err)
REDACTED
	return &payment.CreatePaymentResponse{
		TradeNo: req.OrderID,
		PayURL:  payURL.String(),
REDACTED, nil
REDACTED

func (a *Alipay) createPagePayTrade(client *alipay.Client, req payment.CreatePaymentRequest, notifyURL, returnURL string) (*payment.CreatePaymentResponse, error) {
	param := alipay.TradePagePay{REDACTED
	param.OutTradeNo = req.OrderID
	param.TotalAmount = req.Amount
	param.Subject = req.Subject
	param.ProductCode = alipayProductCodePagePay
	param.NotifyURL = notifyURL
	param.ReturnURL = returnURL

	payURL, err := alipayTradePagePay(client, param)
	if err != nil {
		return nil, fmt.Errorf("alipay TradePagePay: %w", err)
REDACTED
	return &payment.CreatePaymentResponse{
		TradeNo: req.OrderID,
		PayURL:  payURL.String(),
REDACTED, nil
REDACTED

// QueryOrder queries the trade status via Alipay.
func (a *Alipay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	client, err := a.getClient()
	if err != nil {
		return nil, err
REDACTED

	result, err := client.TradeQuery(ctx, alipay.TradeQuery{OutTradeNo: tradeNoREDACTED)
	if err != nil {
		if isTradeNotExist(err) {
			return &payment.QueryOrderResponse{
				TradeNo: tradeNo,
				Status:  payment.ProviderStatusPending,
		REDACTED, nil
	REDACTED
		return nil, fmt.Errorf("alipay TradeQuery: %w", err)
REDACTED

	status := payment.ProviderStatusPending
	switch result.TradeStatus {
	case alipay.TradeStatusSuccess, alipay.TradeStatusFinished:
		status = payment.ProviderStatusPaid
	case alipay.TradeStatusClosed:
		status = payment.ProviderStatusFailed
REDACTED

	amount, err := strconv.ParseFloat(result.TotalAmount, 64)
	if err != nil {
		return nil, fmt.Errorf("alipay parse amount %q: %w", result.TotalAmount, err)
REDACTED

	return &payment.QueryOrderResponse{
		TradeNo:  result.TradeNo,
		Status:   status,
		Amount:   amount,
		PaidAt:   result.SendPayDate,
		Metadata: a.MerchantIdentityMetadata(),
REDACTED, nil
REDACTED

// VerifyNotification decodes and verifies an Alipay async notification.
func (a *Alipay) VerifyNotification(ctx context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	client, err := a.getClient()
	if err != nil {
		return nil, err
REDACTED

	values, err := url.ParseQuery(rawBody)
	if err != nil {
		return nil, fmt.Errorf("alipay parse notification: %w", err)
REDACTED

	notification, err := client.DecodeNotification(ctx, values)
	if err != nil {
		return nil, fmt.Errorf("alipay verify notification: %w", err)
REDACTED

	status := payment.ProviderStatusFailed
	if notification.TradeStatus == alipay.TradeStatusSuccess || notification.TradeStatus == alipay.TradeStatusFinished {
		status = payment.ProviderStatusSuccess
REDACTED

	amount, err := strconv.ParseFloat(notification.TotalAmount, 64)
	if err != nil {
		return nil, fmt.Errorf("alipay parse notification amount %q: %w", notification.TotalAmount, err)
REDACTED

	metadata := a.MerchantIdentityMetadata()
	if appID := strings.TrimSpace(notification.AppId); appID != "" {
		if metadata == nil {
			metadata = map[string]string{REDACTED
	REDACTED
		metadata["app_id"] = appID
REDACTED

	return &payment.PaymentNotification{
		TradeNo:  notification.TradeNo,
		OrderID:  notification.OutTradeNo,
		Amount:   amount,
		Status:   status,
		RawData:  rawBody,
		Metadata: metadata,
REDACTED, nil
REDACTED

// Refund requests a refund through Alipay.
func (a *Alipay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	client, err := a.getClient()
	if err != nil {
		return nil, err
REDACTED

	result, err := client.TradeRefund(ctx, alipay.TradeRefund{
		OutTradeNo:   req.OrderID,
		RefundAmount: req.Amount,
		RefundReason: req.Reason,
		OutRequestNo: fmt.Sprintf("%s-refund-%d", req.OrderID, time.Now().UnixNano()),
REDACTED)
	if err != nil {
		return nil, fmt.Errorf("alipay TradeRefund: %w", err)
REDACTED

	refundStatus := payment.ProviderStatusPending
	if result.FundChange == alipayFundChangeYes {
		refundStatus = payment.ProviderStatusSuccess
REDACTED

	refundID := result.TradeNo
	if refundID == "" {
		refundID = req.OrderID + alipayRefundSuffix
REDACTED

	return &payment.RefundResponse{
		RefundID: refundID,
		Status:   refundStatus,
REDACTED, nil
REDACTED

// CancelPayment closes a pending trade on Alipay.
func (a *Alipay) CancelPayment(ctx context.Context, tradeNo string) error {
	client, err := a.getClient()
	if err != nil {
		return err
REDACTED

	_, err = client.TradeClose(ctx, alipay.TradeClose{OutTradeNo: tradeNoREDACTED)
	if err != nil {
		if isTradeNotExist(err) {
			return nil
	REDACTED
		return fmt.Errorf("alipay TradeClose: %w", err)
REDACTED
	return nil
REDACTED

func isTradeNotExist(err error) bool {
	if err == nil {
		return false
REDACTED
	return strings.Contains(err.Error(), alipayErrTradeNotExist)
REDACTED

// Ensure interface compliance.
var (
	_ payment.Provider                 = (*Alipay)(nil)
	_ payment.CancelableProvider       = (*Alipay)(nil)
	_ payment.MerchantIdentityProvider = (*Alipay)(nil)
)
