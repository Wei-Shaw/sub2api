package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// FetchOpenAIModelsList discovers a single account's raw public model catalog.
// API keys use the standard endpoint; OAuth reuses the authenticated, cached
// Codex source. Account mappings and group policy are applied after this cache.
func (s *OpenAIGatewayService) FetchOpenAIModelsList(ctx context.Context, account *Account) (*OpenAIModelsResponse, error) {
	if s == nil || account == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_MODELS_ACCOUNT_REQUIRED", "OpenAI account is required")
	}
	credentialAccount, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		return nil, fmt.Errorf("resolve model list credentials: %w", err)
	}
	if credentialAccount.IsOpenAIOAuth() {
		clientVersion := CodexCanonicalClientVersion()
		if s.settingService != nil {
			clientVersion = s.settingService.GetOpenAICodexClientVersion(ctx)
		}
		response, err := s.FetchCodexModelsManifest(ctx, account, clientVersion, "")
		if err != nil {
			return nil, err
		}
		body, err := standardOpenAIModelsBody(response.Body, true)
		if err != nil {
			return nil, invalidOpenAIModelsList(err)
		}
		return &OpenAIModelsResponse{Body: body, ETag: codexModelsManifestBodyETag(body)}, nil
	}
	req, err := buildOpenAIAPIKeyModelsRequest(ctx, credentialAccount, s.validateUpstreamBaseURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_MODELS_REQUEST_INVALID", "cannot build upstream model list request: %v", err)
	}
	request := openAIModelsRequest{
		url: req.URL.String(), headers: req.Header,
		proxyURL: upstreamModelsProxyURL(account), accountID: account.ID,
		credentialAccountID: credentialAccount.ID, credentialAccount: credentialAccount,
		accountConcurrency: account.Concurrency, useAPIKeyUpstream: true,
		standardModelsList: true,
	}
	response, err := s.fetchCachedOpenAIModels(ctx, request, func(fetchCtx context.Context, etag string) (*OpenAIModelsResponse, error) {
		response, err := s.fetchOpenAIModelsUpstream(fetchCtx, request, etag)
		if err != nil || response.NotModified {
			return response, err
		}
		body, err := standardOpenAIModelsBody(response.Body, false)
		if err != nil {
			return nil, invalidOpenAIModelsList(err)
		}
		response.Body = body
		response.ETag = codexModelsManifestBodyETag(body)
		return response, nil
	}, "")
	if err != nil {
		return nil, err
	}
	if response.NotModified {
		return nil, invalidOpenAIModelsList(fmt.Errorf("upstream returned 304 without a cached catalog"))
	}
	return response, nil
}

func invalidOpenAIModelsList(err error) error {
	return &codexModelsManifestUpstreamError{
		err:       infraerrors.Newf(http.StatusBadGateway, "OPENAI_MODELS_UPSTREAM_INVALID", "invalid upstream model list: %v", err),
		retryable: true,
	}
}

// modelCatalogEntries requires a real array. An empty array is authoritative;
// absent fields, null and error envelopes must not become successful empty lists.
func modelCatalogEntries(body []byte, field string) (map[string]json.RawMessage, []json.RawMessage, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, nil, err
	}
	raw := bytes.TrimSpace(envelope[field])
	if len(raw) == 0 || raw[0] != '[' {
		return nil, nil, fmt.Errorf("missing or invalid %s array", field)
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, nil, err
	}
	return envelope, entries, nil
}

