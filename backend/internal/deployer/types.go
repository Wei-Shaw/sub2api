package deployer

import "time"

const (
	JobStatusRunning        = "running"
	JobStatusSucceeded      = "succeeded"
	JobStatusFailed         = "failed"
	JobStatusRollbackFailed = "rollback_failed"
	JobStatusDegraded       = "degraded"
)

const (
	TrafficStateOld            = "old"
	TrafficStateSwitchPending  = "switch_pending"
	TrafficStateCandidate      = "candidate"
	TrafficStateRestorePending = "restore_pending"
	TrafficStateUnknown        = "unknown"
)

const (
	StagePulling           = "pulling"
	StagePreparing         = "preparing"
	StageStartingCandidate = "starting_candidate"
	StageCheckingCandidate = "checking_candidate"
	StageSwitchingTraffic  = "switching_traffic"
	StageStabilizing       = "stabilizing"
	StageDraining          = "draining"
	StageActivating        = "activating_background"
	StageRollingBack       = "rolling_back"
	StageCompleted         = "completed"
	StageFailed            = "failed"
)

type Slot struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

type DeployRequest struct {
	Action                 string `json:"action"`
	TargetVersion          string `json:"target_version"`
	ExpectedCurrentVersion string `json:"expected_current_version,omitempty"`
	RequestID              string `json:"request_id"`
}

type Job struct {
	ID                   string     `json:"id"`
	Action               string     `json:"action"`
	TargetVersion        string     `json:"target_version"`
	ExpectedCurrent      string     `json:"expected_current_version,omitempty"`
	Status               string     `json:"status"`
	Stage                string     `json:"stage"`
	Message              string     `json:"message,omitempty"`
	Error                string     `json:"error,omitempty"`
	FromVersion          string     `json:"from_version,omitempty"`
	FromImage            string     `json:"from_image,omitempty"`
	TargetImage          string     `json:"target_image,omitempty"`
	TargetDigest         string     `json:"target_digest,omitempty"`
	OldContainer         string     `json:"old_container,omitempty"`
	OldContainerID       string     `json:"old_container_id,omitempty"`
	HandoffPrepared      bool       `json:"handoff_prepared"`
	HandoffContainer     string     `json:"handoff_container,omitempty"`
	HandoffContainerID   string     `json:"handoff_container_id,omitempty"`
	OldSlot              string     `json:"old_slot,omitempty"`
	OldRuntimeSlot       string     `json:"old_runtime_slot,omitempty"`
	OldSlotCaptured      bool       `json:"old_runtime_slot_captured"`
	CandidateContainer   string     `json:"candidate_container,omitempty"`
	CandidateContainerID string     `json:"candidate_container_id,omitempty"`
	CandidateSlot        string     `json:"candidate_slot,omitempty"`
	CandidatePort        int        `json:"candidate_port,omitempty"`
	TrafficState         string     `json:"traffic_state,omitempty"`
	TrafficSwitched      bool       `json:"traffic_switched"`
	BackgroundActivated  bool       `json:"background_activated"`
	RollbackPerformed    bool       `json:"rollback_performed"`
	RollbackError        string     `json:"rollback_error,omitempty"`
	CleanupWarning       string     `json:"cleanup_warning,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	StartedAt            time.Time  `json:"started_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	FinishedAt           *time.Time `json:"finished_at,omitempty"`
}

type State struct {
	ActiveSlot          string    `json:"active_slot"`
	ActiveContainer     string    `json:"active_container"`
	ActiveContainerID   string    `json:"active_container_id,omitempty"`
	ActivePort          int       `json:"active_port"`
	ActiveVersion       string    `json:"active_version,omitempty"`
	ActiveImage         string    `json:"active_image,omitempty"`
	PreviousSlot        string    `json:"previous_slot,omitempty"`
	PreviousContainer   string    `json:"previous_container,omitempty"`
	PreviousContainerID string    `json:"previous_container_id,omitempty"`
	PreviousPort        int       `json:"previous_port,omitempty"`
	PreviousVersion     string    `json:"previous_version,omitempty"`
	PreviousImage       string    `json:"previous_image,omitempty"`
	Degraded            bool      `json:"degraded"`
	DegradedReason      string    `json:"degraded_reason,omitempty"`
	Job                 *Job      `json:"job,omitempty"`
	JobHistory          []Job     `json:"job_history,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Health struct {
	Status          string `json:"status"`
	Version         string `json:"version"`
	ActiveSlot      string `json:"active_slot,omitempty"`
	ActiveContainer string `json:"active_container,omitempty"`
	ActivePort      int    `json:"active_port,omitempty"`
	ActiveVersion   string `json:"active_version,omitempty"`
	JobRunning      bool   `json:"job_running"`
	Degraded        bool   `json:"degraded"`
	DegradedReason  string `json:"degraded_reason,omitempty"`
}
