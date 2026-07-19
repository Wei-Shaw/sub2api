package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

const AdminPricingCatalogSource = "admin"

var ErrCatalogAdminRequest = errors.New("invalid catalog admin request")

type CatalogAdminModel struct {
	ID                   int64                `json:"id"`
	CanonicalKey         string               `json:"canonical_key"`
	OperatorState        CatalogOperatorState `json:"operator_state"`
	OperatorReason       string               `json:"operator_reason"`
	OperatorVersion      int64                `json:"operator_version"`
	SourceState          CatalogSourceState   `json:"source_state"`
	Provider             string               `json:"provider"`
	Platform             string               `json:"platform"`
	Mode                 string               `json:"mode"`
	PricingSchemaVersion int                  `json:"pricing_schema_version"`
	PricingValid         bool                 `json:"pricing_valid"`
	PricingSource        string               `json:"pricing_source"`
	Pricing              *LiteLLMModelPricing `json:"pricing"`
	SourceHash           string               `json:"source_hash"`
}

type CatalogAdminSnapshot struct {
	Initialized      bool                `json:"initialized"`
	Scope            string              `json:"scope"`
	Epoch            int64               `json:"epoch"`
	RevisionID       int64               `json:"revision_id"`
	Revision         int64               `json:"revision"`
	Checksum         string              `json:"checksum"`
	PublishedAt      string              `json:"published_at"`
	PricingReadMode  string              `json:"pricing_read_mode"`
	AdmissionMode    string              `json:"admission_mode"`
	ImportMode       string              `json:"import_mode"`
	LegacyModelCount int                 `json:"legacy_model_count"`
	Models           []CatalogAdminModel `json:"models"`
}

type CatalogAdminMutationResult struct {
	Snapshot        CatalogAdminSnapshot `json:"snapshot"`
	RuntimeReloaded bool                 `json:"runtime_reloaded"`
}

type CatalogAdminPricingUpdateRequest struct {
	ModelID                int64
	ExpectedEpoch          int64
	ExpectedRevisionID     int64
	ExpectedSourceHash     string
	InputCostPerToken      *float64
	OutputCostPerToken     *float64
	CacheReadCostPerToken  *float64
	CacheWriteCostPerToken *float64
	Reason                 string
	ActorUserID            int64
	RequestID              string
	CorrelationID          string
}

type ModelCatalogAdminService struct {
	repository ModelCatalogRepository
	adminRepo  ModelCatalogAdminRepository
	runtime    *ModelCatalogProjectionRuntime
	pricing    *PricingService
	scope      string
}

func NewModelCatalogAdminService(repository ModelCatalogRepository, adminRepo ModelCatalogAdminRepository, runtime *ModelCatalogProjectionRuntime, pricing *PricingService, scope string) *ModelCatalogAdminService {
	if strings.TrimSpace(scope) == "" {
		scope = CatalogScopeGlobal
	}
	return &ModelCatalogAdminService{
		repository: repository,
		adminRepo:  adminRepo,
		runtime:    runtime,
		pricing:    pricing,
		scope:      normalizeCatalogScope(scope),
	}
}

