package service

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	DisplayBillingModeToken      = "token"
	DisplayBillingModePerRequest = "per_request"
	DisplayBillingModeImage      = "image"
	DisplayCurrencyCNY           = "CNY"
	DisplayCurrencyUSD           = "USD"
	DisplayOfficialPriceManual   = "manual"
)

var (
	ErrDisplayPriceNotFound       = infraerrors.NotFound("DISPLAY_PRICE_NOT_FOUND", "display price not found")
	ErrDisplayPriceInvalid        = infraerrors.BadRequest("DISPLAY_PRICE_INVALID", "invalid display price")
	ErrDisplayProviderInvalid     = infraerrors.BadRequest("DISPLAY_PROVIDER_INVALID", "invalid display pricing provider")
	ErrDisplayProviderNotFound    = infraerrors.NotFound("DISPLAY_PROVIDER_NOT_FOUND", "display pricing provider not found")
	ErrDisplayProviderExists      = infraerrors.Conflict("DISPLAY_PROVIDER_EXISTS", "display pricing provider already exists")
	ErrDisplayPricingInvalidValue = infraerrors.BadRequest("DISPLAY_PRICING_INVALID_VALUE", "display pricing values must be finite and non-negative")
)

var displayProviderKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

const (
	maxDisplayProviderNoteLength = 4000
	maxDisplayModelNoteLength    = 1000
)

