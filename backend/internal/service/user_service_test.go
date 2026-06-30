//go:build unit

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// --- mock: UserRepository ---

type mockUserRepo struct {
	updateBalanceErr        error
	updateBalanceFn         func(ctx context.Context, id int64, amount float64) error
	deductBalanceFn         func(ctx context.Context, id int64, amount float64) error
	getByIDUser             *User
	getByIDErr              error
	identities              []UserAuthIdentityRecord
	unbindIdentityErr       error
	unboundProviders        []string
	updateLastActiveErr     error
	updateLastActiveUserIDs []int64
	updateLastActiveAt      []time.Time
	updateFn                func(ctx context.Context, user *User) error
	updateCalls             int
	upsertAvatarFn          func(ctx context.Context, userID int64, input UpsertUserAvatarInput) (*UserAvatar, error)
	upsertAvatarArgs        []UpsertUserAvatarInput
	deleteAvatarFn          func(ctx context.Context, userID int64) error
	deleteAvatarIDs         []int64
	getAvatarFn             func(ctx context.Context, userID int64) (*UserAvatar, error)
	txCalls                 int
REDACTED

type mockUserRepoTxKey struct{REDACTED

type mockUserRepoTxState struct {
	getByIDUser      *User
	upsertAvatarArgs []UpsertUserAvatarInput
	deleteAvatarIDs  []int64
REDACTED

type mockUserSettingRepo struct {
	values map[string]string
REDACTED

func (m *mockUserSettingRepo) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
REDACTED

func (m *mockUserSettingRepo) GetValue(context.Context, string) (string, error) {
	panic("unexpected GetValue call")
REDACTED

func (m *mockUserSettingRepo) Set(context.Context, string, string) error {
	panic("unexpected Set call")
REDACTED

func (m *mockUserSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := m.values[key]; ok {
			out[key] = value
	REDACTED
REDACTED
	return out, nil
REDACTED

func (m *mockUserSettingRepo) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
REDACTED

func (m *mockUserSettingRepo) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
REDACTED

func (m *mockUserSettingRepo) Delete(context.Context, string) error {
	panic("unexpected Delete call")
REDACTED

func (m *mockUserRepo) Create(context.Context, *User) error { return nil REDACTED
func (m *mockUserRepo) GetByID(ctx context.Context, _ int64) (*User, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
REDACTED
	if txState, _ := ctx.Value(mockUserRepoTxKey{REDACTED).(*mockUserRepoTxState); txState != nil && txState.getByIDUser != nil {
		cloned := *txState.getByIDUser
		return &cloned, nil
REDACTED
	if m.getByIDUser != nil {
		cloned := *m.getByIDUser
		return &cloned, nil
REDACTED
	return &User{REDACTED, nil
REDACTED
func (m *mockUserRepo) GetByEmail(context.Context, string) (*User, error) { return &User{REDACTED, nil REDACTED
func (m *mockUserRepo) GetFirstAdmin(context.Context) (*User, error)      { return &User{REDACTED, nil REDACTED
func (m *mockUserRepo) Update(ctx context.Context, user *User) error {
	m.updateCalls++
	if m.updateFn != nil {
		return m.updateFn(ctx, user)
REDACTED
	return nil
REDACTED
func (m *mockUserRepo) Delete(context.Context, int64) error { return nil REDACTED
func (m *mockUserRepo) GetUserAvatar(ctx context.Context, userID int64) (*UserAvatar, error) {
	if m.getAvatarFn != nil {
		return m.getAvatarFn(ctx, userID)
REDACTED
	return nil, nil
REDACTED
func (m *mockUserRepo) UpsertUserAvatar(ctx context.Context, userID int64, input UpsertUserAvatarInput) (*UserAvatar, error) {
	if txState, _ := ctx.Value(mockUserRepoTxKey{REDACTED).(*mockUserRepoTxState); txState != nil {
		txState.upsertAvatarArgs = append(txState.upsertAvatarArgs, input)
		if txState.getByIDUser != nil {
			txState.getByIDUser.AvatarURL = input.URL
			txState.getByIDUser.AvatarSource = input.StorageProvider
			txState.getByIDUser.AvatarMIME = input.ContentType
			txState.getByIDUser.AvatarByteSize = input.ByteSize
			txState.getByIDUser.AvatarSHA256 = input.SHA256
	REDACTED
		if m.upsertAvatarFn != nil {
			return m.upsertAvatarFn(ctx, userID, input)
	REDACTED
		return &UserAvatar{
			StorageProvider: input.StorageProvider,
			StorageKey:      input.StorageKey,
			URL:             input.URL,
			ContentType:     input.ContentType,
			ByteSize:        input.ByteSize,
			SHA256:          input.SHA256,
	REDACTED, nil
REDACTED
	m.upsertAvatarArgs = append(m.upsertAvatarArgs, input)
	if m.upsertAvatarFn != nil {
		return m.upsertAvatarFn(ctx, userID, input)
REDACTED
	return &UserAvatar{
		StorageProvider: input.StorageProvider,
		StorageKey:      input.StorageKey,
		URL:             input.URL,
		ContentType:     input.ContentType,
		ByteSize:        input.ByteSize,
		SHA256:          input.SHA256,
REDACTED, nil
REDACTED
func (m *mockUserRepo) DeleteUserAvatar(ctx context.Context, userID int64) error {
	if txState, _ := ctx.Value(mockUserRepoTxKey{REDACTED).(*mockUserRepoTxState); txState != nil {
		txState.deleteAvatarIDs = append(txState.deleteAvatarIDs, userID)
		if txState.getByIDUser != nil {
			txState.getByIDUser.AvatarURL = ""
			txState.getByIDUser.AvatarSource = ""
			txState.getByIDUser.AvatarMIME = ""
			txState.getByIDUser.AvatarByteSize = 0
			txState.getByIDUser.AvatarSHA256 = ""
	REDACTED
		if m.deleteAvatarFn != nil {
			return m.deleteAvatarFn(ctx, userID)
	REDACTED
		return nil
REDACTED
	m.deleteAvatarIDs = append(m.deleteAvatarIDs, userID)
	if m.deleteAvatarFn != nil {
		return m.deleteAvatarFn(ctx, userID)
REDACTED
	return nil
REDACTED
func (m *mockUserRepo) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (m *mockUserRepo) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (m *mockUserRepo) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	if m.updateBalanceFn != nil {
		return m.updateBalanceFn(ctx, id, amount)
REDACTED
	return m.updateBalanceErr
REDACTED
func (m *mockUserRepo) UpdateUserLastActiveAt(_ context.Context, userID int64, activeAt time.Time) error {
	m.updateLastActiveUserIDs = append(m.updateLastActiveUserIDs, userID)
	m.updateLastActiveAt = append(m.updateLastActiveAt, activeAt)
	return m.updateLastActiveErr
REDACTED
func (m *mockUserRepo) DeductBalance(ctx context.Context, id int64, amount float64) error {
	if m.deductBalanceFn != nil {
		return m.deductBalanceFn(ctx, id, amount)
REDACTED
	return nil
REDACTED
func (m *mockUserRepo) UpdateConcurrency(context.Context, int64, int) error { return nil REDACTED
func (m *mockUserRepo) ExistsByEmail(context.Context, string) (bool, error) { return false, nil REDACTED
func (m *mockUserRepo) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
REDACTED

func (m *mockUserRepo) BatchSetConcurrency(context.Context, []int64, int) (int, error) { return 0, nil REDACTED
func (m *mockUserRepo) BatchAddConcurrency(context.Context, []int64, int) (int, error) { return 0, nil REDACTED
func (m *mockUserRepo) AddGroupToAllowedGroups(context.Context, int64, int64) error    { return nil REDACTED
func (m *mockUserRepo) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	out := make([]UserAuthIdentityRecord, len(m.identities))
	copy(out, m.identities)
	return out, nil
REDACTED
func (m *mockUserRepo) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{REDACTED, nil
REDACTED
func (m *mockUserRepo) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
REDACTED
func (m *mockUserRepo) UpdateTotpSecret(context.Context, int64, *string) error { return nil REDACTED
func (m *mockUserRepo) EnableTotp(context.Context, int64) error                { return nil REDACTED
func (m *mockUserRepo) DisableTotp(context.Context, int64) error               { return nil REDACTED
func (m *mockUserRepo) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
REDACTED
func (m *mockUserRepo) UnbindUserAuthProvider(_ context.Context, _ int64, provider string) error {
	if m.unbindIdentityErr != nil {
		return m.unbindIdentityErr
REDACTED
	m.unboundProviders = append(m.unboundProviders, provider)
	filtered := m.identities[:0]
	for _, identity := range m.identities {
		if identity.ProviderType == provider {
			continue
	REDACTED
		filtered = append(filtered, identity)
REDACTED
	m.identities = append([]UserAuthIdentityRecord(nil), filtered...)
	return nil
REDACTED

func (m *mockUserRepo) GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error) {
	return m.GetByID(ctx, id)
REDACTED

func (m *mockUserRepo) WithUserProfileIdentityTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	m.txCalls++
	txState := &mockUserRepoTxState{
		upsertAvatarArgs: append([]UpsertUserAvatarInput(nil), m.upsertAvatarArgs...),
		deleteAvatarIDs:  append([]int64(nil), m.deleteAvatarIDs...),
REDACTED
	if m.getByIDUser != nil {
		userCopy := *m.getByIDUser
		txState.getByIDUser = &userCopy
REDACTED
	err := fn(context.WithValue(ctx, mockUserRepoTxKey{REDACTED, txState))
	if err != nil {
		return err
REDACTED
	m.getByIDUser = txState.getByIDUser
	m.upsertAvatarArgs = txState.upsertAvatarArgs
	m.deleteAvatarIDs = txState.deleteAvatarIDs
	return nil
REDACTED

// --- mock: APIKeyAuthCacheInvalidator ---

type mockAuthCacheInvalidator struct {
	invalidatedUserIDs []int64
	mu                 sync.Mutex
REDACTED

func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByKey(context.Context, string)    {REDACTED
func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByGroupID(context.Context, int64) {REDACTED
func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidatedUserIDs = append(m.invalidatedUserIDs, userID)
REDACTED

// --- mock: BillingCache ---

type mockBillingCache struct {
	invalidateErr       error
	invalidateCallCount atomic.Int64
	invalidatedUserIDs  []int64
	mu                  sync.Mutex
REDACTED

func (m *mockBillingCache) GetUserBalance(context.Context, int64) (float64, error)  { return 0, nil REDACTED
func (m *mockBillingCache) SetUserBalance(context.Context, int64, float64) error    { return nil REDACTED
func (m *mockBillingCache) DeductUserBalance(context.Context, int64, float64) error { return nil REDACTED
func (m *mockBillingCache) InvalidateUserBalance(_ context.Context, userID int64) error {
	m.invalidateCallCount.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidatedUserIDs = append(m.invalidatedUserIDs, userID)
	return m.invalidateErr
REDACTED
func (m *mockBillingCache) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	return nil, nil
REDACTED
func (m *mockBillingCache) SetSubscriptionCache(context.Context, int64, int64, *SubscriptionCacheData) error {
	return nil
REDACTED
func (m *mockBillingCache) UpdateSubscriptionUsage(context.Context, int64, int64, float64) error {
	return nil
REDACTED
func (m *mockBillingCache) InvalidateSubscriptionCache(context.Context, int64, int64) error {
	return nil
REDACTED
func (m *mockBillingCache) GetAPIKeyRateLimit(context.Context, int64) (*APIKeyRateLimitCacheData, error) {
	return nil, nil
REDACTED
func (m *mockBillingCache) SetAPIKeyRateLimit(context.Context, int64, *APIKeyRateLimitCacheData) error {
	return nil
REDACTED
func (m *mockBillingCache) UpdateAPIKeyRateLimitUsage(context.Context, int64, float64) error {
	return nil
REDACTED
func (m *mockBillingCache) InvalidateAPIKeyRateLimit(context.Context, int64) error {
	return nil
REDACTED

func (m *mockBillingCache) GetUserPlatformQuotaCache(context.Context, int64, string) (*UserPlatformQuotaCacheEntry, bool, error) {
	return nil, false, nil
REDACTED

func (m *mockBillingCache) SetUserPlatformQuotaCache(context.Context, int64, string, *UserPlatformQuotaCacheEntry, time.Duration) error {
	return nil
REDACTED

func (m *mockBillingCache) DeleteUserPlatformQuotaCache(context.Context, int64, string) error {
	return nil
REDACTED

func (m *mockBillingCache) IncrUserPlatformQuotaUsageCache(context.Context, int64, string, float64, time.Duration, bool) error {
	return nil
REDACTED

func (m *mockBillingCache) PopDirtyUserPlatformQuotaKeys(context.Context, int) ([]UserPlatformQuotaKey, error) {
	return nil, nil
REDACTED

func (m *mockBillingCache) ReaddDirtyUserPlatformQuotaKeys(context.Context, []UserPlatformQuotaKey) error {
	return nil
REDACTED

func (m *mockBillingCache) BatchGetUserPlatformQuotaCache(context.Context, []UserPlatformQuotaKey) ([]*UserPlatformQuotaCacheEntry, error) {
	return nil, nil
REDACTED

// --- 测试 ---

func TestUpdateBalance_Success(t *testing.T) {
	repo := &mockUserRepo{REDACTED
	cache := &mockBillingCache{REDACTED
	svc := NewUserService(repo, nil, nil, cache)

	err := svc.UpdateBalance(context.Background(), 42, 100.0)
REDACTED

	// 等待异步 goroutine 完成
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
REDACTED, 2*time.Second, 10*time.Millisecond, "应异步调用 InvalidateUserBalance")

	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Equal(t, []int64{42REDACTED, cache.invalidatedUserIDs, "应对 userID=42 失效缓存")
REDACTED

func TestGetProfileIdentitySummaries_AllowsUnbindWhenAnotherLoginMethodRemains(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:    7,
			Email: "alice@example.com",
	REDACTED,
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "alice@example.com",
		REDACTED,
			{
				ProviderType:    "linuxdo",
				ProviderKey:     "linuxdo",
				ProviderSubject: "linuxdo-subject-123456",
				Metadata: map[string]any{
					"username": "linuxdo-handle",
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 7, repo.getByIDUser)

REDACTED
	require.True(t, summaries.LinuxDo.Bound)
	require.True(t, summaries.LinuxDo.CanUnbind)
	require.Equal(t, "linuxdo-handle", summaries.LinuxDo.DisplayName)
	require.NotEmpty(t, summaries.LinuxDo.SubjectHint)
REDACTED

func TestUnbindUserAuthProviderRejectsLastRemainingLoginMethod(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:    9,
			Email: "only-user@linuxdo-connect.invalid",
	REDACTED,
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "linuxdo",
				ProviderKey:     "linuxdo",
				ProviderSubject: "linuxdo-only-subject",
		REDACTED,
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	_, err := svc.UnbindUserAuthProvider(context.Background(), 9, "linuxdo")

	require.ErrorIs(t, err, ErrIdentityUnbindLastMethod)
	require.Empty(t, repo.unboundProviders)
REDACTED

func TestGetProfileIdentitySummaries_DoesNotTreatOAuthOnlyCompatEmailAsAlternativeLoginMethod(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           10,
			Email:        "oauth-only@example.com",
			SignupSource: "oidc",
	REDACTED,
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "oidc",
				ProviderKey:     "https://issuer.example.com",
				ProviderSubject: "oidc-only-subject",
		REDACTED,
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 10, repo.getByIDUser)

REDACTED
	require.False(t, summaries.OIDC.CanUnbind)

	_, err = svc.UnbindUserAuthProvider(context.Background(), 10, "oidc")
	require.ErrorIs(t, err, ErrIdentityUnbindLastMethod)
	require.Empty(t, repo.unboundProviders)
REDACTED

func TestGetProfileIdentitySummaries_DoesNotTreatCompatBackfilledEmailIdentityAsAlternativeLoginMethod(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           11,
			Email:        "oauth-only@example.com",
			SignupSource: "wechat",
	REDACTED,
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "oauth-only@example.com",
				Metadata: map[string]any{
					"backfill_source": "users.email",
					"migration":       "109_auth_identity_compat_backfill",
			REDACTED,
		REDACTED,
			{
				ProviderType:    "wechat",
				ProviderKey:     "wechat",
				ProviderSubject: "wechat-only-subject",
		REDACTED,
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 11, repo.getByIDUser)

REDACTED
	require.True(t, summaries.Email.Bound)
	require.False(t, summaries.WeChat.CanUnbind)

	_, err = svc.UnbindUserAuthProvider(context.Background(), 11, "wechat")
	require.ErrorIs(t, err, ErrIdentityUnbindLastMethod)
	require.Empty(t, repo.unboundProviders)
REDACTED

func TestUnbindUserAuthProviderRemovesProviderAndReturnsUpdatedProfile(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:    12,
			Email: "alice@example.com",
	REDACTED,
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "alice@example.com",
		REDACTED,
			{
				ProviderType:    "linuxdo",
				ProviderKey:     "linuxdo",
				ProviderSubject: "linuxdo-subject-12",
		REDACTED,
	REDACTED,
REDACTED
	invalidator := &mockAuthCacheInvalidator{REDACTED
	svc := NewUserService(repo, nil, invalidator, nil)

	user, err := svc.UnbindUserAuthProvider(context.Background(), 12, "linuxdo")

REDACTED
	require.Equal(t, []string{"linuxdo"REDACTED, repo.unboundProviders)
	require.Equal(t, int64(12), user.ID)
	require.Equal(t, []int64{12REDACTED, invalidator.invalidatedUserIDs)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 12, user)
REDACTED
	require.False(t, summaries.LinuxDo.Bound)
	require.True(t, summaries.LinuxDo.CanBind)
REDACTED

func TestGetProfileIdentitySummaries_HidesBindActionWhenProviderExplicitlyDisabled(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:    15,
			Email: "alice@example.com",
	REDACTED,
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "alice@example.com",
		REDACTED,
	REDACTED,
REDACTED
	settingRepo := &mockUserSettingRepo{
		values: map[string]string{
			SettingKeyLinuxDoConnectEnabled: "false",
	REDACTED,
REDACTED
	svc := NewUserService(repo, settingRepo, nil, nil)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 15, repo.getByIDUser)

REDACTED
	require.False(t, summaries.LinuxDo.Bound)
	require.False(t, summaries.LinuxDo.CanBind)
	require.Empty(t, summaries.LinuxDo.BindStartPath)
REDACTED

func TestGetProfileIdentitySummaries_UsesBindStartRoute(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:    16,
			Email: "alice@example.com",
	REDACTED,
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "alice@example.com",
		REDACTED,
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 16, repo.getByIDUser)

REDACTED
	require.Equal(
		t,
		"/api/v1/auth/oauth/linuxdo/bind/start?intent=bind_current_user&redirect=%2Fsettings%2Fprofile",
		summaries.LinuxDo.BindStartPath,
	)
	require.Equal(
		t,
		"/api/v1/auth/oauth/oidc/bind/start?intent=bind_current_user&redirect=%2Fsettings%2Fprofile",
		summaries.OIDC.BindStartPath,
	)
	require.Equal(
		t,
		"/api/v1/auth/oauth/wechat/bind/start?intent=bind_current_user&redirect=%2Fsettings%2Fprofile",
		summaries.WeChat.BindStartPath,
	)
REDACTED

func TestUpdateBalance_NilBillingCache_NoPanic(t *testing.T) {
	repo := &mockUserRepo{REDACTED
	svc := NewUserService(repo, nil, nil, nil) // billingCache = nil

	err := svc.UpdateBalance(context.Background(), 1, 50.0)
	require.NoError(t, err, "billingCache 为 nil 时不应 panic")
REDACTED

func TestUpdateBalance_CacheFailure_DoesNotAffectReturn(t *testing.T) {
	repo := &mockUserRepo{REDACTED
	cache := &mockBillingCache{invalidateErr: errors.New("redis connection refused")REDACTED
	svc := NewUserService(repo, nil, nil, cache)

	err := svc.UpdateBalance(context.Background(), 99, 200.0)
	require.NoError(t, err, "缓存失效失败不应影响主流程返回值")

	// 等待异步 goroutine 完成（即使失败也应调用）
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
REDACTED, 2*time.Second, 10*time.Millisecond, "即使失败也应调用 InvalidateUserBalance")
REDACTED

func TestTouchLastActive_UpdatesWhenStale(t *testing.T) {
	stale := time.Now().Add(-11 * time.Minute)
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           42,
			LastActiveAt: &stale,
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	svc.TouchLastActive(context.Background(), 42)

	require.Equal(t, []int64{42REDACTED, repo.updateLastActiveUserIDs)
	require.Len(t, repo.updateLastActiveAt, 1)
	require.WithinDuration(t, time.Now(), repo.updateLastActiveAt[0], 2*time.Second)
REDACTED

func TestTouchLastActive_SkipsWhenRecent(t *testing.T) {
	recent := time.Now().Add(-time.Minute)
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           42,
			LastActiveAt: &recent,
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	svc.TouchLastActive(context.Background(), 42)

	require.Empty(t, repo.updateLastActiveUserIDs)
	require.Empty(t, repo.updateLastActiveAt)
REDACTED

func TestUpdateBalance_RepoError_ReturnsError(t *testing.T) {
	repo := &mockUserRepo{updateBalanceErr: errors.New("database error")REDACTED
	cache := &mockBillingCache{REDACTED
	svc := NewUserService(repo, nil, nil, cache)

	err := svc.UpdateBalance(context.Background(), 1, 100.0)
	require.Error(t, err, "repo 失败时应返回错误")
	require.Contains(t, err.Error(), "update balance")

	// repo 失败时不应触发缓存失效
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, int64(0), cache.invalidateCallCount.Load(),
		"repo 失败时不应调用 InvalidateUserBalance")
REDACTED

func TestUpdateBalance_WithAuthCacheInvalidator(t *testing.T) {
	repo := &mockUserRepo{REDACTED
	auth := &mockAuthCacheInvalidator{REDACTED
	cache := &mockBillingCache{REDACTED
	svc := NewUserService(repo, nil, auth, cache)

	err := svc.UpdateBalance(context.Background(), 77, 300.0)
REDACTED

	// 验证 auth cache 同步失效
	auth.mu.Lock()
	require.Equal(t, []int64{77REDACTED, auth.invalidatedUserIDs)
	auth.mu.Unlock()

	// 验证 billing cache 异步失效
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
REDACTED, 2*time.Second, 10*time.Millisecond)
REDACTED

func TestNewUserService_FieldsAssignment(t *testing.T) {
	repo := &mockUserRepo{REDACTED
	auth := &mockAuthCacheInvalidator{REDACTED
	cache := &mockBillingCache{REDACTED

	svc := NewUserService(repo, nil, auth, cache)
	require.NotNil(t, svc)
	require.Equal(t, repo, svc.userRepo)
	require.Equal(t, auth, svc.authCacheInvalidator)
	require.Equal(t, cache, svc.billingCache)
REDACTED

func TestUpdateProfile_StoresInlineAvatarWithinLimit(t *testing.T) {
	raw := []byte("small-avatar")
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	expectedSum := sha256.Sum256(raw)
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       7,
			Email:    "avatar@example.com",
			Username: "avatar-user",
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	updated, err := svc.UpdateProfile(context.Background(), 7, UpdateProfileRequest{
		AvatarURL: &dataURL,
REDACTED)
REDACTED
	require.Len(t, repo.upsertAvatarArgs, 1)
	require.Equal(t, "inline", repo.upsertAvatarArgs[0].StorageProvider)
	require.Equal(t, "image/png", repo.upsertAvatarArgs[0].ContentType)
	require.Equal(t, len(raw), repo.upsertAvatarArgs[0].ByteSize)
	require.Equal(t, hex.EncodeToString(expectedSum[:]), repo.upsertAvatarArgs[0].SHA256)
	require.Equal(t, dataURL, updated.AvatarURL)
	require.Equal(t, "inline", updated.AvatarSource)
	require.Equal(t, "image/png", updated.AvatarMIME)
	require.Equal(t, len(raw), updated.AvatarByteSize)
	require.Equal(t, hex.EncodeToString(expectedSum[:]), updated.AvatarSHA256)
REDACTED

func TestUpdateProfile_CompressesInlineAvatarToTwentyKilobytes(t *testing.T) {
	var encoded bytes.Buffer
	for _, size := range []int{192, 224, 256, 288REDACTED {
		encoded.Reset()
		var img image.RGBA
		img.Rect = image.Rect(0, 0, size, size)
		img.Stride = size * 4
		img.Pix = make([]byte, size*size*4)
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				offset := y*img.Stride + x*4
				img.Pix[offset] = uint8((x*x + y*17) % 255)
				img.Pix[offset+1] = uint8((y*y + x*29) % 255)
				img.Pix[offset+2] = uint8(((x * y) + x*13 + y*7) % 255)
				img.Pix[offset+3] = 0xff
		REDACTED
	REDACTED
		require.NoError(t, png.Encode(&encoded, &img))
		if encoded.Len() > 20*1024 && encoded.Len() <= maxInlineAvatarBytes {
			break
	REDACTED
REDACTED
	require.Greater(t, encoded.Len(), 20*1024)
	require.LessOrEqual(t, encoded.Len(), maxInlineAvatarBytes)

	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       17,
			Email:    "avatar-compress@example.com",
			Username: "avatar-compress",
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	updated, err := svc.UpdateProfile(context.Background(), 17, UpdateProfileRequest{
		AvatarURL: &dataURL,
REDACTED)
REDACTED
	require.Len(t, repo.upsertAvatarArgs, 1)
	require.Equal(t, "inline", repo.upsertAvatarArgs[0].StorageProvider)
	require.LessOrEqual(t, repo.upsertAvatarArgs[0].ByteSize, 20*1024)
	require.Equal(t, "image/jpeg", repo.upsertAvatarArgs[0].ContentType)
	require.Contains(t, repo.upsertAvatarArgs[0].URL, "data:image/jpeg;base64,")
	require.Equal(t, "inline", updated.AvatarSource)
	require.Equal(t, "image/jpeg", updated.AvatarMIME)
	require.LessOrEqual(t, updated.AvatarByteSize, 20*1024)
	require.Contains(t, updated.AvatarURL, "data:image/jpeg;base64,")
	require.NotEmpty(t, updated.AvatarSHA256)
REDACTED

func TestUpdateProfile_RejectsInlineAvatarOverLimit(t *testing.T) {
	raw := make([]byte, maxInlineAvatarBytes+1)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       8,
			Email:    "large-avatar@example.com",
			Username: "too-large",
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	_, err := svc.UpdateProfile(context.Background(), 8, UpdateProfileRequest{
		AvatarURL: &dataURL,
REDACTED)
	require.ErrorIs(t, err, ErrAvatarTooLarge)
	require.Empty(t, repo.upsertAvatarArgs)
	require.Empty(t, repo.deleteAvatarIDs)
	require.Zero(t, repo.updateCalls)
REDACTED

func TestUpdateProfile_StoresRemoteAvatarURL(t *testing.T) {
	remoteURL := "https://cdn.example.com/avatar.png"
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       9,
			Email:    "remote-avatar@example.com",
			Username: "remote-avatar",
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	updated, err := svc.UpdateProfile(context.Background(), 9, UpdateProfileRequest{
		AvatarURL: &remoteURL,
REDACTED)
REDACTED
	require.Len(t, repo.upsertAvatarArgs, 1)
	require.Equal(t, "remote_url", repo.upsertAvatarArgs[0].StorageProvider)
	require.Equal(t, remoteURL, repo.upsertAvatarArgs[0].URL)
	require.Equal(t, remoteURL, updated.AvatarURL)
	require.Equal(t, "remote_url", updated.AvatarSource)
	require.Zero(t, updated.AvatarByteSize)
REDACTED

func TestUpdateProfile_DeletesAvatarOnEmptyString(t *testing.T) {
	empty := ""
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           10,
			Email:        "delete-avatar@example.com",
			Username:     "delete-avatar",
			AvatarURL:    "https://cdn.example.com/old.png",
			AvatarSource: "remote_url",
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	updated, err := svc.UpdateProfile(context.Background(), 10, UpdateProfileRequest{
		AvatarURL: &empty,
REDACTED)
REDACTED
	require.Equal(t, []int64{10REDACTED, repo.deleteAvatarIDs)
	require.Empty(t, repo.upsertAvatarArgs)
	require.Empty(t, updated.AvatarURL)
	require.Empty(t, updated.AvatarSource)
REDACTED

func TestUpdateProfile_RollsBackAvatarMutationWhenUserUpdateFails(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           11,
			Email:        "rollback@example.com",
			AvatarURL:    "https://cdn.example.com/original.png",
			AvatarSource: "remote_url",
	REDACTED,
		updateFn: func(context.Context, *User) error {
			return errors.New("write user failed")
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	remoteURL := "https://cdn.example.com/new.png"
	_, err := svc.UpdateProfile(context.Background(), 11, UpdateProfileRequest{
		AvatarURL: &remoteURL,
REDACTED)

	require.EqualError(t, err, "update user: write user failed")
	require.Equal(t, 1, repo.txCalls)
	require.Empty(t, repo.upsertAvatarArgs)
	require.Empty(t, repo.deleteAvatarIDs)
	require.Equal(t, "https://cdn.example.com/original.png", repo.getByIDUser.AvatarURL)
	require.Equal(t, "remote_url", repo.getByIDUser.AvatarSource)
REDACTED

func TestGetProfile_HydratesAvatarFromRepository(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       12,
			Email:    "profile-avatar@example.com",
			Username: "profile-avatar",
	REDACTED,
		getAvatarFn: func(context.Context, int64) (*UserAvatar, error) {
			return &UserAvatar{
				StorageProvider: "remote_url",
				URL:             "https://cdn.example.com/profile.png",
		REDACTED, nil
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	user, err := svc.GetProfile(context.Background(), 12)
REDACTED
	require.Equal(t, "https://cdn.example.com/profile.png", user.AvatarURL)
	require.Equal(t, "remote_url", user.AvatarSource)
REDACTED
