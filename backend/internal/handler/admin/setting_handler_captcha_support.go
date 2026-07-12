package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Fork-only helpers preserved after upstream split setting_handler.go into
// multiple files. These back the captcha (Turnstile/hCaptcha/Tencent) and
// support-chat settings handling that upstream does not have.

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// captchaEditableFields returns the captcha_config keys that admin PUT /settings
// must trim and apply "keep previous value when blank" semantics to.
// publicFields are readable/editable; secretFields are masked on read.
func captchaEditableFields(provider string) (publicFields, secretFields []string) {
	switch provider {
	case service.CaptchaProviderTurnstile, service.CaptchaProviderHcaptcha:
		return []string{"site_key"}, []string{"secret_key"}
	case service.CaptchaProviderTencent:
		return []string{"captcha_app_id"}, []string{"app_secret_key", "secret_id", "secret_key"}
	default:
		return nil, nil
	}
}

// captchaPrimarySecretField returns the captcha_config key used as the primary
// secret for the given provider.
func captchaPrimarySecretField(provider string) string {
	switch provider {
	case service.CaptchaProviderTencent:
		return "app_secret_key"
	default:
		return "secret_key"
	}
}

// captchaPrimarySiteField returns the captcha_config key used as the primary
// public/site key for the given provider.
func captchaPrimarySiteField(provider string) string {
	switch provider {
	case service.CaptchaProviderTencent:
		return "captcha_app_id"
	default:
		return "site_key"
	}
}

// equalSupportChatFAQs reports whether two support-chat FAQ slices are equal.
func equalSupportChatFAQs(a, b []service.SupportChatFAQ) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Question != b[i].Question ||
			a[i].Answer != b[i].Answer ||
			a[i].SortOrder != b[i].SortOrder ||
			a[i].Enabled != b[i].Enabled {
			return false
		}
	}
	return true
}
