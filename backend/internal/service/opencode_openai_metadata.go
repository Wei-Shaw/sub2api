package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	openCodeModelsDevURL = "https://models.dev/api.json"
	openCodeModelsTTL    = 15 * time.Minute
)

type OpenCodeOpenAIModel struct {
	ID               string                        `json:"id"`
	Name             string                        `json:"name"`
	Family           string                        `json:"family,omitempty"`
	Attachment       bool                          `json:"attachment"`
	Reasoning        bool                          `json:"reasoning"`
	ToolCall         bool                          `json:"tool_call"`
	StructuredOutput bool                          `json:"structured_output"`
	Temperature      bool                          `json:"temperature"`
	Knowledge        string                        `json:"knowledge,omitempty"`
	Interleaved      any                           `json:"interleaved,omitempty"`
	Modalities       OpenCodeOpenAIModelModalities `json:"modalities,omitempty"`
	Cost             OpenCodeOpenAIModelCost       `json:"cost,omitempty"`
	Limit            OpenCodeOpenAIModelLimit      `json:"limit,omitempty"`
	ReleaseDate      string                        `json:"release_date,omitempty"`
	Options          map[string]any                `json:"options,omitempty"`
	Headers          map[string]string             `json:"headers,omitempty"`
}

type OpenCodeOpenAIModelModalities struct {
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
}

type OpenCodeOpenAIModelCost struct {
	Input           float64                        `json:"input,omitempty"`
	Output          float64                        `json:"output,omitempty"`
	CacheRead       float64                        `json:"cache_read,omitempty"`
	CacheWrite      float64                        `json:"cache_write,omitempty"`
	ContextOver200K OpenCodeOpenAIModelContextCost `json:"context_over_200k,omitempty"`
}

