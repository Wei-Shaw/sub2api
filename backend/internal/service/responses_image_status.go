package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

const ResponsesImageStatusTTL = 7 * 24 * time.Hour

var ErrResponsesImageStatusNotFound = errors.New("responses image status not found")

const (
	ResponsesImageStatusAccepted     = "accepted"
	ResponsesImageStatusRunning      = "running"
	ResponsesImageStatusUpstreamDone = "upstream_done"
	ResponsesImageStatusCOSUploading = "cos_uploading"
	ResponsesImageStatusSucceeded    = "succeeded"
	ResponsesImageStatusFailed       = "failed"
)

type ResponsesImageStatusError struct {
	Message string `json:"message"`
}

type ResponsesImageStatus struct {
	RequestID string                     `json:"request_id"`
	Status    string                     `json:"status"`
	Progress  int                        `json:"progress"`
	URLs      []string                   `json:"urls,omitempty"`
	COSURLs   []string                   `json:"cos_urls,omitempty"`
	Error     *ResponsesImageStatusError `json:"error,omitempty"`
	CreatedAt time.Time                  `json:"created_at"`
	UpdatedAt time.Time                  `json:"updated_at"`
}

type ResponsesImageStatusStore interface {
	GetResponsesImageStatus(ctx context.Context, requestID string) (*ResponsesImageStatus, error)
	SetResponsesImageStatus(ctx context.Context, status *ResponsesImageStatus, ttl time.Duration) error
}

type responsesImageStatusContextKey struct{}

func WithResponsesImageStatusRequestID(ctx context.Context, requestID string) context.Context {
	requestID = strings.TrimSpace(requestID)
	if ctx == nil || requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, responsesImageStatusContextKey{}, requestID)
}

func ResponsesImageStatusRequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(responsesImageStatusContextKey{}).(string)
	return strings.TrimSpace(requestID)
}

func (s *OpenAIGatewayService) BeginResponsesImageStatus(ctx context.Context, requestID string) {
	s.setResponsesImageStatusBestEffort(ctx, &ResponsesImageStatus{
		RequestID: strings.TrimSpace(requestID),
		Status:    ResponsesImageStatusAccepted,
		Progress:  0,
	})
}

func (s *OpenAIGatewayService) MarkResponsesImageStatusRunning(ctx context.Context, requestID string) {
	s.patchResponsesImageStatusBestEffort(ctx, requestID, func(status *ResponsesImageStatus) {
		status.Status = ResponsesImageStatusRunning
		status.Progress = max(status.Progress, 25)
		status.Error = nil
	})
}

func (s *OpenAIGatewayService) MarkResponsesImageStatusUpstreamDone(ctx context.Context, result *OpenAIForwardResult) {
	requestID := responsesImageStatusRequestIDFromResult(ctx, result)
	if requestID == "" {
		return
	}
	urls := cloneNonEmptyStrings(nil)
	if result != nil {
		urls = cloneNonEmptyStrings(result.ImageOutputURLs)
		result.ImageStatusRequestID = requestID
	}
	s.patchResponsesImageStatusBestEffort(ctx, requestID, func(status *ResponsesImageStatus) {
		status.Status = ResponsesImageStatusUpstreamDone
		status.Progress = max(status.Progress, 70)
		status.URLs = urls
		status.Error = nil
	})
}

func (s *OpenAIGatewayService) MarkResponsesImageStatusCOSUploading(ctx context.Context, result *OpenAIForwardResult) {
	requestID := responsesImageStatusRequestIDFromResult(ctx, result)
	if requestID == "" {
		return
	}
	s.patchResponsesImageStatusBestEffort(ctx, requestID, func(status *ResponsesImageStatus) {
		status.Status = ResponsesImageStatusCOSUploading
		status.Progress = max(status.Progress, 85)
		if result != nil {
			status.URLs = cloneNonEmptyStrings(result.ImageOutputURLs)
		}
		status.Error = nil
	})
}

func (s *OpenAIGatewayService) SucceedResponsesImageStatus(ctx context.Context, result *OpenAIForwardResult) {
	requestID := responsesImageStatusRequestIDFromResult(ctx, result)
	if requestID == "" {
		return
	}
	var urls, cosURLs []string
	if result != nil {
		urls = cloneNonEmptyStrings(result.ImageOutputURLs)
		cosURLs = cloneNonEmptyStrings(result.ImageOutputCosURLs)
	}
	s.patchResponsesImageStatusBestEffort(ctx, requestID, func(status *ResponsesImageStatus) {
		status.Status = ResponsesImageStatusSucceeded
		status.Progress = 100
		status.URLs = urls
		status.COSURLs = cosURLs
		status.Error = nil
	})
}

func (s *OpenAIGatewayService) FailResponsesImageStatus(ctx context.Context, requestID, message string) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "image generation failed"
	}
	s.patchResponsesImageStatusBestEffort(ctx, requestID, func(status *ResponsesImageStatus) {
		status.Status = ResponsesImageStatusFailed
		status.Progress = 100
		status.Error = &ResponsesImageStatusError{Message: message}
	})
}

func (s *OpenAIGatewayService) GetResponsesImageStatus(ctx context.Context, requestID string) (*ResponsesImageStatus, error) {
	if s == nil || s.responsesImageStatusStore == nil {
		return nil, ErrResponsesImageStatusNotFound
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, ErrResponsesImageStatusNotFound
	}
	return s.responsesImageStatusStore.GetResponsesImageStatus(ctx, requestID)
}

func (s *OpenAIGatewayService) setResponsesImageStatusBestEffort(ctx context.Context, status *ResponsesImageStatus) {
	if s == nil || s.responsesImageStatusStore == nil || status == nil {
		return
	}
	status.RequestID = strings.TrimSpace(status.RequestID)
	if status.RequestID == "" {
		return
	}
	now := time.Now().UTC()
	if status.CreatedAt.IsZero() {
		status.CreatedAt = now
	}
	status.UpdatedAt = now
	if status.Progress < 0 {
		status.Progress = 0
	}
	if status.Progress > 100 {
		status.Progress = 100
	}
	_ = s.responsesImageStatusStore.SetResponsesImageStatus(ctx, status, ResponsesImageStatusTTL)
}

func (s *OpenAIGatewayService) patchResponsesImageStatusBestEffort(ctx context.Context, requestID string, patch func(*ResponsesImageStatus)) {
	if s == nil || s.responsesImageStatusStore == nil {
		return
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return
	}
	status, err := s.responsesImageStatusStore.GetResponsesImageStatus(ctx, requestID)
	if err != nil || status == nil {
		status = &ResponsesImageStatus{RequestID: requestID}
	}
	if status.RequestID == "" {
		status.RequestID = requestID
	}
	if patch != nil {
		patch(status)
	}
	s.setResponsesImageStatusBestEffort(ctx, status)
}

func responsesImageStatusRequestIDFromResult(ctx context.Context, result *OpenAIForwardResult) string {
	if result != nil && strings.TrimSpace(result.ImageStatusRequestID) != "" {
		return strings.TrimSpace(result.ImageStatusRequestID)
	}
	return ResponsesImageStatusRequestIDFromContext(ctx)
}

func cloneNonEmptyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
