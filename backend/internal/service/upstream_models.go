package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
)

const (
	upstreamModelsBodyLimit             int64 = 8 << 20
	modelsDevRegistryURL                      = "https://models.dev/api.json"
	modelsDevRegistryTTL                      = 5 * time.Minute
	UpstreamModelMetadataExtraKey             = "upstream_model_metadata"
	UpstreamModelMetadataIncompleteCode       = "upstream_model_metadata_incomplete"
	UpstreamModelMetadataPartialCode          = "upstream_model_metadata_partial"
)

type UpstreamModelMetadata struct {
	ID                       string                     `json:"id"`
	DisplayName              string                     `json:"display_name,omitempty"`
	Description              string                     `json:"description,omitempty"`
	Reasoning                *bool                      `json:"reasoning,omitempty"`
	DefaultReasoningLevel    string                     `json:"default_reasoning_level,omitempty"`
	SupportedReasoningLevels []string                   `json:"supported_reasoning_levels,omitempty"`
	InputModalities          []string                   `json:"input_modalities,omitempty"`
	ContextWindow            int64                      `json:"context_window,omitempty"`
	MaxContextWindow         int64                      `json:"max_context_window,omitempty"`
	MaxOutputTokens          int64                      `json:"max_output_tokens,omitempty"`
	CodexToolCapabilities    map[string]json.RawMessage `json:"codex_tool_capabilities,omitempty"`
}

type UpstreamModelMetadataSnapshot struct {
	Source   string                           `json:"source"`
	SyncedAt string                           `json:"synced_at"`
	Models   map[string]UpstreamModelMetadata `json:"models"`
}

type UpstreamModelCatalog struct {
	Models   []string                         `json:"models"`
	Metadata map[string]UpstreamModelMetadata `json:"metadata,omitempty"`
	Warnings []UpstreamModelSyncWarning       `json:"warnings,omitempty"`
}

type UpstreamModelSyncWarning struct {
	Code          string   `json:"code"`
	Message       string   `json:"message"`
	ModelIDs      []string `json:"model_ids,omitempty"`
	CompleteCount int      `json:"complete_count"`
	TotalCount    int      `json:"total_count"`
	Reason        string   `json:"reason,omitempty"`
}

type modelsDevProvider struct {
	ID     string                    `json:"id"`
	Name   string                    `json:"name"`
	API    string                    `json:"api"`
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	ID               string                     `json:"id"`
	Name             string                     `json:"name"`
	Description      string                     `json:"description"`
	Reasoning        *bool                      `json:"reasoning"`
	ReasoningOptions []modelsDevReasoningOption `json:"reasoning_options"`
	Modalities       modelsDevModalities        `json:"modalities"`
	Limit            modelsDevLimit             `json:"limit"`
}

type modelsDevReasoningOption struct {
	Type   string `json:"type"`
	Values []any  `json:"values"`
}

type modelsDevModalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type modelsDevLimit struct {
	Context int64 `json:"context"`
	Output  int64 `json:"output"`
}

func (a *Account) SetUpstreamModelMetadataSnapshot(snapshot UpstreamModelMetadataSnapshot) {
	if a == nil {
		return
	}
	if a.Extra == nil {
		a.Extra = make(map[string]any)
	}
	a.Extra[UpstreamModelMetadataExtraKey] = snapshot
}

func (a *Account) GetUpstreamModelMetadataSnapshot() *UpstreamModelMetadataSnapshot {
	if a == nil || a.Extra == nil {
		return nil
	}
	raw, ok := a.Extra[UpstreamModelMetadataExtraKey]
	if !ok || raw == nil {
		return nil
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var snapshot UpstreamModelMetadataSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil || len(snapshot.Models) == 0 {
		return nil
	}
	return &snapshot
}

func (a *Account) GetUpstreamModelMetadata(modelID string) (UpstreamModelMetadata, bool) {
	snapshot := a.GetUpstreamModelMetadataSnapshot()
	if snapshot == nil {
		return UpstreamModelMetadata{}, false
	}
	metadata, ok := snapshot.Models[strings.TrimSpace(modelID)]
	return metadata, ok
}

// UpstreamModelSyncErrorKind classifies model sync failures for safe HTTP mapping.
type UpstreamModelSyncErrorKind string

const (
	// UpstreamModelSyncErrorConfiguration means the account or server configuration cannot perform the sync.
	UpstreamModelSyncErrorConfiguration UpstreamModelSyncErrorKind = "configuration"
	// UpstreamModelSyncErrorUnsupported means the account format is intentionally unsupported for live model sync.
	UpstreamModelSyncErrorUnsupported UpstreamModelSyncErrorKind = "unsupported"
	// UpstreamModelSyncErrorUpstream means the configured upstream failed or returned an unusable response.
	UpstreamModelSyncErrorUpstream UpstreamModelSyncErrorKind = "upstream"
	// UpstreamModelSyncErrorInternal means local persistence or service state failed after a valid upstream response.
	UpstreamModelSyncErrorInternal UpstreamModelSyncErrorKind = "internal"
)

// UpstreamModelSyncError keeps internal failure details wrapped while exposing a safe client message.
type UpstreamModelSyncError struct {
	Kind       UpstreamModelSyncErrorKind
	Message    string
	StatusCode int
	Err        error
}

func (e *UpstreamModelSyncError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Message
	}
	return e.Message + ": " + e.Err.Error()
}

