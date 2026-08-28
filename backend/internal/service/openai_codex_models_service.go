package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"golang.org/x/net/http2"
	"golang.org/x/sync/singleflight"
)

// chatgptCodexModelsURL is the ChatGPT Codex models manifest endpoint.
// Package-level variable so tests can point it at a stub server.
var chatgptCodexModelsURL = "https://chatgpt.com/backend-api/codex/models"

const (
	codexModelsManifestCacheBodyLimit  = 1 << 20
	codexModelsManifestCacheMaxEntries = 64
	codexModelsManifestCacheTTL        = 30 * time.Second
	codexModelsManifestCacheStaleTTL   = 5 * time.Minute
	codexModelsManifestRequestTimeout  = 15 * time.Second
	codexAutoModelPrefix               = "codex-auto-"
)

// FilterCodexModelIDsForGroup removes dedicated media-generation models,
// wildcard mapping keys, and Codex automatic modes from a client catalog.
// Automatic modes are retained only when the group's enabled custom model list
// explicitly selects the exact slug; account model mappings describe routing
// and are not feature opt-ins. Wildcard keys such as "foo-*" are routing
// patterns, not concrete Codex models.
func FilterCodexModelIDsForGroup(modelIDs []string, group *Group) []string {
	explicitlyEnabled := make(map[string]struct{})
	if group != nil && group.CustomModelsListEnabled() {
		for _, modelID := range group.ModelsListConfig.Models {
			modelID = strings.TrimSpace(modelID)
			if strings.HasPrefix(modelID, codexAutoModelPrefix) {
				explicitlyEnabled[modelID] = struct{}{}
			}
		}
	}

	filtered := make([]string, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		if isCodexDedicatedMediaModel(modelID) {
			continue
		}
		if strings.Contains(modelID, "*") {
			continue
		}
		if strings.HasPrefix(modelID, codexAutoModelPrefix) {
			if _, ok := explicitlyEnabled[modelID]; !ok {
				continue
			}
		}
		filtered = append(filtered, modelID)
	}
	return filtered
}

func isCodexDedicatedMediaModel(modelID string) bool {
	canonical := codexProviderQualifiedModelID(modelID)
	return IsGPTImageGenerationModel(canonical) ||
		isImageGenerationModel(canonical) ||
		xai.IsGrokImagineModel(modelID)
}

func codexProviderQualifiedModelID(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if slash := strings.LastIndexByte(modelID, '/'); slash >= 0 {
		modelID = strings.TrimSpace(modelID[slash+1:])
	}
	return strings.TrimPrefix(modelID, "models/")
}

// CodexModelsManifest carries the client representation plus caching metadata.
type CodexModelsManifest struct {
	Body                         []byte
	ETag                         string
	upstreamETag                 string
	upstreamSourceBody           []byte
	convertedFromOpenAIModelList bool
	NotModified                  bool
}

// BuildGroupConfiguredCodexModelsManifest builds a Codex catalog exclusively
// from the public model names configured on accounts in an OpenAI group. The
// boolean result distinguishes "no explicit configuration" from a configured
// catalog that becomes empty after group-level filtering.
func (s *OpenAIGatewayService) BuildGroupConfiguredCodexModelsManifest(
	ctx context.Context,
	group *Group,
	ifNoneMatch string,
) (*CodexModelsManifest, bool, error) {
	if s == nil || s.accountRepo == nil || group == nil || group.Platform != PlatformOpenAI {
		return nil, false, nil
	}

	visible, catalog, err := loadCodexGroupCatalogAccounts(ctx, s.accountRepo, group.ID)
	if err != nil {
		return nil, false, fmt.Errorf("load group configured Codex models: %w", err)
	}
	configuredModels := openAIConfiguredCodexModelIDsForGroup(visible, group)
	if len(configuredModels) == 0 {
		return nil, false, nil
	}

	body, err := buildCodexModelsManifestForAccounts(
		PlatformOpenAI,
		configuredModels,
		catalog,
		nil,
		true,
	)
	if err != nil {
		return nil, false, fmt.Errorf("initialize group configured Codex models: %w", err)
	}
	body, _, err = mergeConfiguredCodexModelsManifest(
		body,
		nil,
		group.ModelsListConfig.Models,
		group.CustomModelsListEnabled(),
	)
	if err != nil {
		return nil, false, fmt.Errorf("build group configured Codex models: %w", err)
	}
	manifest := &CodexModelsManifest{
		Body: body,
		ETag: codexModelsManifestBodyETag(body),
	}
	if codexModelsManifestETagMatches(ifNoneMatch, manifest.ETag) {
		manifest.Body = nil
		manifest.NotModified = true
	}
	return manifest, true, nil
}

// MergeGroupConfiguredCodexModels adds account model aliases that are visible
// to the authenticated OpenAI group without discarding metadata from upstream
// Codex model entries. A group's custom models list also filters the picker,
// matching the standard /v1/models display policy.
func (s *OpenAIGatewayService) MergeGroupConfiguredCodexModels(
	ctx context.Context,
	group *Group,
	manifest *CodexModelsManifest,
	ifNoneMatch string,
) error {
	if s == nil || s.accountRepo == nil || group == nil || manifest == nil || manifest.NotModified {
		return nil
	}
	if group.Platform != PlatformOpenAI || len(manifest.Body) == 0 {
		return nil
	}

	configuredModels, err := s.groupConfiguredCodexModelIDs(ctx, group)
	if err != nil {
		return fmt.Errorf("load group configured Codex models: %w", err)
	}
	body, changed, err := mergeConfiguredCodexModelsManifest(
		manifest.Body,
		configuredModels,
		group.ModelsListConfig.Models,
		group.CustomModelsListEnabled(),
	)
	if err != nil {
		return fmt.Errorf("merge group configured Codex models: %w", err)
	}
	if changed {
		manifest.Body = body
		manifest.ETag = codexModelsManifestBodyETag(body)
	}
	if codexModelsManifestETagMatches(ifNoneMatch, manifest.ETag) {
		manifest.Body = nil
		manifest.NotModified = true
	}
	return nil
}

func (s *OpenAIGatewayService) groupConfiguredCodexModelIDs(ctx context.Context, group *Group) ([]string, error) {
	if group == nil {
		return nil, nil
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, group.ID)
	if err != nil {
		return nil, err
	}
	return openAIConfiguredCodexModelIDsForGroup(accounts, group), nil
}

// loadCodexGroupCatalogAccounts separates picker membership from capability
// intersection. visible accounts are currently schedulable and decide which
// public aliases appear. catalog accounts are all non-deleted active group
// members; their snapshots keep advertised capabilities from widening when a
// mapped account is only temporarily unschedulable. If ListByGroup fails, the
// catalog falls back to the schedulable set so a listing error does not fail
// the client request.
func loadCodexGroupCatalogAccounts(ctx context.Context, repo AccountRepository, groupID int64) (visible []Account, catalog []Account, err error) {
	if repo == nil {
		return nil, nil, nil
	}
	visible, err = repo.ListSchedulableByGroupID(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}
	catalog = visible
	groupAccounts, listErr := repo.ListByGroup(ctx, groupID)
	if listErr != nil {
		return visible, catalog, nil
	}
	return visible, groupAccounts, nil
}

func openAIConfiguredCodexModelIDs(accounts []Account) []string {
	seen := make(map[string]struct{})
	models := make([]string, 0)
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != PlatformOpenAI {
			continue
		}
		for modelID := range account.GetModelMapping() {
			modelID = strings.TrimSpace(modelID)
			if modelID == "" || strings.Contains(modelID, "*") {
				continue
			}
			if _, exists := seen[modelID]; exists {
				continue
			}
			seen[modelID] = struct{}{}
			models = append(models, modelID)
		}
	}
	sort.Strings(models)
	return models
}

func openAIConfiguredCodexModelIDsForGroup(accounts []Account, group *Group) []string {
	models := openAIConfiguredCodexModelIDs(accounts)
	if group == nil || !group.CustomModelsListEnabled() {
		return models
	}

	seen := make(map[string]struct{}, len(models)+len(group.ModelsListConfig.Models))
	for _, modelID := range models {
		seen[modelID] = struct{}{}
	}
	for _, selectedModel := range group.ModelsListConfig.Models {
		selectedModel = strings.TrimSpace(selectedModel)
		if selectedModel == "" || strings.Contains(selectedModel, "*") {
			continue
		}
		for i := range accounts {
			account := &accounts[i]
			if account.Platform != PlatformOpenAI {
				continue
			}
			mappedModel, matched := account.ResolveMappedModel(selectedModel)
			if !matched || strings.TrimSpace(mappedModel) == "" {
				continue
			}
			if _, exists := seen[selectedModel]; !exists {
				seen[selectedModel] = struct{}{}
				models = append(models, selectedModel)
			}
			break
		}
	}
	sort.Strings(models)
	return models
}

const (
	configuredCodexModelPriority       = 50
	configuredCodexCustomDescription   = "Custom model routed through Sub2API."
	configuredCodexFallbackContext     = 272_000
	configuredCodexDeepSeekV4Context   = 1_000_000
	configuredCodexGrokContext         = 500_000
	configuredCodexGrokBuildContext    = 256_000
	configuredCodexGPT56MaxContext     = 872_000
	configuredCodexToolOutputMaxTokens = 10_000
)

type configuredCodexReasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

type configuredCodexTruncationPolicy struct {
	Mode  string `json:"mode"`
	Limit int64  `json:"limit"`
}

