package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const (
	EasyPayCompatibilityModeKey  = "compatibilityMode"
	EasyPayCompatibilityStandard = "standard"
	EasyPayCompatibilityA5       = "a5"

	a5OrderStatusPending  = 0
	a5OrderStatusPaid     = 1
	a5OrderStatusRefunded = 2
)

// A5EasyPay is deliberately a separate concrete type. Standard EasyPay must
// not acquire A5's refund-query capability or response semantics.
type A5EasyPay struct {
	*EasyPay
}

var _ payment.Provider = (*A5EasyPay)(nil)
var _ payment.RefundQueryProvider = (*A5EasyPay)(nil)

func EasyPayCompatibilityMode(config map[string]string) string {
	mode := strings.ToLower(strings.TrimSpace(config[EasyPayCompatibilityModeKey]))
	if mode == "" {
		return EasyPayCompatibilityStandard
	}
	return mode
}

func NewA5EasyPay(base *EasyPay) *A5EasyPay {
	return &A5EasyPay{EasyPay: base}
}

// CreatePayment removes merchant-side state from return_url because A5
// HTML-encodes embedded query separators before redirecting the browser.
func (a *A5EasyPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	_, returnURL := a.resolveURLs(req)
	req.ReturnURL = cleanA5ReturnURL(returnURL)
	return a.EasyPay.CreatePayment(ctx, req)
}

func cleanA5ReturnURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

type a5OrderQueryResult struct {
	TradeNo    string
	OutTradeNo string
	PID        string
	Money      string
	Status     string
}

func (a *A5EasyPay) QueryOrder(ctx context.Context, outTradeNo string) (*payment.QueryOrderResponse, error) {
	result, err := a.queryOrder(ctx, strings.TrimSpace(outTradeNo), "")
	if err != nil {
		return nil, err
	}
	amount, err := strconv.ParseFloat(result.Money, 64)
	if err != nil {
		return nil, fmt.Errorf("a5 easypay query returned invalid amount")
	}
	return &payment.QueryOrderResponse{
		TradeNo: result.TradeNo,
		Status:  result.Status,
		Amount:  amount,
		Metadata: map[string]string{
			"pid":          result.PID,
			"out_trade_no": result.OutTradeNo,
		},
	}, nil
}

