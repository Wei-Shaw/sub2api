// Package provider contains concrete payment provider implementations.
package provider

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// EasyPay constants.
const (
	easypayCodeSuccess     = 1
	easypayStatusPaid      = 1
	easypayHTTPTimeout     = 10 * time.Second
	maxEasypayResponseSize = 1 << 20 // 1MB
	tradeStatusSuccess     = "TRADE_SUCCESS"
	signTypeMD5            = "MD5"
)

// EasyPay implements payment.Provider for the EasyPay aggregation platform.
type EasyPay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
REDACTED

// NewEasyPay creates a new EasyPay provider.
// config keys: pid, pkey, apiBase, notifyUrl, returnUrl, cid, cidAlipay, cidWxpay
func NewEasyPay(instanceID string, config map[string]string) (*EasyPay, error) {
	for _, k := range []string{"pid", "pkey", "apiBase", "notifyUrl", "returnUrl"REDACTED {
		if config[k] == "" {
			return nil, fmt.Errorf("easypay config missing required key: %s", k)
	REDACTED
REDACTED
	return &EasyPay{
		instanceID: instanceID,
		config:     config,
		httpClient: &http.Client{Timeout: easypayHTTPTimeoutREDACTED,
REDACTED, nil
REDACTED

func (e *EasyPay) Name() string        { return "EasyPay" REDACTED
func (e *EasyPay) ProviderKey() string { return payment.TypeEasyPay REDACTED
func (e *EasyPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAlipay, payment.TypeWxpayREDACTED
REDACTED

func (e *EasyPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	// Payment mode determined by instance config, not payment type.
	// "popup" → hosted page (submit.php); "qrcode"/default → API call (mapi.php).
	mode := e.config["paymentMode"]
	if mode == "popup" {
		return e.createRedirectPayment(req)
REDACTED
	return e.createAPIPayment(ctx, req)
REDACTED

// createRedirectPayment builds a submit.php URL for browser redirect.
// No server-side API call — the user is redirected to EasyPay's hosted page.
// TradeNo is empty; it arrives via the notify callback after payment.
func (e *EasyPay) createRedirectPayment(req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL, returnURL := e.resolveURLs(req)
	params := map[string]string{
		"pid": e.config["pid"], "type": req.PaymentType,
		"out_trade_no": req.OrderID, "notify_url": notifyURL,
		"return_url": returnURL, "name": req.Subject,
		"money": req.Amount,
REDACTED
	if cid := e.resolveCID(req.PaymentType); cid != "" {
		params["cid"] = cid
REDACTED
	params["sign"] = easyPaySign(params, e.config["pkey"])
	params["sign_type"] = signTypeMD5

	q := url.Values{REDACTED
	for k, v := range params {
		q.Set(k, v)
REDACTED
	base := strings.TrimRight(e.config["apiBase"], "/")
	payURL := base + "/submit.php?" + q.Encode()
	return &payment.CreatePaymentResponse{PayURL: payURLREDACTED, nil
REDACTED

// createAPIPayment calls mapi.php to get payurl/qrcode (existing behavior).
func (e *EasyPay) createAPIPayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL, returnURL := e.resolveURLs(req)
	params := map[string]string{
		"pid": e.config["pid"], "type": req.PaymentType,
		"out_trade_no": req.OrderID, "notify_url": notifyURL,
		"return_url": returnURL, "name": req.Subject,
		"money": req.Amount, "clientip": req.ClientIP,
REDACTED
	if cid := e.resolveCID(req.PaymentType); cid != "" {
		params["cid"] = cid
REDACTED
	if req.IsMobile {
		params["device"] = "mobile"
REDACTED
	params["sign"] = easyPaySign(params, e.config["pkey"])
	params["sign_type"] = signTypeMD5

	body, err := e.post(ctx, strings.TrimRight(e.config["apiBase"], "/")+"/mapi.php", params)
	if err != nil {
		return nil, fmt.Errorf("easypay create: %w", err)
REDACTED
	var resp struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		TradeNo string `json:"trade_no"`
		PayURL  string `json:"payurl"`
		QRCode  string `json:"qrcode"`
REDACTED
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("easypay parse: %w", err)
REDACTED
	if resp.Code != easypayCodeSuccess {
		return nil, fmt.Errorf("easypay error: %s", resp.Msg)
REDACTED
	return &payment.CreatePaymentResponse{TradeNo: resp.TradeNo, PayURL: resp.PayURL, QRCode: resp.QRCodeREDACTED, nil
REDACTED

// resolveURLs returns (notifyURL, returnURL) preferring request values,
// falling back to instance config.
func (e *EasyPay) resolveURLs(req payment.CreatePaymentRequest) (string, string) {
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = e.config["notifyUrl"]
REDACTED
	returnURL := req.ReturnURL
	if returnURL == "" {
		returnURL = e.config["returnUrl"]
REDACTED
	return notifyURL, returnURL
REDACTED

func (e *EasyPay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	params := map[string]string{
		"act": "order", "pid": e.config["pid"],
		"key": e.config["pkey"], "out_trade_no": tradeNo,
REDACTED
	body, err := e.post(ctx, e.config["apiBase"]+"/api.php", params)
	if err != nil {
		return nil, fmt.Errorf("easypay query: %w", err)
REDACTED
	var resp struct {
		Code   int    `json:"code"`
		Msg    string `json:"msg"`
		Status int    `json:"status"`
REDACTED
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("easypay parse query: %w", err)
REDACTED
	status := payment.ProviderStatusPending
	if resp.Status == easypayStatusPaid {
		status = payment.ProviderStatusPaid
REDACTED
	return &payment.QueryOrderResponse{TradeNo: tradeNo, Status: statusREDACTED, nil
REDACTED

func (e *EasyPay) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	values, err := url.ParseQuery(rawBody)
	if err != nil {
		return nil, fmt.Errorf("parse notify: %w", err)
REDACTED
	params := make(map[string]string)
	for k := range values {
		params[k] = decodeURLValue(values.Get(k))
REDACTED
	sign := params["sign"]
	if sign == "" {
		return nil, fmt.Errorf("missing sign")
REDACTED
	if !easyPayVerifySign(params, e.config["pkey"], sign) {
		return nil, fmt.Errorf("invalid signature")
REDACTED
	status := payment.ProviderStatusFailed
	if params["trade_status"] == tradeStatusSuccess {
		status = payment.ProviderStatusSuccess
REDACTED
	amount, _ := strconv.ParseFloat(params["money"], 64)
	return &payment.PaymentNotification{
		TradeNo: params["trade_no"], OrderID: params["out_trade_no"],
		Amount: amount, Status: status, RawData: rawBody,
REDACTED, nil
REDACTED

func (e *EasyPay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	params := map[string]string{
		"pid": e.config["pid"], "key": e.config["pkey"],
		"trade_no": req.TradeNo, "out_trade_no": req.OrderID, "money": req.Amount,
REDACTED
	body, err := e.post(ctx, e.config["apiBase"]+"/api.php?act=refund", params)
	if err != nil {
		return nil, fmt.Errorf("easypay refund: %w", err)
REDACTED
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
REDACTED
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("easypay parse refund: %w", err)
REDACTED
	if resp.Code != easypayCodeSuccess {
		return nil, fmt.Errorf("easypay refund failed: %s", resp.Msg)
REDACTED
	return &payment.RefundResponse{RefundID: req.TradeNo, Status: payment.ProviderStatusSuccessREDACTED, nil
REDACTED

func (e *EasyPay) resolveCID(paymentType string) string {
	if strings.HasPrefix(paymentType, "alipay") {
		if v := e.config["cidAlipay"]; v != "" {
			return v
	REDACTED
		return e.config["cid"]
REDACTED
	if v := e.config["cidWxpay"]; v != "" {
		return v
REDACTED
	return e.config["cid"]
REDACTED

func (e *EasyPay) post(ctx context.Context, endpoint string, params map[string]string) ([]byte, error) {
	form := url.Values{REDACTED
	for k, v := range params {
		form.Set(k, v)
REDACTED
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	return io.ReadAll(io.LimitReader(resp.Body, maxEasypayResponseSize))
REDACTED

func easyPaySign(params map[string]string, pkey string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
	REDACTED
		keys = append(keys, k)
REDACTED
	sort.Strings(keys)
	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			_ = buf.WriteByte('&')
	REDACTED
		_, _ = buf.WriteString(k + "=" + params[k])
REDACTED
	_, _ = buf.WriteString(pkey)
	hash := md5.Sum([]byte(buf.String()))
	return hex.EncodeToString(hash[:])
REDACTED

func easyPayVerifySign(params map[string]string, pkey string, sign string) bool {
	return hmac.Equal([]byte(easyPaySign(params, pkey)), []byte(sign))
REDACTED

// decodeURLValue URL-decodes a string once.
func decodeURLValue(s string) string {
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		return s
REDACTED
	return decoded
REDACTED
