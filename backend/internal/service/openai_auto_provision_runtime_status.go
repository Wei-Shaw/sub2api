package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	OpenAIAutoProvisionPhaseDisabled                 = "disabled"
	OpenAIAutoProvisionPhaseIdle                     = "idle"
	OpenAIAutoProvisionPhaseChecking                 = "checking"
	OpenAIAutoProvisionPhaseSendingProvisionRequest  = "sending_provision_request"
	OpenAIAutoProvisionPhaseWaitingProvisionCallback = "waiting_for_provision_callback"
	OpenAIAutoProvisionPhaseWaitingReauthorization   = "waiting_for_reauthorization_callback"
	OpenAIAutoProvisionPhaseError                    = "error"
)

// OpenAIAutoProvisionRuntimeStatus is the admin-facing, credential-free view
// of the coordinator lifecycle.
type OpenAIAutoProvisionRuntimeStatus struct {
	Phase                       string     `json:"phase"`
	Enabled                     bool       `json:"enabled"`
	Target                      int        `json:"target"`
	IntervalSeconds             int        `json:"interval_seconds"`
	HealthyAccountCount         int        `json:"healthy_account_count"`
	PendingProvisionCount       int        `json:"pending_provision_count"`
	PendingReauthorizationCount int        `json:"pending_reauthorization_count"`
	LastCheckStartedAt          *time.Time `json:"last_check_started_at,omitempty"`
	LastCheckCompletedAt        *time.Time `json:"last_check_completed_at,omitempty"`
	NextCheckAt                 *time.Time `json:"next_check_at,omitempty"`
	LastProvisionRequestedCount int        `json:"last_provision_requested_count"`
	LastProvisionRequestedAt    *time.Time `json:"last_provision_requested_at,omitempty"`
	LastCallbackAt              *time.Time `json:"last_callback_at,omitempty"`
	LastCallbackKind            string     `json:"last_callback_kind,omitempty"`
	LastCallbackStatus          string     `json:"last_callback_status,omitempty"`
	LastCallbackRequestedCount  int        `json:"last_callback_requested_count"`
	LastCallbackSucceededCount  int        `json:"last_callback_succeeded_count"`
	LastCallbackFailedCount     int        `json:"last_callback_failed_count"`
	LastCallbackPendingCount    int        `json:"last_callback_pending_count"`
	LastError                   string     `json:"last_error,omitempty"`
	ProvisionDispatchStale      bool       `json:"provision_dispatch_stale"`
	ProvisionResettable         bool       `json:"provision_resettable"`
	ProvisionRetryRequested     bool       `json:"provision_retry_requested"`
}

func (s *OpenAIAutoProvisionService) GetRuntimeStatus(ctx context.Context) (*OpenAIAutoProvisionRuntimeStatus, error) {
	state, err := s.readState(ctx)
	if err != nil {
		return nil, err
	}
	state = normalizeAutoProvisionState(state)
	cfg, configErr := s.config(ctx)
	healthyCount, countErr := s.healthyOpenAIOAuthCount(ctx)
	lastError := strings.TrimSpace(state.LastCheckError)
	if configErr != nil {
		lastError = joinRuntimeErrors(lastError, configErr)
	}
	if countErr != nil {
		lastError = joinRuntimeErrors(lastError, countErr)
	}

	status := &OpenAIAutoProvisionRuntimeStatus{
		Phase:                       OpenAIAutoProvisionPhaseIdle,
		Enabled:                     cfg.enabled,
		Target:                      cfg.target,
		IntervalSeconds:             int(cfg.interval / time.Second),
		HealthyAccountCount:         healthyCount,
		PendingProvisionCount:       0,
		PendingReauthorizationCount: len(state.Reauthorizations),
		LastCheckStartedAt:          state.LastCheckStartedAt,
		LastCheckCompletedAt:        state.LastCheckCompletedAt,
		LastProvisionRequestedCount: state.LastProvisionRequestedCount,
		LastProvisionRequestedAt:    state.LastProvisionRequestedAt,
		LastCallbackAt:              state.LastCallbackAt,
		LastCallbackKind:            state.LastCallbackKind,
		LastCallbackStatus:          state.LastCallbackStatus,
		LastCallbackRequestedCount:  state.LastCallbackRequestedCount,
		LastCallbackSucceededCount:  state.LastCallbackSucceededCount,
		LastCallbackFailedCount:     state.LastCallbackFailedCount,
		LastCallbackPendingCount:    state.LastCallbackPendingCount,
		LastError:                   lastError,
		ProvisionRetryRequested:     state.ProvisionRetryRequested,
	}
	if state.ProvisionDispatchInProgress {
		startedAt := state.ProvisionDispatchStartedAt
		if startedAt == nil && state.Provision != nil {
			startedAt = &state.Provision.CreatedAt
		}
		status.ProvisionDispatchStale = startedAt != nil && time.Since(*startedAt) >= autoProvisionDispatchStaleTTL
		status.ProvisionResettable = status.ProvisionDispatchStale
	}
	if state.Provision != nil {
		status.PendingProvisionCount = state.Provision.RequestedCount
		status.ProvisionResettable = time.Since(state.Provision.CreatedAt) >= autoProvisionDispatchStaleTTL
	}
	if cfg.enabled && cfg.target > 0 && status.LastCheckCompletedAt != nil && !state.CheckInProgress {
		next := status.LastCheckCompletedAt.Add(cfg.interval)
		status.NextCheckAt = &next
	}
	if status.LastError != "" {
		status.Phase = OpenAIAutoProvisionPhaseError
	} else if !cfg.enabled || cfg.target <= 0 {
		status.Phase = OpenAIAutoProvisionPhaseDisabled
	} else if state.ProvisionDispatchInProgress {
		status.Phase = OpenAIAutoProvisionPhaseSendingProvisionRequest
	} else if state.CheckInProgress {
		status.Phase = OpenAIAutoProvisionPhaseChecking
	} else if state.Provision != nil {
		status.Phase = OpenAIAutoProvisionPhaseWaitingProvisionCallback
	} else if len(state.Reauthorizations) > 0 {
		status.Phase = OpenAIAutoProvisionPhaseWaitingReauthorization
	}
	return status, nil
}

