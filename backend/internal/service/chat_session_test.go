package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type chatSessionRepoScopeStub struct {
	listUserID     int64
	listAPIKeyID   int64
	detailUserID   int64
	detailAPIKeyID int64
	recentUserID   int64
	recentAPIKeyID int64
	deletedCutoffs []time.Time
	deleteLimit    int
	deleteBatches  []int64
}

func (s *chatSessionRepoScopeStub) CreateSessionWithMessages(context.Context, *ChatSessionRecordInput) error {
	return nil
}

func (s *chatSessionRepoScopeStub) ListSessionsByAPIKey(_ context.Context, userID, apiKeyID int64, _ pagination.PaginationParams) ([]*ChatSession, int64, error) {
	s.listUserID = userID
	s.listAPIKeyID = apiKeyID
	return []*ChatSession{}, 0, nil
}

func (s *chatSessionRepoScopeStub) GetSessionDetail(_ context.Context, userID, apiKeyID, sessionID int64, _ pagination.PaginationParams) (*ChatSessionDetail, error) {
	s.detailUserID = userID
	s.detailAPIKeyID = apiKeyID
	return &ChatSessionDetail{ChatSession: ChatSession{ID: sessionID, UserID: userID, APIKeyID: apiKeyID}}, nil
}

func (s *chatSessionRepoScopeStub) GetChatMessageDetail(_ context.Context, userID, apiKeyID, sessionID, messageID int64) (*ChatMessage, error) {
	s.detailUserID = userID
	s.detailAPIKeyID = apiKeyID
	return &ChatMessage{ID: messageID, SessionID: sessionID}, nil
}

func (s *chatSessionRepoScopeStub) ListRecentMessagesByAPIKey(_ context.Context, userID, apiKeyID int64, _ int) ([]ChatMessage, error) {
	s.recentUserID = userID
	s.recentAPIKeyID = apiKeyID
	return []ChatMessage{}, nil
}

func (s *chatSessionRepoScopeStub) DeleteSessionsBefore(_ context.Context, cutoff time.Time, limit int) (int64, error) {
	s.deletedCutoffs = append(s.deletedCutoffs, cutoff)
	s.deleteLimit = limit
	if len(s.deleteBatches) == 0 {
		return 0, nil
	}
	deleted := s.deleteBatches[0]
	s.deleteBatches = s.deleteBatches[1:]
	return deleted, nil
}

func TestChatSessionServiceScopesQueriesByUserAndAPIKey(t *testing.T) {
	t.Parallel()

	repo := &chatSessionRepoScopeStub{}
	svc := NewChatSessionService(repo)

	if _, _, err := svc.ListSessionsByAPIKey(context.Background(), 11, 22, pagination.PaginationParams{}); err != nil {
		t.Fatalf("ListSessionsByAPIKey() error = %v", err)
	}
	if repo.listUserID != 11 || repo.listAPIKeyID != 22 {
		t.Fatalf("list scope = user %d key %d, want user 11 key 22", repo.listUserID, repo.listAPIKeyID)
	}

	if _, err := svc.GetSessionDetail(context.Background(), 33, 44, 55, pagination.PaginationParams{Page: 1, PageSize: 50}); err != nil {
		t.Fatalf("GetSessionDetail() error = %v", err)
	}
	if repo.detailUserID != 33 || repo.detailAPIKeyID != 44 {
		t.Fatalf("detail scope = user %d key %d, want user 33 key 44", repo.detailUserID, repo.detailAPIKeyID)
	}

	if _, err := svc.ListRecentMessagesByAPIKey(context.Background(), 66, 77, 50); err != nil {
		t.Fatalf("ListRecentMessagesByAPIKey() error = %v", err)
	}
	if repo.recentUserID != 66 || repo.recentAPIKeyID != 77 {
		t.Fatalf("recent scope = user %d key %d, want user 66 key 77", repo.recentUserID, repo.recentAPIKeyID)
	}
}

func TestChatSessionRetentionServiceRunOnceDeletesUntilBatchShort(t *testing.T) {
	repo := &chatSessionRepoScopeStub{deleteBatches: []int64{2, 1}}
	svc := NewChatSessionRetentionService(repo, &config.Config{
		ChatSessionRetention: config.ChatSessionRetentionConfig{
			Enabled:            true,
			RetentionDays:      90,
			BatchSize:          2,
			IntervalSeconds:    86400,
			TaskTimeoutSeconds: 10,
		},
	})

	svc.runOnce()

	if got := len(repo.deletedCutoffs); got != 2 {
		t.Fatalf("DeleteSessionsBefore calls = %d, want 2", got)
	}
	if repo.deleteLimit != 2 {
		t.Fatalf("delete limit = %d, want 2", repo.deleteLimit)
	}
	cutoffAge := time.Since(repo.deletedCutoffs[0])
	if cutoffAge < 89*24*time.Hour || cutoffAge > 91*24*time.Hour {
		t.Fatalf("cutoff age = %s, want about 90d", cutoffAge)
	}
}
