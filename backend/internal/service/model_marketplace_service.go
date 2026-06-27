package service

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

const (
	ModelMarketplaceBillingModeToken = "token"
	ModelMarketplaceBillingModeImage = "image"
)

const modelMarketplaceFormulaNote = "InputCost = input_tokens * input_price; OutputCost = output_tokens * output_price; CacheCreationCost = cache_write_tokens * cache_write_price; CacheReadCost = cache_read_tokens * cache_read_price; ImageOutputCost = image_output_tokens * image_output_price; TotalCost = sum; ActualCost = TotalCost * rate_multiplier"

type ModelMarketplaceService struct {
	pricingService *PricingService
	billingService *BillingService
	apiKeyService  *APIKeyService
	gatewayService *GatewayService
}

func NewModelMarketplaceService(pricingService *PricingService, billingService *BillingService, apiKeyService *APIKeyService, gatewayService *GatewayService) *ModelMarketplaceService {
	return &ModelMarketplaceService{
		pricingService: pricingService,
		billingService: billingService,
		apiKeyService:  apiKeyService,
		gatewayService: gatewayService,
	}
}

type ModelMarketplaceListRequest struct {
	Query       string
	Provider    string
	Mode        string
	BillingMode string
	Endpoint    string
	GroupID     *int64
	ServiceTier string
	Unit        string
	Page        int
	PageSize    int
}

type ModelMarketplaceResponse struct {
	Items      []ModelMarketplaceItem       `json:"items"`
	Facets     ModelMarketplaceFacets       `json:"facets"`
	Pagination ModelMarketplacePagination   `json:"pagination"`
	Catalog    ModelMarketplaceCatalogState `json:"catalog"`
}

type ModelMarketplaceItem struct {
	Model              string                   `json:"model"`
	Provider           string                   `json:"provider"`
	ProviderLabel      string                   `json:"provider_label"`
	ProviderIcon       string                   `json:"provider_icon"`
	Family             string                   `json:"family"`
	Mode               string                   `json:"mode"`
	BillingMode        string                   `json:"billing_mode"`
	SupportedEndpoints []string                 `json:"supported_endpoints"`
	Features           ModelMarketplaceFeatures `json:"features"`
	Context            ModelMarketplaceContext  `json:"context"`
	Pricing            ModelMarketplacePricing  `json:"pricing"`
	CalculationNote    string                   `json:"calculation_note"`
	Tags               []string                 `json:"tags"`
}

type ModelMarketplaceFeatures struct {
	PromptCaching bool `json:"prompt_caching"`
	ServiceTier   bool `json:"service_tier"`
	Vision        bool `json:"vision"`
	Reasoning     bool `json:"reasoning"`
	WebSearch     bool `json:"web_search"`
	AudioOutput   bool `json:"audio_output"`
}

type ModelMarketplaceContext struct {
	MaxInputTokens  int `json:"max_input_tokens,omitempty"`
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
}

type ModelMarketplacePricing struct {
	Source         string                           `json:"source"`
	CatalogUnit    string                           `json:"catalog_unit"`
	DisplayUnit    string                           `json:"display_unit"`
	RateMultiplier float64                          `json:"rate_multiplier"`
	GroupID        *int64                           `json:"group_id,omitempty"`
	GroupName      string                           `json:"group_name,omitempty"`
	ServiceTier    string                           `json:"service_tier"`
	Input          ModelMarketplacePricePart        `json:"input"`
	Output         ModelMarketplacePricePart        `json:"output"`
	CacheRead      ModelMarketplacePricePart        `json:"cache_read"`
	CacheWrite     ModelMarketplacePricePart        `json:"cache_write"`
	CacheWrite5m   ModelMarketplacePricePart        `json:"cache_write_5m"`
	CacheWrite1h   ModelMarketplacePricePart        `json:"cache_write_1h"`
	ImageOutput    ModelMarketplacePricePart        `json:"image_output"`
	PerRequest     *ModelMarketplacePerRequestPrice `json:"per_request"`
	LongContext    *ModelMarketplaceLongContext     `json:"long_context,omitempty"`
}