func (s *ModelCatalogAdminService) List(ctx context.Context) (CatalogAdminSnapshot, error) {
	if s == nil || s.repository == nil || s.adminRepo == nil {
		return CatalogAdminSnapshot{}, ErrCatalogUnavailable
	}
	modes := ModelCatalogReadModes{}
	if s.runtime != nil {
		modes = s.runtime.ReadModes()
	}
	status := CatalogAdminSnapshot{
		Scope:           s.scope,
		PricingReadMode: modes.PricingReadMode,
		AdmissionMode:   modes.AdmissionMode,
		ImportMode:      modes.ImportMode,
	}
	if s.pricing != nil {
		status.LegacyModelCount = s.pricing.CatalogStatus().ModelCount
	}
	spec, err := s.repository.LoadActiveSnapshot(ctx, s.scope)
	if errors.Is(err, ErrCatalogNoPublication) {
		return status, nil
	}
	if err != nil {
		return CatalogAdminSnapshot{}, fmt.Errorf("load admin catalog: %w", err)
	}
	states, err := s.adminRepo.ListOperatorStates(ctx, catalogModelIDs(spec.Models))
	if err != nil {
		return CatalogAdminSnapshot{}, err
	}
	status.Initialized = true
	status.Epoch = spec.Epoch
	status.RevisionID = spec.RevisionID
	status.Revision = spec.Revision
	status.Checksum = spec.Checksum
	status.PublishedAt = spec.PublishedAt.UTC().Format("2006-01-02T15:04:05Z")
	status.Models = make([]CatalogAdminModel, 0, len(spec.Models))
	for _, model := range spec.Models {
		state, ok := states[model.ID]
		if !ok {
			return CatalogAdminSnapshot{}, fmt.Errorf("%w: id=%d", ErrCatalogModelNotFound, model.ID)
		}
		status.Models = append(status.Models, CatalogAdminModel{
			ID:                   model.ID,
			CanonicalKey:         model.CanonicalKey,
			OperatorState:        model.OperatorState,
			OperatorReason:       model.OperatorReason,
			OperatorVersion:      state.OperatorVersion,
			SourceState:          model.SourceState,
			Provider:             model.Provider,
			Platform:             model.Platform,
			Mode:                 model.Mode,
			PricingSchemaVersion: model.PricingSchemaVersion,
			PricingValid:         model.PricingValid,
			PricingSource:        model.PricingSource,
			Pricing:              cloneCatalogPricing(model.Pricing),
			SourceHash:           model.SourceHash,
		})
	}
	return status, nil
}

func (s *ModelCatalogAdminService) Sync(ctx context.Context) (CatalogAdminSnapshot, error) {
	if s == nil || s.runtime == nil {
		return CatalogAdminSnapshot{}, ErrCatalogUnavailable
	}
	if err := s.runtime.SyncFromSource(ctx); err != nil {
		return CatalogAdminSnapshot{}, fmt.Errorf("sync model catalog: %w", err)
	}
	return s.List(ctx)
}

func (s *ModelCatalogAdminService) UpdateOperatorState(ctx context.Context, request CatalogOperatorStateUpdateRequest) (CatalogAdminMutationResult, error) {
	if s == nil || s.repository == nil || s.adminRepo == nil {
		return CatalogAdminMutationResult{}, ErrCatalogUnavailable
	}
	if request.ModelID <= 0 || request.ExpectedVersion <= 0 || (request.State != CatalogOperatorStateEnabled && request.State != CatalogOperatorStateDisabled) {
		return CatalogAdminMutationResult{}, ErrCatalogAdminRequest
	}
	active, err := s.repository.LoadActiveSnapshot(ctx, s.scope)
	if err != nil {
		return CatalogAdminMutationResult{}, err
	}
	states, err := s.adminRepo.ListOperatorStates(ctx, catalogModelIDs(active.Models))
	if err != nil {
		return CatalogAdminMutationResult{}, err
	}
	state, ok := states[request.ModelID]
	if !ok {
		return CatalogAdminMutationResult{}, fmt.Errorf("%w: id=%d", ErrCatalogModelNotFound, request.ModelID)
	}
	if state.OperatorVersion != request.ExpectedVersion {
		return CatalogAdminMutationResult{}, ErrCatalogOperatorConflict
	}
	if state.State == CatalogOperatorStateRetired {
		return CatalogAdminMutationResult{}, ErrCatalogAdminRequest
	}
	models := cloneCatalogModels(active.Models)
	if !setCatalogOperatorPolicy(models, request.ModelID, request.State, strings.TrimSpace(request.Reason), request.ExpectedVersion+1) {
		return CatalogAdminMutationResult{}, fmt.Errorf("%w: id=%d", ErrCatalogModelNotFound, request.ModelID)
	}
	stage, err := buildAdminCatalogStage(models, "admin_operator_state", request.ActorUserID, strings.TrimSpace(request.Reason))
	if err != nil {
		return CatalogAdminMutationResult{}, err
	}
	publication, err := s.adminRepo.ApplyRevisionMutation(ctx, CatalogAdminRevisionMutationRequest{
		Scope:                   s.scope,
		Stage:                   stage,
		ModelID:                 request.ModelID,
		ExpectedOperatorVersion: request.ExpectedVersion,
		OperatorState:           &request.State,
		ExpectedEpoch:           active.Epoch,
		ExpectedRevisionID:      active.RevisionID,
		ActorUserID:             request.ActorUserID,
		Reason:                  strings.TrimSpace(request.Reason),
		Action:                  "admin_operator_state",
		RequestID:               request.RequestID,
		CorrelationID:           request.CorrelationID,
	})
	return s.finishAdminMutation(ctx, publication, err)
}