// DisplayPricingSettings is the singleton presentation-only multiplier.
// Without a model override, effective = global * (provider override ?? 1).
// A model override is an absolute fixed multiplier and ignores both values.
// It is intentionally unrelated to Group.RateMultiplier and all billing services.
type DisplayPricingSettings struct {
	GlobalMultiplier float64   `json:"global_multiplier"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type DisplayPricingProvider struct {
	Provider       string    `json:"provider"`
	DisplayName    string    `json:"display_name"`
	ProviderNote   string    `json:"provider_note"`
	PerRequestNote string    `json:"per_request_note"`
	ImageNote      string    `json:"image_note"`
	Currency       string    `json:"currency"`
	Multiplier     *float64  `json:"multiplier"`
	LogoKey        string    `json:"logo_key"`
	LogoURL        string    `json:"logo_url"`
	SortOrder      int       `json:"sort_order"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type DisplayImagePrice struct {
	Label string  `json:"label"`
	Price float64 `json:"price"`
}

// DisplayModelPrice stores a price snapshot for presentation only.
// Token amounts are per one million tokens. Per-request prices are per request.
type DisplayModelPrice struct {
	ID          int64
	Platform    string
	ModelName   string
	Provider    string
	BillingMode string
	Currency    string
	Enabled     bool
	SortOrder   int
	ModelNote   string

	OfficialInputPerMillion      *float64
	OfficialOutputPerMillion     *float64
	OfficialCacheWritePerMillion *float64
	OfficialCacheReadPerMillion  *float64
	OfficialPriceSource          string
	OfficialPriceSourceURL       string
	OfficialPriceSyncedAt        *time.Time
	ModelMultiplier              *float64

	PerRequestLTE256K          *float64
	PerRequest256K512KOverride *float64
	PerRequestGT512KOverride   *float64
	ImagePrices                []DisplayImagePrice
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

type DisplayPricingRepository interface {
	GetSettings(ctx context.Context) (*DisplayPricingSettings, error)
	UpdateSettings(ctx context.Context, settings *DisplayPricingSettings) error
	ListProviders(ctx context.Context) ([]DisplayPricingProvider, error)
	CreateProvider(ctx context.Context, provider *DisplayPricingProvider) error
	UpdateProvider(ctx context.Context, provider *DisplayPricingProvider) error
	DeleteProvider(ctx context.Context, provider string) (int64, error)
	ListModels(ctx context.Context) ([]DisplayModelPrice, error)
	GetModel(ctx context.Context, id int64) (*DisplayModelPrice, error)
	UpsertModel(ctx context.Context, price *DisplayModelPrice) error
	UpdateModel(ctx context.Context, price *DisplayModelPrice) error
	DeleteModel(ctx context.Context, id int64) error
}

type DisplayPricingService struct {
	repo DisplayPricingRepository
}

func NewDisplayPricingService(repo DisplayPricingRepository) *DisplayPricingService {
	return &DisplayPricingService{repo: repo}
}

func (s *DisplayPricingService) GetSettings(ctx context.Context) (*DisplayPricingSettings, error) {
	return s.repo.GetSettings(ctx)
}

func (s *DisplayPricingService) UpdateSettings(ctx context.Context, multiplier float64) (*DisplayPricingSettings, error) {
	if !validPositive(multiplier) {
		return nil, ErrDisplayPricingInvalidValue
	}
	settings := &DisplayPricingSettings{GlobalMultiplier: multiplier}
	if err := s.repo.UpdateSettings(ctx, settings); err != nil {
		return nil, fmt.Errorf("update display pricing settings: %w", err)
	}
	return settings, nil
}

func (s *DisplayPricingService) ListProviders(ctx context.Context) ([]DisplayPricingProvider, error) {
	return s.repo.ListProviders(ctx)
}

func (s *DisplayPricingService) CreateProvider(ctx context.Context, provider DisplayPricingProvider) (*DisplayPricingProvider, error) {
	if err := normalizeAndValidateDisplayProvider(&provider); err != nil {
		return nil, err
	}
	if err := s.repo.CreateProvider(ctx, &provider); err != nil {
		return nil, fmt.Errorf("create display provider: %w", err)
	}
	return &provider, nil
}

func (s *DisplayPricingService) UpdateProvider(ctx context.Context, providerKey string, provider DisplayPricingProvider) (*DisplayPricingProvider, error) {
	provider.Provider = providerKey
	if err := normalizeAndValidateDisplayProvider(&provider); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateProvider(ctx, &provider); err != nil {
		return nil, fmt.Errorf("update display provider: %w", err)
	}
	return &provider, nil
}

// DeleteProvider removes only presentation data. The repository FK cascade is
// deliberately scoped to display_model_prices and never touches channels or billing.
func (s *DisplayPricingService) DeleteProvider(ctx context.Context, providerKey string) (int64, error) {
	providerKey = normalizeDisplayProvider(providerKey)
	if !displayProviderKeyPattern.MatchString(providerKey) {
		return 0, ErrDisplayProviderInvalid
	}
	deletedModels, err := s.repo.DeleteProvider(ctx, providerKey)
	if err != nil {
		return 0, fmt.Errorf("delete display provider: %w", err)
	}
	return deletedModels, nil
}

func (s *DisplayPricingService) ListModels(ctx context.Context) ([]DisplayModelPrice, error) {
	return s.repo.ListModels(ctx)
}

func (s *DisplayPricingService) UpsertModel(ctx context.Context, price DisplayModelPrice) (*DisplayModelPrice, error) {
	if err := normalizeAndValidateDisplayModelPrice(&price); err != nil {
		return nil, err
	}
	if err := s.validateDisplayModelProvider(ctx, &price); err != nil {
		return nil, err
	}
	if err := s.repo.UpsertModel(ctx, &price); err != nil {
		return nil, fmt.Errorf("upsert display model price: %w", err)
	}
	return &price, nil
}

func (s *DisplayPricingService) UpdateModel(ctx context.Context, id int64, price DisplayModelPrice) (*DisplayModelPrice, error) {
	if id <= 0 {
		return nil, ErrDisplayPriceNotFound
	}
	price.ID = id
	if err := normalizeAndValidateDisplayModelPrice(&price); err != nil {
		return nil, err
	}
	if err := s.validateDisplayModelProvider(ctx, &price); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateModel(ctx, &price); err != nil {
		return nil, fmt.Errorf("update display model price: %w", err)
	}
	return &price, nil
}

func (s *DisplayPricingService) DeleteModel(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrDisplayPriceNotFound
	}
	if err := s.repo.DeleteModel(ctx, id); err != nil {
		return fmt.Errorf("delete display model price: %w", err)
	}
	return nil
}

type DisplayOfficialPrices struct {
	InputPerMillion      *float64 `json:"input_per_million"`
	OutputPerMillion     *float64 `json:"output_per_million"`
	CacheWritePerMillion *float64 `json:"cache_write_per_million"`
	CacheReadPerMillion  *float64 `json:"cache_read_per_million"`
}

type DisplayTokenPrices struct {
	InputPerMillion      *float64 `json:"input_per_million"`
	OutputPerMillion     *float64 `json:"output_per_million"`
	CacheWritePerMillion *float64 `json:"cache_write_per_million"`
	CacheReadPerMillion  *float64 `json:"cache_read_per_million"`
}

type DisplayPerRequestPrices struct {
	LTE256K      float64 `json:"lte_256k"`
	From256K512K float64 `json:"from_256k_to_512k"`
	GT512K       float64 `json:"gt_512k"`
}

type DisplayCatalogModel struct {
	ID                  *int64                   `json:"id,omitempty"`
	Platform            string                   `json:"platform"`
	ModelName           string                   `json:"model_name"`
	ModelNote           string                   `json:"model_note"`
	BillingMode         string                   `json:"billing_mode"`
	Provider            string                   `json:"provider"`
	Currency            string                   `json:"currency"`
	Configured          bool                     `json:"configured"`
	Enabled             bool                     `json:"enabled"`
	OfficialPrices      *DisplayOfficialPrices   `json:"official_prices,omitempty"`
	ModelMultiplier     *float64                 `json:"model_multiplier,omitempty"`
	EffectiveMultiplier *float64                 `json:"effective_multiplier,omitempty"`
	DisplayPrices       *DisplayTokenPrices      `json:"display_prices,omitempty"`
	PerRequest          *DisplayPerRequestPrices `json:"per_request,omitempty"`
	ImageBasePrices     []DisplayImagePrice      `json:"image_base_prices,omitempty"`
	ImagePrices         []DisplayImagePrice      `json:"image_prices,omitempty"`
}

type DisplayCatalogProvider struct {
	Provider             string                `json:"provider"`
	DisplayName          string                `json:"display_name"`
	ProviderNote         string                `json:"provider_note"`
	PerRequestNote       string                `json:"per_request_note"`
	ImageNote            string                `json:"image_note"`
	Currency             string                `json:"currency"`
	LogoKey              string                `json:"logo_key"`
	LogoURL              string                `json:"logo_url"`
	ConfiguredMultiplier *float64              `json:"configured_multiplier,omitempty"`
	EffectiveMultiplier  float64               `json:"effective_multiplier"`
	Models               []DisplayCatalogModel `json:"models"`
	SortOrder            int                   `json:"-"`
}

type DisplayPricingCatalog struct {
	GlobalMultiplier float64                  `json:"global_multiplier"`
	UpdatedAt        time.Time                `json:"updated_at"`
	Providers        []DisplayCatalogProvider `json:"providers"`
}

type DiscoveredDisplayModel struct {
	Platform    string `json:"platform"`
	ModelName   string `json:"model_name"`
	BillingMode string `json:"billing_mode"`
	Provider    string `json:"provider"`
	Configured  bool   `json:"configured"`
}

// BuildCatalog intersects the presentation table with models dynamically discovered from
// currently active channels. Prices are sourced exclusively from display_model_prices.
func (s *DisplayPricingService) BuildCatalog(ctx context.Context, groups []PlazaGroup) (*DisplayPricingCatalog, error) {
	settings, err := s.repo.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get display pricing settings: %w", err)
	}
	providers, err := s.repo.ListProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list display pricing providers: %w", err)
	}
	prices, err := s.repo.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list display model prices: %w", err)
	}

	updatedAt := settings.UpdatedAt
	providerByKey := make(map[string]DisplayPricingProvider, len(providers))
	for i := range providers {
		providerByKey[providers[i].Provider] = providers[i]
		if providers[i].UpdatedAt.After(updatedAt) {
			updatedAt = providers[i].UpdatedAt
		}
	}
	priceByKey := make(map[string]*DisplayModelPrice, len(prices))
	for i := range prices {
		p := &prices[i]
		priceByKey[displayModelKey(p.Platform, p.ModelName, p.BillingMode)] = p
		if p.UpdatedAt.After(updatedAt) {
			updatedAt = p.UpdatedAt
		}
	}

	discovered := discoverDisplayModels(groups, priceByKey)
	byProvider := make(map[string]*DisplayCatalogProvider)
	for _, d := range discovered {
		p := priceByKey[displayModelKey(d.Platform, d.ModelName, d.BillingMode)]
		// Newly discovered models remain admin-only until an operator explicitly
		// configures a presentation price. This prevents publishing empty or guessed prices.
		if p == nil || !p.Enabled {
			continue
		}
		providerKey := d.Provider
		providerKey = p.Provider
		providerCfg, ok := providerByKey[providerKey]
		if !ok {
			providerCfg = defaultDisplayProvider(providerKey)
		}
		bucket := byProvider[providerKey]
		if bucket == nil {
			override := 1.0
			if providerCfg.Multiplier != nil {
				override = *providerCfg.Multiplier
			}
			bucket = &DisplayCatalogProvider{
				Provider: providerKey, DisplayName: providerCfg.DisplayName, Currency: providerCfg.Currency,
				ProviderNote:   providerCfg.ProviderNote,
				PerRequestNote: providerCfg.PerRequestNote,
				ImageNote:      providerCfg.ImageNote,
				LogoKey:        providerCfg.LogoKey, LogoURL: providerCfg.LogoURL,
				ConfiguredMultiplier: providerCfg.Multiplier, EffectiveMultiplier: settings.GlobalMultiplier * override,
				Models: []DisplayCatalogModel{}, SortOrder: providerCfg.SortOrder,
			}
			byProvider[providerKey] = bucket
		}
		model := buildDisplayCatalogModel(d, p, settings.GlobalMultiplier, providerCfg)
		bucket.Models = append(bucket.Models, model)
	}

	out := &DisplayPricingCatalog{GlobalMultiplier: settings.GlobalMultiplier, UpdatedAt: updatedAt, Providers: make([]DisplayCatalogProvider, 0, len(byProvider))}
	for _, provider := range byProvider {
		sort.SliceStable(provider.Models, func(i, j int) bool {
			return strings.ToLower(provider.Models[i].ModelName) < strings.ToLower(provider.Models[j].ModelName)
		})
		out.Providers = append(out.Providers, *provider)
	}
	sort.SliceStable(out.Providers, func(i, j int) bool {
		if out.Providers[i].SortOrder != out.Providers[j].SortOrder {
			return out.Providers[i].SortOrder < out.Providers[j].SortOrder
		}
		return out.Providers[i].DisplayName < out.Providers[j].DisplayName
	})
	return out, nil
}