func (a *A5EasyPay) queryOrder(ctx context.Context, outTradeNo, tradeNo string) (*a5OrderQueryResult, error) {
	query := url.Values{
		"act": {"order"},
		"pid": {strings.TrimSpace(a.config["pid"])},
		"key": {strings.TrimSpace(a.config["pkey"])},
	}
	if strings.TrimSpace(tradeNo) != "" {
		query.Set("trade_no", strings.TrimSpace(tradeNo))
	} else if strings.TrimSpace(outTradeNo) != "" {
		query.Set("out_trade_no", strings.TrimSpace(outTradeNo))
	} else {
		return nil, errors.New("a5 easypay query missing order identifier")
	}

	body, status, err := a.getRaw(ctx, a.apiBase()+"/api.php?"+query.Encode())
	if err != nil {
		return nil, fmt.Errorf("a5 easypay query request failed: %w", err)
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("a5 easypay query returned HTTP %d", status)
	}

	var resp struct {
		Code       json.RawMessage `json:"code"`
		Msg        string          `json:"msg"`
		TradeNo    string          `json:"trade_no"`
		OutTradeNo string          `json:"out_trade_no"`
		PID        string          `json:"pid"`
		Money      string          `json:"money"`
		Status     json.RawMessage `json:"status"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, errors.New("a5 easypay query returned invalid JSON")
	}
	code, ok := parseJSONInteger(resp.Code)
	if !ok || code != easypayCodeSuccess {
		return nil, fmt.Errorf("a5 easypay query rejected: code=%s msg=%s", safeJSONScalar(resp.Code), boundedA5Message(resp.Msg))
	}
	numericStatus, ok := parseJSONInteger(resp.Status)
	if !ok {
		return nil, errors.New("a5 easypay query returned invalid status")
	}
	providerStatus := ""
	switch numericStatus {
	case a5OrderStatusPending:
		providerStatus = payment.ProviderStatusPending
	case a5OrderStatusPaid:
		providerStatus = payment.ProviderStatusPaid
	case a5OrderStatusRefunded:
		providerStatus = payment.ProviderStatusRefunded
	default:
		return nil, fmt.Errorf("a5 easypay query returned unknown status: %d", numericStatus)
	}

	result := &a5OrderQueryResult{
		TradeNo:    strings.TrimSpace(resp.TradeNo),
		OutTradeNo: strings.TrimSpace(resp.OutTradeNo),
		PID:        strings.TrimSpace(resp.PID),
		Money:      strings.TrimSpace(resp.Money),
		Status:     providerStatus,
	}
	if err := a.validateQueryIdentity(result, strings.TrimSpace(outTradeNo), strings.TrimSpace(tradeNo)); err != nil {
		return nil, err
	}
	return result, nil
}

func (a *A5EasyPay) validateQueryIdentity(result *a5OrderQueryResult, outTradeNo, tradeNo string) error {
	if result == nil || result.PID == "" || result.OutTradeNo == "" || result.TradeNo == "" || result.Money == "" {
		return fmt.Errorf("%w: A5 response is missing merchant, order, or amount identity", ErrEasyPayQueryIdentityMismatch)
	}
	if result.PID != strings.TrimSpace(a.config["pid"]) {
		return fmt.Errorf("%w: A5 response merchant differs from configured pid", ErrEasyPayQueryIdentityMismatch)
	}
	if outTradeNo != "" && result.OutTradeNo != outTradeNo {
		return fmt.Errorf("%w: A5 response out_trade_no differs from requested order", ErrEasyPayQueryIdentityMismatch)
	}
	if tradeNo != "" && result.TradeNo != tradeNo {
		return fmt.Errorf("%w: A5 response trade_no differs from requested order", ErrEasyPayQueryIdentityMismatch)
	}
	if _, ok := parsePositiveDecimal(result.Money); !ok {
		return fmt.Errorf("%w: A5 response money is invalid", ErrEasyPayQueryIdentityMismatch)
	}
	return nil
}

func (a *A5EasyPay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	if !req.FullRefund {
		return nil, errors.New("a5 easypay only supports full refunds")
	}
	attempts := a.refundAttempts(req)
	if len(attempts) == 0 {
		return nil, errors.New("a5 easypay refund missing order identifier")
	}

	for i, attempt := range attempts {
		body, status, err := a.postRaw(ctx, a.apiBase()+"/api.php?act=refund", attempt.params)
		if err != nil {
			return a.confirmFullRefundOrPending(ctx, req, "", "refund request outcome unknown")
		}
		payload, err := parseA5RefundPayload(status, body)
		if err != nil {
			return a.confirmFullRefundOrPending(ctx, req, "", err.Error())
		}

		switch payload.Code {
		case 0, easypayCodeSuccess:
			if err := a.validateRefundSuccess(payload, req); err == nil {
				return &payment.RefundResponse{
					RefundID:               payload.RefundNo,
					RefundIDProviderIssued: true,
					Status:                 payment.ProviderStatusSuccess,
					Message:                boundedA5Message(payload.Msg),
				}, nil
			}
			return a.confirmFullRefundOrPending(ctx, req, "", "refund success response required verification")
		default:
			if isA5AlreadyFullyRefundedMessage(payload.Msg) {
				return a.confirmFullRefundOrPending(ctx, req, payload.RefundNo, "gateway reports order already fully refunded")
			}
			if i+1 < len(attempts) && isA5OrderNotFoundMessage(payload.Msg) {
				continue
			}
			return nil, fmt.Errorf("a5 easypay refund failed: code=%d msg=%s", payload.Code, boundedA5Message(payload.Msg))
		}
	}
	return nil, errors.New("a5 easypay refund failed for all order identifiers")
}

type a5RefundPayload struct {
	Code        int
	RefundNo    string
	OutRefundNo string
	TradeNo     string
	OutTradeNo  string
	UID         string
	Money       string
	Msg         string
}

func parseA5RefundPayload(status int, body []byte) (*a5RefundPayload, error) {
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("refund response HTTP %d is not conclusive", status)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, errors.New("refund response is empty")
	}
	var raw struct {
		Code        json.RawMessage `json:"code"`
		RefundNo    string          `json:"refund_no"`
		OutRefundNo string          `json:"out_refund_no"`
		TradeNo     string          `json:"trade_no"`
		OutTradeNo  string          `json:"out_trade_no"`
		UID         string          `json:"uid"`
		Money       string          `json:"money"`
		Msg         string          `json:"msg"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, errors.New("refund response is not valid JSON")
	}
	code, ok := parseJSONInteger(raw.Code)
	if !ok {
		return nil, errors.New("refund response code is missing or invalid")
	}
	return &a5RefundPayload{
		Code:        code,
		RefundNo:    strings.TrimSpace(raw.RefundNo),
		OutRefundNo: strings.TrimSpace(raw.OutRefundNo),
		TradeNo:     strings.TrimSpace(raw.TradeNo),
		OutTradeNo:  strings.TrimSpace(raw.OutTradeNo),
		UID:         strings.TrimSpace(raw.UID),
		Money:       strings.TrimSpace(raw.Money),
		Msg:         strings.TrimSpace(raw.Msg),
	}, nil
}

