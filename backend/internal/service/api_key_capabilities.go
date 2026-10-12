package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// CapabilityReason is the stable, public reason returned when a capability is
// unavailable. Keep this set closed: internal lookup details must not cross the
// user-facing capability contract.
type CapabilityReason string

const (
	CapabilityReasonNoGroup                     CapabilityReason = "no_group"
	CapabilityReasonGroupDisabled               CapabilityReason = "group_disabled"
	CapabilityReasonPlatformUnsupported         CapabilityReason = "platform_unsupported"
	CapabilityReasonFeatureDisabled             CapabilityReason = "feature_disabled"
	CapabilityReasonNoAccount                   CapabilityReason = "no_account"
	CapabilityReasonModelNotAvailable           CapabilityReason = "model_not_available"
	CapabilityReasonAccountCapabilityUnverified CapabilityReason = "account_capability_unverified"
	CapabilityReasonRouteUnsupported            CapabilityReason = "route_unsupported"
)

// Keep the shorter aliases for callers that use the names from the
// implementation plan. The wire values remain the closed public enum above.
const (
	ReasonNoGroup                     = CapabilityReasonNoGroup
	ReasonGroupDisabled               = CapabilityReasonGroupDisabled
	ReasonPlatformUnsupported         = CapabilityReasonPlatformUnsupported
	ReasonFeatureDisabled             = CapabilityReasonFeatureDisabled
	ReasonNoAccount                   = CapabilityReasonNoAccount
	ReasonModelNotAvailable           = CapabilityReasonModelNotAvailable
	ReasonAccountCapabilityUnverified = CapabilityReasonAccountCapabilityUnverified
	ReasonRouteUnsupported            = CapabilityReasonRouteUnsupported
)

type capabilityAPIKeyReader interface {
	GetByID(ctx context.Context, id int64) (*APIKey, error)
}

type capabilityGroupReader interface {
	GetByID(ctx context.Context, id int64) (*Group, error)
}

type capabilityAccountReader interface {
	ListModelAvailabilityCandidates(ctx context.Context, groupID *int64, platforms []string, includeGrouped bool) ([]Account, error)
}

type capabilityAsyncImageState interface {
	Enabled() bool
	Pollable() bool
}

type capabilityBatchImageReader interface {
	ListModels(ctx context.Context, owner BatchImageOwner) (*BatchImagePublicModelsResponse, error)
}

// APIKeyCapabilitiesService resolves public, non-secret capability metadata for
// one API key. The narrow reader interfaces keep the resolver independent from
// repository mutation methods and allow the same candidate query used by the
// scheduler to be injected without duplicating scheduler behavior.
type APIKeyCapabilitiesService struct {
	apiKeyRepo       capabilityAPIKeyReader
	groupRepo        capabilityGroupReader
	accountRepo      capabilityAccountReader
	asyncImageState  capabilityAsyncImageState
	batchImageReader capabilityBatchImageReader
}

func NewAPIKeyCapabilitiesService(
	apiKeyRepo APIKeyRepository,
	groupRepo GroupRepository,
	accountRepo AccountRepository,
) *APIKeyCapabilitiesService {
	return &APIKeyCapabilitiesService{
		apiKeyRepo:  apiKeyRepo,
		groupRepo:   groupRepo,
		accountRepo: accountRepo,
	}
}

// SetAsyncImageState wires the existing image task service into capability
// resolution without exposing storage or task implementation details.
func (s *APIKeyCapabilitiesService) SetAsyncImageState(state capabilityAsyncImageState) {
	if s != nil {
		s.asyncImageState = state
	}
}

// SetBatchImageReader wires the existing public batch model discovery service
// into capability resolution. That service owns provider, scheduler, mapping,
// and pricing eligibility, so this resolver does not duplicate those rules.
func (s *APIKeyCapabilitiesService) SetBatchImageReader(reader capabilityBatchImageReader) {
	if s != nil {
		s.batchImageReader = reader
	}
}

type APIKeyCapabilities struct {
	Object       string                       `json:"object"`
	KeyID        int64                        `json:"key_id"`
	Group        *APIKeyCapabilityGroup       `json:"group,omitempty"`
	Image        APIKeyImageCapability        `json:"image"`
	BatchImage   APIKeyBatchImageCapability   `json:"batch_image"`
	Video        APIKeyVideoCapability        `json:"video"`
	Seedance     APIKeySeedanceCapability     `json:"seedance"`
	GeminiNative APIKeyGeminiNativeCapability `json:"gemini_native"`
}