type configuredCodexModelMessages struct {
	InstructionsTemplate  string `json:"instructions_template"`
	InstructionsVariables any    `json:"instructions_variables"`
	Approvals             any    `json:"approvals"`
	CollaborationModes    any    `json:"collaboration_modes"`
	AutoReview            any    `json:"auto_review"`
	Permissions           any    `json:"permissions"`
	MultiAgent            any    `json:"multi_agent"`
	TokenBudget           any    `json:"token_budget"`
	GuardianV2            any    `json:"guardian_v2"`
}

// configuredCodexModelDescriptor is the minimum complete ModelInfo contract
// understood by current Codex clients. Several nullable fields are intentionally
// emitted: unlike ordinary OpenAI /v1/models entries, the Codex manifest parser
// requires them to be present.
type configuredCodexModelDescriptor struct {
	Slug                              string                          `json:"slug"`
	DisplayName                       string                          `json:"display_name"`
	Description                       string                          `json:"description"`
	DefaultReasoningLevel             *string                         `json:"default_reasoning_level,omitempty"`
	SupportedReasoningLevels          []configuredCodexReasoningLevel `json:"supported_reasoning_levels"`
	ShellType                         string                          `json:"shell_type"`
	Visibility                        string                          `json:"visibility"`
	SupportedInAPI                    bool                            `json:"supported_in_api"`
	Priority                          int                             `json:"priority"`
	AdditionalSpeedTiers              []string                        `json:"additional_speed_tiers"`
	ServiceTiers                      []any                           `json:"service_tiers"`
	DefaultServiceTier                any                             `json:"default_service_tier"`
	AvailabilityNUX                   any                             `json:"availability_nux"`
	Upgrade                           any                             `json:"upgrade"`
	ModelMessages                     configuredCodexModelMessages    `json:"model_messages"`
	IncludeSkillsUsageInstructions    bool                            `json:"include_skills_usage_instructions"`
	IncludePluginUsageInstructions    bool                            `json:"include_plugin_usage_instructions"`
	IncludeAppsUsageInstructions      bool                            `json:"include_apps_usage_instructions"`
	SupportsReasoningSummaryParameter bool                            `json:"supports_reasoning_summary_parameter"`
	DefaultReasoningSummary           string                          `json:"default_reasoning_summary"`
	SupportVerbosity                  bool                            `json:"support_verbosity"`
	DefaultVerbosity                  *string                         `json:"default_verbosity"`
	ApplyPatchToolType                *string                         `json:"apply_patch_tool_type"`
	WebSearchToolType                 string                          `json:"web_search_tool_type"`
	TruncationPolicy                  configuredCodexTruncationPolicy `json:"truncation_policy"`
	SupportsImageDetailOriginal       bool                            `json:"supports_image_detail_original"`
	SupportsParallelToolCalls         bool                            `json:"supports_parallel_tool_calls"`
	ContextWindow                     int64                           `json:"context_window"`
	MaxContextWindow                  int64                           `json:"max_context_window"`
	AutoCompactTokenLimit             any                             `json:"auto_compact_token_limit"`
	CompHash                          any                             `json:"comp_hash"`
	EffectiveContextWindowPercent     int64                           `json:"effective_context_window_percent"`
	ExperimentalSupportedTools        []string                        `json:"experimental_supported_tools"`
	InputModalities                   []string                        `json:"input_modalities"`
	SupportsSearchTool                bool                            `json:"supports_search_tool"`
	UseResponsesLite                  bool                            `json:"use_responses_lite"`
	NodeREPLAutoReviewRequired        bool                            `json:"node_repl_auto_review_required"`
	NodeREPLDisabled                  bool                            `json:"node_repl_disabled"`
	AutoReviewModelOverride           any                             `json:"auto_review_model_override"`
	ModelSpecialty                    any                             `json:"model_specialty"`
	ToolMode                          any                             `json:"tool_mode"`
	MultiAgentVersion                 any                             `json:"multi_agent_version"`
}

type codexModelMetadataOverride struct {
	UpstreamModelMetadata
	reasoningConflict       bool
	inputModalitiesConflict bool
}

func newConfiguredCodexModelDescriptor(modelID string) configuredCodexModelDescriptor {
	modelID = strings.TrimSpace(modelID)
	noReasoningLevel := "none"
	descriptor := configuredCodexModelDescriptor{
		Slug:                  modelID,
		DisplayName:           modelID,
		Description:           configuredCodexCustomDescription,
		DefaultReasoningLevel: &noReasoningLevel,
		SupportedReasoningLevels: []configuredCodexReasoningLevel{
			{Effort: "none", Description: configuredCodexReasoningLevelDescription("none")},
		},
		ShellType:                         "unified_exec",
		Visibility:                        "list",
		SupportedInAPI:                    true,
		Priority:                          configuredCodexModelPriority,
		AdditionalSpeedTiers:              []string{},
		ServiceTiers:                      []any{},
		ModelMessages:                     configuredCodexModelMessages{InstructionsTemplate: openai.CodexBaseInstructionsForModel(modelID)},
		SupportsReasoningSummaryParameter: true,
		DefaultReasoningSummary:           "auto",
		WebSearchToolType:                 "text",
		TruncationPolicy:                  configuredCodexTruncationPolicy{Mode: "bytes", Limit: configuredCodexToolOutputMaxTokens},
		ContextWindow:                     configuredCodexFallbackContext,
		MaxContextWindow:                  configuredCodexFallbackContext,
		EffectiveContextWindowPercent:     95,
		ExperimentalSupportedTools:        []string{},
		InputModalities:                   []string{"text"},
	}

	if isDeepSeekCodexModel(modelID) {
		defaultReasoningLevel := "high"
		descriptor.DisplayName = deepSeekCodexDisplayName(modelID)
		descriptor.Description = "DeepSeek coding and reasoning model routed through Sub2API."
		descriptor.DefaultReasoningLevel = &defaultReasoningLevel
		descriptor.SupportedReasoningLevels = []configuredCodexReasoningLevel{
			{Effort: "low", Description: "Fast responses with lighter reasoning"},
			{Effort: "high", Description: "Greater reasoning depth for coding and agent tasks"},
			{Effort: "max", Description: "Maximum reasoning depth for complex tasks"},
		}
		descriptor.SupportsParallelToolCalls = true
		descriptor.ContextWindow = configuredCodexDeepSeekV4Context
		descriptor.MaxContextWindow = configuredCodexDeepSeekV4Context
	}

	if isGrokCodexModel(modelID) {
		descriptor.DisplayName = grokCodexDisplayName(modelID)
		descriptor.Description = "Grok coding and reasoning model routed through Sub2API."
		descriptor.SupportsParallelToolCalls = true
		descriptor.ContextWindow = grokCodexContextWindow(modelID)
		descriptor.MaxContextWindow = descriptor.ContextWindow
		if grokCodexSupportsReasoningEffort(modelID) {
			defaultReasoningLevel := "high"
			descriptor.DefaultReasoningLevel = &defaultReasoningLevel
			descriptor.SupportedReasoningLevels = configuredCodexGrokReasoningLevels(modelID)
		}
	}

	if isClaudeCodexModel(modelID) {
		descriptor.DisplayName = claudeCodexDisplayName(modelID)
		descriptor.Description = "Claude coding and reasoning model routed through Sub2API."
		descriptor.SupportsParallelToolCalls = true
		if levels := configuredCodexClaudeReasoningLevels(modelID); len(levels) > 0 {
			defaultReasoningLevel := claudeCodexDefaultReasoningLevel(levels)
			descriptor.DefaultReasoningLevel = &defaultReasoningLevel
			descriptor.SupportedReasoningLevels = levels
		}
	}

	if isOpenAICodexGPTModel(modelID) {
		descriptor.DisplayName = openaiCodexDisplayName(modelID)
		descriptor.Description = "OpenAI GPT coding model routed through Sub2API."
		descriptor.SupportsParallelToolCalls = true
		if isOpenAICodexReasoningGPTModel(modelID) {
			defaultReasoningLevel := "medium"
			if getNormalizedCodexModel(modelID) == "gpt-5.6-sol" {
				defaultReasoningLevel = "low"
			}
			descriptor.DefaultReasoningLevel = &defaultReasoningLevel
			descriptor.SupportedReasoningLevels = configuredCodexGPTReasoningLevels(modelID)
			descriptor.DefaultReasoningSummary = "none"
			descriptor.TruncationPolicy = configuredCodexTruncationPolicy{Mode: "tokens", Limit: configuredCodexToolOutputMaxTokens}
			if isOpenAIGPT56Model(modelID) {
				descriptor.MaxContextWindow = configuredCodexGPT56MaxContext
			}
		}
		if SupportsVerbosity(modelID) {
			defaultVerbosity := "low"
			descriptor.SupportVerbosity = true
			descriptor.DefaultVerbosity = &defaultVerbosity
		}
	}

	return descriptor
}

func configuredCodexGrokReasoningLevels(modelID string) []configuredCodexReasoningLevel {
	levels := []configuredCodexReasoningLevel{
		{Effort: "low", Description: "Fast responses with lighter reasoning"},
		{Effort: "medium", Description: "Balanced reasoning for most coding tasks"},
		{Effort: "high", Description: "Greater reasoning depth for coding and agent tasks"},
	}
	if GrokSupportsXHighReasoningEffort(modelID) {
		levels = append(levels, configuredCodexReasoningLevel{
			Effort:      "xhigh",
			Description: "Extra-high reasoning depth for difficult tasks",
		})
	}
	return levels
}

