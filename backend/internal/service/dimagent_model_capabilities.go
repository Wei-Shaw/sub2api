package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const DimAgentCapabilitiesObject = "model_capability_list"

type DimAgentReasoningEffort struct {
	Level     string `json:"level"`
	WireValue string `json:"wire_value"`
}

type DimAgentReasoningCapability struct {
	Supported    bool                      `json:"supported"`
	DefaultLevel string                    `json:"default_level,omitempty"`
	Efforts      []DimAgentReasoningEffort `json:"efforts"`
}

type DimAgentModelCapability struct {
	ID               string                       `json:"id"`
	DisplayName      string                       `json:"display_name"`
	Protocol         string                       `json:"protocol"`
	ContextWindow    int                          `json:"context_window"`
	MaxOutputTokens  int                          `json:"max_output_tokens"`
	InputModalities  []string                     `json:"input_modalities"`
	Reasoning        *DimAgentReasoningCapability `json:"reasoning,omitempty"`
	ToolCall         bool                         `json:"tool_call"`
	StructuredOutput bool                         `json:"structured_output"`
}

type DimAgentCatalogScope struct {
	Platform string `json:"platform"`
	GroupID  int64  `json:"group_id"`
}

type DimAgentModelCapabilitiesEnvelope struct {
	Object         string                    `json:"object"`
	CatalogVersion string                    `json:"catalog_version"`
	CatalogScope   DimAgentCatalogScope      `json:"catalog_scope"`
	Data           []DimAgentModelCapability `json:"data"`
}

type dimAgentFallbackModel struct {
	Name       string
	Context    int
	Output     int
	Modalities []string
	Efforts    []string
	Default    string
}

// Mirror @arcships/dsh-dim-oauth's sparse fallback catalog. Live `dim`
// metadata always wins. seed-2.0-mini is added explicitly with the adapter's
// generic 128k/16k text fallback, while seedream-5.0 stays excluded because this
// endpoint feeds a Chat Completions provider, not an image-generation route.
var dimAgentFallbackModels = map[string]dimAgentFallbackModel{
	"deepseek-v4-pro": {Name: "DeepSeek V4 Pro", Context: 1_000_000, Output: 384_000, Modalities: []string{"text"}, Efforts: []string{"high", "max"}, Default: "high"},
	"glm-5.2":         {Name: "GLM-5.2", Context: 1_000_000, Output: 131_072, Modalities: []string{"text"}, Efforts: []string{"high", "max"}, Default: "high"},
	"glm-5.3":         {Name: "GLM-5.3", Context: 1_000_000, Output: 131_072, Modalities: []string{"text"}, Efforts: []string{"high", "max"}, Default: "high"},
	"kimi-k3":         {Name: "Kimi K3", Context: 1_048_576, Output: 131_072, Modalities: []string{"text", "image"}, Efforts: []string{"low", "high", "max"}, Default: "high"},
	"seed-2.1-pro":    {Name: "Seed 2.1 Pro", Context: 128_000, Output: 16_384, Modalities: []string{"text"}},
	"seed-2.0-mini":   {Name: "Seed 2.0 Mini", Context: 128_000, Output: 16_384, Modalities: []string{"text"}},
}

var dimAgentThinkingLevelOrder = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

var dimAgentThinkingLevels = func() map[string]struct{} {
	levels := make(map[string]struct{}, len(dimAgentThinkingLevelOrder))
	for _, level := range dimAgentThinkingLevelOrder {
		levels[level] = struct{}{}
	}
	return levels
}()