type ModelMarketplacePricePart struct {
	PerToken   float64 `json:"per_token"`
	Per1K      float64 `json:"per_1k"`
	Per1M      float64 `json:"per_1m"`
	DisplayUSD float64 `json:"display_usd"`
}

type ModelMarketplacePerRequestPrice struct {
	UnitPrice  float64 `json:"unit_price"`
	DisplayUSD float64 `json:"display_usd"`
	Tiered     bool    `json:"tiered"`
}

type ModelMarketplaceLongContext struct {
	InputTokenThreshold  int     `json:"input_token_threshold"`
	InputCostMultiplier  float64 `json:"input_cost_multiplier"`
	OutputCostMultiplier float64 `json:"output_cost_multiplier"`
}

type ModelMarketplaceFacets struct {
	Providers    []ModelMarketplaceFacetOption `json:"providers"`
	Modes        []ModelMarketplaceFacetOption `json:"modes"`
	BillingModes []ModelMarketplaceFacetOption `json:"billing_modes"`
	Endpoints    []ModelMarketplaceFacetOption `json:"endpoints"`
	Groups       []ModelMarketplaceGroupFacet  `json:"groups"`
}

type ModelMarketplaceFacetOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int    `json:"count"`
	Icon  string `json:"icon,omitempty"`
}

type ModelMarketplaceGroupFacet struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	Platform         string  `json:"platform"`
	RateMultiplier   float64 `json:"rate_multiplier"`
	SubscriptionType string  `json:"subscription_type"`
	Count            int     `json:"count"`
}

type ModelMarketplacePagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
	Pages    int `json:"pages"`
}

type ModelMarketplaceCatalogState struct {
	LastUpdated string `json:"last_updated,omitempty"`
	LocalHash   string `json:"local_hash,omitempty"`
	ModelCount  int    `json:"model_count"`
}

type modelMarketplaceGroupContext struct {
	groups        []Group
	userRates     map[int64]float64
	selectedID    *int64
	selectedName  string
	selectedGroup *Group
	rate          float64
}

func (s *ModelMarketplaceService) ListPricing(ctx context.Context, userID int64, req ModelMarketplaceListRequest) (*ModelMarketplaceResponse, error) {
	req = normalizeModelMarketplaceRequest(req)
	groupCtx, err := s.resolveGroupContext(ctx, userID, req.GroupID)
	if err != nil {
		return nil, err
	}

	items := s.buildItems(ctx, groupCtx, req.ServiceTier, req.Unit)
	filtered := make([]ModelMarketplaceItem, 0, len(items))
	for _, item := range items {
		if !matchesModelMarketplaceFilters(item, req) {
			continue
		}
		filtered = append(filtered, item)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].ProviderLabel != filtered[j].ProviderLabel {
			return filtered[i].ProviderLabel < filtered[j].ProviderLabel
		}
		return filtered[i].Model < filtered[j].Model
	})

	total := len(filtered)
	pages := int(math.Ceil(float64(total) / float64(req.PageSize)))
	if pages < 1 {
		pages = 1
	}
	if req.Page > pages {
		req.Page = pages
	}
	start := (req.Page - 1) * req.PageSize
	if start > total {
		start = total
	}
	end := start + req.PageSize
	if end > total {
		end = total
	}

	return &ModelMarketplaceResponse{
		Items:      filtered[start:end],
		Facets:     buildModelMarketplaceFacets(filtered, groupCtx.groups, groupCtx.userRates),
		Pagination: ModelMarketplacePagination{Page: req.Page, PageSize: req.PageSize, Total: total, Pages: pages},
		Catalog:    s.catalogState(),
	}, nil
}

