//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPaymentFXStatusDetectsStaleAndMissingCurrencies(t *testing.T) {
	updatedAt := time.Now().UTC().Add(-2 * time.Hour)
	svc := &PaymentConfigService{}
	cfg := svc.parsePaymentConfig(map[string]string{
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND,KRW",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686}`,
		SettingFXRatesSource:            "manual",
		SettingFXRatesUpdatedAt:         updatedAt.Format(time.RFC3339),
		SettingFXRatesStaleAfterSeconds: "60",
	})

	require.Equal(t, "manual", cfg.FXStatus.Source)
	require.True(t, cfg.FXStatus.Stale)
	require.Equal(t, []string{"KRW"}, cfg.FXStatus.MissingCurrencies)
	require.NotNil(t, cfg.FXStatus.UpdatedAt)
}

func TestPaymentFXStatusUsesAdminManualRates(t *testing.T) {
	updatedAt := time.Now().UTC().Add(-5 * time.Minute)
	svc := &PaymentConfigService{}
	cfg := svc.parsePaymentConfig(map[string]string{
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND,CNY",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686,"CNY":0.139}`,
		SettingFXRatesSource:            "manual",
		SettingFXRatesUpdatedAt:         updatedAt.Format(time.RFC3339),
		SettingFXRatesStaleAfterSeconds: "3600",
	})

	require.Equal(t, "manual", cfg.FXStatus.Source)
	require.False(t, cfg.FXStatus.Stale)
	require.Empty(t, cfg.FXStatus.MissingCurrencies)
	require.InDelta(t, 0.000039215686, cfg.ManualFXRates["VND"], 0.000000000001)
	require.InDelta(t, 0.139, cfg.ManualFXRates["CNY"], 0.000000000001)
}

func TestValidatePaymentFXRatesReportsMissingAdminRates(t *testing.T) {
	rates, missing := validatePaymentFXRates("USD", []string{"USD", "VND", "KRW"}, map[string]float64{"USD": 1, "VND": 0.00004})

	require.Equal(t, map[string]float64{"USD": 1, "VND": 0.00004}, rates)
	require.Equal(t, []string{"KRW"}, missing)
}
