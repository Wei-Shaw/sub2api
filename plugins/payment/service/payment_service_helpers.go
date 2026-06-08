package service

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// joinTypes joins a supported_types slice into the comma-separated form
// stored in payment_provider_instances.supported_types.
func joinTypes(types []string) string {
	out := make([]string, 0, len(types))
	for _, t := range types {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, ",")
}

// psStartOfDayUTC returns midnight UTC of the supplied instant. Used by
// the daily-recharge limit query to align the window with the business
// day reset that is documented to operators.
func psStartOfDayUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Validity period units for subscription plans.
const (
	validityUnitWeek  = "week"
	validityUnitMonth = "month"
)

// psComputeValidityDays converts (raw_count, unit) into days. "month" is
// counted as 30 days per the original payment service convention.
func psComputeValidityDays(days int, unit string) int {
	switch unit {
	case validityUnitWeek:
		return days * 7
	case validityUnitMonth:
		return days * 30
	default:
		return days
	}
}

// psSliceContains is a small generic-free contains helper kept here to
// avoid pulling in a separate slices import in this package.
func psSliceContains(sl []string, s string) bool {
	for _, v := range sl {
		if v == s {
			return true
		}
	}
	return false
}

// psIsRefundStatus reports whether the supplied order status falls
// inside the refund state-machine. Used by fulfillment / retry guards
// that must refuse to act on an already-refunded or in-flight refund
// order.
func psIsRefundStatus(s string) bool {
	switch s {
	case payment.OrderStatusRefundRequested,
		payment.OrderStatusRefunding,
		payment.OrderStatusPartiallyRefunded,
		payment.OrderStatusRefunded,
		payment.OrderStatusRefundFailed:
		return true
	}
	return false
}
