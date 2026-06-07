package service

import (
	"context"
	"sort"
	"strings"
)

// ProviderCatalogEntry represents a single provider in the catalog response.
type ProviderCatalogEntry struct {
	ProviderID   string                 `json:"provider_id"`
	ProviderName string                 `json:"provider_name"`
	APIStyle     string                 `json:"api_style"`
	Models       []ProviderCatalogModel `json:"models"`
}

// ProviderCatalogModel represents a model within a provider catalog entry.
type ProviderCatalogModel struct {
	ID            string                      `json:"id"`
	Name          string                      `json:"name"`
	Reasoning     bool                        `json:"reasoning"`
	Input         []string                    `json:"input"`
	Output        []string                    `json:"output"`
	ContextWindow int                         `json:"contextWindow"`
	MaxTokens     int                         `json:"maxTokens"`
	Cost          ProviderCatalogModelCost    `json:"cost"`
	Compat        *ProviderCatalogModelCompat `json:"compat,omitempty"`
}

// ProviderCatalogModelCost represents the cost structure for a model.
// Currently all zeros since V-Claw uses subscription-based billing.
type ProviderCatalogModelCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// ProviderCatalogModelCompat represents compatibility flags for a model.
type ProviderCatalogModelCompat struct {
	SupportsReasoningEffort  bool `json:"supportsReasoningEffort,omitempty"`
	SupportsUsageInStreaming bool `json:"supportsUsageInStreaming,omitempty"`
}

// ProviderCatalogResponse is the top-level response for the provider-catalog endpoint.
type ProviderCatalogResponse struct {
	ProviderCatalog []ProviderCatalogEntry `json:"provider_catalog"`
}

// apiStyleForPlatform maps a platform to its wire protocol style.
// This is protocol-level knowledge (how v-claw talks to that provider type),
// not a list of what's available — availability is determined by the DB.
var apiStyleForPlatform = map[string]string{
	"openai":      "openai-responses",
	"anthropic":   "anthropic-messages",
	"gemini":      "google-native",
	"antigravity": "google-native",
	"kiro":        "anthropic-messages",
}

// platformDisplayName maps platform to a human-friendly provider name.
// Falls back to capitalizing the platform string if not found.
var platformDisplayName = map[string]string{
	"openai":      "OpenAI",
	"anthropic":   "Anthropic",
	"gemini":      "Google",
	"antigravity": "Google",
	"kiro":        "Kiro",
}

// platformToProviderID maps platform to the client-facing provider ID.
// Keep this one-to-one with platform so provider traffic cannot collapse distinct
// entitlement groups (for example, gemini and antigravity must not share a provider ID).
var platformToProviderID = map[string]string{
	"openai":      "v-claw-openai",
	"anthropic":   "v-claw-anthropic",
	"gemini":      "v-claw-gemini",
	"antigravity": "v-claw-antigravity",
	"kiro":        "v-claw-kiro",
}

// resolveProviderMeta derives provider metadata from a platform string.
// It uses the DB-driven platform value and maps it to v-claw conventions.
// Returns false if the platform should not be exposed (e.g., internal-only platforms).
func resolveProviderMeta(platform string) (providerID, providerName, apiStyle string, ok bool) {
	lower := strings.ToLower(platform)

	providerID, found := platformToProviderID[lower]
	if !found {
		// Unknown platform — derive dynamically: v-claw-{platform}
		providerID = "v-claw-" + lower
	}

	providerName, found = platformDisplayName[lower]
	if !found {
		// Capitalize first letter as fallback
		if len(platform) > 0 {
			providerName = strings.ToUpper(platform[:1]) + platform[1:]
		} else {
			return "", "", "", false
		}
	}

	apiStyle, found = apiStyleForPlatform[lower]
	if !found {
		// Default to openai-chat for unknown platforms (most compatible)
		apiStyle = "openai-chat"
	}

	return providerID, providerName, apiStyle, true
}

