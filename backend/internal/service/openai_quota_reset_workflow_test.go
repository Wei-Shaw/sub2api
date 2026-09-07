package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type resetWorkflowQuotaStub struct {
	usage    *OpenAIQuotaUsage
	cacheErr error
	order    *[]string
}

func (s *resetWorkflowQuotaStub) QueryUsage(context.Context, int64) (*OpenAIQuotaUsage, error) {
	*s.order = append(*s.order, "query")
	return s.usage, nil
}

func (s *resetWorkflowQuotaStub) CachePostResetSnapshot(context.Context, int64, *OpenAIQuotaUsage) error {
	*s.order = append(*s.order, "cache")
	return s.cacheErr
}

func TestRunOpenAIQuotaResetPostProcess_SyncsAutoResetBeforeReloadingAccount(t *testing.T) {
	var order []string
	usage := &OpenAIQuotaUsage{RateLimitResetCredits: &OpenAIRateLimitResetCredits{}}
	quota := &resetWorkflowQuotaStub{usage: usage, order: &order}
	loadAccount := func(_ context.Context, id int64) (*Account, error) {
		order = append(order, "load")
		return &Account{ID: id}, nil
	}
	syncAutoReset := func(_ context.Context, id int64, got *OpenAIQuotaUsage) {
		order = append(order, "sync")
		require.Equal(t, int64(7), id)
		require.Same(t, usage, got)
	}

	result := RunOpenAIQuotaResetPostProcess(context.Background(), 7, quota, autoResetTestRecoverer{}, loadAccount, syncAutoReset)

	require.True(t, result.CacheRefreshed)
	require.Empty(t, result.WarningCode)
	require.Equal(t, []string{"query", "cache", "sync", "load"}, order, "先同步调度器再重读账号，响应里的账号行才带新计划时刻")
}

func TestRunOpenAIQuotaResetPostProcess_SkipsSyncWhenSnapshotNotPersisted(t *testing.T) {
	var order []string
	quota := &resetWorkflowQuotaStub{usage: &OpenAIQuotaUsage{}, cacheErr: errors.New("boom"), order: &order}
	loadAccount := func(_ context.Context, id int64) (*Account, error) {
		order = append(order, "load")
		return &Account{ID: id}, nil
	}
	syncAutoReset := func(context.Context, int64, *OpenAIQuotaUsage) {
		order = append(order, "sync")
	}

	result := RunOpenAIQuotaResetPostProcess(context.Background(), 7, quota, autoResetTestRecoverer{}, loadAccount, syncAutoReset)

	require.False(t, result.CacheRefreshed)
	require.Equal(t, OpenAIQuotaResetWarningCacheRefreshFailed, result.WarningCode)
	require.Equal(t, []string{"query", "cache", "load"}, order, "快照没落库时不拿它去重排定时器")
}
