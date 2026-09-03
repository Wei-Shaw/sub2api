//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 订阅到期时间是读改写字段（续期、退款扣减、管理员调整都会读后写）。
// 服务层依赖 GetByIDForUpdate 的行级锁把读与写圈进同一个事务；
// 这里用两个真实并发事务验证 PostgreSQL 侧确实提供了这层串行化保证：
// 并发两次 +30 天必须累计成 +60 天，而不是只生效一次。
func TestUserSubscription_GetByIDForUpdateSerializesConcurrentExtensions(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUserSubscriptionRepository(client)

	user, err := client.User.Create().
		SetEmail("subscription-for-update@example.com").
		SetPasswordHash("test-password-hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(ctx)
	require.NoError(t, err, "create user")

	group, err := client.Group.Create().
		SetName("subscription-for-update-group").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err, "create group")

	base := time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC)
	created, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(base.AddDate(0, 0, -30)).
		SetExpiresAt(base).
		SetStatus(service.SubscriptionStatusActive).
		SetAssignedAt(time.Now()).
		Save(ctx)
	require.NoError(t, err, "create subscription")

	t.Cleanup(func() {
		// 本用例用的是会真正提交的全局 client，必须硬删除。
		// user_subscription 带软删除 hook，直接 DeleteOneID 只会把行改成 revoked，
		// 而 List 在 status 为空或 revoked 时走 SkipSoftDelete 会把它读出来，
		// 从而污染同包的 TestUserSubscriptionRepoSuite。硬删除写法见
		// soft_delete_ent_integration_test.go:107。
		cleanupCtx := mixins.SkipSoftDelete(context.Background())
		_ = client.UserSubscription.DeleteOneID(created.ID).Exec(cleanupCtx)
		_ = client.Group.DeleteOneID(group.ID).Exec(cleanupCtx)
		_ = client.User.DeleteOneID(user.ID).Exec(cleanupCtx)
	})

	extendUnderRowLock := func() error {
		tx, err := client.Tx(ctx)
		if err != nil {
			return err
		}
		txCtx := dbent.NewTxContext(ctx, tx)

		sub, err := repo.GetByIDForUpdate(txCtx, created.ID)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		// 拉大读改写窗口：没有行级锁时两个事务必然读到同一个过期时间。
		time.Sleep(200 * time.Millisecond)

		if err := repo.ExtendExpiry(txCtx, created.ID, sub.ExpiresAt.AddDate(0, 0, 30)); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}

	var (
		start sync.WaitGroup
		done  sync.WaitGroup
		errs  [2]error
	)
	start.Add(1)
	done.Add(2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer done.Done()
			start.Wait()
			errs[idx] = extendUnderRowLock()
		}(i)
	}
	start.Done()
	done.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	final, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err, "GetByID after concurrent extensions")
	require.Equal(t, base.AddDate(0, 0, 60).UTC(), final.ExpiresAt.UTC(),
		"并发两次 +30 天必须累计为 +60 天，行级锁不得被绕过")
}
