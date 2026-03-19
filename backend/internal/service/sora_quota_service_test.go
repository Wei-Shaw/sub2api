//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// ==================== Stub: GroupRepository (用于 SoraQuotaService) ====================

var _ GroupRepository = (*stubGroupRepoForQuota)(nil)

type stubGroupRepoForQuota struct {
	groups map[int64]*Group
REDACTED

func newStubGroupRepoForQuota() *stubGroupRepoForQuota {
	return &stubGroupRepoForQuota{groups: make(map[int64]*Group)REDACTED
REDACTED

func (r *stubGroupRepoForQuota) GetByID(_ context.Context, id int64) (*Group, error) {
	if g, ok := r.groups[id]; ok {
		return g, nil
REDACTED
	return nil, fmt.Errorf("group not found")
REDACTED
func (r *stubGroupRepoForQuota) Create(context.Context, *Group) error { return nil REDACTED
func (r *stubGroupRepoForQuota) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	return r.GetByID(context.Background(), id)
REDACTED
func (r *stubGroupRepoForQuota) Update(context.Context, *Group) error { return nil REDACTED
func (r *stubGroupRepoForQuota) Delete(context.Context, int64) error  { return nil REDACTED
func (r *stubGroupRepoForQuota) DeleteCascade(context.Context, int64) ([]int64, error) {
	return nil, nil
REDACTED
func (r *stubGroupRepoForQuota) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (r *stubGroupRepoForQuota) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (r *stubGroupRepoForQuota) ListActive(context.Context) ([]Group, error) { return nil, nil REDACTED
func (r *stubGroupRepoForQuota) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	return nil, nil
REDACTED
func (r *stubGroupRepoForQuota) ExistsByName(context.Context, string) (bool, error) {
	return false, nil
REDACTED
func (r *stubGroupRepoForQuota) GetAccountCount(context.Context, int64) (int64, int64, error) {
	return 0, 0, nil
REDACTED
func (r *stubGroupRepoForQuota) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
REDACTED
func (r *stubGroupRepoForQuota) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	return nil, nil
REDACTED
func (r *stubGroupRepoForQuota) BindAccountsToGroup(context.Context, int64, []int64) error {
	return nil
REDACTED
func (r *stubGroupRepoForQuota) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	return nil
REDACTED

// ==================== Stub: SettingRepository (用于 SettingService) ====================

var _ SettingRepository = (*stubSettingRepoForQuota)(nil)

type stubSettingRepoForQuota struct {
	values map[string]string
REDACTED

func newStubSettingRepoForQuota(values map[string]string) *stubSettingRepoForQuota {
	if values == nil {
		values = make(map[string]string)
REDACTED
	return &stubSettingRepoForQuota{values: valuesREDACTED
REDACTED

func (r *stubSettingRepoForQuota) Get(_ context.Context, key string) (*Setting, error) {
	if v, ok := r.values[key]; ok {
		return &Setting{Key: key, Value: vREDACTED, nil
REDACTED
	return nil, ErrSettingNotFound
REDACTED
func (r *stubSettingRepoForQuota) GetValue(_ context.Context, key string) (string, error) {
	if v, ok := r.values[key]; ok {
		return v, nil
REDACTED
	return "", ErrSettingNotFound
REDACTED
func (r *stubSettingRepoForQuota) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
REDACTED
func (r *stubSettingRepoForQuota) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, k := range keys {
		if v, ok := r.values[k]; ok {
			result[k] = v
	REDACTED
REDACTED
	return result, nil
REDACTED
func (r *stubSettingRepoForQuota) SetMultiple(_ context.Context, settings map[string]string) error {
	for k, v := range settings {
		r.values[k] = v
REDACTED
	return nil
REDACTED
func (r *stubSettingRepoForQuota) GetAll(_ context.Context) (map[string]string, error) {
	return r.values, nil
REDACTED
func (r *stubSettingRepoForQuota) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
REDACTED

// ==================== GetQuota ====================

func TestGetQuota_UserLevel(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{
		ID:                    1,
		SoraStorageQuotaBytes: 10 * 1024 * 1024, // 10MB
		SoraStorageUsedBytes:  3 * 1024 * 1024,  // 3MB
REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	quota, err := svc.GetQuota(context.Background(), 1)
REDACTED
	require.Equal(t, int64(10*1024*1024), quota.QuotaBytes)
	require.Equal(t, int64(3*1024*1024), quota.UsedBytes)
	require.Equal(t, "user", quota.Source)
REDACTED

func TestGetQuota_GroupLevel(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{
		ID:                    1,
		SoraStorageQuotaBytes: 0, // 用户级无配额
		SoraStorageUsedBytes:  1024,
		AllowedGroups:         []int64{10, 20REDACTED,
REDACTED

	groupRepo := newStubGroupRepoForQuota()
	groupRepo.groups[10] = &Group{ID: 10, SoraStorageQuotaBytes: 5 * 1024 * 1024REDACTED
	groupRepo.groups[20] = &Group{ID: 20, SoraStorageQuotaBytes: 20 * 1024 * 1024REDACTED

	svc := NewSoraQuotaService(userRepo, groupRepo, nil)
	quota, err := svc.GetQuota(context.Background(), 1)
REDACTED
	require.Equal(t, int64(20*1024*1024), quota.QuotaBytes) // 取最大值
	require.Equal(t, "group", quota.Source)
REDACTED

func TestGetQuota_SystemLevel(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageQuotaBytes: 0, SoraStorageUsedBytes: 512REDACTED

	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraDefaultStorageQuotaBytes: "104857600", // 100MB
REDACTED)
	settingService := NewSettingService(settingRepo, &config.Config{REDACTED)
	svc := NewSoraQuotaService(userRepo, nil, settingService)

	quota, err := svc.GetQuota(context.Background(), 1)
REDACTED
	require.Equal(t, int64(104857600), quota.QuotaBytes)
	require.Equal(t, "system", quota.Source)
REDACTED

func TestGetQuota_NoLimit(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageQuotaBytes: 0, SoraStorageUsedBytes: 0REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	quota, err := svc.GetQuota(context.Background(), 1)
REDACTED
	require.Equal(t, int64(0), quota.QuotaBytes)
	require.Equal(t, "unlimited", quota.Source)
REDACTED

func TestGetQuota_UserNotFound(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	svc := NewSoraQuotaService(userRepo, nil, nil)

	_, err := svc.GetQuota(context.Background(), 999)
REDACTED
	require.Contains(t, err.Error(), "get user")
REDACTED

func TestGetQuota_GroupRepoError(t *testing.T) {
	// 分组获取失败时跳过该分组（不影响整体）
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{
		ID: 1, SoraStorageQuotaBytes: 0,
		AllowedGroups: []int64{999REDACTED, // 不存在的分组
REDACTED

	groupRepo := newStubGroupRepoForQuota()
	svc := NewSoraQuotaService(userRepo, groupRepo, nil)

	quota, err := svc.GetQuota(context.Background(), 1)
REDACTED
	require.Equal(t, "unlimited", quota.Source) // 分组获取失败，回退到无限制
REDACTED

// ==================== CheckQuota ====================

func TestCheckQuota_Sufficient(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{
		ID:                    1,
		SoraStorageQuotaBytes: 10 * 1024 * 1024,
		SoraStorageUsedBytes:  3 * 1024 * 1024,
REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.CheckQuota(context.Background(), 1, 1024)
REDACTED
REDACTED

func TestCheckQuota_Exceeded(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{
		ID:                    1,
		SoraStorageQuotaBytes: 10 * 1024 * 1024,
		SoraStorageUsedBytes:  10 * 1024 * 1024, // 已满
REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.CheckQuota(context.Background(), 1, 1)
REDACTED
	require.Contains(t, err.Error(), "配额不足")
REDACTED

func TestCheckQuota_NoLimit(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{
		ID:                    1,
		SoraStorageQuotaBytes: 0, // 无限制
		SoraStorageUsedBytes:  1000000000,
REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.CheckQuota(context.Background(), 1, 999999999)
REDACTED // 无限制时始终通过
REDACTED

func TestCheckQuota_ExactBoundary(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{
		ID:                    1,
		SoraStorageQuotaBytes: 1024,
		SoraStorageUsedBytes:  1024, // 恰好满
REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	// 额外 0 字节不超
	require.NoError(t, svc.CheckQuota(context.Background(), 1, 0))
	// 额外 1 字节超出
	require.Error(t, svc.CheckQuota(context.Background(), 1, 1))
REDACTED

// ==================== AddUsage ====================

func TestAddUsage_Success(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 1024REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.AddUsage(context.Background(), 1, 2048)
REDACTED
	require.Equal(t, int64(3072), userRepo.users[1].SoraStorageUsedBytes)
REDACTED

func TestAddUsage_ZeroBytes(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 1024REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.AddUsage(context.Background(), 1, 0)
REDACTED
	require.Equal(t, int64(1024), userRepo.users[1].SoraStorageUsedBytes) // 不变
REDACTED

func TestAddUsage_NegativeBytes(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 1024REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.AddUsage(context.Background(), 1, -100)
REDACTED
	require.Equal(t, int64(1024), userRepo.users[1].SoraStorageUsedBytes) // 不变
REDACTED

func TestAddUsage_UserNotFound(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.AddUsage(context.Background(), 999, 1024)
REDACTED
REDACTED

func TestAddUsage_UpdateError(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 0REDACTED
	userRepo.updateErr = fmt.Errorf("db error")
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.AddUsage(context.Background(), 1, 1024)
REDACTED
	require.Contains(t, err.Error(), "update user quota usage")
REDACTED

// ==================== ReleaseUsage ====================

func TestReleaseUsage_Success(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 3072REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.ReleaseUsage(context.Background(), 1, 1024)
REDACTED
	require.Equal(t, int64(2048), userRepo.users[1].SoraStorageUsedBytes)
REDACTED

func TestReleaseUsage_ClampToZero(t *testing.T) {
	// 释放量大于已用量时，应 clamp 到 0
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 500REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.ReleaseUsage(context.Background(), 1, 1000)
REDACTED
	require.Equal(t, int64(0), userRepo.users[1].SoraStorageUsedBytes)
REDACTED

func TestReleaseUsage_ZeroBytes(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 1024REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.ReleaseUsage(context.Background(), 1, 0)
REDACTED
	require.Equal(t, int64(1024), userRepo.users[1].SoraStorageUsedBytes) // 不变
REDACTED

func TestReleaseUsage_NegativeBytes(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 1024REDACTED
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.ReleaseUsage(context.Background(), 1, -50)
REDACTED
	require.Equal(t, int64(1024), userRepo.users[1].SoraStorageUsedBytes) // 不变
REDACTED

func TestReleaseUsage_UserNotFound(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.ReleaseUsage(context.Background(), 999, 1024)
REDACTED
REDACTED

func TestReleaseUsage_UpdateError(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 1024REDACTED
	userRepo.updateErr = fmt.Errorf("db error")
	svc := NewSoraQuotaService(userRepo, nil, nil)

	err := svc.ReleaseUsage(context.Background(), 1, 512)
REDACTED
	require.Contains(t, err.Error(), "update user quota release")
REDACTED

// ==================== GetQuotaFromSettings ====================

func TestGetQuotaFromSettings_NilSettingService(t *testing.T) {
	svc := NewSoraQuotaService(nil, nil, nil)
	require.Equal(t, int64(0), svc.GetQuotaFromSettings(context.Background()))
REDACTED

func TestGetQuotaFromSettings_WithSettings(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraDefaultStorageQuotaBytes: "52428800", // 50MB
REDACTED)
	settingService := NewSettingService(settingRepo, &config.Config{REDACTED)
	svc := NewSoraQuotaService(nil, nil, settingService)

	require.Equal(t, int64(52428800), svc.GetQuotaFromSettings(context.Background()))
REDACTED

// ==================== SetUserSoraQuota ====================

func TestSetUserSoraQuota_Success(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageQuotaBytes: 0REDACTED

	err := SetUserSoraQuota(context.Background(), userRepo, 1, 10*1024*1024)
REDACTED
	require.Equal(t, int64(10*1024*1024), userRepo.users[1].SoraStorageQuotaBytes)
REDACTED

func TestSetUserSoraQuota_UserNotFound(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	err := SetUserSoraQuota(context.Background(), userRepo, 999, 1024)
REDACTED
REDACTED

// ==================== ParseQuotaBytes ====================

func TestParseQuotaBytes(t *testing.T) {
	require.Equal(t, int64(1048576), ParseQuotaBytes("1048576"))
	require.Equal(t, int64(0), ParseQuotaBytes(""))
	require.Equal(t, int64(0), ParseQuotaBytes("abc"))
	require.Equal(t, int64(-1), ParseQuotaBytes("-1"))
REDACTED

// ==================== 优先级完整测试 ====================

func TestQuotaPriority_UserOverridesGroup(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{
		ID:                    1,
		SoraStorageQuotaBytes: 5 * 1024 * 1024,
		AllowedGroups:         []int64{10REDACTED,
REDACTED

	groupRepo := newStubGroupRepoForQuota()
	groupRepo.groups[10] = &Group{ID: 10, SoraStorageQuotaBytes: 20 * 1024 * 1024REDACTED

	svc := NewSoraQuotaService(userRepo, groupRepo, nil)
	quota, err := svc.GetQuota(context.Background(), 1)
REDACTED
	require.Equal(t, "user", quota.Source) // 用户级优先
	require.Equal(t, int64(5*1024*1024), quota.QuotaBytes)
REDACTED

func TestQuotaPriority_GroupOverridesSystem(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{
		ID:                    1,
		SoraStorageQuotaBytes: 0,
		AllowedGroups:         []int64{10REDACTED,
REDACTED

	groupRepo := newStubGroupRepoForQuota()
	groupRepo.groups[10] = &Group{ID: 10, SoraStorageQuotaBytes: 20 * 1024 * 1024REDACTED

	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraDefaultStorageQuotaBytes: "104857600", // 100MB
REDACTED)
	settingService := NewSettingService(settingRepo, &config.Config{REDACTED)

	svc := NewSoraQuotaService(userRepo, groupRepo, settingService)
	quota, err := svc.GetQuota(context.Background(), 1)
REDACTED
	require.Equal(t, "group", quota.Source) // 分组级优先于系统
	require.Equal(t, int64(20*1024*1024), quota.QuotaBytes)
REDACTED

func TestQuotaPriority_FallbackToSystem(t *testing.T) {
	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{
		ID:                    1,
		SoraStorageQuotaBytes: 0,
		AllowedGroups:         []int64{10REDACTED,
REDACTED

	groupRepo := newStubGroupRepoForQuota()
	groupRepo.groups[10] = &Group{ID: 10, SoraStorageQuotaBytes: 0REDACTED // 分组无配额

	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraDefaultStorageQuotaBytes: "52428800", // 50MB
REDACTED)
	settingService := NewSettingService(settingRepo, &config.Config{REDACTED)

	svc := NewSoraQuotaService(userRepo, groupRepo, settingService)
	quota, err := svc.GetQuota(context.Background(), 1)
REDACTED
	require.Equal(t, "system", quota.Source)
	require.Equal(t, int64(52428800), quota.QuotaBytes)
REDACTED
