package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	CatalogScopeGlobal      = "global"
	CatalogPlatformScopeAny = "*"
)

var (
	ErrCatalogUnavailable     = errors.New("model catalog unavailable")
	ErrCatalogInvalidSnapshot = errors.New("invalid model catalog snapshot")
	ErrCatalogDuplicateLookup = errors.New("duplicate model catalog lookup")
	ErrCatalogEpochRegression = errors.New("model catalog epoch regression")
	ErrCatalogEpochConflict   = errors.New("model catalog epoch conflict")
	ErrCatalogKnownStale      = errors.New("model catalog snapshot is known stale")
	ErrCatalogSnapshotStale   = errors.New("model catalog snapshot exceeded max stale duration")
)

type CatalogOperatorState string

const (
	CatalogOperatorStateEnabled  CatalogOperatorState = "enabled"
	CatalogOperatorStateDisabled CatalogOperatorState = "disabled"
	CatalogOperatorStateRetired  CatalogOperatorState = "retired"
)

type CatalogSourceState string

const (
	CatalogSourceStatePresent CatalogSourceState = "present"
	CatalogSourceStateMissing CatalogSourceState = "missing"
	CatalogSourceStateInvalid CatalogSourceState = "invalid"
)

type CatalogLifecycle string

const (
	CatalogLifecycleActive      CatalogLifecycle = "active"
	CatalogLifecycleDisabled    CatalogLifecycle = "disabled"
	CatalogLifecycleRetired     CatalogLifecycle = "retired"
	CatalogLifecycleUnavailable CatalogLifecycle = "unavailable"
)

// CatalogAliasSpec is a normalized lookup alias included in one immutable model
// revision. PlatformScope defaults to "*" when omitted.
type CatalogAliasSpec struct {
	Alias         string
	PlatformScope string
}

// CatalogSnapshotModelSpec is the validated input used by the background
// projection loader. NewCatalogRuntimeSnapshot copies every field required by
// request-time decisions so callers cannot mutate the published view.
type CatalogSnapshotModelSpec struct {
	ID                   int64
	RevisionID           int64
	CanonicalKey         string
	OperatorState        CatalogOperatorState
	SourceState          CatalogSourceState
	Provider             string
	Platform             string
	Mode                 string
	ContextWindow        int64
	MaxOutputTokens      int64
	PricingSchemaVersion int
	PricingValid         bool
	PricingSource        string
	Pricing              *LiteLLMModelPricing
	SourceHash           string
	Capabilities         json.RawMessage
	SourceMetadata       json.RawMessage
	Aliases              []CatalogAliasSpec
}

// CatalogSnapshotSpec describes one complete active publication. It is built
// and validated outside the request path.
type CatalogSnapshotSpec struct {
	Scope       string
	Epoch       int64
	RevisionID  int64
	Revision    int64
	Checksum    string
	PublishedAt time.Time
	VerifiedAt  time.Time
	Models      []CatalogSnapshotModelSpec
}

type catalogSnapshotModel struct {
	id                   int64
	revisionID           int64
	canonicalKey         string
	operatorState        CatalogOperatorState
	sourceState          CatalogSourceState
	provider             string
	platform             string
	mode                 string
	contextWindow        int64
	maxOutputTokens      int64
	pricingSchemaVersion int
	pricingValid         bool
	pricingSource        string
	pricing              LiteLLMModelPricing
	sourceHash           string
	capabilities         json.RawMessage
	sourceMetadata       json.RawMessage
}

// CatalogRuntimeSnapshot is immutable after construction. Its maps and model
// records are intentionally private; requests can only obtain value copies
// through a CatalogReadView.
type CatalogRuntimeSnapshot struct {
	scope       string
	epoch       int64
	revisionID  int64
	revision    int64
	checksum    string
	publishedAt time.Time
	verifiedAt  time.Time
	models      map[int64]catalogSnapshotModel
	lookup      map[string]map[string]int64
}

