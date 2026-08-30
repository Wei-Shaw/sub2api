package service

import (
	"fmt"
	"strings"
	"time"
)

const (
	autoProvisionStateVersion     = 1
	autoProvisionStateKey         = "openai_auto_provision_runtime_state"
	autoProvisionRunLockKey       = "openai:auto-provision:run"
	autoProvisionStateLockKey     = "openai:auto-provision:state"
	autoProvisionLockTTL          = 30 * time.Second
	autoProvisionPendingTTL       = 2 * time.Hour
	autoProvisionDispatchStaleTTL = 10 * time.Minute
	maxAutoProvisionBatch         = 100
	maxAutoReauthorization        = 20
)

type autoProvisionState struct {
	Version                     int                                   `json:"version"`
	Provision                   *autoProvisionPending                 `json:"provision,omitempty"`
	SupersededProvisions        map[string]autoProvisionPending       `json:"superseded_provisions,omitempty"`
	Reauthorizations            map[string]autoReauthorizationPending `json:"reauthorizations,omitempty"`
	ProcessedEvents             map[string]time.Time                  `json:"processed_events,omitempty"`
	CheckInProgress             bool                                  `json:"check_in_progress,omitempty"`
	ProvisionDispatchInProgress bool                                  `json:"provision_dispatch_in_progress,omitempty"`
	ProvisionDispatchStartedAt  *time.Time                            `json:"provision_dispatch_started_at,omitempty"`
	ProvisionRetryRequested     bool                                  `json:"provision_retry_requested,omitempty"`
	LastCheckStartedAt          *time.Time                            `json:"last_check_started_at,omitempty"`
	LastCheckCompletedAt        *time.Time                            `json:"last_check_completed_at,omitempty"`
	LastCheckError              string                                `json:"last_check_error,omitempty"`
	LastProvisionRequestedCount int                                   `json:"last_provision_requested_count,omitempty"`
	LastProvisionRequestedAt    *time.Time                            `json:"last_provision_requested_at,omitempty"`
	LastCallbackAt              *time.Time                            `json:"last_callback_at,omitempty"`
	LastCallbackKind            string                                `json:"last_callback_kind,omitempty"`
	LastCallbackStatus          string                                `json:"last_callback_status,omitempty"`
	LastCallbackRequestedCount  int                                   `json:"last_callback_requested_count,omitempty"`
	LastCallbackSucceededCount  int                                   `json:"last_callback_succeeded_count,omitempty"`
	LastCallbackFailedCount     int                                   `json:"last_callback_failed_count,omitempty"`
	LastCallbackPendingCount    int                                   `json:"last_callback_pending_count,omitempty"`
}

type autoProvisionPending struct {
	RequestID      string    `json:"request_id"`
	RequestedCount int       `json:"requested_count"`
	CreatedAt      time.Time `json:"created_at"`
}

