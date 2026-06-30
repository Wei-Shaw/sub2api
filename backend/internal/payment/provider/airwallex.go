package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	airwallexDemoAPIBase      = "https://api-demo.airwallex.com/api/v1"
	airwallexProdAPIBase      = "https://api.airwallex.com/api/v1"
	airwallexDefaultCountry   = "CN"
	airwallexHTTPTimeout      = 15 * time.Second
	airwallexMaxResponseSize  = 1 << 20
	airwallexMaxErrorSummary  = 512
	airwallexTokenSkew        = 2 * time.Minute
	airwallexWebhookTolerance = 5 * time.Minute

	airwallexEventPaymentSucceeded = "payment_intent.succeeded"
	airwallexEventPaymentCancelled = "payment_intent.cancelled"

	airwallexPaymentStatusSucceeded = "SUCCEEDED"
	airwallexPaymentStatusCancelled = "CANCELLED"
	airwallexRefundStatusReceived   = "RECEIVED"
	airwallexRefundStatusAccepted   = "ACCEPTED"
	airwallexRefundStatusSettled    = "SETTLED"
	airwallexRefundStatusFailed     = "FAILED"
)

type Airwallex struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
REDACTED

type airwallexTokenState struct {
	mu        sync.Mutex
	token     string
	expiresAt time.Time
REDACTED

var airwallexAccessTokens sync.Map

func NewAirwallex(instanceID string, config map[string]string) (*Airwallex, error) {
	for _, k := range []string{"clientId", "apiKey", "webhookSecret", "apiBase"REDACTED {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("airwallex config missing required key: %s", k)
	REDACTED
REDACTED
	cfg := cloneStringMap(config)
	apiBase, err := normalizeAirwallexAPIBase(cfg["apiBase"])
	if err != nil {
		return nil, err
REDACTED
	cfg["apiBase"] = apiBase
	currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
	if err != nil {
		return nil, fmt.Errorf("airwallex config currency: %w", err)
REDACTED
	cfg["currency"] = currency
	countryCode, err := normalizeAirwallexCountryCode(cfg["countryCode"])
	if err != nil {
		return nil, err
REDACTED
	cfg["countryCode"] = countryCode
	return &Airwallex{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: airwallexHTTPTimeoutREDACTED,
REDACTED, nil
REDACTED

func normalizeAirwallexCountryCode(raw string) (string, error) {
	countryCode := strings.ToUpper(strings.TrimSpace(raw))
	if countryCode == "" {
		return airwallexDefaultCountry, nil
REDACTED
	if len(countryCode) != 2 {
		return "", fmt.Errorf("airwallex config countryCode must be a two-letter ISO country code")
REDACTED
	for _, ch := range countryCode {
		if ch < 'A' || ch > 'Z' {
			return "", fmt.Errorf("airwallex config countryCode must be a two-letter ISO country code")
	REDACTED
REDACTED
	return countryCode, nil
REDACTED

func normalizeAirwallexAPIBase(raw string) (string, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		return "", fmt.Errorf("airwallex apiBase is required")
REDACTED
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("airwallex apiBase must be an HTTPS URL")
REDACTED
	host := strings.ToLower(parsed.Host)
	if host != "api-demo.airwallex.com" && host != "api.airwallex.com" {
		return "", fmt.Errorf("airwallex apiBase host must be api-demo.airwallex.com or api.airwallex.com")
REDACTED
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawPath = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = "/api/v1"
REDACTED
	if parsed.Path != "/api/v1" {
		return "", fmt.Errorf("airwallex apiBase path must be /api/v1")
REDACTED
	return parsed.String(), nil
REDACTED

func (a *Airwallex) Name() string        { return "空中云汇" REDACTED
func (a *Airwallex) ProviderKey() string { return payment.TypeAirwallex REDACTED
func (a *Airwallex) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAirwallexREDACTED
REDACTED

func (a *Airwallex) MerchantIdentityMetadata() map[string]string {
	if a == nil {
		return nil
REDACTED
	metadata := map[string]string{"currency": a.currency()REDACTED
	if accountID := strings.TrimSpace(a.config["accountId"]); accountID != "" {
		metadata["account_id"] = accountID
REDACTED
	return metadata
REDACTED

func (a *Airwallex) currency() string {
	if a == nil {
		return payment.DefaultPaymentCurrency
REDACTED
	currency, err := payment.NormalizePaymentCurrency(a.config["currency"])
	if err != nil {
		return payment.DefaultPaymentCurrency
REDACTED
	return currency
REDACTED

func (a *Airwallex) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("airwallex create payment: invalid amount %s", req.Amount)
REDACTED
	token, err := a.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("airwallex auth: %w", err)
REDACTED

	currency := a.currency()
	requestID := airwallexDeterministicRequestID("payment-intent", req.OrderID, req.Amount, currency)
	payload := airwallexCreatePaymentIntentRequest{
		RequestID:       requestID,
		Amount:          newAirwallexRequestAmount(amount),
		Currency:        currency,
		MerchantOrderID: req.OrderID,
		ReturnURL:       req.ReturnURL,
		Metadata: map[string]string{
			"order_id": req.OrderID,
	REDACTED,
REDACTED
	if descriptor := strings.TrimSpace(a.config["descriptor"]); descriptor != "" {
		payload.Descriptor = descriptor
REDACTED

	var intent airwallexPaymentIntent
	if err := a.doJSON(ctx, http.MethodPost, "/pa/payment_intents/create", token, payload, &intent); err != nil {
		return nil, fmt.Errorf("airwallex create payment: %w", err)
REDACTED
	if strings.TrimSpace(intent.ID) == "" || strings.TrimSpace(intent.ClientSecret) == "" {
		return nil, fmt.Errorf("airwallex create payment: missing payment intent id or client secret")
REDACTED
	return &payment.CreatePaymentResponse{
		TradeNo:      intent.ID,
		ClientSecret: intent.ClientSecret,
		IntentID:     intent.ID,
		Currency:     currency,
		CountryCode:  a.config["countryCode"],
		PaymentEnv:   a.checkoutEnv(),
REDACTED, nil
REDACTED

func (a *Airwallex) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	intentID := strings.TrimSpace(tradeNo)
	if intentID == "" {
		return nil, fmt.Errorf("airwallex query order: missing payment intent id")
REDACTED
	token, err := a.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("airwallex auth: %w", err)
REDACTED

	var intent airwallexPaymentIntent
	if err := a.doJSON(ctx, http.MethodGet, "/pa/payment_intents/"+url.PathEscape(intentID), token, nil, &intent); err != nil {
		return nil, fmt.Errorf("airwallex query order: %w", err)
REDACTED
	return &payment.QueryOrderResponse{
		TradeNo:  intent.ID,
		Status:   airwallexProviderStatus(intent.Status),
		Amount:   intent.Amount.InexactFloat64(),
		Metadata: a.intentMetadata(intent, ""),
REDACTED, nil
REDACTED

func (a *Airwallex) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if err := verifyAirwallexWebhookSignature(rawBody, headers, a.config["webhookSecret"], time.Now()); err != nil {
		return nil, err
REDACTED

	var event airwallexWebhookEvent
	if err := json.Unmarshal([]byte(rawBody), &event); err != nil {
		return nil, fmt.Errorf("airwallex parse webhook: %w", err)
REDACTED
	switch event.Name {
	case airwallexEventPaymentSucceeded, airwallexEventPaymentCancelled:
	default:
		return nil, nil
REDACTED

	var intent airwallexPaymentIntent
	if err := json.Unmarshal(event.Data.Object, &intent); err != nil {
		return nil, fmt.Errorf("airwallex parse payment intent: %w", err)
REDACTED
	if strings.TrimSpace(intent.ID) == "" || strings.TrimSpace(intent.MerchantOrderID) == "" {
		return nil, fmt.Errorf("airwallex webhook missing payment intent id or merchant_order_id")
REDACTED
	status := payment.ProviderStatusFailed
	if event.Name == airwallexEventPaymentSucceeded {
		if strings.ToUpper(strings.TrimSpace(intent.Status)) != airwallexPaymentStatusSucceeded {
			return nil, fmt.Errorf("airwallex succeeded webhook has non-succeeded status: %s", intent.Status)
	REDACTED
		status = payment.NotificationStatusSuccess
REDACTED

	return &payment.PaymentNotification{
		TradeNo:  intent.ID,
		OrderID:  intent.MerchantOrderID,
		Amount:   intent.Amount.InexactFloat64(),
		Status:   status,
		RawData:  rawBody,
		Metadata: a.intentMetadata(intent, event.accountID()),
REDACTED, nil
REDACTED

func (a *Airwallex) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	intentID := strings.TrimSpace(req.TradeNo)
	if intentID == "" {
		return nil, fmt.Errorf("airwallex refund missing payment intent id")
REDACTED
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("airwallex refund: invalid amount %s", req.Amount)
REDACTED
	token, err := a.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("airwallex auth: %w", err)
REDACTED

	payload := airwallexCreateRefundRequest{
		RequestID:       airwallexDeterministicRequestID("refund", intentID, req.Amount),
		PaymentIntentID: intentID,
		Amount:          newAirwallexRequestAmount(amount),
		Reason:          strings.TrimSpace(req.Reason),
REDACTED
	if payload.Reason == "" {
		payload.Reason = "refund"
REDACTED

	var resp airwallexRefund
	if err := a.doJSON(ctx, http.MethodPost, "/pa/refunds/create", token, payload, &resp); err != nil {
		return nil, fmt.Errorf("airwallex refund: %w", err)
REDACTED
	if strings.TrimSpace(resp.ID) == "" {
		return nil, fmt.Errorf("airwallex refund: missing refund id")
REDACTED
	refundResp := &payment.RefundResponse{
		RefundID: resp.ID,
		Status:   airwallexRefundProviderStatus(resp.Status),
REDACTED
	if refundResp.Status != payment.ProviderStatusSuccess {
		return refundResp, fmt.Errorf("airwallex refund not settled: status %s", strings.ToUpper(strings.TrimSpace(resp.Status)))
REDACTED
	return refundResp, nil
REDACTED

func (a *Airwallex) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	refundID := strings.TrimSpace(req.RefundID)
	if refundID == "" {
		return nil, fmt.Errorf("airwallex query refund: missing refund id")
REDACTED
	token, err := a.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("airwallex auth: %w", err)
REDACTED
	var resp airwallexRefund
	if err := a.doJSON(ctx, http.MethodGet, "/pa/refunds/"+url.PathEscape(refundID), token, nil, &resp); err != nil {
		return nil, fmt.Errorf("airwallex query refund: %w", err)
REDACTED
	if strings.TrimSpace(resp.ID) == "" {
		resp.ID = refundID
REDACTED
	return &payment.RefundResponse{RefundID: resp.ID, Status: airwallexRefundProviderStatus(resp.Status)REDACTED, nil
REDACTED

func (a *Airwallex) CancelPayment(ctx context.Context, tradeNo string) error {
	intentID := strings.TrimSpace(tradeNo)
	if intentID == "" {
		return nil
REDACTED
	token, err := a.accessToken(ctx)
	if err != nil {
		return fmt.Errorf("airwallex auth: %w", err)
REDACTED
	var intent airwallexPaymentIntent
	if err := a.doJSON(ctx, http.MethodPost, "/pa/payment_intents/"+url.PathEscape(intentID)+"/cancel", token, nil, &intent); err != nil {
		return fmt.Errorf("airwallex cancel payment: %w", err)
REDACTED
	return nil
REDACTED

func (a *Airwallex) intentMetadata(intent airwallexPaymentIntent, accountID string) map[string]string {
	metadata := map[string]string{
		"currency": strings.ToUpper(strings.TrimSpace(intent.Currency)),
		"status":   strings.ToUpper(strings.TrimSpace(intent.Status)),
REDACTED
	if accountID = strings.TrimSpace(accountID); accountID != "" {
		metadata["account_id"] = accountID
REDACTED else if configured := strings.TrimSpace(a.config["accountId"]); configured != "" {
		metadata["account_id"] = configured
REDACTED
	return metadata
REDACTED

func (a *Airwallex) checkoutEnv() string {
	if strings.EqualFold(a.config["apiBase"], airwallexProdAPIBase) {
		return "prod"
REDACTED
	return "demo"
REDACTED

func (a *Airwallex) accessToken(ctx context.Context) (string, error) {
	cacheKey := a.tokenCacheKey()
	rawState, _ := airwallexAccessTokens.LoadOrStore(cacheKey, &airwallexTokenState{REDACTED)
	state, ok := rawState.(*airwallexTokenState)
	if !ok {
		return "", fmt.Errorf("airwallex auth token cache state type mismatch")
REDACTED
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.token != "" && time.Now().Add(airwallexTokenSkew).Before(state.expiresAt) {
		return state.token, nil
REDACTED

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config["apiBase"]+"/authentication/login", nil)
	if err != nil {
		return "", err
REDACTED
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-client-id", a.config["clientId"])
	req.Header.Set("x-api-key", a.config["apiKey"])
	if accountID := strings.TrimSpace(a.config["accountId"]); accountID != "" {
		req.Header.Set("x-login-as", accountID)
REDACTED

	body, status, err := a.do(req)
	if err != nil {
		return "", err
REDACTED
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", formatAirwallexAuthHTTPError(status, body)
REDACTED
	var resp airwallexAuthResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse authentication response: %w", err)
REDACTED
	if strings.TrimSpace(resp.Token) == "" {
		return "", fmt.Errorf("authentication response missing token")
REDACTED
	expiresAt, err := parseAirwallexTime(resp.ExpiresAt)
	if err != nil {
		expiresAt = time.Now().Add(25 * time.Minute)
REDACTED
	state.token = resp.Token
	state.expiresAt = expiresAt
	return state.token, nil
REDACTED

func formatAirwallexAuthHTTPError(status int, body []byte) error {
	summary := summarizeAirwallexResponse(body)
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("authentication HTTP %d: %s; Airwallex credentials were rejected, check Client ID/API Key, API Base environment (sandbox: https://api-demo.airwallex.com/api/v1, production: https://api.airwallex.com/api/v1), and Account ID (leave it empty for single-account scoped keys)", status, summary)
REDACTED
	return fmt.Errorf("authentication HTTP %d: %s", status, summary)
REDACTED

func (a *Airwallex) tokenCacheKey() string {
	sum := sha256.Sum256([]byte(a.config["apiKey"]))
	return a.config["apiBase"] + "|" + a.config["clientId"] + "|" + strings.TrimSpace(a.config["accountId"]) + "|" + hex.EncodeToString(sum[:8])
REDACTED

func (a *Airwallex) doJSON(ctx context.Context, method, path, token string, payload any, out any) error {
	var bodyReader io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return err
	REDACTED
		bodyReader = bytes.NewReader(body)
REDACTED
	req, err := http.NewRequestWithContext(ctx, method, a.config["apiBase"]+path, bodyReader)
	if err != nil {
		return err
REDACTED
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
REDACTED
	if accountID := strings.TrimSpace(a.config["accountId"]); accountID != "" {
		req.Header.Set("x-on-behalf-of", accountID)
REDACTED

	body, status, err := a.do(req)
	if err != nil {
		return err
REDACTED
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return fmt.Errorf("HTTP %d: %s", status, summarizeAirwallexResponse(body))
REDACTED
	if out == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
REDACTED
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
REDACTED
	return nil
REDACTED

func (a *Airwallex) do(req *http.Request) ([]byte, int, error) {
	client := a.httpClient
	if client == nil {
		client = &http.Client{Timeout: airwallexHTTPTimeoutREDACTED
REDACTED
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	body, err := io.ReadAll(io.LimitReader(resp.Body, airwallexMaxResponseSize))
	if err != nil {
		return nil, resp.StatusCode, err
REDACTED
	return body, resp.StatusCode, nil
REDACTED

func airwallexProviderStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case airwallexPaymentStatusSucceeded:
		return payment.ProviderStatusPaid
	case airwallexPaymentStatusCancelled:
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
REDACTED
REDACTED

func airwallexRefundProviderStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case airwallexRefundStatusSettled:
		return payment.ProviderStatusSuccess
	case airwallexRefundStatusFailed:
		return payment.ProviderStatusFailed
	case airwallexRefundStatusReceived, airwallexRefundStatusAccepted:
		return payment.ProviderStatusPending
	default:
		return payment.ProviderStatusPending
REDACTED
REDACTED

func airwallexDeterministicRequestID(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	var id uuid.UUID
	copy(id[:], hash[:16])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id.String()
REDACTED

func verifyAirwallexWebhookSignature(rawBody string, headers map[string]string, secret string, now time.Time) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return fmt.Errorf("airwallex webhookSecret not configured")
REDACTED
	timestamp := strings.TrimSpace(headers["x-timestamp"])
	signature := strings.ToLower(strings.TrimSpace(headers["x-signature"]))
	if timestamp == "" || signature == "" {
		return fmt.Errorf("airwallex notification missing x-timestamp or x-signature header")
REDACTED

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte(rawBody))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("airwallex invalid signature")
REDACTED

	ts, err := parseAirwallexWebhookTimestamp(timestamp)
	if err != nil {
		return err
REDACTED
	if now.IsZero() {
		now = time.Now()
REDACTED
	if diff := now.Sub(ts).Abs(); diff > airwallexWebhookTolerance {
		return fmt.Errorf("airwallex webhook timestamp outside tolerance")
REDACTED
	return nil
REDACTED

func parseAirwallexWebhookTimestamp(raw string) (time.Time, error) {
	ts, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil {
		return time.Time{REDACTED, fmt.Errorf("airwallex invalid webhook timestamp")
REDACTED
	millis := ts.IntPart()
	if millis <= 0 {
		return time.Time{REDACTED, fmt.Errorf("airwallex invalid webhook timestamp")
REDACTED
	return time.UnixMilli(millis), nil
REDACTED

func parseAirwallexTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{REDACTED, fmt.Errorf("empty time")
REDACTED
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05-0700", "2006-01-02T15:04:05.000-0700"REDACTED {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
	REDACTED
REDACTED
	return time.Time{REDACTED, fmt.Errorf("invalid time: %s", raw)
REDACTED

func summarizeAirwallexResponse(body []byte) string {
	summary := strings.Join(strings.Fields(string(body)), " ")
	if summary == "" {
		return "<empty>"
REDACTED
	if len(summary) > airwallexMaxErrorSummary {
		return summary[:airwallexMaxErrorSummary] + "..."
REDACTED
	return summary
REDACTED

type airwallexAuthResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
REDACTED

type airwallexCreatePaymentIntentRequest struct {
	RequestID       string                 `json:"request_id"`
	Amount          airwallexRequestAmount `json:"amount"`
	Currency        string                 `json:"currency"`
	MerchantOrderID string                 `json:"merchant_order_id"`
	ReturnURL       string                 `json:"return_url,omitempty"`
	Descriptor      string                 `json:"descriptor,omitempty"`
	Metadata        map[string]string      `json:"metadata,omitempty"`
REDACTED

type airwallexCreateRefundRequest struct {
	RequestID       string                 `json:"request_id"`
	PaymentIntentID string                 `json:"payment_intent_id"`
	Amount          airwallexRequestAmount `json:"amount,omitempty"`
	Reason          string                 `json:"reason,omitempty"`
REDACTED

type airwallexRequestAmount struct {
	decimal.Decimal
REDACTED

func newAirwallexRequestAmount(amount decimal.Decimal) airwallexRequestAmount {
	return airwallexRequestAmount{Decimal: amountREDACTED
REDACTED

func (a airwallexRequestAmount) MarshalJSON() ([]byte, error) {
	return []byte(a.String()), nil
REDACTED

func (a *airwallexRequestAmount) UnmarshalJSON(data []byte) error {
	amount, err := decimal.NewFromString(strings.Trim(string(data), `"`))
	if err != nil {
		return err
REDACTED
	a.Decimal = amount
	return nil
REDACTED

type airwallexPaymentIntent struct {
	ID              string            `json:"id"`
	RequestID       string            `json:"request_id"`
	ClientSecret    string            `json:"client_secret"`
	MerchantOrderID string            `json:"merchant_order_id"`
	Amount          decimal.Decimal   `json:"amount"`
	Currency        string            `json:"currency"`
	Status          string            `json:"status"`
	Metadata        map[string]string `json:"metadata"`
REDACTED

type airwallexRefund struct {
	ID              string          `json:"id"`
	RequestID       string          `json:"request_id"`
	PaymentIntentID string          `json:"payment_intent_id"`
	Amount          decimal.Decimal `json:"amount"`
	Currency        string          `json:"currency"`
	Status          string          `json:"status"`
REDACTED

type airwallexWebhookEvent struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	AccountID      string `json:"accountId"`
	AccountIDSnake string `json:"account_id"`
	Data           struct {
		Object json.RawMessage `json:"object"`
REDACTED `json:"data"`
REDACTED

func (e airwallexWebhookEvent) accountID() string {
	if accountID := strings.TrimSpace(e.AccountID); accountID != "" {
		return accountID
REDACTED
	return strings.TrimSpace(e.AccountIDSnake)
REDACTED

var (
	_ payment.Provider                 = (*Airwallex)(nil)
	_ payment.CancelableProvider       = (*Airwallex)(nil)
	_ payment.MerchantIdentityProvider = (*Airwallex)(nil)
)