// CatalogModelDecision is a value copy safe for request-local use and
// settlement pinning. Mutating Pricing cannot mutate the active snapshot.
type CatalogModelDecision struct {
	ID                   int64
	RevisionID           int64
	CatalogRevisionID    int64
	CatalogRevision      int64
	CatalogEpoch         int64
	CanonicalKey         string
	OperatorState        CatalogOperatorState
	SourceState          CatalogSourceState
	Provider             string
	Platform             string
	Mode                 string
	ContextWindow        int64
	MaxOutputTokens      int64
	PricingSchemaVersion int
	PricingSource        string
	HasPricing           bool
	Pricing              LiteLLMModelPricing
	SourceHash           string
}

func (m CatalogModelDecision) Lifecycle() CatalogLifecycle {
	switch m.OperatorState {
	case CatalogOperatorStateRetired:
		return CatalogLifecycleRetired
	case CatalogOperatorStateDisabled:
		return CatalogLifecycleDisabled
	}
	if m.SourceState != CatalogSourceStatePresent {
		return CatalogLifecycleUnavailable
	}
	return CatalogLifecycleActive
}

// CatalogReadView pins one immutable publication for one admission/billing
// decision. It performs only process-local map lookups.
type CatalogReadView struct {
	snapshot *CatalogRuntimeSnapshot
}

func (v CatalogReadView) Scope() string {
	if v.snapshot == nil {
		return ""
	}
	return v.snapshot.scope
}

func (v CatalogReadView) Epoch() int64 {
	if v.snapshot == nil {
		return 0
	}
	return v.snapshot.epoch
}

func (v CatalogReadView) RevisionID() int64 {
	if v.snapshot == nil {
		return 0
	}
	return v.snapshot.revisionID
}

func (v CatalogReadView) Revision() int64 {
	if v.snapshot == nil {
		return 0
	}
	return v.snapshot.revision
}

func (v CatalogReadView) Checksum() string {
	if v.snapshot == nil {
		return ""
	}
	return v.snapshot.checksum
}

func (v CatalogReadView) Resolve(modelName, platform string) (CatalogModelDecision, bool) {
	if v.snapshot == nil {
		return CatalogModelDecision{}, false
	}
	modelKey := normalizeCatalogLookupValue(modelName)
	if modelKey == "" {
		return CatalogModelDecision{}, false
	}

	var modelID int64
	var ok bool
	platformKey := normalizeCatalogPlatformScope(platform)
	if platformKey != CatalogPlatformScopeAny {
		platformLookup := v.snapshot.lookup[platformKey]
		modelID, ok = platformLookup[modelKey]
	}
	if !ok {
		anyLookup := v.snapshot.lookup[CatalogPlatformScopeAny]
		modelID, ok = anyLookup[modelKey]
	}
	if !ok {
		return CatalogModelDecision{}, false
	}

	model, ok := v.snapshot.models[modelID]
	if !ok {
		return CatalogModelDecision{}, false
	}
	return CatalogModelDecision{
		ID:                   model.id,
		RevisionID:           model.revisionID,
		CatalogRevisionID:    v.snapshot.revisionID,
		CatalogRevision:      v.snapshot.revision,
		CatalogEpoch:         v.snapshot.epoch,
		CanonicalKey:         model.canonicalKey,
		OperatorState:        model.operatorState,
		SourceState:          model.sourceState,
		Provider:             model.provider,
		Platform:             model.platform,
		Mode:                 model.mode,
		ContextWindow:        model.contextWindow,
		MaxOutputTokens:      model.maxOutputTokens,
		PricingSchemaVersion: model.pricingSchemaVersion,
		PricingSource:        model.pricingSource,
		HasPricing:           model.pricingValid,
		Pricing:              model.pricing,
		SourceHash:           model.sourceHash,
	}, true
}

// ModelCatalogReader is the only catalog dependency request-path services
// should consume. The interface deliberately has no context or repository
// methods, preventing hidden SQL/Redis/network fallbacks in normal reads.
type ModelCatalogReader interface {
	BeginDecision(now time.Time) (CatalogReadView, error)
}

