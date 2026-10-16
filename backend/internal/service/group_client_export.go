package service

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// GroupClientExportOptions exposes public model IDs only. The lists describe
// configured catalogs, not successful inference probes or account credentials.
type GroupClientExportOptions struct {
	CodexModels  []string `json:"codex_models"`
	GeminiModels []string `json:"gemini_models"`
	GrokModels   []string `json:"grok_models"`
}

var clientExportModelID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/+\-]{0,199}$`)

// GetGroupClientExportOptions returns the selected group's configured text
// catalog. id=0 previews platform defaults before a new group has members.
func (s *adminServiceImpl) GetGroupClientExportOptions(ctx context.Context, id int64, platform string) (*GroupClientExportOptions, error) {
	platform = strings.TrimSpace(platform)
	if platform != "" && platform != PlatformComposite && !isConcreteRequestPlatform(platform) {
		return nil, infraerrors.BadRequest("INVALID_GROUP_PLATFORM", "unsupported group platform")
	}
	group := &Group{Platform: NormalizeGroupPlatform(platform)}
	if id > 0 {
		stored, err := s.GetGroup(ctx, id)
		if err != nil {
			return nil, err
		}
		copy := *stored
		group = &copy
		if platform != "" && platform != group.Platform {
			group.Platform = platform
			group.ModelAllowlist = GroupModelAllowlist{}
			group.CodexModelsManifestConfig = GroupCodexModelsManifestConfig{}
		}
	}
	return s.groupClientExportOptions(ctx, group)
}

// groupClientExportOptions reads members through the existing repository and
// preserves mapping, retirement, pinned-account and allowlist semantics.
func (s *adminServiceImpl) groupClientExportOptions(ctx context.Context, group *Group) (*GroupClientExportOptions, error) {
	var accounts []Account
	if group.ID > 0 && s.accountRepo != nil {
		var err error
		accounts, err = s.accountRepo.ListSchedulableByGroupID(ctx, group.ID)
		if err != nil {
			return nil, err
		}
	}
	var routes []CompositeModelRoute
	if group.Platform == PlatformComposite && group.ID > 0 && s.compositeRouteRepo != nil {
		var err error
		routes, err = s.compositeRouteRepo.ListByGroup(ctx, group.ID, true)
		if err != nil {
			return nil, err
		}
	}
	return buildGroupClientExportOptions(group, accounts, routes), nil
}

// buildGroupClientExportOptions shares native catalog filters and excludes
// account aliases whose target is a dedicated media or embedding model.
func buildGroupClientExportOptions(group *Group, accounts []Account, routes []CompositeModelRoute) *GroupClientExportOptions {
	result := &GroupClientExportOptions{CodexModels: []string{}, GeminiModels: []string{}, GrokModels: []string{}}
	if group.Platform == PlatformTypeSafe {
		return result
	}
	models := []string{}
	visibleAccounts := []Account{}
	for i := range accounts {
		account := &accounts[i]
		if group.Platform != PlatformComposite && account.Platform != group.Platform && !mixedListingAccountAllowed(group.Platform, account) {
			continue
		}
		if group.Platform == PlatformOpenAI && group.CodexModelsManifestConfig.Enabled && !slices.Contains(group.CodexModelsManifestConfig.AccountIDs, account.ID) {
			continue
		}
		visibleAccounts = append(visibleAccounts, *account)
		if account.Platform == PlatformTypeSafe {
			continue
		}
		mapping := account.GetModelMapping()
		if len(mapping) == 0 || account.IsOpenAIPassthroughEnabled() {
			if snapshot := account.GetUpstreamModelMetadataSnapshot(); snapshot != nil && len(snapshot.Models) > 0 {
				for id := range snapshot.Models {
					models = append(models, id)
				}
			} else {
				models = append(models, defaultClientExportModels(account.Platform)...)
			}
		} else {
			for id, target := range mapping {
				if !clientExportTextModel(target) || (account.Platform != group.Platform && group.Platform == PlatformGemini && !mixedListingModelAllowed(group.Platform, id)) {
					continue
				}
				models = append(models, id)
			}
		}
	}
	// New or empty groups can preview native defaults, but an enabled pinned
	// source with no available member must stay empty rather than invent models.
	if len(visibleAccounts) == 0 && !group.CodexModelsManifestConfig.Enabled {
		models = defaultClientExportModels(group.Platform)
	}
	for _, route := range routes {
		endpoint := normalizeCompositeRouteEndpoint(route.Endpoint)
		upstream := route.UpstreamModel
		if upstream == "" {
			upstream = route.PublicModel
		}
		if route.Enabled && route.MatchType == CompositeRouteMatchExact && (endpoint == CompositeRouteEndpointAny || endpoint == CompositeRouteEndpointResponses) && clientExportTextModel(upstream) && route.TargetPlatform != PlatformTypeSafe {
			models = append(models, route.PublicModel)
		}
	}
	if group.Platform == PlatformOpenAI {
		models = filterRetiredOpenAIModelIDs(visibleAccounts, models)
	}
	models = FilterCodexModelIDsForGroup(models, group)
	for _, model := range models {
		if !clientExportTextModel(model) {
			continue
		}
		if group.Platform == PlatformAntigravity {
			if isAntigravityGeminiModel(model) && len(group.SupportedModelScopes) > 0 && !slices.Contains(group.SupportedModelScopes, "gemini_text") {
				continue
			}
			if strings.HasPrefix(model, "claude-") && len(group.SupportedModelScopes) > 0 && !slices.Contains(group.SupportedModelScopes, "claude") {
				continue
			}
		}
		result.CodexModels = append(result.CodexModels, model)
		if (group.Platform == PlatformGemini || group.Platform == PlatformAntigravity) && isAntigravityGeminiModel(model) {
			result.GeminiModels = append(result.GeminiModels, model)
		}
		if group.Platform == PlatformGrok {
			result.GrokModels = append(result.GrokModels, model)
		}
	}
	for _, models := range []*[]string{&result.CodexModels, &result.GeminiModels, &result.GrokModels} {
		sort.Strings(*models)
		*models = slices.Compact(*models)
	}
	return result
}

// defaultClientExportModels follows the native routed Codex template defaults
// for providers that do not yet have a shared static model catalog.
func defaultClientExportModels(platform string) []string {
	switch platform {
	case PlatformDeepseek:
		return []string{"deepseek-v4-pro", "deepseek-v4-flash", "deepseek-flash"}
	case PlatformMiniMax:
		return []string{"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.5"}
	case "kimi":
		return []string{"kimi-k2.5"}
	case "zhipu":
		return []string{"glm-4.7"}
	default:
		return defaultModelsListCandidateIDs(platform)
	}
}

// clientExportTextModel accepts concrete safe IDs and rejects native dedicated
// media, audio and embedding families before they reach shell/TOML templates.
func clientExportTextModel(model string) bool {
	if !clientExportModelID.MatchString(model) || isCodexDedicatedMediaModel(model) {
		return false
	}
	value := strings.ToLower(codexProviderQualifiedModelID(model))
	for _, marker := range []string{"embedding", "embed-", "whisper", "tts-", "audio", "realtime", "video", "imagen", "veo-", "jev-"} {
		if strings.Contains(value, marker) {
			return false
		}
	}
	return true
}

// validateGroupClientExport rejects arbitrary or disallowed selections before
// repository writes. Empty preferences preserve native export behavior.
func (s *adminServiceImpl) validateGroupClientExport(ctx context.Context, group *Group) error {
	cfg := group.ClientExportConfig
	if cfg == (GroupClientExportConfig{}) {
		return nil
	}
	// Validate enum values independently of the browser. Dedicated templates and
	// routed Codex providers must not accept settings their native paths ignore.
	for _, setting := range []struct {
		name      string
		value     string
		allowed   []string
		supported bool
	}{
		{"codex_reasoning_effort", cfg.CodexReasoningEffort, []string{"low", "medium", "high", "xhigh"}, group.Platform != PlatformTypeSafe},
		{"codex_auth_mode", cfg.CodexAuthMode, []string{"legacy", "api-key"}, group.Platform == PlatformOpenAI},
		{"codex_catalog_mode", cfg.CodexCatalogMode, []string{"remote", "file"}, group.Platform != PlatformTypeSafe},
		{"claude_traffic", cfg.ClaudeTraffic, []string{"enabled", "disabled"}, group.Platform != PlatformTypeSafe && group.Platform != PlatformGemini},
		{"cc_usage_mode", cfg.CCUsageMode, []string{"enabled", "disabled"}, group.Platform != PlatformTypeSafe},
		{"cc_usage_interval", cfg.CCUsageInterval, []string{"0", "5", "15", "30", "60"}, group.Platform != PlatformTypeSafe},
	} {
		if setting.value != "" && (!setting.supported || !slices.Contains(setting.allowed, setting.value)) {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_CLIENT_EXPORT_SETTING", "%s must be a supported preset for this platform", setting.name)
		}
	}
	options, err := s.groupClientExportOptions(ctx, group)
	if err != nil {
		return err
	}
	for _, selection := range []struct {
		name    string
		value   string
		options []string
	}{{"codex_model", cfg.CodexModel, options.CodexModels}, {"gemini_model", cfg.GeminiModel, options.GeminiModels}, {"grok_model", cfg.GrokModel, options.GrokModels}} {
		if selection.value != "" && (!clientExportTextModel(selection.value) || !slices.Contains(selection.options, selection.value)) {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_CLIENT_EXPORT_MODEL", "%s must be selected from this group's client model catalog", selection.name)
		}
	}
	return nil
}