// isReasoningModel determines if a model supports extended thinking/reasoning.
func isReasoningModel(modelID string) bool {
	lower := strings.ToLower(modelID)
	// Non-reasoning patterns
	nonReasoningPatterns := []string{
		"gpt-4o-mini", "gpt-4o-audio", "gpt-4-turbo",
		"claude-haiku",
	}
	for _, p := range nonReasoningPatterns {
		if strings.Contains(lower, p) {
			return false
		}
	}
	// Reasoning patterns
	reasoningPatterns := []string{
		"o1", "o3", "o4",
		"claude-opus", "claude-sonnet",
		"gemini-2.5", "gemini-3",
		"deepseek-r", "deepseek-reasoner",
		"gpt-5",
	}
	for _, p := range reasoningPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// canOutputImage determines if a model can generate images as output.
func canOutputImage(modelID string) bool {
	lower := strings.ToLower(modelID)
	imageOutputPatterns := []string{
		"image",     // gemini-*-image, gemini-*-image-preview
		"dall-e",    // dall-e models
		"gpt-image", // gpt-image-*
	}
	for _, p := range imageOutputPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// supportsImageInput determines if a model supports image/vision input.
func supportsImageInput(modelID string) bool {
	lower := strings.ToLower(modelID)
	visionPatterns := []string{
		"gpt-5", "gpt-4o", "gpt-4-turbo", "o1", "o3", "o4",
		"claude-opus", "claude-sonnet", "claude-haiku",
		"gemini-",
	}
	for _, p := range visionPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// defaultContextWindow returns a sensible default context window for a model.
func defaultContextWindow(modelID string) int {
	lower := strings.ToLower(modelID)
	switch {
	case strings.HasPrefix(lower, "gpt-5"):
		return 1050000
	case strings.HasPrefix(lower, "gpt-4o"):
		return 128000
	case strings.HasPrefix(lower, "o1"), strings.HasPrefix(lower, "o3"), strings.HasPrefix(lower, "o4"):
		return 200000
	case strings.HasPrefix(lower, "claude-opus"):
		return 1000000
	case strings.HasPrefix(lower, "claude-sonnet"), strings.HasPrefix(lower, "claude-haiku"):
		return 200000
	case strings.HasPrefix(lower, "gemini"):
		return 1048576
	default:
		return 128000
	}
}

// defaultMaxTokens returns a sensible default max output tokens for a model.
func defaultMaxTokens(modelID string) int {
	lower := strings.ToLower(modelID)
	switch {
	case strings.HasPrefix(lower, "gpt-5"):
		return 128000
	case strings.HasPrefix(lower, "gpt-4o"):
		return 16384
	case strings.HasPrefix(lower, "o1"), strings.HasPrefix(lower, "o3"), strings.HasPrefix(lower, "o4"):
		return 100000
	case strings.HasPrefix(lower, "claude-opus"):
		return 128000
	case strings.HasPrefix(lower, "claude-sonnet"):
		return 64000
	case strings.HasPrefix(lower, "claude-haiku"):
		return 8192
	case strings.HasPrefix(lower, "gemini"):
		return 65536
	default:
		return 8192
	}
}

// ProviderCatalogService builds the provider catalog from channel/group data.
type ProviderCatalogService struct {
	channelService *ChannelService
}

// NewProviderCatalogService creates a new ProviderCatalogService.
func NewProviderCatalogService(channelService *ChannelService) *ProviderCatalogService {
	return &ProviderCatalogService{
		channelService: channelService,
	}
}

// BuildCatalog aggregates all active channels and their supported models into a
// provider catalog. Providers are derived dynamically from the platforms found
// in the database (via channel groups), not from a hardcoded list.
func (s *ProviderCatalogService) BuildCatalog(ctx context.Context) (*ProviderCatalogResponse, error) {
	channels, err := s.channelService.ListAvailable(ctx)
	if err != nil {
		return nil, err
	}

	// Aggregate models by provider ID, deduplicating by model name.
	type providerAgg struct {
		entry ProviderCatalogEntry
		seen  map[string]struct{}
	}
	providers := make(map[string]*providerAgg)

	for _, ch := range channels {
		if ch.Status != StatusActive {
			continue
		}
		for _, model := range ch.SupportedModels {
			providerID, providerName, apiStyle, ok := resolveProviderMeta(model.Platform)
			if !ok {
				continue
			}

			agg, exists := providers[providerID]
			if !exists {
				agg = &providerAgg{
					entry: ProviderCatalogEntry{
						ProviderID:   providerID,
						ProviderName: providerName,
						APIStyle:     apiStyle,
						Models:       make([]ProviderCatalogModel, 0),
					},
					seen: make(map[string]struct{}),
				}
				providers[providerID] = agg
			}

			modelKey := strings.ToLower(model.Name)
			if _, dup := agg.seen[modelKey]; dup {
				continue
			}
			agg.seen[modelKey] = struct{}{}

			// All models on our system support text+image input (GPT-5.x, Claude, Gemini)
			input := []string{"text", "image"}

			// Output: image-generation models output images, others text-only
			output := []string{"text"}
			if canOutputImage(model.Name) {
				output = []string{"text", "image"}
			}

			var compat *ProviderCatalogModelCompat
			if apiStyle == "openai-responses" {
				compat = &ProviderCatalogModelCompat{
					SupportsReasoningEffort:  isReasoningModel(model.Name),
					SupportsUsageInStreaming: true,
				}
			}

			agg.entry.Models = append(agg.entry.Models, ProviderCatalogModel{
				ID:            model.Name,
				Name:          model.Name,
				Reasoning:     isReasoningModel(model.Name),
				Input:         input,
				Output:        output,
				ContextWindow: defaultContextWindow(model.Name),
				MaxTokens:     defaultMaxTokens(model.Name),
				Cost:          ProviderCatalogModelCost{},
				Compat:        compat,
			})
		}
	}

	// Build sorted result
	catalog := make([]ProviderCatalogEntry, 0, len(providers))
	for _, agg := range providers {
		sort.Slice(agg.entry.Models, func(i, j int) bool {
			return agg.entry.Models[i].ID < agg.entry.Models[j].ID
		})
		catalog = append(catalog, agg.entry)
	}
	sort.Slice(catalog, func(i, j int) bool {
		return catalog[i].ProviderID < catalog[j].ProviderID
	})

	return &ProviderCatalogResponse{
		ProviderCatalog: catalog,
	}, nil
}
