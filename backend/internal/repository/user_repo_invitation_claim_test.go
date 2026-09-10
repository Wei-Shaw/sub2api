//go:build integration

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestCreateWithEmailAliasGuardJoinsOuterTransaction 验证用户创建会加入调用方开启的
// 外部 ent 事务（注册流程“建用户 + 占用邀请码”原子性的基础）：
//   - 外层事务回滚后，用户与邀请码占用必须一并撤销（不得残留孤儿账号）；
//   - 外层事务提交后，用户与邀请码占用同时生效。
//
// 回归背景：此前 create() 通过 r.client.Tx(ctx) 自开事务且自行 Commit —— ent 的
// Client.Tx 不感知上下文事务（只检查 driver 类型），ErrTxStarted 分支实际是死代码，
// 导致外层事务包不住用户写入；并发注册同一邀请码时，败者用户的写入已自行提交，
// 即使注册被拒绝也会留下可登录的孤儿账号（1 个邀请码仍可生成任意数量账号）。
func TestCreateWithEmailAliasGuardJoinsOuterTransaction(t *testing.T) {
	client := testEntClient(t)
	userRepo := NewUserRepository(client, integrationDB)
	redeemRepo := NewRedeemCodeRepository(client)

	ctx := context.Background()

	// 清理：本测试会真实提交少量数据，确保不影响同包其它集成测试。
	var committedUserEmails []string
	var seededCodeIDs []int64
	t.Cleanup(func() {
		if len(committedUserEmails) > 0 {
			_, _ = client.User.Delete().Where(user.EmailIn(committedUserEmails...)).Exec(ctx)
		}
		if len(seededCodeIDs) > 0 {
			_, _ = client.RedeemCode.Delete().Where(redeemcode.IDIn(seededCodeIDs...)).Exec(ctx)
		}
	})

	seedCode := func(code string) int64 {
		_, err := client.RedeemCode.Create().
			SetCode(code).
			SetType(service.RedeemTypeInvitation).
			SetStatus(service.StatusUnused).
			SetValue(0).
			Save(ctx)
		require.NoError(t, err, "seed redeem code")
		c, err := client.RedeemCode.Query().Where(redeemcode.CodeEQ(code)).Only(ctx)
		require.NoError(t, err)
		seededCodeIDs = append(seededCodeIDs, c.ID)
		return c.ID
	}

	t.Run("rollback removes user and releases claim", func(t *testing.T) {
		codeID := seedCode("ITX-RACE-ROLLBACK-001")
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		txCtx := dbent.NewTxContext(ctx, tx)

		u := &service.User{
			Email:        "itx-rollback@example.com",
			PasswordHash: "test-password-hash",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
			Balance:      0,
			Concurrency:  1,
		}
		require.NoError(t, userRepo.CreateWithEmailAliasGuard(txCtx, u))
		require.Greater(t, u.ID, int64(0), "create 应回填用户 ID")
		require.NoError(t, redeemRepo.Use(txCtx, codeID, u.ID))
		require.NoError(t, tx.Rollback())

		exists, err := userRepo.ExistsByEmail(ctx, "itx-rollback@example.com")
		require.NoError(t, err)
		require.False(t, exists, "回滚后不得残留孤儿用户")

		after, err := client.RedeemCode.Get(ctx, codeID)
		require.NoError(t, err)
		require.Equal(t, service.StatusUnused, after.Status, "回滚后邀请码应保持 unused")
	})

	t.Run("commit persists user and claim together", func(t *testing.T) {
		codeID := seedCode("ITX-RACE-COMMIT-001")
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		txCtx := dbent.NewTxContext(ctx, tx)

		u := &service.User{
			Email:        "itx-commit@example.com",
			PasswordHash: "test-password-hash",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
			Balance:      0,
			Concurrency:  1,
		}
		require.NoError(t, userRepo.CreateWithEmailAliasGuard(txCtx, u))
		require.NoError(t, redeemRepo.Use(txCtx, codeID, u.ID))
		require.NoError(t, tx.Commit())
		committedUserEmails = append(committedUserEmails, u.Email)

		exists, err := userRepo.ExistsByEmail(ctx, "itx-commit@example.com")
		require.NoError(t, err)
		require.True(t, exists, "提交后用户应存在")

		after, err := client.RedeemCode.Get(ctx, codeID)
		require.NoError(t, err)
		require.Equal(t, service.StatusUsed, after.Status, "提交后邀请码应为 used")
	})
}