// ResetProvisioningStatus retires a stale registration request. The next
// scheduler cycle creates a fresh request, while the tombstone lets a late
// callback for the old request reconcile without clearing the new request.
func (s *OpenAIAutoProvisionService) ResetProvisioningStatus(ctx context.Context) error {
	release, ok := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, autoProvisionRunLockKey, s.instanceID, 5*time.Minute)
	if !ok {
		return infraerrors.Conflict("OPENAI_AUTO_PROVISION_BUSY", "openai auto-provision cycle is currently running")
	}
	defer release()

	return s.updateState(ctx, func(state *autoProvisionState) error {
		if state.Provision == nil {
			state.CheckInProgress = false
			state.LastCheckError = ""
			return nil
		}
		if time.Since(state.Provision.CreatedAt) < autoProvisionDispatchStaleTTL {
			return infraerrors.Conflict("OPENAI_AUTO_PROVISION_BUSY", "openai provisioning callback is still within the retry window")
		}
		if state.ProvisionDispatchInProgress {
			startedAt := state.ProvisionDispatchStartedAt
			if startedAt == nil && state.Provision != nil {
				startedAt = &state.Provision.CreatedAt
			}
			if startedAt != nil && time.Since(*startedAt) < autoProvisionDispatchStaleTTL {
				return infraerrors.Conflict("OPENAI_AUTO_PROVISION_BUSY", "openai provisioning request is currently being dispatched")
			}
		}
		state.SupersededProvisions[state.Provision.RequestID] = *state.Provision
		state.Provision = nil
		state.ProvisionDispatchInProgress = false
		state.ProvisionDispatchStartedAt = nil
		state.ProvisionRetryRequested = false
		state.CheckInProgress = false
		state.LastCheckError = ""
		return nil
	})
}

type openAIAutoProvisionAccountCounter interface {
	CountSchedulableOpenAIOAuthByPlatform(context.Context, string) (int, error)
}

func (s *OpenAIAutoProvisionService) healthyOpenAIOAuthCount(ctx context.Context) (int, error) {
	if s.accounts == nil {
		return 0, fmt.Errorf("openai account repository is unavailable")
	}
	if counter, ok := s.accounts.(openAIAutoProvisionAccountCounter); ok {
		return counter.CountSchedulableOpenAIOAuthByPlatform(ctx, PlatformOpenAI)
	}
	accounts, err := s.accounts.ListSchedulableByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return 0, err
	}
	return countHealthyOpenAIOAuthAccounts(accounts), nil
}

func joinRuntimeErrors(existing string, runtimeErr error) string {
	if runtimeErr == nil {
		return existing
	}
	message := strings.TrimSpace(runtimeErr.Error())
	if existing == "" {
		return message
	}
	return fmt.Sprintf("%s; %s", existing, message)
}