type catalogReaderFreshness struct {
	epoch         int64
	verifiedAt    time.Time
	observedEpoch int64
	observedAt    time.Time
}

// AtomicModelCatalogReader publishes complete snapshots with an atomic pointer.
// Request reads never acquire writerMu; only background publication/freshness
// updates serialize on it.
type AtomicModelCatalogReader struct {
	current   atomic.Pointer[CatalogRuntimeSnapshot]
	freshness atomic.Pointer[catalogReaderFreshness]
	maxStale  time.Duration
	writerMu  sync.Mutex
}

func NewAtomicModelCatalogReader(initial *CatalogRuntimeSnapshot, maxStale time.Duration) *AtomicModelCatalogReader {
	reader := &AtomicModelCatalogReader{maxStale: maxStale}
	if initial != nil {
		reader.current.Store(initial)
		reader.freshness.Store(&catalogReaderFreshness{
			epoch:         initial.epoch,
			verifiedAt:    initial.verifiedAt,
			observedEpoch: initial.epoch,
		})
	}
	return reader
}

func (r *AtomicModelCatalogReader) BeginDecision(now time.Time) (CatalogReadView, error) {
	if r == nil {
		return CatalogReadView{}, ErrCatalogUnavailable
	}
	snapshot := r.current.Load()
	if snapshot == nil {
		return CatalogReadView{}, ErrCatalogUnavailable
	}

	freshness := r.freshness.Load()
	verifiedAt := snapshot.verifiedAt
	if freshness != nil && freshness.epoch == snapshot.epoch {
		if freshness.observedEpoch > snapshot.epoch {
			return CatalogReadView{}, fmt.Errorf("%w: local=%d observed=%d", ErrCatalogKnownStale, snapshot.epoch, freshness.observedEpoch)
		}
		if freshness.verifiedAt.After(verifiedAt) {
			verifiedAt = freshness.verifiedAt
		}
	}
	if verifiedAt.IsZero() {
		return CatalogReadView{}, fmt.Errorf("%w: missing verified_at", ErrCatalogSnapshotStale)
	}
	if r.maxStale <= 0 || now.After(verifiedAt.Add(r.maxStale)) {
		return CatalogReadView{}, fmt.Errorf("%w: epoch=%d verified_at=%s", ErrCatalogSnapshotStale, snapshot.epoch, verifiedAt.UTC().Format(time.RFC3339Nano))
	}
	return snapshot.readView(), nil
}

// CurrentIdentity returns the currently published epoch and database revision.
// It is a background-coordination helper; request admission should only use
// BeginDecision and the returned pinned view.
func (r *AtomicModelCatalogReader) CurrentIdentity() (epoch, revisionID int64) {
	if r == nil {
		return 0, 0
	}
	snapshot := r.current.Load()
	if snapshot == nil {
		return 0, 0
	}
	return snapshot.epoch, snapshot.revisionID
}

func (r *AtomicModelCatalogReader) Publish(snapshot *CatalogRuntimeSnapshot) error {
	if r == nil || snapshot == nil {
		return ErrCatalogInvalidSnapshot
	}
	r.writerMu.Lock()
	defer r.writerMu.Unlock()

	current := r.current.Load()
	if current != nil {
		switch {
		case snapshot.epoch < current.epoch:
			return fmt.Errorf("%w: current=%d candidate=%d", ErrCatalogEpochRegression, current.epoch, snapshot.epoch)
		case snapshot.epoch == current.epoch:
			if snapshot.revisionID == current.revisionID && snapshot.checksum == current.checksum {
				return nil
			}
			return fmt.Errorf("%w: epoch=%d current_revision=%d candidate_revision=%d", ErrCatalogEpochConflict, snapshot.epoch, current.revisionID, snapshot.revisionID)
		}
	}

	observedEpoch := snapshot.epoch
	observedAt := time.Time{}
	if previous := r.freshness.Load(); previous != nil && previous.observedEpoch > observedEpoch {
		observedEpoch = previous.observedEpoch
		observedAt = previous.observedAt
	}
	r.current.Store(snapshot)
	r.freshness.Store(&catalogReaderFreshness{
		epoch:         snapshot.epoch,
		verifiedAt:    snapshot.verifiedAt,
		observedEpoch: observedEpoch,
		observedAt:    observedAt,
	})
	return nil
}

