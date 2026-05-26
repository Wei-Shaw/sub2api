package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// UpdatePaymentConfig persists each non-nil field via the SDK Settings
// API. Non-nil pointer fields are written one by one; the order doesn't
// matter because each field has its own settings key. Errors short-circuit
// at the first failure so the caller knows which field failed.
//
// The handler is responsible for calling paymentService.RefreshProviders
// after a successful update so the in-memory load balancer picks up
// changes immediately (matches release-branch behaviour).
func (s *PaymentConfigService) UpdatePaymentConfig(ctx context.Context, req UpdatePaymentConfigRequest) error {
	if s == nil || s.settings == nil {
		return errors.New("payment: settings client unavailable")
	}
	if err := s.updatePaymentScalarSettings(ctx, req); err != nil {
		return err
	}
	if err := s.updatePaymentVisibleMethodSettings(ctx, req); err != nil {
		return err
	}
	if err := s.updatePaymentCancelSettings(ctx, req); err != nil {
		return err
	}
	if req.EnabledTypes != nil {
		normalized := NormalizeVisibleMethods(req.EnabledTypes)
		if err := s.settings.Set(ctx, SettingEnabledPaymentTypes, normalized); err != nil {
			return fmt.Errorf("set %s: %w", SettingEnabledPaymentTypes, err)
		}
	}
	return nil
}

// updatePaymentScalarSettings persists the core payment configuration fields.
func (s *PaymentConfigService) updatePaymentScalarSettings(ctx context.Context, req UpdatePaymentConfigRequest) error {
	if err := s.setIfNotNilBool(ctx, SettingPaymentEnabled, req.Enabled); err != nil {
		return err
	}
	if err := s.setIfNotNilDecimal(ctx, SettingMinRechargeAmount, req.MinAmount); err != nil {
		return err
	}
	if err := s.setIfNotNilDecimal(ctx, SettingMaxRechargeAmount, req.MaxAmount); err != nil {
		return err
	}
	if err := s.setIfNotNilDecimal(ctx, SettingDailyRechargeLimit, req.DailyLimit); err != nil {
		return err
	}
	if err := s.setIfNotNilInt(ctx, SettingOrderTimeoutMinutes, req.OrderTimeoutMin); err != nil {
		return err
	}
	if err := s.setIfNotNilInt(ctx, SettingMaxPendingOrders, req.MaxPendingOrders); err != nil {
		return err
	}
	if err := s.setIfNotNilBool(ctx, SettingBalancePayDisabled, req.BalanceDisabled); err != nil {
		return err
	}
	if err := s.setIfNotNilDecimal(ctx, SettingBalanceRechargeMult, req.BalanceRechargeMultiplier); err != nil {
		return err
	}
	if err := s.setIfNotNilDecimal(ctx, SettingRechargeFeeRate, req.RechargeFeeRate); err != nil {
		return err
	}
	if err := s.setIfNotNilString(ctx, SettingLoadBalanceStrategy, req.LoadBalanceStrategy); err != nil {
		return err
	}
	if err := s.setIfNotNilString(ctx, SettingProductNamePrefix, req.ProductNamePrefix); err != nil {
		return err
	}
	if err := s.setIfNotNilString(ctx, SettingProductNameSuffix, req.ProductNameSuffix); err != nil {
		return err
	}
	if err := s.setIfNotNilString(ctx, SettingHelpImageURL, req.HelpImageURL); err != nil {
		return err
	}
	return s.setIfNotNilString(ctx, SettingHelpText, req.HelpText)
}

// updatePaymentCancelSettings persists the cancel rate-limit configuration.
func (s *PaymentConfigService) updatePaymentCancelSettings(ctx context.Context, req UpdatePaymentConfigRequest) error {
	if err := s.setIfNotNilBool(ctx, SettingCancelRateLimitOn, req.CancelRateLimitEnabled); err != nil {
		return err
	}
	if err := s.setIfNotNilInt(ctx, SettingCancelRateLimitMax, req.CancelRateLimitMax); err != nil {
		return err
	}
	if err := s.setIfNotNilInt(ctx, SettingCancelWindowSize, req.CancelRateLimitWindow); err != nil {
		return err
	}
	if err := s.setIfNotNilString(ctx, SettingCancelWindowUnit, req.CancelRateLimitUnit); err != nil {
		return err
	}
	return s.setIfNotNilString(ctx, SettingCancelWindowMode, req.CancelRateLimitMode)
}

// updatePaymentVisibleMethodSettings persists visible-method toggles and source assignments.
func (s *PaymentConfigService) updatePaymentVisibleMethodSettings(ctx context.Context, req UpdatePaymentConfigRequest) error {
	if err := s.setIfNotNilBool(ctx, SettingPaymentVisibleMethodAlipayEnabled, req.VisibleMethodAlipayEnabled); err != nil {
		return err
	}
	if err := s.setIfNotNilBool(ctx, SettingPaymentVisibleMethodWxpayEnabled, req.VisibleMethodWxpayEnabled); err != nil {
		return err
	}
	if err := s.setIfNotNilString(ctx, SettingPaymentVisibleMethodAlipaySource, req.VisibleMethodAlipaySource); err != nil {
		return err
	}
	return s.setIfNotNilString(ctx, SettingPaymentVisibleMethodWxpaySource, req.VisibleMethodWxpaySource)
}

func (s *PaymentConfigService) setIfNotNilBool(ctx context.Context, key string, v *bool) error {
	if v == nil {
		return nil
	}
	if err := s.settings.Set(ctx, key, *v); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	return nil
}

func (s *PaymentConfigService) setIfNotNilInt(ctx context.Context, key string, v *int) error {
	if v == nil {
		return nil
	}
	if err := s.settings.Set(ctx, key, *v); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	return nil
}

// setIfNotNilDecimal serialises a decimal value as its canonical string
// form so the settings store keeps full precision. The legacy float
// variant lost precision on values like 0.1+0.2; passing strings makes
// the round-trip lossless.
func (s *PaymentConfigService) setIfNotNilDecimal(ctx context.Context, key string, v *decimal.Decimal) error {
	if v == nil {
		return nil
	}
	if err := s.settings.Set(ctx, key, v.String()); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	return nil
}

func (s *PaymentConfigService) setIfNotNilString(ctx context.Context, key string, v *string) error {
	if v == nil {
		return nil
	}
	if err := s.settings.Set(ctx, key, *v); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	return nil
}

// splitTypes splits a CSV settings value, trimming whitespace.
func splitTypes(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
