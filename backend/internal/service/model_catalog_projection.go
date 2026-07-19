package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ModelCatalogProjectionService owns the background-only transition from the
// legacy pricing catalog to the durable catalog and then to an atomic runtime
// snapshot. It is intentionally not called from request admission.
type ModelCatalogProjectionService struct {
	repository ModelCatalogRepository
	pricing    *PricingService
	reader     *AtomicModelCatalogReader
	scope      string
	refreshMu  sync.Mutex
}

func NewModelCatalogProjectionService(repository ModelCatalogRepository, pricing *PricingService, reader *AtomicModelCatalogReader, scope string) (*ModelCatalogProjectionService, error) {
	if repository == nil || pricing == nil || reader == nil {
		return nil, errors.New("catalog projection dependencies are required")
	}
	if scope = normalizeCatalogScope(scope); scope == "" {
		return nil, errors.New("catalog projection scope is required")
	}
	return &ModelCatalogProjectionService{
		repository: repository,
		pricing:    pricing,
		reader:     reader,
		scope:      scope,
	}, nil
}

// Bootstrap loads the currently published durable projection. An empty DB is
// a valid pre-publication state during the additive rollout.
func (p *ModelCatalogProjectionService) Bootstrap(ctx context.Context) error {
	if p == nil {
		return ErrCatalogUnavailable
	}
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()
	return p.bootstrap(ctx)
}

func (p *ModelCatalogProjectionService) bootstrap(ctx context.Context) error {
	spec, err := p.repository.LoadActiveSnapshot(ctx, p.scope)
	if err != nil {
		if errors.Is(err, ErrCatalogNoPublication) {
			return nil
		}
		return fmt.Errorf("bootstrap catalog projection: %w", err)
	}
	if err := p.publishLoadedSpec(spec); err != nil {
		return err
	}
	p.reader.MarkVerified(spec.Epoch, time.Now().UTC())
	return nil
}

// Refresh imports the legacy map into a staged revision and publishes it only
// after validation. The active database pointer is the publication authority;
// the process-local reader is updated only from the post-publish projection.
func (p *ModelCatalogProjectionService) Refresh(ctx context.Context) error {
	if p == nil {
		return ErrCatalogUnavailable
	}
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()
	return p.refresh(ctx)
}

func (p *ModelCatalogProjectionService) refresh(ctx context.Context) error {
	if err := p.bootstrap(ctx); err != nil {
		return err
	}
	revision, err := p.repository.NextRevision(ctx, p.scope)
	if err != nil {
		return fmt.Errorf("allocate catalog revision: %w", err)
	}
	stage, err := p.pricing.BuildCatalogRevisionStage(p.scope, revision)
	if err != nil {
		return fmt.Errorf("build catalog revision: %w", err)
	}
	catalogRevisionID, err := p.repository.StageRevision(ctx, stage)
	if err != nil {
		return fmt.Errorf("stage catalog revision: %w", err)
	}
	if err := p.repository.ValidateRevision(ctx, catalogRevisionID); err != nil {
		return fmt.Errorf("validate catalog revision %d: %w", catalogRevisionID, err)
	}
	expectedEpoch, expectedRevisionID := p.reader.CurrentIdentity()
	if expectedRevisionID == catalogRevisionID {
		return nil
	}
	publication, err := p.repository.PublishRevision(ctx, CatalogPublishRequest{
		Scope:              p.scope,
		CatalogRevisionID:  catalogRevisionID,
		ExpectedEpoch:      expectedEpoch,
		ExpectedRevisionID: expectedRevisionID,
		ActorType:          "system",
		Reason:             "legacy pricing shadow projection",
	})
	if err != nil {
		return fmt.Errorf("publish catalog revision %d: %w", catalogRevisionID, err)
	}
	spec, err := p.repository.LoadActiveSnapshot(ctx, p.scope)
	if err != nil {
		return fmt.Errorf("load published catalog revision %d: %w", catalogRevisionID, err)
	}
	if spec.Epoch != publication.Epoch || spec.RevisionID != publication.CatalogRevisionID {
		return fmt.Errorf("%w: publication epoch/revision mismatch", ErrCatalogInvalidSnapshot)
	}
	return p.publishLoadedSpec(spec)
}

func (p *ModelCatalogProjectionService) publishLoadedSpec(spec CatalogSnapshotSpec) error {
	snapshot, err := NewCatalogRuntimeSnapshot(spec)
	if err != nil {
		return fmt.Errorf("build runtime catalog snapshot: %w", err)
	}
	return p.reader.Publish(snapshot)
}
