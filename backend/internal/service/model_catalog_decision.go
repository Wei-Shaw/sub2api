package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var (
	ErrCatalogModelNotFound      = errors.New("catalog model not found")
	ErrCatalogModelDisabled      = errors.New("catalog model disabled")
	ErrCatalogModelRetired       = errors.New("catalog model retired")
	ErrCatalogSourceUnavailable  = errors.New("catalog model source unavailable")
	ErrCatalogPricingUnavailable = errors.New("catalog model pricing unavailable")
	ErrCatalogDecisionInvalid    = errors.New("catalog decision invalid")
)

// CatalogDecisionRequest is the input to the shared request-time catalog gate.
// EffectiveModel is supplied by the existing mapping layer. The catalog gate
// verifies both requested and mapped/effective identities from one pinned view.
type CatalogDecisionRequest struct {
	RequestedModel string
	EffectiveModel string
	Platform       string
}

// CatalogDecision is immutable request provenance. It must be carried through
// upstream execution and settlement; it is never rebuilt from the current
// catalog after admission.
type CatalogDecision struct {
	Requested CatalogModelDecision
	Effective CatalogModelDecision
	View      CatalogReadView
	CreatedAt time.Time
}

// CatalogUsagePin is the immutable request provenance required to reproduce
// pricing after a later publication. It is value-owned and safe to carry into
// deferred usage/billing work.
type CatalogUsagePin struct {
	CatalogEpoch             int64
	CatalogRevisionID        int64
	RequestedModelRevisionID int64
	EffectiveModelRevisionID int64
	PricingSource            string
	PricingSnapshot          map[string]any
}

func (p CatalogUsagePin) Valid() bool {
	return p.CatalogEpoch > 0 &&
		p.CatalogRevisionID > 0 &&
		p.RequestedModelRevisionID > 0 &&
		p.EffectiveModelRevisionID > 0 &&
		strings.TrimSpace(p.PricingSource) != "" &&
		p.PricingSnapshot != nil
}

// ApplyToUsageLog copies the pin into nullable usage provenance columns. The
// JSON round trip deliberately gives the log its own map so neither the
// catalog decision nor another deferred worker can mutate persisted provenance.
func (p CatalogUsagePin) ApplyToUsageLog(log *UsageLog) error {
	if !p.Valid() {
		return ErrCatalogDecisionInvalid
	}
	if log == nil {
		return fmt.Errorf("%w: usage log is nil", ErrCatalogDecisionInvalid)
	}
	snapshot, err := cloneCatalogPricingSnapshot(p.PricingSnapshot)
	if err != nil {
		return fmt.Errorf("clone catalog pricing snapshot: %w", err)
	}
	catalogEpoch := p.CatalogEpoch
	catalogRevisionID := p.CatalogRevisionID
	requestedRevisionID := p.RequestedModelRevisionID
	effectiveRevisionID := p.EffectiveModelRevisionID
	pricingSource := strings.TrimSpace(p.PricingSource)
	log.CatalogEpoch = &catalogEpoch
	log.CatalogRevisionID = &catalogRevisionID
	log.RequestedModelRevisionID = &requestedRevisionID
	log.EffectiveModelRevisionID = &effectiveRevisionID
	log.PricingSource = &pricingSource
	log.PricingSnapshot = snapshot
	return nil
}

