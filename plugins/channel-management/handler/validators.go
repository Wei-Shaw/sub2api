// validators.go — register custom gin/binding validators backed by the
// service-layer enum lists. The handler struct tags reference the validator
// names (`channel_status` / `billing_mode` / `billing_model_source` /
// `platform`) so the *single source of truth* for "which strings are valid"
// is service.BillingModes / service.ChannelStatuses / service.Platforms /
// service.BillingModelSources. Renaming or extending an enum no longer
// requires keeping `oneof=...` tags hand-synced.
//
// Wiring: plugin.go / main.go must call RegisterCustomValidators() once
// during plugin initialisation, before the gin engine handles any
// request that could trigger ShouldBindJSON. Doing so lazily on first
// request would race with concurrent binds.

package handler

import (
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/service"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// RegisterCustomValidators teaches gin's default validator about the
// channel-management enum tags. Safe to call multiple times — re-registering
// the same tag name simply replaces the previous func.
//
// Returns an error if gin's underlying validator is not the expected
// go-playground/validator/v10 instance (gin can be reconfigured to use a
// different engine; if that happens we want a loud failure rather than a
// silent permissive-bind path).
func RegisterCustomValidators() error {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return errValidatorEngineUnsupported
	}
	tags := map[string][]string{
		"channel_status":       service.ChannelStatuses(),
		"billing_mode":         service.BillingModes(),
		"billing_model_source": service.BillingModelSources(),
		"platform":             service.Platforms(),
	}
	for tag, allowed := range tags {
		// Capture per-iteration `allowed` so the closure does not race with
		// later loop iterations (Go ≥ 1.22 fixes this at the language level
		// but we keep the explicit alias for readability).
		allowed := allowed
		if err := v.RegisterValidation(tag, makeOneOfValidator(allowed)); err != nil {
			return err
		}
	}
	return nil
}

// makeOneOfValidator returns a validator.Func that passes when the field
// value (treated as string) is empty or equals one of the allowed values.
// Empty-string passthrough mirrors the old struct-tag form
// `binding:"omitempty,oneof=..."` where omitempty short-circuited on "".
func makeOneOfValidator(allowed []string) validator.Func {
	return func(fl validator.FieldLevel) bool {
		got := fl.Field().String()
		if got == "" {
			return true
		}
		for _, v := range allowed {
			if got == v {
				return true
			}
		}
		return false
	}
}

// errValidatorEngineUnsupported is returned when gin's validator engine
// is not the expected go-playground/validator/v10 instance. Declared as a
// package-level value so callers can compare against it without import
// cycles.
var errValidatorEngineUnsupported = &unsupportedValidatorError{}

type unsupportedValidatorError struct{}

func (*unsupportedValidatorError) Error() string {
	return "channel-management handler: gin binding.Validator engine is not *validator.Validate; cannot register custom enum validators"
}