func (a *A5EasyPay) validateRefundSuccess(payload *a5RefundPayload, req payment.RefundRequest) error {
	if payload == nil || !isA5RefundSuccessMessage(payload.Msg) {
		return errors.New("A5 success message is missing")
	}
	if payload.RefundNo == "" || payload.OutRefundNo == "" {
		return errors.New("A5 refund number is missing")
	}
	if payload.OutRefundNo != payload.RefundNo {
		return errors.New("A5 refund number mismatch")
	}
	if payload.TradeNo == "" || payload.OutTradeNo == "" || payload.UID == "" || payload.Money == "" {
		return errors.New("A5 refund response identity is incomplete")
	}
	if req.TradeNo != "" && payload.TradeNo != strings.TrimSpace(req.TradeNo) {
		return errors.New("A5 refund trade_no mismatch")
	}
	if req.OrderID != "" && payload.OutTradeNo != strings.TrimSpace(req.OrderID) {
		return errors.New("A5 refund out_trade_no mismatch")
	}
	if payload.UID != strings.TrimSpace(a.config["pid"]) {
		return errors.New("A5 refund merchant mismatch")
	}
	if !equalDecimal(payload.Money, req.Amount) {
		return errors.New("A5 refund amount mismatch")
	}
	return nil
}

func (a *A5EasyPay) confirmFullRefundOrPending(ctx context.Context, req payment.RefundRequest, refundID, reason string) (*payment.RefundResponse, error) {
	result, err := a.queryOrder(ctx, strings.TrimSpace(req.OrderID), strings.TrimSpace(req.TradeNo))
	if err != nil {
		return &payment.RefundResponse{
			RefundID:               strings.TrimSpace(refundID),
			RefundIDProviderIssued: strings.TrimSpace(refundID) != "",
			Status:                 payment.ProviderStatusPending,
			Message:                boundedA5Message(reason + "; verification unavailable"),
		}, nil
	}
	if result.Status == payment.ProviderStatusRefunded && equalDecimal(result.Money, req.Amount) {
		return &payment.RefundResponse{
			RefundID:               strings.TrimSpace(refundID),
			RefundIDProviderIssued: strings.TrimSpace(refundID) != "",
			Status:                 payment.ProviderStatusRefunded,
			Message:                boundedA5Message(reason + "; confirmed by A5 order query"),
		}, nil
	}
	return &payment.RefundResponse{
		RefundID:               strings.TrimSpace(refundID),
		RefundIDProviderIssued: strings.TrimSpace(refundID) != "",
		Status:                 payment.ProviderStatusPending,
		Message:                boundedA5Message(reason + "; A5 order is not confirmed fully refunded"),
	}, nil
}

