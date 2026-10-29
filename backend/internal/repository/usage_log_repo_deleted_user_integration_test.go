//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLog_ListWithFilters_ResolvesSoftDeletedUser(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	// 一个活跃用户、一个将被软删的用户，各一条日志。
	active := mustCreateUser(t, client, &service.User{Email: "active-listfilter@test.com"})
	deleted := mustCreateUser(t, client, &service.User{Email: "deleted-listfilter@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: deleted.ID, Key: "sk-del-1", Name: "k"})
	apiKey2 := mustCreateApiKey(t, client, &service.APIKey{UserID: active.ID, Key: "sk-act-1", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-listfilter"})

	now := time.Now().UTC()
	for _, u := range []struct {
		uid int64
		kid int64
	}{{deleted.ID, apiKey.ID}, {active.ID, apiKey2.ID}} {
		_, err := repo.Create(ctx, &service.UsageLog{
			UserID: u.uid, APIKeyID: u.kid, AccountID: account.ID,
			Model: "claude-3", InputTokens: 1, OutputTokens: 1,
			TotalCost: 0.1, ActualCost: 0.1, CreatedAt: now,
		})
		require.NoError(t, err)
	}

	// 软删除该用户（触发 SoftDeleteMixin Hook → UPDATE deleted_at）。
	require.NoError(t, client.User.DeleteOneID(deleted.ID).Exec(ctx))

	logs, _, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 50},
		usagestats.UsageLogFilters{ExactTotal: true})
	require.NoError(t, err)

	byUser := map[int64]service.UsageLog{}
	for _, l := range logs {
		byUser[l.UserID] = l
	}

	// 已删用户的日志行：富化后 User 非 nil、邮箱正确、DeletedAt 非 nil。
	delLog, ok := byUser[deleted.ID]
	require.True(t, ok, "deleted user's usage log must still be listed")
	require.NotNil(t, delLog.User, "deleted user identity must resolve")
	require.Equal(t, "deleted-listfilter@test.com", delLog.User.Email)
	require.NotNil(t, delLog.User.DeletedAt, "DeletedAt must be set for soft-deleted user")

	// 活跃用户：DeletedAt 为 nil。
	actLog := byUser[active.ID]
	require.NotNil(t, actLog.User)
	require.Nil(t, actLog.User.DeletedAt)
}
