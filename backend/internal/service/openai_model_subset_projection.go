package service

import (
	"sort"
	"strconv"
	"strings"
)

const (
	openAICapabilityExplicitModelsExtraKey = "openai_capability_explicit_models"
	openAICapabilityWildcardRulesExtraKey  = "openai_capability_wildcard_rules"
	openAICapabilityDefaultAllowExtraKey   = "openai_capability_default_allow"
)

type OpenAIModelCapabilitySnapshot struct {
	ExplicitModels map[string]struct{}
	WildcardRules  []string
	DefaultAllow   bool
}

type OpenAIProjectionInputs struct {
	Bucket            SchedulerBucket
	CanonicalCatalog  []string
	AccountsAll       []Account
	ExhaustedBroadIDs map[int64]struct{}
	CapabilityByID    map[int64]OpenAIModelCapabilitySnapshot
}

type OpenAIModelRoleView struct {
	CanonicalModel     string
	ExhaustedBaseIDs   []int64
	ReserveOverflowIDs []int64
}

type OpenAIModelSubsetProjection struct {
	Bucket            SchedulerBucket
	AccountReserveIDs map[int64]struct{}
	Models            map[string]OpenAIModelRoleView
}

type openAIProjectionModelMembers struct {
	exhausted []Account
	active    []Account
}

func (p *OpenAIModelSubsetProjection) ViewForModel(model string) (OpenAIModelRoleView, bool) {
	canonical := NormalizeOpenAIProjectionModelKey(model)
	if canonical == "" || p == nil || p.Models == nil {
		return OpenAIModelRoleView{CanonicalModel: canonical}, false
	}
	if view, ok := p.Models[canonical]; ok {
		return view, true
	}
	return OpenAIModelRoleView{CanonicalModel: canonical}, false
}

func BuildOpenAIModelSubsetProjection(inputs *OpenAIProjectionInputs) *OpenAIModelSubsetProjection {
	projection := &OpenAIModelSubsetProjection{
		AccountReserveIDs: make(map[int64]struct{}),
		Models:            make(map[string]OpenAIModelRoleView),
	}
	if inputs == nil {
		return projection
	}
	projection.Bucket = inputs.Bucket

	catalog := canonicalizeOpenAIProjectionCatalog(inputs.CanonicalCatalog)
	if len(catalog) == 0 {
		return projection
	}
	catalogSet := stringSliceToSet(catalog)
	localViews, supportedModels := buildOpenAIModelRoleViews(inputs, catalog, catalogSet)
	for accountID := range collectOpenAIReserveIDs(localViews) {
		projection.AccountReserveIDs[accountID] = struct{}{}
	}
	projection.Models = liftModelSubsetReserveIdentities(localViews, supportedModels)
	return projection
}

