package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// VideoTaskRecordRepository journals legacy video lifecycles without handing
// their billing to the OpenAI worker. No credentials, prompts or URLs are saved.
type VideoTaskRecordRepository interface {
	BeginVideoTaskRecord(context.Context, *VideoTask) error
	AcceptVideoTaskRecord(context.Context, *VideoTask, string, string) error
	ObserveVideoTaskRecord(context.Context, string, int64, int64, int64, string) error
}

func (s *OpenAIGatewayService) videoTaskRecordRepository() (VideoTaskRecordRepository, error) {
	if s != nil {
		if repo, ok := s.usageBillingRepo.(VideoTaskRecordRepository); ok && repo != nil {
			return repo, nil
		}
	}
	return nil, ErrVideoTaskRepositoryUnavailable
}

// BeginVideoTaskRecord runs BEFORE upstream submission. An interrupted or
// rejected submission remains visible as unknown, never falsely marked failed:
// providers can return even HTTP 400 after accepting a billable video.
func (s *OpenAIGatewayService) BeginVideoTaskRecord(ctx context.Context, key *APIKey, account *Account, subscription *UserSubscription, pending GrokVideoPendingBilling) (*VideoTask, error) {
	if key == nil || key.ID <= 0 || key.UserID <= 0 || account == nil || account.ID <= 0 {
		return nil, errors.New("video record requires an authenticated owner and account")
	}
	repo, err := s.videoTaskRecordRepository()
	if err != nil {
		return nil, err
	}
	pending.TrackingOnly = true
	if pending.CreatedAt == "" {
		pending.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	t := &VideoTask{TaskID: "submission:" + uuid.NewString(), UserID: key.UserID, APIKeyID: key.ID,
		GroupID: copyVideoID(key.GroupID), AccountID: account.ID, Pending: pending,
		RoutingFingerprint: videoTaskMonitorRoutingFingerprint(account)}
	if subscription != nil {
		t.SubscriptionID = copyVideoID(&subscription.ID)
	}
	return t, repo.BeginVideoTaskRecord(ctx, t)
}

func (s *OpenAIGatewayService) AcceptVideoTaskRecord(ctx context.Context, t *VideoTask, taskID, state string) error {
	if t == nil || validateVideoTaskID(taskID) != nil || !validVideoRecordState(state) {
		return errors.New("invalid accepted video record")
	}
	repo, err := s.videoTaskRecordRepository()
	if err != nil {
		return err
	}
	return repo.AcceptVideoTaskRecord(ctx, t, taskID, state)
}

func (s *OpenAIGatewayService) ObserveVideoTaskRecord(ctx context.Context, taskID string, userID, keyID, accountID int64, state string) error {
	if validateVideoTaskID(taskID) != nil || !validVideoRecordState(state) {
		return errors.New("invalid video record observation")
	}
	repo, err := s.videoTaskRecordRepository()
	if err != nil {
		return err
	}
	return repo.ObserveVideoTaskRecord(ctx, taskID, userID, keyID, accountID, state)
}

func validVideoRecordState(state string) bool {
	return state == VideoTaskPending || state == VideoTaskCompleted || state == VideoTaskFailed
}