func ParseDimAgentModelCapabilities(body []byte) ([]DimAgentModelCapability, error) {
	_, entries, err := modelCatalogEntries(body, "data")
	if err != nil {
		return nil, fmt.Errorf("parse DimAgent capability catalog: %w", err)
	}
	if len(entries) == 0 {
		return nil, errors.New("DimAgent capability catalog is empty")
	}
	seen := make(map[string]struct{}, len(entries))
	models := make([]DimAgentModelCapability, 0, len(entries))
	for _, raw := range entries {
		var entry map[string]any
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, fmt.Errorf("parse DimAgent capability entry: %w", err)
		}
		id := strings.TrimSpace(dimAgentStringValue(entry["id"]))
		if id == "" {
			return nil, errors.New("DimAgent capability entry has no id")
		}
		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf("DimAgent capability catalog contains duplicate id %q", id)
		}
		seen[id] = struct{}{}
		capability, ok, err := parseDimAgentCapabilityEntry(id, entry)
		if err != nil {
			return nil, err
		}
		if ok {
			models = append(models, capability)
		}
	}
	if len(models) == 0 {
		return nil, errors.New("DimAgent capability catalog contains no chat models")
	}
	sort.SliceStable(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func parseDimAgentCapabilityEntry(id string, entry map[string]any) (DimAgentModelCapability, bool, error) {
	fallback, hasFallback := dimAgentFallbackModels[strings.ToLower(id)]
	dim, hasDim := dimAgentObjectValue(entry["dim"])
	if !hasDim || len(dim) == 0 {
		if !hasFallback {
			return DimAgentModelCapability{}, false, nil
		}
		return fallbackDimAgentCapability(id, fallback), true, nil
	}
	if kind := strings.TrimSpace(dimAgentStringValue(dim["type"])); kind != "" && kind != "chat" {
		return DimAgentModelCapability{}, false, nil
	}
	limit, _ := dimAgentObjectValue(dim["limit"])
	contextWindow := dimAgentPositiveInt(limit["context"])
	maxOutput := dimAgentPositiveInt(limit["output"])
	if contextWindow == 0 && hasFallback {
		contextWindow = fallback.Context
	}
	if maxOutput == 0 && hasFallback {
		maxOutput = fallback.Output
	}
	if contextWindow == 0 || maxOutput == 0 {
		return DimAgentModelCapability{}, false, nil
	}
	// Account model_mapping deliberately writes the public alias to the outer
	// display_name. Prefer it over upstream-private dim names so the picker and
	// request id remain aligned.
	name := dimAgentFirstString(entry["display_name"], entry["name"], dim["displayName"], dim["display_name"], dim["name"])
	if name == "" {
		name = id
	}
	modalities := dimAgentInputModalities(dim["modalities"])
	if len(modalities) == 0 && dimAgentBoolValue(dim["vision"]) {
		modalities = []string{"text", "image"}
	}
	if len(modalities) == 0 && hasFallback {
		modalities = append([]string(nil), fallback.Modalities...)
	}
	if len(modalities) == 0 {
		modalities = []string{"text"}
	}
	reasoning := parseDimAgentReasoning(dim, fallback, hasFallback)
	return DimAgentModelCapability{
		ID: id, DisplayName: name, Protocol: "openai-completions",
		ContextWindow: contextWindow, MaxOutputTokens: maxOutput,
		InputModalities: modalities, Reasoning: reasoning,
		ToolCall: dimAgentBoolValue(dim["tool_call"]), StructuredOutput: dimAgentBoolValue(dim["structured_output"]),
	}, true, nil
}

func fallbackDimAgentCapability(id string, fallback dimAgentFallbackModel) DimAgentModelCapability {
	var reasoning *DimAgentReasoningCapability
	if len(fallback.Efforts) > 0 {
		efforts := make([]DimAgentReasoningEffort, 0, len(fallback.Efforts))
		for _, effort := range fallback.Efforts {
			efforts = append(efforts, DimAgentReasoningEffort{Level: effort, WireValue: effort})
		}
		reasoning = &DimAgentReasoningCapability{Supported: true, DefaultLevel: fallback.Default, Efforts: efforts}
	}
	return DimAgentModelCapability{ID: id, DisplayName: fallback.Name, Protocol: "openai-completions", ContextWindow: fallback.Context, MaxOutputTokens: fallback.Output, InputModalities: append([]string(nil), fallback.Modalities...), Reasoning: reasoning}
}

func parseDimAgentReasoning(dim map[string]any, fallback dimAgentFallbackModel, hasFallback bool) *DimAgentReasoningCapability {
	raw, _ := dimAgentObjectValue(dim["reasoning"])
	if raw != nil && raw["supported"] == false {
		return nil
	}
	wireValues := make([]string, 0)
	wireValues = append(wireValues, dimAgentStringSlice(raw["effortOptions"])...)
	wireValues = append(wireValues, dimAgentStringSlice(raw["effort_options"])...)
	if extra, ok := dimAgentObjectValue(dim["extra_capabilities"]); ok {
		if extraReasoning, ok := dimAgentObjectValue(extra["reasoning"]); ok {
			wireValues = append(wireValues, dimAgentStringSlice(extraReasoning["effortOptions"])...)
			wireValues = append(wireValues, dimAgentStringSlice(extraReasoning["effort_options"])...)
		}
	}
	if options, ok := dim["reasoning_options"].([]any); ok {
		for _, option := range options {
			candidate, ok := dimAgentObjectValue(option)
			if ok && dimAgentStringValue(candidate["type"]) == "effort" {
				wireValues = append(wireValues, dimAgentStringSlice(candidate["values"])...)
			}
		}
	}
	if len(wireValues) == 0 && hasFallback {
		wireValues = append(wireValues, fallback.Efforts...)
	}
	seen := make(map[string]string)
	for _, wire := range wireValues {
		wire = strings.TrimSpace(wire)
		level := wire
		if wire == "none" {
			level = "off"
		}
		if _, allowed := dimAgentThinkingLevels[level]; !allowed || wire == "" {
			continue
		}
		if _, exists := seen[level]; !exists {
			seen[level] = wire
		}
	}
	efforts := make([]DimAgentReasoningEffort, 0, len(seen))
	for _, level := range dimAgentThinkingLevelOrder {
		if wire, ok := seen[level]; ok {
			efforts = append(efforts, DimAgentReasoningEffort{Level: level, WireValue: wire})
		}
	}
	if len(efforts) == 0 {
		return nil
	}
	defaultLevel := strings.TrimSpace(dimAgentFirstString(raw["effort"], raw["defaultEffort"], raw["default_effort"]))
	if defaultLevel == "none" {
		defaultLevel = "off"
	}
	if _, ok := seen[defaultLevel]; !ok {
		defaultLevel = fallback.Default
		if _, ok := seen[defaultLevel]; !ok {
			defaultLevel = ""
		}
	}
	return &DimAgentReasoningCapability{Supported: true, DefaultLevel: defaultLevel, Efforts: efforts}
}

func MergeDimAgentModelCapabilities(catalogs [][]DimAgentModelCapability) ([]DimAgentModelCapability, error) {
	if len(catalogs) == 0 {
		return nil, errors.New("DimAgent capability catalogs contain no models")
	}
	byID := make(map[string]DimAgentModelCapability, len(catalogs[0]))
	for _, model := range catalogs[0] {
		byID[model.ID] = cloneDimAgentCapability(model)
	}
	for _, catalog := range catalogs[1:] {
		next := make(map[string]DimAgentModelCapability, len(catalog))
		for _, model := range catalog {
			next[model.ID] = model
		}
		for id, current := range byID {
			candidate, ok := next[id]
			if !ok {
				delete(byID, id)
				continue
			}
			byID[id] = intersectDimAgentCapability(current, candidate)
		}
	}
	if len(byID) == 0 {
		return nil, errors.New("DimAgent accounts have no common capable chat model")
	}
	models := make([]DimAgentModelCapability, 0, len(byID))
	for _, model := range byID {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func intersectDimAgentCapability(a, b DimAgentModelCapability) DimAgentModelCapability {
	a.ContextWindow = dimAgentMinPositive(a.ContextWindow, b.ContextWindow)
	a.MaxOutputTokens = dimAgentMinPositive(a.MaxOutputTokens, b.MaxOutputTokens)
	a.InputModalities = dimAgentIntersectStrings(a.InputModalities, b.InputModalities)
	a.ToolCall = a.ToolCall && b.ToolCall
	a.StructuredOutput = a.StructuredOutput && b.StructuredOutput
	a.Reasoning = intersectDimAgentReasoning(a.Reasoning, b.Reasoning)
	return a
}

func intersectDimAgentReasoning(a, b *DimAgentReasoningCapability) *DimAgentReasoningCapability {
	if a == nil || b == nil || !a.Supported || !b.Supported {
		return nil
	}
	bEfforts := make(map[string]string, len(b.Efforts))
	for _, effort := range b.Efforts {
		bEfforts[effort.Level] = effort.WireValue
	}
	efforts := make([]DimAgentReasoningEffort, 0)
	for _, effort := range a.Efforts {
		if wire, ok := bEfforts[effort.Level]; ok && wire == effort.WireValue {
			efforts = append(efforts, effort)
		}
	}
	if len(efforts) == 0 {
		return nil
	}
	defaultLevel := ""
	if a.DefaultLevel != "" && a.DefaultLevel == b.DefaultLevel {
		defaultLevel = a.DefaultLevel
	}
	return &DimAgentReasoningCapability{Supported: true, DefaultLevel: defaultLevel, Efforts: efforts}
}

func DimAgentCatalogVersion(groupID int64, models []DimAgentModelCapability) string {
	hasher := sha256.New()
	_, _ = fmt.Fprintf(hasher, "group:%d\n", groupID)
	encoded, _ := json.Marshal(models)
	_, _ = hasher.Write(encoded)
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}

// FetchDimAgentGroupCapabilities builds the DSH-facing catalog authorized by one
// API-key group. OAuth credentials and upstream private metadata never leave the
// service; only normalized capabilities do.
func (s *OpenAIGatewayService) FetchDimAgentGroupCapabilities(ctx context.Context, group *Group) (*DimAgentModelCapabilitiesEnvelope, error) {
	if group == nil || group.Platform != PlatformDimAgent {
		return nil, infraerrors.New(http.StatusNotFound, "DIMAGENT_CAPABILITIES_UNAVAILABLE", "model capabilities are only available for DimAgent groups")
	}
	if s == nil || s.accountRepo == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "DIMAGENT_CAPABILITIES_ACCOUNT_REPOSITORY_UNAVAILABLE", "DimAgent account repository is unavailable")
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, group.ID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "DIMAGENT_CAPABILITIES_ACCOUNTS_FAILED", "cannot list DimAgent accounts: %v", err)
	}
	catalogs := make([][]DimAgentModelCapability, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if !account.IsDimAgent() {
			continue
		}
		response, err := s.FetchOpenAIModelsList(ctx, account)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "DIMAGENT_CAPABILITIES_UPSTREAM_FAILED", "cannot obtain the DimAgent capability catalog for account %d: %v", account.ID, err)
		}
		body, err := projectAccountModelsBody(response.Body, account, group, false)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "DIMAGENT_CAPABILITIES_MAPPING_FAILED", "cannot project the DimAgent capability catalog for account %d: %v", account.ID, err)
		}
		models, err := ParseDimAgentModelCapabilities(body)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "DIMAGENT_CAPABILITIES_INVALID", "invalid DimAgent capability catalog for account %d: %v", account.ID, err)
		}
		catalogs = append(catalogs, models)
	}
	if len(catalogs) == 0 {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "DIMAGENT_CAPABILITIES_NO_ACCOUNTS", "no schedulable DimAgent account is available")
	}
	models, err := MergeDimAgentModelCapabilities(catalogs)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "DIMAGENT_CAPABILITIES_INVALID", "%v", err)
	}
	if group.ModelAllowlistEnabled() {
		models = filterDimAgentCapabilities(models, group.ModelAllowlist.FilterForListing(dimAgentCapabilityIDs(models)))
		if len(models) == 0 {
			return nil, infraerrors.New(http.StatusNotFound, "DIMAGENT_CAPABILITIES_EMPTY", "the DimAgent group allowlist exposes no capable chat model")
		}
	}
	return &DimAgentModelCapabilitiesEnvelope{
		Object: DimAgentCapabilitiesObject, CatalogVersion: DimAgentCatalogVersion(group.ID, models),
		CatalogScope: DimAgentCatalogScope{Platform: PlatformDimAgent, GroupID: group.ID}, Data: models,
	}, nil
}