func BuildOpenAICanonicalModelCatalog(accounts []Account, explicitCapabilityModels []string, configuredModels []string) []string {
	catalog := make(map[string]struct{})
	addOpenAICanonicalModels(catalog, explicitCapabilityModels...)
	addOpenAICanonicalModels(catalog, configuredModels...)

	for _, account := range accounts {
		snapshot := buildOpenAIModelCapabilitySnapshot(account)
		for model := range snapshot.ExplicitModels {
			catalog[model] = struct{}{}
		}
	}

	models := make([]string, 0, len(catalog))
	for model := range catalog {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}

func accountSupportsProjectionModel(account Account, model string) bool {
	snapshot := buildOpenAIModelCapabilitySnapshot(account)
	catalog := stringSliceToSet(BuildOpenAICanonicalModelCatalog([]Account{account}, nil, nil))
	return account.SupportsProjectionModel(model, snapshot, catalog)
}

func buildOpenAIModelCapabilitySnapshot(account Account) OpenAIModelCapabilitySnapshot {
	snapshot := OpenAIModelCapabilitySnapshot{
		ExplicitModels: make(map[string]struct{}),
	}
	for requestedModel, mappedModel := range account.GetModelMapping() {
		if strings.Contains(requestedModel, "*") {
			addOpenAIProjectionWildcardRule(&snapshot, requestedModel)
		} else {
			addOpenAIProjectionExplicitModel(&snapshot, requestedModel)
		}
		addOpenAIProjectionExplicitModel(&snapshot, mappedModel)
	}
	addOpenAIProjectionGroupModels(&snapshot, account.Groups)
	if account.Extra == nil {
		return snapshot
	}

	for _, model := range parseOpenAIProjectionStringSlice(account.Extra[openAICapabilityExplicitModelsExtraKey]) {
		addOpenAIProjectionExplicitModel(&snapshot, model)
	}
	for _, rule := range parseOpenAIProjectionStringSlice(account.Extra[openAICapabilityWildcardRulesExtraKey]) {
		addOpenAIProjectionWildcardRule(&snapshot, rule)
	}
	snapshot.DefaultAllow = parseOpenAIProjectionBool(account.Extra[openAICapabilityDefaultAllowExtraKey])
	return snapshot
}

func addOpenAICanonicalModels(catalog map[string]struct{}, models ...string) {
	for _, model := range models {
		if strings.Contains(model, "*") {
			continue
		}
		if canonical := NormalizeOpenAIProjectionModelKey(model); canonical != "" {
			catalog[canonical] = struct{}{}
		}
	}
}

func addOpenAIProjectionGroupModels(snapshot *OpenAIModelCapabilitySnapshot, groups []*Group) {
	for _, group := range groups {
		if group == nil {
			continue
		}
		addOpenAIProjectionExplicitModel(snapshot, group.DefaultMappedModel)
		if !group.AllowMessagesDispatch {
			continue
		}
		addOpenAIProjectionExplicitModel(snapshot, group.MessagesDispatchModelConfig.OpusMappedModel)
		addOpenAIProjectionExplicitModel(snapshot, group.MessagesDispatchModelConfig.SonnetMappedModel)
		addOpenAIProjectionExplicitModel(snapshot, group.MessagesDispatchModelConfig.HaikuMappedModel)
		for _, mappedModel := range group.MessagesDispatchModelConfig.ExactModelMappings {
			addOpenAIProjectionExplicitModel(snapshot, mappedModel)
		}
	}
}

func addOpenAIProjectionExplicitModel(snapshot *OpenAIModelCapabilitySnapshot, model string) {
	if snapshot == nil {
		return
	}
	if canonical := NormalizeOpenAIProjectionModelKey(model); canonical != "" {
		snapshot.ExplicitModels[canonical] = struct{}{}
	}
}

func addOpenAIProjectionWildcardRule(snapshot *OpenAIModelCapabilitySnapshot, rule string) {
	if snapshot == nil {
		return
	}
	if normalized := normalizeOpenAIProjectionPattern(rule); normalized != "" {
		for _, existing := range snapshot.WildcardRules {
			if existing == normalized {
				return
			}
		}
		snapshot.WildcardRules = append(snapshot.WildcardRules, normalized)
	}
}

func wildcardRulesSupportProjectionModel(rules []string, canonicalModel string) bool {
	for _, rule := range rules {
		if projectionPatternMatches(rule, canonicalModel) {
			return true
		}
	}
	return false
}

func projectionPatternMatches(pattern, canonicalModel string) bool {
	normalizedPattern := normalizeOpenAIProjectionPattern(pattern)
	if normalizedPattern == "" || canonicalModel == "" {
		return false
	}
	if strings.HasSuffix(normalizedPattern, "*") {
		return strings.HasPrefix(canonicalModel, strings.TrimSuffix(normalizedPattern, "*"))
	}
	return normalizedPattern == canonicalModel
}

func normalizeOpenAIProjectionPattern(pattern string) string {
	trimmed := strings.TrimSpace(StripSysSuffix(pattern))
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "*") {
		return NormalizeOpenAIProjectionModelKey(trimmed)
	}
	if !strings.HasSuffix(trimmed, "*") {
		return strings.ToLower(trimmed)
	}
	prefix := strings.TrimSpace(strings.TrimSuffix(trimmed, "*"))
	if prefix == "" {
		return ""
	}
	normalizedPrefix := NormalizeOpenAIProjectionModelKey(prefix)
	if normalizedPrefix == "" {
		normalizedPrefix = strings.ToLower(openAICompatModelID(prefix))
	}
	if normalizedPrefix == "" {
		return ""
	}
	return normalizedPrefix + "*"
}

func parseOpenAIProjectionStringSlice(value any) []string {
	if value == nil {
		return nil
	}

	var raw []string
	switch v := value.(type) {
	case []string:
		raw = v
	case []any:
		raw = make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				raw = append(raw, s)
			}
		}
	default:
		return nil
	}

	out := make([]string, 0, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseOpenAIProjectionBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		return err == nil && parsed
	default:
		return false
	}
}

func stringSliceToSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return set
}

