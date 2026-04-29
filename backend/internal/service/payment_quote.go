package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	paymentQuoteTokenType = "payment_quote"
	paymentQuoteTokenTTL  = 10 * time.Minute
)

type CreatePaymentQuoteRequest struct {
	UserID          int64
	Amount          float64
	AmountMode      string
	PaymentCurrency string
	PaymentType     string
	OrderType       string
	PlanID          int64
}

type PaymentQuoteResponse struct {
	QuoteID         string    `json:"quote_id"`
	ExpiresAt       time.Time `json:"expires_at"`
	Amount          float64   `json:"amount"`
	AmountMode      string    `json:"amount_mode"`
	PaymentType     string    `json:"payment_type"`
	OrderType       string    `json:"order_type"`
	PlanID          int64     `json:"plan_id,omitempty"`
	PaymentAmount   float64   `json:"payment_amount"`
	PaymentCurrency string    `json:"payment_currency"`
	LedgerAmount    float64   `json:"ledger_amount"`
	LedgerCurrency  string    `json:"ledger_currency"`
	FXRate          float64   `json:"fx_rate"`
	FXSource        string    `json:"fx_source"`
	FXTimestamp     time.Time `json:"fx_timestamp"`
}

type PaymentQuoteClaims struct {
	TokenType       string  `json:"tk"`
	UserID          int64   `json:"uid"`
	Amount          float64 `json:"amt"`
	AmountMode      string  `json:"am"`
	PaymentType     string  `json:"pt"`
	OrderType       string  `json:"ot"`
	PlanID          int64   `json:"pid,omitempty"`
	PaymentAmount   float64 `json:"pam"`
	PaymentCurrency string  `json:"pc"`
	LedgerAmount    float64 `json:"lam"`
	LedgerCurrency  string  `json:"lc"`
	LimitAmount     float64 `json:"lim"`
	FXRate          float64 `json:"fx"`
	FXSource        string  `json:"fxs,omitempty"`
	FXTimestamp     int64   `json:"fxt"`
	IssuedAt        int64   `json:"iat"`
	ExpiresAt       int64   `json:"exp"`
}

func (s *PaymentService) CreatePaymentQuote(ctx context.Context, req CreatePaymentQuoteRequest) (*PaymentQuoteResponse, error) {
	if req.OrderType == "" {
		req.OrderType = payment.OrderTypeBalance
	}
	if normalized := NormalizeVisibleMethod(req.PaymentType); normalized != "" {
		req.PaymentType = normalized
	}
	amountMode, err := normalizePaymentAmountMode(req.AmountMode)
	if err != nil {
		return nil, err
	}
	req.AmountMode = amountMode

	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
	}
	if !cfg.Enabled {
		return nil, infraerrors.Forbidden("PAYMENT_DISABLED", "payment system is disabled")
	}

	createReq := CreateOrderRequest{
		UserID:          req.UserID,
		Amount:          req.Amount,
		AmountMode:      req.AmountMode,
		PaymentCurrency: req.PaymentCurrency,
		PaymentType:     req.PaymentType,
		OrderType:       req.OrderType,
		PlanID:          req.PlanID,
	}
	if err := s.resolveRequestPaymentCurrency(ctx, &createReq, cfg); err != nil {
		return nil, err
	}
	plan, err := s.validateOrderInput(ctx, createReq, cfg)
	if err != nil {
		return nil, err
	}
	amounts, err := computeCreateOrderAmounts(createReq, cfg, plan, time.Now())
	if err != nil {
		return nil, err
	}
	if err := validateLedgerAmountLimits(req.OrderType, amounts.LimitLedgerAmount, cfg); err != nil {
		return nil, err
	}

	issuedAt := time.Now()
	expiresAt := issuedAt.Add(paymentQuoteTokenTTL)
	claims := PaymentQuoteClaims{
		TokenType:       paymentQuoteTokenType,
		UserID:          req.UserID,
		Amount:          req.Amount,
		AmountMode:      req.AmountMode,
		PaymentType:     req.PaymentType,
		OrderType:       req.OrderType,
		PlanID:          req.PlanID,
		PaymentAmount:   amounts.PaymentAmount,
		PaymentCurrency: amounts.FXSnapshot.PaymentCurrency,
		LedgerAmount:    amounts.LedgerAmount,
		LedgerCurrency:  amounts.FXSnapshot.LedgerCurrency,
		LimitAmount:     amounts.LimitLedgerAmount,
		FXRate:          amounts.FXSnapshot.RatePaymentToLedger,
		FXSource:        amounts.FXSnapshot.Source,
		FXTimestamp:     amounts.FXSnapshot.Timestamp.Unix(),
		IssuedAt:        issuedAt.Unix(),
		ExpiresAt:       expiresAt.Unix(),
	}
	quoteID, err := s.createPaymentQuoteToken(claims)
	if err != nil {
		return nil, err
	}
	return paymentQuoteResponseFromClaims(quoteID, claims), nil
}

func (s *PaymentService) createPaymentQuoteToken(claims PaymentQuoteClaims) (string, error) {
	if s == nil || s.resumeService == nil {
		return "", infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
	}
	if err := s.resumeService.ensureSigningKey(); err != nil {
		return "", err
	}
	claims.TokenType = paymentQuoteTokenType
	if claims.IssuedAt == 0 {
		claims.IssuedAt = time.Now().Unix()
	}
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = time.Now().Add(paymentQuoteTokenTTL).Unix()
	}
	return s.resumeService.createSignedToken(claims)
}

