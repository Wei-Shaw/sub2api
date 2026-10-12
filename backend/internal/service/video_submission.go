package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var ErrVideoSubmissionConflict = errors.New("video idempotency key was used with another request")
var ErrVideoSubmissionPending = errors.New("video submission is in progress or its outcome is unknown; do not resubmit with a new key")

type VideoSubmissionRepository interface {
	ClaimVideoSubmission(context.Context, int64, string, string) (bool, string, error)
	CompleteVideoSubmission(context.Context, int64, string, string) error
}

// BeginVideoSubmission returns the existing task for a completed replay. It never
// expires or steals an in-flight claim, including when the original HTTP call timed out.
func (s *OpenAIGatewayService) BeginVideoSubmission(ctx context.Context, keyID, groupID int64, key, contentType string, body []byte) (string, error) {
	if key == "" {
		return "", nil
	}
	if len(key) > 200 || strings.TrimSpace(key) != key {
		return "", fmt.Errorf("invalid video Idempotency-Key")
	}
	repo, ok := s.usageBillingRepo.(VideoSubmissionRepository)
	if !ok {
		return "", errors.New("durable video submission store unavailable")
	}
	digest := sha256.New()
	_, _ = fmt.Fprintf(digest, "%d\x00%s\x00", groupID, contentType)
	_, _ = digest.Write(body)
	claimed, taskID, err := repo.ClaimVideoSubmission(ctx, keyID, videoSubmissionKeyHash(key), hex.EncodeToString(digest.Sum(nil)))
	if err != nil {
		return "", err
	}
	if !claimed && taskID == "" {
		return "", ErrVideoSubmissionPending
	}
	return taskID, nil
}

func (s *OpenAIGatewayService) CompleteVideoSubmission(ctx context.Context, keyID int64, key, taskID string) error {
	if key == "" {
		return nil
	}
	repo, ok := s.usageBillingRepo.(VideoSubmissionRepository)
	if !ok {
		return errors.New("durable video submission store unavailable")
	}
	return repo.CompleteVideoSubmission(ctx, keyID, videoSubmissionKeyHash(key), taskID)
}

func videoSubmissionKeyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