func (s *ModelCatalogAdminService) UpdatePricing(ctx context.Context, request CatalogAdminPricingUpdateRequest) (CatalogAdminMutationResult, error) {
	if s == nil || s.repository == nil || s.adminRepo == nil {
		return CatalogAdminMutationResult{}, ErrCatalogUnavailable
	}
	if request.ModelID <= 0 || request.ExpectedEpoch <= 0 || request.ExpectedRevisionID <= 0 || strings.TrimSpace(request.ExpectedSourceHash) == "" {
		return CatalogAdminMutationResult{}, ErrCatalogAdminRequest
	}
	if request.InputCostPerToken == nil && request.OutputCostPerToken == nil && request.CacheReadCostPerToken == nil && request.CacheWriteCostPerToken == nil {
		return CatalogAdminMutationResult{}, ErrCatalogAdminRequest
	}
	for _, value := range []*float64{request.InputCostPerToken, request.OutputCostPerToken, request.CacheReadCostPerToken, request.CacheWriteCostPerToken} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0) {
			return CatalogAdminMutationResult{}, ErrCatalogAdminRequest
		}
	}
	active, err := s.repository.LoadActiveSnapshot(ctx, s.scope)
	if err != nil {
		return CatalogAdminMutationResult{}, err
	}
	if active.Epoch != request.ExpectedEpoch || active.RevisionID != request.ExpectedRevisionID {
		return CatalogAdminMutationResult{}, ErrCatalogPublicationConflict
	}
	states, err := s.adminRepo.ListOperatorStates(ctx, catalogModelIDs(active.Models))
	if err != nil {
		return CatalogAdminMutationResult{}, err
	}
	state, ok := states[request.ModelID]
	if !ok {
		return CatalogAdminMutationResult{}, fmt.Errorf("%w: id=%d", ErrCatalogModelNotFound, request.ModelID)
	}
	if state.State == CatalogOperatorStateRetired {
		return CatalogAdminMutationResult{}, ErrCatalogAdminRequest
	}
	models := cloneCatalogModels(active.Models)
	var target *CatalogSnapshotModelSpec
	for index := range models {
		if models[index].ID == request.ModelID {
			target = &models[index]
			break
		}
	}
	if target == nil {
		return CatalogAdminMutationResult{}, fmt.Errorf("%w: id=%d", ErrCatalogModelNotFound, request.ModelID)
	}
	if target.SourceHash != request.ExpectedSourceHash || target.SourceState != CatalogSourceStatePresent {
		return CatalogAdminMutationResult{}, ErrCatalogPublicationConflict
	}
	if target.Pricing == nil {
		target.Pricing = &LiteLLMModelPricing{}
	}
	if request.InputCostPerToken != nil {
		target.Pricing.InputCostPerToken = *request.InputCostPerToken
	}
	if request.OutputCostPerToken != nil {
		target.Pricing.OutputCostPerToken = *request.OutputCostPerToken
	}
	if request.CacheReadCostPerToken != nil {
		target.Pricing.CacheReadInputTokenCost = *request.CacheReadCostPerToken
	}
	if request.CacheWriteCostPerToken != nil {
		target.Pricing.CacheCreationInputTokenCost = *request.CacheWriteCostPerToken
	}
	pricingJSON, err := json.Marshal(target.Pricing)
	if err != nil {
		return CatalogAdminMutationResult{}, fmt.Errorf("marshal admin pricing: %w", err)
	}
	target.PricingValid = true
	target.PricingSource = AdminPricingCatalogSource
	hashInput := append([]byte(target.CanonicalKey+"\x00"), pricingJSON...)
	sourceHash := sha256.Sum256(hashInput)
	target.SourceHash = hex.EncodeToString(sourceHash[:])
	target.OperatorState = CatalogOperatorState(strings.TrimSpace(string(target.OperatorState)))
	target.OperatorVersion = state.OperatorVersion
	stage, err := buildAdminCatalogStage(models, "admin_pricing_update", request.ActorUserID, strings.TrimSpace(request.Reason))
	if err != nil {
		return CatalogAdminMutationResult{}, err
	}
	publication, err := s.adminRepo.ApplyRevisionMutation(ctx, CatalogAdminRevisionMutationRequest{
		Scope:                   s.scope,
		Stage:                   stage,
		ModelID:                 request.ModelID,
		ExpectedOperatorVersion: state.OperatorVersion,
		ExpectedEpoch:           active.Epoch,
		ExpectedRevisionID:      active.RevisionID,
		ActorUserID:             request.ActorUserID,
		Reason:                  strings.TrimSpace(request.Reason),
		Action:                  "admin_pricing_update",
		RequestID:               request.RequestID,
		CorrelationID:           request.CorrelationID,
	})
	return s.finishAdminMutation(ctx, publication, err)
}

