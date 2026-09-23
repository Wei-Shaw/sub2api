package cursor

import "strings"

// Model is the OpenAI-style /v1/models item for cursor groups.
type Model struct {
	ID          string   `json:"id"`
	Object      string   `json:"object"`
	Created     int64    `json:"created,omitempty"`
	OwnedBy     string   `json:"owned_by"`
	DisplayName string   `json:"display_name,omitempty"`
	Aliases     []string `json:"-"`
}

// DefaultModelIDs is the static fallback advertised on GET /v1/models when
// AvailableModels cannot be reached. Live picker slugs come from Cursor's API.
func DefaultModelIDs() []string {
	ids := make([]string, 0, len(DefaultModels))
	for _, model := range DefaultModels {
		ids = append(ids, model.ID)
	}
	return ids
}

func pickerModel(id, display string, aliases ...string) Model {
	return Model{
		ID:          id,
		Object:      "model",
		OwnedBy:     "cursor",
		DisplayName: display,
		Aliases:     aliases,
	}
}

// DefaultModels is used only when the live Cursor catalog cannot be fetched.
// Slugs match Cursor Pro AvailableModels / the IDE picker (including models
// toggled off for Auto). composer-1 is kept for older clients.
var DefaultModels = []Model{
	pickerModel("default", "Auto", "auto"),
	pickerModel("grok-4.6", "Cursor Grok 4.6", "cursor-grok-4.6"),
	pickerModel("composer-2.5", "Composer 2.5", "composer", "composer-latest", "composer-2-5"),
	pickerModel("claude-opus-5", "Claude Opus 5", "opus", "opus-latest", "opus-5"),
	pickerModel("claude-opus-4-8", "Claude Opus 4.8", "opus-4.8", "opus-4-8"),
	pickerModel("gpt-5.6-sol", "GPT-5.6 Sol", "gpt-5.6", "gpt-latest", "gpt", "gpt-5-6-sol"),
	pickerModel("gpt-5.5", "GPT-5.5", "gpt-5-5"),
	pickerModel("claude-fable-5", "Claude Fable 5", "fable", "fable-5"),
	pickerModel("grok-4.5", "Cursor Grok 4.5", "cursor-grok-4.5"),
	pickerModel("gemini-3.7-flash", "Gemini 3.7 Flash", "gemini-flash-latest", "gemini-flash"),
	pickerModel("gpt-5.6-terra", "GPT-5.6 Terra", "gpt-5-6-terra"),
	pickerModel("claude-sonnet-5", "Claude Sonnet 5", "sonnet-latest", "sonnet-5"),
	pickerModel("claude-sonnet-4-6", "Claude Sonnet 4.6", "sonnet", "sonnet-4.6", "sonnet-4-6"),
	pickerModel("gpt-5.3-codex", "Codex 5.3", "codex", "codex-latest", "codex-5.3"),
	pickerModel("claude-opus-4-7", "Claude Opus 4.7", "opus-4.7", "opus-4-7"),
	pickerModel("gpt-5.4", "GPT-5.4"),
	pickerModel("claude-opus-4-6", "Claude Opus 4.6", "opus-4.6", "opus-4-6"),
	pickerModel("claude-opus-4-5", "Claude Opus 4.5", "opus-4.5", "opus-4-5"),
	pickerModel("gpt-5.2", "GPT-5.2"),
	pickerModel("gpt-5.6-luna", "GPT-5.6 Luna", "gpt-5-6-luna"),
	pickerModel("gemini-3.6-flash", "Gemini 3.6 Flash"),
	pickerModel("gemini-3.1-pro", "Gemini 3.1 Pro", "gemini", "gemini-latest", "gemini-pro", "gemini-pro-latest"),
	pickerModel("gpt-5.4-mini", "GPT-5.4 Mini", "gpt-mini-latest", "gpt-mini"),
	pickerModel("gpt-5.4-nano", "GPT-5.4 Nano", "gpt-nano-latest", "gpt-nano"),
	pickerModel("claude-haiku-4-5", "Claude Haiku 4.5", "haiku", "haiku-latest", "haiku-4.5", "haiku-4-5"),
	pickerModel("claude-sonnet-4-5", "Claude Sonnet 4.5", "sonnet-4.5", "sonnet-4-5"),
	pickerModel("gpt-5.1", "GPT-5.1"),
	pickerModel("gemini-3-flash", "Gemini 3 Flash"),
	pickerModel("gemini-3.5-flash", "Gemini 3.5 Flash"),
	pickerModel("claude-sonnet-4", "Claude Sonnet 4", "sonnet-4"),
	pickerModel("gpt-5-mini", "GPT-5 Mini"),
	pickerModel("gemini-2.5-flash", "Gemini 2.5 Flash"),
	pickerModel("kimi-k3", "Kimi K3"),
	pickerModel("kimi-k2.7-code", "Kimi K2.7 Code", "kimi", "kimi-latest"),
	pickerModel("glm-5.2", "GLM 5.2"),
	pickerModel("GLM-4.7", "GLM-4.7"),
	pickerModel("GLM-5.1", "GLM-5.1"),
	pickerModel("composer-1", "Composer 1"),
}

// ResolveModel maps a client model id, alias, or display name onto a Cursor
// picker slug. usedFallback is true when the resolved slug differs from the
// requested value (after trim).
func ResolveModel(requested string) (canonical string, usedFallback bool) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", false
	}
	if id, ok := lookupPickerModel(requested); ok {
		return id, id != requested
	}
	return requested, false
}

func lookupPickerModel(requested string) (string, bool) {
	if id, ok := matchPickerKey(requested); ok {
		return id, true
	}
	normalized := normalizePickerKey(requested)
	if normalized == "" || normalized == requested {
		return "", false
	}
	return matchPickerKey(normalized)
}

func matchPickerKey(key string) (string, bool) {
	for _, model := range DefaultModels {
		if model.ID == key || strings.EqualFold(model.ID, key) {
			return model.ID, true
		}
		if strings.EqualFold(model.DisplayName, key) || normalizePickerKey(model.DisplayName) == key {
			return model.ID, true
		}
		for _, alias := range model.Aliases {
			if alias == key || strings.EqualFold(alias, key) || normalizePickerKey(alias) == key {
				return model.ID, true
			}
		}
	}
	return "", false
}

func normalizePickerKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if i := strings.LastIndex(value, "/"); i >= 0 {
		value = value[i+1:]
	}
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "_", "-")
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, "-")
}

// ModelsFromAvailable converts a live AvailableModels catalog to OpenAI-style items.
func ModelsFromAvailable(models []AvailableModel) []Model {
	out := make([]Model, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.Name)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		display := strings.TrimSpace(model.DisplayName)
		if display == "" {
			display = id
		}
		out = append(out, Model{
			ID:          id,
			Object:      "model",
			OwnedBy:     "cursor",
			DisplayName: display,
			Aliases:     append(append([]string{}, model.Aliases...), model.LegacySlugs...),
		})
	}
	return out
}