func normalizeModelMarketplaceRequest(req ModelMarketplaceListRequest) ModelMarketplaceListRequest {
	req.Query = strings.ToLower(strings.TrimSpace(req.Query))
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	req.BillingMode = strings.ToLower(strings.TrimSpace(req.BillingMode))
	req.Endpoint = strings.ToLower(strings.TrimSpace(req.Endpoint))
	req.ServiceTier = strings.ToLower(strings.TrimSpace(req.ServiceTier))
	if req.ServiceTier == "" {
		req.ServiceTier = "standard"
	}
	if req.ServiceTier != "standard" && req.ServiceTier != "priority" && req.ServiceTier != "flex" {
		req.ServiceTier = "standard"
	}
	req.Unit = strings.ToUpper(strings.TrimSpace(req.Unit))
	if req.Unit != "1K" {
		req.Unit = "1M"
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 50
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
	return req
}

func (s *ModelMarketplaceService) resolveGroupContext(ctx context.Context, userID int64, selectedGroupID *int64) (modelMarketplaceGroupContext, error) {
	out := modelMarketplaceGroupContext{rate: 1}
	if s.apiKeyService == nil || userID <= 0 {
		return out, nil
	}
	groups, err := s.apiKeyService.GetAvailableGroups(ctx, userID)
	if err != nil {
		return out, err
	}
	userRates, err := s.apiKeyService.GetUserGroupRates(ctx, userID)
	if err != nil {
		return out, err
	}
	out.groups = groups
	out.userRates = userRates
	if selectedGroupID == nil {
		return out, nil
	}
	for i := range groups {
		if groups[i].ID != *selectedGroupID {
			continue
		}
		selected := groups[i]
		out.selectedID = selectedGroupID
		out.selectedName = selected.Name
		out.selectedGroup = &selected
		out.rate = selected.RateMultiplier
		if userRates != nil {
			if userRate, ok := userRates[selected.ID]; ok && userRate >= 0 {
				out.rate = userRate
			}
		}
		return out, nil
	}
	return out, infraerrors.BadRequest("MODEL_GROUP_NOT_AVAILABLE", "selected group is not available to this user")
}

func (s *ModelMarketplaceService) catalogState() ModelMarketplaceCatalogState {
	if s.pricingService == nil {
		return ModelMarketplaceCatalogState{}
	}
	status := s.pricingService.CatalogStatus()
	state := ModelMarketplaceCatalogState{
		LocalHash:  status.LocalHash,
		ModelCount: status.ModelCount,
	}
	if !status.LastUpdated.IsZero() {
		state.LastUpdated = status.LastUpdated.UTC().Format(time.RFC3339)
	}
	return state
}

func (s *ModelMarketplaceService) buildItems(ctx context.Context, groupCtx modelMarketplaceGroupContext, serviceTier, unit string) []ModelMarketplaceItem {
	items := make([]ModelMarketplaceItem, 0)
	seen := make(map[string]struct{})
	scope := s.modelScopeForSelectedGroup(ctx, groupCtx)

	appendIfAllowed := func(item ModelMarketplaceItem) {
		if !scope.allows(item) {
			return
		}
		items = append(items, item)
	}

	if s.pricingService != nil {
		for _, entry := range s.pricingService.ListModelPricingCatalog() {
			modelKey := strings.ToLower(strings.TrimSpace(entry.Model))
			if modelKey == "" {
				continue
			}
			seen[modelKey] = struct{}{}
			pricing := s.modelPricingFor(entry.Model, &entry.Pricing)
			appendIfAllowed(buildModelMarketplaceItem(entry.Model, "litellm_catalog", &entry.Pricing, pricing, groupCtx, serviceTier, unit))
		}
	}

	if s.billingService != nil {
		for model, pricing := range s.billingService.ListFallbackModelPricing() {
			modelKey := strings.ToLower(strings.TrimSpace(model))
			if modelKey == "" {
				continue
			}
			if _, ok := seen[modelKey]; ok {
				continue
			}
			appendIfAllowed(buildModelMarketplaceItem(model, "sub2api_fallback", nil, pricing, groupCtx, serviceTier, unit))
		}
	}
	return items
}

func (s *ModelMarketplaceService) modelScopeForSelectedGroup(ctx context.Context, groupCtx modelMarketplaceGroupContext) modelMarketplaceModelScope {
	if groupCtx.selectedGroup == nil {
		return modelMarketplaceModelScope{}
	}

	group := groupCtx.selectedGroup
	scope := modelMarketplaceModelScope{selected: true, platform: group.Platform}
	if s.gatewayService != nil {
		scope.models = s.gatewayService.GetAvailableModels(ctx, &group.ID, group.Platform)
	}
	if len(scope.models) == 0 {
		scope.models = defaultModelMarketplaceModelIDsForPlatform(group.Platform)
	}
	if len(scope.models) > 0 {
		scope.restrictModels = true
	}
	if group.CustomModelsListEnabled() {
		scope.models = filterModelMarketplaceModelsByCustomList(scope.models, group.ModelsListConfig.Models)
		scope.restrictModels = true
	}
	return scope
}

type modelMarketplaceModelScope struct {
	selected       bool
	platform       string
	models         []string
	restrictModels bool
}

func (s modelMarketplaceModelScope) allows(item ModelMarketplaceItem) bool {
	if !s.selected {
		return true
	}
	if s.restrictModels {
		return modelMarketplaceAllowsModel(s.models, item.Model)
	}
	return modelMarketplaceGroupPlatformAllowsItem(s.platform, item)
}

func filterModelMarketplaceModelsByCustomList(availableModels, selectedModels []string) []string {
	if len(selectedModels) == 0 {
		return availableModels
	}
	if len(availableModels) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(selectedModels))
	seen := make(map[string]struct{}, len(selectedModels))
	for _, model := range selectedModels {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if !modelMarketplaceAllowsModel(availableModels, model) {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		filtered = append(filtered, model)
	}
	return filtered
}

func modelMarketplaceAllowsModel(availablePatterns []string, model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, pattern := range availablePatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if pattern == model {
			return true
		}
		if strings.HasSuffix(pattern, "*") && strings.HasPrefix(model, strings.TrimSuffix(pattern, "*")) {
			return true
		}
	}
	return false
}