func configuredCodexClaudeReasoningLevels(modelID string) []configuredCodexReasoningLevel {
	descriptions := map[string]string{
		"low":    "Fast responses with lighter reasoning",
		"medium": "Balanced reasoning for most coding tasks",
		"high":   "Greater reasoning depth for coding and agent tasks",
		"xhigh":  "Extra-high reasoning depth for difficult tasks",
		"max":    "Maximum reasoning depth for complex tasks",
	}
	levels := claude.EffortLevelsForModel(modelID)
	out := make([]configuredCodexReasoningLevel, 0, len(levels))
	for _, effort := range levels {
		out = append(out, configuredCodexReasoningLevel{
			Effort:      effort,
			Description: descriptions[effort],
		})
	}
	return out
}

func claudeCodexDefaultReasoningLevel(levels []configuredCodexReasoningLevel) string {
	for _, preferred := range []string{"medium", "high", "low"} {
		for _, level := range levels {
			if level.Effort == preferred {
				return preferred
			}
		}
	}
	if len(levels) == 0 {
		return ""
	}
	return levels[0].Effort
}

func configuredCodexGPTReasoningLevels(modelID string) []configuredCodexReasoningLevel {
	levels := []configuredCodexReasoningLevel{
		{Effort: "low", Description: "Fast responses with lighter reasoning"},
		{Effort: "medium", Description: "Balanced reasoning for most coding tasks"},
		{Effort: "high", Description: "Greater reasoning depth for coding and agent tasks"},
		{Effort: "xhigh", Description: "Extra-high reasoning depth for difficult tasks"},
	}
	normalized := getNormalizedCodexModel(modelID)
	if isOpenAIGPT56Model(modelID) {
		levels = append(levels, configuredCodexReasoningLevel{
			Effort:      "max",
			Description: "Maximum reasoning depth for complex tasks",
		})
	}
	if normalized == "gpt-5.6-sol" || normalized == "gpt-5.6-terra" {
		levels = append(levels, configuredCodexReasoningLevel{
			Effort:      "ultra",
			Description: "Maximum reasoning with automatic task delegation",
		})
	}
	return levels
}

func isOpenAICodexGPTModel(modelID string) bool {
	normalized := canonicalizeOpenAIModelAliasSpelling(modelID)
	if normalized == "" || strings.HasPrefix(normalized, "gpt-image") {
		return false
	}
	return strings.HasPrefix(normalized, "gpt-")
}

func isOpenAICodexReasoningGPTModel(modelID string) bool {
	normalized := canonicalizeOpenAIModelAliasSpelling(modelID)
	return strings.HasPrefix(normalized, "gpt-5")
}

func isOpenAICodexImageInputModel(modelID string) bool {
	normalized := canonicalizeOpenAIModelAliasSpelling(modelID)
	return strings.HasPrefix(normalized, "gpt-5") ||
		strings.HasPrefix(normalized, "gpt-4o") ||
		strings.HasPrefix(normalized, "gpt-4.1") ||
		strings.HasPrefix(normalized, "gpt-4.5") ||
		strings.HasPrefix(normalized, "gpt-4-turbo") ||
		strings.HasPrefix(normalized, "gpt-4-vision")
}

func isOfficialOpenAICodexCatalogModel(modelID string) bool {
	normalized := strings.ToLower(codexProviderQualifiedModelID(modelID))
	if normalized == "" || isCodexDedicatedMediaModel(normalized) {
		return false
	}
	if strings.HasPrefix(normalized, "codex-") {
		return true
	}
	if strings.HasPrefix(normalized, "o1") || strings.HasPrefix(normalized, "o3") || strings.HasPrefix(normalized, "o4") {
		return true
	}
	if !strings.HasPrefix(normalized, "gpt-") {
		return false
	}
	for _, incompatibleFamily := range []string{"audio", "realtime", "transcribe", "tts"} {
		if strings.Contains(normalized, incompatibleFamily) {
			return false
		}
	}
	return true
}

func openaiCodexDisplayName(modelID string) string {
	normalized := canonicalizeOpenAIModelAliasSpelling(modelID)
	if normalized == "" {
		return modelID
	}
	for _, model := range openai.DefaultModels {
		if strings.EqualFold(model.ID, normalized) && strings.TrimSpace(model.DisplayName) != "" {
			return model.DisplayName
		}
	}
	return modelID
}

func deepSeekCodexDisplayName(modelID string) string {
	switch strings.ToLower(strings.TrimSpace(modelID)) {
	case "deepseek-v4-pro", "deepseek-4-pro":
		return "DeepSeek V4 Pro"
	case "deepseek-v4-flash", "deepseek-4-flash":
		return "DeepSeek V4 Flash"
	default:
		return modelID
	}
}

func isDeepSeekCodexModel(modelID string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelID)), "deepseek-")
}

func isGrokCodexModel(modelID string) bool {
	return xai.IsGrokModelID(modelID)
}

func grokCodexSupportsReasoningEffort(modelID string) bool {
	if grokSupportsReasoningEffort(modelID) {
		return true
	}
	canonical := xai.ResolveGrokTextResponsesModelID(modelID)
	if canonical == "" || strings.EqualFold(canonical, modelID) {
		return false
	}
	return grokSupportsReasoningEffort(canonical)
}

func grokCodexDisplayName(modelID string) string {
	normalized := strings.ToLower(xai.StripGrokProviderPrefix(strings.TrimSpace(modelID)))
	if normalized == "" {
		return modelID
	}
	if name := grokDefaultDisplayName(normalized); name != "" {
		return name
	}
	canonical := strings.ToLower(xai.ResolveGrokTextResponsesModelID(normalized))
	if canonical != "" && canonical != normalized {
		if name := grokDefaultDisplayName(canonical); name != "" {
			return name
		}
	}
	return modelID
}

func grokDefaultDisplayName(modelID string) string {
	for _, model := range xai.DefaultModels() {
		if model.ID == modelID {
			return strings.TrimSpace(model.DisplayName)
		}
	}
	return ""
}

func grokCodexContextWindow(modelID string) int64 {
	normalized := strings.ToLower(xai.StripGrokProviderPrefix(strings.TrimSpace(modelID)))
	if strings.HasPrefix(normalized, "grok-build") {
		return configuredCodexGrokBuildContext
	}
	return configuredCodexGrokContext
}

func isClaudeCodexModel(modelID string) bool {
	platform, detected := DetectModelPlatform(modelID)
	return detected && platform == PlatformAnthropic
}

func claudeCodexDisplayName(modelID string) string {
	normalized := strings.ToLower(codexProviderQualifiedModelID(modelID))
	normalized = strings.TrimPrefix(normalized, "anthropic.")
	if normalized == "" {
		return modelID
	}
	for _, model := range claude.DefaultModels {
		if strings.EqualFold(model.ID, normalized) && strings.TrimSpace(model.DisplayName) != "" {
			return model.DisplayName
		}
	}
	if canonical, ok := claude.ModelIDOverrides[normalized]; ok {
		for _, model := range claude.DefaultModels {
			if model.ID == canonical && strings.TrimSpace(model.DisplayName) != "" {
				return model.DisplayName
			}
		}
	}
	return modelID
}

// BuildCodexModelsManifest builds a standalone Codex model catalog for models
// routed through a custom provider. The response is also suitable for saving
// as model_catalog_json in clients that do not refresh custom-provider catalogs.
func BuildCodexModelsManifest(modelIDs []string) ([]byte, error) {
	return buildCodexModelsManifest(modelIDs, nil, nil, nil)
}

// BuildCodexModelsManifestForGroup derives input capabilities from the
// concrete Responses route and group accounts behind a group. Unknown or mixed
// capabilities fail closed to the text-only descriptor used by the standalone
// builder. Caller-supplied model IDs still decide which slugs appear; advertised
// capabilities intersect all active group members that map the alias, including
// accounts that are not currently schedulable.
func (s *GatewayService) BuildCodexModelsManifestForGroup(
	ctx context.Context,
	group *Group,
	platformOverride string,
	modelIDs []string,
) ([]byte, error) {
	if s == nil || s.accountRepo == nil || group == nil {
		return BuildCodexModelsManifest(modelIDs)
	}
	effectivePlatform := strings.TrimSpace(platformOverride)
	if effectivePlatform == "" {
		effectivePlatform = group.Platform
	}
	if effectivePlatform != PlatformComposite && !isConcreteRequestPlatform(effectivePlatform) {
		return BuildCodexModelsManifest(modelIDs)
	}

	_, catalog, err := loadCodexGroupCatalogAccounts(ctx, s.accountRepo, group.ID)
	if err != nil {
		return BuildCodexModelsManifest(modelIDs)
	}
	var compositeRoutes []CompositeModelRoute
	compositeRoutesAvailable := true
	if effectivePlatform == PlatformComposite && s.compositeResolver != nil && s.compositeResolver.repo != nil {
		compositeRoutes, err = s.compositeResolver.repo.ListByGroup(ctx, group.ID, false)
		if err != nil {
			compositeRoutesAvailable = false
		}
	}
	return buildCodexModelsManifestForAccounts(
		effectivePlatform,
		modelIDs,
		catalog,
		compositeRoutes,
		compositeRoutesAvailable,
	)
}

func buildCodexModelsManifestForAccounts(
	effectivePlatform string,
	modelIDs []string,
	accounts []Account,
	compositeRoutes []CompositeModelRoute,
	compositeRoutesAvailable bool,
) ([]byte, error) {
	imageInputModels := make(map[string]bool, len(modelIDs))
	metadataModels := codexCatalogMetadataModels(
		effectivePlatform,
		modelIDs,
		accounts,
		compositeRoutes,
		compositeRoutesAvailable,
	)
	modelMetadata := make(map[string]codexModelMetadataOverride, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if groupCodexModelSupportsImageInput(
			effectivePlatform,
			modelID,
			accounts,
			compositeRoutes,
			compositeRoutesAvailable,
		) {
			imageInputModels[modelID] = true
		}
		if metadata, ok := groupCodexModelMetadata(
			effectivePlatform,
			modelID,
			accounts,
			compositeRoutes,
			compositeRoutesAvailable,
		); ok {
			modelMetadata[modelID] = metadata
		}
	}
	return buildCodexModelsManifest(modelIDs, imageInputModels, metadataModels, modelMetadata)
}