func (e *UpstreamModelSyncError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// SafeMessage returns the sanitized message that can be sent to API clients.
func (e *UpstreamModelSyncError) SafeMessage() string {
	if e == nil || strings.TrimSpace(e.Message) == "" {
		return "Failed to sync upstream models"
	}
	return e.Message
}

func newUpstreamModelSyncConfigError(message string, err error) error {
	return &UpstreamModelSyncError{Kind: UpstreamModelSyncErrorConfiguration, Message: message, Err: err}
}

func newUpstreamModelSyncUnsupportedError(message string, err error) error {
	return &UpstreamModelSyncError{Kind: UpstreamModelSyncErrorUnsupported, Message: message, Err: err}
}

func newUpstreamModelSyncUpstreamError(message string, err error) error {
	return &UpstreamModelSyncError{Kind: UpstreamModelSyncErrorUpstream, Message: message, Err: err}
}

func newUpstreamModelSyncInternalError(message string, err error) error {
	return &UpstreamModelSyncError{Kind: UpstreamModelSyncErrorInternal, Message: message, Err: err}
}

// FetchUpstreamSupportedModels fetches only live model IDs. The admin sync path
// uses SyncUpstreamModelCatalog so capability metadata can also be persisted.
func (s *AccountTestService) FetchUpstreamSupportedModels(ctx context.Context, account *Account) ([]string, error) {
	models, _, err := s.fetchUpstreamModelList(ctx, account)
	return models, err
}

// SyncUpstreamModelCatalog fetches the account's live model list, enriches
// missing capability fields from the provider registry used by the upstream,
// and persists a normalized account snapshot when complete metadata is available.
//
// Persistence is per-model: models with complete capability fields are saved even
// when other IDs in the same sync remain incomplete. An incomplete warning is
// still returned so admins can tell ID sync succeeded without a full capability
// snapshot. When no model is complete, the existing account snapshot is left
// untouched.
func (s *AccountTestService) SyncUpstreamModelCatalog(ctx context.Context, account *Account) (*UpstreamModelCatalog, error) {
	models, body, err := s.fetchUpstreamModelList(ctx, account)
	liveListAvailable := err == nil
	if err != nil {
		configuredModels := configuredUpstreamModelsForCapabilitySync(account)
		if !upstreamModelListEndpointUnsupported(err) || len(configuredModels) == 0 {
			return nil, err
		}
		models = configuredModels
		body = nil
		slog.Info("upstream model list endpoint unavailable; using configured models for capability sync",
			"account_id", upstreamModelSyncAccountID(account),
			"platform", upstreamModelSyncPlatform(account),
			"status_code", upstreamModelSyncStatusCode(err),
			"model_count", len(models),
		)
	}
	catalog := &UpstreamModelCatalog{Models: models, Metadata: make(map[string]UpstreamModelMetadata)}
	if len(body) > 0 {
		_, directMetadata, parseErr := extractUpstreamModelCatalog(body, account != nil && account.IsGrok())
		if parseErr == nil {
			catalog.Metadata = directMetadata
		}
	}

	// Capability enrichment also covers concrete model_mapping targets. Admins may
	// whitelist models that the live /models list omitted; those still need registry
	// metadata so Codex catalogs can advertise reasoning and modalities.
	enrichIDs := dedupeAndSortModelIDs(append(append([]string{}, models...), configuredUpstreamModelsForCapabilitySync(account)...))
	// Dedicated image/video generators are not Codex agent catalog entries and often
	// omit context windows in public registries. Keep them out of completeness checks
	// so they do not mask successful agent-model capability sync.
	capabilityIDs := capabilitySyncModelIDs(enrichIDs)

	source := "upstream"
	metadataReason := "metadata_missing"
	if upstreamCatalogNeedsRegistry(capabilityIDs, catalog.Metadata) {
		if registryMetadata, registryErr := s.fetchModelsDevMetadata(ctx, account, enrichIDs); registryErr == nil {
			for modelID, fallback := range registryMetadata {
				current := catalog.Metadata[modelID]
				merged, changed := mergeUpstreamModelMetadata(current, fallback)
				catalog.Metadata[modelID] = merged
				if changed {
					source = "models.dev"
				}
			}
		} else {
			metadataReason = "registry_unavailable"
			slog.Warn("upstream model capability metadata enrichment failed",
				"account_id", upstreamModelSyncAccountID(account),
				"platform", upstreamModelSyncPlatform(account),
				"error", registryErr,
			)
		}
	}

	completeMetadata := completeUpstreamModelMetadataSubset(capabilityIDs, catalog.Metadata)
	if len(completeMetadata) > 0 && account != nil && account.ID > 0 && s.accountRepo != nil {
		// Retain known metadata only for models still listed or explicitly mapped.
		if previous := account.GetUpstreamModelMetadataSnapshot(); previous != nil {
			retainedModels := capabilityIDs
			if !liveListAvailable {
				retainedModels = append([]string(nil), capabilityIDs...)
				for modelID := range previous.Models {
					retainedModels = append(retainedModels, modelID)
				}
			}
			for _, modelID := range retainedModels {
				old, exists := previous.Models[modelID]
				if !exists {
					continue
				}
				if entry, ok := completeMetadata[modelID]; ok {
					if entry.CodexToolCapabilities == nil {
						entry.CodexToolCapabilities = make(map[string]json.RawMessage)
					}
					applyCodexToolCapabilities(entry.CodexToolCapabilities, old.CodexToolCapabilities, false)
					completeMetadata[modelID] = entry
				} else {
					completeMetadata[modelID] = old
				}
			}
		}
		snapshot := UpstreamModelMetadataSnapshot{
			Source:   source,
			SyncedAt: time.Now().UTC().Format(time.RFC3339),
			Models:   completeMetadata,
		}
		if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{UpstreamModelMetadataExtraKey: snapshot}); err != nil {
			return nil, newUpstreamModelSyncInternalError("Failed to save upstream model metadata", err)
		}
		account.SetUpstreamModelMetadataSnapshot(snapshot)
	}

	if upstreamCatalogNeedsRegistry(capabilityIDs, catalog.Metadata) {
		missing := make([]string, 0)
		for _, modelID := range capabilityIDs {
			if !upstreamModelMetadataIsComplete(catalog.Metadata[modelID]) {
				missing = append(missing, modelID)
			}
		}
		// Preview has no account ID and intentionally does not persist. Classify
		// the result by capabilities obtained, not by whether a DB write occurred.
		if len(missing) < len(capabilityIDs) {
			catalog.Warnings = append(catalog.Warnings, UpstreamModelSyncWarning{
				Code:     UpstreamModelMetadataPartialCode,
				Message:  "Some model capabilities were obtained; remaining models are still incomplete.",
				ModelIDs: missing, CompleteCount: len(capabilityIDs) - len(missing), TotalCount: len(capabilityIDs), Reason: metadataReason,
			})
		} else {
			catalog.Warnings = append(catalog.Warnings, UpstreamModelSyncWarning{
				Code:     UpstreamModelMetadataIncompleteCode,
				Message:  "Model IDs were synced, but capability metadata is incomplete.",
				ModelIDs: missing, CompleteCount: 0, TotalCount: len(capabilityIDs), Reason: metadataReason,
			})
		}
	}
	return catalog, nil
}

func upstreamModelSyncStatusCode(err error) int {
	var syncErr *UpstreamModelSyncError
	if errors.As(err, &syncErr) {
		return syncErr.StatusCode
	}
	return 0
}

func upstreamModelListEndpointUnsupported(err error) bool {
	statusCode := upstreamModelSyncStatusCode(err)
	return statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed
}

func configuredUpstreamModelsForCapabilitySync(account *Account) []string {
	if account == nil {
		return nil
	}
	models := make([]string, 0)
	for _, mappedModel := range account.GetModelMapping() {
		mappedModel = strings.TrimSpace(mappedModel)
		if mappedModel == "" || strings.Contains(mappedModel, "*") {
			continue
		}
		models = append(models, mappedModel)
	}
	return dedupeAndSortModelIDs(models)
}

func capabilitySyncModelIDs(modelIDs []string) []string {
	filtered := make([]string, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" || isCodexDedicatedMediaModel(modelID) {
			continue
		}
		filtered = append(filtered, modelID)
	}
	return filtered
}

func upstreamModelSyncAccountID(account *Account) int64 {
	if account == nil {
		return 0
	}
	return account.ID
}

func upstreamModelSyncPlatform(account *Account) string {
	if account == nil {
		return ""
	}
	return account.Platform
}

func upstreamCatalogNeedsRegistry(models []string, metadata map[string]UpstreamModelMetadata) bool {
	for _, modelID := range models {
		modelID = strings.TrimSpace(modelID)
		model, ok := metadata[modelID]
		if !ok || !upstreamModelMetadataIsComplete(model) {
			return true
		}
	}
	return false
}

func upstreamModelMetadataIsUseful(metadata UpstreamModelMetadata) bool {
	return strings.TrimSpace(metadata.DisplayName) != "" ||
		strings.TrimSpace(metadata.Description) != "" ||
		metadata.Reasoning != nil ||
		len(metadata.SupportedReasoningLevels) > 0 ||
		len(metadata.InputModalities) > 0 ||
		len(metadata.CodexToolCapabilities) > 0 ||
		metadata.ContextWindow > 0 ||
		metadata.MaxContextWindow > 0 ||
		metadata.MaxOutputTokens > 0
}

// upstreamModelMetadataIsComplete reports whether a snapshot entry is safe to
// persist and later prefer over local Codex name-based fallbacks.
func upstreamModelMetadataIsComplete(metadata UpstreamModelMetadata) bool {
	if metadata.Reasoning == nil {
		return false
	}
	if len(normalizeCodexInputModalities(metadata.InputModalities)) == 0 {
		return false
	}
	if metadata.ContextWindow <= 0 {
		return false
	}
	if *metadata.Reasoning && len(normalizeReasoningLevels(metadata.SupportedReasoningLevels)) == 0 {
		return false
	}
	return true
}