func canonicalizeOpenAIProjectionCatalog(models []string) []string {
	set := make(map[string]struct{}, len(models))
	for _, model := range models {
		if canonical := NormalizeOpenAIProjectionModelKey(model); canonical != "" {
			set[canonical] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for model := range set {
		out = append(out, model)
	}
	sort.Strings(out)
	return out
}

func buildOpenAIModelRoleViews(inputs *OpenAIProjectionInputs, catalog []string, catalogSet map[string]struct{}) (map[string]OpenAIModelRoleView, map[int64][]string) {
	views := make(map[string]OpenAIModelRoleView, len(catalog))
	members := make(map[string]*openAIProjectionModelMembers, len(catalog))
	for _, model := range catalog {
		views[model] = OpenAIModelRoleView{CanonicalModel: model}
		members[model] = &openAIProjectionModelMembers{}
	}

	supportedModels := make(map[int64][]string)
	for _, account := range inputs.AccountsAll {
		snapshot, ok := inputs.CapabilityByID[account.ID]
		if !ok {
			snapshot = buildOpenAIModelCapabilitySnapshot(account)
		}

		supported := make([]string, 0, len(catalog))
		for _, model := range catalog {
			if !account.SupportsProjectionModel(model, snapshot, catalogSet) {
				continue
			}
			supported = append(supported, model)
			if account.IsSchedulableForTargetGroup(TargetGroupExhausted) {
				members[model].exhausted = append(members[model].exhausted, account)
				continue
			}
			if account.IsSchedulableForTargetGroup(TargetGroupActive) {
				members[model].active = append(members[model].active, account)
			}
		}
		if len(supported) > 0 {
			supportedModels[account.ID] = supported
		}
	}

	for _, model := range catalog {
		member := members[model]
		view := views[model]
		view.ExhaustedBaseIDs = sortedOpenAIProjectionIDs(member.exhausted)
		view.ReserveOverflowIDs = sortedOpenAIProjectionIDs(buildOpenAIReserveOverflowPool(member.active, member.exhausted))
		views[model] = view
	}
	return views, supportedModels
}

func collectOpenAIReserveIDs(local map[string]OpenAIModelRoleView) map[int64]struct{} {
	reserve := make(map[int64]struct{})
	for _, view := range local {
		for _, accountID := range view.ReserveOverflowIDs {
			reserve[accountID] = struct{}{}
		}
	}
	return reserve
}

func liftModelSubsetReserveIdentities(local map[string]OpenAIModelRoleView, supportedModels map[int64][]string) map[string]OpenAIModelRoleView {
	lifted := make(map[string]OpenAIModelRoleView, len(local))
	for model, view := range local {
		lifted[model] = OpenAIModelRoleView{
			CanonicalModel:     view.CanonicalModel,
			ExhaustedBaseIDs:   append([]int64(nil), view.ExhaustedBaseIDs...),
			ReserveOverflowIDs: append([]int64(nil), view.ReserveOverflowIDs...),
		}
	}

	for accountID := range collectOpenAIReserveIDs(local) {
		for _, model := range supportedModels[accountID] {
			view, ok := lifted[model]
			if !ok || containsOpenAIProjectionID(view.ExhaustedBaseIDs, accountID) {
				continue
			}
			if !containsOpenAIProjectionID(view.ReserveOverflowIDs, accountID) {
				view.ReserveOverflowIDs = append(view.ReserveOverflowIDs, accountID)
				sort.Slice(view.ReserveOverflowIDs, func(i, j int) bool {
					return view.ReserveOverflowIDs[i] < view.ReserveOverflowIDs[j]
				})
			}
			lifted[model] = view
		}
	}
	return lifted
}

func sortedOpenAIProjectionIDs(accounts []Account) []int64 {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return ids
}

func containsOpenAIProjectionID(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func mergeOpenAIExhaustedAccountsFromBroadSource(base []Account, broad []Account) ([]Account, map[int64]struct{}) {
	merged := make([]Account, 0, len(base)+len(broad))
	baseIndex := make(map[int64]int, len(base))
	exhaustedBroadIDs := make(map[int64]struct{})

	for _, account := range base {
		baseIndex[account.ID] = len(merged)
		merged = append(merged, account)
	}

	for _, account := range broad {
		if !account.IsOpenAI() {
			continue
		}
		if idx, ok := baseIndex[account.ID]; ok {
			merged[idx] = account
			if account.IsSchedulableForTargetGroup(TargetGroupExhausted) {
				exhaustedBroadIDs[account.ID] = struct{}{}
			} else {
				delete(exhaustedBroadIDs, account.ID)
			}
			continue
		}
		if !account.IsSchedulableForTargetGroup(TargetGroupExhausted) {
			continue
		}
		baseIndex[account.ID] = len(merged)
		merged = append(merged, account)
		exhaustedBroadIDs[account.ID] = struct{}{}
	}

	return merged, exhaustedBroadIDs
}