func standardOpenAIModelsBody(body []byte, fromManifest bool) ([]byte, error) {
	field, idField := "data", "id"
	if fromManifest {
		field, idField = "models", "slug"
	}
	_, entries, err := modelCatalogEntries(body, field)
	if err != nil {
		return nil, err
	}
	models := make([]json.RawMessage, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, raw := range entries {
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entry); err != nil || entry == nil {
			return nil, fmt.Errorf("model entry must be an object")
		}
		var id string
		if err := json.Unmarshal(entry[idField], &id); err != nil || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("model entry has no valid %s", idField)
		}
		id = strings.TrimSpace(id)
		if _, ok := seen[id]; ok || strings.Contains(id, "*") {
			continue
		}
		seen[id] = struct{}{}
		if fromManifest {
			entry = make(map[string]json.RawMessage)
		}
		entry["id"], _ = json.Marshal(id)
		entry["object"] = json.RawMessage(`"model"`)
		if len(entry["created"]) == 0 || string(entry["created"]) == "null" {
			entry["created"] = json.RawMessage(`0`)
		}
		if len(entry["owned_by"]) == 0 || string(entry["owned_by"]) == "null" {
			entry["owned_by"] = json.RawMessage(`"openai"`)
		}
		encoded, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		models = append(models, encoded)
	}
	return json.Marshal(struct {
		Object string            `json:"object"`
		Data   []json.RawMessage `json:"data"`
	}{Object: "list", Data: models})
}