func defaultModelMarketplaceModelIDsForPlatform(platform string) []string {
	switch strings.TrimSpace(platform) {
	case PlatformOpenAI:
		return openai.DefaultModelIDs()
	case PlatformGemini:
		ids := make([]string, 0, len(geminicli.DefaultModels))
		for _, model := range geminicli.DefaultModels {
			ids = append(ids, model.ID)
		}
		return ids
	case PlatformAntigravity:
		models := antigravity.DefaultModels()
		ids := make([]string, 0, len(models))
		for _, model := range models {
			ids = append(ids, model.ID)
		}
		return ids
	case PlatformKiro, PlatformAnthropic:
		return claude.DefaultModelIDs()
	default:
		return nil
	}
}

func modelMarketplaceGroupPlatformAllowsItem(platform string, item ModelMarketplaceItem) bool {
	endpoint := modelMarketplaceEndpointForGroupPlatform(platform)
	if endpoint == "" {
		return true
	}
	for _, candidate := range item.SupportedEndpoints {
		if strings.EqualFold(candidate, endpoint) {
			return true
		}
	}
	return false
}

func modelMarketplaceEndpointForGroupPlatform(platform string) string {
	switch strings.TrimSpace(platform) {
	case PlatformOpenAI:
		return "openai"
	case PlatformGemini:
		return "gemini"
	case PlatformAnthropic, PlatformKiro:
		return "anthropic"
	default:
		return ""
	}
}

func (s *ModelMarketplaceService) modelPricingFor(model string, catalogPricing *LiteLLMModelPricing) *ModelPricing {
	if s.billingService != nil {
		if pricing, err := s.billingService.GetModelPricing(model); err == nil && pricing != nil {
			return pricing
		}
	}
	if catalogPricing == nil {
		return nil
	}
	return modelPricingFromLiteLLM(catalogPricing)
}

func modelPricingFromLiteLLM(p *LiteLLMModelPricing) *ModelPricing {
	if p == nil {
		return nil
	}
	price5m := p.CacheCreationInputTokenCost
	price1h := p.CacheCreationInputTokenCostAbove1hr
	return &ModelPricing{
		InputPricePerToken:             p.InputCostPerToken,
		InputPricePerTokenPriority:     p.InputCostPerTokenPriority,
		OutputPricePerToken:            p.OutputCostPerToken,
		OutputPricePerTokenPriority:    p.OutputCostPerTokenPriority,
		CacheCreationPricePerToken:     p.CacheCreationInputTokenCost,
		CacheReadPricePerToken:         p.CacheReadInputTokenCost,
		CacheReadPricePerTokenPriority: p.CacheReadInputTokenCostPriority,
		CacheCreation5mPrice:           price5m,
		CacheCreation1hPrice:           price1h,
		SupportsCacheBreakdown:         price1h > 0 && price1h > price5m,
		LongContextInputThreshold:      p.LongContextInputTokenThreshold,
		LongContextInputMultiplier:     p.LongContextInputCostMultiplier,
		LongContextOutputMultiplier:    p.LongContextOutputCostMultiplier,
		ImageOutputPricePerToken:       p.OutputCostPerImageToken,
	}
}