type OpenCodeOpenAIModelContextCost struct {
	Input      float64 `json:"input,omitempty"`
	Output     float64 `json:"output,omitempty"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
}

type OpenCodeOpenAIModelLimit struct {
	Context int `json:"context,omitempty"`
	Input   int `json:"input,omitempty"`
	Output  int `json:"output,omitempty"`
}

type OpenCodeMetadataService struct {
	client *http.Client
	url    string
	ttl    time.Duration
	mu     sync.RWMutex
	cache  map[string]OpenCodeOpenAIModel
	exp    time.Time
}

func NewOpenCodeMetadataService() *OpenCodeMetadataService {
	return &OpenCodeMetadataService{
		client: &http.Client{Timeout: 15 * time.Second},
		url:    openCodeModelsDevURL,
		ttl:    openCodeModelsTTL,
	}
}

func (s *OpenCodeMetadataService) GetOpenAIModels(ctx context.Context) (map[string]OpenCodeOpenAIModel, error) {
	if s == nil {
		return nil, fmt.Errorf("opencode metadata service unavailable")
	}

	s.mu.RLock()
	if len(s.cache) > 0 && time.Now().Before(s.exp) {
		cached := cloneOpenCodeOpenAIModels(s.cache)
		s.mu.RUnlock()
		return cached, nil
	}
	stale := cloneOpenCodeOpenAIModels(s.cache)
	s.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		if len(stale) > 0 {
			return stale, nil
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if len(stale) > 0 {
			return stale, nil
		}
		return nil, fmt.Errorf("models.dev status: %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		if len(stale) > 0 {
			return stale, nil
		}
		return nil, err
	}

	models, err := extractOpenCodeOpenAIModels(payload)
	if err != nil {
		if len(stale) > 0 {
			return stale, nil
		}
		return nil, err
	}

	s.mu.Lock()
	s.cache = cloneOpenCodeOpenAIModels(models)
	s.exp = time.Now().Add(s.ttl)
	s.mu.Unlock()

	return models, nil
}

func extractOpenCodeOpenAIModels(payload map[string]any) (map[string]OpenCodeOpenAIModel, error) {
	provider, ok := mapValue(payload, "openai")
	if !ok {
		return nil, fmt.Errorf("openai provider missing")
	}
	modelsRaw, ok := mapValue(provider, "models")
	if !ok {
		return nil, fmt.Errorf("openai models missing")
	}

	models := map[string]OpenCodeOpenAIModel{}
	for id, item := range modelsRaw {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if !shouldKeepOpenCodeOpenAIModel(id, raw) {
			continue
		}
		model, err := convertOpenCodeOpenAIModel(raw)
		if err != nil {
			return nil, fmt.Errorf("convert %s: %w", id, err)
		}
		if model.ID == "" {
			model.ID = id
		}
		models[id] = model
		for derivedID, derivedModel := range materializeOpenCodeOpenAIExperimentalModes(id, model, raw) {
			models[derivedID] = derivedModel
		}
	}

	return filterOpenCodeOpenAIModelsForCodexOAuth(models), nil
}

func filterOpenCodeOpenAIModelsForCodexOAuth(src map[string]OpenCodeOpenAIModel) map[string]OpenCodeOpenAIModel {
	if len(src) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"gpt-5.1-codex-max":  {},
		"gpt-5.1-codex-mini": {},
		"gpt-5.2":            {},
		"gpt-5.4":            {},
		"gpt-5.4-mini":       {},
		"gpt-5.2-codex":      {},
		"gpt-5.3-codex":      {},
		"gpt-5.1-codex":      {},
	}
	out := map[string]OpenCodeOpenAIModel{}
	for id, model := range src {
		if shouldAllowOpenCodeOpenAIModelForCodexOAuth(id, allowed) {
			out[id] = model
		}
	}
	return out
}

func shouldAllowOpenCodeOpenAIModelForCodexOAuth(id string, allowed map[string]struct{}) bool {
	if strings.Contains(id, "codex") {
		return true
	}
	if _, ok := allowed[id]; ok {
		return true
	}
	if strings.HasSuffix(id, "-fast") {
		return shouldAllowOpenCodeOpenAIModelForCodexOAuth(strings.TrimSuffix(id, "-fast"), allowed)
	}
	return false
}

func shouldKeepOpenCodeOpenAIModel(id string, raw map[string]any) bool {
	status := strings.ToLower(strings.TrimSpace(stringValue(raw, "status")))
	if id == "gpt-5-chat-latest" {
		return false
	}
	if status == "alpha" || status == "deprecated" {
		return false
	}
	return true
}

func convertOpenCodeOpenAIModel(raw map[string]any) (OpenCodeOpenAIModel, error) {
	id := stringValue(raw, "id")
	name := stringValue(raw, "name")
	if id == "" || name == "" {
		return OpenCodeOpenAIModel{}, fmt.Errorf("id/name missing")
	}

	model := OpenCodeOpenAIModel{
		ID:               id,
		Name:             name,
		Family:           stringValue(raw, "family"),
		Attachment:       boolValue(raw, "attachment"),
		Reasoning:        boolValue(raw, "reasoning"),
		ToolCall:         boolValue(raw, "tool_call"),
		StructuredOutput: boolValue(raw, "structured_output"),
		Temperature:      boolValue(raw, "temperature"),
		Knowledge:        stringValue(raw, "knowledge"),
		Interleaved:      raw["interleaved"],
		ReleaseDate:      stringValue(raw, "release_date"),
	}

	if modalities, ok := mapValue(raw, "modalities"); ok {
		model.Modalities = OpenCodeOpenAIModelModalities{
			Input:  stringSliceValue(modalities["input"]),
			Output: stringSliceValue(modalities["output"]),
		}
	}

	if cost, ok := mapValue(raw, "cost"); ok {
		model.Cost = OpenCodeOpenAIModelCost{
			Input:      floatValue(cost, "input"),
			Output:     floatValue(cost, "output"),
			CacheRead:  floatValue(cost, "cache_read"),
			CacheWrite: floatValue(cost, "cache_write"),
		}
		if over, ok := mapValue(cost, "context_over_200k"); ok {
			model.Cost.ContextOver200K = OpenCodeOpenAIModelContextCost{
				Input:      floatValue(over, "input"),
				Output:     floatValue(over, "output"),
				CacheRead:  floatValue(over, "cache_read"),
				CacheWrite: floatValue(over, "cache_write"),
			}
		}
	}

	if limit, ok := mapValue(raw, "limit"); ok {
		model.Limit = OpenCodeOpenAIModelLimit{
			Context: intValue(limit, "context"),
			Input:   intValue(limit, "input"),
			Output:  intValue(limit, "output"),
		}
	}

	return model, nil
}

func materializeOpenCodeOpenAIExperimentalModes(baseID string, baseModel OpenCodeOpenAIModel, raw map[string]any) map[string]OpenCodeOpenAIModel {
	experimental, ok := mapValue(raw, "experimental")
	if !ok {
		return nil
	}
	modes, ok := mapValue(experimental, "modes")
	if !ok {
		return nil
	}

	derived := make(map[string]OpenCodeOpenAIModel, len(modes))
	// Mirrors anomalyco/opencode packages/opencode/src/provider/provider.ts::fromModelsDevProvider()
	// at commit 7a6ce05, especially the experimental.modes flattening at
	// https://github.com/anomalyco/opencode/blob/7a6ce05/packages/opencode/src/provider/provider.ts#L1004-L1020.
	// backend only mirrors/flattens upstream model aliases here; local -Sys and builtin_tools
	// expansions are handled later by our own recommendation layer.
	for mode, item := range modes {
		mode = strings.TrimSpace(mode)
		if mode == "" {
			continue
		}
		rawMode, ok := item.(map[string]any)
		if !ok {
			continue
		}

		derivedID := baseID + "-" + mode
		model := cloneOpenCodeOpenAIModel(baseModel)
		model.ID = derivedID
		model.Name = baseModel.Name + " " + strings.ToUpper(mode[:1]) + mode[1:]
		if cost, ok := mapValue(rawMode, "cost"); ok {
			model.Cost = mergeOpenCodeOpenAIModelCost(model.Cost, cost)
		}
		if provider, ok := mapValue(rawMode, "provider"); ok {
			if body, ok := mapValue(provider, "body"); ok {
				model.Options = toCamelCaseProviderBody(body)
			}
			if headers, ok := mapValue(provider, "headers"); ok {
				model.Headers = stringMapValue(headers)
			}
		}
		derived[derivedID] = model
	}
	return derived
}

func mergeOpenCodeOpenAIModelCost(base OpenCodeOpenAIModelCost, raw map[string]any) OpenCodeOpenAIModelCost {
	merged := base
	if _, ok := raw["input"]; ok {
		merged.Input = floatValue(raw, "input")
	}
	if _, ok := raw["output"]; ok {
		merged.Output = floatValue(raw, "output")
	}
	if _, ok := raw["cache_read"]; ok {
		merged.CacheRead = floatValue(raw, "cache_read")
	}
	if _, ok := raw["cache_write"]; ok {
		merged.CacheWrite = floatValue(raw, "cache_write")
	}
	if over, ok := mapValue(raw, "context_over_200k"); ok {
		if _, ok := over["input"]; ok {
			merged.ContextOver200K.Input = floatValue(over, "input")
		}
		if _, ok := over["output"]; ok {
			merged.ContextOver200K.Output = floatValue(over, "output")
		}
		if _, ok := over["cache_read"]; ok {
			merged.ContextOver200K.CacheRead = floatValue(over, "cache_read")
		}
		if _, ok := over["cache_write"]; ok {
			merged.ContextOver200K.CacheWrite = floatValue(over, "cache_write")
		}
	}
	return merged
}

func toCamelCaseProviderBody(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[toCamelCaseKey(key)] = value
	}
	return output
}

func toCamelCaseKey(input string) string {
	if input == "" || !strings.Contains(input, "_") {
		return input
	}
	parts := strings.Split(input, "_")
	if len(parts) == 0 {
		return input
	}
	var builder strings.Builder
	builder.WriteString(parts[0])
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		builder.WriteString(strings.ToUpper(part[:1]))
		if len(part) > 1 {
			builder.WriteString(part[1:])
		}
	}
	return builder.String()
}

func mapValue(raw map[string]any, key string) (map[string]any, bool) {
	value, ok := raw[key]
	if !ok {
		return nil, false
	}
	mapped, ok := value.(map[string]any)
	return mapped, ok
}

func stringValue(raw map[string]any, key string) string {
	value, ok := raw[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func boolValue(raw map[string]any, key string) bool {
	value, ok := raw[key]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}

func floatValue(raw map[string]any, key string) float64 {
	value, ok := raw[key]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}

func intValue(raw map[string]any, key string) int {
	value, ok := raw[key]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}

func stringSliceValue(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func stringMapValue(raw map[string]any) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out[key] = text
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneOpenCodeOpenAIModel(model OpenCodeOpenAIModel) OpenCodeOpenAIModel {
	copy := model
	copy.Modalities = OpenCodeOpenAIModelModalities{
		Input:  append([]string(nil), model.Modalities.Input...),
		Output: append([]string(nil), model.Modalities.Output...),
	}
	copy.Options = cloneAnyMap(model.Options)
	copy.Headers = cloneStringMap(model.Headers)
	return copy
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func cloneOpenCodeOpenAIModels(src map[string]OpenCodeOpenAIModel) map[string]OpenCodeOpenAIModel {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]OpenCodeOpenAIModel, len(src))
	for id, model := range src {
		out[id] = cloneOpenCodeOpenAIModel(model)
	}
	return out
}