// UsagePin converts an admitted decision into the settlement contract without
// consulting the current reader, database, Redis, or remote pricing source.
func (d CatalogDecision) UsagePin() (CatalogUsagePin, error) {
	if !d.Valid() || d.Requested.RevisionID <= 0 || d.Effective.RevisionID <= 0 ||
		d.Requested.CatalogRevisionID != d.View.RevisionID() ||
		d.Effective.CatalogRevisionID != d.View.RevisionID() {
		return CatalogUsagePin{}, ErrCatalogDecisionInvalid
	}
	pricingSource := strings.TrimSpace(d.Effective.PricingSource)
	if pricingSource == "" {
		return CatalogUsagePin{}, fmt.Errorf("%w: missing pricing source", ErrCatalogDecisionInvalid)
	}
	pricingSnapshot, err := catalogPricingSnapshot(d.Effective.Pricing)
	if err != nil {
		return CatalogUsagePin{}, fmt.Errorf("build catalog pricing snapshot: %w", err)
	}
	pin := CatalogUsagePin{
		CatalogEpoch:             d.View.Epoch(),
		CatalogRevisionID:        d.View.RevisionID(),
		RequestedModelRevisionID: d.Requested.RevisionID,
		EffectiveModelRevisionID: d.Effective.RevisionID,
		PricingSource:            pricingSource,
		PricingSnapshot:          pricingSnapshot,
	}
	if !pin.Valid() {
		return CatalogUsagePin{}, ErrCatalogDecisionInvalid
	}
	return pin, nil
}

func catalogPricingSnapshot(pricing LiteLLMModelPricing) (map[string]any, error) {
	encoded, err := json.Marshal(pricing)
	if err != nil {
		return nil, err
	}
	var snapshot map[string]any
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func cloneCatalogPricingSnapshot(snapshot map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	var clone map[string]any
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return nil, err
	}
	return clone, nil
}

func catalogUsagePinFromDecision(decision *CatalogDecision) (*CatalogUsagePin, error) {
	if decision == nil {
		return nil, nil
	}
	pin, err := decision.UsagePin()
	if err != nil {
		return nil, err
	}
	return &pin, nil
}

func (d CatalogDecision) Valid() bool {
	return d.View.Epoch() > 0 &&
		d.Requested.ID > 0 &&
		d.Effective.ID > 0 &&
		d.Requested.CatalogEpoch == d.View.Epoch() &&
		d.Effective.CatalogEpoch == d.View.Epoch() &&
		d.Effective.HasPricing
}

func (d CatalogDecision) CatalogEpoch() int64 {
	return d.View.Epoch()
}

func (d CatalogDecision) CatalogRevisionID() int64 {
	return d.View.RevisionID()
}

// WithEffectiveModel finalizes the provider/account-mapped model against the
// same immutable read view that admitted the original request. It performs no
// repository or cache I/O and preserves requested-model provenance.
func (d CatalogDecision) WithEffectiveModel(effectiveModel, platform string) (CatalogDecision, error) {
	effectiveName := strings.TrimSpace(effectiveModel)
	if effectiveName == "" {
		return CatalogDecision{}, fmt.Errorf("%w: effective model is empty", ErrCatalogDecisionInvalid)
	}
	effective, ok := d.View.Resolve(effectiveName, platform)
	if !ok {
		return CatalogDecision{}, fmt.Errorf("%w: effective=%q", ErrCatalogModelNotFound, effectiveName)
	}
	if err := validateCatalogAdmissionState("effective", effective); err != nil {
		return CatalogDecision{}, err
	}
	if !effective.HasPricing {
		return CatalogDecision{}, fmt.Errorf("%w: effective=%s", ErrCatalogPricingUnavailable, effective.CanonicalKey)
	}

	finalized := d
	finalized.Effective = effective
	if !finalized.Valid() {
		return CatalogDecision{}, ErrCatalogDecisionInvalid
	}
	return finalized, nil
}

