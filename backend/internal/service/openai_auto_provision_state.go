package service

import (
	"fmt"
	"strings"
	"time"
)

const (
	autoProvisionStateVersion = 1
	autoProvisionStateKey     = "openai_auto_provision_runtime_state"
	autoProvisionRunLockKey   = "openai:auto-provision:run"
	autoProvisionStateLockKey = "openai:auto-provision:state"
	autoProvisionLockTTL      = 30 * time.Second
	autoProvisionPendingTTL   = 2 * time.Hour
	maxAutoProvisionBatch     = 100
	maxAutoReauthorization    = 20
)

type autoProvisionState struct {
	Version          int                                   `json:"version"`
	Provision        *autoProvisionPending                 `json:"provision,omitempty"`
	Reauthorizations map[string]autoReauthorizationPending `json:"reauthorizations,omitempty"`
	ProcessedEvents  map[string]time.Time                  `json:"processed_events,omitempty"`
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
		Version:          autoProvisionStateVersion,
		Reauthorizations: make(map[string]autoReauthorizationPending),
		ProcessedEvents:  make(map[string]time.Time),
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
		if state.Provision == nil || state.Provision.RequestID != requestID {
			return false, false, fmt.Errorf("registration request is not pending")
		}
		state.Provision = nil
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
	state.ProcessedEvents[eventID] = time.Now().UTC()
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
