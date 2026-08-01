package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	PaymentFXProviderOpenExchangeRates = "openexchangerates"
	defaultOpenExchangeRatesBaseURL    = "https://openexchangerates.org/api/latest.json"
)

type OpenExchangeRatesProviderConfig struct {
	AppID          string
	BaseURL        string
	TimeoutSeconds int
}

type OpenExchangeRatesProvider struct {
	appID   string
	baseURL string
	client  *http.Client
}

func NewOpenExchangeRatesProvider(cfg OpenExchangeRatesProviderConfig) *OpenExchangeRatesProvider {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultOpenExchangeRatesBaseURL
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &OpenExchangeRatesProvider{
		appID:   strings.TrimSpace(cfg.AppID),
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

type openExchangeRatesLatestResponse struct {
	Base        string             `json:"base"`
	Timestamp   int64              `json:"timestamp"`
	Rates       map[string]float64 `json:"rates"`
	Error       bool               `json:"error"`
	Status      int                `json:"status"`
	Message     string             `json:"message"`
	Description string             `json:"description"`
}

func (p *OpenExchangeRatesProvider) FetchPaymentFXRates(ctx context.Context, req PaymentFXRateRequest) (PaymentFXRateSnapshot, error) {
	if p == nil {
		return PaymentFXRateSnapshot{}, fmt.Errorf("open exchange rates provider is not configured")
	}
	if p.appID == "" {
		return PaymentFXRateSnapshot{}, fmt.Errorf("open exchange rates app id is required")
	}

	endpoint, err := url.Parse(p.baseURL)
	if err != nil {
		return PaymentFXRateSnapshot{}, fmt.Errorf("open exchange rates base url invalid: %w", err)
	}
	q := endpoint.Query()
	q.Set("app_id", p.appID)
	q.Set("symbols", strings.Join(openExchangeRatesSymbols(req), ","))
	endpoint.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return PaymentFXRateSnapshot{}, err
	}
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return PaymentFXRateSnapshot{}, fmt.Errorf("open exchange rates request failed: %w", err)
	}
	defer resp.Body.Close()

	var payload openExchangeRatesLatestResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return PaymentFXRateSnapshot{}, fmt.Errorf("open exchange rates response decode failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || payload.Error {
		msg := strings.TrimSpace(payload.Description)
		if msg == "" {
			msg = strings.TrimSpace(payload.Message)
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return PaymentFXRateSnapshot{}, fmt.Errorf("open exchange rates returned status %d: %s", resp.StatusCode, msg)
	}

	rates, err := openExchangeRatesToPaymentRates(req, payload)
	if err != nil {
		return PaymentFXRateSnapshot{}, err
	}
	timestamp := time.Now().UTC()
	if payload.Timestamp > 0 {
		timestamp = time.Unix(payload.Timestamp, 0).UTC()
	}
	return PaymentFXRateSnapshot{
		Rates:     rates,
		Source:    PaymentFXProviderOpenExchangeRates,
		Timestamp: timestamp,
	}, nil
}

func openExchangeRatesSymbols(req PaymentFXRateRequest) []string {
	set := map[string]struct{}{}
	add := func(currency string) {
		currency = normalizeCurrencyCode(currency, "")
		if currency != "" {
			set[currency] = struct{}{}
		}
	}
	add(req.LedgerCurrency)
	for _, currency := range req.PaymentCurrencies {
		add(currency)
	}
	keys := make([]string, 0, len(set))
	for currency := range set {
		keys = append(keys, currency)
	}
	sort.Strings(keys)
	return keys
}

func openExchangeRatesToPaymentRates(req PaymentFXRateRequest, payload openExchangeRatesLatestResponse) (map[string]float64, error) {
	base := normalizeCurrencyCode(payload.Base, defaultLedgerCurrency)
	if payload.Rates == nil {
		return nil, fmt.Errorf("open exchange rates response has no rates")
	}

	unitsPerBase := func(currency string) (float64, bool) {
		currency = normalizeCurrencyCode(currency, "")
		if currency == "" {
			return 0, false
		}
		if currency == base {
			return 1, true
		}
		rate, ok := payload.Rates[currency]
		if !ok || math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
			return 0, false
		}
		return rate, true
	}

	ledgerCurrency := normalizeCurrencyCode(req.LedgerCurrency, defaultLedgerCurrency)
	ledgerPerBase, ok := unitsPerBase(ledgerCurrency)
	if !ok {
		return nil, fmt.Errorf("open exchange rates missing ledger currency %s", ledgerCurrency)
	}

	out := map[string]float64{ledgerCurrency: 1}
	for _, currency := range paymentFXSyncCurrencies(ledgerCurrency, req.PaymentCurrencies) {
		if currency == ledgerCurrency {
			out[currency] = 1
			continue
		}
		paymentPerBase, ok := unitsPerBase(currency)
		if !ok {
			return nil, fmt.Errorf("open exchange rates missing payment currency %s", currency)
		}
		out[currency] = ledgerPerBase / paymentPerBase
	}
	return out, nil
}
