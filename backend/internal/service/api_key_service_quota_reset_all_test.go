//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// resetAllQuotaRepoStub 复用 quotaBaseAPIKeyRepoStub 的其余方法，
// 只覆写"一键重置额度"用到的 ListAllByUserID 与 Update。
type resetAllQuotaRepoStub struct {
	quotaBaseAPIKeyRepoStub
	keys       []APIKey
	listErr    error
	updateErrs map[int64]error
	updated    map[int64]APIKey
	fields     map[int64]APIKeyUpdateFields
	listCalls  int
}

func newResetAllQuotaRepoStub(keys ...APIKey) *resetAllQuotaRepoStub {
	return &resetAllQuotaRepoStub{
		keys:       keys,
		updateErrs: map[int64]error{},
		updated:    map[int64]APIKey{},
		fields:     map[int64]APIKeyUpdateFields{},
	}
}

func (s *resetAllQuotaRepoStub) ListAllByUserID(_ context.Context, _ int64, _ APIKeyListFilters) ([]APIKey, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]APIKey, len(s.keys))
	copy(out, s.keys)
	return out, nil
}

func (s *resetAllQuotaRepoStub) Update(_ context.Context, key *APIKey, fields APIKeyUpdateFields) error {
	if err := s.updateErrs[key.ID]; err != nil {
		return err
	}
	s.updated[key.ID] = *key
	s.fields[key.ID] = fields
	return nil
}

func TestAPIKeyService_ResetQuotaUsageAll_OnlyResetsQuotaLimitedKeys(t *testing.T) {
	repo := newResetAllQuotaRepoStub(
		APIKey{ID: 1, Key: "sk-limited", Status: StatusActive, Quota: 10, QuotaUsed: 4},
		APIKey{ID: 2, Key: "sk-exhausted", Status: StatusAPIKeyQuotaExhausted, Quota: 5, QuotaUsed: 5},
		APIKey{ID: 3, Key: "sk-unlimited", Status: StatusActive, Quota: 0, QuotaUsed: 0},
	)
	cache := &quotaStateCacheStub{}
	svc := &APIKeyService{apiKeyRepo: repo, cache: cache}

	result, err := svc.ResetQuotaUsageAll(context.Background(), 7, false)

	require.NoError(t, err)
	require.Equal(t, 1, repo.listCalls)
	require.Equal(t, 2, result.Matched)
	require.Equal(t, 2, result.Reset)
	require.Equal(t, 1, result.Skipped)
	require.Equal(t, 0, result.Failed)
	require.Equal(t, []int64{1, 2}, result.ResetKeyIDs)

	// 未设置额度限制的 Key 不应被写入
	_, touched := repo.updated[3]
	require.False(t, touched, "unlimited key must not be updated")

	require.Equal(t, 0.0, repo.updated[1].QuotaUsed)
	require.Equal(t, StatusActive, repo.updated[1].Status)
	require.True(t, repo.fields[1].QuotaUsed)
	require.False(t, repo.fields[1].Status, "active key status should not be rewritten")

	// 额度耗尽 -> 活跃，否则额度归零后 Key 仍然不可用
	require.Equal(t, 0.0, repo.updated[2].QuotaUsed)
	require.Equal(t, StatusActive, repo.updated[2].Status)
	require.True(t, repo.fields[2].Status)

	// 逐个 Key 失效认证缓存，避免网关继续用旧额度判定
	require.Len(t, cache.deleteAuthKeys, 2)
}

func TestAPIKeyService_ResetQuotaUsageAll_IncludeRateLimitUsage(t *testing.T) {
	repo := newResetAllQuotaRepoStub(
		APIKey{
			ID: 11, Key: "sk-rate", Status: StatusActive,
			Quota: 20, QuotaUsed: 8,
			RateLimit5h: 1, RateLimit1d: 5,
			Usage5h: 1, Usage1d: 8, Usage7d: 0,
		},
	)
	svc := &APIKeyService{apiKeyRepo: repo, cache: &quotaStateCacheStub{}}

	result, err := svc.ResetQuotaUsageAll(context.Background(), 7, true)

	require.NoError(t, err)
	require.Equal(t, 1, result.Reset)
	require.True(t, repo.fields[11].RateLimitUsage)
	require.Equal(t, 0.0, repo.updated[11].Usage5h)
	require.Equal(t, 0.0, repo.updated[11].Usage1d)
	require.Nil(t, repo.updated[11].Window5hStart)
	require.Nil(t, repo.updated[11].Window1dStart)
	// 限速阈值本身不能被改掉
	require.Equal(t, 1.0, repo.updated[11].RateLimit5h)
	require.Equal(t, 5.0, repo.updated[11].RateLimit1d)
}

func TestAPIKeyService_ResetQuotaUsageAll_ReportsPartialFailure(t *testing.T) {
	repo := newResetAllQuotaRepoStub(
		APIKey{ID: 21, Key: "sk-ok", Status: StatusActive, Quota: 10, QuotaUsed: 3},
		APIKey{ID: 22, Key: "sk-bad", Status: StatusActive, Quota: 10, QuotaUsed: 3},
	)
	repo.updateErrs[22] = errors.New("db is down")
	svc := &APIKeyService{apiKeyRepo: repo, cache: &quotaStateCacheStub{}}

	result, err := svc.ResetQuotaUsageAll(context.Background(), 7, false)

	require.NoError(t, err)
	require.Equal(t, 2, result.Matched)
	require.Equal(t, 1, result.Reset)
	require.Equal(t, 1, result.Failed)
	require.Equal(t, []int64{22}, result.FailedKeyIDs)
}

func TestAPIKeyService_ResetQuotaUsageAll_ReturnsListError(t *testing.T) {
	repo := newResetAllQuotaRepoStub()
	repo.listErr = errors.New("db is down")
	svc := &APIKeyService{apiKeyRepo: repo, cache: &quotaStateCacheStub{}}

	result, err := svc.ResetQuotaUsageAll(context.Background(), 7, false)

	require.Error(t, err)
	require.Nil(t, result)
}

func TestAPIKeyService_ResetQuotaUsageAll_RejectsInvalidUser(t *testing.T) {
	repo := newResetAllQuotaRepoStub()
	svc := &APIKeyService{apiKeyRepo: repo, cache: &quotaStateCacheStub{}}

	_, err := svc.ResetQuotaUsageAll(context.Background(), 0, false)

	require.ErrorIs(t, err, ErrInsufficientPerms)
	require.Equal(t, 0, repo.listCalls)
}
