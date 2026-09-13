package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// History observes existing OpenAI OAuth quota data. Dollar values are local
// API-price comparisons, never a provider-issued monetary allowance.
const (
	windowTypeFiveHour = "5h"
	windowTypeSevenDay = "7d"
)

var windowTypeDuration = map[string]time.Duration{windowTypeFiveHour: 5 * time.Hour, windowTypeSevenDay: 7 * 24 * time.Hour}

func recordedWindow(kind string) bool { _, ok := windowTypeDuration[kind]; return ok }

// AccountWindowUsageEntry keeps accumulated cost separate from the last usable
// estimate. A later sample with missing prices must not manufacture a new limit.
type AccountWindowUsageEntry struct {
	WindowStart             time.Time  `json:"window_start"`
	WindowEnd               time.Time  `json:"window_end"`
	FirstObservedAt         time.Time  `json:"first_observed_at"`
	LastSampleAt            *time.Time `json:"last_sample_at"`
	PeakUsedPercent         float64    `json:"peak_used_percent"`
	LastUsedPercent         float64    `json:"last_used_percent"`
	FinalUsedPercent        *float64   `json:"final_used_percent"`
	SampleCount             int        `json:"sample_count"`
	Finalized               bool       `json:"finalized"`
	EndReason               *string    `json:"end_reason"`
	Requests                int64      `json:"requests"`
	TokensTotal             int64      `json:"tokens_total"`
	APIReferenceCost        *float64   `json:"api_reference_cost"`
	PricedRequests          int64      `json:"priced_requests"`
	MissingPricingRequests  int64      `json:"missing_pricing_requests"`
	EstimatedReferenceLimit *float64   `json:"estimated_reference_limit"`
	EstimateReferenceCost   *float64   `json:"estimate_reference_cost"`
	EstimateUsedPercent     *float64   `json:"estimate_used_percent"`
	EstimateObservedAt      *time.Time `json:"estimate_observed_at"`
	QualityFlags            []string   `json:"quality_flags"`
}

type AccountWindowUsageRecord struct {
	AccountWindowUsageEntry
	ID         int64  `json:"id"`
	AccountID  int64  `json:"account_id"`
	WindowType string `json:"window_type"`
	// ResetAt remains the upstream boundary when an early reset truncates WindowEnd.
	ResetAt           time.Time  `json:"reset_at"`
	DurationMinutes   int        `json:"duration_minutes"`
	LastObservationID int64      `json:"last_observation_id"`
	FinalizedAt       *time.Time `json:"finalized_at"`
	StatsFinalizedAt  *time.Time `json:"stats_finalized_at"`
}

type AccountQuotaObservation struct {
	ID         int64
	AccountID  int64
	ObservedAt time.Time
	Payload    json.RawMessage
}

type AccountWindowReferenceStats struct {
	Requests               int64
	TokensTotal            int64
	ReferenceCost          *float64
	PricedRequests         int64
	MissingPricingRequests int64
}

// All callbacks run under an account row lock. Processing the journal item,
// updating every affected window, and setting processed_at share one transaction.
// Failed callbacks roll back and leave the observation pending for a later tick.
type AccountWindowUsageRepository interface {
	ConsumeNextObservation(context.Context, time.Time, func(context.Context, *AccountQuotaObservation) error) (bool, error)
	GetOpenWindow(context.Context, int64, string) (*AccountWindowUsageRecord, error)
	GetLatestWindow(context.Context, int64, string) (*AccountWindowUsageRecord, error)
	GetClosedWindow(context.Context, int64, string, time.Time, time.Time) (*AccountWindowUsageRecord, error)
	SaveWindow(context.Context, *AccountWindowUsageRecord) error
	LatestResetMarker(context.Context, int64, time.Time, time.Time) (*time.Time, error)
	AggregateReferenceUsage(context.Context, int64, time.Time, time.Time) (*AccountWindowReferenceStats, error)
	ReconcileNextWindow(context.Context, time.Time, func(context.Context, *AccountWindowUsageRecord) error) (bool, error)
	ListHistorySince(context.Context, int64, time.Time) ([]*AccountWindowUsageRecord, error)
	PruneHistory(context.Context, time.Time, time.Time) error
}

type AccountWindowHistoryResponse struct {
	Windows map[string][]*AccountWindowUsageEntry `json:"windows"`
}
type AccountWindowUsageHistoryService struct {
	windowRepo  AccountWindowUsageRepository
	accountRepo AccountRepository
}

func NewAccountWindowUsageHistoryService(w AccountWindowUsageRepository, a AccountRepository) *AccountWindowUsageHistoryService {
	return &AccountWindowUsageHistoryService{w, a}
}
func (s *AccountWindowUsageHistoryService) GetWindowHistory(ctx context.Context, id int64, days int) (*AccountWindowHistoryResponse, error) {
	if id <= 0 || days < 1 || days > 90 {
		return nil, fmt.Errorf("invalid window history query")
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	response := &AccountWindowHistoryResponse{Windows: map[string][]*AccountWindowUsageEntry{"5h": {}, "7d": {}}}
	if !IsOpenAIQuotaHistoryAccount(account) {
		return response, nil
	}
	rows, err := s.windowRepo.ListHistorySince(ctx, id, time.Now().AddDate(0, 0, -days))
	if err != nil {
		return nil, fmt.Errorf("list window history: %w", err)
	}
	for _, row := range rows {
		if !recordedWindow(row.WindowType) {
			continue
		}
		entry := row.AccountWindowUsageEntry
		entry.Finalized = row.FinalizedAt != nil
		if entry.Finalized {
			v := row.LastUsedPercent
			entry.FinalUsedPercent = &v
		}
		if entry.QualityFlags == nil {
			entry.QualityFlags = []string{}
		}
		response.Windows[row.WindowType] = append(response.Windows[row.WindowType], &entry)
	}
	return response, nil
}

// IsOpenAIQuotaHistoryAccount limits the first history version to ordinary
// global OAuth quotas. Shadow/Spark and token identity scopes are independent.
func IsOpenAIQuotaHistoryAccount(account *Account) bool {
	if account == nil || !account.IsOpenAIOAuth() || account.IsShadow() {
		return false
	}
	dimension := strings.ToLower(strings.TrimSpace(account.QuotaDimension))
	if dimension != "" && dimension != "global" {
		return false
	}
	for _, key := range []string{"auth_mode", "openai_auth_mode"} {
		switch strings.ToLower(strings.TrimSpace(account.GetCredential(key))) {
		case "agent_identity", "personal_access_token", "personalaccesstoken":
			return false
		}
	}
	return true
}
