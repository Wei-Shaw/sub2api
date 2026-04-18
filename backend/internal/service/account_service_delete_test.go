//go:build unit

// 账号服务删除方法的单元测试
// 测试 AccountService.Delete 方法在各种场景下的行为

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// accountRepoStub 是 AccountRepository 接口的测试桩实现。
// 用于隔离测试 AccountService.Delete 方法，避免依赖真实数据库。
//
// 设计说明：
//   - exists: 模拟 ExistsByID 返回的存在性结果
//   - existsErr: 模拟 ExistsByID 返回的错误
//   - deleteErr: 模拟 Delete 返回的错误
//   - deletedIDs: 记录被调用删除的账号 ID，用于断言验证
type accountRepoStub struct {
	exists     bool    // ExistsByID 的返回值
	existsErr  error   // ExistsByID 的错误返回值
	deleteErr  error   // Delete 的错误返回值
	deletedIDs []int64 // 记录已删除的账号 ID 列表
}

// 以下方法在本测试中不应被调用，使用 panic 确保测试失败时能快速定位问题

func (s *accountRepoStub) Create(ctx context.Context, account *Account) error {
	panic("unexpected Create call")
}

func (s *accountRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	panic("unexpected GetByID call")
}

func (s *accountRepoStub) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	panic("unexpected GetByIDs call")
}

// ExistsByID 返回预设的存在性检查结果。
// 这是 Delete 方法调用的第一个仓储方法，用于验证账号是否存在。
func (s *accountRepoStub) ExistsByID(ctx context.Context, id int64) (bool, error) {
	return s.exists, s.existsErr
}

func (s *accountRepoStub) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*Account, error) {
	panic("unexpected GetByCRSAccountID call")
}

func (s *accountRepoStub) FindByExtraField(ctx context.Context, key string, value any) ([]Account, error) {
	panic("unexpected FindByExtraField call")
}

func (s *accountRepoStub) CountByTLSFingerprintProfile(ctx context.Context) (map[int64]int, error) {
	panic("unexpected CountByTLSFingerprintProfile call")
}

func (s *accountRepoStub) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	panic("unexpected ListCRSAccountIDs call")
}

func (s *accountRepoStub) Update(ctx context.Context, account *Account) error {
	panic("unexpected Update call")
}

// Delete 记录被删除的账号 ID 并返回预设的错误。
// 通过 deletedIDs 可以验证删除操作是否被正确调用。
func (s *accountRepoStub) Delete(ctx context.Context, id int64) error {
	s.deletedIDs = append(s.deletedIDs, id)
	return s.deleteErr
}

// 以下是接口要求实现但本测试不关心的方法

func (s *accountRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *accountRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *accountRepoStub) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	panic("unexpected ListByGroup call")
}

func (s *accountRepoStub) ListActive(ctx context.Context) ([]Account, error) {
	panic("unexpected ListActive call")
}

func (s *accountRepoStub) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	panic("unexpected ListByPlatform call")
}

func (s *accountRepoStub) UpdateLastUsed(ctx context.Context, id int64) error {
	panic("unexpected UpdateLastUsed call")
}

func (s *accountRepoStub) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	panic("unexpected BatchUpdateLastUsed call")
}

func (s *accountRepoStub) SetError(ctx context.Context, id int64, errorMsg string) error {
	panic("unexpected SetError call")
}

func (s *accountRepoStub) ClearError(ctx context.Context, id int64) error {
	panic("unexpected ClearError call")
}

func (s *accountRepoStub) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	panic("unexpected SetSchedulable call")
}

func (s *accountRepoStub) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	panic("unexpected AutoPauseExpiredAccounts call")
}

func (s *accountRepoStub) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	panic("unexpected BindGroups call")
}

func (s *accountRepoStub) ListSchedulable(ctx context.Context) ([]Account, error) {
	panic("unexpected ListSchedulable call")
}

func (s *accountRepoStub) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupID call")
}

func (s *accountRepoStub) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	panic("unexpected ListSchedulableByPlatform call")
}

func (s *accountRepoStub) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupIDAndPlatform call")
}

func (s *accountRepoStub) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	panic("unexpected ListSchedulableByPlatforms call")
}

func (s *accountRepoStub) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupIDAndPlatforms call")
}