func buildModelMarketplaceItem(model, source string, catalogPricing *LiteLLMModelPricing, pricing *ModelPricing, groupCtx modelMarketplaceGroupContext, serviceTier, unit string) ModelMarketplaceItem {
	provider := normalizeModelMarketplaceProvider(model, "")
	mode := "chat"
	context := ModelMarketplaceContext{}
	features := ModelMarketplaceFeatures{}
	perRequest := (*ModelMarketplacePerRequestPrice)(nil)
	if catalogPricing != nil {
		provider = normalizeModelMarketplaceProvider(model, catalogPricing.LiteLLMProvider)
		mode = strings.TrimSpace(catalogPricing.Mode)
		if mode == "" {
			mode = "chat"
		}
		context.MaxInputTokens = catalogPricing.MaxInputTokens
		context.MaxOutputTokens = catalogPricing.MaxOutputTokens
		features.PromptCaching = catalogPricing.SupportsPromptCaching
		features.ServiceTier = catalogPricing.SupportsServiceTier
		if catalogPricing.OutputCostPerImage > 0 {
			perRequest = &ModelMarketplacePerRequestPrice{
				UnitPrice:  catalogPricing.OutputCostPerImage,
				DisplayUSD: catalogPricing.OutputCostPerImage * groupCtx.rate,
				Tiered:     false,
			}
		}
	}
	providerLabel, providerIcon := modelMarketplaceProviderLabel(provider)
	family := inferModelMarketplaceFamily(model, provider)
	endpoints := inferModelMarketplaceEndpoints(model, provider)
	billingMode := inferModelMarketplaceBillingMode(mode, pricing, perRequest)
	if pricing == nil {
		pricing = &ModelPricing{}
	}
	features = enrichModelMarketplaceFeatures(features, model, mode, pricing)
	if billingMode == ModelMarketplaceBillingModeImage && perRequest == nil && pricing.ImageOutputPricePerToken > 0 {
		perRequest = nil
	}
	return ModelMarketplaceItem{
		Model:              model,
		Provider:           provider,
		ProviderLabel:      providerLabel,
		ProviderIcon:       providerIcon,
		Family:             family,
		Mode:               mode,
		BillingMode:        billingMode,
		SupportedEndpoints: endpoints,
		Features:           features,
		Context:            context,
		Pricing:            buildModelMarketplacePricing(source, pricing, perRequest, groupCtx, serviceTier, unit),
		CalculationNote:    modelMarketplaceFormulaNote,
		Tags:               buildModelMarketplaceTags(model, mode, features, billingMode),
	}
}