// ResolveCatalogDecision performs no I/O. All status checks happen before the
// final pricing check, and requested/effective lookup is pinned to the same
// CatalogReadView.
func ResolveCatalogDecision(view CatalogReadView, request CatalogDecisionRequest, now time.Time) (CatalogDecision, error) {
	requestedName := strings.TrimSpace(request.RequestedModel)
	if requestedName == "" {
		return CatalogDecision{}, fmt.Errorf("%w: requested model is empty", ErrCatalogDecisionInvalid)
	}
	requested, ok := view.Resolve(requestedName, request.Platform)
	if !ok {
		return CatalogDecision{}, fmt.Errorf("%w: requested=%q", ErrCatalogModelNotFound, requestedName)
	}
	if err := validateCatalogAdmissionState("requested", requested); err != nil {
		return CatalogDecision{}, err
	}

	effectiveName := strings.TrimSpace(request.EffectiveModel)
	if effectiveName == "" {
		effectiveName = requestedName
	}
	effective := requested
	if normalizeCatalogLookupValue(effectiveName) != normalizeCatalogLookupValue(requestedName) {
		var found bool
		effective, found = view.Resolve(effectiveName, request.Platform)
		if !found {
			return CatalogDecision{}, fmt.Errorf("%w: effective=%q", ErrCatalogModelNotFound, effectiveName)
		}
	}
	if err := validateCatalogAdmissionState("effective", effective); err != nil {
		return CatalogDecision{}, err
	}
	if !effective.HasPricing {
		return CatalogDecision{}, fmt.Errorf("%w: effective=%s", ErrCatalogPricingUnavailable, effective.CanonicalKey)
	}

	decision := CatalogDecision{
		Requested: requested,
		Effective: effective,
		View:      view,
		CreatedAt: now,
	}
	if !decision.Valid() {
		return CatalogDecision{}, ErrCatalogDecisionInvalid
	}
	return decision, nil
}

func validateCatalogAdmissionState(role string, decision CatalogModelDecision) error {
	switch decision.OperatorState {
	case CatalogOperatorStateDisabled:
		return fmt.Errorf("%w: %s=%s", ErrCatalogModelDisabled, role, decision.CanonicalKey)
	case CatalogOperatorStateRetired:
		return fmt.Errorf("%w: %s=%s", ErrCatalogModelRetired, role, decision.CanonicalKey)
	case CatalogOperatorStateEnabled:
		// Continue to source state validation below.
	default:
		return fmt.Errorf("%w: %s=%s operator_state=%s", ErrCatalogDecisionInvalid, role, decision.CanonicalKey, decision.OperatorState)
	}
	if decision.SourceState != CatalogSourceStatePresent {
		return fmt.Errorf("%w: %s=%s source_state=%s", ErrCatalogSourceUnavailable, role, decision.CanonicalKey, decision.SourceState)
	}
	return nil
}

// CatalogAdmissionHTTPStatus maps catalog lifecycle failures to a safe gateway
// response. Disabled/retired models are client policy errors; unavailable
// snapshots/source/prices are temporary control-plane failures and fail closed.
func CatalogAdmissionHTTPStatus(err error) int {
	switch {
	case errors.Is(err, ErrCatalogModelDisabled), errors.Is(err, ErrCatalogModelRetired):
		return http.StatusForbidden
	case errors.Is(err, ErrCatalogModelNotFound):
		return http.StatusBadRequest
	case errors.Is(err, ErrCatalogSourceUnavailable),
		errors.Is(err, ErrCatalogPricingUnavailable),
		errors.Is(err, ErrCatalogUnavailable),
		errors.Is(err, ErrCatalogSnapshotStale),
		errors.Is(err, ErrCatalogKnownStale):
		return http.StatusServiceUnavailable
	default:
		return http.StatusServiceUnavailable
	}
}

func CatalogAdmissionHTTPMessage(err error) string {
	switch {
	case errors.Is(err, ErrCatalogModelDisabled):
		return "model is disabled"
	case errors.Is(err, ErrCatalogModelRetired):
		return "model is retired"
	case errors.Is(err, ErrCatalogModelNotFound):
		return "model is not available"
	case errors.Is(err, ErrCatalogPricingUnavailable):
		return "model pricing is unavailable"
	case errors.Is(err, ErrCatalogSourceUnavailable):
		return "model source is unavailable"
	default:
		return "model catalog is temporarily unavailable"
	}
}