// ObserveEpoch records a durable publication epoch seen by a background poller.
// A request fails closed when that epoch is newer than its local snapshot.
func (r *AtomicModelCatalogReader) ObserveEpoch(epoch int64, observedAt time.Time) {
	if r == nil || epoch <= 0 {
		return
	}
	r.writerMu.Lock()
	defer r.writerMu.Unlock()

	freshness := cloneCatalogReaderFreshness(r.freshness.Load())
	if current := r.current.Load(); current != nil && freshness.epoch == 0 {
		freshness.epoch = current.epoch
		freshness.verifiedAt = current.verifiedAt
	}
	if epoch > freshness.observedEpoch {
		freshness.observedEpoch = epoch
		freshness.observedAt = observedAt
	}
	r.freshness.Store(freshness)
}

// MarkVerified refreshes the last-known-good age only when the background
// verifier confirms the exact currently published epoch.
func (r *AtomicModelCatalogReader) MarkVerified(epoch int64, verifiedAt time.Time) {
	if r == nil || epoch <= 0 || verifiedAt.IsZero() {
		return
	}
	r.writerMu.Lock()
	defer r.writerMu.Unlock()

	current := r.current.Load()
	if current == nil || current.epoch != epoch {
		return
	}
	freshness := cloneCatalogReaderFreshness(r.freshness.Load())
	freshness.epoch = epoch
	if verifiedAt.After(freshness.verifiedAt) {
		freshness.verifiedAt = verifiedAt
	}
	if freshness.observedEpoch < epoch {
		freshness.observedEpoch = epoch
	}
	r.freshness.Store(freshness)
}

func cloneCatalogReaderFreshness(value *catalogReaderFreshness) *catalogReaderFreshness {
	if value == nil {
		return &catalogReaderFreshness{}
	}
	copyValue := *value
	return &copyValue
}

func NewCatalogRuntimeSnapshot(spec CatalogSnapshotSpec) (*CatalogRuntimeSnapshot, error) {
	scope := normalizeCatalogScope(spec.Scope)
	if scope == "" || spec.Epoch <= 0 || spec.RevisionID <= 0 || spec.Revision <= 0 || strings.TrimSpace(spec.Checksum) == "" || spec.VerifiedAt.IsZero() {
		return nil, ErrCatalogInvalidSnapshot
	}
	if len(spec.Models) == 0 {
		return nil, fmt.Errorf("%w: empty model set", ErrCatalogInvalidSnapshot)
	}

	snapshot := &CatalogRuntimeSnapshot{
		scope:       scope,
		epoch:       spec.Epoch,
		revisionID:  spec.RevisionID,
		revision:    spec.Revision,
		checksum:    strings.TrimSpace(spec.Checksum),
		publishedAt: spec.PublishedAt,
		verifiedAt:  spec.VerifiedAt,
		models:      make(map[int64]catalogSnapshotModel, len(spec.Models)),
		lookup:      make(map[string]map[string]int64, len(spec.Models)*2),
	}

	for _, candidate := range spec.Models {
		model, err := validateCatalogSnapshotModel(candidate)
		if err != nil {
			return nil, err
		}
		if _, exists := snapshot.models[model.id]; exists {
			return nil, fmt.Errorf("%w: duplicate model id %d", ErrCatalogInvalidSnapshot, model.id)
		}
		snapshot.models[model.id] = model

		if err := snapshot.addLookup(CatalogPlatformScopeAny, model.canonicalKey, model.id); err != nil {
			return nil, err
		}
		for _, alias := range candidate.Aliases {
			aliasValue := normalizeCatalogLookupValue(alias.Alias)
			if aliasValue == "" {
				return nil, fmt.Errorf("%w: empty alias for %s", ErrCatalogInvalidSnapshot, model.canonicalKey)
			}
			if err := snapshot.addLookup(normalizeCatalogPlatformScope(alias.PlatformScope), aliasValue, model.id); err != nil {
				return nil, err
			}
		}
	}
	return snapshot, nil
}

