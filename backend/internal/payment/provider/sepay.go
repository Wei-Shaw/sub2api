package provider

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const (
	sepayDefaultQRBase   = "https://qr.sepay.vn/img"
	sepayDefaultCurrency = "VND"
	sepayTransferTypeIn  = "in"
)

var sepayOrderCodePattern = regexp.MustCompile(`(?i)\bsub2_[0-9]{8}[a-z0-9]{8}\b`)
var sepayEmbeddedOrderCodePattern = regexp.MustCompile(`(?i)sub2_[0-9]{8}[a-z0-9]{8}`)
var sepayOrderPlaceholderPattern = regexp.MustCompile(`(?i)\{(?:orderid|code)\}`)
var sepayQRContentCodePlaceholderPattern = regexp.MustCompile(`(?i)\{code\}`)

// Sepay implements the webhook-settled SePay bank-transfer QR flow.
//
// The provider deliberately has no HTTP client. QR creation is offline and
// uses the bank account selected in the provider config. The SePay API token,
// when configured, is used only by the admin bank-account picker.
type Sepay struct {
	instanceID      string
	config          map[string]string
	contentPatterns []*regexp.Regexp
}

// NewSepay creates a SePay provider. apiToken is intentionally not required
// here: QR creation and webhook settlement do not call the SePay REST API.
func NewSepay(instanceID string, config map[string]string) (*Sepay, error) {
	cfg := cloneStringMap(config)
	if strings.TrimSpace(sepayConfigValue(cfg, "bankCode")) == "" || strings.TrimSpace(sepayConfigValue(cfg, "accountNo")) == "" {
		return nil, errors.New("SEPAY_BANK_CONFIG_MISSING")
	}
	if strings.TrimSpace(sepayConfigValue(cfg, "webhookApiKey")) == "" {
		return nil, errors.New("SEPAY_WEBHOOK_API_KEY_MISSING")
	}
	if rawCurrency := strings.TrimSpace(sepayConfigValue(cfg, "currency")); rawCurrency != "" {
		currency, err := payment.NormalizePaymentCurrency(rawCurrency)
		if err != nil {
			return nil, fmt.Errorf("sepay config currency: %w", err)
		}
		cfg["currency"] = currency
	}
	contentPatterns, err := compileSepayContentPatterns(sepayConfigValue(cfg, "paymentContentRecognitionPatterns"))
	if err != nil {
		return nil, err
	}
	if err := validateSepayContentTemplates(sepayConfigValue(cfg, "paymentContentTemplates")); err != nil {
		return nil, err
	}
	return &Sepay{instanceID: instanceID, config: cfg, contentPatterns: contentPatterns}, nil
}

func (s *Sepay) Name() string        { return "SePay" }
func (s *Sepay) ProviderKey() string { return payment.TypeSepay }
func (s *Sepay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeSepay}
}

func (s *Sepay) MerchantIdentityMetadata() map[string]string {
	return map[string]string{"currency": sepayDefaultCurrency}
}

// CreatePayment builds a VietQR-compatible SePay URL without network access.
func (s *Sepay) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	currency := strings.ToUpper(strings.TrimSpace(req.PaymentCurrency))
	if currency == "" {
		currency = sepayDefaultCurrency
	}
	if currency != sepayDefaultCurrency {
		return nil, errors.New("SEPAY_CURRENCY_NOT_VND")
	}

	amount, err := parseSepayWholeVND(req.Amount)
	if err != nil {
		return nil, err
	}
	bankCode := strings.TrimSpace(sepayConfigValue(s.config, "bankCode"))
	accountNo := strings.TrimSpace(sepayConfigValue(s.config, "accountNo"))
	if bankCode == "" || accountNo == "" {
		return nil, errors.New("SEPAY_BANK_CONFIG_MISSING")
	}

	contentAliases := sepayConfigValue(s.config, "paymentContentAliases")
	if strings.TrimSpace(contentAliases) == "" {
		contentAliases = sepayConfigValue(s.config, "paymentContentPrefix")
	}
	transferContent := buildSepayTransferContent(req.OrderID, contentAliases, sepayConfigValue(s.config, "paymentContentTemplates"))
	qrBase := strings.TrimSpace(sepayConfigValue(s.config, "qrBase"))
	if qrBase == "" {
		qrBase = sepayDefaultQRBase
	}
	parsed, err := url.Parse(qrBase)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("SEPAY_QR_BASE_INVALID")
	}
	query := parsed.Query()
	query.Set("acc", accountNo)
	query.Set("bank", bankCode)
	query.Set("amount", strconv.FormatInt(amount, 10))
	query.Set("des", transferContent)
	parsed.RawQuery = query.Encode()

	return &payment.CreatePaymentResponse{
		TradeNo:  req.OrderID,
		QRCode:   parsed.String(),
		Currency: sepayDefaultCurrency,
	}, nil
}

func (s *Sepay) QueryOrder(_ context.Context, _ string) (*payment.QueryOrderResponse, error) {
	return nil, errors.New("sepay is webhook-only")
}