func completeUpstreamModelMetadataSubset(
	modelIDs []string,
	metadata map[string]UpstreamModelMetadata,
) map[string]UpstreamModelMetadata {
	if len(metadata) == 0 {
		return nil
	}
	complete := make(map[string]UpstreamModelMetadata)
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		entry, ok := metadata[modelID]
		if !ok || !upstreamModelMetadataIsComplete(entry) {
			continue
		}
		if strings.TrimSpace(entry.ID) == "" {
			entry.ID = modelID
		}
		complete[modelID] = entry
	}
	if len(complete) == 0 {
		return nil
	}
	return complete
}

func mergeUpstreamModelMetadata(primary, fallback UpstreamModelMetadata) (UpstreamModelMetadata, bool) {
	merged := primary
	changed := false
	if strings.TrimSpace(merged.ID) == "" && strings.TrimSpace(fallback.ID) != "" {
		merged.ID = strings.TrimSpace(fallback.ID)
		changed = true
	}
	if strings.TrimSpace(merged.DisplayName) == "" && strings.TrimSpace(fallback.DisplayName) != "" {
		merged.DisplayName = strings.TrimSpace(fallback.DisplayName)
		changed = true
	}
	if strings.TrimSpace(merged.Description) == "" && strings.TrimSpace(fallback.Description) != "" {
		merged.Description = strings.TrimSpace(fallback.Description)
		changed = true
	}
	if merged.Reasoning == nil && fallback.Reasoning != nil {
		reasoning := *fallback.Reasoning
		merged.Reasoning = &reasoning
		changed = true
	}
	if strings.TrimSpace(merged.DefaultReasoningLevel) == "" && strings.TrimSpace(fallback.DefaultReasoningLevel) != "" {
		merged.DefaultReasoningLevel = strings.TrimSpace(fallback.DefaultReasoningLevel)
		changed = true
	}
	if len(merged.SupportedReasoningLevels) == 0 && len(fallback.SupportedReasoningLevels) > 0 {
		merged.SupportedReasoningLevels = append([]string(nil), fallback.SupportedReasoningLevels...)
		changed = true
	}
	if len(merged.InputModalities) == 0 && len(fallback.InputModalities) > 0 {
		merged.InputModalities = append([]string(nil), fallback.InputModalities...)
		changed = true
	}
	if merged.ContextWindow <= 0 && fallback.ContextWindow > 0 {
		merged.ContextWindow = fallback.ContextWindow
		// Keep the registry's context limits together. A direct upstream default
		// without an explicit maximum remains the conservative ceiling.
		if merged.MaxContextWindow <= 0 {
			merged.MaxContextWindow = fallback.MaxContextWindow
		}
		changed = true
	}
	if merged.MaxOutputTokens <= 0 && fallback.MaxOutputTokens > 0 {
		merged.MaxOutputTokens = fallback.MaxOutputTokens
		changed = true
	}
	return merged, changed
}

func (s *AccountTestService) fetchModelsDevMetadata(
	ctx context.Context,
	account *Account,
	modelIDs []string,
) (map[string]UpstreamModelMetadata, error) {
	if s == nil || s.httpUpstream == nil || account == nil {
		return nil, fmt.Errorf("model metadata registry is not configured")
	}
	registry, err := s.fetchModelsDevRegistry(ctx, account)
	if err != nil {
		return nil, err
	}
	preferredProvider, providerMatched := matchModelsDevProvider(registry, upstreamModelRegistryBaseURL(account))
	// Third-party relay hosts are not registry providers. Resolve known model
	// families independently of the wire protocol, never arbitrary private IDs.
	allowCrossProvider := account.IsOpenAICompatible() || account.IsAnthropic() || account.IsGemini() || account.Platform == PlatformAntigravity
	if !providerMatched && !allowCrossProvider {
		return nil, fmt.Errorf("no model metadata provider matches account base URL")
	}

	metadata := make(map[string]UpstreamModelMetadata)
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		model, found := lookupModelsDevModelForAccount(registry, preferredProvider, modelID, allowCrossProvider)
		if !found {
			continue
		}
		entry := upstreamMetadataFromModelsDevModel(modelID, model)
		if upstreamModelMetadataIsUseful(entry) {
			metadata[modelID] = entry
		}
	}
	return metadata, nil
}

func lookupModelsDevModel(models map[string]modelsDevModel, modelID string) (modelsDevModel, bool) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return modelsDevModel{}, false
	}

	candidates := modelRegistryLookupCandidates(modelID)
	if normalized := normalizeKnownOpenAICodexModel(modelID); normalized != "" && normalized != modelID {
		candidates = appendUniqueModelRegistryCandidates(candidates, normalized)
	}
	// This is a local Codex probe/guardian alias, not a separate upstream
	// capability family. Use the current Codex text-model profile for it.
	if strings.EqualFold(modelID, "codex-auto-review") {
		candidates = appendUniqueModelRegistryCandidates(candidates, "gpt-5.5")
	}

	keys := make([]string, 0, len(models))
	for registryID := range models {
		keys = append(keys, registryID)
	}
	sort.Strings(keys)
	for _, candidateID := range candidates {
		candidateKey := normalizeModelRegistryLookupID(candidateID)
		for _, registryID := range keys {
			candidate := models[registryID]
			if strings.EqualFold(strings.TrimSpace(registryID), candidateID) ||
				strings.EqualFold(strings.TrimSpace(candidate.ID), candidateID) ||
				(candidateKey != "" && (normalizeModelRegistryLookupID(registryID) == candidateKey ||
					normalizeModelRegistryLookupID(candidate.ID) == candidateKey)) {
				return candidate, true
			}
		}
	}
	return modelsDevModel{}, false
}

func lookupModelsDevModelForAccount(
	registry map[string]modelsDevProvider,
	preferred modelsDevProvider,
	modelID string,
	allowCrossProvider bool,
) (modelsDevModel, bool) {
	if model, ok := lookupModelsDevModel(preferred.Models, modelID); ok {
		return model, true
	}
	if !allowCrossProvider {
		return modelsDevModel{}, false
	}

	family := knownModelsDevModelFamily(modelID)
	if family == "" {
		return modelsDevModel{}, false
	}
	for _, providerID := range modelsDevProviderIDsForFamily(family) {
		provider, ok := registry[providerID]
		if !ok || len(provider.Models) == 0 {
			continue
		}
		if model, found := lookupModelsDevModel(provider.Models, modelID); found {
			return model, true
		}
	}
	return modelsDevModel{}, false
}

func modelRegistryLookupCandidates(modelID string) []string {
	candidates := make([]string, 0, 4)
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		for _, existing := range candidates {
			if strings.EqualFold(existing, candidate) {
				return
			}
		}
		candidates = append(candidates, candidate)
	}

	add(modelID)
	add(strings.TrimPrefix(strings.TrimSpace(modelID), "models/"))
	if slash := strings.LastIndexByte(modelID, '/'); slash >= 0 {
		add(modelID[slash+1:])
	}
	for _, candidate := range append([]string(nil), candidates...) {
		add(trimModelRegistryDateSuffix(candidate))
	}
	return candidates
}

func appendUniqueModelRegistryCandidates(candidates []string, values ...string) []string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		found := false
		for _, existing := range candidates {
			if strings.EqualFold(existing, value) {
				found = true
				break
			}
		}
		if !found {
			candidates = append(candidates, value)
		}
	}
	return candidates
}

