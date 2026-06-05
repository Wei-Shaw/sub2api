package service

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// NormalizeVisibleMethod collapses provider-specific aliases (alipay /
// alipay_direct / wxpay / wxpay_direct / easypay) onto the canonical
// alipay or wxpay payment-type token used in storage and settings.
func NormalizeVisibleMethod(method string) string {
	return payment.GetBasePaymentType(strings.TrimSpace(method))
}

// NormalizeVisibleMethods de-duplicates a slice of methods after
// normalisation. The empty string is filtered out.
func NormalizeVisibleMethods(methods []string) []string {
	if len(methods) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(methods))
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		normalized := NormalizeVisibleMethod(method)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

// NormalizePaymentSource maps free-form source strings onto the two
// canonical sources understood by the rest of the plugin.
func NormalizePaymentSource(source string) string {
	switch strings.TrimSpace(strings.ToLower(source)) {
	case "", PaymentSourceHostedRedirect:
		return PaymentSourceHostedRedirect
	case "wechat_in_app", "wxpay_resume", PaymentSourceWechatInAppResume:
		return PaymentSourceWechatInAppResume
	default:
		return strings.TrimSpace(strings.ToLower(source))
	}
}

// NormalizeVisibleMethodSource pairs a method (alipay/wxpay) with the
// "source" the operator enabled for it (official/easypay), returning the
// canonical compound key written to storage.
func NormalizeVisibleMethodSource(method, source string) string {
	switch NormalizeVisibleMethod(method) {
	case payment.TypeAlipay:
		switch strings.TrimSpace(strings.ToLower(source)) {
		case VisibleMethodSourceOfficialAlipay, payment.TypeAlipay, payment.TypeAlipayDirect, "official":
			return VisibleMethodSourceOfficialAlipay
		case VisibleMethodSourceEasyPayAlipay, payment.TypeEasyPay:
			return VisibleMethodSourceEasyPayAlipay
		}
	case payment.TypeWxpay:
		switch strings.TrimSpace(strings.ToLower(source)) {
		case VisibleMethodSourceOfficialWechat, payment.TypeWxpay, payment.TypeWxpayDirect, "wechat", "official":
			return VisibleMethodSourceOfficialWechat
		case VisibleMethodSourceEasyPayWechat, payment.TypeEasyPay:
			return VisibleMethodSourceEasyPayWechat
		}
	}
	return ""
}

// VisibleMethodProviderKeyForSource returns the provider-key the load
// balancer should use when an operator paired (method, source). The
// boolean indicates whether the requested method matches the canonical
// official base; false signals an indirect routing (e.g. easypay).
func VisibleMethodProviderKeyForSource(method, source string) (string, bool) {
	switch NormalizeVisibleMethodSource(method, source) {
	case VisibleMethodSourceOfficialAlipay:
		return payment.TypeAlipay, NormalizeVisibleMethod(method) == payment.TypeAlipay
	case VisibleMethodSourceEasyPayAlipay:
		return payment.TypeEasyPay, NormalizeVisibleMethod(method) == payment.TypeAlipay
	case VisibleMethodSourceOfficialWechat:
		return payment.TypeWxpay, NormalizeVisibleMethod(method) == payment.TypeWxpay
	case VisibleMethodSourceEasyPayWechat:
		return payment.TypeEasyPay, NormalizeVisibleMethod(method) == payment.TypeWxpay
	default:
		return "", false
	}
}

// CanonicalizeReturnURL validates a caller-supplied return_url against
// the same-origin policy and forces it onto the canonical /payment/result
// path. Returns the cleaned URL or a 400 ApplicationError.
func CanonicalizeReturnURL(raw, srcHost, srcURL string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be an absolute http/https URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must use http or https")
	}
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	if parsed.Path != paymentResultReturnPath {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must target the canonical internal payment result page")
	}
	if !allowedReturnURLHost(parsed.Host, srcHost, srcURL) {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must use the same host as the current site or browser origin")
	}
	return parsed.String(), nil
}

func allowedReturnURLHost(returnURLHost, requestHost, refererURL string) bool {
	if sameOriginHost(returnURLHost, requestHost) {
		return true
	}
	refererURL = strings.TrimSpace(refererURL)
	if refererURL == "" {
		return false
	}
	parsedReferer, err := url.Parse(refererURL)
	if err != nil || parsedReferer.Host == "" {
		return false
	}
	return sameOriginHost(returnURLHost, parsedReferer.Host)
}

func buildPaymentReturnURL(base string, orderID int64, outTradeNo, resumeToken string) (string, error) {
	canonical := strings.TrimSpace(base)
	if canonical == "" {
		return "", nil
	}
	parsed, err := url.Parse(canonical)
	if err != nil {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be a valid URL")
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be a valid absolute URL")
	}
	parsed.Fragment = ""
	query := parsed.Query()
	if orderID > 0 {
		query.Set("order_id", strconv.FormatInt(orderID, 10))
	}
	if strings.TrimSpace(outTradeNo) != "" {
		query.Set("out_trade_no", strings.TrimSpace(outTradeNo))
	}
	if strings.TrimSpace(resumeToken) != "" {
		query.Set("resume_token", strings.TrimSpace(resumeToken))
	}
	query.Set("status", "success")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func sameOriginHost(returnURLHost, requestHost string) bool {
	returnHost := strings.TrimSpace(returnURLHost)
	reqHost := strings.TrimSpace(requestHost)
	if returnHost == "" || reqHost == "" {
		return false
	}
	if strings.EqualFold(returnHost, reqHost) {
		return true
	}
	returnName, returnPort := splitHostPortDefault(returnHost)
	reqName, reqPort := splitHostPortDefault(reqHost)
	if returnName == "" || reqName == "" {
		return false
	}
	return strings.EqualFold(returnName, reqName) && returnPort == reqPort
}

func splitHostPortDefault(raw string) (string, string) {
	if host, port, err := net.SplitHostPort(raw); err == nil {
		return host, port
	}
	return raw, ""
}
