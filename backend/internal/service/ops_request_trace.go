package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OpsRequestTrace 是「一次请求的完整追溯视图」。
//
// 它把四张按 request_id 关联的表拼成一个响应，回答支持工程师真正会问的问题：
// 这条请求打到了哪个账号（attempts）、最终成功与否（usage）、上游为什么拒绝
// （error log + error_code）、以及过程中账号状态是否发生了变化（status events）。
//
// 设计上新表（upstream_attempts / account_status_events）不替代 usage_logs 与
// ops_error_logs，而是补上「按账号可索引、可聚合」的那一层；这里只做聚合读取。
type OpsRequestTrace struct {
	RequestID    string                 `json:"request_id"`
	Usage        *OpsTraceUsage         `json:"usage,omitempty"`
	Attempts     []*OpsTraceAttempt     `json:"attempts"`
	StatusEvents []*OpsTraceStatusEvent `json:"status_events"`
	Error        *OpsTraceError         `json:"error,omitempty"`
}

// OpsTraceUsage 是 usage_logs 的计费侧摘要（成功请求才有）。
type OpsTraceUsage struct {
	UserID         int64     `json:"user_id"`
	APIKeyID       int64     `json:"api_key_id"`
	AccountID      int64     `json:"account_id"`
	Model          string    `json:"model"`
	RequestedModel string    `json:"requested_model,omitempty"`
	UpstreamModel  string    `json:"upstream_model,omitempty"`
	InputTokens    int       `json:"input_tokens"`
	OutputTokens   int       `json:"output_tokens"`
	TotalCost      float64   `json:"total_cost"`
	DurationMs     int       `json:"duration_ms"`
	FirstTokenMs   int       `json:"first_token_ms,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// OpsTraceAttempt 是 upstream_attempts 的一行。
type OpsTraceAttempt struct {
	AttemptNo      int        `json:"attempt_no"`
	AccountID      *int64     `json:"account_id,omitempty"`
	RetryFrom      *int64     `json:"retry_from,omitempty"`
	Platform       string     `json:"platform"`
	ModelReq       string     `json:"model_req,omitempty"`
	ModelUpstream  *string    `json:"model_upstream,omitempty"`
	UpstreamURL    *string    `json:"upstream_url,omitempty"`
	UpstreamStatus *int       `json:"upstream_status,omitempty"`
	FailureKind    string     `json:"failure_kind"`
	ErrorCode      *string    `json:"error_code,omitempty"`
	ErrorMsg       *string    `json:"error_msg,omitempty"`
	AccountState   *string    `json:"account_state,omitempty"`
	CoolingUntil   *time.Time `json:"cooling_until,omitempty"`
	CredentialFP   *string    `json:"credential_fp,omitempty"`
	ProxyID        *int64     `json:"proxy_id,omitempty"`
	// RequestID 在按账号反查时区分各条尝试所属的请求；按 request_id 反查时与外层相同。
	RequestID string    `json:"request_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// OpsTraceStatusEvent 是 account_status_events 的一行。
type OpsTraceStatusEvent struct {
	AccountID    int64      `json:"account_id"`
	FromState    *string    `json:"from_state,omitempty"`
	ToState      string     `json:"to_state"`
	Reason       *string    `json:"reason,omitempty"`
	ErrorCode    *string    `json:"error_code,omitempty"`
	CoolingUntil *time.Time `json:"cooling_until,omitempty"`
	Source       string     `json:"source"`
	RequestID    *string    `json:"request_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// OpsTraceError 是 ops_error_logs 的错误侧摘要。
type OpsTraceError struct {
	ID                 int64     `json:"id"`
	ErrorPhase         string    `json:"error_phase"`
	ErrorType          string    `json:"error_type"`
	StatusCode         *int      `json:"status_code,omitempty"`
	UpstreamStatusCode *int      `json:"upstream_status_code,omitempty"`
	ErrorMessage       *string   `json:"error_message,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// GetRequestTrace 按 request_id 聚合追溯视图。
//
// 找不到任何记录时返回 NotFound，而不是空对象：调用方（支持工程师）需要区分
// 「这条请求没有留下痕迹」和「请求 id 打错了」。
func (s *OpsService) GetRequestTrace(ctx context.Context, requestID string) (*OpsRequestTrace, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, infraerrors.BadRequest("TRACE_REQUEST_ID_REQUIRED", "request_id is required")
	}
	if len(requestID) > 64 {
		return nil, infraerrors.BadRequest("TRACE_REQUEST_ID_INVALID", "request_id is too long")
	}
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.NotFound("TRACE_NOT_FOUND", "request trace not found")
	}

	trace, err := s.opsRepo.GetRequestTrace(ctx, requestID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("TRACE_NOT_FOUND", "request trace not found")
		}
		return nil, infraerrors.InternalServer("TRACE_LOAD_FAILED", "Failed to load request trace").WithCause(err)
	}
	if trace == nil || (trace.Usage == nil && len(trace.Attempts) == 0 && trace.Error == nil && len(trace.StatusEvents) == 0) {
		return nil, infraerrors.NotFound("TRACE_NOT_FOUND", "request trace not found")
	}
	return trace, nil
}
