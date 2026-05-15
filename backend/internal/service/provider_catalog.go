package service

import (
	"context"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
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
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Reasoning     bool     `json:"reasoning"`
	Input         []string `json:"input"`
	ContextWindow int      `json:"context_window"`
	MaxTokens     int      `json:"max_tokens"`
}

// ProviderCatalogResponse is the top-level response for the provider-catalog endpoint.
type ProviderCatalogResponse struct {
	ProviderCatalog []ProviderCatalogEntry `json:"provider_catalog"`
}

// providerMeta holds static metadata for a v-claw provider.
type providerMeta struct {
	ProviderID   string
	ProviderName string
	APIStyle     string
}

// platformProviders maps sub2api platform constants to v-claw provider metadata.
var platformProviders = map[string]providerMeta{
	domain.PlatformOpenAI: {
		ProviderID:   "v-claw-openai",
		ProviderName: "OpenAI",
		APIStyle:     "openai-responses",
	},
	domain.PlatformAnthropic: {
		ProviderID:   "v-claw-anthropic",
		ProviderName: "Anthropic",
		APIStyle:     "anthropic-messages",
	},
	domain.PlatformGemini: {
		ProviderID:   "v-claw-google",
		ProviderName: "Google",
		APIStyle:     "google-native",
	},
	domain.PlatformAntigravity: {
		ProviderID:   "v-claw-google",
		ProviderName: "Google",
		APIStyle:     "google-native",
	},
}

// resolveProvider determines which v-claw provider a model belongs to based on its platform.
func resolveProvider(modelID, platform string) (providerMeta, bool) {
	meta, ok := platformProviders[platform]
	return meta, ok
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
// provider catalog grouped by v-claw provider ID.
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
			meta, ok := resolveProvider(model.Name, model.Platform)
			if !ok {
				continue
			}

			agg, exists := providers[meta.ProviderID]
			if !exists {
				agg = &providerAgg{
					entry: ProviderCatalogEntry{
						ProviderID:   meta.ProviderID,
						ProviderName: meta.ProviderName,
						APIStyle:     meta.APIStyle,
						Models:       make([]ProviderCatalogModel, 0),
					},
					seen: make(map[string]struct{}),
				}
				providers[meta.ProviderID] = agg
			}

			modelKey := strings.ToLower(model.Name)
			if _, dup := agg.seen[modelKey]; dup {
				continue
			}
			agg.seen[modelKey] = struct{}{}

			input := []string{"text"}
			if supportsImageInput(model.Name) {
				input = []string{"text", "image"}
			}

			agg.entry.Models = append(agg.entry.Models, ProviderCatalogModel{
				ID:            model.Name,
				Name:          model.Name,
				Reasoning:     isReasoningModel(model.Name),
				Input:         input,
				ContextWindow: defaultContextWindow(model.Name),
				MaxTokens:     defaultMaxTokens(model.Name),
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