type APIKeyCapabilityGroup struct {
	ID       int64  `json:"id"`
	Platform string `json:"platform"`
}

type APIKeyCapabilityRoute struct {
	Available bool             `json:"available"`
	Reason    CapabilityReason `json:"reason,omitempty"`
	Method    string           `json:"method,omitempty"`
	Path      string           `json:"path,omitempty"`
}

type APIKeyImageCapability struct {
	Available        bool                       `json:"available"`
	GateOpen         bool                       `json:"gate_open"`
	AccountAvailable bool                       `json:"account_available"`
	Sync             APIKeyCapabilityRoute      `json:"sync"`
	Async            APIKeyImageAsyncCapability `json:"async"`
}

type APIKeyImageAsyncCapability struct {
	Generation APIKeyCapabilityRoute `json:"generation"`
	Edit       APIKeyCapabilityRoute `json:"edit"`
	Poll       APIKeyCapabilityRoute `json:"poll"`
}

type APIKeyBatchImageCapability struct {
	Available bool                  `json:"available"`
	Reason    CapabilityReason      `json:"reason,omitempty"`
	Models    APIKeyCapabilityRoute `json:"models"`
}

type APIKeyVideoCapability struct {
	Generation APIKeyCapabilityRoute `json:"generation"`
	Status     APIKeyCapabilityRoute `json:"status"`
	Content    APIKeyCapabilityRoute `json:"content"`
	Edit       APIKeyCapabilityRoute `json:"edit"`
	Extension  APIKeyCapabilityRoute `json:"extension"`
}

type APIKeySeedanceCapability struct {
	Available bool                  `json:"available"`
	Reason    CapabilityReason      `json:"reason,omitempty"`
	Create    APIKeyCapabilityRoute `json:"create"`
	Status    APIKeyCapabilityRoute `json:"status"`
	Delete    APIKeyCapabilityRoute `json:"delete"`
}

type APIKeyGeminiNativeCapability struct {
	ImageGeneration APIKeyGeminiNativeImageCapability `json:"image_generation"`
}

type APIKeyGeminiNativeImageCapability struct {
	Available             bool                  `json:"available"`
	Reason                CapabilityReason      `json:"reason,omitempty"`
	GenerateContent       APIKeyCapabilityRoute `json:"generate_content"`
	StreamGenerateContent APIKeyCapabilityRoute `json:"stream_generate_content"`
}

// ResolveCapabilities returns only public capability metadata. Missing key or
// group is represented as an unavailable response so the user-facing endpoint
// does not leak repository details.
func (s *APIKeyCapabilitiesService) ResolveCapabilities(ctx context.Context, userID, keyID int64) (*APIKeyCapabilities, error) {
	result := unavailableAPIKeyCapabilities(keyID, CapabilityReasonNoGroup)
	if s == nil || s.apiKeyRepo == nil {
		return unavailableAPIKeyCapabilities(keyID, CapabilityReasonAccountCapabilityUnverified), nil
	}

	key, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil || key == nil || key.UserID != userID || key.GroupID == nil {
		return result, nil
	}

	group := key.Group
	if group == nil && s.groupRepo != nil {
		group, err = s.groupRepo.GetByID(ctx, *key.GroupID)
		if err != nil {
			return unavailableAPIKeyCapabilities(keyID, CapabilityReasonAccountCapabilityUnverified), nil
		}
	}
	if group == nil {
		return result, nil
	}

	result.Group = &APIKeyCapabilityGroup{ID: group.ID, Platform: group.Platform}
	result.Image = s.resolveImageCapability(ctx, key, group)
	result.BatchImage = s.resolveBatchImageCapability(ctx, key, group)
	result.Video = s.resolveVideoCapability(ctx, key, group)
	result.Seedance = s.resolveSeedanceCapability(ctx, key, group)
	result.GeminiNative = s.resolveGeminiNativeImageCapability(ctx, key, group)
	return result, nil
}

