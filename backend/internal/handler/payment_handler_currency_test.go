//go:build unit

package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestPaymentCheckoutInfoReturnsCurrencyMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", "file:payment_handler_currency_checkout?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	settings := &paymentHandlerCurrencySettingRepo{values: map[string]string{
		service.SettingPaymentEnabled:           "true",
		service.SettingLedgerCurrency:           "USD",
		service.SettingAllowedPaymentCurrencies: "USD,CNY,VND,KRW",
		service.SettingManualFXRates:            `{"USD":1,"CNY":0.139,"VND":0.000039215686,"KRW":0.00073}`,
		service.SettingFXRatesSource:            "manual",
		service.SettingFXRatesUpdatedAt:         time.Now().UTC().Format(time.RFC3339),
	}}
	configSvc := service.NewPaymentConfigService(client, settings, []byte("0123456789abcdef0123456789abcdef"))
	h := NewPaymentHandler(nil, configSvc, nil)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/checkout-info", nil)

	h.GetCheckoutInfo(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			LedgerCurrency           string             `json:"ledger_currency"`
			AllowedPaymentCurrencies []string           `json:"allowed_payment_currencies"`
			ManualFXRates            map[string]float64 `json:"manual_fx_rates"`
			FXStatus                 struct {
				Source            string   `json:"source"`
				UpdatedAt         string   `json:"updated_at"`
				Stale             bool     `json:"stale"`
				MissingCurrencies []string `json:"missing_currencies"`
			} `json:"fx_status"`
			CurrencyMeta map[string]struct {
				MinorUnits int    `json:"minor_units"`
				Symbol     string `json:"symbol"`
			} `json:"currency_meta"`
			StripePublishableKey string `json:"stripe_publishable_key"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "USD", resp.Data.LedgerCurrency)
	require.Equal(t, []string{"USD", "CNY", "VND", "KRW"}, resp.Data.AllowedPaymentCurrencies)
	require.Equal(t, 1.0, resp.Data.ManualFXRates["USD"])
	require.InDelta(t, 0.000039215686, resp.Data.ManualFXRates["VND"], 0.000000000001)
	require.Equal(t, "manual", resp.Data.FXStatus.Source)
	require.NotEmpty(t, resp.Data.FXStatus.UpdatedAt)
	require.False(t, resp.Data.FXStatus.Stale)
	require.Empty(t, resp.Data.FXStatus.MissingCurrencies)
	require.Equal(t, 0, resp.Data.CurrencyMeta["VND"].MinorUnits)
	require.Equal(t, "₫", resp.Data.CurrencyMeta["VND"].Symbol)
	require.Equal(t, 0, resp.Data.CurrencyMeta["KRW"].MinorUnits)
	require.Equal(t, "₩", resp.Data.CurrencyMeta["KRW"].Symbol)
	require.Equal(t, 2, resp.Data.CurrencyMeta["CNY"].MinorUnits)
	require.Equal(t, "¥", resp.Data.CurrencyMeta["CNY"].Symbol)
	require.Empty(t, resp.Data.StripePublishableKey)
}

func TestCreateOrderRequestBindsCurrencyAmountMode(t *testing.T) {
	var req CreateOrderRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"amount":255000,
		"amount_mode":"payment",
		"payment_currency":"VND",
		"payment_type":"sepay",
		"order_type":"balance"
	}`), &req))

	require.Equal(t, 255000.0, req.Amount)
	require.Equal(t, service.PaymentAmountModePayment, req.AmountMode)
	require.Equal(t, "VND", req.PaymentCurrency)
	require.Equal(t, payment.TypeSepay, req.PaymentType)
	require.Equal(t, payment.OrderTypeBalance, req.OrderType)
}

func TestPaymentQuoteEndpointReturnsSignedSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", "file:payment_handler_currency_quote?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	settings := &paymentHandlerCurrencySettingRepo{values: map[string]string{
		service.SettingPaymentEnabled:           "true",
		service.SettingLedgerCurrency:           "USD",
		service.SettingAllowedPaymentCurrencies: "USD,VND,CNY",
		service.SettingManualFXRates:            `{"USD":1,"VND":0.000039215686,"CNY":0.139}`,
		service.SettingMinRechargeAmount:        "1",
		service.SettingMaxRechargeAmount:        "1000",
	}}
	configSvc := service.NewPaymentConfigService(client, settings, []byte("0123456789abcdef0123456789abcdef"))
	paymentSvc := service.NewPaymentService(nil, nil, nil, nil, nil, configSvc, nil, nil, nil)
	h := NewPaymentHandler(paymentSvc, configSvc, nil)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/payment/quote", bytes.NewBufferString(`{
		"amount":255000,
		"amount_mode":"payment",
		"payment_currency":"VND",
		"payment_type":"sepay",
		"order_type":"balance"
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.CreatePaymentQuote(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			QuoteID         string  `json:"quote_id"`
			AmountMode      string  `json:"amount_mode"`
			PaymentCurrency string  `json:"payment_currency"`
			LedgerCurrency  string  `json:"ledger_currency"`
			PaymentAmount   float64 `json:"payment_amount"`
			LedgerAmount    float64 `json:"ledger_amount"`
			FXRate          float64 `json:"fx_rate"`
			FXSource        string  `json:"fx_source"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.NotEmpty(t, resp.Data.QuoteID)
	require.Equal(t, service.PaymentAmountModePayment, resp.Data.AmountMode)
	require.Equal(t, "VND", resp.Data.PaymentCurrency)
	require.Equal(t, "USD", resp.Data.LedgerCurrency)
	require.Equal(t, 255000.0, resp.Data.PaymentAmount)
	require.InDelta(t, 10.0, resp.Data.LedgerAmount, 0.01)
	require.InDelta(t, 0.000039215686, resp.Data.FXRate, 0.000000000001)
	require.Equal(t, "manual", resp.Data.FXSource)
}

type paymentHandlerCurrencySettingRepo struct {
	values map[string]string
}

func (s *paymentHandlerCurrencySettingRepo) Get(_ context.Context, key string) (*service.Setting, error) {
	return &service.Setting{Key: key, Value: s.values[key]}, nil
}

func (s *paymentHandlerCurrencySettingRepo) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s *paymentHandlerCurrencySettingRepo) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func (s *paymentHandlerCurrencySettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = s.values[key]
	}
	return result, nil
}

func (s *paymentHandlerCurrencySettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *paymentHandlerCurrencySettingRepo) GetAll(context.Context) (map[string]string, error) {
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}

func (s *paymentHandlerCurrencySettingRepo) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}
