package service

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
)

// OpenAIVideoCreateUnknownError means a submitted create cannot safely be
// replayed. TaskID is retained when acceptance succeeded but local persistence
// failed, so the durable task integration can reconcile the original job.
// Deliberately do not unwrap Cause into the generic account-failover machinery.
type OpenAIVideoCreateUnknownError struct {
	TaskID string
	Cause  error
}

func (e *OpenAIVideoCreateUnknownError) Error() string {
	return fmt.Sprintf("video create outcome requires reconciliation: %v", e.Cause)
}

func openAIVideoCreateUnknown(c *gin.Context, taskID string, cause error) error {
	setOpsUpstreamError(c, 0, "Video creation outcome is unknown; automatic retry suppressed", "")
	return &OpenAIVideoCreateUnknownError{TaskID: taskID, Cause: cause}
}

// GetOpenAIVideoLookupAccount must only be called after verifying the task's
// owner binding. Creation eligibility (quota, cooldown, scheduling and model
// availability) must not move a previously accepted task to another account.
func (s *OpenAIGatewayService) GetOpenAIVideoLookupAccount(ctx context.Context, boundAccountID int64) (*Account, error) {
	account, err := s.GetDurableVideoLookupAccount(ctx, boundAccountID)
	if err != nil {
		return nil, err
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("video task account is unavailable")
	}
	return account, nil
}

func (s *OpenAIGatewayService) GetDurableVideoLookupAccount(ctx context.Context, boundAccountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil || boundAccountID <= 0 {
		return nil, fmt.Errorf("video task account is unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, boundAccountID)
	if err != nil {
		return nil, err
	}
	if account == nil || account.ID != boundAccountID || !durableVideoPlatform(account.Platform) || (account.Platform == PlatformOpenAI && account.Type != AccountTypeAPIKey) {
		return nil, fmt.Errorf("video task account is unavailable")
	}
	return account, nil
}