func (s *APIKeyCapabilitiesService) resolveImageCapability(ctx context.Context, key *APIKey, group *Group) APIKeyImageCapability {
	cap := APIKeyImageCapability{}
	cap.GateOpen = GroupAllowsImageGeneration(group)
	if !cap.GateOpen {
		setImageCapabilityReason(&cap, CapabilityReasonGroupDisabled)
		return cap
	}
	if group == nil {
		setImageCapabilityReason(&cap, CapabilityReasonNoGroup)
		return cap
	}

	platform := group.Platform
	if platform != PlatformOpenAI && platform != PlatformGrok {
		setImageCapabilityReason(&cap, CapabilityReasonPlatformUnsupported)
		return cap
	}
	if s == nil || s.accountRepo == nil || key == nil || key.GroupID == nil {
		setImageCapabilityReason(&cap, CapabilityReasonAccountCapabilityUnverified)
		return cap
	}

	accounts, err := s.accountRepo.ListModelAvailabilityCandidates(ctx, key.GroupID, []string{platform}, false)
	if err != nil {
		setImageCapabilityReason(&cap, CapabilityReasonAccountCapabilityUnverified)
		return cap
	}
	for i := range accounts {
		account := &accounts[i]
		for _, publicModel := range imageCapabilityModelCandidates(account, group, platform) {
			mappedModel := account.GetMappedModel(publicModel)
			if !imageCapabilityMappedModelSupported(platform, mappedModel) ||
				!group.ModelAllowlist.Allows(publicModel) ||
				!account.IsSchedulable() ||
				!account.IsModelSupported(publicModel) ||
				!account.IsSchedulableForModelWithContext(ctx, publicModel) {
				continue
			}
			if platform == PlatformOpenAI {
				requiredCapability := OpenAIImagesCapabilityNative
				if isGeminiCompatibleImageModel(mappedModel) {
					requiredCapability = OpenAIImagesCapabilityAPIKey
				}
				if !account.SupportsOpenAIImageCapability(requiredCapability) {
					continue
				}
			} else if !account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityGrokMediaGeneration) ||
				!grokMediaCapabilityDiscoveryEligible(account) {
				continue
			}
			cap.AccountAvailable = true
			break
		}
		if cap.AccountAvailable {
			break
		}
	}
	if !cap.AccountAvailable {
		setImageCapabilityReason(&cap, CapabilityReasonNoAccount)
		return cap
	}

	cap.Available = true
	cap.Sync = APIKeyCapabilityRoute{Available: true, Method: http.MethodPost, Path: "/v1/images/generations"}
	if s.asyncImageState != nil && s.asyncImageState.Enabled() {
		cap.Async.Generation = APIKeyCapabilityRoute{Available: true, Method: http.MethodPost, Path: "/v1/images/generations/async"}
		cap.Async.Edit = APIKeyCapabilityRoute{Available: true, Method: http.MethodPost, Path: "/v1/images/edits/async"}
	} else {
		cap.Async.Generation.Reason = CapabilityReasonFeatureDisabled
		cap.Async.Edit.Reason = CapabilityReasonFeatureDisabled
	}
	if s.asyncImageState != nil && s.asyncImageState.Pollable() {
		cap.Async.Poll = APIKeyCapabilityRoute{Available: true, Method: http.MethodGet, Path: "/v1/images/tasks/{task_id}"}
	} else {
		cap.Async.Poll.Reason = CapabilityReasonFeatureDisabled
	}
	return cap
}

func setImageCapabilityReason(cap *APIKeyImageCapability, reason CapabilityReason) {
	if cap == nil {
		return
	}
	cap.Sync.Reason = reason
	cap.Async.Generation.Reason = reason
	cap.Async.Edit.Reason = reason
	cap.Async.Poll.Reason = reason
}

