package service

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"time"
)

const (
	// Match the shortest Codex quota window so recent traffic drives
	// replenishment decisions without carrying a full day's stale demand.
	openAIProvisionDemandWindow         = 5 * time.Hour
	openAIProvisionQuotaReserve         = 0.20
	openAIProvisionMinimumPrewarmTokens = 10_000
	openAIProvisionSignalWriteTimeout   = 250 * time.Millisecond
	openAIProvisionSignalWriterLimit    = 64
	openAIProvisionMaxImmediateAccounts = 1
	// OpenAIProvisionCapacitySignalWindow bounds the failed-capacity signal to
	// requests recent enough to justify an immediate registration.
	OpenAIProvisionCapacitySignalWindow = 5 * time.Minute
)

// OpenAIProvisionDemand is the recent demand signal used by the replenishment
// coordinator. It intentionally contains aggregates only; no user identity or
// credential data crosses the coordinator boundary.
type OpenAIProvisionDemand struct {
	ActiveUsers            int64
	Requests               int64
	Tokens                 int64
	CapacityDeniedRequests int64
	CapacityDeniedUsers    int64
}

// OpenAIProvisionDemandReader supplies usage aggregates for the OpenAI pool.
// It is separate from UsageLogRepository so existing consumers do not need to
// depend on the automation-specific query.
type OpenAIProvisionDemandReader interface {
	GetOpenAIProvisionDemand(context.Context, time.Time, time.Time) (OpenAIProvisionDemand, error)
}

// OpenAIProvisionCapacityDemandStore keeps a short-lived set of users who
// actually hit an OpenAI capacity failure. It is deliberately separate from
// usage logs and Ops monitoring because failed requests never produce usage
// rows and Ops may be disabled by configuration.
type OpenAIProvisionCapacityDemandStore interface {
	RecordOpenAICapacityDenied(context.Context, int64) error
	GetOpenAICapacityDeniedUsers(context.Context, time.Time) (int64, error)
}

// OpenAIProvisionDemandService combines durable usage aggregates with the
// short-lived failed-capacity signal used by the replenishment coordinator.
type OpenAIProvisionDemandService struct {
	usage       OpenAIProvisionDemandReader
	store       OpenAIProvisionCapacityDemandStore
	signalSlots chan struct{}
}

func NewOpenAIProvisionDemandService(usage OpenAIProvisionDemandReader, store OpenAIProvisionCapacityDemandStore) *OpenAIProvisionDemandService {
	return &OpenAIProvisionDemandService{
		usage:       usage,
		store:       store,
		signalSlots: make(chan struct{}, openAIProvisionSignalWriterLimit),
	}
}

func (s *OpenAIProvisionDemandService) GetOpenAIProvisionDemand(ctx context.Context, start, end time.Time) (OpenAIProvisionDemand, error) {
	if s == nil || s.usage == nil {
		return OpenAIProvisionDemand{}, errors.New("openai provision usage reader is unavailable")
	}
	demand, err := s.usage.GetOpenAIProvisionDemand(ctx, start, end)
	if err != nil {
		return OpenAIProvisionDemand{}, err
	}
	if s.store != nil {
		denied, storeErr := s.store.GetOpenAICapacityDeniedUsers(ctx, end)
		if storeErr != nil {
			return OpenAIProvisionDemand{}, storeErr
		}
		demand.CapacityDeniedUsers = denied
		if denied > 0 {
			demand.CapacityDeniedRequests = denied
		}
	}
	return demand, nil
}

// RecordOpenAICapacityDenied is best effort: a Redis outage must not turn an
// already-failed user request into a second failure or block the gateway.
func (s *OpenAIProvisionDemandService) RecordOpenAICapacityDenied(ctx context.Context, userID int64) {
	if s == nil || s.store == nil || userID <= 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Do not hold the gateway response open on Redis. The signal is advisory and
	// can be dropped during shutdown, a full Redis outage, or a saturated writer
	// pool without affecting the already-failed request. The bounded semaphore
	// prevents one goroutine from being created for every failed request.
	if s.signalSlots == nil {
		return
	}
	select {
	case s.signalSlots <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-s.signalSlots }()
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIProvisionSignalWriteTimeout)
		defer cancel()
		if err := s.store.RecordOpenAICapacityDenied(writeCtx, userID); err != nil {
			slog.Warn("openai_capacity_demand_record_failed", "error", err)
		}
	}()
}

