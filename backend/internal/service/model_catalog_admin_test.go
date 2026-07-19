package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type adminCatalogRepositoryFake struct {
	spec CatalogSnapshotSpec
}

func (f *adminCatalogRepositoryFake) NextRevision(context.Context, string) (int64, error) {
	return f.spec.Revision + 1, nil
}
func (f *adminCatalogRepositoryFake) StageRevision(context.Context, CatalogRevisionStage) (int64, error) {
	return 0, errors.New("unexpected StageRevision")
}
func (f *adminCatalogRepositoryFake) ValidateRevision(context.Context, int64) error {
	return errors.New("unexpected ValidateRevision")
}
func (f *adminCatalogRepositoryFake) PublishRevision(context.Context, CatalogPublishRequest) (CatalogPublicationRecord, error) {
	return CatalogPublicationRecord{}, errors.New("unexpected PublishRevision")
}
func (f *adminCatalogRepositoryFake) MarkRevisionFailed(context.Context, int64, string) error {
	return errors.New("unexpected MarkRevisionFailed")
}
func (f *adminCatalogRepositoryFake) LoadActiveSnapshot(context.Context, string) (CatalogSnapshotSpec, error) {
	return f.spec, nil
}
func (f *adminCatalogRepositoryFake) ListOutboxAfter(context.Context, string, int64, int) ([]CatalogOutboxEvent, error) {
	return nil, nil
}
func (f *adminCatalogRepositoryFake) LatestOutboxCursor(context.Context, string) (int64, error) {
	return 0, nil
}

// Keep the fake strict enough to prove the admin service sends the complete
// transaction contract to persistence rather than mutating the runtime reader.
type adminCatalogMutationRepositoryFake struct {
	catalog *adminCatalogRepositoryFake
	states  map[int64]CatalogOperatorStateRecord
	request CatalogAdminRevisionMutationRequest
	calls   int
}

func (f *adminCatalogMutationRepositoryFake) ListOperatorStates(_ context.Context, _ []int64) (map[int64]CatalogOperatorStateRecord, error) {
	out := make(map[int64]CatalogOperatorStateRecord, len(f.states))
	for id, state := range f.states {
		out[id] = state
	}
	return out, nil
}

func (f *adminCatalogMutationRepositoryFake) ApplyRevisionMutation(_ context.Context, request CatalogAdminRevisionMutationRequest) (CatalogPublicationRecord, error) {
	f.calls++
	f.request = request
	f.catalog.spec.Models = request.Stage.Models
	f.catalog.spec.Epoch = request.ExpectedEpoch + 1
	f.catalog.spec.RevisionID++
	f.catalog.spec.Revision++
	f.catalog.spec.Checksum = request.Stage.NormalizedHash
	f.catalog.spec.PublishedAt = time.Date(2026, 7, 19, 13, 0, 0, 0, time.UTC)
	if len(request.OperatorStateUpdates) == 0 && request.OperatorState != nil {
		state := f.states[request.ModelID]
		state.State = *request.OperatorState
		state.Reason = request.Reason
		state.OperatorVersion++
		f.states[request.ModelID] = state
	}
	for _, update := range request.OperatorStateUpdates {
		state := f.states[update.ModelID]
		state.State = update.State
		state.Reason = request.Reason
		state.OperatorVersion++
		f.states[update.ModelID] = state
	}
	return CatalogPublicationRecord{Scope: request.Scope, CatalogRevisionID: f.catalog.spec.RevisionID, Epoch: f.catalog.spec.Epoch}, nil
}