func (s *accountRepoStub) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	panic("unexpected ListSchedulableUngroupedByPlatform call")
}

func (s *accountRepoStub) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	panic("unexpected ListSchedulableUngroupedByPlatforms call")
}

func (s *accountRepoStub) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	panic("unexpected SetRateLimited call")
}

func (s *accountRepoStub) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time) error {
	panic("unexpected SetModelRateLimit call")
}

func (s *accountRepoStub) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	panic("unexpected SetOverloaded call")
}

func (s *accountRepoStub) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	panic("unexpected SetTempUnschedulable call")
}

func (s *accountRepoStub) ClearTempUnschedulable(ctx context.Context, id int64) error {
	panic("unexpected ClearTempUnschedulable call")
}

func (s *accountRepoStub) ClearRateLimit(ctx context.Context, id int64) error {
	panic("unexpected ClearRateLimit call")
}

func (s *accountRepoStub) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	panic("unexpected ClearAntigravityQuotaScopes call")
}

func (s *accountRepoStub) ClearModelRateLimits(ctx context.Context, id int64) error {
	panic("unexpected ClearModelRateLimits call")
}

func (s *accountRepoStub) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	panic("unexpected UpdateSessionWindow call")
}

func (s *accountRepoStub) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	panic("unexpected UpdateExtra call")
}

func (s *accountRepoStub) BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	panic("unexpected BulkUpdate call")
}

func (s *accountRepoStub) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	return nil
}

func (s *accountRepoStub) ResetQuotaUsed(ctx context.Context, id int64) error {
	return nil
}

// TestAccountService_Delete_NotFound 测试删除不存在的账号时返回正确的错误。
// 预期行为：
//   - ExistsByID 返回 false（账号不存在）
//   - 返回 ErrAccountNotFound 错误
//   - Delete 方法不被调用（deletedIDs 为空）
func TestAccountService_Delete_NotFound(t *testing.T) {
	repo := &accountRepoStub{exists: false}
	svc := &AccountService{accountRepo: repo}

	err := svc.Delete(context.Background(), 55)
	require.ErrorIs(t, err, ErrAccountNotFound)
	require.Empty(t, repo.deletedIDs) // 验证删除操作未被调用
}

// TestAccountService_Delete_CheckError 测试存在性检查失败时的错误处理。
// 预期行为：
//   - ExistsByID 返回数据库错误
//   - 返回包含 "check account" 的错误信息
//   - Delete 方法不被调用
func TestAccountService_Delete_CheckError(t *testing.T) {
	repo := &accountRepoStub{existsErr: errors.New("db down")}
	svc := &AccountService{accountRepo: repo}

	err := svc.Delete(context.Background(), 55)
	require.Error(t, err)
	require.ErrorContains(t, err, "check account") // 验证错误信息包含上下文
	require.Empty(t, repo.deletedIDs)
}

// TestAccountService_Delete_DeleteError 测试删除操作失败时的错误处理。
// 预期行为：
//   - ExistsByID 返回 true（账号存在）
//   - Delete 被调用但返回错误
//   - 返回包含 "delete account" 的错误信息
//   - deletedIDs 记录了尝试删除的 ID
func TestAccountService_Delete_DeleteError(t *testing.T) {
	repo := &accountRepoStub{
		exists:    true,
		deleteErr: errors.New("delete failed"),
	}
	svc := &AccountService{accountRepo: repo}

	err := svc.Delete(context.Background(), 55)
	require.Error(t, err)
	require.ErrorContains(t, err, "delete account")
	require.Equal(t, []int64{55}, repo.deletedIDs) // 验证删除操作被调用
}

// TestAccountService_Delete_Success 测试删除操作成功的场景。
// 预期行为：
//   - ExistsByID 返回 true（账号存在）
//   - Delete 成功执行
//   - 返回 nil 错误
//   - deletedIDs 记录了被删除的 ID
func TestAccountService_Delete_Success(t *testing.T) {
	repo := &accountRepoStub{exists: true}
	svc := &AccountService{accountRepo: repo}

	err := svc.Delete(context.Background(), 55)
	require.NoError(t, err)
	require.Equal(t, []int64{55}, repo.deletedIDs) // 验证正确的 ID 被删除
}