func (s *PaymentService) ParsePaymentQuoteToken(token string) (*PaymentQuoteClaims, error) {
	if s == nil || s.resumeService == nil {
		return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
	}
	if err := s.resumeService.ensureSigningKey(); err != nil {
		return nil, err
	}
	var claims PaymentQuoteClaims
	if err := s.resumeService.parseSignedToken(token, &claims); err != nil {
		return nil, infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote payload is invalid")
	}
	if err := validatePaymentQuoteClaims(&claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func validatePaymentQuoteClaims(claims *PaymentQuoteClaims) error {
	if claims == nil || claims.TokenType != paymentQuoteTokenType {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote token type mismatch")
	}
	if claims.UserID <= 0 {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote missing user")
	}
	if claims.Amount <= 0 || math.IsNaN(claims.Amount) || math.IsInf(claims.Amount, 0) {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote amount is invalid")
	}
	if claims.PaymentAmount <= 0 || claims.LedgerAmount <= 0 || claims.LimitAmount <= 0 || claims.FXRate <= 0 {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote snapshot is invalid")
	}
	if err := validatePaymentResumeExpiry(claims.ExpiresAt, "PAYMENT_QUOTE_EXPIRED", "payment quote has expired"); err != nil {
		return err
	}
	claims.AmountMode = strings.TrimSpace(claims.AmountMode)
	if _, err := normalizePaymentAmountMode(claims.AmountMode); err != nil {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote amount mode is invalid")
	}
	claims.PaymentType = strings.TrimSpace(claims.PaymentType)
	if normalized := NormalizeVisibleMethod(claims.PaymentType); normalized != "" {
		claims.PaymentType = normalized
	}
	if claims.PaymentType == "" {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote missing payment type")
	}
	claims.OrderType = strings.TrimSpace(claims.OrderType)
	if claims.OrderType == "" {
		claims.OrderType = payment.OrderTypeBalance
	}
	claims.PaymentCurrency = normalizeCurrencyCode(claims.PaymentCurrency, "")
	claims.LedgerCurrency = normalizeCurrencyCode(claims.LedgerCurrency, defaultLedgerCurrency)
	if claims.PaymentCurrency == "" || claims.LedgerCurrency == "" {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote currency is invalid")
	}
	if claims.FXTimestamp <= 0 {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote missing fx timestamp")
	}
	return nil
}

func paymentQuoteResponseFromClaims(quoteID string, claims PaymentQuoteClaims) *PaymentQuoteResponse {
	return &PaymentQuoteResponse{
		QuoteID:         quoteID,
		ExpiresAt:       time.Unix(claims.ExpiresAt, 0).UTC(),
		Amount:          claims.Amount,
		AmountMode:      claims.AmountMode,
		PaymentType:     claims.PaymentType,
		OrderType:       claims.OrderType,
		PlanID:          claims.PlanID,
		PaymentAmount:   claims.PaymentAmount,
		PaymentCurrency: claims.PaymentCurrency,
		LedgerAmount:    claims.LedgerAmount,
		LedgerCurrency:  claims.LedgerCurrency,
		FXRate:          claims.FXRate,
		FXSource:        claims.FXSource,
		FXTimestamp:     time.Unix(claims.FXTimestamp, 0).UTC(),
	}
}

func (s *PaymentService) applyPaymentQuoteToCreateOrder(req *CreateOrderRequest) error {
	if req == nil || strings.TrimSpace(req.QuoteID) == "" {
		return nil
	}
	claims, err := s.ParsePaymentQuoteToken(req.QuoteID)
	if err != nil {
		return err
	}
	if claims.UserID != req.UserID {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote user mismatch")
	}
	if req.PaymentType != "" {
		requestPaymentType := req.PaymentType
		if normalized := NormalizeVisibleMethod(requestPaymentType); normalized != "" {
			requestPaymentType = normalized
		}
		if requestPaymentType != claims.PaymentType {
			return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote payment type mismatch")
		}
	}
	if req.OrderType != "" && req.OrderType != claims.OrderType {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote order type mismatch")
	}
	if req.PlanID > 0 && req.PlanID != claims.PlanID {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote plan mismatch")
	}
	if req.Amount > 0 && !paymentQuoteFloatEqual(req.Amount, claims.Amount) {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote amount mismatch")
	}
	if strings.TrimSpace(req.AmountMode) != "" && strings.TrimSpace(req.AmountMode) != claims.AmountMode {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote amount mode mismatch")
	}
	if normalizeCurrencyCode(req.PaymentCurrency, "") != "" && normalizeCurrencyCode(req.PaymentCurrency, "") != claims.PaymentCurrency {
		return infraerrors.BadRequest("INVALID_PAYMENT_QUOTE", "payment quote currency mismatch")
	}
	req.Amount = claims.Amount
	req.AmountMode = claims.AmountMode
	req.PaymentCurrency = claims.PaymentCurrency
	req.PaymentType = claims.PaymentType
	req.OrderType = claims.OrderType
	req.PlanID = claims.PlanID
	req.paymentQuote = claims
	return nil
}

func createOrderAmountsFromPaymentQuote(claims *PaymentQuoteClaims) createOrderAmounts {
	if claims == nil {
		return createOrderAmounts{}
	}
	return createOrderAmounts{
		LedgerAmount:      claims.LedgerAmount,
		PaymentAmount:     claims.PaymentAmount,
		LimitLedgerAmount: claims.LimitAmount,
		FXSnapshot: fxSnapshot{
			LedgerCurrency:      claims.LedgerCurrency,
			PaymentCurrency:     claims.PaymentCurrency,
			RatePaymentToLedger: claims.FXRate,
			Source:              claims.FXSource,
			Timestamp:           time.Unix(claims.FXTimestamp, 0).UTC(),
		},
	}
}

func paymentQuoteFloatEqual(a, b float64) bool {
	return math.Abs(a-b) <= floatRoundoffTolerance
}
