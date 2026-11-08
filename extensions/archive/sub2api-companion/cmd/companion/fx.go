package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type fxRateSnapshot struct {
	Rate        decimal.Decimal
	Source      string
	EffectiveAt time.Time
	FetchedAt   time.Time
	Stale       bool
}

func (a *app) ensureCurrentFXRate(now time.Time) (fxRateSnapshot, error) {
	hour := now.UTC().Truncate(a.cfg.FXRefreshInterval)
	if existing, ok := a.fxRateAtExactHour(hour); ok {
		return existing, nil
	}
	providers := []struct{ name, url string }{{"coinbase", a.cfg.FXPrimaryURL}, {"er-api", a.cfg.FXFallbackURL}}
	var failures []string
	for _, provider := range providers {
		if strings.TrimSpace(provider.url) == "" {
			continue
		}
		rate, err := a.fetchUSDCNY(provider.name, provider.url)
		if err != nil {
			failures = append(failures, provider.name+": "+err.Error())
			continue
		}
		snapshot := fxRateSnapshot{Rate: rate, Source: provider.name, EffectiveAt: hour, FetchedAt: now.UTC()}
		if err := a.insertFXRate(snapshot); err != nil {
			return fxRateSnapshot{}, err
		}
		return snapshot, nil
	}

	fallback := a.latestFXRate()
	if !fallback.Rate.IsPositive() {
		fallback = fxRateSnapshot{Rate: a.cfg.A6USDToCNY, Source: "bootstrap_fixed"}
	}
	fallback.Source += "_stale"
	fallback.EffectiveAt = hour
	fallback.FetchedAt = now.UTC()
	fallback.Stale = true
	if err := a.insertFXRate(fallback); err != nil {
		return fxRateSnapshot{}, err
	}
	return fallback, fmt.Errorf("all FX providers failed; stored stale fallback: %s", strings.Join(failures, "; "))
}

func (a *app) fetchUSDCNY(provider, endpoint string) (decimal.Decimal, error) {
	if a.fxHTTP == nil {
		a.fxHTTP = &http.Client{Timeout: a.cfg.FXHTTPTimeout}
	}
	resp, err := a.fxHTTP.Get(endpoint)
	if err != nil {
		return decimal.Zero, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return decimal.Zero, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var rateText string
	switch provider {
	case "coinbase":
		var payload struct {
			Data struct {
				Currency string            `json:"currency"`
				Rates    map[string]string `json:"rates"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return decimal.Zero, err
		}
		if payload.Data.Currency != "USD" {
			return decimal.Zero, fmt.Errorf("unexpected base %q", payload.Data.Currency)
		}
		rateText = payload.Data.Rates["CNY"]
	case "er-api":
		var payload struct {
			Result   string                 `json:"result"`
			BaseCode string                 `json:"base_code"`
			Rates    map[string]json.Number `json:"rates"`
		}
		decoder := json.NewDecoder(resp.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			return decimal.Zero, err
		}
		if payload.Result != "success" || payload.BaseCode != "USD" {
			return decimal.Zero, fmt.Errorf("unexpected response")
		}
		rateText = payload.Rates["CNY"].String()
	default:
		return decimal.Zero, fmt.Errorf("unsupported provider %q", provider)
	}
	rate, err := decimal.NewFromString(rateText)
	if err != nil || !rate.IsPositive() || rate.LessThan(decimal.NewFromInt(4)) || rate.GreaterThan(decimal.NewFromInt(12)) {
		return decimal.Zero, fmt.Errorf("invalid USD/CNY rate %q", rateText)
	}
	return rate, nil
}

func (a *app) insertFXRate(rate fxRateSnapshot) error {
	_, err := a.db.Exec(`INSERT OR IGNORE INTO fx_rates(base_currency,quote_currency,rate,effective_at,fetched_at,source,stale) VALUES('USD','CNY',?,?,?,?,?)`, rate.Rate.String(), rate.EffectiveAt.UTC().Format(time.RFC3339Nano), rate.FetchedAt.UTC().Format(time.RFC3339Nano), rate.Source, boolInt(rate.Stale))
	return err
}

func (a *app) fxRateAtExactHour(hour time.Time) (fxRateSnapshot, bool) {
	return a.scanFXRate(`SELECT rate,effective_at,fetched_at,source,stale FROM fx_rates WHERE base_currency='USD' AND quote_currency='CNY' AND effective_at=?`, hour.UTC().Format(time.RFC3339Nano))
}

func (a *app) latestFXRate() fxRateSnapshot {
	rate, _ := a.scanFXRate(`SELECT rate,effective_at,fetched_at,source,stale FROM fx_rates WHERE base_currency='USD' AND quote_currency='CNY' ORDER BY effective_at DESC LIMIT 1`)
	return rate
}

func (a *app) fxRateForTime(occurredAt time.Time) fxRateSnapshot {
	rate, ok := a.scanFXRate(`SELECT rate,effective_at,fetched_at,source,stale FROM fx_rates WHERE base_currency='USD' AND quote_currency='CNY' AND effective_at<=? ORDER BY effective_at DESC LIMIT 1`, occurredAt.UTC().Format(time.RFC3339Nano))
	if !ok {
		return fxRateSnapshot{Rate: a.cfg.A6USDToCNY, Source: "legacy_fixed", Stale: true}
	}
	if occurredAt.Sub(rate.EffectiveAt) > a.cfg.FXMaxStaleness {
		rate.Stale = true
	}
	return rate
}

func (a *app) scanFXRate(query string, args ...any) (fxRateSnapshot, bool) {
	var rateText, effectiveText, fetchedText string
	var rate fxRateSnapshot
	var stale int
	if err := a.db.QueryRow(query, args...).Scan(&rateText, &effectiveText, &fetchedText, &rate.Source, &stale); err != nil {
		return fxRateSnapshot{}, false
	}
	rate.Rate, _ = decimal.NewFromString(rateText)
	rate.EffectiveAt, _ = time.Parse(time.RFC3339Nano, effectiveText)
	rate.FetchedAt, _ = time.Parse(time.RFC3339Nano, fetchedText)
	rate.Stale = stale != 0
	return rate, rate.Rate.IsPositive()
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