func buildCodexModelsManifest(
	modelIDs []string,
	imageInputModels map[string]bool,
	metadataModels map[string]string,
	modelMetadata map[string]codexModelMetadataOverride,
) ([]byte, error) {
	seen := make(map[string]struct{}, len(modelIDs))
	models := make([]configuredCodexModelDescriptor, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		if _, exists := seen[modelID]; exists {
			continue
		}
		metadataModelID := strings.TrimSpace(metadataModels[modelID])
		if metadataModelID == "" {
			metadataModelID = modelID
		}
		if isCodexDedicatedMediaModel(modelID) || isCodexDedicatedMediaModel(metadataModelID) {
			continue
		}
		seen[modelID] = struct{}{}
		descriptor := newConfiguredCodexModelDescriptor(metadataModelID)
		descriptor.Slug = modelID
		if imageInputModels[modelID] {
			descriptor.InputModalities = []string{"text", "image"}
		}
		if metadata, ok := modelMetadata[modelID]; ok {
			applyUpstreamModelMetadataToCodexDescriptor(&descriptor, metadata)
		}
		if metadataModelID != modelID {
			descriptor.DisplayName = modelID
			descriptor.Description = configuredCodexCustomDescription
		}
		models = append(models, descriptor)
	}
	return json.Marshal(struct {
		Models []configuredCodexModelDescriptor `json:"models"`
	}{Models: models})
}

func codexCatalogMetadataModels(
	platform string,
	modelIDs []string,
	accounts []Account,
	compositeRoutes []CompositeModelRoute,
	compositeRoutesAvailable bool,
) map[string]string {
	metadataModels := make(map[string]string, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		metadataModelID := resolveCodexCatalogMetadataModel(
			platform,
			modelID,
			accounts,
			compositeRoutes,
			compositeRoutesAvailable,
		)
		if metadataModelID != "" && metadataModelID != modelID {
			metadataModels[modelID] = metadataModelID
		}
	}
	return metadataModels
}

func resolveCodexCatalogMetadataModel(
	platform string,
	modelID string,
	accounts []Account,
	compositeRoutes []CompositeModelRoute,
	compositeRoutesAvailable bool,
) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	if platform == PlatformComposite {
		if !compositeRoutesAvailable {
			return modelID
		}
		if route, matched := matchCompositeRoute(compositeRoutes, modelID, CompositeRouteEndpointResponses); matched {
			if upstreamModel := strings.TrimSpace(route.UpstreamModel); upstreamModel != "" {
				return upstreamModel
			}
			return modelID
		}
		if codexCompositeRouteMatchesModel(compositeRoutes, modelID) {
			return modelID
		}

		claimedPlatforms := make(map[string]struct{})
		for _, account := range accounts {
			accountPlatform := strings.TrimSpace(account.Platform)
			if !isConcreteRequestPlatform(accountPlatform) || !codexExplicitModelMappingClaims(account, modelID) {
				continue
			}
			claimedPlatforms[accountPlatform] = struct{}{}
		}
		if len(claimedPlatforms) > 1 {
			return modelID
		}
		for accountPlatform := range claimedPlatforms {
			return uniqueCodexMappedModel(accounts, accountPlatform, modelID)
		}

		detectedPlatform, detected := DetectModelPlatform(modelID)
		if !detected {
			return modelID
		}
		platform = detectedPlatform
	}
	return uniqueCodexMappedModel(accounts, platform, modelID)
}

func uniqueCodexMappedModel(accounts []Account, platform string, modelID string) string {
	targets := make(map[string]struct{})
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != platform {
			continue
		}
		mappedModel, matched := account.ResolveMappedModel(modelID)
		mappedModel = strings.TrimSpace(mappedModel)
		if !matched || mappedModel == "" {
			continue
		}
		targets[mappedModel] = struct{}{}
	}
	if len(targets) != 1 {
		return modelID
	}
	for target := range targets {
		return target
	}
	return modelID
}

func groupCodexModelSupportsImageInput(
	platform string,
	modelID string,
	accounts []Account,
	compositeRoutes []CompositeModelRoute,
	compositeRoutesAvailable bool,
) bool {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return false
	}
	upstreamModel := modelID
	if platform == PlatformComposite {
		var resolved bool
		platform, upstreamModel, resolved = resolveCodexCompositeModelTarget(
			modelID,
			accounts,
			compositeRoutes,
			compositeRoutesAvailable,
		)
		if !resolved {
			return false
		}
	}
	if platform != PlatformOpenAI && platform != PlatformGrok {
		return false
	}

	candidates := 0
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != platform || !account.IsModelSupported(upstreamModel) {
			continue
		}
		candidates++
		if !accountCodexModelSupportsImageInput(account, account.GetMappedModel(upstreamModel)) {
			return false
		}
	}
	return candidates > 0
}

func resolveCodexCompositeModelTarget(
	modelID string,
	accounts []Account,
	routes []CompositeModelRoute,
	routesAvailable bool,
) (string, string, bool) {
	if !routesAvailable {
		return "", "", false
	}
	if route, matched := matchCompositeRoute(routes, modelID, CompositeRouteEndpointResponses); matched {
		upstreamModel := strings.TrimSpace(route.UpstreamModel)
		if upstreamModel == "" {
			upstreamModel = modelID
		}
		return route.TargetPlatform, upstreamModel, true
	}
	if codexCompositeRouteMatchesModel(routes, modelID) {
		return "", "", false
	}

	claimedPlatforms := make(map[string]struct{})
	for _, account := range accounts {
		platform := strings.TrimSpace(account.Platform)
		if !isConcreteRequestPlatform(platform) || !codexExplicitModelMappingClaims(account, modelID) {
			continue
		}
		claimedPlatforms[platform] = struct{}{}
	}
	if len(claimedPlatforms) > 1 {
		return "", "", false
	}
	for platform := range claimedPlatforms {
		return platform, modelID, true
	}

	platform, detected := DetectModelPlatform(modelID)
	if !detected {
		return "", "", false
	}
	return platform, modelID, true
}

func codexCompositeRouteMatchesModel(routes []CompositeModelRoute, modelID string) bool {
	for _, route := range routes {
		publicModel := strings.TrimSpace(route.PublicModel)
		if publicModel == "" {
			continue
		}
		switch normalizeCompositeRouteMatchType(route.MatchType) {
		case CompositeRouteMatchPrefix:
			if strings.HasPrefix(modelID, publicModel) {
				return true
			}
		default:
			if modelID == publicModel {
				return true
			}
		}
	}
	return false
}

func codexExplicitModelMappingClaims(account Account, modelID string) bool {
	if account.Credentials == nil || strings.TrimSpace(modelID) == "" {
		return false
	}
	mapped := strings.TrimSpace(account.GetModelMapping()[modelID])
	return mapped != ""
}

func accountCodexModelSupportsImageInput(account *Account, upstreamModel string) bool {
	if account == nil {
		return false
	}
	switch account.Platform {
	case PlatformOpenAI:
		if !isOpenAICodexImageInputModel(upstreamModel) {
			return false
		}
		if account.IsOpenAIOAuth() {
			return true
		}
		if !account.IsOpenAIApiKey() {
			return false
		}
		baseURL := strings.TrimSpace(account.GetCredential("base_url"))
		return baseURL == "" || isOfficialOpenAIModelsBaseURL(baseURL)
	case PlatformGrok:
		if !isOfficialGrokCodexBaseURL(account.GetGrokBaseURL()) {
			return false
		}
		canonical := xai.ResolveGrokTextResponsesModelID(upstreamModel)
		return isGrokCodexImageInputModel(canonical)
	default:
		return false
	}
}

func isGrokCodexImageInputModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "grok-4.3",
		"grok-4.5",
		"grok-4.6",
		"grok-build-0.1",
		"grok-4.20-0309-reasoning",
		"grok-4.20-0309-non-reasoning",
		"grok-4.20-multi-agent-0309":
		return true
	default:
		return false
	}
}

func isOfficialGrokCodexBaseURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return false
	}
	return xai.IsOfficialBaseURLHost(strings.TrimSuffix(parsed.Hostname(), "."))
}

// BuildDeepSeekCodexModelsManifest preserves the historical entry point for
// callers that still use the provider-specific function name.
func BuildDeepSeekCodexModelsManifest(modelIDs []string) ([]byte, error) {
	return BuildCodexModelsManifest(modelIDs)
}

