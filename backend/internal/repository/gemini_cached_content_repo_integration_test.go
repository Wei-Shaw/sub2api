//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGeminiCachedContentRepository_Lifecycle(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := &geminiCachedContentRepository{sql: tx}
	now := time.Now().UTC().Truncate(time.Microsecond)
	suffix := time.Now().Format("150405.000000")
	apiKeyID := int64(910001)

	newRecord := func(publicID string, keyID, groupID int64, expire time.Time) *service.GeminiCachedContent {
		return &service.GeminiCachedContent{
			PublicID: publicID + suffix, UserID: 1, APIKeyID: keyID, GroupID: groupID, AccountID: 99,
			UpstreamName: "cachedContents/up" + publicID, Model: "gemini-3.8-flash", UpstreamModel: "models/gemini-3.8-flash",
			DisplayName: "label", TotalTokenCount: 2730, ExpireTime: expire,
			ChannelUsage: service.ChannelUsageFields{ChannelID: 7, OriginalModel: "gemini-pro", ChannelMappedModel: "gemini-3.8-flash", BillingModelSource: "requested"},
		}
	}
	first := newRecord("a", apiKeyID, 42, now.Add(time.Hour))
	require.NoError(t, repo.Create(ctx, first))
	require.Positive(t, first.ID)
	require.False(t, first.CreatedAt.IsZero())
	_, err := tx.ExecContext(ctx, `SAVEPOINT gemini_cached_content_duplicate`)
	require.NoError(t, err)
	require.ErrorIs(t, repo.Create(ctx, newRecord("a", apiKeyID, 42, now.Add(time.Hour))), service.ErrGeminiCachedContentExists)
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT gemini_cached_content_duplicate`)
	require.NoError(t, err)

	second := newRecord("b", apiKeyID, 42, now.Add(time.Hour))
	require.NoError(t, repo.Create(ctx, second))
	require.NoError(t, repo.Create(ctx, newRecord("expired", apiKeyID, 42, now.Add(-time.Minute))))
	require.NoError(t, repo.Create(ctx, newRecord("othergroup", apiKeyID, 43, now.Add(time.Hour))))
	require.NoError(t, repo.Create(ctx, newRecord("otherkey", apiKeyID+1, 42, now.Add(time.Hour))))

	got, err := repo.GetForOwner(ctx, apiKeyID, first.PublicID)
	require.NoError(t, err)
	require.Equal(t, "cachedContents/upa", got.UpstreamName)
	require.Equal(t, int64(2730), got.TotalTokenCount)
	require.True(t, got.ExpireTime.Equal(first.ExpireTime))
	require.Equal(t, first.ChannelUsage, got.ChannelUsage, "创建时的渠道计费口径随记录持久化")
	_, err = repo.GetForOwner(ctx, apiKeyID+1, first.PublicID)
	require.ErrorIs(t, err, service.ErrGeminiCachedContentNotFound)

	list, err := repo.ListForOwner(ctx, apiKeyID, 42, now, 0, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, second.PublicID, list[0].PublicID)
	page, err := repo.ListForOwner(ctx, apiKeyID, 42, now, list[0].ID, 10)
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Equal(t, first.PublicID, page[0].PublicID)

	lease := 2 * time.Minute
	acquired, err := repo.BeginMutation(ctx, first.ID, first.ExpireTime, "t1", lease)
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = repo.BeginMutation(ctx, first.ID, first.ExpireTime, "t2", lease)
	require.NoError(t, err)
	require.False(t, acquired, "有进行中的修改时不能再登记")
	taken, err := repo.TakeOverStaleMutation(ctx, first.ID, "t2", lease)
	require.NoError(t, err)
	require.Nil(t, taken, "租约未过期不能接管")
	require.ErrorIs(t, repo.CommitMutation(ctx, first.ID, "t2", now), service.ErrGeminiCachedContentMutationConflict)

	newExpire := now.Add(3 * time.Hour)
	require.NoError(t, repo.CommitMutation(ctx, first.ID, "t1", newExpire))
	got, err = repo.GetForOwner(ctx, apiKeyID, first.PublicID)
	require.NoError(t, err)
	require.True(t, got.ExpireTime.Equal(newExpire))
	require.ErrorIs(t, repo.CommitMutation(ctx, first.ID, "t1", now), service.ErrGeminiCachedContentMutationConflict, "提交后修改意图已清除")

	acquired, err = repo.BeginMutation(ctx, first.ID, first.ExpireTime, "t3", lease)
	require.NoError(t, err)
	require.False(t, acquired, "到期时间已被其他修改改变")
	acquired, err = repo.BeginMutation(ctx, first.ID, newExpire, "t3", lease)
	require.NoError(t, err)
	require.True(t, acquired)
	stale, err := repo.ListStaleMutations(ctx, 10)
	require.NoError(t, err)
	require.NotContains(t, stale, first.ID)

	_, err = tx.ExecContext(ctx, `UPDATE gemini_cached_contents SET mutation_lease_until = NOW() - INTERVAL '1 second' WHERE id = $1`, first.ID)
	require.NoError(t, err)
	stale, err = repo.ListStaleMutations(ctx, 10)
	require.NoError(t, err)
	require.Contains(t, stale, first.ID)
	taken, err = repo.TakeOverStaleMutation(ctx, first.ID, "t4", lease)
	require.NoError(t, err)
	require.NotNil(t, taken)
	require.True(t, taken.ExpireTime.Equal(newExpire), "接管返回修改前的到期时间")
	require.ErrorIs(t, repo.CommitMutation(ctx, first.ID, "t3", now), service.ErrGeminiCachedContentMutationConflict, "被接管后原持有者不能提交")
	require.NoError(t, repo.ReleaseMutation(ctx, first.ID, "t3"))
	acquired, err = repo.BeginMutation(ctx, first.ID, newExpire, "t5", lease)
	require.NoError(t, err)
	require.False(t, acquired, "原持有者释放不影响接管者")
	require.NoError(t, repo.ReleaseMutation(ctx, first.ID, "t4"))
	stale, err = repo.ListStaleMutations(ctx, 10)
	require.NoError(t, err)
	require.NotContains(t, stale, first.ID)

	require.NoError(t, repo.SoftDelete(ctx, first.ID))
	_, err = repo.GetForOwner(ctx, apiKeyID, first.PublicID)
	require.ErrorIs(t, err, service.ErrGeminiCachedContentNotFound)
	_, err = repo.BeginMutation(ctx, first.ID, newExpire, "t6", lease)
	require.ErrorIs(t, err, service.ErrGeminiCachedContentNotFound)
	list, err = repo.ListForOwner(ctx, apiKeyID, 42, now, 0, 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
}
