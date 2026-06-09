package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// EmailBroadcast is the admin-facing JSON representation of a broadcast record.
type EmailBroadcast struct {
	ID               int64      `json:"id"`
	Subject          string     `json:"subject"`
	Body             string     `json:"body"`
	BodyFormat       string     `json:"body_format"`
	RecipientsMode   string     `json:"recipients_mode"`
	RecipientUserIDs []int64    `json:"recipient_user_ids,omitempty"`
	Status           string     `json:"status"`
	TotalCount       int        `json:"total_count"`
	SuccessCount     int        `json:"success_count"`
	FailedCount      int        `json:"failed_count"`
	ErrorMessage     *string    `json:"error_message,omitempty"`
	CreatedBy        *int64     `json:"created_by,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// EmailBroadcastSummary is the trimmed representation returned in list responses
// (omits the full body to keep payloads small).
type EmailBroadcastSummary struct {
	ID             int64      `json:"id"`
	Subject        string     `json:"subject"`
	BodyFormat     string     `json:"body_format"`
	RecipientsMode string     `json:"recipients_mode"`
	Status         string     `json:"status"`
	TotalCount     int        `json:"total_count"`
	SuccessCount   int        `json:"success_count"`
	FailedCount    int        `json:"failed_count"`
	CreatedBy      *int64     `json:"created_by,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func EmailBroadcastFromService(b *service.EmailBroadcast) *EmailBroadcast {
	if b == nil {
		return nil
	}
	return &EmailBroadcast{
		ID:               b.ID,
		Subject:          b.Subject,
		Body:             b.Body,
		BodyFormat:       b.BodyFormat,
		RecipientsMode:   b.RecipientsMode,
		RecipientUserIDs: append([]int64(nil), b.RecipientUserIDs...),
		Status:           b.Status,
		TotalCount:       b.TotalCount,
		SuccessCount:     b.SuccessCount,
		FailedCount:      b.FailedCount,
		ErrorMessage:     b.ErrorMessage,
		CreatedBy:        b.CreatedBy,
		StartedAt:        b.StartedAt,
		FinishedAt:       b.FinishedAt,
		CreatedAt:        b.CreatedAt,
		UpdatedAt:        b.UpdatedAt,
	}
}

func EmailBroadcastSummaryFromService(b *service.EmailBroadcast) EmailBroadcastSummary {
	return EmailBroadcastSummary{
		ID:             b.ID,
		Subject:        b.Subject,
		BodyFormat:     b.BodyFormat,
		RecipientsMode: b.RecipientsMode,
		Status:         b.Status,
		TotalCount:     b.TotalCount,
		SuccessCount:   b.SuccessCount,
		FailedCount:    b.FailedCount,
		CreatedBy:      b.CreatedBy,
		StartedAt:      b.StartedAt,
		FinishedAt:     b.FinishedAt,
		CreatedAt:      b.CreatedAt,
	}
}