func mergeConfiguredCodexModelsManifest(
	body []byte,
	configuredModels []string,
	selectedModels []string,
	filterBySelection bool,
) ([]byte, bool, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, false, err
	}
	var upstreamModels []json.RawMessage
	if err := json.Unmarshal(envelope["models"], &upstreamModels); err != nil {
		return nil, false, err
	}

	selected := make(map[string]struct{}, len(selectedModels))
	for _, modelID := range selectedModels {
		modelID = strings.TrimSpace(modelID)
		if modelID != "" {
			selected[modelID] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(upstreamModels)+len(configuredModels))
	merged := make([]json.RawMessage, 0, len(upstreamModels)+len(configuredModels))
	changed := false
	for _, rawModel := range upstreamModels {
		var descriptor struct {
			Slug string `json:"slug"`
		}
		if err := json.Unmarshal(rawModel, &descriptor); err != nil || strings.TrimSpace(descriptor.Slug) == "" {
			if filterBySelection {
				changed = true
				continue
			}
			merged = append(merged, rawModel)
			continue
		}
		descriptor.Slug = strings.TrimSpace(descriptor.Slug)
		if isCodexDedicatedMediaModel(descriptor.Slug) {
			changed = true
			continue
		}
		if filterBySelection {
			if _, allowed := selected[descriptor.Slug]; !allowed {
				changed = true
				continue
			}
		}
		if strings.HasPrefix(descriptor.Slug, codexAutoModelPrefix) {
			_, explicitlyEnabled := selected[descriptor.Slug]
			explicitlyEnabled = filterBySelection && explicitlyEnabled
			if !explicitlyEnabled {
				changed = true
				continue
			}
			visibleModel, visibilityChanged, err := codexModelWithVisibility(rawModel, "list")
			if err != nil {
				return nil, false, err
			}
			rawModel = visibleModel
			changed = changed || visibilityChanged
		}
		seen[descriptor.Slug] = struct{}{}
		merged = append(merged, rawModel)
	}

	for _, modelID := range configuredModels {
		if isCodexDedicatedMediaModel(modelID) {
			continue
		}
		if filterBySelection {
			if _, allowed := selected[modelID]; !allowed {
				continue
			}
		}
		if strings.HasPrefix(modelID, codexAutoModelPrefix) {
			if _, explicitlyEnabled := selected[modelID]; !filterBySelection || !explicitlyEnabled {
				continue
			}
		}
		if _, exists := seen[modelID]; exists {
			continue
		}
		rawModel, err := json.Marshal(newConfiguredCodexModelDescriptor(modelID))
		if err != nil {
			return nil, false, err
		}
		merged = append(merged, rawModel)
		seen[modelID] = struct{}{}
		changed = true
	}
	if !changed {
		return body, false, nil
	}

	rawModels, err := json.Marshal(merged)
	if err != nil {
		return nil, false, err
	}
	envelope["models"] = rawModels
	mergedBody, err := json.Marshal(envelope)
	if err != nil {
		return nil, false, err
	}
	return mergedBody, true, nil
}

func codexModelWithVisibility(rawModel json.RawMessage, visibility string) (json.RawMessage, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawModel, &fields); err != nil {
		return nil, false, err
	}
	var current string
	if rawVisibility, ok := fields["visibility"]; ok {
		if err := json.Unmarshal(rawVisibility, &current); err == nil && current == visibility {
			return rawModel, false, nil
		}
	}
	rawVisibility, err := json.Marshal(visibility)
	if err != nil {
		return nil, false, err
	}
	fields["visibility"] = rawVisibility
	updated, err := json.Marshal(fields)
	if err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

type codexModelsManifestUpstreamError struct {
	err        error
	retryable  bool
	statusCode int
	headers    http.Header
	body       []byte
}

func (e *codexModelsManifestUpstreamError) Error() string { return e.err.Error() }

func (e *codexModelsManifestUpstreamError) Unwrap() error { return e.err }

// IsRetryableCodexModelsManifestError reports whether another selected account
// may succeed without changing the request. API key upstream 404/405 responses
// mean that the selected account does not expose a model-discovery endpoint, so
// another account may still serve the manifest. Other upstream 4xx responses,
// except 429 and ChatGPT-backend 401, are intentionally not retried. A manifest
// 401 from the ChatGPT Codex backend reflects the selected OAuth account's
// upstream token rather than the client request (the client's own API key was
// already validated locally). Custom API key upstreams keep the no-failover 401
// behavior because their /models auth semantics are not authoritative for the
// account.
func IsRetryableCodexModelsManifestError(err error) bool {
	var upstreamErr *codexModelsManifestUpstreamError
	return errors.As(err, &upstreamErr) && upstreamErr.retryable
}

func isRetryableCodexModelsManifestStatus(statusCode int, useAPIKeyUpstream bool) bool {
	return (useAPIKeyUpstream &&
		(statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed)) ||
		(statusCode == http.StatusUnauthorized && !useAPIKeyUpstream) ||
		statusCode == http.StatusTooManyRequests ||
		(statusCode >= http.StatusInternalServerError && statusCode < 600)
}

func isRetryableCodexModelsManifestTransportError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) {
		return true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var goAwayErr http2.GoAwayError
	if errors.As(err, &goAwayErr) {
		return true
	}
	var streamErr http2.StreamError
	if errors.As(err, &streamErr) {
		return true
	}
	var connectionErr http2.ConnectionError
	if errors.As(err, &connectionErr) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// net/http uses unexported HTTP/2 error types, so typed matching is not
	// possible for errors produced by the standard library transport.
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "http2:") &&
		(strings.Contains(message, "goaway") ||
			strings.Contains(message, "refused_stream") ||
			strings.Contains(message, "frame too large")) {
		return true
	}
	if strings.Contains(message, "stream error: stream id ") {
		return true
	}
	for _, code := range []http2.ErrCode{
		http2.ErrCodeNo,
		http2.ErrCodeProtocol,
		http2.ErrCodeInternal,
		http2.ErrCodeFlowControl,
		http2.ErrCodeSettingsTimeout,
		http2.ErrCodeStreamClosed,
		http2.ErrCodeFrameSize,
		http2.ErrCodeRefusedStream,
		http2.ErrCodeCancel,
		http2.ErrCodeCompression,
		http2.ErrCodeConnect,
		http2.ErrCodeEnhanceYourCalm,
		http2.ErrCodeInadequateSecurity,
		http2.ErrCodeHTTP11Required,
	} {
		if strings.Contains(message, "connection error: "+strings.ToLower(code.String())) {
			return true
		}
	}
	return false
}

type codexModelsManifestRequest struct {
	url                 string
	headers             http.Header
	proxyURL            string
	accountID           int64
	credentialAccountID int64
	credentialAccount   *Account
	accountConcurrency  int
	useAPIKeyUpstream   bool
}

type codexModelsManifestCacheEntry struct {
	manifest   *CodexModelsManifest
	order      uint64
	expiresAt  time.Time
	staleUntil time.Time
}

type codexModelsManifestCacheState uint8

const (
	codexModelsManifestCacheMiss codexModelsManifestCacheState = iota
	codexModelsManifestCacheFresh
	codexModelsManifestCacheStale
)

type codexModelsManifestCache struct {
	mu        sync.Mutex
	entries   map[string]codexModelsManifestCacheEntry
	nextOrder uint64
	refresh   singleflight.Group
}

func (c *codexModelsManifestCache) get(key string, now time.Time) (*CodexModelsManifest, codexModelsManifestCacheState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, codexModelsManifestCacheMiss
	}
	if !now.Before(entry.staleUntil) {
		delete(c.entries, key)
		return nil, codexModelsManifestCacheMiss
	}
	if now.Before(entry.expiresAt) {
		return entry.manifest, codexModelsManifestCacheFresh
	}
	return entry.manifest, codexModelsManifestCacheStale
}

func (c *codexModelsManifestCache) set(key string, manifest *CodexModelsManifest, now time.Time) {
	if manifest == nil || len(manifest.Body) > codexModelsManifestCacheBodyLimit {
		return
	}
	remainingBodyBudget := codexModelsManifestCacheBodyLimit - len(manifest.Body)
	if len(manifest.upstreamSourceBody) > remainingBodyBudget {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]codexModelsManifestCacheEntry)
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= codexModelsManifestCacheMaxEntries {
		oldestKey := ""
		var oldestOrder uint64
		for candidateKey, entry := range c.entries {
			if !now.Before(entry.staleUntil) {
				delete(c.entries, candidateKey)
				continue
			}
			if oldestKey == "" || entry.order < oldestOrder {
				oldestKey = candidateKey
				oldestOrder = entry.order
			}
		}
		if len(c.entries) >= codexModelsManifestCacheMaxEntries && oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}
	c.nextOrder++
	c.entries[key] = codexModelsManifestCacheEntry{
		manifest:   manifest,
		order:      c.nextOrder,
		expiresAt:  now.Add(codexModelsManifestCacheTTL),
		staleUntil: now.Add(codexModelsManifestCacheStaleTTL),
	}
}