func normalizeModelRegistryLookupID(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if slash := strings.LastIndexByte(modelID, '/'); slash >= 0 {
		modelID = modelID[slash+1:]
	}
	modelID = strings.TrimPrefix(modelID, "models/")
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	modelID = strings.ReplaceAll(modelID, "_", "-")
	modelID = strings.Join(strings.Fields(modelID), "-")
	for strings.Contains(modelID, "--") {
		modelID = strings.ReplaceAll(modelID, "--", "-")
	}
	return modelID
}

func trimModelRegistryDateSuffix(modelID string) string {
	modelID = normalizeModelRegistryLookupID(modelID)
	if modelID == "" {
		return ""
	}
	parts := strings.Split(modelID, "-")
	if len(parts) > 1 {
		last := parts[len(parts)-1]
		if (len(last) == 8 && strings.HasPrefix(last, "20") && allASCIIDigits(last)) ||
			(len(last) == 4 && validMonthDaySuffix(last)) {
			return strings.Join(parts[:len(parts)-1], "-")
		}
		if len(parts) >= 4 && len(parts[len(parts)-3]) == 4 && strings.HasPrefix(parts[len(parts)-3], "20") &&
			len(parts[len(parts)-2]) == 2 && len(parts[len(parts)-1]) == 2 &&
			allASCIIDigits(parts[len(parts)-3]) && allASCIIDigits(parts[len(parts)-2]) && allASCIIDigits(parts[len(parts)-1]) {
			return strings.Join(parts[:len(parts)-3], "-")
		}
	}
	return modelID
}

func allASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func validMonthDaySuffix(value string) bool {
	if len(value) != 4 || !allASCIIDigits(value) {
		return false
	}
	month := int(value[0]-'0')*10 + int(value[1]-'0')
	day := int(value[2]-'0')*10 + int(value[3]-'0')
	return month >= 1 && month <= 12 && day >= 1 && day <= 31
}

func knownModelsDevModelFamily(modelID string) string {
	modelID = trimModelRegistryDateSuffix(modelID)
	switch {
	case strings.HasPrefix(modelID, "claude-"):
		return "anthropic"
	case strings.HasPrefix(modelID, "gemini-"):
		return "google"
	case strings.HasPrefix(modelID, "gpt-") || strings.HasPrefix(modelID, "o1") ||
		strings.HasPrefix(modelID, "o3") || strings.HasPrefix(modelID, "o4") || strings.Contains(modelID, "codex"):
		return "openai"
	case strings.HasPrefix(modelID, "deepseek"):
		return "deepseek"
	case strings.HasPrefix(modelID, "glm-") || strings.HasPrefix(modelID, "chatglm"):
		return "zhipu"
	case strings.HasPrefix(modelID, "kimi-") || strings.HasPrefix(modelID, "moonshot"):
		return "kimi"
	case strings.HasPrefix(modelID, "qwen") || strings.HasPrefix(modelID, "qwq") ||
		strings.HasPrefix(modelID, "qvq") || strings.HasPrefix(modelID, "tongyi"):
		return "alibaba"
	case strings.HasPrefix(modelID, "mimo"):
		return "xiaomi"
	case strings.HasPrefix(modelID, "minimax"):
		return "minimax"
	case strings.HasPrefix(modelID, "hy") || strings.HasPrefix(modelID, "hunyuan"):
		return "tencent"
	case strings.HasPrefix(modelID, "grok"):
		return "xai"
	default:
		return ""
	}
}

func modelsDevProviderIDsForFamily(family string) []string {
	switch family {
	case "anthropic":
		return []string{"anthropic"}
	case "google":
		return []string{"google", "google-vertex"}
	case "openai":
		return []string{"openai"}
	case "deepseek":
		return []string{"deepseek", "alibaba-cn", "alibaba"}
	case "zhipu":
		return []string{"zhipuai", "zhipuai-coding-plan", "alibaba-cn", "alibaba", "tencent-coding-plan"}
	case "kimi":
		return []string{"moonshotai-cn", "moonshotai", "kimi-for-coding", "alibaba-cn", "tencent-coding-plan"}
	case "alibaba":
		return []string{"alibaba-cn", "alibaba"}
	case "xiaomi":
		return []string{"xiaomi", "xiaomi-token-plan-cn", "xiaomi-token-plan-sgp", "xiaomi-token-plan-ams"}
	case "minimax":
		return []string{"minimax-cn", "minimax", "minimax-cn-coding-plan", "minimax-coding-plan", "alibaba-cn", "tencent-coding-plan"}
	case "tencent":
		return []string{"tencent-token-plan", "tencent-coding-plan"}
	case "xai":
		return []string{"xai"}
	default:
		return nil
	}
}

func (s *AccountTestService) fetchModelsDevRegistry(ctx context.Context, account *Account) (map[string]modelsDevProvider, error) {
	now := time.Now()
	s.modelMetadataRegistryMu.Lock()
	if len(s.modelMetadataRegistry) > 0 && now.Sub(s.modelMetadataRegistryAt) < modelsDevRegistryTTL {
		cached := s.modelMetadataRegistry
		s.modelMetadataRegistryMu.Unlock()
		return cached, nil
	}
	s.modelMetadataRegistryMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsDevRegistryURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.doUpstreamModelsRequest(req, upstreamModelsProxyURL(account), account)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("model metadata registry returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, upstreamModelsBodyLimit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > upstreamModelsBodyLimit {
		return nil, fmt.Errorf("model metadata registry response exceeds %d bytes", upstreamModelsBodyLimit)
	}
	var registry map[string]modelsDevProvider
	if err := json.Unmarshal(body, &registry); err != nil {
		return nil, fmt.Errorf("parse model metadata registry: %w", err)
	}
	if len(registry) == 0 {
		return nil, fmt.Errorf("model metadata registry is empty")
	}

	s.modelMetadataRegistryMu.Lock()
	s.modelMetadataRegistry = registry
	s.modelMetadataRegistryAt = now
	s.modelMetadataRegistryMu.Unlock()
	return registry, nil
}

func upstreamMetadataFromModelsDevModel(modelID string, model modelsDevModel) UpstreamModelMetadata {
	levels := reasoningLevelsFromModelsDevOptions(model.Reasoning, model.ReasoningOptions)
	reasoning := model.Reasoning
	if reasoning == nil && len(levels) > 0 {
		inferred := true
		reasoning = &inferred
	}
	metadata := UpstreamModelMetadata{
		ID:                       strings.TrimSpace(modelID),
		DisplayName:              strings.TrimSpace(model.Name),
		Description:              strings.TrimSpace(model.Description),
		Reasoning:                reasoning,
		SupportedReasoningLevels: levels,
		InputModalities:          normalizeCodexInputModalities(model.Modalities.Input),
		ContextWindow:            model.Limit.Context,
		MaxContextWindow:         model.Limit.Context,
		MaxOutputTokens:          model.Limit.Output,
	}
	if len(levels) > 0 {
		metadata.DefaultReasoningLevel = defaultModelsDevReasoningLevel(model.ReasoningOptions, levels)
	}
	if strings.TrimSpace(model.ID) != "" {
		metadata.ID = strings.TrimSpace(model.ID)
	}
	return metadata
}