func imageCapabilityModelCandidates(account *Account, group *Group, platform string) []string {
	if account == nil {
		return nil
	}
	seen := make(map[string]struct{})
	candidates := make([]string, 0)
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		if _, exists := seen[model]; exists {
			return
		}
		seen[model] = struct{}{}
		candidates = append(candidates, model)
	}

	for publicModel := range account.GetModelMapping() {
		mappedModel := account.GetMappedModel(publicModel)
		if imageCapabilityMappedModelSupported(platform, publicModel) || imageCapabilityMappedModelSupported(platform, mappedModel) {
			add(publicModel)
		}
	}
	if group != nil && group.ModelAllowlist.Enabled {
		for _, publicModel := range group.ModelAllowlist.Models {
			if imageCapabilityMappedModelSupported(platform, publicModel) {
				add(publicModel)
			}
		}
	}
	if len(candidates) == 0 && (group == nil || !group.ModelAllowlist.Enabled) {
		if platform == PlatformGrok {
			add("grok-imagine-image")
		} else {
			add("gpt-image-2")
		}
	}
	return candidates
}

func imageCapabilityMappedModelSupported(platform, model string) bool {
	if platform == PlatformGrok {
		return isGrokImageGenerationModel(model)
	}
	return isOpenAIImageGenerationModel(model) || isGeminiCompatibleImageModel(model)
}

func grokMediaCapabilityDiscoveryEligible(account *Account) bool {
	eligible, reason := account.GrokMediaGenerationEligibility()
	if !eligible {
		return false
	}
	switch reason {
	case "eligible", "non_oauth", "override_enabled":
		return true
	default:
		return false
	}
}

func (s *APIKeyCapabilitiesService) resolveBatchImageCapability(ctx context.Context, key *APIKey, group *Group) APIKeyBatchImageCapability {
	cap := APIKeyBatchImageCapability{}
	if group == nil {
		cap.Reason = CapabilityReasonNoGroup
		return cap
	}
	if !group.AllowBatchImageGeneration {
		cap.Reason = CapabilityReasonGroupDisabled
		return cap
	}
	if group.Platform != PlatformGemini {
		cap.Reason = CapabilityReasonPlatformUnsupported
		return cap
	}
	if s == nil || s.accountRepo == nil || s.batchImageReader == nil || key == nil || key.GroupID == nil {
		cap.Reason = CapabilityReasonAccountCapabilityUnverified
		return cap
	}

	accounts, err := s.accountRepo.ListModelAvailabilityCandidates(ctx, key.GroupID, []string{PlatformGemini}, false)
	if err != nil {
		cap.Reason = CapabilityReasonAccountCapabilityUnverified
		return cap
	}
	if len(accounts) == 0 {
		cap.Reason = CapabilityReasonNoAccount
		return cap
	}

	models, err := s.batchImageReader.ListModels(ctx, BatchImageOwner{
		UserID:   key.UserID,
		APIKeyID: key.ID,
		GroupID:  key.GroupID,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrBatchImageDisabled):
			cap.Reason = CapabilityReasonFeatureDisabled
		case errors.Is(err, ErrBatchImageGroupDisabled):
			cap.Reason = CapabilityReasonGroupDisabled
		default:
			cap.Reason = CapabilityReasonAccountCapabilityUnverified
		}
		return cap
	}
	if models == nil || len(models.Data) == 0 {
		cap.Reason = CapabilityReasonModelNotAvailable
		return cap
	}

	for _, model := range models.Data {
		if batchImageCapabilityCandidateEligible(ctx, model, accounts, group) {
			cap.Available = true
			cap.Models = APIKeyCapabilityRoute{Available: true, Method: http.MethodGet, Path: "/v1/images/batches/models"}
			return cap
		}
	}
	cap.Reason = CapabilityReasonModelNotAvailable
	return cap
}

// batchImageCapabilityCandidateEligible repeats the runtime candidate gates at
// capability-discovery time. The batch reader is authoritative for provider
// discovery, but a public model must still be backed by the same group
// allowlist, account mapping, model support, scheduler, and Gemini lane.
func batchImageCapabilityCandidateEligible(ctx context.Context, model BatchImagePublicModel, accounts []Account, group *Group) bool {
	modelID := strings.TrimSpace(model.ID)
	provider := strings.TrimSpace(model.Provider)
	if modelID == "" || batchImageProviderPlatform(provider) != PlatformGemini {
		return false
	}
	if group != nil && !group.ModelAllowlist.Allows(modelID) {
		return false
	}
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != PlatformGemini ||
			!account.IsSchedulableForModelWithContext(ctx, modelID) ||
			!account.IsModelSupported(modelID) ||
			!imageCapabilityMappedModelSupported(PlatformGemini, account.GetMappedModel(modelID)) {
			continue
		}
		return true
	}
	return false
}

