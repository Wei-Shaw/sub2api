// Package openaiasync implements local Files and Batch persistence. Batch
// execution uses the application's normal authenticated gateway and prices.
package openaiasync

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound  = errors.New("resource not found")
	ErrQuota     = errors.New("local storage, temporary batch storage, or active batch quota exceeded")
	ErrLeaseLost = errors.New("batch execution lease lost")
)

func NewID(prefix string) string { return prefix + uuid.NewString() }

type File struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Bytes     int64  `json:"bytes"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	Filename  string `json:"filename"`
	Purpose   string `json:"purpose"`
	Status    string `json:"status"`
}

type Counts struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

type Batch struct {
	ID               string            `json:"id"`
	Object           string            `json:"object"`
	Endpoint         string            `json:"endpoint"`
	Errors           any               `json:"errors"`
	InputFileID      string            `json:"input_file_id"`
	CompletionWindow string            `json:"completion_window"`
	Status           string            `json:"status"`
	OutputFileID     *string           `json:"output_file_id"`
	ErrorFileID      *string           `json:"error_file_id"`
	CreatedAt        int64             `json:"created_at"`
	InProgressAt     *int64            `json:"in_progress_at"`
	ExpiresAt        int64             `json:"expires_at"`
	FinalizingAt     *int64            `json:"finalizing_at"`
	CompletedAt      *int64            `json:"completed_at"`
	FailedAt         *int64            `json:"failed_at"`
	ExpiredAt        *int64            `json:"expired_at"`
	CancellingAt     *int64            `json:"cancelling_at"`
	CancelledAt      *int64            `json:"cancelled_at"`
	RequestCounts    Counts            `json:"request_counts"`
	Metadata         map[string]string `json:"metadata"`
}

type Job struct {
	Batch
	UserID       int64
	APIKeyID     int64
	GroupID      int64
	RemoteIP     string
	Lease        string
	OutputTTL    time.Duration
	InputBytes   int64
	OutputBudget int64
}

type Item struct {
	Ordinal  int
	CustomID string          `json:"custom_id"`
	Method   string          `json:"method"`
	URL      string          `json:"url"`
	Body     json.RawMessage `json:"body"`
}

type Result struct {
	ID       string     `json:"id"`
	CustomID string     `json:"custom_id"`
	Response *Response  `json:"response"`
	Error    *ItemError `json:"error"`
}

type Response struct {
	StatusCode int             `json:"status_code"`
	RequestID  string          `json:"request_id"`
	Body       json.RawMessage `json:"body"`
}

type ItemError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Failure(customID, code, message string) Result {
	return Result{ID: NewID("batch_req_"), CustomID: customID, Error: &ItemError{Code: code, Message: message}}
}

func Terminal(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "expired":
		return true
	}
	return false
}