func reasoningLevelsFromModelsDevOptions(reasoning *bool, options []modelsDevReasoningOption) []string {
	levels := make([]string, 0)
	hasToggle := false
	for _, option := range options {
		switch strings.ToLower(strings.TrimSpace(option.Type)) {
		case "toggle":
			hasToggle = true
		case "effort":
			for _, value := range option.Values {
				if value == nil {
					levels = append(levels, "none")
					continue
				}
				if effort, ok := value.(string); ok {
					levels = append(levels, effort)
				}
			}
		}
	}
	if hasToggle {
		levels = append([]string{"none", "high"}, levels...)
	}
	// Some providers publish reasoning=true without enumerating an option. Keep
	// that known capability usable as a conservative binary toggle.
	if len(levels) == 0 && reasoning != nil && *reasoning {
		levels = []string{"none", "high"}
	}
	return normalizeReasoningLevels(levels)
}

func defaultModelsDevReasoningLevel(options []modelsDevReasoningOption, levels []string) string {
	for _, option := range options {
		if strings.EqualFold(strings.TrimSpace(option.Type), "toggle") && stringSliceContains(levels, "high") {
			return "high"
		}
	}
	if len(levels) == 0 {
		return ""
	}
	return levels[0]
}

func upstreamModelRegistryBaseURL(account *Account) string {
	if account == nil {
		return ""
	}
	switch {
	case account.IsOpenAI() || account.IsMultiProtocolAPIKey():
		return account.GetOpenAIFormatBaseURL()
	case account.IsGrok():
		return account.GetGrokBaseURL()
	case account.IsGemini():
		return account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	case account.IsAnthropic():
		return account.GetBaseURL()
	case account.Platform == PlatformAntigravity:
		return account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	default:
		return strings.TrimSpace(account.GetCredential("base_url"))
	}
}

func matchModelsDevProvider(registry map[string]modelsDevProvider, accountBaseURL string) (modelsDevProvider, bool) {
	if provider, ok := matchModelsDevProviderByAPIURL(registry, accountBaseURL); ok {
		return provider, true
	}
	return matchModelsDevProviderByKnownHost(registry, accountBaseURL)
}

func matchModelsDevProviderByAPIURL(registry map[string]modelsDevProvider, accountBaseURL string) (modelsDevProvider, bool) {
	accountBaseURL = normalizeModelRegistryBaseURL(accountBaseURL)
	if accountBaseURL == "" {
		return modelsDevProvider{}, false
	}
	var best modelsDevProvider
	bestScore := -1
	for _, provider := range registry {
		providerBaseURL := normalizeModelRegistryBaseURL(provider.API)
		if providerBaseURL == "" {
			continue
		}
		if accountBaseURL != providerBaseURL &&
			!strings.HasPrefix(accountBaseURL, providerBaseURL+"/") &&
			!strings.HasPrefix(providerBaseURL, accountBaseURL+"/") {
			continue
		}
		if len(providerBaseURL) > bestScore {
			best = provider
			bestScore = len(providerBaseURL)
		}
	}
	return best, bestScore >= 0
}

// matchModelsDevProviderByKnownHost covers first-party hosts whose models.dev
// entries omit the `api` field (notably the official OpenAI provider). Custom
// compatible gateways must still match by API URL so same-named models are not
// cross-attributed across vendors.
func matchModelsDevProviderByKnownHost(registry map[string]modelsDevProvider, accountBaseURL string) (modelsDevProvider, bool) {
	host := modelRegistryHostname(accountBaseURL)
	if host == "" {
		return modelsDevProvider{}, false
	}
	providerID := ""
	switch host {
	case "api.openai.com", "chatgpt.com":
		providerID = "openai"
	case "opencode.ai":
		providerID = "opencode-go"
	default:
		return modelsDevProvider{}, false
	}
	provider, ok := registry[providerID]
	if !ok || len(provider.Models) == 0 {
		return modelsDevProvider{}, false
	}
	if strings.TrimSpace(provider.ID) == "" {
		provider.ID = providerID
	}
	return provider, true
}

func modelRegistryHostname(raw string) string {
	normalized := normalizeModelRegistryBaseURL(raw)
	if normalized == "" {
		return ""
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func normalizeModelRegistryBaseURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(strings.ToLower(path), "/models") {
		path = strings.TrimRight(path[:len(path)-len("/models")], "/")
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host) + path
}

func (s *AccountTestService) fetchUpstreamModelList(ctx context.Context, account *Account) ([]string, []byte, error) {
	if s == nil {
		return nil, nil, newUpstreamModelSyncConfigError("Account test service is not configured", nil)
	}
	if account == nil {
		return nil, nil, newUpstreamModelSyncConfigError("Account is required", nil)
	}

	if account.Platform == PlatformAntigravity && account.Type != AccountTypeAPIKey {
		models, err := s.fetchAntigravityOAuthUpstreamModels(ctx, account)
		return models, nil, err
	}

	if s.httpUpstream == nil {
		return nil, nil, newUpstreamModelSyncConfigError("Upstream HTTP client is not configured", nil)
	}

	req, err := s.buildUpstreamModelsRequest(ctx, account)
	if err != nil {
		return nil, nil, err
	}

	proxyURL := upstreamModelsProxyURL(account)
	resp, err := s.doUpstreamModelsRequest(req, proxyURL, account)
	if err != nil {
		return nil, nil, newUpstreamModelSyncUpstreamError("Failed to request upstream model list", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyLimit := resolveModelsListReadLimit(s.cfg)
	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
	if err != nil {
		return nil, nil, newUpstreamModelSyncUpstreamError("Failed to read upstream model list", err)
	}
	if int64(len(body)) > bodyLimit {
		return nil, nil, newUpstreamModelSyncUpstreamError("Upstream model list response is too large", fmt.Errorf("response exceeds %d bytes", bodyLimit))
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, nil, &UpstreamModelSyncError{
			Kind:       UpstreamModelSyncErrorUpstream,
			Message:    fmt.Sprintf("Upstream model list request failed with HTTP %d", resp.StatusCode),
			StatusCode: resp.StatusCode,
			Err:        fmt.Errorf("upstream model list returned HTTP %d", resp.StatusCode),
		}
	}

	extractModels := extractUpstreamModelIDs
	if account.IsGrok() {
		extractModels = extractGrokUpstreamModelIDs
	}
	models, err := extractModels(body)
	if err != nil {
		return nil, nil, newUpstreamModelSyncUpstreamError("Upstream model list response was not valid JSON", err)
	}
	if len(models) == 0 {
		return nil, nil, newUpstreamModelSyncUpstreamError("Upstream returned no supported models", nil)
	}

	return models, body, nil
}

func (s *AccountTestService) buildUpstreamModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	switch {
	case account.Platform == PlatformAntigravity:
		return s.buildAntigravityAPIKeyModelsRequest(ctx, account)
	case account.IsGrok():
		return s.buildGrokUpstreamModelsRequest(ctx, account)
	case account.IsOpenAI() || account.IsMultiProtocolAPIKey():
		// 多协议 API Key 供应商（国产厂商与聚合平台）复用 OpenAI /v1/models 探测。
		return s.buildOpenAIUpstreamModelsRequest(ctx, account)
	case account.IsGemini():
		return s.buildGeminiUpstreamModelsRequest(ctx, account)
	case account.IsAnthropic():
		return s.buildAnthropicUpstreamModelsRequest(ctx, account)
	default:
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported platform for upstream model sync: %s", account.Platform), nil,
		)
	}
}

func (s *AccountTestService) buildGrokUpstreamModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	if account == nil {
		return nil, newUpstreamModelSyncConfigError("Account is required", nil)
	}

	var (
		authToken         string
		normalizedBaseURL string
		isOAuth           = account.IsGrokOAuth()
	)
	switch account.Type {
	case AccountTypeAPIKey:
		authToken = strings.TrimSpace(account.GetCredential("api_key"))
		if authToken == "" {
			return nil, newUpstreamModelSyncConfigError("No Grok API key is available", nil)
		}

		baseURL := strings.TrimSpace(account.GetCredential("base_url"))
		if baseURL == "" {
			baseURL = "https://api.x.ai"
		}
		validatedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, newUpstreamModelSyncConfigError("Invalid Grok base URL", err)
		}
		normalizedBaseURL = validatedBaseURL
	case AccountTypeOAuth:
		if s.grokTokenProvider == nil {
			return nil, newUpstreamModelSyncConfigError("Grok token provider is not configured", nil)
		}
		accessToken, err := s.grokTokenProvider.GetAccessTokenForManualTest(ctx, account)
		if err != nil {
			return nil, newUpstreamModelSyncUpstreamError("Failed to get Grok access token", err)
		}
		authToken = strings.TrimSpace(accessToken)
		if authToken == "" {
			return nil, newUpstreamModelSyncConfigError("No Grok access token is available", nil)
		}

		validator, err := grokBaseURLValidator(account, s.cfg)
		if err != nil {
			return nil, newUpstreamModelSyncConfigError("Invalid Grok base URL", err)
		}
		baseURL := account.GetGrokBaseURL()
		if s.settingService != nil {
			baseURL = s.settingService.ResolveGrokBaseURL(ctx, account)
		}
		validatedBaseURL, err := validator(baseURL)
		if err != nil {
			return nil, newUpstreamModelSyncConfigError("Invalid Grok base URL", err)
		}
		normalizedBaseURL = validatedBaseURL
	default:
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported Grok account type for upstream model sync: %s", account.Type), nil,
		)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildOpenAIModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Grok model list URL", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)
	if isOAuth {
		// The shared HTTP transport adds the official CLI marker/version for the
		// exact proxy host. Keep the request builder aligned with the other Grok
		// probes and only forward account identity headers to that trusted host.
		applyGrokCLIHeaders(req.Header)
		if isGrokCLIProxyTarget(req.URL.String()) {
			if userID := strings.TrimSpace(account.GetCredential("sub")); userID != "" {
				req.Header.Set("X-UserID", userID)
			}
			if email := strings.TrimSpace(account.GetCredential("email")); email != "" {
				req.Header.Set("X-Email", email)
			}
		}
	}
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