func (a *A5EasyPay) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	if !req.FullRefund {
		return &payment.RefundResponse{RefundID: req.RefundID, RefundIDProviderIssued: req.RefundID != "", Status: payment.ProviderStatusPending, Message: "A5 partial refund requires manual review"}, nil
	}
	result, err := a.queryOrder(ctx, strings.TrimSpace(req.OrderID), strings.TrimSpace(req.TradeNo))
	if err != nil {
		return nil, err
	}
	if result.Status == payment.ProviderStatusRefunded && equalDecimal(result.Money, req.Amount) {
		return &payment.RefundResponse{RefundID: req.RefundID, RefundIDProviderIssued: req.RefundID != "", Status: payment.ProviderStatusRefunded, Message: "confirmed by A5 order query"}, nil
	}
	return &payment.RefundResponse{RefundID: req.RefundID, RefundIDProviderIssued: req.RefundID != "", Status: payment.ProviderStatusPending, Message: "A5 order is not confirmed fully refunded"}, nil
}

func (a *A5EasyPay) getRaw(ctx context.Context, endpoint string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	originHost := req.URL.Host
	client := a.httpClient
	if client == nil {
		client = &http.Client{Timeout: easypayHTTPTimeout}
	}
	cloned := *client
	previousRedirectCheck := cloned.CheckRedirect
	cloned.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if !strings.EqualFold(next.URL.Host, originHost) {
			return errors.New("A5 query refused cross-host redirect")
		}
		if previousRedirectCheck != nil {
			return previousRedirectCheck(next, via)
		}
		if len(via) >= 10 {
			return errors.New("A5 query stopped after 10 redirects")
		}
		return nil
	}
	resp, err := cloned.Do(req)
	if err != nil {
		return nil, 0, errors.New("A5 query transport failed")
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxEasypayResponseSize))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func parseJSONInteger(raw json.RawMessage) (int, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return 0, false
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, true
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, false
	}
	number, err := strconv.Atoi(strings.TrimSpace(text))
	return number, err == nil
}

func safeJSONScalar(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "<missing>"
	}
	if len(value) > 32 {
		return "<invalid>"
	}
	return value
}

func parsePositiveDecimal(value string) (*big.Rat, bool) {
	parsed, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	return parsed, ok && parsed.Sign() > 0
}

func equalDecimal(left, right string) bool {
	l, ok := parsePositiveDecimal(left)
	if !ok {
		return false
	}
	r, ok := parsePositiveDecimal(right)
	return ok && l.Cmp(r) == 0
}

func isA5RefundSuccessMessage(message string) bool {
	normalized := strings.TrimSpace(message)
	return normalized == "退款成功" ||
		strings.HasPrefix(normalized, "退款成功！退款金额¥") ||
		strings.HasPrefix(normalized, "退款成功！退款金额￥")
}

func isA5AlreadyFullyRefundedMessage(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(normalized, "已全额退款") || strings.Contains(normalized, "already fully refunded")
}

func isA5OrderNotFoundMessage(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(normalized, "订单不存在") ||
		strings.Contains(normalized, "订单编号不存在") ||
		strings.Contains(normalized, "当前订单不存在") ||
		strings.Contains(normalized, "order not found")
}

func boundedA5Message(message string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if len(message) > maxEasypayErrorSummary {
		return message[:maxEasypayErrorSummary] + "..."
	}
	return message
}
