package service

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/meta"
)

func DefaultMetaModels() []meta.Model {
	return meta.DefaultModels()
}

func (s *AccountTestService) FetchMetaAccountModels(ctx context.Context, account *Account) ([]meta.Model, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, newUpstreamModelSyncConfigError("Upstream HTTP client is not configured", nil)
	}
	req, err := s.buildMetaUpstreamModelsRequest(ctx, account)
	if err != nil {
		return nil, err
	}
	body, err := s.readUpstreamModelsResponse(req, account)
	if err != nil {
		return nil, err
	}
	models, err := meta.ParseModelList(body)
	if err != nil {
		return nil, newUpstreamModelSyncUpstreamError("Upstream model list response has no usable muse models", err)
	}
	return projectMetaModels(models, account.GetModelMapping()), nil
}

func projectMetaModels(models []meta.Model, mapping map[string]string) []meta.Model {
	if len(mapping) == 0 {
		return models
	}
	byID := make(map[string]meta.Model, len(models))
	for _, model := range models {
		byID[model.ID] = model
	}
	requested := make([]string, 0, len(mapping))
	for source := range mapping {
		requested = append(requested, source)
	}
	sort.Strings(requested)

	projected := make([]meta.Model, 0, len(requested))
	for _, source := range requested {
		target := strings.TrimSpace(mapping[source])
		if target == "" {
			target = source
		}
		projected = append(projected, meta.NewModel(source))
		if upstream, ok := byID[target]; ok {
			entry := upstream
			entry.ID = source
			projected[len(projected)-1] = entry
		}
	}
	return projected
}

func (s *AccountTestService) buildMetaUpstreamModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	if account == nil {
		return nil, newUpstreamModelSyncConfigError("Account is required", nil)
	}
	if !account.IsMeta() {
		return nil, newUpstreamModelSyncUnsupportedError("Account is not a Meta account", nil)
	}

	apiKey := strings.TrimSpace(account.GetMetaAPIKey())
	if apiKey == "" && account.ID > 0 && s.openaiGatewayService != nil {
		minted, err := s.openaiGatewayService.MetaAPIKey(ctx, account)
		if err != nil {
			return nil, newUpstreamModelSyncConfigError("No Meta API key is available", err)
		}
		apiKey = strings.TrimSpace(minted)
	}
	if apiKey == "" {
		return nil, newUpstreamModelSyncConfigError("No Meta API key is available", nil)
	}

	baseURL, err := s.validateUpstreamBaseURL(account.GetMetaBaseURL())
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Meta base URL", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildV1ModelsURL(baseURL), nil)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Meta model list URL", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	meta.ApplyClientIdentity(req.Header, "")
	return req, nil
}