// projectAccountModelsBody applies the same public-name mapping to both model
// representations while retaining the source entry's metadata. It never changes
// the shared response and never synthesizes models absent from this account.
func projectAccountModelsBody(body []byte, account *Account, group *Group, codex bool) ([]byte, error) {
	if account.IsOpenAIPassthroughEnabled() || len(account.GetModelMapping()) == 0 {
		return body, nil
	}
	field, idField := "data", "id"
	if codex {
		field, idField = "models", "slug"
	}
	envelope, entries, err := modelCatalogEntries(body, field)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]json.RawMessage, len(entries))
	candidates := make([]string, 0, len(entries))
	for _, raw := range entries {
		var entry map[string]json.RawMessage
		if json.Unmarshal(raw, &entry) != nil {
			continue
		}
		var id string
		if json.Unmarshal(entry[idField], &id) != nil {
			continue
		}
		id = strings.TrimSpace(id)
		if id == "" || strings.Contains(id, "*") {
			continue
		}
		// Apply the allowlist to public names after mapping, not upstream targets.
		if codex {
			if isCodexDedicatedMediaModel(id) {
				continue
			}
			if strings.HasPrefix(id, codexAutoModelPrefix) && len(FilterCodexModelIDsForGroup([]string{id}, group)) == 0 {
				continue
			}
		}
		if _, ok := byID[id]; !ok {
			byID[id] = raw
			candidates = append(candidates, id)
		}
	}
	aliases := make([]string, 0, len(account.GetModelMapping()))
	for alias := range account.GetModelMapping() {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	candidates = append(candidates, aliases...)
	if group.ModelAllowlistEnabled() {
		candidates = append(candidates, group.ModelAllowlist.Models...)
	}
	projected := make([]json.RawMessage, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, id := range candidates {
		id = strings.TrimSpace(id)
		if id == "" || strings.Contains(id, "*") {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		target, matched := account.ResolveMappedModel(id)
		raw, available := byID[strings.TrimSpace(target)]
		if !matched || !available {
			continue
		}
		seen[id] = struct{}{}
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, err
		}
		entry[idField], _ = json.Marshal(id)
		if id != target {
			entry["display_name"], _ = json.Marshal(id)
		}
		encoded, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		projected = append(projected, encoded)
	}
	envelope[field], err = json.Marshal(projected)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

// ApplyPinnedCodexModelsMapping is used by pinned discovery and its scheduler
// fallback. The ordinary (non-pinned) Codex path retains its local catalog policy.
func ApplyPinnedCodexModelsMapping(response *OpenAIModelsResponse, account *Account, group *Group) error {
	if group == nil || group.Platform != PlatformOpenAI || !group.CodexModelsManifestConfig.Enabled {
		return nil
	}
	body, err := projectAccountModelsBody(response.Body, account, group, true)
	if err != nil {
		return err
	}
	response.Body = body
	response.ETag = codexModelsManifestBodyETag(body)
	return nil
}

// FetchPinnedOpenAIModelsList includes explicitly enabled scheduler fallback.
// An authoritative empty catalog is success, including after group filtering.
func (s *OpenAIGatewayService) FetchPinnedOpenAIModelsList(ctx context.Context, group *Group, maxAccountSwitches int, ifNoneMatch string) (*OpenAIModelsResponse, *Account, error) {
	fetch := func(ctx context.Context, account *Account) (*OpenAIModelsResponse, error) {
		response, err := s.FetchOpenAIModelsList(ctx, account)
		if err != nil {
			return nil, err
		}
		response.Body, err = projectAccountModelsBody(response.Body, account, group, false)
		return response, err
	}
	results, err := s.fetchPinnedOpenAIModels(ctx, group, fetch)
	if err != nil && ctx.Err() == nil && group != nil && group.CodexModelsManifestConfig.FallbackToScheduler {
		results, err = s.fetchScheduledOpenAIModels(ctx, group, maxAccountSwitches, fetch)
	}
	if err != nil {
		return nil, nil, err
	}
	models := make([]json.RawMessage, 0)
	modelIDs := make([]string, 0)
	byID := make(map[string]json.RawMessage)
	for _, result := range results {
		_, entries, err := modelCatalogEntries(result.response.Body, "data")
		if err != nil {
			return nil, nil, err
		}
		for _, raw := range entries {
			var model struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(raw, &model); err != nil {
				return nil, nil, err
			}
			if _, exists := byID[model.ID]; !exists {
				byID[model.ID] = raw
				modelIDs = append(modelIDs, model.ID)
				models = append(models, raw)
			}
		}
	}
	if group.ModelAllowlistEnabled() {
		models = selectModelCatalogEntries(byID, group.ModelAllowlist.FilterForListing(modelIDs))
	}
	body, err := json.Marshal(struct {
		Object string            `json:"object"`
		Data   []json.RawMessage `json:"data"`
	}{Object: "list", Data: models})
	if err != nil {
		return nil, nil, err
	}
	response := &OpenAIModelsResponse{Body: body, ETag: codexModelsManifestBodyETag(body)}
	return openAIModelsResponseForClient(response, ifNoneMatch), results[0].account, nil
}

func selectModelCatalogEntries(byID map[string]json.RawMessage, selected []string) []json.RawMessage {
	models := make([]json.RawMessage, 0, len(selected))
	seen := make(map[string]struct{}, len(selected))
	for _, id := range selected {
		id = strings.TrimSpace(id)
		if _, ok := seen[id]; ok {
			continue
		}
		if raw, exists := byID[id]; exists {
			models = append(models, raw)
			seen[id] = struct{}{}
		}
	}
	return models
}

func orderPinnedCodexModelsBySelection(body []byte, allowlist GroupModelAllowlist) ([]byte, error) {
	envelope, entries, err := modelCatalogEntries(body, "models")
	if err != nil {
		return nil, err
	}
	byID := make(map[string]json.RawMessage, len(entries))
	modelIDs := make([]string, 0, len(entries))
	for _, raw := range entries {
		var entry struct {
			Slug string `json:"slug"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, err
		}
		byID[entry.Slug] = raw
		modelIDs = append(modelIDs, entry.Slug)
	}
	envelope["models"], err = json.Marshal(selectModelCatalogEntries(byID, allowlist.FilterForListing(modelIDs)))
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

func (s *OpenAIGatewayService) fetchScheduledOpenAIModels(ctx context.Context, group *Group, maxSwitches int, fetch func(context.Context, *Account) (*OpenAIModelsResponse, error)) ([]pinnedOpenAIModelsResult, error) {
	if maxSwitches <= 0 {
		maxSwitches = 3
	}
	excluded := make(map[int64]struct{})
	var lastErr error
	for attempt := 0; attempt <= maxSwitches; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		account, err := s.SelectAccountForModelWithExclusions(ctx, &group.ID, "", "", excluded)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, ErrNoPinnedCodexModelsAccounts
		}
		response, err := fetch(ctx, account)
		if err == nil {
			return []pinnedOpenAIModelsResult{{account: account, response: response}}, nil
		}
		lastErr = err
		if !IsRetryableCodexModelsManifestError(err) {
			return nil, err
		}
		excluded[account.ID] = struct{}{}
	}
	return nil, lastErr
}