func dimAgentCapabilityIDs(models []DimAgentModelCapability) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

func filterDimAgentCapabilities(models []DimAgentModelCapability, selected []string) []DimAgentModelCapability {
	byID := make(map[string]DimAgentModelCapability, len(models))
	for _, model := range models {
		byID[model.ID] = model
	}
	filtered := make([]DimAgentModelCapability, 0, len(selected))
	for _, id := range selected {
		if model, ok := byID[id]; ok {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func cloneDimAgentCapability(model DimAgentModelCapability) DimAgentModelCapability {
	model.InputModalities = append([]string(nil), model.InputModalities...)
	if model.Reasoning != nil {
		reasoning := *model.Reasoning
		reasoning.Efforts = append([]DimAgentReasoningEffort(nil), model.Reasoning.Efforts...)
		model.Reasoning = &reasoning
	}
	return model
}

func dimAgentInputModalities(value any) []string {
	if object, ok := dimAgentObjectValue(value); ok {
		return dimAgentNormalizeModalities(dimAgentStringSlice(object["input"]))
	}
	return dimAgentNormalizeModalities(dimAgentStringSlice(value))
}

func dimAgentNormalizeModalities(values []string) []string {
	out := make([]string, 0, 2)
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "text" && value != "image" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func dimAgentIntersectStrings(a, b []string) []string {
	set := make(map[string]struct{}, len(b))
	for _, value := range b {
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(a))
	for _, value := range a {
		if _, ok := set[value]; ok {
			out = append(out, value)
		}
	}
	return out
}

func dimAgentObjectValue(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}
func dimAgentStringValue(value any) string { valueString, _ := value.(string); return valueString }
func dimAgentBoolValue(value any) bool     { valueBool, _ := value.(bool); return valueBool }
func dimAgentFirstString(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(dimAgentStringValue(value)); text != "" {
			return text
		}
	}
	return ""
}
func dimAgentStringSlice(value any) []string {
	values, _ := value.([]any)
	out := make([]string, 0, len(values))
	for _, item := range values {
		if text := strings.TrimSpace(dimAgentStringValue(item)); text != "" {
			out = append(out, text)
		}
	}
	return out
}
func dimAgentPositiveInt(value any) int {
	number, ok := value.(float64)
	if !ok || number <= 0 {
		return 0
	}
	return int(number)
}
func dimAgentMinPositive(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}