// FetchCodexModelsManifest fetches the live Codex models manifest from either
// the ChatGPT backend for OAuth accounts or a custom upstream for API key accounts.
//
// After validating the stable top-level envelope, OAuth response bodies are
// passed through verbatim. Custom API key manifests receive only the narrowly
// scoped compatibility adjustments required by custom-provider Codex clients.
func (s *OpenAIGatewayService) FetchCodexModelsManifest(ctx context.Context, account *Account, clientVersion, ifNoneMatch string) (*CodexModelsManifest, error) {
	if account == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_ACCOUNT_REQUIRED", "account is required")
	}
	credAccount, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_CREDENTIALS_FAILED", "resolve credential account: %v", err)
	}

	clientVersion = strings.TrimSpace(clientVersion)
	if clientVersion == "" {
		clientVersion = CodexCanonicalClientVersion()
	}

	requestEndpoint := chatgptCodexModelsURL
	authToken := ""
	useAPIKeyUpstream := false
	appendModelsPath := false
	switch {
	case credAccount.IsOpenAIOAuth():
		authToken = strings.TrimSpace(credAccount.GetOpenAIAccessToken())
		if authToken == "" && !credAccount.IsOpenAIAgentIdentity() {
			return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_MODELS_TOKEN_MISSING", "account has no Codex backend access token")
		}
	case credAccount.IsOpenAIApiKey():
		baseURL := strings.TrimSpace(credAccount.GetOpenAIBaseURL())
		authToken = strings.TrimSpace(credAccount.GetOpenAIApiKey())
		if authToken == "" {
			return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_MODELS_API_KEY_MISSING", "account has no API key for the Codex models upstream")
		}
		normalizedBaseURL, validateErr := s.validateUpstreamBaseURL(baseURL)
		if validateErr != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_API_KEY_UPSTREAM_INVALID", "invalid Codex models upstream base URL: %v", validateErr)
		}
		requestEndpoint = normalizedBaseURL
		useAPIKeyUpstream = true
		appendModelsPath = true
	default:
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_ACCOUNT_TYPE_UNSUPPORTED", "account type %q cannot fetch the Codex models manifest", credAccount.Type)
	}

	requestURL, err := buildCodexModelsManifestURL(requestEndpoint, appendModelsPath, clientVersion)
	if err != nil {
		if useAPIKeyUpstream {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_API_KEY_UPSTREAM_INVALID", "invalid Codex models upstream base URL: %v", err)
		}
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_REQUEST_FAILED", "parse codex models request URL: %v", err)
	}

	headers := make(http.Header)
	if useAPIKeyUpstream {
		headers.Set("Authorization", "Bearer "+authToken)
		credAccount.ApplyHeaderOverrides(headers)
	} else {
		authHeaders, authErr := s.buildOpenAIAuthenticationHeaders(ctx, credAccount, authToken)
		if authErr != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_AUTH_FAILED", "build Codex models authentication: %v", authErr)
		}
		for key, values := range authHeaders {
			for _, value := range values {
				headers.Add(key, value)
			}
		}
		setOpenAIChatGPTAccountHeaders(headers, credAccount)
	}
	headers.Set("Accept", "application/json")
	overrideUA := ""
	if !useAPIKeyUpstream {
		overrideUA = credAccount.GetOpenAIUserAgent()
	}
	identity := resolveCodexOutboundIdentity(overrideUA)
	headers.Set("Originator", identity.originator)
	headers.Set("User-Agent", identity.userAgent)
	// Version 头优先与 client_version 查询参数同源：客户端自报版本合法且不低于上游
	// 门槛时原样使用；否则回退规范版本，避免陈旧 version 触发上游 404（issue #3901）。
	// client_version 查询参数本身始终按客户端原值透传（内容协商语义，契约见
	// TestFetchCodexModelsManifestPassthrough）。
	headerVersion := NormalizeCodexClientVersion(clientVersion)
	if headerVersion == "" || CompareVersions(headerVersion, codexUpstreamMinVersion) < 0 {
		headerVersion = identity.version
	}
	headers.Set("Version", headerVersion)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	request := codexModelsManifestRequest{
		url:                 requestURL.String(),
		headers:             headers,
		proxyURL:            proxyURL,
		accountID:           account.ID,
		credentialAccountID: credAccount.ID,
		credentialAccount:   credAccount,
		accountConcurrency:  account.Concurrency,
		useAPIKeyUpstream:   useAPIKeyUpstream,
	}
	if useAPIKeyUpstream {
		return s.fetchCachedAPIKeyCodexModelsManifest(ctx, request, ifNoneMatch)
	}
	manifest, fetchErr := s.fetchCodexModelsManifestUpstream(ctx, request, ifNoneMatch)
	if !credAccount.IsOpenAIAgentIdentity() || !isAgentIdentityTaskInvalidCodexModelsError(fetchErr) {
		s.handleCodexModelsManifestAccountAuthError(ctx, account, credAccount, fetchErr)
		return manifest, fetchErr
	}
	expectedTaskID := strings.TrimSpace(credAccount.GetCredential("task_id"))
	if recoverErr := s.recoverAgentIdentityTask(ctx, credAccount, expectedTaskID); recoverErr != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_AUTH_FAILED", "agent identity task recovery failed: %v", recoverErr)
	}
	authHeaders, authErr := s.buildOpenAIAuthenticationHeaders(ctx, credAccount, "")
	if authErr != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_AUTH_FAILED", "build Codex models authentication after task recovery: %v", authErr)
	}
	request.headers.Del("Authorization")
	request.headers.Del("ChatGPT-Account-ID")
	for key, values := range authHeaders {
		for _, value := range values {
			request.headers.Add(key, value)
		}
	}
	setOpenAIChatGPTAccountHeaders(request.headers, credAccount)
	return s.fetchCodexModelsManifestUpstream(ctx, request, ifNoneMatch)
}

func isAgentIdentityTaskInvalidCodexModelsError(err error) bool {
	var upstreamErr *codexModelsManifestUpstreamError
	return errors.As(err, &upstreamErr) &&
		isAgentIdentityTaskInvalidHTTPResponse(upstreamErr.statusCode, upstreamErr.body)
}

// handleCodexModelsManifestAccountAuthError feeds manifest 401s from the
// ChatGPT Codex backend into the shared upstream-error state machinery
// (token cache invalidation, temp-unschedulable cooldown, or permanent
// disable for token_revoked/token_invalidated). Without this, an account
// whose OAuth token was revoked upstream stays active and schedulable and
// keeps being selected for every subsequent /models request (#4544).
//
// Scope is deliberately limited to plain OAuth accounts: the manifest
// endpoint authenticates with the same token as /responses forwarding, so a
// 401 is authoritative for the account. Agent Identity accounts are excluded
// because their 401s can be task-scoped and have a dedicated recovery flow,
// and API key manifests come from custom upstreams whose /models auth may
// diverge from their chat endpoints.
func (s *OpenAIGatewayService) handleCodexModelsManifestAccountAuthError(ctx context.Context, account, credAccount *Account, err error) {
	if s == nil || account == nil || err == nil {
		return
	}
	if credAccount == nil || !credAccount.IsOpenAIOAuth() || credAccount.IsOpenAIAgentIdentity() {
		return
	}
	var upstreamErr *codexModelsManifestUpstreamError
	if !errors.As(err, &upstreamErr) || upstreamErr.statusCode != http.StatusUnauthorized {
		return
	}
	headers := upstreamErr.headers
	if headers == nil {
		headers = http.Header{}
	}
	s.handleOpenAIAccountUpstreamError(ctx, account, upstreamErr.statusCode, headers, upstreamErr.body)
}

func (s *OpenAIGatewayService) fetchCachedAPIKeyCodexModelsManifest(ctx context.Context, request codexModelsManifestRequest, ifNoneMatch string) (*CodexModelsManifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cacheKey := buildCodexModelsManifestCacheKey(request)
	manifest, state := s.codexModelsManifestCache.get(cacheKey, time.Now())
	if state == codexModelsManifestCacheFresh {
		return codexModelsManifestForClient(manifest, ifNoneMatch), nil
	}
	resultCh := s.refreshCachedAPIKeyCodexModelsManifest(cacheKey, request)
	if state == codexModelsManifestCacheStale {
		return codexModelsManifestForClient(manifest, ifNoneMatch), nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		manifest, ok := result.Val.(*CodexModelsManifest)
		if !ok || manifest == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_REQUEST_FAILED", "invalid shared Codex models manifest result")
		}
		return codexModelsManifestForClient(manifest, ifNoneMatch), nil
	}
}

func (s *OpenAIGatewayService) refreshCachedAPIKeyCodexModelsManifest(cacheKey string, request codexModelsManifestRequest) <-chan singleflight.Result {
	return s.codexModelsManifestCache.refresh.DoChan(cacheKey, func() (any, error) {
		cached, _ := s.codexModelsManifestCache.get(cacheKey, time.Now())
		ifNoneMatch := ""
		if cached != nil {
			ifNoneMatch = cached.upstreamETag
		}
		manifest, err := s.fetchCodexModelsManifestUpstream(context.Background(), request, ifNoneMatch)
		if err != nil {
			return nil, err
		}
		if manifest.NotModified && cached != nil {
			s.codexModelsManifestCache.set(cacheKey, cached, time.Now())
			return cached, nil
		}
		if !manifest.NotModified {
			s.codexModelsManifestCache.set(cacheKey, manifest, time.Now())
		}
		return manifest, nil
	})
}