const (
	grokVideoGenerationPath = "/v1/videos/generations"
	grokVideoStatusPath     = "/v1/videos/generations/{request_id}"
	grokVideoContentPath    = "/v1/videos/generations/{request_id}/content"
	grokVideoEditPath       = "/v1/videos/edits"
	grokVideoExtensionPath  = "/v1/videos/extensions"
	seedanceDefaultModel    = "doubao-seedance-1-0-pro"
	seedanceCreatePath      = "/contents/generations/tasks"
	seedanceStatusPath      = "/contents/generations/tasks/{task_id}"
)

func (s *APIKeyCapabilitiesService) resolveVideoCapability(ctx context.Context, key *APIKey, group *Group) APIKeyVideoCapability {
	cap := APIKeyVideoCapability{}
	composite := group != nil && group.Platform == PlatformComposite
	setReason := func(reason CapabilityReason) {
		cap.Generation.Reason = reason
		cap.Status.Reason = reason
		cap.Content.Reason = reason
		if composite {
			cap.Edit.Reason = CapabilityReasonRouteUnsupported
			cap.Extension.Reason = CapabilityReasonRouteUnsupported
		} else {
			cap.Edit.Reason = reason
			cap.Extension.Reason = reason
		}
	}
	if group == nil {
		setReason(CapabilityReasonNoGroup)
		return cap
	}
	if group.Platform != PlatformGrok && group.Platform != PlatformComposite {
		setReason(CapabilityReasonPlatformUnsupported)
		return cap
	}
	if !GroupAllowsImageGeneration(group) {
		setReason(CapabilityReasonGroupDisabled)
		return cap
	}
	if composite {
		cap.Edit.Reason = CapabilityReasonRouteUnsupported
		cap.Extension.Reason = CapabilityReasonRouteUnsupported
	}
	if s == nil || s.accountRepo == nil || key == nil || key.GroupID == nil {
		setReason(CapabilityReasonAccountCapabilityUnverified)
		return cap
	}

	accounts, err := s.accountRepo.ListModelAvailabilityCandidates(ctx, key.GroupID, []string{PlatformGrok}, false)
	if err != nil {
		setReason(CapabilityReasonAccountCapabilityUnverified)
		return cap
	}
	for i := range accounts {
		account := &accounts[i]
		for _, publicModel := range grokVideoModelCandidates(account, group) {
			mappedModel := account.GetMappedModel(publicModel)
			if CanonicalGrokImagineVideoPriceFamily(mappedModel) == "" ||
				!group.ModelAllowlist.Allows(publicModel) ||
				!account.IsSchedulable() || !account.IsModelSupported(publicModel) ||
				!account.IsSchedulableForModelWithContext(ctx, publicModel) ||
				!account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityGrokMediaGeneration) ||
				!grokMediaCapabilityDiscoveryEligible(account) {
				continue
			}
			cap.Generation = APIKeyCapabilityRoute{Available: true, Method: http.MethodPost, Path: grokVideoGenerationPath}
			cap.Status = APIKeyCapabilityRoute{Available: true, Method: http.MethodGet, Path: grokVideoStatusPath}
			cap.Content = APIKeyCapabilityRoute{Available: true, Method: http.MethodGet, Path: grokVideoContentPath}
			if !composite {
				cap.Edit = APIKeyCapabilityRoute{Available: true, Method: http.MethodPost, Path: grokVideoEditPath}
				cap.Extension = APIKeyCapabilityRoute{Available: true, Method: http.MethodPost, Path: grokVideoExtensionPath}
			}
			return cap
		}
	}

	setReason(CapabilityReasonNoAccount)
	return cap
}

func grokVideoModelCandidates(account *Account, group *Group) []string {
	if account == nil {
		return nil
	}
	seen := make(map[string]struct{})
	candidates := make([]string, 0)
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		if _, exists := seen[model]; exists {
			return
		}
		seen[model] = struct{}{}
		candidates = append(candidates, model)
	}
	add("grok-imagine-video")
	add("grok-imagine-video-1.5")
	for publicModel := range account.GetModelMapping() {
		mappedModel := account.GetMappedModel(publicModel)
		if CanonicalGrokImagineVideoPriceFamily(publicModel) != "" || CanonicalGrokImagineVideoPriceFamily(mappedModel) != "" {
			add(publicModel)
		}
	}
	if group != nil && group.ModelAllowlist.Enabled {
		for _, publicModel := range group.ModelAllowlist.Models {
			if CanonicalGrokImagineVideoPriceFamily(publicModel) != "" {
				add(publicModel)
			}
		}
	}
	return candidates
}