func (s *AccountTestService) buildAnthropicUpstreamModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	if account.IsBedrock() || account.Type == AccountTypeServiceAccount {
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported Anthropic account type for upstream model sync: %s", account.Type), nil,
		)
	}

	baseURL := "https://api.anthropic.com"
	authHeaderName := ""
	authHeaderValue := ""
	apiKeyAuthToken := ""
	betaHeader := ""

	if account.IsOAuth() {
		accessToken := strings.TrimSpace(account.GetCredential("access_token"))
		if accessToken == "" && s.claudeTokenProvider != nil {
			token, tokenErr := s.claudeTokenProvider.GetAccessToken(ctx, account)
			if tokenErr != nil {
				return nil, newUpstreamModelSyncUpstreamError("Failed to get Anthropic access token", tokenErr)
			}
			accessToken = strings.TrimSpace(token)
		}
		if accessToken == "" {
			return nil, newUpstreamModelSyncConfigError("No Anthropic access token is available", nil)
		}
		authHeaderName = "Authorization"
		authHeaderValue = "Bearer " + accessToken
		betaHeader = claude.DefaultBetaHeader
	} else if account.Type == AccountTypeAPIKey {
		apiKey := strings.TrimSpace(account.GetCredential("api_key"))
		if apiKey == "" {
			return nil, newUpstreamModelSyncConfigError("No Anthropic API key is available", nil)
		}
		baseURL = account.GetBaseURL()
		if strings.TrimSpace(baseURL) == "" {
			baseURL = "https://api.anthropic.com"
		}
		apiKeyAuthToken = apiKey
		betaHeader = claude.APIKeyBetaHeader
	} else {
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported Anthropic account type for upstream model sync: %s", account.Type), nil,
		)
	}

	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Anthropic base URL", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildV1ModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Anthropic model list URL", err)
	}
	for key, value := range claude.DefaultHeaders() {
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", betaHeader)
	if authHeaderName != "" {
		req.Header.Set(authHeaderName, authHeaderValue)
	} else {
		// Ollama Cloud Anthropic 兼容端点按实际 base_url 强制 Bearer，其余保持
		// extra/default 行为。
		setAnthropicAPIKeyAuthHeader(req.Header, account, apiKeyAuthToken, normalizedBaseURL)
	}
	// 账号级请求头覆写：模型列表探测与真实转发保持一致的最终头
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

func (s *AccountTestService) buildAntigravityAPIKeyModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	if account.Type != AccountTypeAPIKey {
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported Antigravity account type for upstream model sync: %s", account.Type), nil,
		)
	}
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, newUpstreamModelSyncConfigError("No Antigravity API key is available", nil)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(account.GetCredential("base_url")), "/")
	if baseURL == "" {
		return nil, newUpstreamModelSyncConfigError("Antigravity API-key base URL is required for upstream model sync", nil)
	}
	if !strings.HasSuffix(strings.ToLower(baseURL), "/antigravity") {
		return nil, newUpstreamModelSyncUnsupportedError(
			"Antigravity API-key upstream model sync requires a compatible gateway base URL ending in /antigravity; use Antigravity OAuth for official Cloud Code upstreams",
			nil,
		)
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Antigravity base URL", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildV1ModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Antigravity model list URL", err)
	}
	for key, value := range claude.DefaultHeaders() {
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", claude.APIKeyBetaHeader)
	req.Header.Set("x-api-key", apiKey)
	return req, nil
}

func (s *AccountTestService) buildOpenAIUpstreamModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	if account.IsOpenAIOAuth() {
		return s.buildOpenAIOAuthUpstreamModelsRequest(ctx, account)
	}
	return buildOpenAIAPIKeyModelsRequest(ctx, account, s.validateUpstreamBaseURL)
}

// buildOpenAIAPIKeyModelsRequest is shared by admin discovery and public model
// listing. Codex content negotiation is intentionally absent from this request.
func buildOpenAIAPIKeyModelsRequest(ctx context.Context, account *Account, validateBaseURL func(string) (string, error)) (*http.Request, error) {
	if account.Type != AccountTypeAPIKey {
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported OpenAI account type for upstream model sync: %s", account.Type), nil,
		)
	}
	apiKey := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if apiKey == "" {
		return nil, newUpstreamModelSyncConfigError("No OpenAI API key is available", nil)
	}

	// 协议感知：Anthropic 协议账号的凭证 base_url 指向 /anthropic 端点，模型
	// 列表同步需使用 OpenAI 格式 base（供应商 × 模式默认）。
	baseURL := account.GetOpenAIFormatBaseURL()
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.openai.com"
	}
	normalizedBaseURL, err := validateBaseURL(baseURL)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid OpenAI base URL", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildOpenAIModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid OpenAI model list URL", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	// 账号级请求头覆写：模型列表探测与真实转发保持一致的最终头
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