func (s *Sepay) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if !isSepayWebhookAuthorized(sepayConfigValue(s.config, "webhookApiKey"), headerValue(headers, "authorization")) {
		return nil, errors.New("SEPAY_WEBHOOK_UNAUTHORIZED")
	}

	var payload sepayWebhookPayload
	if err := json.Unmarshal([]byte(rawBody), &payload); err != nil {
		return nil, fmt.Errorf("SEPAY_WEBHOOK_INVALID_JSON: %w", err)
	}
	transferType := strings.ToLower(strings.TrimSpace(payload.TransferType.String()))
	if transferType != "" && transferType != sepayTransferTypeIn {
		return nil, nil
	}

	content := payload.Content.String()
	if content == "" {
		content = payload.TransactionContent.String()
	}
	orderID := s.extractOrderIDFromContent(content)
	if orderID == "" {
		orderID = NormalizeSepayOrderID(payload.Code.String())
	}
	if orderID == "" {
		return nil, nil
	}

	amount, err := payload.amount()
	if err != nil {
		return nil, err
	}
	transactionID := payload.ID.String()
	if transactionID == "" || transactionID == "0" {
		transactionID = firstNonBlank(payload.ReferenceCode.String(), payload.ReferenceNumber.String())
	}
	if transactionID == "" {
		return nil, errors.New("SEPAY_TRANSACTION_ID_MISSING")
	}

	metadata := map[string]string{
		"currency":      sepayDefaultCurrency,
		"bank_txn_id":   transactionID,
		"transfer_type": transferType,
	}
	return &payment.PaymentNotification{
		TradeNo:  transactionID,
		OrderID:  orderID,
		Amount:   float64(amount),
		Currency: sepayDefaultCurrency,
		Status:   payment.NotificationStatusSuccess,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

func (s *Sepay) Refund(_ context.Context, _ payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, errors.New("sepay does not support refunds")
}

func parseSepayWholeVND(raw string) (int64, error) {
	amount, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, errors.New("SEPAY_INVALID_AMOUNT")
	}
	if math.Trunc(amount) != amount {
		return 0, errors.New("SEPAY_AMOUNT_MUST_BE_WHOLE_VND")
	}
	if amount > math.MaxInt64 {
		return 0, errors.New("SEPAY_INVALID_AMOUNT")
	}
	return int64(amount), nil
}

func buildSepayTransferContent(orderID, configuredPrefixes, configuredTemplates string) string {
	orderID = strings.TrimSpace(orderID)
	prefixes := splitSepayConfigValues(configuredPrefixes)
	if len(prefixes) == 0 {
		return "CK " + orderID
	}
	prefix := prefixes[randomSepayIndex(len(prefixes))]
	code := prefix + orderID
	templates := splitSepayTemplates(configuredTemplates)
	if len(templates) == 0 {
		return code
	}
	template := templates[randomSepayIndex(len(templates))]
	return sepayQRContentCodePlaceholderPattern.ReplaceAllString(template, code)
}

func (s *Sepay) extractOrderIDFromContent(content string) string {
	if len(s.contentPatterns) == 0 {
		return ExtractSepayOrderIDFromContent(content)
	}
	return extractSepayOrderIDWithPatterns(content, s.contentPatterns)
}

func splitSepayConfigValues(raw string) []string {
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, item := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == ',' || r == ';' || r == '|'
	}) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		values = append(values, item)
	}
	return values
}

func splitSepayTemplates(raw string) []string {
	var templates []string
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &templates); err != nil {
			templates = splitSepayConfigValues(raw)
		}
	}
	result := make([]string, 0, len(templates))
	for _, template := range templates {
		if strings.TrimSpace(template) != "" {
			result = append(result, template)
		}
	}
	return result
}

func randomSepayIndex(length int) int {
	if length <= 1 {
		return 0
	}
	var b [1]byte
	if _, err := rand.Read(b[:]); err == nil {
		return int(b[0]) % length
	}
	return 0
}

// NormalizeSepayOrderID extracts and normalizes the exact order-ID format
// generated by sub2api. Only the prefix is lowercased; the date and random
// suffix retain their source casing for exact database lookup.
func NormalizeSepayOrderID(value string) string {
	match := sepayOrderCodePattern.FindString(strings.TrimSpace(value))
	if match == "" {
		return ""
	}
	return strings.ToLower(match[:len("sub2_")]) + match[len("sub2_"):]
}

// ExtractSepayOrderIDFromContent finds the sub2api order ID in noisy bank text.
func ExtractSepayOrderIDFromContent(content string) string {
	return NormalizeSepayOrderID(content)
}

