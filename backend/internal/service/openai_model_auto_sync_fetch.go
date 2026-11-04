package service

import (
	"context"
	"encoding/json"
	"strings"
)

// Automatic sync uses only live discovery, never the configured-model fallback
// used by the manual capability-enrichment endpoint.
func (s *AccountTestService) fetchOpenAIModelsForAutoSync(ctx context.Context, account *Account) ([]string, error) {
	models, body, err := s.fetchUpstreamModelList(ctx, account)
	if err != nil {
		return nil, err
	}
	return filterOpenAIModelAutoSyncResponse(account, models, body)
}

func filterOpenAIModelAutoSyncResponse(account *Account, models []string, body []byte) ([]string, error) {
	var envelope struct {
		HasMore       bool            `json:"has_more"`
		Next          json.RawMessage `json:"next"`
		NextPage      json.RawMessage `json:"next_page"`
		NextPageToken string          `json:"next_page_token"`
		NextToken     string          `json:"nextPageToken"`
		Models        []struct {
			ID         string `json:"id"`
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
		} `json:"models"`
	}
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, newUpstreamModelSyncUpstreamError("Invalid model list envelope", err)
		}
		if envelope.HasMore || modelSyncHasNextPage(envelope.Next) || modelSyncHasNextPage(envelope.NextPage) ||
			envelope.NextPageToken != "" || envelope.NextToken != "" {
			return nil, newUpstreamModelSyncUnsupportedError("Automatic model sync requires a complete, unpaginated model list", nil)
		}
	}
	if account.Type != AccountTypeOAuth {
		return models, nil
	}
	hidden := make(map[string]bool)
	// supported_in_api describes API-key login, not ChatGPT subscription access.
	// OAuth-only models remain valid even when that flag is false.
	for _, entry := range envelope.Models {
		if entry.Visibility == "hide" {
			id := strings.TrimSpace(entry.ID)
			if id == "" {
				id = strings.TrimSpace(entry.Slug)
			}
			hidden[id] = true
		}
	}
	filtered := make([]string, 0, len(models))
	for _, model := range models {
		if !hidden[model] && !isRetiredOpenAISubscriptionModel(account, model) {
			filtered = append(filtered, model)
		}
	}
	return filtered, nil
}

func modelSyncHasNextPage(raw json.RawMessage) bool {
	switch strings.TrimSpace(string(raw)) {
	case "", "null", `""`, "false", "0":
		return false
	default:
		return true
	}
}