// buildOpenAIOAuthUpstreamModelsRequest uses ChatGPT's Codex model manifest.
// OAuth subscriptions do not expose the public Platform API /v1/models endpoint,
// so treating them like API-key accounts makes the admin sync button fail locally.
func (s *AccountTestService) buildOpenAIOAuthUpstreamModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	credentialAccount, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Failed to resolve OpenAI account credentials", err)
	}
	if !credentialAccount.IsOpenAIOAuth() {
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported OpenAI account type for upstream model sync: %s", credentialAccount.Type), nil,
		)
	}

	modelsURL, err := buildCodexModelsManifestURL(
		chatgptCodexModelsURL,
		false,
		CodexCanonicalClientVersion(),
	)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid OpenAI Codex model list URL", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL.String(), nil)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid OpenAI Codex model list request", err)
	}

	if credentialAccount.IsOpenAIAgentIdentity() {
		authHeaders, authErr := buildAgentIdentityAuthenticationHeaders(
			ctx,
			s.accountRepo,
			s.agentIdentityWS,
			&s.agentIdentityTaskMu,
			credentialAccount,
		)
		if authErr != nil {
			return nil, newUpstreamModelSyncUpstreamError("Failed to build OpenAI Agent Identity authentication", authErr)
		}
		for key, values := range authHeaders {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	} else {
		accessToken := strings.TrimSpace(credentialAccount.GetOpenAIAccessToken())
		if accessToken == "" {
			return nil, newUpstreamModelSyncConfigError("No OpenAI access token is available", nil)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	identity := resolveCodexOutboundIdentity(credentialAccount.GetOpenAIUserAgent())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Originator", identity.originator)
	req.Header.Set("User-Agent", identity.userAgent)
	req.Header.Set("Version", identity.version)
	setOpenAIChatGPTAccountHeaders(req.Header, credentialAccount)
	credentialAccount.ApplyHeaderOverrides(req.Header)
	enforceCodexIdentityHeadersWithUA(req.Header, credentialAccount.GetOpenAIUserAgent())
	return req, nil
}

func (s *AccountTestService) buildGeminiUpstreamModelsRequest(ctx context.Context, account *Account) (*http.Request, error) {
	baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	if strings.TrimSpace(baseURL) == "" {
		baseURL = geminicli.AIStudioBaseURL
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Gemini base URL", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildGeminiModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Gemini model list URL", err)
	}
	req.Header.Set("Accept", "application/json")

	switch account.Type {
	case AccountTypeAPIKey:
		apiKey := strings.TrimSpace(account.GetCredential("api_key"))
		if apiKey == "" {
			return nil, newUpstreamModelSyncConfigError("No Gemini API key is available", nil)
		}
		req.Header.Set("x-goog-api-key", apiKey)
	case AccountTypeOAuth:
		if strings.TrimSpace(account.GetCredential("project_id")) != "" {
			return nil, newUpstreamModelSyncUnsupportedError("Gemini Code Assist model listing is not supported by this sync button", nil)
		}
		if s.geminiTokenProvider == nil {
			return nil, newUpstreamModelSyncConfigError("Gemini token provider is not configured", nil)
		}
		accessToken, tokenErr := s.geminiTokenProvider.GetAccessToken(ctx, account)
		if tokenErr != nil {
			return nil, newUpstreamModelSyncUpstreamError("Failed to get Gemini access token", tokenErr)
		}
		accessToken = strings.TrimSpace(accessToken)
		if accessToken == "" {
			return nil, newUpstreamModelSyncConfigError("No Gemini access token is available", nil)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
	default:
		return nil, newUpstreamModelSyncUnsupportedError(
			fmt.Sprintf("Unsupported Gemini account type for upstream model sync: %s", account.Type), nil,
		)
	}

	return req, nil
}

func (s *AccountTestService) fetchAntigravityOAuthUpstreamModels(ctx context.Context, account *Account) ([]string, error) {
	if s.antigravityGatewayService == nil || s.antigravityGatewayService.GetTokenProvider() == nil {
		return nil, newUpstreamModelSyncConfigError("Antigravity token provider is not configured", nil)
	}

	accessToken, err := s.antigravityGatewayService.GetTokenProvider().GetAccessToken(ctx, account)
	if err != nil {
		return nil, newUpstreamModelSyncUpstreamError("Failed to get Antigravity access token", err)
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, newUpstreamModelSyncConfigError("No Antigravity access token is available", nil)
	}

	client, err := antigravity.NewClient(upstreamModelsProxyURL(account))
	if err != nil {
		return nil, newUpstreamModelSyncConfigError("Failed to configure Antigravity client", err)
	}
	modelsResp, _, err := client.FetchAvailableModels(
		ctx,
		accessToken,
		strings.TrimSpace(account.GetCredential("project_id")),
		resolveModelsListReadLimit(s.cfg),
	)
	if err != nil {
		return nil, newUpstreamModelSyncUpstreamError("Failed to fetch Antigravity available models", err)
	}
	if modelsResp == nil || len(modelsResp.Models) == 0 {
		return nil, newUpstreamModelSyncUpstreamError("Upstream returned no supported models", nil)
	}

	models := make([]string, 0, len(modelsResp.Models))
	for modelID := range modelsResp.Models {
		models = append(models, strings.TrimSpace(modelID))
	}
	return dedupeAndSortModelIDs(models), nil
}

func (s *AccountTestService) doUpstreamModelsRequest(req *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	if s.tlsFPProfileService == nil {
		return s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, nil)
	}
	return s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
}

func upstreamModelsProxyURL(account *Account) string {
	if account != nil && account.ProxyID != nil && account.Proxy != nil {
		return account.Proxy.URL()
	}
	return ""
}

func buildV1ModelsURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(normalized, "/v1/models") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/v1") {
		return normalized + "/models"
	}
	return normalized + "/v1/models"
}

func buildOpenAIModelsURL(base string) string {
	return buildOpenAIEndpointURL(base, "/v1/models")
}

func buildGeminiModelsURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(normalized, "/v1beta/models") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/v1beta") {
		return normalized + "/models"
	}
	return normalized + "/v1beta/models"
}

type upstreamModelEntry struct {
	ID           string          `json:"id"`
	Slug         string          `json:"slug"`
	Model        string          `json:"model"`
	ModelID      string          `json:"modelId"`
	ModelIDSnake string          `json:"model_id"`
	Name         string          `json:"name"`
	Meta         json.RawMessage `json:"_meta"`
}

type upstreamModelEntryMetadata struct {
	ID           string `json:"id"`
	Slug         string `json:"slug"`
	Model        string `json:"model"`
	ModelID      string `json:"modelId"`
	ModelIDSnake string `json:"model_id"`
	Name         string `json:"name"`
}

type upstreamModelCapabilityEntry struct {
	upstreamModelEntry
	DisplayName              string                     `json:"display_name"`
	Description              string                     `json:"description"`
	Reasoning                *bool                      `json:"reasoning"`
	DefaultReasoningLevel    string                     `json:"default_reasoning_level"`
	SupportedReasoningLevels []json.RawMessage          `json:"supported_reasoning_levels"`
	ReasoningOptions         []modelsDevReasoningOption `json:"reasoning_options"`
	InputModalities          []string                   `json:"input_modalities"`
	Modalities               modelsDevModalities        `json:"modalities"`
	ContextWindow            int64                      `json:"context_window"`
	MaxContextWindow         int64                      `json:"max_context_window"`
	MaxOutputTokens          int64                      `json:"max_output_tokens"`
	Limit                    modelsDevLimit             `json:"limit"`
}

func extractUpstreamModelIDs(body []byte) ([]string, error) {
	return extractUpstreamModelIDsWithSelector(body, upstreamModelEntryID)
}

func extractGrokUpstreamModelIDs(body []byte) ([]string, error) {
	return extractUpstreamModelIDsWithSelector(body, grokUpstreamModelEntryID)
}