func (s *DisplayPricingService) ListDiscovered(ctx context.Context, groups []PlazaGroup) ([]DiscoveredDisplayModel, error) {
	prices, err := s.repo.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	priceByKey := make(map[string]*DisplayModelPrice, len(prices))
	for i := range prices {
		priceByKey[displayModelKey(prices[i].Platform, prices[i].ModelName, prices[i].BillingMode)] = &prices[i]
	}
	return discoverDisplayModels(groups, priceByKey), nil
}

func discoverDisplayModels(groups []PlazaGroup, priceByKey map[string]*DisplayModelPrice) []DiscoveredDisplayModel {
	seen := make(map[string]struct{})
	out := make([]DiscoveredDisplayModel, 0)
	for i := range groups {
		for j := range groups[i].Models {
			m := groups[i].Models[j]
			mode := DisplayBillingModeToken
			if m.Pricing != nil && validDisplayBillingMode(string(m.Pricing.BillingMode)) {
				mode = string(m.Pricing.BillingMode)
			}
			key := displayModelKey(m.Platform, m.Name, mode)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			p := priceByKey[key]
			provider := inferDisplayProvider(m.Platform, m.Name)
			configured := p != nil
			if p != nil {
				provider = p.Provider
			}
			out = append(out, DiscoveredDisplayModel{Platform: m.Platform, ModelName: m.Name, BillingMode: mode, Provider: provider, Configured: configured})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return strings.ToLower(out[i].ModelName) < strings.ToLower(out[j].ModelName)
	})
	return out
}