func compileSepayContentPatterns(raw string) ([]*regexp.Regexp, error) {
	patterns := splitSepayTemplates(raw)
	if len(patterns) == 0 {
		return nil, nil
	}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		matches := sepayOrderPlaceholderPattern.FindAllStringIndex(pattern, -1)
		if len(matches) != 1 {
			return nil, fmt.Errorf("SEPAY_PAYMENT_CONTENT_PATTERN_INVALID: pattern %q must contain exactly one {orderId} or {code} placeholder", pattern)
		}

		placeholder := matches[0]
		literalBefore := quoteSepayPatternLiteral(pattern[:placeholder[0]])
		literalAfter := quoteSepayPatternLiteral(pattern[placeholder[1]:])
		compiledPattern, err := regexp.Compile(`(?i)` + literalBefore + sepayEmbeddedOrderCodePattern.String() + literalAfter)
		if err != nil {
			return nil, fmt.Errorf("SEPAY_PAYMENT_CONTENT_PATTERN_INVALID: pattern %q: %w", pattern, err)
		}
		compiled = append(compiled, compiledPattern)
	}
	return compiled, nil
}

func validateSepayContentTemplates(raw string) error {
	for _, template := range splitSepayTemplates(raw) {
		if len(sepayQRContentCodePlaceholderPattern.FindAllStringIndex(template, -1)) != 1 {
			return fmt.Errorf("SEPAY_PAYMENT_CONTENT_TEMPLATE_INVALID: template %q must contain exactly one {code} placeholder", template)
		}
	}
	return nil
}

func quoteSepayPatternLiteral(value string) string {
	var builder strings.Builder
	whitespace := false
	for _, char := range value {
		if unicode.IsSpace(char) {
			if !whitespace {
				_, _ = builder.WriteString(`\s+`)
				whitespace = true
			}
			continue
		}
		whitespace = false
		_, _ = builder.WriteString(regexp.QuoteMeta(string(char)))
	}
	return builder.String()
}

func extractSepayOrderIDWithPatterns(content string, patterns []*regexp.Regexp) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	matches := findValidSepayEmbeddedOrderMatches(content)
	if len(matches) != 1 {
		return ""
	}
	for _, pattern := range patterns {
		if pattern.MatchString(content) {
			return normalizeSepayOrderMatch(matches[0])
		}
	}
	return ""
}

func findValidSepayEmbeddedOrderMatches(content string) []string {
	indices := sepayEmbeddedOrderCodePattern.FindAllStringIndex(content, -1)
	matches := make([]string, 0, len(indices))
	for _, index := range indices {
		if index[1] < len(content) && isSepayOrderCodeChar(content[index[1]]) {
			continue
		}
		matches = append(matches, content[index[0]:index[1]])
	}
	return matches
}

func isSepayOrderCodeChar(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
}

func normalizeSepayOrderMatch(match string) string {
	return strings.ToLower(match[:len("sub2_")]) + match[len("sub2_"):]
}

func isSepayWebhookAuthorized(configuredKey, authorization string) bool {
	expected := strings.TrimSpace(configuredKey)
	if expected == "" {
		return false
	}
	parts := strings.Fields(strings.TrimSpace(authorization))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "apikey") {
		return false
	}
	actual := strings.TrimSpace(parts[1])
	if len(actual) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func sepayConfigValue(config map[string]string, name string) string {
	for key, value := range config {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

type sepayFlexibleString string

func (s *sepayFlexibleString) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*s = sepayFlexibleString(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		*s = sepayFlexibleString(number.String())
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(string(data)), "null") {
		*s = ""
		return nil
	}
	return fmt.Errorf("unsupported SePay string value")
}

func (s sepayFlexibleString) String() string {
	return strings.TrimSpace(string(s))
}

type sepayFlexibleNumber struct {
	value   float64
	present bool
}

func (n *sepayFlexibleNumber) UnmarshalJSON(data []byte) error {
	if strings.EqualFold(strings.TrimSpace(string(data)), "null") {
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	value, err := strconv.ParseFloat(number.String(), 64)
	if err != nil {
		return err
	}
	n.value = value
	n.present = true
	return nil
}

func (n sepayFlexibleNumber) validValue() bool {
	return n.present
}

type sepayWebhookPayload struct {
	ID                 sepayFlexibleString `json:"id"`
	Code               sepayFlexibleString `json:"code"`
	TransferType       sepayFlexibleString `json:"transferType"`
	TransferAmount     sepayFlexibleNumber `json:"transferAmount"`
	Amount             sepayFlexibleNumber `json:"amount"`
	AmountIn           sepayFlexibleNumber `json:"amount_in"`
	ReferenceCode      sepayFlexibleString `json:"referenceCode"`
	ReferenceNumber    sepayFlexibleString `json:"reference_number"`
	Content            sepayFlexibleString `json:"content"`
	TransactionContent sepayFlexibleString `json:"transaction_content"`
}

func (p sepayWebhookPayload) amount() (int64, error) {
	for _, candidate := range []sepayFlexibleNumber{p.TransferAmount, p.AmountIn, p.Amount} {
		if candidate.validValue() {
			return parseSepayWholeVND(strconv.FormatFloat(candidate.value, 'f', -1, 64))
		}
	}
	return 0, errors.New("SEPAY_INVALID_AMOUNT")
}
