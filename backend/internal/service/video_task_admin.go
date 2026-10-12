package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// VideoTaskAdminFilter is the GET /api/v1/admin/video-tasks contract.
// Defaults: page=1, page_size=20, days=7. Search is a case-insensitive literal
// substring of task ID, displayed model, user email/username/ID or account name/ID.
type VideoTaskAdminFilter struct {
	Page     int
	PageSize int
	Search   string
	Status   string
	Days     int
}

func (f VideoTaskAdminFilter) Validate() error {
	if f.Page < 1 || f.Page > 2147483647 {
		return errors.New("page must be between 1 and 2147483647")
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		return errors.New("page_size must be between 1 and 100")
	}
	if !utf8.ValidString(f.Search) || strings.ContainsRune(f.Search, '\x00') || utf8.RuneCountInString(f.Search) > 100 {
		return errors.New("search must be at most 100 characters")
	}
	if f.Days != 1 && f.Days != 7 && f.Days != 30 {
		return errors.New("days must be 1, 7 or 30")
	}
	switch f.Status {
	case "", "in_progress", "delayed", "query_retry", "billing_pending", "completed", "failed", "submission_unknown", "review_required":
		return nil
	default:
		return errors.New("invalid video task status")
	}
}

// VideoTaskAdminItem is an explicit allowlist; never embed a task, submission,
// snapshot or usage log here. All fields are always present in JSON.
//
// Status precedence: completed/failed > ready (billing_pending) > pending with
// any last_error (query_retry) > pending older than 2h (delayed) > in_progress.
// reason_code exposes only known internal codes, not arbitrary last_error text.
// Current whitelist: poll_failed, poll_timeout, upstream_failed,
// upstream_cancelled, upstream_expired, completion_save_failed, billing_failed.
//
// Billing is independent of display_status: an exact usage row means charged,
// including a ready task whose finish failed. Without a usage row, ready is
// pending, completed is review, and pending/failed are not_charged. There is no
// refund implication for an uncharged failure. A failed task WITH a usage row is
// review; retain its charged_cost and usage_log_id, never automatically refund.
// Site billing occurs only on completion; upstream costs may already have been
// incurred independently, especially for unresolved submissions.
// Unresolved submissions are review
// (possibly still in flight), never automatically retried by this read endpoint.
//
// Submissions have task_id/account_name/group_name/resolution="", null
// model/account/group/cost/duration/usage/completion/next-attempt fields, attempts=0,
// state=display_status=submission_unknown, updated_at=created_at (only known
// timestamp). Their user is recovered from api_keys including soft-deleted keys;
// user_id is null and user_email empty if a physically deleted key lost ownership.
type VideoTaskAdminItem struct {
	Kind                 string     `json:"kind"`
	TaskID               string     `json:"task_id"`
	UserID               *int64     `json:"user_id"`
	UserEmail            string     `json:"user_email"`
	APIKeyID             int64      `json:"api_key_id"`
	APIKeyName           string     `json:"api_key_name"`
	AccountID            *int64     `json:"account_id"`
	AccountName          string     `json:"account_name"`
	GroupID              *int64     `json:"group_id"`
	GroupName            string     `json:"group_name"`
	Model                *string    `json:"model"`
	State                string     `json:"state"`
	DisplayStatus        string     `json:"display_status"`
	ReasonCode           string     `json:"reason_code"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	CompletedAt          *time.Time `json:"completed_at"`
	NextAttemptAt        *time.Time `json:"next_attempt_at"`
	Attempts             int        `json:"attempts"`
	ExpectedCost         *float64   `json:"expected_cost"`
	ChargedCost          *float64   `json:"charged_cost"`
	UsageLogID           *int64     `json:"usage_log_id"`
	BillingStatus        string     `json:"billing_status"`
	VideoDurationSeconds *int       `json:"video_duration_seconds"`
	VideoResolution      string     `json:"video_resolution"`
}

// VideoTaskAdminRepository is optional on UsageBillingRepository; it introduces
// no dependency on the worker or its mutation interface.
type VideoTaskAdminRepository interface {
	ListVideoTasks(context.Context, VideoTaskAdminFilter) ([]VideoTaskAdminItem, int64, error)
}

func (s *OpenAIGatewayService) ListAdminVideoTasks(ctx context.Context, f VideoTaskAdminFilter) ([]VideoTaskAdminItem, int64, error) {
	if err := f.Validate(); err != nil {
		return nil, 0, err
	}
	if s != nil {
		if repo, ok := s.usageBillingRepo.(VideoTaskAdminRepository); ok && repo != nil {
			return repo.ListVideoTasks(ctx, f)
		}
	}
	return nil, 0, ErrVideoTaskRepositoryUnavailable
}
