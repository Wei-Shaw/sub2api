package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
)

// Cancel rate limit configuration constants.
const (
	rateLimitUnitDay           = "day"
	rateLimitUnitMinute        = "minute"
	rateLimitUnitHour          = "hour"
	rateLimitModeFixed         = "fixed"
	checkPaidResultAlreadyPaid = "already_paid"
	checkPaidResultCancelled   = "cancelled"
	// checkPaidResultExpired is returned by cancelCore when an order
	// successfully transitioned from PENDING to EXPIRED via the CAS
	// update (i.e. expiry sweep actually flipped the row, not a no-op).
	checkPaidResultExpired = "expired"
)

// errLifecycleServiceUnavailable signals the plugin service was constructed
// without a working ent client. Handlers translate this to 503.
var errLifecycleServiceUnavailable = errors.New("payment: order lifecycle service unavailable")

// checkCancelRateLimit refuses excessive cancellation activity per user.
// Counts the ORDER_CANCELLED audit-log entries for the user inside the
// configured window and returns TOO_MANY_REQUESTS once the limit is hit.
// Failures during the count are intentionally ignored ("fail open") so a
// transient DB error doesn't block legitimate order creation.
func (s *PaymentService) checkCancelRateLimit(ctx context.Context, userID int64, cfg *PaymentConfig) error {
	if !cfg.CancelRateLimitEnabled || cfg.CancelRateLimitMax <= 0 {
		return nil
	}
	windowStart := cancelRateLimitWindowStart(cfg)
	operator := fmt.Sprintf("user:%d", userID)
	count, err := s.entClient.PaymentAuditLog.Query().
		Where(
			paymentauditlog.ActionEQ("ORDER_CANCELLED"),
			paymentauditlog.OperatorEQ(operator),
			paymentauditlog.CreatedAtGTE(windowStart),
		).Count(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("check cancel rate limit failed", "user_id", userID, "error", err)
		}
		return nil
	}
	if count >= cfg.CancelRateLimitMax {
		return infraerrors.TooManyRequests("CANCEL_RATE_LIMITED", "cancel rate limited").
			WithMetadata(map[string]string{
				"max":    strconv.Itoa(cfg.CancelRateLimitMax),
				"window": strconv.Itoa(cfg.CancelRateLimitWindow),
				"unit":   cfg.CancelRateLimitUnit,
			})
	}
	return nil
}

func cancelRateLimitWindowStart(cfg *PaymentConfig) time.Time {
	now := time.Now()
	w := cfg.CancelRateLimitWindow
	if w <= 0 {
		w = 1
	}
	unit := cfg.CancelRateLimitUnit
	if unit == "" {
		unit = rateLimitUnitDay
	}
	if cfg.CancelRateLimitMode == rateLimitModeFixed {
		return cancelRateLimitFixedStart(now, w, unit)
	}
	return cancelRateLimitRollingStart(now, w, unit)
}

func cancelRateLimitFixedStart(now time.Time, w int, unit string) time.Time {
	switch unit {
	case rateLimitUnitMinute:
		t := now.Truncate(time.Minute)
		return t.Add(-time.Duration(w-1) * time.Minute)
	case rateLimitUnitDay:
		y, m, d := now.Date()
		t := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
		return t.AddDate(0, 0, -(w - 1))
	default: // hour
		t := now.Truncate(time.Hour)
		return t.Add(-time.Duration(w-1) * time.Hour)
	}
}

func cancelRateLimitRollingStart(now time.Time, w int, unit string) time.Time {
	switch unit {
	case rateLimitUnitMinute:
		return now.Add(-time.Duration(w) * time.Minute)
	case rateLimitUnitDay:
		return now.AddDate(0, 0, -w)
	default: // hour
		return now.Add(-time.Duration(w) * time.Hour)
	}
}

// VerifyOrderByOutTradeNo is the authenticated reconciliation entry
// point for "did I actually pay?" UX flows (e.g. EasyPay popup mode
// where the notify callback may have been missed).
func (s *PaymentService) VerifyOrderByOutTradeNo(ctx context.Context, outTradeNo string, userID int64) (*pluginent.PaymentOrder, error) {
	if s == nil || s.entClient == nil {
		return nil, errLifecycleServiceUnavailable
	}
	outTradeNo, err := normalizeOrderLookupOutTradeNo(outTradeNo)
	if err != nil {
		return nil, err
	}
	o, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.OutTradeNoEQ(outTradeNo)).
		Only(ctx)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != userID {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
	}
	if o.Status == OrderStatusPending || o.Status == OrderStatusExpired {
		if s.checkPaid(ctx, o) == checkPaidResultAlreadyPaid {
			o, err = s.entClient.PaymentOrder.Get(ctx, o.ID)
			if err != nil {
				return nil, fmt.Errorf("reload order: %w", err)
			}
		}
	}
	return o, nil
}

// VerifyOrderPublic returns the persisted public order state without
// triggering upstream reconciliation. Anonymous callers (e.g. result
// page on a different device) only see the local snapshot.
func (s *PaymentService) VerifyOrderPublic(ctx context.Context, outTradeNo string) (*pluginent.PaymentOrder, error) {
	if s == nil || s.entClient == nil {
		return nil, errLifecycleServiceUnavailable
	}
	outTradeNo, err := normalizeOrderLookupOutTradeNo(outTradeNo)
	if err != nil {
		return nil, err
	}
	o, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.OutTradeNoEQ(outTradeNo)).
		Only(ctx)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	return o, nil
}

func normalizeOrderLookupOutTradeNo(raw string) (string, error) {
	outTradeNo := strings.TrimSpace(raw)
	if outTradeNo == "" {
		return "", infraerrors.BadRequest("INVALID_OUT_TRADE_NO", "out_trade_no is required")
	}
	if len(outTradeNo) > 64 {
		return "", infraerrors.BadRequest("INVALID_OUT_TRADE_NO", "out_trade_no is invalid")
	}
	for _, ch := range outTradeNo {
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
		case ch == '_' || ch == '-':
		default:
			return "", infraerrors.BadRequest("INVALID_OUT_TRADE_NO", "out_trade_no is invalid")
		}
	}
	return outTradeNo, nil
}

// ExpireTimedOutOrders sweeps orders whose timeout has elapsed and
// transitions them to EXPIRED. Each candidate is first reconciled
// against the upstream provider so a payment that arrived inside the
// grace window is not lost. Returns the number of orders successfully
// transitioned to EXPIRED so the caller can emit a single summary log
// (and avoid one log line per order).
func (s *PaymentService) ExpireTimedOutOrders(ctx context.Context) (int, error) {
	if s == nil || s.entClient == nil {
		return 0, nil
	}
	now := time.Now()
	orders, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.StatusEQ(OrderStatusPending), paymentorder.ExpiresAtLTE(now)).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query expired: %w", err)
	}
	expired := 0
	for _, o := range orders {
		outcome, _ := s.cancelCore(ctx, o, OrderStatusExpired, "system", "order expired")
		switch outcome {
		case checkPaidResultExpired:
			expired++
		case checkPaidResultAlreadyPaid:
			if s.logger != nil {
				s.logger.Info("order was paid during expiry sweep", "order_id", o.ID)
			}
		}
	}
	return expired, nil
}