func buildModelMarketplacePricing(source string, pricing *ModelPricing, perRequest *ModelMarketplacePerRequestPrice, groupCtx modelMarketplaceGroupContext, serviceTier, unit string) ModelMarketplacePricing {
	unitTokens := 1_000_000.0
	displayUnit := "1M_tokens"
	if unit == "1K" {
		unitTokens = 1_000.0
		displayUnit = "1K_tokens"
	}
	inputPrice := pricing.InputPricePerToken
	outputPrice := pricing.OutputPricePerToken
	cacheReadPrice := pricing.CacheReadPricePerToken
	tierMultiplier := 1.0
	if usePriorityServiceTierPricing(serviceTier, pricing) {
		if pricing.InputPricePerTokenPriority > 0 {
			inputPrice = pricing.InputPricePerTokenPriority
		}
		if pricing.OutputPricePerTokenPriority > 0 {
			outputPrice = pricing.OutputPricePerTokenPriority
		}
		if pricing.CacheReadPricePerTokenPriority > 0 {
			cacheReadPrice = pricing.CacheReadPricePerTokenPriority
		}
	} else {
		tierMultiplier = serviceTierCostMultiplier(serviceTier)
	}
	if perRequest != nil {
		perRequest = &ModelMarketplacePerRequestPrice{
			UnitPrice:  perRequest.UnitPrice,
			DisplayUSD: perRequest.UnitPrice * groupCtx.rate,
			Tiered:     perRequest.Tiered,
		}
	}
	var longContext *ModelMarketplaceLongContext
	if pricing.LongContextInputThreshold > 0 || pricing.LongContextInputMultiplier > 0 || pricing.LongContextOutputMultiplier > 0 {
		longContext = &ModelMarketplaceLongContext{
			InputTokenThreshold:  pricing.LongContextInputThreshold,
			InputCostMultiplier:  pricing.LongContextInputMultiplier,
			OutputCostMultiplier: pricing.LongContextOutputMultiplier,
		}
	}
	return ModelMarketplacePricing{
		Source:         source,
		CatalogUnit:    "per_token_usd",
		DisplayUnit:    displayUnit,
		RateMultiplier: groupCtx.rate,
		GroupID:        groupCtx.selectedID,
		GroupName:      groupCtx.selectedName,
		ServiceTier:    serviceTier,
		Input:          makeModelMarketplacePricePart(inputPrice, unitTokens, tierMultiplier, groupCtx.rate),
		Output:         makeModelMarketplacePricePart(outputPrice, unitTokens, tierMultiplier, groupCtx.rate),
		CacheRead:      makeModelMarketplacePricePart(cacheReadPrice, unitTokens, tierMultiplier, groupCtx.rate),
		CacheWrite:     makeModelMarketplacePricePart(pricing.CacheCreationPricePerToken, unitTokens, tierMultiplier, groupCtx.rate),
		CacheWrite5m:   makeModelMarketplacePricePart(pricing.CacheCreation5mPrice, unitTokens, tierMultiplier, groupCtx.rate),
		CacheWrite1h:   makeModelMarketplacePricePart(pricing.CacheCreation1hPrice, unitTokens, tierMultiplier, groupCtx.rate),
		ImageOutput:    makeModelMarketplacePricePart(pricing.ImageOutputPricePerToken, unitTokens, tierMultiplier, groupCtx.rate),
		PerRequest:     perRequest,
		LongContext:    longContext,
	}
}

func makeModelMarketplacePricePart(perToken, unitTokens, tierMultiplier, rateMultiplier float64) ModelMarketplacePricePart {
	if perToken < 0 {
		perToken = 0
	}
	basePerToken := perToken * tierMultiplier
	return ModelMarketplacePricePart{
		PerToken:   basePerToken,
		Per1K:      basePerToken * 1_000,
		Per1M:      basePerToken * 1_000_000,
		DisplayUSD: basePerToken * unitTokens * rateMultiplier,
	}
}