func TestModelCatalogAdminUpdateOperatorStateBuildsImmutableRevisionMutation(t *testing.T) {
	repository, adminRepository := newAdminCatalogTestRepositories()
	serviceUnderTest := NewModelCatalogAdminService(repository, adminRepository, nil, nil, CatalogScopeGlobal)

	result, err := serviceUnderTest.UpdateOperatorState(context.Background(), CatalogOperatorStateUpdateRequest{
		ModelID: 1, ExpectedVersion: 2, State: CatalogOperatorStateDisabled, Reason: "provider maintenance", ActorUserID: 42,
	})
	require.NoError(t, err)
	require.Equal(t, 1, adminRepository.calls)
	require.Equal(t, int64(1), adminRepository.request.ModelID)
	require.Equal(t, int64(2), adminRepository.request.ExpectedOperatorVersion)
	require.Equal(t, "admin_operator_state", adminRepository.request.Action)
	require.Equal(t, "provider maintenance", adminRepository.request.Reason)
	require.NotNil(t, adminRepository.request.OperatorState)
	require.Equal(t, CatalogOperatorStateDisabled, *adminRepository.request.OperatorState)
	require.Equal(t, CatalogOperatorStateDisabled, adminRepository.request.Stage.Models[0].OperatorState)
	require.Equal(t, int64(3), adminRepository.request.Stage.Models[0].OperatorVersion)
	require.NotEmpty(t, adminRepository.request.Stage.NormalizedHash)
	require.Equal(t, CatalogOperatorStateDisabled, result.Snapshot.Models[0].OperatorState)
	require.False(t, result.RuntimeReloaded)
}

func TestModelCatalogAdminUpdateOperatorStatesPublishesOneAtomicRevision(t *testing.T) {
	repository, adminRepository := newAdminCatalogTestRepositories()
	repository.spec.Models = append(repository.spec.Models, CatalogSnapshotModelSpec{
		ID: 2, RevisionID: 9002, CanonicalKey: "gpt-5.6-mini", OperatorState: CatalogOperatorStateEnabled, OperatorVersion: 4,
		SourceState: CatalogSourceStatePresent, Provider: "openai", Platform: "openai", Mode: "chat", PricingSchemaVersion: 1,
		PricingValid: true, PricingSource: "legacy-lite-llm", SourceHash: "source-hash-2",
		Pricing: &LiteLLMModelPricing{InputCostPerToken: 0.000001, OutputCostPerToken: 0.000006},
	})
	adminRepository.states[2] = CatalogOperatorStateRecord{ModelID: 2, State: CatalogOperatorStateEnabled, OperatorVersion: 4}
	serviceUnderTest := NewModelCatalogAdminService(repository, adminRepository, nil, nil, CatalogScopeGlobal)

	result, err := serviceUnderTest.UpdateOperatorStates(context.Background(), CatalogOperatorStateBulkUpdateRequest{
		ExpectedEpoch: 7, ExpectedRevisionID: 701,
		Updates: []CatalogOperatorStateMutation{
			{ModelID: 1, ExpectedVersion: 2, State: CatalogOperatorStateDisabled},
			{ModelID: 2, ExpectedVersion: 4, State: CatalogOperatorStateDisabled},
		},
		Reason: "provider maintenance", ActorUserID: 42,
	})
	require.NoError(t, err)
	require.Equal(t, 1, adminRepository.calls)
	require.Len(t, adminRepository.request.OperatorStateUpdates, 2)
	require.Equal(t, "admin_operator_state_bulk", adminRepository.request.Action)
	require.Equal(t, "provider maintenance", adminRepository.request.Reason)
	require.Equal(t, CatalogOperatorStateDisabled, adminRepository.request.Stage.Models[0].OperatorState)
	require.Equal(t, int64(3), adminRepository.request.Stage.Models[0].OperatorVersion)
	require.Equal(t, CatalogOperatorStateDisabled, adminRepository.request.Stage.Models[1].OperatorState)
	require.Equal(t, int64(5), adminRepository.request.Stage.Models[1].OperatorVersion)
	require.Equal(t, CatalogOperatorStateDisabled, result.Snapshot.Models[0].OperatorState)
	require.Equal(t, CatalogOperatorStateDisabled, result.Snapshot.Models[1].OperatorState)
}

