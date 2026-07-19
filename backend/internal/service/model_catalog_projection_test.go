package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeModelCatalogRepository struct {
	pricing       CatalogSnapshotSpec
	loadCalls     int
	loadNotified  chan struct{}
	stageCalls    int
	nextRevision  int64
	staged        CatalogRevisionStage
	stagedID      int64
	validatedID   int64
	published     CatalogPublicationRecord
	noPublication bool
}

func (f *fakeModelCatalogRepository) NextRevision(context.Context, string) (int64, error) {
	return f.nextRevision, nil
}

func (f *fakeModelCatalogRepository) StageRevision(_ context.Context, stage CatalogRevisionStage) (int64, error) {
	f.stageCalls++
	f.staged = stage
	return f.stagedID, nil
}

func (f *fakeModelCatalogRepository) ValidateRevision(_ context.Context, id int64) error {
	f.validatedID = id
	return nil
}

func (f *fakeModelCatalogRepository) PublishRevision(_ context.Context, request CatalogPublishRequest) (CatalogPublicationRecord, error) {
	if request.ExpectedEpoch != 0 || request.ExpectedRevisionID != 0 {
		return CatalogPublicationRecord{}, errors.New("unexpected non-empty initial fence")
	}
	return f.published, nil
}

func (f *fakeModelCatalogRepository) LoadActiveSnapshot(context.Context, string) (CatalogSnapshotSpec, error) {
	f.loadCalls++
	if f.loadNotified != nil {
		select {
		case f.loadNotified <- struct{}{}:
		default:
		}
	}
	if f.noPublication {
		f.noPublication = false
		return CatalogSnapshotSpec{}, ErrCatalogNoPublication
	}
	return f.pricing, nil
}

func (f *fakeModelCatalogRepository) ListOutboxAfter(context.Context, string, int64, int) ([]CatalogOutboxEvent, error) {
	return nil, nil
}

func TestModelCatalogProjectionRuntime_BootstrapsAdmissionWithoutImport(t *testing.T) {
	now := time.Date(2026, 7, 18, 14, 0, 0, 0, time.UTC)
	repository := &fakeModelCatalogRepository{
		pricing: catalogSnapshotFixture(7, 701, now, 5e-6),
	}
	reader := NewAtomicModelCatalogReader(nil, time.Hour)
	projection, err := NewModelCatalogProjectionService(repository, &PricingService{}, reader, CatalogScopeGlobal)
	require.NoError(t, err)
	runtime := NewModelCatalogProjectionRuntimeWithModes(projection, reader, "shadow", time.Hour, ModelCatalogReadModes{
		ImportMode:    "off",
		AdmissionMode: "observe",
	})

	runtime.Start()
	defer runtime.Stop()

	require.Equal(t, 1, repository.loadCalls)
	epoch, revisionID := reader.CurrentIdentity()
	require.Equal(t, int64(7), epoch)
	require.Equal(t, int64(700), revisionID)
	require.Zero(t, repository.stageCalls)
	view, err := reader.BeginDecision(now)
	require.NoError(t, err)
	model, ok := view.Resolve("gpt-5.6", PlatformOpenAI)
	require.True(t, ok)
	require.Equal(t, int64(701), model.RevisionID)
}

func TestModelCatalogProjectionBootstrapMarksLoadedEpochVerified(t *testing.T) {
	verifiedAt := time.Now().UTC().Add(-2 * time.Hour)
	repository := &fakeModelCatalogRepository{
		pricing: catalogSnapshotFixture(7, 701, verifiedAt, 5e-6),
	}
	reader := NewAtomicModelCatalogReader(nil, time.Hour)
	projection, err := NewModelCatalogProjectionService(repository, &PricingService{}, reader, CatalogScopeGlobal)
	require.NoError(t, err)

	require.NoError(t, projection.Bootstrap(context.Background()))

	_, err = reader.BeginDecision(time.Now().UTC())
	require.NoError(t, err)
}

func TestModelCatalogProjectionRuntimeRevalidatesPublishedSnapshotWithoutImport(t *testing.T) {
	loadNotified := make(chan struct{}, 4)
	repository := &fakeModelCatalogRepository{
		pricing:      catalogSnapshotFixture(7, 701, time.Now().UTC().Add(-2*time.Hour), 5e-6),
		loadNotified: loadNotified,
	}
	reader := NewAtomicModelCatalogReader(nil, 100*time.Millisecond)
	projection, err := NewModelCatalogProjectionService(repository, &PricingService{}, reader, CatalogScopeGlobal)
	require.NoError(t, err)
	runtime := NewModelCatalogProjectionRuntimeWithModes(projection, reader, "shadow", 10*time.Millisecond, ModelCatalogReadModes{
		ImportMode:    "off",
		AdmissionMode: "observe",
	})

	runtime.Start()
	defer runtime.Stop()

	select {
	case <-loadNotified:
	case <-time.After(time.Second):
		t.Fatal("initial published snapshot load was not observed")
	}
	select {
	case <-loadNotified:
	case <-time.After(time.Second):
		t.Fatal("periodic published snapshot revalidation was not observed")
	}
	time.Sleep(150 * time.Millisecond)
	runtime.Stop()

	require.GreaterOrEqual(t, repository.loadCalls, 2)
	require.Zero(t, repository.stageCalls)
	_, err = reader.BeginDecision(time.Now().UTC())
	require.NoError(t, err)
}

func TestModelCatalogProjectionRefreshStagesValidatesPublishesAndSwapsOnlyAfterLoad(t *testing.T) {
	now := time.Date(2026, 7, 18, 13, 0, 0, 0, time.UTC)
	pricing := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-5.6": {InputCostPerToken: 5e-6, OutputCostPerToken: 3e-5, LiteLLMProvider: "openai", Mode: "chat"},
	}}
	repository := &fakeModelCatalogRepository{
		nextRevision:  1,
		stagedID:      701,
		published:     CatalogPublicationRecord{Scope: CatalogScopeGlobal, CatalogRevisionID: 701, Epoch: 1},
		noPublication: true,
	}
	reader := NewAtomicModelCatalogReader(nil, time.Hour)
	projection, err := NewModelCatalogProjectionService(repository, pricing, reader, CatalogScopeGlobal)
	require.NoError(t, err)

	stage, err := pricing.BuildCatalogRevisionStage(CatalogScopeGlobal, 1)
	require.NoError(t, err)
	repository.pricing = CatalogSnapshotSpec{
		Scope:      CatalogScopeGlobal,
		Epoch:      1,
		RevisionID: 701,
		Revision:   stage.Revision,
		Checksum:   stage.NormalizedHash,
		VerifiedAt: now,
		Models:     stage.Models,
	}

	err = projection.Refresh(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(701), repository.validatedID)
	epoch, revisionID := reader.CurrentIdentity()
	require.Equal(t, int64(1), epoch)
	require.Equal(t, int64(701), revisionID)
	view, err := reader.BeginDecision(now.Add(time.Minute))
	require.NoError(t, err)
	decision, ok := view.Resolve("gpt-5.6", "openai")
	require.True(t, ok)
	require.Equal(t, "gpt-5.6", decision.CanonicalKey)
}