func (s *ModelCatalogAdminService) finishAdminMutation(ctx context.Context, publication CatalogPublicationRecord, err error) (CatalogAdminMutationResult, error) {
	if err != nil {
		return CatalogAdminMutationResult{}, err
	}
	reloaded := s.runtime != nil && s.runtime.ReloadPublished(ctx) == nil
	snapshot, err := s.List(ctx)
	if err != nil {
		return CatalogAdminMutationResult{}, err
	}
	if snapshot.RevisionID != publication.CatalogRevisionID || snapshot.Epoch != publication.Epoch {
		return CatalogAdminMutationResult{}, fmt.Errorf("%w: runtime publication mismatch", ErrCatalogInvalidSnapshot)
	}
	return CatalogAdminMutationResult{Snapshot: snapshot, RuntimeReloaded: reloaded}, nil
}

func buildAdminCatalogStage(models []CatalogSnapshotModelSpec, action string, actorID int64, reason string) (CatalogRevisionStage, error) {
	payload, err := json.Marshal(models)
	if err != nil {
		return CatalogRevisionStage{}, fmt.Errorf("marshal admin catalog revision: %w", err)
	}
	digest := sha256.Sum256(payload)
	var actor *int64
	if actorID > 0 {
		actor = &actorID
	}
	return CatalogRevisionStage{
		Revision:       1,
		NormalizedHash: hex.EncodeToString(digest[:]),
		Normalizer:     CatalogNormalizerVersion,
		SyncRun: CatalogSyncRunSpec{
			SourceSet: AdminPricingCatalogSource, Trigger: action, ActorUserID: actor,
			Normalizer: CatalogNormalizerVersion, SourceCount: len(models), NormalizedCount: len(models), ChangedCount: 1,
		},
		Models: models,
	}, nil
}

func setCatalogOperatorPolicy(models []CatalogSnapshotModelSpec, modelID int64, state CatalogOperatorState, reason string, version int64) bool {
	for index := range models {
		if models[index].ID == modelID {
			models[index].OperatorState = state
			models[index].OperatorReason = reason
			models[index].OperatorVersion = version
			return true
		}
	}
	return false
}

func catalogModelIDs(models []CatalogSnapshotModelSpec) []int64 {
	ids := make([]int64, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

func cloneCatalogPricing(pricing *LiteLLMModelPricing) *LiteLLMModelPricing {
	if pricing == nil {
		return nil
	}
	copy := *pricing
	return &copy
}

func cloneCatalogModels(models []CatalogSnapshotModelSpec) []CatalogSnapshotModelSpec {
	cloned := make([]CatalogSnapshotModelSpec, len(models))
	for index, model := range models {
		cloned[index] = model
		cloned[index].Pricing = cloneCatalogPricing(model.Pricing)
		cloned[index].Capabilities = append([]byte(nil), model.Capabilities...)
		cloned[index].SourceMetadata = append([]byte(nil), model.SourceMetadata...)
		cloned[index].Aliases = append([]CatalogAliasSpec(nil), model.Aliases...)
	}
	return cloned
}