func (s *OpenAIGatewayService) fetchCodexModelsManifestUpstream(ctx context.Context, request codexModelsManifestRequest, ifNoneMatch string) (*CodexModelsManifest, error) {
	reqCtx, cancel := context.WithTimeout(ctx, codexModelsManifestRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, request.url, nil)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_REQUEST_FAILED", "create codex models request: %v", err)
	}
	req.Header = request.headers.Clone()
	if ifNoneMatch = strings.TrimSpace(ifNoneMatch); ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}

	var resp *http.Response
	if request.useAPIKeyUpstream {
		if s.httpUpstream == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_UPSTREAM_NOT_CONFIGURED", "Codex models upstream HTTP client is not configured")
		}
		req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
		resp, err = s.httpUpstream.Do(req, request.proxyURL, request.accountID, request.accountConcurrency)
	} else {
		handled := false
		if s.pluginManager != nil {
			resp, handled, err = s.pluginManager.RoundTripOpenAIOAuth(reqCtx, req, request.proxyURL, request.credentialAccount)
		}
		if !handled {
			client, clientErr := httpclient.GetClient(httpclient.Options{
				ProxyURL:              request.proxyURL,
				Timeout:               codexModelsManifestRequestTimeout,
				ResponseHeaderTimeout: 10 * time.Second,
			})
			if clientErr != nil {
				return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_PROXY_INVALID", "invalid proxy configuration: %v", clientErr)
			}
			resp, err = client.Do(req)
		}
	}
	if err != nil {
		return nil, &codexModelsManifestUpstreamError{
			err:       infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "codex models manifest request failed: %v", err),
			retryable: isRetryableCodexModelsManifestTransportError(err),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		return &CodexModelsManifest{ETag: resp.Header.Get("ETag"), NotModified: true}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		body = s.redactAgentIdentitySensitiveBody(reqCtx, request.credentialAccount, body)
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return nil, &codexModelsManifestUpstreamError{
			err:        infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "codex models manifest upstream error %d: %s", resp.StatusCode, message),
			statusCode: resp.StatusCode,
			headers:    resp.Header.Clone(),
			body:       body,
			retryable:  isRetryableCodexModelsManifestStatus(resp.StatusCode, request.useAPIKeyUpstream),
		}
	}

	bodyLimit := resolveModelsListReadLimit(s.cfg)
	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
	if err != nil {
		return nil, &codexModelsManifestUpstreamError{
			err:       infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "read codex models manifest response: %v", err),
			retryable: isRetryableCodexModelsManifestTransportError(err),
		}
	}
	if int64(len(body)) > bodyLimit {
		return nil, &codexModelsManifestUpstreamError{
			err:       infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "codex models manifest response exceeds %d bytes", bodyLimit),
			retryable: true,
		}
	}
	upstreamBody := body
	convertedFromOpenAIModelList := false
	if request.useAPIKeyUpstream {
		convertedBody := convertOpenAIModelListToCodexManifest(body)
		convertedFromOpenAIModelList = !bytes.Equal(convertedBody, body)
		body = convertedBody
	}
	if err := validateCodexModelsManifestEnvelope(body); err != nil {
		return nil, &codexModelsManifestUpstreamError{
			err: infraerrors.Newf(
				http.StatusBadGateway,
				"OPENAI_CODEX_MODELS_UPSTREAM_INVALID_MANIFEST",
				"codex models manifest upstream returned an invalid envelope: %v",
				err,
			),
			retryable: true,
		}
	}
	if request.useAPIKeyUpstream {
		body, err = completeAPIKeyCodexModelsManifestMetadata(
			body,
			false,
			request.credentialAccount != nil && isOfficialOpenAIModelsBaseURL(request.credentialAccount.GetOpenAIBaseURL()),
		)
		if err != nil {
			return nil, &codexModelsManifestUpstreamError{
				err: infraerrors.Newf(
					http.StatusBadGateway,
					"OPENAI_CODEX_MODELS_UPSTREAM_INVALID_MANIFEST",
					"codex models manifest upstream metadata could not be completed: %v",
					err,
				),
				retryable: true,
			}
		}
		body, err = adjustAPIKeyCodexModelsManifest(body)
		if err != nil {
			return nil, &codexModelsManifestUpstreamError{
				err: infraerrors.Newf(
					http.StatusBadGateway,
					"OPENAI_CODEX_MODELS_UPSTREAM_INVALID_MANIFEST",
					"codex models manifest upstream could not be adjusted: %v",
					err,
				),
				retryable: true,
			}
		}
	}
	etag := resp.Header.Get("ETag")
	manifest := &CodexModelsManifest{
		Body:                         body,
		ETag:                         etag,
		upstreamSourceBody:           append([]byte(nil), upstreamBody...),
		convertedFromOpenAIModelList: convertedFromOpenAIModelList,
	}
	if request.useAPIKeyUpstream {
		manifest.upstreamETag = etag
		if !bytes.Equal(body, upstreamBody) {
			manifest.ETag = codexModelsManifestBodyETag(body)
		}
	}
	return manifest, nil
}

func codexModelsManifestBodyETag(body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf(`"%x"`, sum)
}

// CodexModelsManifestETag returns the strong ETag for a generated client
// catalog. It is based on the final JSON body after local filtering.
func CodexModelsManifestETag(body []byte) string {
	return codexModelsManifestBodyETag(body)
}

var apiKeyCodexModelsWithoutResponsesLite = map[string]struct{}{
	"gpt-5.6-sol":   {},
	"gpt-5.6-terra": {},
	"gpt-5.6-luna":  {},
}

// adjustAPIKeyCodexModelsManifest prevents Codex from selecting Responses
// Lite for custom API key providers. Those clients do not install web.run in
// Lite mode, so the affected model manifests must advertise the full Responses
// path. Return the original body when no targeted true value is present.
func adjustAPIKeyCodexModelsManifest(body []byte) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode JSON object: %w", err)
	}
	var models []json.RawMessage
	if err := json.Unmarshal(envelope["models"], &models); err != nil {
		return nil, fmt.Errorf("decode top-level models array: %w", err)
	}

	changed := false
	for i, rawModel := range models {
		var model map[string]json.RawMessage
		if err := json.Unmarshal(rawModel, &model); err != nil || model == nil {
			continue
		}
		var slug string
		if err := json.Unmarshal(model["slug"], &slug); err != nil {
			continue
		}
		if _, targeted := apiKeyCodexModelsWithoutResponsesLite[slug]; !targeted {
			continue
		}
		var useResponsesLite bool
		if err := json.Unmarshal(model["use_responses_lite"], &useResponsesLite); err != nil || !useResponsesLite {
			continue
		}
		model["use_responses_lite"] = json.RawMessage("false")
		adjusted, err := json.Marshal(model)
		if err != nil {
			return nil, fmt.Errorf("encode model %q: %w", slug, err)
		}
		models[i] = adjusted
		changed = true
	}
	if !changed {
		return body, nil
	}

	adjustedModels, err := json.Marshal(models)
	if err != nil {
		return nil, fmt.Errorf("encode top-level models array: %w", err)
	}
	envelope["models"] = adjustedModels
	adjusted, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode JSON object: %w", err)
	}
	return adjusted, nil
}

// convertOpenAIModelListToCodexManifest rewrites a standard OpenAI
// GET /v1/models response ({"object":"list","data":[{"id":...},...]}) into the
// same complete Codex manifest used by locally generated custom-provider
// catalogs. Bodies that already carry a top-level models field, are not the
// standard list shape, or yield no usable model IDs are returned unchanged so
// envelope validation reports the original payload.
func convertOpenAIModelListToCodexManifest(body []byte) []byte {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil || envelope == nil {
		return body
	}
	if _, ok := envelope["models"]; ok {
		return body
	}
	data, ok := envelope["data"]
	if !ok {
		return body
	}
	var entries []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return body
	}
	modelIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		modelIDs = append(modelIDs, id)
	}
	if len(modelIDs) == 0 {
		return body
	}
	converted, err := BuildCodexModelsManifest(modelIDs)
	if err != nil {
		return body
	}
	return converted
}

// completeAPIKeyCodexModelsManifestMetadata fills fields omitted by standard
// OpenAI-compatible /models endpoints. Existing provider metadata always wins;
// only absent or null values are synthesized.
// CompleteAPIKeyCodexModelsManifestForClient fills the complete ModelInfo
// contract immediately before a group-specific API key manifest is returned.
// The shared upstream cache remains independent from local group policy.
func (s *OpenAIGatewayService) CompleteAPIKeyCodexModelsManifestForClient(manifest *CodexModelsManifest, account *Account) error {
	if manifest == nil || account == nil || !account.IsOpenAIApiKey() || manifest.NotModified || len(manifest.Body) == 0 {
		return nil
	}
	body := manifest.Body
	if len(manifest.upstreamSourceBody) > 0 {
		body = append([]byte(nil), manifest.upstreamSourceBody...)
		if manifest.convertedFromOpenAIModelList {
			body = convertOpenAIModelListToCodexManifest(body)
		}
	}
	var err error
	body, err = applySyncedAPIKeyCodexModelMetadata(body, account, manifest.convertedFromOpenAIModelList)
	if err != nil {
		return err
	}
	body, err = completeAPIKeyCodexModelsManifestMetadata(
		body,
		true,
		isOfficialOpenAIModelsBaseURL(account.GetOpenAIBaseURL()),
	)
	if err != nil {
		return err
	}
	body, err = adjustAPIKeyCodexModelsManifest(body)
	if err != nil {
		return err
	}
	manifest.Body = body
	manifest.ETag = codexModelsManifestBodyETag(manifest.Body)
	return nil
}

func applySyncedAPIKeyCodexModelMetadata(body []byte, account *Account, overwriteLocalDefaults bool) ([]byte, error) {
	snapshot := account.GetUpstreamModelMetadataSnapshot()
	if snapshot == nil || len(snapshot.Models) == 0 {
		return body, nil
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode JSON object: %w", err)
	}
	var models []json.RawMessage
	if err := json.Unmarshal(envelope["models"], &models); err != nil {
		return nil, fmt.Errorf("decode top-level models array: %w", err)
	}

	changed := false
	for i, rawModel := range models {
		var model map[string]json.RawMessage
		if err := json.Unmarshal(rawModel, &model); err != nil || model == nil {
			continue
		}
		var slug string
		if err := json.Unmarshal(model["slug"], &slug); err != nil {
			continue
		}
		slug = strings.TrimSpace(slug)
		metadata, ok := snapshot.Models[slug]
		if !ok {
			continue
		}

		descriptor := newConfiguredCodexModelDescriptor(slug)
		applyUpstreamModelMetadataToCodexDescriptor(
			&descriptor,
			codexModelMetadataOverride{UpstreamModelMetadata: metadata},
		)
		descriptorBody, err := json.Marshal(descriptor)
		if err != nil {
			return nil, fmt.Errorf("encode synced model %q: %w", slug, err)
		}
		var syncedFields map[string]json.RawMessage
		if err := json.Unmarshal(descriptorBody, &syncedFields); err != nil {
			return nil, fmt.Errorf("decode synced model %q: %w", slug, err)
		}

		fields := make([]string, 0, 7)
		if strings.TrimSpace(metadata.DisplayName) != "" {
			fields = append(fields, "display_name")
		}
		if strings.TrimSpace(metadata.Description) != "" {
			fields = append(fields, "description")
		}
		if metadata.Reasoning != nil {
			fields = append(fields, "default_reasoning_level", "supported_reasoning_levels")
		}
		if len(normalizeCodexInputModalities(metadata.InputModalities)) > 0 {
			fields = append(fields, "input_modalities")
		}
		if metadata.ContextWindow > 0 {
			fields = append(fields, "context_window", "max_context_window")
		}

		modelChanged := false
		for _, field := range fields {
			value, exists := syncedFields[field]
			if !exists {
				continue
			}
			current, currentExists := model[field]
			current = bytes.TrimSpace(current)
			if !overwriteLocalDefaults && currentExists && len(current) > 0 && !bytes.Equal(current, []byte("null")) {
				continue
			}
			if bytes.Equal(current, bytes.TrimSpace(value)) {
				continue
			}
			model[field] = value
			modelChanged = true
		}
		if !modelChanged {
			continue
		}
		encoded, err := json.Marshal(model)
		if err != nil {
			return nil, fmt.Errorf("encode model %q with synced metadata: %w", slug, err)
		}
		models[i] = encoded
		changed = true
	}
	if !changed {
		return body, nil
	}

	encodedModels, err := json.Marshal(models)
	if err != nil {
		return nil, fmt.Errorf("encode models with synced metadata: %w", err)
	}
	envelope["models"] = encodedModels
	updated, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode manifest with synced metadata: %w", err)
	}
	return updated, nil
}

