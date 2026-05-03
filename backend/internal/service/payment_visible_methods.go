// Package service — payment_visible_methods.go
//
// Host-side payment-method *routing* settings. These settings live in
// the global settings table because they decide which payment plugin
// ("official_alipay", "easypay_wxpay", …) the front-end should advertise
// to the user — independent of the payment plugin's own internal config.
//
// The full payment package (gateways, providers, order lifecycle) moved
// to plugins/payment/ during the gRPC plugin migration. The constants /
// helpers below are the minimal subset the host needs to keep
// SettingService working without depending on the plugin.
package service

import (
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Setting keys for payment-method routing (alipay/wxpay → which provider
// the front-end should hit). Stored in the host-side settings table.
const (
	SettingPaymentVisibleMethodAlipaySource  = "payment_visible_method_alipay_source"
	SettingPaymentVisibleMethodWxpaySource   = "payment_visible_method_wxpay_source"
	SettingPaymentVisibleMethodAlipayEnabled = "payment_visible_method_alipay_enabled"
	SettingPaymentVisibleMethodWxpayEnabled  = "payment_visible_method_wxpay_enabled"
)

// Canonical source identifiers — must mirror what the payment plugin
// recognizes when it receives the routing decision.
const (
	VisibleMethodSourceOfficialAlipay = "official_alipay"
	VisibleMethodSourceEasyPayAlipay  = "easypay_alipay"
	VisibleMethodSourceOfficialWechat = "official_wxpay"
	VisibleMethodSourceEasyPayWechat  = "easypay_wxpay"
)

// Method names — bare strings, not tied to the (now-plugin) payment
// package's PaymentType type.
const (
	visibleMethodAlipay       = "alipay"
	visibleMethodAlipayDirect = "alipay_direct"
	visibleMethodWxpay        = "wxpay"
	visibleMethodWxpayDirect  = "wxpay_direct"
	visibleMethodEasyPay      = "easypay"
)

// NormalizeVisibleMethod canonicalizes a method string to its base
// payment type ("alipay" or "wxpay"). The plugin-side getBasePaymentType
// implementation is the source of truth; this is the host-side mirror
// kept narrow on purpose (no provider/strategy logic in core).
func NormalizeVisibleMethod(method string) string {
	switch strings.TrimSpace(strings.ToLower(method)) {
	case visibleMethodAlipay, visibleMethodAlipayDirect:
		return visibleMethodAlipay
	case visibleMethodWxpay, visibleMethodWxpayDirect:
		return visibleMethodWxpay
	default:
		return ""
	}
}

// NormalizeVisibleMethodSource maps an arbitrary source string to one
// of the four canonical VisibleMethodSource* constants for the given
// method, or "" when the pairing is invalid.
func NormalizeVisibleMethodSource(method, source string) string {
	switch NormalizeVisibleMethod(method) {
	case visibleMethodAlipay:
		switch strings.TrimSpace(strings.ToLower(source)) {
		case VisibleMethodSourceOfficialAlipay,
			visibleMethodAlipay, visibleMethodAlipayDirect, "official":
			return VisibleMethodSourceOfficialAlipay
		case VisibleMethodSourceEasyPayAlipay, visibleMethodEasyPay:
			return VisibleMethodSourceEasyPayAlipay
		}
	case visibleMethodWxpay:
		switch strings.TrimSpace(strings.ToLower(source)) {
		case VisibleMethodSourceOfficialWechat,
			visibleMethodWxpay, visibleMethodWxpayDirect, "wechat", "official":
			return VisibleMethodSourceOfficialWechat
		case VisibleMethodSourceEasyPayWechat, visibleMethodEasyPay:
			return VisibleMethodSourceEasyPayWechat
		}
	}
	return ""
}

// normalizeVisibleMethodSettingSource validates a source string supplied
// via PUT /settings, returning a structured error on rejection.
func normalizeVisibleMethodSettingSource(method, source string, enabled bool) (string, error) {
	_ = enabled
	source = strings.TrimSpace(source)
	if source == "" {
		return "", nil
	}
	normalized := NormalizeVisibleMethodSource(method, source)
	if normalized == "" {
		return "", infraerrors.BadRequest(
			"INVALID_PAYMENT_VISIBLE_METHOD_SOURCE",
			fmt.Sprintf("%s source must be one of the supported payment providers", method),
		)
	}
	return normalized, nil
}