func seedanceAccountRuntimeEligible(account *Account) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return false
	}
	if strings.TrimSpace(account.GetCredential("base_url")) == "" {
		return false
	}
	return account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilitySeedance)
}

func (s *APIKeyCapabilitiesService) resolveSeedanceCapability(ctx context.Context, key *APIKey, group *Group) APIKeySeedanceCapability {
	cap := APIKeySeedanceCapability{}
	setReason := func(reason CapabilityReason) {
		cap.Reason = reason
		cap.Create.Reason = reason
		cap.Status.Reason = reason
		cap.Delete.Reason = reason
	}
	if group == nil {
		setReason(CapabilityReasonNoGroup)
		return cap
	}
	if group.Platform != PlatformOpenAI {
		setReason(CapabilityReasonPlatformUnsupported)
		return cap
	}
	if !GroupAllowsImageGeneration(group) {
		setReason(CapabilityReasonGroupDisabled)
		return cap
	}
	if s == nil || s.accountRepo == nil || key == nil || key.GroupID == nil {
		setReason(CapabilityReasonAccountCapabilityUnverified)
		return cap
	}

	accounts, err := s.accountRepo.ListModelAvailabilityCandidates(ctx, key.GroupID, []string{PlatformOpenAI}, false)
	if err != nil {
		setReason(CapabilityReasonAccountCapabilityUnverified)
		return cap
	}
	for i := range accounts {
		account := &accounts[i]
		for _, publicModel := range seedanceModelCandidates(account, group) {
			mappedModel := account.GetMappedModel(publicModel)
			if !isSeedanceModel(mappedModel) {
				continue
			}
			if !group.ModelAllowlist.Allows(publicModel) {
				continue
			}
			if !account.IsSchedulable() || !account.IsModelSupported(publicModel) ||
				!account.IsSchedulableForModelWithContext(ctx, publicModel) ||
				!seedanceAccountRuntimeEligible(account) {
				continue
			}
			cap.Available = true
			cap.Create = APIKeyCapabilityRoute{Available: true, Method: http.MethodPost, Path: seedanceCreatePath}
			cap.Status = APIKeyCapabilityRoute{Available: true, Method: http.MethodGet, Path: seedanceStatusPath}
			cap.Delete = APIKeyCapabilityRoute{Available: true, Method: http.MethodDelete, Path: seedanceStatusPath}
			return cap
		}
	}
	setReason(CapabilityReasonNoAccount)
	return cap
}

func seedanceModelCandidates(account *Account, group *Group) []string {
	if account == nil {
		return nil
	}
	candidates := make([]string, 0)
	seen := make(map[string]struct{})
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || !isSeedanceModel(model) {
			return
		}
		if _, ok := seen[model]; ok {
			return
		}
		seen[model] = struct{}{}
		candidates = append(candidates, model)
	}
	for publicModel := range account.GetModelMapping() {
		mappedModel := account.GetMappedModel(publicModel)
		if isSeedanceModel(publicModel) || isSeedanceModel(mappedModel) {
			model := strings.TrimSpace(publicModel)
			if model != "" {
				if _, ok := seen[model]; !ok {
					seen[model] = struct{}{}
					candidates = append(candidates, model)
				}
			}
		}
	}
	if group != nil && group.ModelAllowlist.Enabled {
		for _, publicModel := range group.ModelAllowlist.Models {
			add(publicModel)
		}
	}
	if len(candidates) == 0 && len(account.GetModelMapping()) == 0 && (group == nil || !group.ModelAllowlist.Enabled) {
		add(seedanceDefaultModel)
	}
	return candidates
}

func isSeedanceModel(model string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(model)), "seedance")
}

const (
	geminiNativeGenerateContentPath       = "/v1beta/models/{model}:generateContent"
	geminiNativeStreamGenerateContentPath = "/v1beta/models/{model}:streamGenerateContent?alt=sse"
)

