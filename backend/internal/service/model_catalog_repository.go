package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrCatalogNoPublication        = ErrCatalogUnavailable
	ErrCatalogRevisionNotValidated = errors.New("catalog revision is not validated")
	ErrCatalogPublicationConflict  = errors.New("catalog publication fencing conflict")
)

type CatalogSyncRunSpec struct {
	SourceSet       string
	Trigger         string
	ActorUserID     *int64
	UpstreamVersion string
	UpstreamETag    string
	UpstreamHash    string
	Normalizer      string
	SourceCount     int
	NormalizedCount int
	AddedCount      int
	ChangedCount    int
	MissingCount    int
	InvalidCount    int
}

type CatalogRevisionStage struct {
	Revision       int64
	NormalizedHash string
	Normalizer     string
	SyncRun        CatalogSyncRunSpec
	Models         []CatalogSnapshotModelSpec
}

type CatalogPublishRequest struct {
	Scope              string
	CatalogRevisionID  int64
	ExpectedEpoch      int64
	ExpectedRevisionID int64
	ActorType          string
	ActorUserID        *int64
	Reason             string
	RequestID          string
	CorrelationID      string
}

type CatalogPublicationRecord struct {
	Scope             string
	CatalogRevisionID int64
	Epoch             int64
	UpdatedAt         time.Time
}

type CatalogOutboxEvent struct {
	ID                int64
	EventType         string
	Scope             string
	PublicationEpoch  int64
	CatalogRevisionID int64
	ModelID           *int64
	Payload           json.RawMessage
	CreatedAt         time.Time
}

// ModelCatalogRepository is used only by the background sync/projection plane.
// It intentionally has no method suitable for normal request admission or
// pricing reads; those use ModelCatalogReader and a pinned CatalogReadView.
type ModelCatalogRepository interface {
	NextRevision(ctx context.Context, scope string) (int64, error)
	StageRevision(ctx context.Context, stage CatalogRevisionStage) (int64, error)
	ValidateRevision(ctx context.Context, catalogRevisionID int64) error
	PublishRevision(ctx context.Context, request CatalogPublishRequest) (CatalogPublicationRecord, error)
	LoadActiveSnapshot(ctx context.Context, scope string) (CatalogSnapshotSpec, error)
	ListOutboxAfter(ctx context.Context, scope string, afterID int64, limit int) ([]CatalogOutboxEvent, error)
}

// ModelCatalogAdminRepository is the control-plane repository used by the
// admin model-management surface. It is deliberately separate from the
// background projection repository so existing projection fakes and request
// path contracts do not grow admin-only methods.
type ModelCatalogAdminRepository interface {
	ListOperatorStates(ctx context.Context, modelIDs []int64) (map[int64]CatalogOperatorStateRecord, error)
	ApplyRevisionMutation(ctx context.Context, request CatalogAdminRevisionMutationRequest) (CatalogPublicationRecord, error)
}

type CatalogAdminRevisionMutationRequest struct {
	Scope                   string
	Stage                   CatalogRevisionStage
	ModelID                 int64
	ExpectedOperatorVersion int64
	OperatorState           *CatalogOperatorState
	OperatorStateUpdates    []CatalogOperatorStateMutation
	ExpectedEpoch           int64
	ExpectedRevisionID      int64
	ActorUserID             int64
	Reason                  string
	Action                  string
	RequestID               string
	CorrelationID           string
}

type CatalogOperatorStateRecord struct {
	ModelID         int64
	State           CatalogOperatorState
	Reason          string
	OperatorVersion int64
}

type CatalogOperatorStateMutation struct {
	ModelID         int64
	ExpectedVersion int64
	State           CatalogOperatorState
}

type CatalogOperatorStateBulkUpdateRequest struct {
	ExpectedEpoch      int64
	ExpectedRevisionID int64
	Updates            []CatalogOperatorStateMutation
	Reason             string
	ActorUserID        int64
	RequestID          string
	CorrelationID      string
}

type CatalogOperatorStateUpdateRequest struct {
	ModelID         int64
	ExpectedVersion int64
	State           CatalogOperatorState
	Reason          string
	ActorUserID     int64
	RequestID       string
	CorrelationID   string
}

var (
	ErrCatalogOperatorConflict = errors.New("catalog operator state conflict")
)