type openAIProvisionPlan struct {
	ShouldProvision bool
	RequestedCount  int
	RequiredCount   int
}

// calculateOpenAIProvisionPlan keeps the configured target as a hard ceiling,
// while sizing the active pool from observed users and token demand. A
// zero-demand window is a deliberate no-op: an account deficit alone must
// never buy verification resources.
func calculateOpenAIProvisionPlan(demand OpenAIProvisionDemand, healthy int, effectiveCapacity float64, target int, _ time.Time) openAIProvisionPlan {
	if target <= 0 || healthy < 0 || effectiveCapacity < 0 {
		return openAIProvisionPlan{}
	}
	if demand.Requests <= 0 && demand.CapacityDeniedRequests <= 0 {
		return openAIProvisionPlan{}
	}
	// A recent capacity denial is the strongest signal. When the quota snapshot
	// is already inside the reserve, recent token-bearing traffic is enough to
	// prewarm one replacement before the next request fails. Normal traffic with
	// usable quota never buys credentials.
	demandCapacity := 0.0
	if demand.CapacityDeniedUsers > 0 {
		// Distinct denied users are a pressure signal, not a one-to-one account
		// requirement. Bound each polling cycle to a small step-up; repeated
		// denial windows will scale further only while pressure persists.
		demandCapacity = math.Min(float64(demand.CapacityDeniedUsers), effectiveCapacity+openAIProvisionMaxImmediateAccounts)
	} else if effectiveCapacity <= openAIProvisionQuotaReserve && demand.Requests > 0 && demand.Tokens >= openAIProvisionMinimumPrewarmTokens {
		demandCapacity = 1
	} else {
		return openAIProvisionPlan{}
	}
	deficit := int(math.Ceil(demandCapacity - effectiveCapacity))
	if deficit <= 0 {
		return openAIProvisionPlan{}
	}
	availableTarget := target - healthy
	if availableTarget <= 0 {
		return openAIProvisionPlan{}
	}
	if deficit > availableTarget {
		deficit = availableTarget
	}
	return openAIProvisionPlan{
		ShouldProvision: true,
		RequestedCount:  deficit,
		RequiredCount:   healthy + deficit,
	}
}

func openAIAccountQuotaHeadroom(account *Account, now time.Time) float64 {
	if account == nil {
		return 0
	}
	headroom := 1.0
	known := false
	for _, window := range []string{"5h", "7d"} {
		used, ok := resolveOpenAIQuotaUtilization(account.Extra, window, now)
		if !ok {
			continue
		}
		known = true
		remaining := 1 - clamp01(used)
		if remaining < headroom {
			headroom = remaining
		}
	}
	if !known {
		return 1
	}
	return clamp01(headroom)
}

// openAIPoolEffectiveCapacity returns the number of usable accounts after
// applying the smallest known 5h/7d headroom per account. A usable account
// contributes one whole slot, so ordinary demand cannot buy a replacement
// merely because an account has consumed part of its quota. Unknown or stale
// snapshots intentionally contribute one full slot.
func openAIPoolEffectiveCapacity(accounts []Account, now time.Time) float64 {
	capacity := 0.0
	for index := range accounts {
		account := &accounts[index]
		if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || account.ParentAccountID != nil {
			continue
		}
		headroom := openAIAccountQuotaHeadroom(account, now)
		if headroom > openAIProvisionQuotaReserve {
			capacity++
		}
	}
	return capacity
}

// countProvisionableOpenAIOAuthAccounts counts accounts that can still serve
// new traffic. OAuth accounts remain schedulable when their provider quota is
// exhausted, so the provisioning target must exclude those accounts while the
// runtime status can continue to report the full active pool separately.
func countProvisionableOpenAIOAuthAccounts(accounts []Account, now time.Time) int {
	count := 0
	for index := range accounts {
		account := &accounts[index]
		if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || account.ParentAccountID != nil ||
			account.Status != StatusActive || !account.Schedulable || account.IsCredentialShadow() {
			continue
		}
		if openAIAccountQuotaHeadroom(account, now) > openAIProvisionQuotaReserve {
			count++
		}
	}
	return count
}
