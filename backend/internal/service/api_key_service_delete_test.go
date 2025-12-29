//go:build unit

// API Key 服务删除方法的单元测试
// 测试 ApiKeyService.Delete 方法在各种场景下的行为，
// 包括权限验证、缓存清理和错误处理

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// apiKeyRepoStub 是 ApiKeyRepository 接口的测试桩实现。
// 用于隔离测试 ApiKeyService.Delete 方法，避免依赖真实数据库。
//
// 设计说明：
//   - ownerID: 模拟 GetOwnerID 返回的所有者 ID
//   - ownerErr: 模拟 GetOwnerID 返回的错误（如 ErrApiKeyNotFound）
//   - deleteErr: 模拟 Delete 返回的错误
//   - deletedIDs: 记录被调用删除的 API Key ID，用于断言验证
type apiKeyRepoStub struct {
	ownerID    int64   // GetOwnerID 的返回值
	ownerErr   error   // GetOwnerID 的错误返回值
	deleteErr  error   // Delete 的错误返回值
	deletedIDs []int64 // 记录已删除的 API Key ID 列表
REDACTED

// 以下方法在本测试中不应被调用，使用 panic 确保测试失败时能快速定位问题

func (s *apiKeyRepoStub) Create(ctx context.Context, key *ApiKey) error {
	panic("unexpected Create call")
REDACTED

func (s *apiKeyRepoStub) GetByID(ctx context.Context, id int64) (*ApiKey, error) {
	panic("unexpected GetByID call")
REDACTED

// GetOwnerID 返回预设的所有者 ID 或错误。
// 这是 Delete 方法调用的第一个仓储方法，用于验证调用者是否为 API Key 的所有者。
func (s *apiKeyRepoStub) GetOwnerID(ctx context.Context, id int64) (int64, error) {
	return s.ownerID, s.ownerErr
REDACTED

func (s *apiKeyRepoStub) GetByKey(ctx context.Context, key string) (*ApiKey, error) {
	panic("unexpected GetByKey call")
REDACTED

func (s *apiKeyRepoStub) Update(ctx context.Context, key *ApiKey) error {
	panic("unexpected Update call")
REDACTED

// Delete 记录被删除的 API Key ID 并返回预设的错误。
// 通过 deletedIDs 可以验证删除操作是否被正确调用。
func (s *apiKeyRepoStub) Delete(ctx context.Context, id int64) error {
	s.deletedIDs = append(s.deletedIDs, id)
	return s.deleteErr
REDACTED

// 以下是接口要求实现但本测试不关心的方法

func (s *apiKeyRepoStub) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams) ([]ApiKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserID call")
REDACTED

func (s *apiKeyRepoStub) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	panic("unexpected VerifyOwnership call")
REDACTED

func (s *apiKeyRepoStub) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	panic("unexpected CountByUserID call")
REDACTED

func (s *apiKeyRepoStub) ExistsByKey(ctx context.Context, key string) (bool, error) {
	panic("unexpected ExistsByKey call")
REDACTED

func (s *apiKeyRepoStub) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]ApiKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
REDACTED

func (s *apiKeyRepoStub) SearchApiKeys(ctx context.Context, userID int64, keyword string, limit int) ([]ApiKey, error) {
	panic("unexpected SearchApiKeys call")
REDACTED

func (s *apiKeyRepoStub) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected ClearGroupIDByGroupID call")
REDACTED

func (s *apiKeyRepoStub) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected CountByGroupID call")
REDACTED

// apiKeyCacheStub 是 ApiKeyCache 接口的测试桩实现。
// 用于验证删除操作时缓存清理逻辑是否被正确调用。
//
// 设计说明：
//   - invalidated: 记录被清除缓存的用户 ID 列表
type apiKeyCacheStub struct {
	invalidated []int64 // 记录调用 DeleteCreateAttemptCount 时传入的用户 ID
REDACTED

// GetCreateAttemptCount 返回 0，表示用户未超过创建次数限制
func (s *apiKeyCacheStub) GetCreateAttemptCount(ctx context.Context, userID int64) (int, error) {
	return 0, nil
REDACTED

// IncrementCreateAttemptCount 空实现，本测试不验证此行为
func (s *apiKeyCacheStub) IncrementCreateAttemptCount(ctx context.Context, userID int64) error {
	return nil
REDACTED

// DeleteCreateAttemptCount 记录被清除缓存的用户 ID。
// 删除 API Key 时会调用此方法清除用户的创建尝试计数缓存。
func (s *apiKeyCacheStub) DeleteCreateAttemptCount(ctx context.Context, userID int64) error {
	s.invalidated = append(s.invalidated, userID)
	return nil
REDACTED

// IncrementDailyUsage 空实现，本测试不验证此行为
func (s *apiKeyCacheStub) IncrementDailyUsage(ctx context.Context, apiKey string) error {
	return nil
REDACTED

// SetDailyUsageExpiry 空实现，本测试不验证此行为
func (s *apiKeyCacheStub) SetDailyUsageExpiry(ctx context.Context, apiKey string, ttl time.Duration) error {
	return nil
REDACTED

// TestApiKeyService_Delete_OwnerMismatch 测试非所有者尝试删除时返回权限错误。
// 预期行为：
//   - GetOwnerID 返回所有者 ID 为 1
//   - 调用者 userID 为 2（不匹配）
//   - 返回 ErrInsufficientPerms 错误
//   - Delete 方法不被调用
//   - 缓存不被清除
func TestApiKeyService_Delete_OwnerMismatch(t *testing.T) {
	repo := &apiKeyRepoStub{ownerID: 1REDACTED
	cache := &apiKeyCacheStub{REDACTED
	svc := &ApiKeyService{apiKeyRepo: repo, cache: cacheREDACTED

	err := svc.Delete(context.Background(), 10, 2) // API Key ID=10, 调用者 userID=2
	require.ErrorIs(t, err, ErrInsufficientPerms)
	require.Empty(t, repo.deletedIDs)    // 验证删除操作未被调用
	require.Empty(t, cache.invalidated)  // 验证缓存未被清除
REDACTED

// TestApiKeyService_Delete_Success 测试所有者成功删除 API Key 的场景。
// 预期行为：
//   - GetOwnerID 返回所有者 ID 为 7
//   - 调用者 userID 为 7（匹配）
//   - Delete 成功执行
//   - 缓存被正确清除（使用 ownerID）
//   - 返回 nil 错误
func TestApiKeyService_Delete_Success(t *testing.T) {
	repo := &apiKeyRepoStub{ownerID: 7REDACTED
	cache := &apiKeyCacheStub{REDACTED
	svc := &ApiKeyService{apiKeyRepo: repo, cache: cacheREDACTED

	err := svc.Delete(context.Background(), 42, 7) // API Key ID=42, 调用者 userID=7
REDACTED
	require.Equal(t, []int64{42REDACTED, repo.deletedIDs) // 验证正确的 API Key 被删除
	require.Equal(t, []int64{7REDACTED, cache.invalidated) // 验证所有者的缓存被清除
REDACTED

// TestApiKeyService_Delete_NotFound 测试删除不存在的 API Key 时返回正确的错误。
// 预期行为：
//   - GetOwnerID 返回 ErrApiKeyNotFound 错误
//   - 返回 ErrApiKeyNotFound 错误（被 fmt.Errorf 包装）
//   - Delete 方法不被调用
//   - 缓存不被清除
func TestApiKeyService_Delete_NotFound(t *testing.T) {
	repo := &apiKeyRepoStub{ownerErr: ErrApiKeyNotFoundREDACTED
	cache := &apiKeyCacheStub{REDACTED
	svc := &ApiKeyService{apiKeyRepo: repo, cache: cacheREDACTED

	err := svc.Delete(context.Background(), 99, 1)
	require.ErrorIs(t, err, ErrApiKeyNotFound)
	require.Empty(t, repo.deletedIDs)
	require.Empty(t, cache.invalidated)
REDACTED

// TestApiKeyService_Delete_DeleteFails 测试删除操作失败时的错误处理。
// 预期行为：
//   - GetOwnerID 返回正确的所有者 ID
//   - 所有权验证通过
//   - 缓存被清除（在删除之前）
//   - Delete 被调用但返回错误
//   - 返回包含 "delete api key" 的错误信息
func TestApiKeyService_Delete_DeleteFails(t *testing.T) {
	repo := &apiKeyRepoStub{
		ownerID:   3,
		deleteErr: errors.New("delete failed"),
REDACTED
	cache := &apiKeyCacheStub{REDACTED
	svc := &ApiKeyService{apiKeyRepo: repo, cache: cacheREDACTED

	err := svc.Delete(context.Background(), 3, 3) // API Key ID=3, 调用者 userID=3
REDACTED
	require.ErrorContains(t, err, "delete api key")
	require.Equal(t, []int64{3REDACTED, repo.deletedIDs)   // 验证删除操作被调用
	require.Equal(t, []int64{3REDACTED, cache.invalidated) // 验证缓存已被清除（即使删除失败）
REDACTED