func (s *APIKeyCapabilitiesService) resolveGeminiNativeImageCapability(ctx context.Context, key *APIKey, group *Group) APIKeyGeminiNativeCapability {
	cap := APIKeyGeminiNativeCapability{}
	cap.ImageGeneration = APIKeyGeminiNativeImageCapability{}
	if group == nil {
		cap.ImageGeneration.Reason = CapabilityReasonNoGroup
		return cap
	}
	if group.Platform != PlatformGemini {
		cap.ImageGeneration.Reason = CapabilityReasonPlatformUnsupported
		return cap
	}
	if !GroupAllowsImageGeneration(group) {
		cap.ImageGeneration.Reason = CapabilityReasonGroupDisabled
		return cap
	}
	if s == nil || s.accountRepo == nil || key == nil || key.GroupID == nil {
		cap.ImageGeneration.Reason = CapabilityReasonAccountCapabilityUnverified
		return cap
	}

	// Native Gemini requests use the same mixed-scheduling candidate scope as the
	// Gemini handler: native Gemini accounts plus explicitly opted-in
	// Antigravity accounts. The capability resolver deliberately reuses the
	// model-availability candidate query instead of inventing a second account
	// lookup with different group/scheduling semantics.
	accounts, err := s.accountRepo.ListModelAvailabilityCandidates(
		ctx,
		key.GroupID,
		[]string{PlatformGemini, PlatformAntigravity},
		false,
	)
	if err != nil {
		cap.ImageGeneration.Reason = CapabilityReasonAccountCapabilityUnverified
		return cap
	}

	seenImageModel := false
	seenSchedulableAccount := false
	for i := range accounts {
		account := &accounts[i]
		if !geminiNativeProviderLane(account) {
			continue
		}
		if !account.IsSchedulable() {
			continue
		}
		seenSchedulableAccount = true

		for _, publicModel := range geminiNativeImageModelCandidates(account, group) {
			mappedModel := account.GetMappedModel(publicModel)
			if !isImageGenerationModel(mappedModel) {
				continue
			}
			seenImageModel = true
			if !group.ModelAllowlist.Allows(publicModel) {
				continue
			}
			if !account.IsModelSupported(publicModel) || !account.IsSchedulableForModelWithContext(ctx, publicModel) {
				continue
			}
			if !geminiNativeProviderRouteSupported(account, mappedModel) {
				continue
			}
			return geminiNativeImageCapabilityAvailable()
		}
	}

	switch {
	case !seenSchedulableAccount:
		cap.ImageGeneration.Reason = CapabilityReasonNoAccount
	case !seenImageModel:
		cap.ImageGeneration.Reason = CapabilityReasonModelNotAvailable
	default:
		// A mapped image model existed, but its group or model scheduler gate
		// rejected it. Do not expose a route that would fail at request time.
		cap.ImageGeneration.Reason = CapabilityReasonNoAccount
	}
	return cap
}

func geminiNativeProviderLane(account *Account) bool {
	if account == nil {
		return false
	}
	if account.Platform == PlatformGemini {
		return true
	}
	return account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled()
}

const geminiNativeDefaultImageModel = "gemini-3.1-flash-image"

// geminiNativeImageModelCandidates derives the public model names that can
// exercise the native image route. Empty mappings are intentional passthrough
// mode, so they must not be reported as unavailable just because there is no
// explicit mapping entry to iterate.
func geminiNativeImageModelCandidates(account *Account, group *Group) []string {
	if account == nil {
		return nil
	}

	mapping := account.GetModelMapping()
	candidates := make([]string, 0, len(mapping)+1)
	seen := make(map[string]struct{}, len(mapping)+1)
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		if _, exists := seen[model]; exists {
			return
		}
		seen[model] = struct{}{}
		candidates = append(candidates, model)
	}

	for publicModel := range mapping {
		mappedModel := account.GetMappedModel(publicModel)
		if isImageGenerationModel(publicModel) || isImageGenerationModel(mappedModel) {
			add(publicModel)
		}
	}
	if len(candidates) > 0 {
		return candidates
	}

	if len(mapping) == 0 {
		if group != nil && group.ModelAllowlist.Enabled {
			for _, publicModel := range group.ModelAllowlist.Models {
				if isImageGenerationModel(publicModel) {
					add(publicModel)
				}
			}
		}
		if len(candidates) == 0 && (group == nil || !group.ModelAllowlist.Enabled) {
			add(geminiNativeDefaultImageModel)
		}
	}
	return candidates
}