func completeAPIKeyCodexModelsManifestMetadata(body []byte, completeAll, officialOpenAI bool) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode JSON object: %w", err)
	}
	var models []json.RawMessage
	if err := json.Unmarshal(envelope["models"], &models); err != nil {
		return nil, fmt.Errorf("decode top-level models array: %w", err)
	}

	changed := false
	if officialOpenAI {
		filtered := make([]json.RawMessage, 0, len(models))
		for _, rawModel := range models {
			var model struct {
				Slug string `json:"slug"`
			}
			if err := json.Unmarshal(rawModel, &model); err != nil || strings.TrimSpace(model.Slug) == "" {
				filtered = append(filtered, rawModel)
				continue
			}
			if !isOfficialOpenAICodexCatalogModel(model.Slug) {
				changed = true
				continue
			}
			filtered = append(filtered, rawModel)
		}
		models = filtered
	}
	for i, rawModel := range models {
		var model map[string]json.RawMessage
		if err := json.Unmarshal(rawModel, &model); err != nil || model == nil {
			continue
		}
		var slug string
		if err := json.Unmarshal(model["slug"], &slug); err != nil {
			continue
		}
		slug = strings.TrimSpace(slug)
		if slug == "" {
			continue
		}

		completeDescriptor := completeAll || isDeepSeekCodexModel(slug)
		forceOfficialImage := officialOpenAI && isOpenAICodexImageInputModel(slug)
		if !completeDescriptor && !forceOfficialImage {
			continue
		}

		descriptor := newConfiguredCodexModelDescriptor(slug)
		if forceOfficialImage {
			descriptor.InputModalities = []string{"text", "image"}
			descriptor.SupportsImageDetailOriginal = true
		}
		defaultBody, err := json.Marshal(descriptor)
		if err != nil {
			return nil, fmt.Errorf("encode default model %q: %w", slug, err)
		}
		var defaults map[string]json.RawMessage
		if err := json.Unmarshal(defaultBody, &defaults); err != nil {
			return nil, fmt.Errorf("decode default model %q: %w", slug, err)
		}

		modelChanged := false
		if completeDescriptor {
			merged, err := mergeMissingCodexModelFields(model, defaults)
			if err != nil {
				return nil, fmt.Errorf("complete model %q: %w", slug, err)
			}
			modelChanged = merged
		}
		if forceOfficialImage {
			modalities, err := json.Marshal([]string{"text", "image"})
			if err != nil {
				return nil, fmt.Errorf("encode input modalities for model %q: %w", slug, err)
			}
			if !bytes.Equal(bytes.TrimSpace(model["input_modalities"]), modalities) {
				model["input_modalities"] = modalities
				modelChanged = true
			}
			imageDetailOriginal := json.RawMessage("true")
			if !bytes.Equal(bytes.TrimSpace(model["supports_image_detail_original"]), imageDetailOriginal) {
				model["supports_image_detail_original"] = imageDetailOriginal
				modelChanged = true
			}
		}
		if !modelChanged {
			continue
		}
		encoded, err := json.Marshal(model)
		if err != nil {
			return nil, fmt.Errorf("encode completed model %q: %w", slug, err)
		}
		models[i] = encoded
		changed = true
	}
	if !changed {
		return body, nil
	}

	encodedModels, err := json.Marshal(models)
	if err != nil {
		return nil, fmt.Errorf("encode top-level models array: %w", err)
	}
	envelope["models"] = encodedModels
	completed, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode JSON object: %w", err)
	}
	return completed, nil
}

func mergeMissingCodexModelFields(current, defaults map[string]json.RawMessage) (bool, error) {
	changed := false
	for key, defaultValue := range defaults {
		currentValue, exists := current[key]
		if !exists || (bytes.Equal(bytes.TrimSpace(currentValue), []byte("null")) &&
			!bytes.Equal(bytes.TrimSpace(defaultValue), []byte("null"))) {
			current[key] = defaultValue
			changed = true
			continue
		}

		var currentObject map[string]json.RawMessage
		var defaultObject map[string]json.RawMessage
		if err := json.Unmarshal(currentValue, &currentObject); err != nil || currentObject == nil {
			continue
		}
		if err := json.Unmarshal(defaultValue, &defaultObject); err != nil || defaultObject == nil {
			continue
		}
		nestedChanged, err := mergeMissingCodexModelFields(currentObject, defaultObject)
		if err != nil {
			return false, err
		}
		if !nestedChanged {
			continue
		}
		mergedValue, err := json.Marshal(currentObject)
		if err != nil {
			return false, fmt.Errorf("encode field %q: %w", key, err)
		}
		current[key] = mergedValue
		changed = true
	}
	return changed, nil
}

func validateCodexModelsManifestEnvelope(body []byte) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode JSON object: %w", err)
	}
	if envelope == nil {
		return errors.New("expected a JSON object")
	}
	models, ok := envelope["models"]
	if !ok {
		return errors.New("missing top-level models array")
	}
	models = bytes.TrimSpace(models)
	var entries []json.RawMessage
	if len(models) == 0 || models[0] != '[' {
		return errors.New("top-level models field is not an array")
	}
	if err := json.Unmarshal(models, &entries); err != nil {
		return fmt.Errorf("decode top-level models array: %w", err)
	}
	return nil
}

func buildCodexModelsManifestCacheKey(request codexModelsManifestRequest) string {
	hasher := sha256.New()
	_, _ = fmt.Fprintf(hasher, "%d\n%d\n%s\n%s\n", request.accountID, request.credentialAccountID, request.proxyURL, request.url)
	headerNames := make([]string, 0, len(request.headers))
	for name := range request.headers {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	for _, name := range headerNames {
		_, _ = fmt.Fprintf(hasher, "%s\n", strings.ToLower(name))
		for _, value := range request.headers[name] {
			_, _ = fmt.Fprintf(hasher, "%s\n", value)
		}
	}
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func cloneCodexModelsManifest(manifest *CodexModelsManifest) *CodexModelsManifest {
	if manifest == nil {
		return nil
	}
	cloned := *manifest
	if manifest.Body != nil {
		cloned.Body = append([]byte(nil), manifest.Body...)
	}
	if manifest.upstreamSourceBody != nil {
		cloned.upstreamSourceBody = append([]byte(nil), manifest.upstreamSourceBody...)
	}
	return &cloned
}

func codexModelsManifestForClient(manifest *CodexModelsManifest, ifNoneMatch string) *CodexModelsManifest {
	if manifest == nil {
		return nil
	}
	if codexModelsManifestETagMatches(ifNoneMatch, manifest.ETag) {
		return &CodexModelsManifest{ETag: manifest.ETag, NotModified: true}
	}
	return cloneCodexModelsManifest(manifest)
}

func codexModelsManifestETagMatches(ifNoneMatch, etag string) bool {
	etag = strings.TrimSpace(etag)
	if etag == "" {
		return false
	}
	normalize := func(value string) string {
		value = strings.TrimSpace(value)
		if len(value) >= 2 && strings.EqualFold(value[:2], "W/") {
			value = strings.TrimSpace(value[2:])
		}
		return value
	}
	want := normalize(etag)
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || normalize(candidate) == want {
			return true
		}
	}
	return false
}

// CodexModelsManifestETagMatches applies If-None-Match semantics to a Codex
// catalog ETag, including weak and comma-separated validators.
func CodexModelsManifestETagMatches(ifNoneMatch, etag string) bool {
	return codexModelsManifestETagMatches(ifNoneMatch, etag)
}

func isOfficialOpenAIModelsBaseURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	hostname := strings.TrimSuffix(parsed.Hostname(), ".")
	return strings.EqualFold(hostname, "api.openai.com")
}

func buildCodexModelsManifestURL(endpoint string, appendModelsPath bool, clientVersion string) (*url.URL, error) {
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if requestURL.Fragment != "" {
		return nil, fmt.Errorf("URL fragments are not supported")
	}

	query := requestURL.Query()
	requestURL.RawQuery = ""
	requestURL.ForceQuery = false
	if appendModelsPath {
		requestURL, err = url.Parse(buildOpenAIModelsURL(requestURL.String()))
		if err != nil {
			return nil, err
		}
	}
	query.Set("client_version", clientVersion)
	requestURL.RawQuery = query.Encode()
	return requestURL, nil
}
