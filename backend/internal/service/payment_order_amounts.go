package service

import (
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type createOrderAmounts struct {
	LedgerAmount      float64
	PaymentAmount     float64
	LimitLedgerAmount float64
	FXSnapshot        fxSnapshot
}

func computeCreateOrderAmounts(req CreateOrderRequest, cfg *PaymentConfig, plan *dbent.SubscriptionPlan, now time.Time) (createOrderAmounts, error) {
	if req.paymentQuote != nil {
		return createOrderAmountsFromPaymentQuote(req.paymentQuote), nil
	}
	snapshot, err := resolveFXSnapshot(req.PaymentCurrency, cfg, now)
	if err != nil {
		return createOrderAmounts{}, err
	}
	mode, err := normalizePaymentAmountMode(req.AmountMode)
	if err != nil {
		return createOrderAmounts{}, err
	}

	if plan != nil {
		ledgerAmount := roundLedgerAmountForCredit(plan.Price, snapshot.LedgerCurrency)
		paymentAmount, err := convertLedgerToPayment(ledgerAmount, snapshot)
		if err != nil {
			return createOrderAmounts{}, err
		}
		return createOrderAmounts{
			LedgerAmount:      ledgerAmount,
			PaymentAmount:     paymentAmount,
			LimitLedgerAmount: ledgerAmount,
			FXSnapshot:        snapshot,
		}, nil
	}

	var ledgerBaseAmount float64
	var paymentAmount float64
	switch mode {
	case PaymentAmountModePayment:
		paymentAmount = roundPaymentAmountForCollection(req.Amount, snapshot.PaymentCurrency)
		ledgerBaseAmount, err = convertPaymentToLedger(paymentAmount, snapshot)
		if err != nil {
			return createOrderAmounts{}, err
		}
	case PaymentAmountModeLedger:
		ledgerBaseAmount = roundLedgerAmountForCredit(req.Amount, snapshot.LedgerCurrency)
		paymentAmount, err = convertLedgerToPayment(ledgerBaseAmount, snapshot)
		if err != nil {
			return createOrderAmounts{}, err
		}
	}

	creditedLedgerAmount := calculateCreditedBalance(ledgerBaseAmount, cfg.BalanceRechargeMultiplier)
	return createOrderAmounts{
		LedgerAmount:      creditedLedgerAmount,
		PaymentAmount:     paymentAmount,
		LimitLedgerAmount: ledgerBaseAmount,
		FXSnapshot:        snapshot,
	}, nil
}

func normalizePaymentAmountMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", PaymentAmountModeLedger:
		return PaymentAmountModeLedger, nil
	case PaymentAmountModePayment:
		return PaymentAmountModePayment, nil
	default:
		return "", infraerrors.BadRequest("INVALID_AMOUNT_MODE", "amount_mode must be ledger or payment")
	}
}

func validateLedgerAmountLimits(orderType string, amount float64, cfg *PaymentConfig) error {
	if orderType != payment.OrderTypeBalance {
		return nil
	}
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
		return infraerrors.BadRequest("INVALID_AMOUNT", "amount must be a positive number")
	}
	if (cfg.MinAmount > 0 && amount < cfg.MinAmount) || (cfg.MaxAmount > 0 && amount > cfg.MaxAmount) {
		return infraerrors.BadRequest("INVALID_AMOUNT", "amount out of range").
			WithMetadata(map[string]string{"min": formatAmountLimit(cfg.MinAmount), "max": formatAmountLimit(cfg.MaxAmount)})
	}
	return nil
}

func formatAmountLimit(amount float64) string {
	formatted := strings.TrimRight(strings.TrimRight(strconv.FormatFloat(amount, 'f', 2, 64), "0"), ".")
	if formatted == "" {
		return "0"
	}
	return formatted
}