// geminiNativeProviderRouteSupported mirrors the account-type branches in
// GeminiMessagesCompatService.ForwardNative without performing a network call
// or exposing credentials. A route is only advertised when its local provider
// contract can be constructed for the selected lane.
func geminiNativeProviderRouteSupported(account *Account, mappedModel string) bool {
	if account == nil || !isImageGenerationModel(mappedModel) {
		return false
	}

	switch account.Platform {
	case PlatformGemini:
		switch account.Type {
		case AccountTypeAPIKey:
			return strings.TrimSpace(account.GetCredential("api_key")) != ""
		case AccountTypeOAuth:
			// The token provider may refresh an expired access token, so either
			// persisted token form is sufficient static route evidence.
			return strings.TrimSpace(account.GetCredential("access_token")) != "" ||
				strings.TrimSpace(account.GetCredential("refresh_token")) != ""
		case AccountTypeServiceAccount:
			_, err := parseVertexServiceAccountKey(account)
			return err == nil
		default:
			return false
		}
	case PlatformAntigravity:
		// Mixed scheduling reaches the Gemini compatibility forwarder through
		// the Antigravity API-key lane, which requires its custom base URL.
		return account.Type == AccountTypeAPIKey &&
			strings.TrimSpace(account.GetCredential("api_key")) != "" &&
			strings.TrimSpace(account.GetCredential("base_url")) != ""
	default:
		return false
	}
}

func geminiNativeImageCapabilityAvailable() APIKeyGeminiNativeCapability {
	return APIKeyGeminiNativeCapability{
		ImageGeneration: APIKeyGeminiNativeImageCapability{
			Available: true,
			GenerateContent: APIKeyCapabilityRoute{
				Available: true,
				Method:    http.MethodPost,
				Path:      geminiNativeGenerateContentPath,
			},
			StreamGenerateContent: APIKeyCapabilityRoute{
				Available: true,
				Method:    http.MethodPost,
				Path:      geminiNativeStreamGenerateContentPath,
			},
		},
	}
}

func unavailableAPIKeyCapabilities(keyID int64, reason CapabilityReason) *APIKeyCapabilities {
	return &APIKeyCapabilities{
		Object:       "api_key_capabilities",
		KeyID:        keyID,
		Image:        unavailableImageCapability(reason),
		BatchImage:   APIKeyBatchImageCapability{Reason: reason},
		Video:        unavailableVideoCapability(reason),
		Seedance:     APIKeySeedanceCapability{Reason: reason},
		GeminiNative: unavailableGeminiNativeCapability(reason),
	}
}

func unavailableImageCapability(reason CapabilityReason) APIKeyImageCapability {
	return APIKeyImageCapability{
		Sync: APIKeyCapabilityRoute{Reason: reason},
		Async: APIKeyImageAsyncCapability{
			Generation: APIKeyCapabilityRoute{Reason: reason},
			Edit:       APIKeyCapabilityRoute{Reason: reason},
			Poll:       APIKeyCapabilityRoute{Reason: reason},
		},
	}
}

func unavailableVideoCapability(reason CapabilityReason) APIKeyVideoCapability {
	return APIKeyVideoCapability{
		Generation: APIKeyCapabilityRoute{Reason: reason},
		Status:     APIKeyCapabilityRoute{Reason: reason},
		Content:    APIKeyCapabilityRoute{Reason: reason},
		Edit:       APIKeyCapabilityRoute{Reason: reason},
		Extension:  APIKeyCapabilityRoute{Reason: reason},
	}
}

func unavailableGeminiNativeCapability(reason CapabilityReason) APIKeyGeminiNativeCapability {
	return APIKeyGeminiNativeCapability{
		ImageGeneration: APIKeyGeminiNativeImageCapability{
			Reason:                reason,
			GenerateContent:       APIKeyCapabilityRoute{Reason: reason},
			StreamGenerateContent: APIKeyCapabilityRoute{Reason: reason},
		},
	}
}