func buildDisplayCatalogModel(d DiscoveredDisplayModel, p *DisplayModelPrice, global float64, provider DisplayPricingProvider) DisplayCatalogModel {
	currency := provider.Currency
	model := DisplayCatalogModel{Platform: d.Platform, ModelName: d.ModelName, BillingMode: d.BillingMode, Provider: d.Provider, Currency: currency, Configured: p != nil, Enabled: p != nil && p.Enabled}
	if p == nil {
		return model
	}
	model.ID = &p.ID
	model.ModelNote = p.ModelNote
	model.Provider = p.Provider
	model.Currency = p.Currency
	switch p.BillingMode {
	case DisplayBillingModePerRequest:
		if p.PerRequestLTE256K != nil {
			mid := *p.PerRequestLTE256K * 1.5
			high := *p.PerRequestLTE256K * 2
			if p.PerRequest256K512KOverride != nil {
				mid = *p.PerRequest256K512KOverride
			}
			if p.PerRequestGT512KOverride != nil {
				high = *p.PerRequestGT512KOverride
			}
			model.PerRequest = &DisplayPerRequestPrices{LTE256K: *p.PerRequestLTE256K, From256K512K: mid, GT512K: high}
		}
	case DisplayBillingModeToken, DisplayBillingModeImage:
		effective := global
		if p.ModelMultiplier != nil {
			effective = *p.ModelMultiplier
		} else if provider.Multiplier != nil {
			effective *= *provider.Multiplier
		}
		model.ModelMultiplier = p.ModelMultiplier
		model.EffectiveMultiplier = displayFloat64Ptr(effective)
		if p.BillingMode == DisplayBillingModeToken {
			model.OfficialPrices = &DisplayOfficialPrices{p.OfficialInputPerMillion, p.OfficialOutputPerMillion, p.OfficialCacheWritePerMillion, p.OfficialCacheReadPerMillion}
			model.DisplayPrices = &DisplayTokenPrices{multiplyFloatPtr(p.OfficialInputPerMillion, effective), multiplyFloatPtr(p.OfficialOutputPerMillion, effective), multiplyFloatPtr(p.OfficialCacheWritePerMillion, effective), multiplyFloatPtr(p.OfficialCacheReadPerMillion, effective)}
		} else {
			model.ImageBasePrices = cloneImagePrices(p.ImagePrices)
			model.ImagePrices = make([]DisplayImagePrice, 0, len(p.ImagePrices))
			for _, tier := range p.ImagePrices {
				model.ImagePrices = append(model.ImagePrices, DisplayImagePrice{Label: tier.Label, Price: tier.Price * effective})
			}
		}
	}
	return model
}

