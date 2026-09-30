package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
)

type excelBPSAttachmentError struct {
	status     int
	retryAfter string
	stage      string
	cause      error
}

func (e *excelBPSAttachmentError) Error() string {
	return fmt.Sprintf("excel BPS attachment failed: %s (status %d)", e.stage, e.status)
}

func (e *excelBPSAttachmentError) Unwrap() error { return e.cause }

// Retain only context sentinels, never a raw URL, response body, file ID or
// credential-bearing transport error.
func excelBPSAttachmentFailure(stage string, cause error) *excelBPSAttachmentError {
	failure := &excelBPSAttachmentError{status: http.StatusBadGateway, stage: stage}
	if errors.Is(cause, context.Canceled) {
		failure.cause = context.Canceled
	} else if errors.Is(cause, context.DeadlineExceeded) {
		failure.cause = context.DeadlineExceeded
	}
	return failure
}

func (s *OpenAIGatewayService) uploadExcelBPSAttachment(ctx context.Context, account *Account, token, accountID, proxyURL string, img basispoints.InlineAttachment) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	ctx = WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileLongStream))
	reader, contentType, length, err := img.Multipart()
	if err != nil {
		return "", excelBPSAttachmentFailure("attachment_prepare", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, basispoints.AttachmentsURL, reader)
	if err != nil {
		return "", excelBPSAttachmentFailure("attachment_prepare", err)
	}
	req.Header = excelBPSHeaders(token, accountID, "application/json")
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept-Encoding", "identity")
	req.ContentLength = length
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return "", excelBPSAttachmentFailure("attachment_transport", err)
	}
	if resp == nil || resp.Body == nil {
		return "", excelBPSAttachmentFailure("attachment_response", nil)
	}
	// Release the account's upstream connection slot before starting Responses.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		s.handleExcelBPSUnauthorized(ctx, account, resp.StatusCode, resp.Header, raw)
		failure := excelBPSAttachmentFailure("attachment_http", nil)
		if resp.StatusCode >= 400 && resp.StatusCode <= 599 {
			failure.status = resp.StatusCode
		}
		failure.retryAfter = resp.Header.Get("Retry-After")
		return "", failure
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return "", excelBPSAttachmentFailure("attachment_response", err)
	}
	var result struct {
		OpenAIFileID string `json:"openai_file_id"`
	}
	if json.Unmarshal(raw, &result) != nil || !basispoints.ValidAttachmentID(result.OpenAIFileID) {
		return "", excelBPSAttachmentFailure("attachment_response", nil)
	}
	return result.OpenAIFileID, nil
}