func TestModelCatalogAdminUpdatePricingRejectsStaleSourceHashBeforePersistence(t *testing.T) {
	repository, adminRepository := newAdminCatalogTestRepositories()
	serviceUnderTest := NewModelCatalogAdminService(repository, adminRepository, nil, nil, CatalogScopeGlobal)
	input := 0.000006

	_, err := serviceUnderTest.UpdatePricing(context.Background(), CatalogAdminPricingUpdateRequest{
		ModelID: 1, ExpectedEpoch: 7, ExpectedRevisionID: 701, ExpectedSourceHash: "stale", InputCostPerToken: &input, ActorUserID: 42,
	})
	require.ErrorIs(t, err, ErrCatalogPublicationConflict)
	require.Zero(t, adminRepository.calls)
}

func TestModelCatalogAdminUpdatePricingRejectsNegativePrice(t *testing.T) {
	repository, adminRepository := newAdminCatalogTestRepositories()
	serviceUnderTest := NewModelCatalogAdminService(repository, adminRepository, nil, nil, CatalogScopeGlobal)
	negative := -1.0

	_, err := serviceUnderTest.UpdatePricing(context.Background(), CatalogAdminPricingUpdateRequest{
		ModelID: 1, ExpectedEpoch: 7, ExpectedRevisionID: 701, ExpectedSourceHash: "source-hash-1", InputCostPerToken: &negative, ActorUserID: 42,
	})
	require.ErrorIs(t, err, ErrCatalogAdminRequest)
	require.Zero(t, adminRepository.calls)
}

func TestModelCatalogAdminRejectsMutationsForRetiredModel(t *testing.T) {
	repository, adminRepository := newAdminCatalogTestRepositories()
	adminRepository.states[1] = CatalogOperatorStateRecord{ModelID: 1, State: CatalogOperatorStateRetired, OperatorVersion: 2}
	serviceUnderTest := NewModelCatalogAdminService(repository, adminRepository, nil, nil, CatalogScopeGlobal)

	_, err := serviceUnderTest.UpdateOperatorState(context.Background(), CatalogOperatorStateUpdateRequest{
		ModelID: 1, ExpectedVersion: 2, State: CatalogOperatorStateEnabled, Reason: "re-enable retired model",
	})
	require.ErrorIs(t, err, ErrCatalogAdminRequest)

	input := 0.000006
	_, err = serviceUnderTest.UpdatePricing(context.Background(), CatalogAdminPricingUpdateRequest{
		ModelID: 1, ExpectedEpoch: 7, ExpectedRevisionID: 701, ExpectedSourceHash: "source-hash-1", InputCostPerToken: &input,
	})
	require.ErrorIs(t, err, ErrCatalogAdminRequest)
	require.Zero(t, adminRepository.calls)
}

func newAdminCatalogTestRepositories() (*adminCatalogRepositoryFake, *adminCatalogMutationRepositoryFake) {
	repository := &adminCatalogRepositoryFake{spec: CatalogSnapshotSpec{
		Scope: CatalogScopeGlobal, Epoch: 7, RevisionID: 701, Revision: 7, Checksum: "catalog-hash-7", PublishedAt: time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC),
		Models: []CatalogSnapshotModelSpec{{
			ID: 1, RevisionID: 9001, CanonicalKey: "gpt-5.6-sol", OperatorState: CatalogOperatorStateEnabled, OperatorVersion: 2,
			SourceState: CatalogSourceStatePresent, Provider: "openai", Platform: "openai", Mode: "chat", PricingSchemaVersion: 1,
			PricingValid: true, PricingSource: "legacy-lite-llm", SourceHash: "source-hash-1",
			Pricing: &LiteLLMModelPricing{InputCostPerToken: 0.000005, OutputCostPerToken: 0.00003},
		}},
	}}
	adminRepository := &adminCatalogMutationRepositoryFake{catalog: repository, states: map[int64]CatalogOperatorStateRecord{
		1: {ModelID: 1, State: CatalogOperatorStateEnabled, OperatorVersion: 2},
	}}
	return repository, adminRepository
}