type autoReauthorizationPending struct {
	RequestID string    `json:"request_id"`
	AccountID int64     `json:"account_id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// OpenAIAutoProvisionCallback is the terminal event sent by turb. It never
// contains OAuth credentials; reauthorization codes use the separate callback.
type OpenAIAutoProvisionCallback struct {
	RequestID      string `json:"request_id"`
	EventID        string `json:"event_id"`
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	RequestedCount int    `json:"requested_count,omitempty"`
	SucceededCount int    `json:"succeeded_count,omitempty"`
	FailedCount    int    `json:"failed_count,omitempty"`
	PendingCount   int    `json:"pending_count,omitempty"`
	AccountID      int64  `json:"account_id,omitempty"`
	Email          string `json:"email,omitempty"`
	Error          string `json:"error,omitempty"`
}

type OpenAIAutoReauthorizationCallback struct {
	RequestID   string `json:"request_id"`
	EventID     string `json:"event_id"`
	AccountID   int64  `json:"account_id"`
	Email       string `json:"email"`
	SessionID   string `json:"session_id"`
	CallbackURL string `json:"callback_url"`
	RedirectURI string `json:"redirect_uri,omitempty"`
}

type autoProvisionCallback = OpenAIAutoProvisionCallback

func newAutoProvisionState() *autoProvisionState {
	return &autoProvisionState{
		Version:              autoProvisionStateVersion,
		SupersededProvisions: make(map[string]autoProvisionPending),
		Reauthorizations:     make(map[string]autoReauthorizationPending),
		ProcessedEvents:      make(map[string]time.Time),
	}
}

func normalizeAutoProvisionState(state *autoProvisionState) *autoProvisionState {
	if state == nil {
		return newAutoProvisionState()
	}
	if state.Version == 0 {
		state.Version = autoProvisionStateVersion
	}
	if state.Reauthorizations == nil {
		state.Reauthorizations = make(map[string]autoReauthorizationPending)
	}
	if state.SupersededProvisions == nil {
		state.SupersededProvisions = make(map[string]autoProvisionPending)
	}
	if state.ProcessedEvents == nil {
		state.ProcessedEvents = make(map[string]time.Time)
	}
	return state
}

func consumeAutoProvisionCallback(state *autoProvisionState, callback OpenAIAutoProvisionCallback) (accepted, replay bool, err error) {
	state = normalizeAutoProvisionState(state)
	requestID := strings.TrimSpace(callback.RequestID)
	eventID := strings.TrimSpace(callback.EventID)
	kind := strings.TrimSpace(callback.Kind)
	status := strings.TrimSpace(callback.Status)
	if requestID == "" || eventID == "" {
		return false, false, fmt.Errorf("request_id and event_id are required")
	}
	if _, ok := state.ProcessedEvents[eventID]; ok {
		return true, true, nil
	}
	if kind != "registration" && kind != "reauthorization" {
		return false, false, fmt.Errorf("unsupported callback kind")
	}
	if kind == "registration" {
		if status != "completed" {
			return false, false, fmt.Errorf("registration callback status must be completed")
		}
		if state.Provision != nil && state.Provision.RequestID == requestID {
			state.Provision = nil
			state.ProvisionDispatchInProgress = false
			state.ProvisionDispatchStartedAt = nil
			state.ProvisionRetryRequested = false
		} else if _, ok := state.SupersededProvisions[requestID]; ok {
			delete(state.SupersededProvisions, requestID)
		} else {
			return false, false, fmt.Errorf("registration request is not pending")
		}
	} else {
		if status != "succeeded" && status != "failed" {
			return false, false, fmt.Errorf("reauthorization callback status is invalid")
		}
		if pending, ok := state.Reauthorizations[requestID]; ok {
			if pending.AccountID != callback.AccountID {
				return false, false, fmt.Errorf("reauthorization account does not match request")
			}
			delete(state.Reauthorizations, requestID)
		} else if !hasProcessedAutoReauthorizationCallback(state, requestID) {
			return false, false, fmt.Errorf("reauthorization request is not pending")
		}
	}
	now := time.Now().UTC()
	state.LastCallbackAt = &now
	state.LastCallbackKind = kind
	state.LastCallbackStatus = status
	state.LastCallbackRequestedCount = callback.RequestedCount
	state.LastCallbackSucceededCount = callback.SucceededCount
	state.LastCallbackFailedCount = callback.FailedCount
	state.LastCallbackPendingCount = callback.PendingCount
	state.LastCheckError = ""
	state.ProcessedEvents[eventID] = now
	return true, false, nil
}

func hasProcessedAutoReauthorizationCallback(state *autoProvisionState, requestID string) bool {
	prefix := strings.TrimSpace(requestID) + ":reauthorization:oauth-callback"
	_, ok := state.ProcessedEvents[prefix]
	return ok
}

func pruneAutoProvisionState(state *autoProvisionState, now time.Time) {
	state = normalizeAutoProvisionState(state)
	if state.Provision != nil && now.Sub(state.Provision.CreatedAt) > autoProvisionPendingTTL {
		state.Provision = nil
	}
	for requestID, pending := range state.SupersededProvisions {
		if now.Sub(pending.CreatedAt) > autoProvisionPendingTTL {
			delete(state.SupersededProvisions, requestID)
		}
	}
	for requestID, pending := range state.Reauthorizations {
		if now.Sub(pending.CreatedAt) > autoProvisionPendingTTL {
			delete(state.Reauthorizations, requestID)
		}
	}
	for eventID, processedAt := range state.ProcessedEvents {
		if now.Sub(processedAt) > 24*time.Hour {
			delete(state.ProcessedEvents, eventID)
		}
	}
}

func countHealthyOpenAIOAuthAccounts(accounts []Account) int {
	count := 0
	for index := range accounts {
		account := &accounts[index]
		if account.IsOpenAIOAuth() && account.Status == StatusActive && account.Schedulable && !account.IsCredentialShadow() {
			count++
		}
	}
	return count
}

func openAIProvisionDeficit(healthy, target int) int {
	if target <= healthy {
		return 0
	}
	return target - healthy
}