func validateCatalogSnapshotModel(candidate CatalogSnapshotModelSpec) (catalogSnapshotModel, error) {
	canonicalKey := normalizeCatalogLookupValue(candidate.CanonicalKey)
	if candidate.ID <= 0 || candidate.RevisionID <= 0 || canonicalKey == "" {
		return catalogSnapshotModel{}, fmt.Errorf("%w: invalid model identity", ErrCatalogInvalidSnapshot)
	}
	if !validCatalogOperatorState(candidate.OperatorState) || !validCatalogSourceState(candidate.SourceState) {
		return catalogSnapshotModel{}, fmt.Errorf("%w: invalid lifecycle state for %s", ErrCatalogInvalidSnapshot, canonicalKey)
	}
	if candidate.PricingValid && candidate.Pricing == nil {
		return catalogSnapshotModel{}, fmt.Errorf("%w: valid pricing missing for %s", ErrCatalogInvalidSnapshot, canonicalKey)
	}

	model := catalogSnapshotModel{
		id:                   candidate.ID,
		revisionID:           candidate.RevisionID,
		canonicalKey:         canonicalKey,
		operatorState:        candidate.OperatorState,
		sourceState:          candidate.SourceState,
		provider:             strings.TrimSpace(candidate.Provider),
		platform:             normalizeCatalogLookupValue(candidate.Platform),
		mode:                 normalizeCatalogLookupValue(candidate.Mode),
		contextWindow:        candidate.ContextWindow,
		maxOutputTokens:      candidate.MaxOutputTokens,
		pricingSchemaVersion: candidate.PricingSchemaVersion,
		pricingValid:         candidate.PricingValid,
		pricingSource:        strings.TrimSpace(candidate.PricingSource),
		sourceHash:           strings.TrimSpace(candidate.SourceHash),
		capabilities:         append(json.RawMessage(nil), candidate.Capabilities...),
		sourceMetadata:       append(json.RawMessage(nil), candidate.SourceMetadata...),
	}
	if candidate.Pricing != nil {
		model.pricing = *candidate.Pricing
	}
	return model, nil
}

func (s *CatalogRuntimeSnapshot) addLookup(platformScope, alias string, modelID int64) error {
	platformScope = normalizeCatalogPlatformScope(platformScope)
	alias = normalizeCatalogLookupValue(alias)
	platformLookup := s.lookup[platformScope]
	if platformLookup == nil {
		platformLookup = make(map[string]int64)
		s.lookup[platformScope] = platformLookup
	}
	if existing, ok := platformLookup[alias]; ok && existing != modelID {
		return fmt.Errorf("%w: %s\x00%s", ErrCatalogDuplicateLookup, platformScope, alias)
	}
	platformLookup[alias] = modelID
	return nil
}

func (s *CatalogRuntimeSnapshot) readView() CatalogReadView {
	return CatalogReadView{snapshot: s}
}

func validCatalogOperatorState(state CatalogOperatorState) bool {
	switch state {
	case CatalogOperatorStateEnabled, CatalogOperatorStateDisabled, CatalogOperatorStateRetired:
		return true
	default:
		return false
	}
}

func validCatalogSourceState(state CatalogSourceState) bool {
	switch state {
	case CatalogSourceStatePresent, CatalogSourceStateMissing, CatalogSourceStateInvalid:
		return true
	default:
		return false
	}
}

func normalizeCatalogScope(scope string) string {
	return normalizeCatalogLookupValue(scope)
}

func normalizeCatalogLookupValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeCatalogPlatformScope(platform string) string {
	platform = normalizeCatalogLookupValue(platform)
	if platform == "" {
		return CatalogPlatformScopeAny
	}
	return platform
}
