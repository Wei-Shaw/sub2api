//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestUserRepository_DeleteUser_AtomicWithAPIKeys 复现 AdminService.DeleteUser 的事务编排场景：
// 把"tombstone 并删 API Key"(apiKeyRepo.DeleteWithAudit) 与"删 User"(userRepo.Delete) 放进同一个外部事务时，
// userRepo.Delete 必须复用 context 中的事务，而不是用 base client 自起一个独立事务并提前提交。
//
// 用例用"回滚外层事务"来模拟 commit 失败 / 中止：
//   - 修复前：userRepo.Delete 用 base client 自起独立事务并 commit，回滚外层事务后用户仍被删除，
//     而 API Key 随外层事务回滚 → Case 1 断言失败，暴露原子性缺陷（即 issue #3021 的不可恢复状态）。
//   - 修复后：两者落在同一事务，回滚后用户与 API Key 一起恢复。
//
// 关键点：repo 必须用 base client 构造（NewUserRepository/NewAPIKeyRepository），并由本测试手动
// 开启外层事务，这与生产环境 wire 注入的方式一致；不能复用 APIKeyRepoSuite 的 testEntTx
// （那会让 repo 持有 tx client，走的是另一条 ErrTxStarted 复用路径，无法覆盖本场景）。
func TestUserRepository_DeleteUser_AtomicWithAPIKeys(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	userRepo := NewUserRepository(client, integrationDB)
	apiKeyRepo := NewAPIKeyRepository(client, integrationDB)

	// 已提交的初始数据：1 个用户 + 2 个 active API Key。
	user := mustCreateUser(t, client, &service.User{})
	key1 := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-atomic-a-%d", user.ID)})
	key2 := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-atomic-b-%d", user.ID)})

	t.Cleanup(func() {
		// testEntClient 的写入不会自动回滚，best-effort 清理避免污染共享库。
		_, _ = integrationDB.Exec(`DELETE FROM deleted_api_key_audits WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.Exec(`DELETE FROM api_keys WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.Exec(`DELETE FROM users WHERE id = $1`, user.ID)
	})

	listParams := pagination.PaginationParams{Page: 1, PageSize: 10}

	// --- Case 1: 外层事务回滚 → 删 Key 与删 User 必须一起回滚 ---
	tx, err := client.Tx(ctx)
	require.NoError(t, err, "begin outer tx")
	opCtx := dbent.NewTxContext(ctx, tx)

	require.NoError(t, apiKeyRepo.DeleteWithAudit(opCtx, key1.ID))
	require.NoError(t, apiKeyRepo.DeleteWithAudit(opCtx, key2.ID))
	require.NoError(t, userRepo.Delete(opCtx, user.ID))

	require.NoError(t, tx.Rollback(), "rollback outer tx (模拟 commit 失败/中止)")

	// 已提交视图：不应有任何写入"逃逸"到独立已提交事务。
	gotUser, err := userRepo.GetByID(ctx, user.ID)
	require.NoError(t, err, "回滚后用户必须仍然存在（未被独立事务提前删除）")
	require.Equal(t, user.ID, gotUser.ID)

	keys, _, err := apiKeyRepo.ListByUserID(ctx, user.ID, listParams, service.APIKeyListFilters{})
	require.NoError(t, err, "ListByUserID")
	require.Len(t, keys, 2, "回滚后 2 个 API Key 必须仍为 active")

	var auditCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM deleted_api_key_audits WHERE user_id = $1`, user.ID).Scan(&auditCount))
	require.Zero(t, auditCount, "回滚后不应有已提交的审计行")

	// --- Case 2: 外层事务提交 → 删 Key 与删 User 一起生效 ---
	tx2, err := client.Tx(ctx)
	require.NoError(t, err, "begin outer tx #2")
	opCtx2 := dbent.NewTxContext(ctx, tx2)

	require.NoError(t, apiKeyRepo.DeleteWithAudit(opCtx2, key1.ID))
	require.NoError(t, apiKeyRepo.DeleteWithAudit(opCtx2, key2.ID))
	require.NoError(t, userRepo.Delete(opCtx2, user.ID))

	require.NoError(t, tx2.Commit(), "commit outer tx")

	_, err = userRepo.GetByID(ctx, user.ID)
	require.Error(t, err, "提交后用户应被软删除，GetByID 应返回未找到")

	keysAfter, _, err := apiKeyRepo.ListByUserID(ctx, user.ID, listParams, service.APIKeyListFilters{})
	require.NoError(t, err, "ListByUserID")
	require.Empty(t, keysAfter, "提交后 API Key 应全部被软删除")

	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM deleted_api_key_audits WHERE user_id = $1`, user.ID).Scan(&auditCount))
	require.Zero(t, auditCount, "提交后也不得保留被删 Key 的凭据材料")
}