func normalizeAndValidateDisplayModelPrice(p *DisplayModelPrice) error {
	p.Platform = strings.ToLower(strings.TrimSpace(p.Platform))
	p.ModelName = strings.TrimSpace(p.ModelName)
	p.Provider = normalizeDisplayProvider(p.Provider)
	p.BillingMode = strings.ToLower(strings.TrimSpace(p.BillingMode))
	p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
	p.ModelNote = strings.TrimSpace(p.ModelNote)
	p.OfficialPriceSource = strings.ToLower(strings.TrimSpace(p.OfficialPriceSource))
	p.OfficialPriceSourceURL = strings.TrimSpace(p.OfficialPriceSourceURL)
	if p.OfficialPriceSource == "" {
		p.OfficialPriceSource = DisplayOfficialPriceManual
	}
	if p.Platform == "" || p.ModelName == "" || p.Provider == "" || !validDisplayBillingMode(p.BillingMode) || !validDisplayCurrency(p.Currency) {
		return ErrDisplayPriceInvalid
	}
	if !displayProviderKeyPattern.MatchString(p.OfficialPriceSource) || !validDisplayOfficialSourceURL(p.OfficialPriceSourceURL) {
		return ErrDisplayPriceInvalid
	}
	if len([]rune(p.ModelNote)) > maxDisplayModelNoteLength {
		return ErrDisplayPriceInvalid
	}
	values := []*float64{p.OfficialInputPerMillion, p.OfficialOutputPerMillion, p.OfficialCacheWritePerMillion, p.OfficialCacheReadPerMillion, p.PerRequestLTE256K, p.PerRequest256K512KOverride, p.PerRequestGT512KOverride}
	for _, v := range values {
		if v != nil && !validNonNegative(*v) {
			return ErrDisplayPricingInvalidValue
		}
	}
	if p.ModelMultiplier != nil && !validPositive(*p.ModelMultiplier) {
		return ErrDisplayPricingInvalidValue
	}
	for i := range p.ImagePrices {
		p.ImagePrices[i].Label = strings.TrimSpace(p.ImagePrices[i].Label)
		if p.ImagePrices[i].Label == "" || !validNonNegative(p.ImagePrices[i].Price) {
			return ErrDisplayPricingInvalidValue
		}
	}
	switch p.BillingMode {
	case DisplayBillingModeToken:
		p.PerRequestLTE256K, p.PerRequest256K512KOverride, p.PerRequestGT512KOverride = nil, nil, nil
		p.ImagePrices = nil
	case DisplayBillingModePerRequest:
		if p.PerRequestLTE256K == nil {
			return ErrDisplayPriceInvalid
		}
		p.OfficialInputPerMillion, p.OfficialOutputPerMillion, p.OfficialCacheWritePerMillion, p.OfficialCacheReadPerMillion = nil, nil, nil, nil
		p.ModelMultiplier = nil
		p.OfficialPriceSource = DisplayOfficialPriceManual
		p.OfficialPriceSourceURL = ""
		p.OfficialPriceSyncedAt = nil
		p.ImagePrices = nil
	case DisplayBillingModeImage:
		if len(p.ImagePrices) == 0 {
			return ErrDisplayPriceInvalid
		}
		p.OfficialInputPerMillion, p.OfficialOutputPerMillion, p.OfficialCacheWritePerMillion, p.OfficialCacheReadPerMillion = nil, nil, nil, nil
		p.PerRequestLTE256K, p.PerRequest256K512KOverride, p.PerRequestGT512KOverride = nil, nil, nil
		p.OfficialPriceSource = DisplayOfficialPriceManual
		p.OfficialPriceSourceURL = ""
		p.OfficialPriceSyncedAt = nil
	}
	return nil
}