func extractUpstreamModelCatalog(body []byte, grok bool) ([]string, map[string]UpstreamModelMetadata, error) {
	entries, err := extractUpstreamModelRawEntries(body)
	if err != nil {
		return nil, nil, err
	}
	selectID := upstreamModelEntryID
	if grok {
		selectID = grokUpstreamModelEntryID
	}

	models := make([]string, 0, len(entries))
	metadata := make(map[string]UpstreamModelMetadata)
	for _, raw := range entries {
		var capability upstreamModelCapabilityEntry
		if err := json.Unmarshal(raw, &capability); err != nil {
			continue
		}
		modelID := strings.TrimSpace(selectID(capability.upstreamModelEntry))
		if modelID == "" {
			continue
		}
		models = append(models, modelID)
		entry := upstreamMetadataFromCapabilityEntry(modelID, capability)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err == nil {
			entry.CodexToolCapabilities = make(map[string]json.RawMessage)
			applyCodexToolCapabilities(entry.CodexToolCapabilities, fields, true)
		}
		if upstreamModelMetadataIsUseful(entry) {
			metadata[modelID] = entry
		}
	}
	return dedupeAndSortModelIDs(models), metadata, nil
}

func extractUpstreamModelRawEntries(body []byte) ([]json.RawMessage, error) {
	var response struct {
		Data   []json.RawMessage `json:"data"`
		Models []json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(body, &response); err == nil && (response.Data != nil || response.Models != nil) {
		entries := make([]json.RawMessage, 0, len(response.Data)+len(response.Models))
		entries = append(entries, response.Data...)
		entries = append(entries, response.Models...)
		return entries, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parse upstream model catalog: %w", err)
	}
	return entries, nil
}

func upstreamMetadataFromCapabilityEntry(modelID string, entry upstreamModelCapabilityEntry) UpstreamModelMetadata {
	levels := reasoningLevelsFromRawEntries(entry.SupportedReasoningLevels)
	if len(levels) == 0 {
		levels = reasoningLevelsFromModelsDevOptions(entry.Reasoning, entry.ReasoningOptions)
	}
	reasoning := entry.Reasoning
	if reasoning == nil && len(levels) > 0 {
		inferred := len(levels) != 1 || levels[0] != "none"
		reasoning = &inferred
	}
	modalities := entry.InputModalities
	if len(modalities) == 0 {
		modalities = entry.Modalities.Input
	}
	contextWindow := entry.ContextWindow
	if contextWindow <= 0 {
		contextWindow = entry.MaxContextWindow
	}
	if contextWindow <= 0 {
		contextWindow = entry.Limit.Context
	}
	maxOutputTokens := entry.MaxOutputTokens
	if maxOutputTokens <= 0 {
		maxOutputTokens = entry.Limit.Output
	}
	defaultReasoningLevel := normalizeReasoningLevel(entry.DefaultReasoningLevel)
	if defaultReasoningLevel == "" && len(levels) > 0 {
		defaultReasoningLevel = levels[0]
	}
	displayName := strings.TrimSpace(entry.DisplayName)
	if displayName == "" && strings.TrimSpace(entry.Name) != "" && strings.TrimSpace(entry.Name) != modelID {
		displayName = strings.TrimSpace(entry.Name)
	}
	return UpstreamModelMetadata{
		ID:                       modelID,
		DisplayName:              displayName,
		Description:              strings.TrimSpace(entry.Description),
		Reasoning:                reasoning,
		DefaultReasoningLevel:    defaultReasoningLevel,
		SupportedReasoningLevels: levels,
		InputModalities:          normalizeCodexInputModalities(modalities),
		ContextWindow:            contextWindow,
		MaxContextWindow:         entry.MaxContextWindow,
		MaxOutputTokens:          maxOutputTokens,
	}
}

func reasoningLevelsFromRawEntries(entries []json.RawMessage) []string {
	levels := make([]string, 0, len(entries))
	for _, raw := range entries {
		var effort string
		if err := json.Unmarshal(raw, &effort); err == nil {
			levels = append(levels, effort)
			continue
		}
		var level struct {
			Effort string `json:"effort"`
		}
		if err := json.Unmarshal(raw, &level); err == nil {
			levels = append(levels, level.Effort)
		}
	}
	return normalizeReasoningLevels(levels)
}

func normalizeReasoningLevels(levels []string) []string {
	seen := make(map[string]struct{}, len(levels))
	normalized := make([]string, 0, len(levels))
	for _, level := range levels {
		level = normalizeReasoningLevel(level)
		if level == "" {
			continue
		}
		if _, exists := seen[level]; exists {
			continue
		}
		seen[level] = struct{}{}
		normalized = append(normalized, level)
	}
	return normalized
}

func normalizeReasoningLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	switch level {
	case "off", "disabled":
		return "none"
	case "extra-high", "extra_high":
		return "xhigh"
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		return level
	default:
		return ""
	}
}

func normalizeCodexInputModalities(modalities []string) []string {
	seen := make(map[string]struct{}, len(modalities))
	normalized := make([]string, 0, len(modalities))
	for _, modality := range modalities {
		modality = strings.ToLower(strings.TrimSpace(modality))
		if modality != "text" && modality != "image" {
			continue
		}
		if _, exists := seen[modality]; exists {
			continue
		}
		seen[modality] = struct{}{}
		normalized = append(normalized, modality)
	}
	return normalized
}

func extractUpstreamModelIDsWithSelector(body []byte, selectID func(upstreamModelEntry) string) ([]string, error) {
	var response struct {
		Data   []upstreamModelEntry `json:"data"`
		Models []upstreamModelEntry `json:"models"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		var arrayResponse []upstreamModelEntry
		if arrayErr := json.Unmarshal(body, &arrayResponse); arrayErr != nil {
			return nil, fmt.Errorf("parse upstream model list: %w", err)
		}

		models := make([]string, 0, len(arrayResponse))
		for _, entry := range arrayResponse {
			models = append(models, selectID(entry))
		}
		return dedupeAndSortModelIDs(models), nil
	}

	models := make([]string, 0, len(response.Data)+len(response.Models))
	for _, entry := range response.Data {
		models = append(models, selectID(entry))
	}
	for _, entry := range response.Models {
		models = append(models, selectID(entry))
	}

	if len(models) == 0 {
		var arrayResponse []upstreamModelEntry
		if err := json.Unmarshal(body, &arrayResponse); err == nil {
			for _, entry := range arrayResponse {
				models = append(models, selectID(entry))
			}
		}
	}

	return dedupeAndSortModelIDs(models), nil
}

func upstreamModelEntryID(entry upstreamModelEntry) string {
	modelID := strings.TrimSpace(entry.ID)
	if modelID == "" {
		modelID = strings.TrimSpace(entry.Slug)
	}
	if modelID == "" {
		modelID = strings.TrimSpace(entry.Name)
	}
	return strings.TrimPrefix(modelID, "models/")
}

func grokUpstreamModelEntryID(entry upstreamModelEntry) string {
	candidates := []string{
		entry.Model,
		entry.ModelID,
		entry.ModelIDSnake,
		entry.ID,
		entry.Slug,
	}
	if len(entry.Meta) > 0 {
		var meta upstreamModelEntryMetadata
		if err := json.Unmarshal(entry.Meta, &meta); err == nil {
			candidates = append(candidates,
				meta.Model,
				meta.ModelID,
				meta.ModelIDSnake,
				meta.ID,
				meta.Slug,
				meta.Name,
			)
		}
	}
	// `name` is a display label in the Grok catalog, so keep it as the final
	// compatibility fallback rather than preferring it over protocol model IDs.
	candidates = append(candidates, entry.Name)
	for _, candidate := range candidates {
		modelID := strings.TrimSpace(candidate)
		if modelID != "" {
			return strings.TrimPrefix(modelID, "models/")
		}
	}
	return ""
}

func dedupeAndSortModelIDs(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	sort.Strings(result)
	return result
}
