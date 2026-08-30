package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	OfficialPriceSourceHerohaoAggregate = "herohao_aggregate"
	OfficialPriceSourceConfidence       = "unverified"
	HerohaoOfficialPriceCandidateURL    = "https://sub2.herohao.top/pricing/api/pricing"

	maxOfficialPriceSourceBytes = 4 << 20
)

type OfficialPriceCandidate struct {
	ModelName   string
	ProviderKey string
	Currency    string
	Enabled     bool
	UpdatedAt   *time.Time
	Input       *decimal.Decimal
	Output      *decimal.Decimal
	CacheWrite  *decimal.Decimal
	CacheRead   *decimal.Decimal
}

type OfficialPriceSourceSnapshot struct {
	FetchedAt time.Time
	UpdatedAt *time.Time
	Warning   *string
	Models    map[string]OfficialPriceCandidate
}

type OfficialPriceCandidateFetcher interface {
	Fetch(context.Context) (*OfficialPriceSourceSnapshot, error)
}

type herohaoOfficialPriceFetcher struct {
	client *http.Client
}

func NewHerohaoOfficialPriceFetcher(client *http.Client) OfficialPriceCandidateFetcher {
	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &herohaoOfficialPriceFetcher{client: client}
}

func (f *herohaoOfficialPriceFetcher) Fetch(ctx context.Context) (*OfficialPriceSourceSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, HerohaoOfficialPriceCandidateURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create official price candidate request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "sub2api-official-price-sync/1")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch official price candidates: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch official price candidates: unexpected HTTP status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOfficialPriceSourceBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read official price candidates: %w", err)
	}
	if len(body) > maxOfficialPriceSourceBytes {
		return nil, fmt.Errorf("official price candidate response exceeds %d bytes", maxOfficialPriceSourceBytes)
	}
	return parseHerohaoOfficialPriceSnapshot(body)
}

type herohaoPriceResponse struct {
	Currency  string    `json:"currency"`
	FetchedAt time.Time `json:"fetchedAt"`
	Warning   *string   `json:"warning"`
	Token     struct {
		Source struct {
			UpdatedAt *time.Time `json:"updatedAt"`
		} `json:"source"`
		DatabaseUpdatedAt *time.Time          `json:"databaseUpdatedAt"`
		Models            []herohaoPriceModel `json:"models"`
	} `json:"token"`
}

type herohaoPriceModel struct {
	Model       string     `json:"model"`
	ProviderKey string     `json:"providerKey"`
	UpdatedAt   *time.Time `json:"updatedAt"`
	Enabled     bool       `json:"enabled"`
	Prices      struct {
		Input      herohaoPriceComponent `json:"input"`
		Output     herohaoPriceComponent `json:"output"`
		CacheWrite herohaoPriceComponent `json:"cacheWrite"`
		CacheRead  herohaoPriceComponent `json:"cacheRead"`
	} `json:"prices"`
}

type herohaoPriceComponent struct {
	Official json.RawMessage `json:"official"`
}

func parseHerohaoOfficialPriceSnapshot(body []byte) (*OfficialPriceSourceSnapshot, error) {
	var payload herohaoPriceResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse official price candidate response: %w", err)
	}
	currency := strings.ToUpper(strings.TrimSpace(payload.Currency))
	if currency != DisplayCurrencyCNY {
		return nil, fmt.Errorf("official price candidate source returned unsupported currency %q", payload.Currency)
	}
	if payload.FetchedAt.IsZero() {
		return nil, fmt.Errorf("official price candidate source omitted fetchedAt")
	}

	updatedAt := payload.Token.Source.UpdatedAt
	if updatedAt == nil {
		updatedAt = payload.Token.DatabaseUpdatedAt
	}
	out := &OfficialPriceSourceSnapshot{
		FetchedAt: payload.FetchedAt,
		UpdatedAt: updatedAt,
		Warning:   payload.Warning,
		Models:    make(map[string]OfficialPriceCandidate, len(payload.Token.Models)),
	}
	for i := range payload.Token.Models {
		model := payload.Token.Models[i]
		name := strings.TrimSpace(model.Model)
		if name == "" {
			return nil, fmt.Errorf("official price candidate source contains an empty model name")
		}
		if _, exists := out.Models[name]; exists {
			return nil, fmt.Errorf("official price candidate source contains duplicate model %q", name)
		}
		input, err := parseHerohaoOfficialDecimal(model.Prices.Input.Official)
		if err != nil {
			return nil, fmt.Errorf("parse %s input official price: %w", name, err)
		}
		output, err := parseHerohaoOfficialDecimal(model.Prices.Output.Official)
		if err != nil {
			return nil, fmt.Errorf("parse %s output official price: %w", name, err)
		}
		cacheWrite, err := parseHerohaoOfficialDecimal(model.Prices.CacheWrite.Official)
		if err != nil {
			return nil, fmt.Errorf("parse %s cache-write official price: %w", name, err)
		}
		cacheRead, err := parseHerohaoOfficialDecimal(model.Prices.CacheRead.Official)
		if err != nil {
			return nil, fmt.Errorf("parse %s cache-read official price: %w", name, err)
		}
		if input == nil && output == nil && cacheWrite == nil && cacheRead == nil {
			continue
		}
		out.Models[name] = OfficialPriceCandidate{
			ModelName: name, ProviderKey: strings.ToLower(strings.TrimSpace(model.ProviderKey)),
			Currency: currency, Enabled: model.Enabled, UpdatedAt: model.UpdatedAt,
			Input: input, Output: output, CacheWrite: cacheWrite, CacheRead: cacheRead,
		}
	}
	return out, nil
}

func parseHerohaoOfficialDecimal(raw json.RawMessage) (*decimal.Decimal, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, err
		}
		trimmed = strings.TrimSpace(text)
	}
	value, err := decimal.NewFromString(trimmed)
	if err != nil {
		return nil, err
	}
	if value.IsNegative() {
		return nil, fmt.Errorf("price must be non-negative")
	}
	value = value.Round(8)
	return &value, nil
}
