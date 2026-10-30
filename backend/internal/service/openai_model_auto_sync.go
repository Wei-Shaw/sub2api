package service

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	OpenAIModelAutoSyncEnabledExtraKey = "openai_model_auto_sync_enabled"
	OpenAIModelAutoSyncStateExtraKey   = "openai_model_auto_sync"
	openAIModelAutoSyncInterval        = time.Hour
)

var ErrOpenAIModelAutoSyncChanged = errors.New("account configuration changed during model synchronization")

// OpenAIModelAutoSyncState records ownership independently of the whitelist.
// Only unchanged identity mappings created by automatic sync may be removed.
type OpenAIModelAutoSyncState struct {
	Status         string   `json:"status"`
	ErrorCode      string   `json:"error_code,omitempty"`
	LastAttemptAt  string   `json:"last_attempt_at,omitempty"`
	LastSuccessAt  string   `json:"last_success_at,omitempty"`
	NextSyncAt     string   `json:"next_sync_at,omitempty"`
	NextSyncUnix   int64    `json:"next_sync_unix"`
	ManagedModels  []string `json:"managed_models,omitempty"`
	MissingModels  []string `json:"missing_models,omitempty"`
	ExcludedModels []string `json:"excluded_models,omitempty"`
}

func isOpenAIModelAutoSyncAccount(account *Account) bool {
	return account != nil && account.Platform == PlatformOpenAI &&
		(account.Type == AccountTypeOAuth || account.Type == AccountTypeAPIKey) && !account.IsShadow()
}

func openAIModelAutoSyncEnabled(account *Account) bool {
	if !isOpenAIModelAutoSyncAccount(account) {
		return false
	}
	enabled, _ := account.Extra[OpenAIModelAutoSyncEnabledExtraKey].(bool)
	return enabled
}

func normalizeOpenAIModelAutoSyncExtra(platform, accountType string, shadow bool, extra map[string]any) (map[string]any, error) {
	if extra == nil {
		return nil, nil
	}
	next := maps.Clone(extra)
	delete(next, OpenAIModelAutoSyncStateExtraKey)
	if raw, present := next[OpenAIModelAutoSyncEnabledExtraKey]; present {
		if _, ok := raw.(bool); !ok {
			return nil, infraerrors.BadRequest("INVALID_MODEL_AUTO_SYNC_ENABLED", "openai_model_auto_sync_enabled must be a boolean")
		}
		if platform != PlatformOpenAI || (accountType != AccountTypeOAuth && accountType != AccountTypeAPIKey) || shadow {
			return nil, infraerrors.BadRequest("INVALID_MODEL_AUTO_SYNC_ACCOUNT", "automatic model sync requires an OpenAI OAuth or API key account, excluding shadows")
		}
	}
	return next, nil
}

func decodeOpenAIModelAutoSyncState(extra map[string]any) OpenAIModelAutoSyncState {
	var state OpenAIModelAutoSyncState
	if raw := extra[OpenAIModelAutoSyncStateExtraKey]; raw != nil {
		if body, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(body, &state)
		}
	}
	return state
}

func openAIModelSyncAttempt(previous OpenAIModelAutoSyncState, now time.Time) OpenAIModelAutoSyncState {
	next := previous
	next.LastAttemptAt = now.UTC().Format(time.RFC3339)
	next.NextSyncAt = now.Add(openAIModelAutoSyncInterval).UTC().Format(time.RFC3339)
	next.NextSyncUnix = now.Add(openAIModelAutoSyncInterval).Unix()
	next.ErrorCode = ""
	return next
}

func failedOpenAIModelSync(previous OpenAIModelAutoSyncState, now time.Time, code string) OpenAIModelAutoSyncState {
	next := openAIModelSyncAttempt(previous, now)
	next.Status = "error"
	next.ErrorCode = code
	// A failed observation breaks the consecutive-success retirement check.
	next.MissingModels = nil
	return next
}

// reconcileOpenAIModels never takes ownership of pre-existing mappings. Exact
// additions also respect wildcards, which would otherwise lose precedence.
func reconcileOpenAIModels(account *Account, models []string, previous OpenAIModelAutoSyncState, now time.Time) (map[string]string, OpenAIModelAutoSyncState) {
	current := stringMappingFromRaw(account.Credentials["model_mapping"])
	next := maps.Clone(current)
	if next == nil {
		next = make(map[string]string)
	}
	live := make(map[string]bool, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || strings.ContainsAny(model, "*\r\n\t") {
			return nil, failedOpenAIModelSync(previous, now, "invalid_model_list")
		}
		live[model] = true
	}
	if len(live) == 0 {
		return nil, failedOpenAIModelSync(previous, now, "empty_model_list")
	}

	state := openAIModelSyncAttempt(previous, now)
	state.Status = "success"
	state.ManagedModels = nil
	state.MissingModels = nil
	excluded := make(map[string]bool, len(previous.ExcludedModels))
	for _, model := range previous.ExcludedModels {
		excluded[model] = true
	}
	for _, model := range previous.ManagedModels {
		target, exists := current[model]
		if !exists || target != model {
			// A manual deletion or rewrite relinquishes automatic ownership.
			excluded[model] = true
			continue
		}
		if !live[model] {
			if slices.Contains(previous.MissingModels, model) {
				delete(next, model)
				continue
			}
			state.MissingModels = append(state.MissingModels, model)
		}
		state.ManagedModels = append(state.ManagedModels, model)
	}
	for model := range live {
		if excluded[model] || mappingSupportsRequestedModel(next, model) {
			continue
		}
		next[model] = model
		state.ManagedModels = append(state.ManagedModels, model)
	}
	// An empty mapping means unrestricted access in the existing scheduler.
	// Never turn retirement of the final managed model into allow-all.
	if len(next) == 0 {
		return nil, failedOpenAIModelSync(previous, now, "empty_mapping")
	}
	state.ExcludedModels = make([]string, 0, len(excluded))
	for model := range excluded {
		state.ExcludedModels = append(state.ExcludedModels, model)
	}
	state.ManagedModels = dedupeAndSortModelIDs(state.ManagedModels)
	state.MissingModels = dedupeAndSortModelIDs(state.MissingModels)
	state.ExcludedModels = dedupeAndSortModelIDs(state.ExcludedModels)
	state.LastSuccessAt = now.UTC().Format(time.RFC3339)
	return next, state
}
