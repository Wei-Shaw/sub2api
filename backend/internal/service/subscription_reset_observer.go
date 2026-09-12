package service

import (
	"context"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const SubscriptionResetMaxAccounts = 200

var ErrSubscriptionResetPolicyConflict = infraerrors.Conflict("SUBSCRIPTION_RESET_POLICY_CONFLICT", "subscription reset policy changed; reload before saving")

// Observe is intentionally the only enabled mode. No observer operation writes
// subscription usage, expiry dates, billing commands, or reset-credit balances.
type SubscriptionResetPolicy struct {
	GroupID            int64     `json:"group_id"`
	Mode               string    `json:"mode"`
	Source             string    `json:"source"`
	AccountIDs         []int64   `json:"account_ids"`
	QuorumPercent      int       `json:"quorum_percent"`
	AggregationMinutes int       `json:"aggregation_minutes"`
	ResetDimensions    []string  `json:"reset_dimensions"`
	AllowSingleSubject bool      `json:"allow_single_subject"`
	Version            int64     `json:"version"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func DefaultSubscriptionResetPolicy(groupID int64) *SubscriptionResetPolicy {
	return &SubscriptionResetPolicy{GroupID: groupID, Mode: "off", Source: "7d", AccountIDs: []int64{}, QuorumPercent: 80, AggregationMinutes: 10, ResetDimensions: []string{"daily", "weekly", "monthly"}}
}

func (p *SubscriptionResetPolicy) Validate() error {
	invalid := func(message string) error {
		return infraerrors.BadRequest("INVALID_SUBSCRIPTION_RESET_POLICY", message)
	}
	if p == nil || p.GroupID <= 0 || p.Version < 0 {
		return invalid("group_id and policy version are invalid")
	}
	if p.Mode != "off" && p.Mode != "observe" {
		return invalid("mode must be off or observe; automatic execution is not supported")
	}
	if p.Source != "7d" || p.QuorumPercent < 1 || p.QuorumPercent > 100 || p.AggregationMinutes < 1 || p.AggregationMinutes > 60 {
		return invalid("source must be 7d, quorum_percent 1..100, and aggregation_minutes 1..60")
	}
	if len(p.AccountIDs) > SubscriptionResetMaxAccounts || (p.Mode == "observe" && len(p.AccountIDs) == 0) {
		return invalid("observe mode requires 1..200 explicitly selected accounts")
	}
	seen := make(map[int64]bool, len(p.AccountIDs))
	for _, id := range p.AccountIDs {
		if id <= 0 || seen[id] {
			return invalid("account_ids must contain unique positive IDs")
		}
		seen[id] = true
	}
	if len(p.ResetDimensions) == 0 || len(p.ResetDimensions) > 3 {
		return invalid("select at least one reset dimension")
	}
	dimensions := map[string]bool{}
	for _, dimension := range p.ResetDimensions {
		if (dimension != "daily" && dimension != "weekly" && dimension != "monthly") || dimensions[dimension] {
			return invalid("reset_dimensions must contain unique daily, weekly, or monthly values")
		}
		dimensions[dimension] = true
	}
	return nil
}

// Quota identity comes from the lightweight upstream GET, not local account IDs
// or a workspace alone. Raw identifiers are hashed before entering stored state.
type SubscriptionResetQuotaSubject struct {
	UserID    string `json:"user_id"`
	AccountID string `json:"account_id"`
	Scope     string `json:"scope"`
}

type SubscriptionResetSample struct {
	AccountID     int64
	ObservedAt    time.Time
	Subject       *SubscriptionResetQuotaSubject
	UsedPercent   *float64
	ResetAt       *time.Time
	WindowMinutes *int
	PlanType      string
	LocalResetAt  *time.Time
	// Error must be a safe reason code, never an upstream response/token.
	Error string
}

type SubscriptionResetAccountState struct {
	AccountID      int64      `json:"account_id"`
	SubjectKey     string     `json:"subject_key"`
	DuplicateOf    *int64     `json:"duplicate_of"`
	LastObservedAt *time.Time `json:"last_observed_at"`
	UsedPercent    *float64   `json:"used_percent"`
	ResetAt        *time.Time `json:"reset_at"`
	WindowMinutes  *int       `json:"window_minutes"`
	PlanType       string     `json:"plan_type"`
	State          string     `json:"state"`
	Reason         string     `json:"reason"`
	LastError      string     `json:"last_error"`
}

type SubscriptionResetEventMember struct {
	SubjectKey          string     `json:"subject_key"`
	AccountIDs          []int64    `json:"account_ids"`
	Confirmed           bool       `json:"confirmed"`
	ConfirmedAt         *time.Time `json:"confirmed_at"`
	TransitionID        string     `json:"transition_id"`
	State               string     `json:"state"`
	Reason              string     `json:"reason"`
	OldResetAt          *time.Time `json:"old_reset_at,omitempty"`
	NewResetAt          *time.Time `json:"new_reset_at,omitempty"`
	OldUsedPercent      *float64   `json:"old_used_percent,omitempty"`
	NewUsedPercent      *float64   `json:"new_used_percent,omitempty"`
	ConfirmationSamples int        `json:"confirmation_samples"`
	LastEvidenceAt      *time.Time `json:"last_evidence_at,omitempty"`
}

type SubscriptionResetEvent struct {
	ID              string                         `json:"id"`
	GroupID         int64                          `json:"group_id"`
	PolicyVersion   int64                          `json:"policy_version"`
	Source          string                         `json:"source"`
	Kind            string                         `json:"kind"`
	Status          string                         `json:"status"`
	Reason          string                         `json:"reason"`
	OpenedAt        time.Time                      `json:"opened_at"`
	DeadlineAt      time.Time                      `json:"deadline_at"`
	ConfirmedAt     *time.Time                     `json:"confirmed_at"`
	UpdatedAt       time.Time                      `json:"updated_at"`
	SourceResetAt   time.Time                      `json:"source_reset_at"`
	ResetDimensions []string                       `json:"reset_dimensions"`
	Denominator     int                            `json:"denominator"`
	ConfirmedCount  int                            `json:"confirmed_count"`
	RequiredCount   int                            `json:"required_count"`
	Members         []SubscriptionResetEventMember `json:"members"`
}

type SubscriptionResetStatus struct {
	Policy               *SubscriptionResetPolicy        `json:"policy"`
	Accounts             []SubscriptionResetAccountState `json:"accounts"`
	SubjectCount         int                             `json:"subject_count"`
	VerifiedSubjectCount int                             `json:"verified_subject_count"`
	Ready                bool                            `json:"ready"`
	ActiveEvent          *SubscriptionResetEvent         `json:"active_event"`
	Events               []*SubscriptionResetEvent       `json:"events"`
	LastObservedAt       *time.Time                      `json:"last_observed_at"`
	ObservationOnly      bool                            `json:"observation_only"`
}

type SubscriptionResetObserverRepository interface {
	GetPolicy(context.Context, int64) (*SubscriptionResetPolicy, error)
	SavePolicy(context.Context, *SubscriptionResetPolicy) (*SubscriptionResetPolicy, error)
	ListEnabled(context.Context) ([]*SubscriptionResetPolicy, error)
	RecordSample(context.Context, int64, int64, *SubscriptionResetSample) error
	GetStatus(context.Context, int64, int) (*SubscriptionResetStatus, error)
}

// Persisted JSON under the policy row lock. Events also have their own durable
// audit rows; the bounded recent list associates late evidence with its batch.
type SubscriptionResetObserverState struct {
	Accounts       map[int64]*SubscriptionResetObserverAccount `json:"accounts"`
	Events         []*SubscriptionResetEvent                   `json:"events"`
	LastObservedAt *time.Time                                  `json:"last_observed_at"`
}

type SubscriptionResetObserverAccount struct {
	SubscriptionResetAccountState
	Baseline     *SubscriptionResetEvidence `json:"baseline"`
	ReviewReason string                     `json:"review_reason,omitempty"`
}

type SubscriptionResetEvidence struct {
	ObservedAt time.Time `json:"observed_at"`
	SubjectKey string    `json:"subject_key"`
	Used       float64   `json:"used"`
	ResetAt    time.Time `json:"reset_at"`
	PlanType   string    `json:"plan_type"`
}

func NewSubscriptionResetObserverState() *SubscriptionResetObserverState {
	return &SubscriptionResetObserverState{Accounts: map[int64]*SubscriptionResetObserverAccount{}, Events: []*SubscriptionResetEvent{}}
}

// Snapshot projection shares freshness rules with the engine; missing or stale
// observations remain unknown and never reduce a candidate's frozen denominator.
func SubscriptionResetObserverStatus(p *SubscriptionResetPolicy, s *SubscriptionResetObserverState, events []*SubscriptionResetEvent, now time.Time) *SubscriptionResetStatus {
	out := &SubscriptionResetStatus{Policy: p, Accounts: []SubscriptionResetAccountState{}, Events: events, ObservationOnly: true}
	if out.Events == nil {
		out.Events = []*SubscriptionResetEvent{}
	}
	projected := make([]*SubscriptionResetEvent, 0, len(out.Events))
	for _, original := range out.Events {
		event := *original
		if event.Status == "pending" && now.After(event.DeadlineAt) {
			event.Status, event.Reason = "timed_out", "quorum_not_met"
		}
		projected = append(projected, &event)
	}
	out.Events = projected
	if s == nil {
		s = NewSubscriptionResetObserverState()
	}
	out.LastObservedAt = s.LastObservedAt
	firstIDs := map[string]int64{}
	verified := map[string]bool{}
	ids := append([]int64{}, p.AccountIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		a := SubscriptionResetAccountState{AccountID: id, State: "waiting_baseline", Reason: "waiting_for_fresh_sample"}
		if stored := s.Accounts[id]; stored != nil {
			a = stored.SubscriptionResetAccountState
			if a.LastObservedAt != nil && now.Sub(*a.LastObservedAt) > subscriptionResetFreshness(p) {
				a.State, a.Reason = "unknown", "stale_sample"
			}
		}
		key := a.SubjectKey
		if key == "" {
			out.SubjectCount++
		} else if first, ok := firstIDs[key]; ok {
			a.DuplicateOf = &first
		} else {
			firstIDs[key] = id
			out.SubjectCount++
		}
		if key != "" && a.State == "observing" {
			verified[key] = true
		}
		out.Accounts = append(out.Accounts, a)
	}
	out.VerifiedSubjectCount = len(verified)
	out.Ready = p.Mode == "observe" && out.SubjectCount > 0 && len(verified) == out.SubjectCount && (out.SubjectCount > 1 || p.AllowSingleSubject)
	for _, event := range out.Events {
		if event.PolicyVersion == p.Version && (event.Status == "pending" || event.Status == "needs_review") {
			copy := *event
			if copy.Status == "pending" && now.After(copy.DeadlineAt) {
				copy.Status = "timed_out"
			} else {
				out.ActiveEvent = &copy
				break
			}
		}
	}
	return out
}

func subscriptionResetSafeReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	if len(reason) > 80 {
		return "query_failed"
	}
	for _, c := range reason {
		if c != '_' && c != '-' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return "query_failed"
		}
	}
	return reason
}