func matchesModelMarketplaceFilters(item ModelMarketplaceItem, req ModelMarketplaceListRequest) bool {
	if req.Query != "" {
		haystack := strings.ToLower(strings.Join([]string{item.Model, item.Provider, item.ProviderLabel, item.Family, item.Mode, strings.Join(item.Tags, " ")}, " "))
		if !strings.Contains(haystack, req.Query) {
			return false
		}
	}
	if req.Provider != "" && req.Provider != item.Provider {
		return false
	}
	if req.Mode != "" && req.Mode != strings.ToLower(item.Mode) {
		return false
	}
	if req.BillingMode != "" && req.BillingMode != item.BillingMode {
		return false
	}
	if req.Endpoint != "" {
		matched := false
		for _, endpoint := range item.SupportedEndpoints {
			if req.Endpoint == strings.ToLower(endpoint) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func buildModelMarketplaceFacets(items []ModelMarketplaceItem, groups []Group, userRates map[int64]float64) ModelMarketplaceFacets {
	providerCounts := map[string]int{}
	providerLabels := map[string]string{}
	providerIcons := map[string]string{}
	modeCounts := map[string]int{}
	billingCounts := map[string]int{}
	endpointCounts := map[string]int{}
	for _, item := range items {
		providerCounts[item.Provider]++
		providerLabels[item.Provider] = item.ProviderLabel
		providerIcons[item.Provider] = item.ProviderIcon
		modeCounts[item.Mode]++
		billingCounts[item.BillingMode]++
		for _, endpoint := range item.SupportedEndpoints {
			endpointCounts[endpoint]++
		}
	}
	groupFacets := make([]ModelMarketplaceGroupFacet, 0, len(groups))
	for _, group := range groups {
		rate := group.RateMultiplier
		if userRates != nil {
			if userRate, ok := userRates[group.ID]; ok && userRate >= 0 {
				rate = userRate
			}
		}
		groupFacets = append(groupFacets, ModelMarketplaceGroupFacet{
			ID:               group.ID,
			Name:             group.Name,
			Platform:         group.Platform,
			RateMultiplier:   rate,
			SubscriptionType: group.SubscriptionType,
			Count:            len(items),
		})
	}
	sort.SliceStable(groupFacets, func(i, j int) bool {
		if groupFacets[i].Name != groupFacets[j].Name {
			return groupFacets[i].Name < groupFacets[j].Name
		}
		return groupFacets[i].ID < groupFacets[j].ID
	})
	return ModelMarketplaceFacets{
		Providers:    modelMarketplaceFacetOptions(providerCounts, providerLabels, providerIcons),
		Modes:        modelMarketplaceFacetOptions(modeCounts, nil, nil),
		BillingModes: modelMarketplaceFacetOptions(billingCounts, map[string]string{"token": "Token", "image": "Image"}, nil),
		Endpoints:    modelMarketplaceFacetOptions(endpointCounts, map[string]string{"openai": "OpenAI-compatible", "anthropic": "Anthropic", "gemini": "Gemini"}, nil),
		Groups:       groupFacets,
	}
}

func modelMarketplaceFacetOptions(counts map[string]int, labels map[string]string, icons map[string]string) []ModelMarketplaceFacetOption {
	out := make([]ModelMarketplaceFacetOption, 0, len(counts))
	for value, count := range counts {
		label := value
		if labels != nil && labels[value] != "" {
			label = labels[value]
		}
		icon := ""
		if icons != nil {
			icon = icons[value]
		}
		out = append(out, ModelMarketplaceFacetOption{Value: value, Label: label, Count: count, Icon: icon})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	return out
}

func normalizeModelMarketplaceProvider(model, provider string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	p := strings.ToLower(strings.TrimSpace(provider))
	switch {
	case strings.Contains(p, "anthropic") || strings.Contains(m, "claude"):
		return "anthropic"
	case strings.Contains(p, "vertex") || strings.Contains(p, "gemini") || strings.Contains(m, "gemini"):
		return "google"
	case strings.Contains(p, "deepseek") || strings.Contains(m, "deepseek"):
		return "deepseek"
	case strings.Contains(p, "zhipu") || strings.Contains(p, "glm") || strings.Contains(m, "glm-"):
		return "zhipu"
	case strings.Contains(p, "moonshot") || strings.Contains(p, "kimi") || strings.Contains(m, "kimi"):
		return "moonshot"
	case strings.Contains(p, "qwen") || strings.Contains(p, "alibaba") || strings.Contains(m, "qwen"):
		return "qwen"
	case strings.Contains(p, "volc") || strings.Contains(p, "doubao") || strings.Contains(m, "doubao"):
		return "doubao"
	case strings.Contains(p, "minimax") || strings.Contains(m, "minimax"):
		return "minimax"
	case strings.Contains(p, "openai") || strings.HasPrefix(m, "gpt-") || strings.Contains(m, "codex") || strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4"):
		return "openai"
	default:
		if p != "" {
			return strings.ReplaceAll(p, "_", "-")
		}
		return "unknown"
	}
}

func modelMarketplaceProviderLabel(provider string) (string, string) {
	switch provider {
	case "openai":
		return "OpenAI", "openai"
	case "anthropic":
		return "Anthropic / Claude", "anthropic"
	case "google":
		return "Google / Gemini", "gemini"
	case "deepseek":
		return "DeepSeek", "deepseek"
	case "zhipu":
		return "智谱 / GLM", "glm"
	case "moonshot":
		return "Moonshot / Kimi", "moonshot"
	case "qwen":
		return "Alibaba / Qwen", "qwen"
	case "doubao":
		return "ByteDance / Doubao", "doubao"
	case "minimax":
		return "MiniMax", "minimax"
	default:
		if provider == "" || provider == "unknown" {
			return "Unknown", "model"
		}
		return titleModelMarketplaceProvider(provider), provider
	}
}

func titleModelMarketplaceProvider(provider string) string {
	words := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(provider))
	if len(words) == 0 {
		return "Unknown"
	}
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func inferModelMarketplaceFamily(model, provider string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "claude"):
		return "Claude"
	case strings.HasPrefix(m, "gpt-"):
		return "GPT"
	case strings.Contains(m, "codex"):
		return "Codex"
	case strings.Contains(m, "gemini"):
		return "Gemini"
	case strings.Contains(m, "deepseek"):
		return "DeepSeek"
	case strings.Contains(m, "glm"):
		return "GLM"
	case strings.Contains(m, "kimi"):
		return "Kimi"
	case strings.Contains(m, "qwen"):
		return "Qwen"
	case strings.Contains(m, "doubao"):
		return "Doubao"
	case strings.Contains(m, "minimax"):
		return "MiniMax"
	case provider == "anthropic":
		return "Claude"
	case provider == "google":
		return "Gemini"
	default:
		return "General"
	}
}

func inferModelMarketplaceEndpoints(model, provider string) []string {
	m := strings.ToLower(model)
	switch provider {
	case "anthropic":
		return []string{"anthropic"}
	case "google":
		return []string{"gemini"}
	default:
		if strings.Contains(m, "claude") && provider == "anthropic" {
			return []string{"anthropic"}
		}
		return []string{"openai"}
	}
}

func inferModelMarketplaceBillingMode(mode string, pricing *ModelPricing, perRequest *ModelMarketplacePerRequestPrice) string {
	modeLower := strings.ToLower(mode)
	if perRequest != nil || strings.Contains(modeLower, "image") {
		return ModelMarketplaceBillingModeImage
	}
	if pricing != nil && pricing.ImageOutputPricePerToken > 0 && strings.Contains(modeLower, "image") {
		return ModelMarketplaceBillingModeImage
	}
	return ModelMarketplaceBillingModeToken
}

func enrichModelMarketplaceFeatures(features ModelMarketplaceFeatures, model, mode string, pricing *ModelPricing) ModelMarketplaceFeatures {
	m := strings.ToLower(model)
	mode = strings.ToLower(mode)
	features.PromptCaching = features.PromptCaching || pricing.CacheReadPricePerToken > 0 || pricing.CacheCreationPricePerToken > 0 || pricing.CacheCreation5mPrice > 0 || pricing.CacheCreation1hPrice > 0
	features.ServiceTier = features.ServiceTier || pricing.InputPricePerTokenPriority > 0 || pricing.OutputPricePerTokenPriority > 0 || pricing.CacheReadPricePerTokenPriority > 0
	features.Vision = strings.Contains(mode, "image") || strings.Contains(m, "vision") || strings.Contains(m, "image") || pricing.ImageInputPricePerToken > 0 || pricing.ImageOutputPricePerToken > 0
	features.Reasoning = strings.Contains(m, "reason") || strings.Contains(m, "thinking") || strings.Contains(m, "gpt-5") || strings.Contains(m, "o1") || strings.Contains(m, "o3") || strings.Contains(m, "o4")
	features.AudioOutput = strings.Contains(mode, "audio") || strings.Contains(m, "audio") || strings.Contains(m, "tts")
	return features
}

func buildModelMarketplaceTags(model, mode string, features ModelMarketplaceFeatures, billingMode string) []string {
	seen := map[string]struct{}{}
	add := func(tag string) {
		if tag == "" {
			return
		}
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
	}
	add(billingMode)
	add(mode)
	if features.PromptCaching {
		add("cache")
	}
	if features.ServiceTier {
		add("service_tier")
	}
	if features.Vision {
		add("vision")
	}
	if features.Reasoning {
		add("reasoning")
	}
	if strings.Contains(strings.ToLower(model), "embedding") {
		add("embedding")
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func ParseModelMarketplaceGroupID(raw string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return nil, infraerrors.BadRequest("INVALID_MODEL_GROUP_ID", "group_id must be a positive integer")
	}
	return &id, nil
}