func validDisplayOfficialSourceURL(raw string) bool {
	if raw == "" {
		return true
	}
	if len(raw) > 2048 {
		return false
	}
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func normalizeAndValidateDisplayProvider(p *DisplayPricingProvider) error {
	p.Provider = normalizeDisplayProvider(p.Provider)
	p.DisplayName = strings.TrimSpace(p.DisplayName)
	p.ProviderNote = strings.TrimSpace(p.ProviderNote)
	p.PerRequestNote = strings.TrimSpace(p.PerRequestNote)
	p.ImageNote = strings.TrimSpace(p.ImageNote)
	p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
	p.LogoKey = strings.ToLower(strings.TrimSpace(p.LogoKey))
	p.LogoURL = strings.TrimSpace(p.LogoURL)
	if !displayProviderKeyPattern.MatchString(p.Provider) || p.DisplayName == "" ||
		len([]rune(p.ProviderNote)) > maxDisplayProviderNoteLength ||
		len([]rune(p.PerRequestNote)) > maxDisplayProviderNoteLength ||
		len([]rune(p.ImageNote)) > maxDisplayProviderNoteLength ||
		!validDisplayCurrency(p.Currency) ||
		(p.Multiplier != nil && !validPositive(*p.Multiplier)) ||
		(p.LogoKey != "" && !displayProviderKeyPattern.MatchString(p.LogoKey)) || !validDisplayLogoURL(p.LogoURL) {
		return ErrDisplayProviderInvalid
	}
	return nil
}

func (s *DisplayPricingService) validateDisplayModelProvider(ctx context.Context, price *DisplayModelPrice) error {
	providers, err := s.repo.ListProviders(ctx)
	if err != nil {
		return fmt.Errorf("list display providers for model validation: %w", err)
	}
	for i := range providers {
		if normalizeDisplayProvider(providers[i].Provider) != price.Provider {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(providers[i].Currency)) != price.Currency {
			return ErrDisplayPriceInvalid
		}
		return nil
	}
	return ErrDisplayProviderNotFound
}

func validDisplayLogoURL(value string) bool {
	if value == "" {
		return true
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f || r == '\\' {
			return false
		}
	}
	if strings.HasPrefix(value, "https://") {
		u, err := url.Parse(value)
		return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && safeDisplayLogoPath(u.EscapedPath())
	}
	if strings.HasPrefix(value, "//") {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.IsAbs() || u.Host != "" || u.User != nil || u.Path == "" {
		return false
	}
	return safeDisplayLogoPath(u.EscapedPath())
}

func safeDisplayLogoPath(escapedPath string) bool {
	decoded, err := url.PathUnescape(escapedPath)
	if err != nil {
		return false
	}
	for _, r := range decoded {
		if r < 0x20 || r == 0x7f || r == '\\' {
			return false
		}
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == ".." {
			return false
		}
	}
	return true
}

func displayModelKey(platform, name, mode string) string {
	return strings.ToLower(strings.TrimSpace(platform)) + "\x00" + strings.ToLower(strings.TrimSpace(name)) + "\x00" + mode
}
func validDisplayBillingMode(v string) bool {
	return v == DisplayBillingModeToken || v == DisplayBillingModePerRequest || v == DisplayBillingModeImage
}
func validDisplayCurrency(v string) bool   { return v == DisplayCurrencyCNY || v == DisplayCurrencyUSD }
func validPositive(v float64) bool         { return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0 }
func validNonNegative(v float64) bool      { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }
func displayFloat64Ptr(v float64) *float64 { return &v }
func multiplyFloatPtr(v *float64, multiplier float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v * multiplier
	return &out
}
func cloneImagePrices(in []DisplayImagePrice) []DisplayImagePrice {
	if len(in) == 0 {
		return nil
	}
	return append([]DisplayImagePrice(nil), in...)
}

func normalizeDisplayProvider(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

func defaultDisplayProvider(key string) DisplayPricingProvider {
	currency := DisplayCurrencyUSD
	if isDomesticDisplayProvider(key) {
		currency = DisplayCurrencyCNY
	}
	return DisplayPricingProvider{Provider: key, DisplayName: displayProviderName(key), Currency: currency, LogoKey: key, SortOrder: 1000}
}

func inferDisplayProvider(platform, model string) string {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "deepseek"):
		return "deepseek"
	case strings.Contains(lower, "glm") || strings.Contains(lower, "chatglm"):
		return "zhipu"
	case strings.Contains(lower, "kimi") || strings.Contains(lower, "moonshot"):
		return "moonshot"
	case strings.Contains(lower, "minimax"):
		return "minimax"
	case strings.Contains(lower, "qwen") || strings.Contains(lower, "qwq"):
		return "qwen"
	case strings.Contains(lower, "mimo"):
		return "mimo"
	case strings.Contains(lower, "hunyuan") || lower == "hy3":
		return "hunyuan"
	case strings.Contains(lower, "claude"):
		return "anthropic"
	case strings.Contains(lower, "gemini") || strings.Contains(lower, "imagen"):
		return "gemini"
	case strings.Contains(lower, "grok"):
		return "grok"
	case strings.HasPrefix(lower, "auto"):
		return "auto"
	}
	switch strings.ToLower(platform) {
	case "anthropic":
		return "anthropic"
	case "gemini", "antigravity":
		return "gemini"
	case "grok":
		return "grok"
	case "kimi":
		return "moonshot"
	case "zhipu", "deepseek":
		return strings.ToLower(platform)
	default:
		return "openai"
	}
}

func isDomesticDisplayProvider(v string) bool {
	switch v {
	case "auto", "deepseek", "zhipu", "moonshot", "minimax", "qwen", "mimo", "hunyuan":
		return true
	default:
		return false
	}
}

func displayProviderName(v string) string {
	switch v {
	case "zhipu":
		return "GLM"
	case "moonshot":
		return "Kimi"
	case "hunyuan":
		return "Hunyuan"
	case "mimo":
		return "MiMo"
	case "grok":
		return "Grok"
	case "openai":
		return "OpenAI"
	case "anthropic":
		return "Anthropic"
	case "gemini":
		return "Gemini"
	case "qwen":
		return "Qwen"
	case "deepseek":
		return "DeepSeek"
	case "minimax":
		return "MiniMax"
	case "auto":
		return "Auto"
	default:
		if v == "" {
			return "Other"
		}
		return strings.ToUpper(v[:1]) + v[1:]
	}
}
