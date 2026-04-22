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
}

type mockUserRepoTxKey struct{}

type mockUserRepoTxState struct {
	getByIDUser      *User
	upsertAvatarArgs []UpsertUserAvatarInput
	deleteAvatarIDs  []int64
}

type mockUserSettingRepo struct {
	values map[string]string
}

func (m *mockUserSettingRepo) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (m *mockUserSettingRepo) GetValue(context.Context, string) (string, error) {
	panic("unexpected GetValue call")
}

func (m *mockUserSettingRepo) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (m *mockUserSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := m.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (m *mockUserSettingRepo) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (m *mockUserSettingRepo) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (m *mockUserSettingRepo) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func (m *mockUserRepo) Create(context.Context, *User) error { return nil }
func (m *mockUserRepo) GetByID(ctx context.Context, _ int64) (*User, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	if txState, _ := ctx.Value(mockUserRepoTxKey{}).(*mockUserRepoTxState); txState != nil && txState.getByIDUser != nil {
		cloned := *txState.getByIDUser
		return &cloned, nil
	}
	if m.getByIDUser != nil {
		cloned := *m.getByIDUser
		return &cloned, nil
	}
	return &User{}, nil
}
func (m *mockUserRepo) GetByEmail(context.Context, string) (*User, error) { return &User{}, nil }
func (m *mockUserRepo) GetFirstAdmin(context.Context) (*User, error)      { return &User{}, nil }
func (m *mockUserRepo) Update(ctx context.Context, user *User) error {
	m.updateCalls++
	if m.updateFn != nil {
		return m.updateFn(ctx, user)
	}
	return nil
}
func (m *mockUserRepo) Delete(context.Context, int64) error { return nil }
func (m *mockUserRepo) GetUserAvatar(ctx context.Context, userID int64) (*UserAvatar, error) {
	if m.getAvatarFn != nil {
		return m.getAvatarFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockUserRepo) UpsertUserAvatar(ctx context.Context, userID int64, input UpsertUserAvatarInput) (*UserAvatar, error) {
	if txState, _ := ctx.Value(mockUserRepoTxKey{}).(*mockUserRepoTxState); txState != nil {
		txState.upsertAvatarArgs = append(txState.upsertAvatarArgs, input)
		if txState.getByIDUser != nil {
			txState.getByIDUser.AvatarURL = input.URL
			txState.getByIDUser.AvatarSource = input.StorageProvider
			txState.getByIDUser.AvatarMIME = input.ContentType
			txState.getByIDUser.AvatarByteSize = input.ByteSize
			txState.getByIDUser.AvatarSHA256 = input.SHA256
		}
		if m.upsertAvatarFn != nil {
			return m.upsertAvatarFn(ctx, userID, input)
		}
		return &UserAvatar{
			StorageProvider: input.StorageProvider,
			StorageKey:      input.StorageKey,
			URL:             input.URL,
			ContentType:     input.ContentType,
			ByteSize:        input.ByteSize,
			SHA256:          input.SHA256,
		}, nil
	}
	m.upsertAvatarArgs = append(m.upsertAvatarArgs, input)
	if m.upsertAvatarFn != nil {
		return m.upsertAvatarFn(ctx, userID, input)
	}
	return &UserAvatar{
		StorageProvider: input.StorageProvider,
		StorageKey:      input.StorageKey,
		URL:             input.URL,
		ContentType:     input.ContentType,
		ByteSize:        input.ByteSize,
		SHA256:          input.SHA256,
	}, nil
}
func (m *mockUserRepo) DeleteUserAvatar(ctx context.Context, userID int64) error {
	if txState, _ := ctx.Value(mockUserRepoTxKey{}).(*mockUserRepoTxState); txState != nil {
		txState.deleteAvatarIDs = append(txState.deleteAvatarIDs, userID)
		if txState.getByIDUser != nil {
			txState.getByIDUser.AvatarURL = ""
			txState.getByIDUser.AvatarSource = ""
			txState.getByIDUser.AvatarMIME = ""
			txState.getByIDUser.AvatarByteSize = 0
			txState.getByIDUser.AvatarSHA256 = ""
		}
		if m.deleteAvatarFn != nil {
			return m.deleteAvatarFn(ctx, userID)
		}
		return nil
	}
	m.deleteAvatarIDs = append(m.deleteAvatarIDs, userID)
	if m.deleteAvatarFn != nil {
		return m.deleteAvatarFn(ctx, userID)
	}
	return nil
}
func (m *mockUserRepo) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockUserRepo) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockUserRepo) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	if m.updateBalanceFn != nil {
		return m.updateBalanceFn(ctx, id, amount)
	}
	return m.updateBalanceErr
}
func (m *mockUserRepo) UpdateUserLastActiveAt(_ context.Context, userID int64, activeAt time.Time) error {
	m.updateLastActiveUserIDs = append(m.updateLastActiveUserIDs, userID)
	m.updateLastActiveAt = append(m.updateLastActiveAt, activeAt)
	return m.updateLastActiveErr
}
func (m *mockUserRepo) DeductBalance(context.Context, int64, float64) error { return nil }
func (m *mockUserRepo) UpdateConcurrency(context.Context, int64, int) error { return nil }
func (m *mockUserRepo) ExistsByEmail(context.Context, string) (bool, error) { return false, nil }
func (m *mockUserRepo) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}
func (m *mockUserRepo) AddGroupToAllowedGroups(context.Context, int64, int64) error { return nil }
func (m *mockUserRepo) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	out := make([]UserAuthIdentityRecord, len(m.identities))
	copy(out, m.identities)
	return out, nil
}
func (m *mockUserRepo) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{}, nil
}
func (m *mockUserRepo) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
}
func (m *mockUserRepo) UpdateTotpSecret(context.Context, int64, *string) error { return nil }
func (m *mockUserRepo) EnableTotp(context.Context, int64) error                { return nil }
func (m *mockUserRepo) DisableTotp(context.Context, int64) error               { return nil }
func (m *mockUserRepo) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (m *mockUserRepo) UnbindUserAuthProvider(_ context.Context, _ int64, provider string) error {
	if m.unbindIdentityErr != nil {
		return m.unbindIdentityErr
	}
	m.unboundProviders = append(m.unboundProviders, provider)
	filtered := m.identities[:0]
	for _, identity := range m.identities {
		if identity.ProviderType == provider {
			continue
		}
		filtered = append(filtered, identity)
	}
	m.identities = append([]UserAuthIdentityRecord(nil), filtered...)
	return nil
}

func (m *mockUserRepo) WithUserProfileIdentityTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	m.txCalls++
	txState := &mockUserRepoTxState{
		upsertAvatarArgs: append([]UpsertUserAvatarInput(nil), m.upsertAvatarArgs...),
		deleteAvatarIDs:  append([]int64(nil), m.deleteAvatarIDs...),
	}
	if m.getByIDUser != nil {
		userCopy := *m.getByIDUser
		txState.getByIDUser = &userCopy
	}
	err := fn(context.WithValue(ctx, mockUserRepoTxKey{}, txState))
	if err != nil {
		return err
	}
	m.getByIDUser = txState.getByIDUser
	m.upsertAvatarArgs = txState.upsertAvatarArgs
	m.deleteAvatarIDs = txState.deleteAvatarIDs
	return nil
}

// --- mock: APIKeyAuthCacheInvalidator ---

type mockAuthCacheInvalidator struct {
	invalidatedUserIDs []int64
	mu                 sync.Mutex
}

func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByKey(context.Context, string)    {}
func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByGroupID(context.Context, int64) {}
func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidatedUserIDs = append(m.invalidatedUserIDs, userID)
}

// --- mock: BillingCache ---

type mockBillingCache struct {
	invalidateErr       error
	invalidateCallCount atomic.Int64
	invalidatedUserIDs  []int64
	mu                  sync.Mutex
}

func (m *mockBillingCache) GetUserBalance(context.Context, int64) (float64, error)  { return 0, nil }
func (m *mockBillingCache) SetUserBalance(context.Context, int64, float64) error    { return nil }
func (m *mockBillingCache) DeductUserBalance(context.Context, int64, float64) error { return nil }
func (m *mockBillingCache) InvalidateUserBalance(_ context.Context, userID int64) error {
	m.invalidateCallCount.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidatedUserIDs = append(m.invalidatedUserIDs, userID)
	return m.invalidateErr
}
func (m *mockBillingCache) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	return nil, nil
}
func (m *mockBillingCache) SetSubscriptionCache(context.Context, int64, int64, *SubscriptionCacheData) error {
	return nil
}
func (m *mockBillingCache) UpdateSubscriptionUsage(context.Context, int64, int64, float64) error {
	return nil
}
func (m *mockBillingCache) InvalidateSubscriptionCache(context.Context, int64, int64) error {
	return nil
}
func (m *mockBillingCache) GetAPIKeyRateLimit(context.Context, int64) (*APIKeyRateLimitCacheData, error) {
	return nil, nil
}
func (m *mockBillingCache) SetAPIKeyRateLimit(context.Context, int64, *APIKeyRateLimitCacheData) error {
	return nil
}
func (m *mockBillingCache) UpdateAPIKeyRateLimitUsage(context.Context, int64, float64) error {
	return nil
}
func (m *mockBillingCache) InvalidateAPIKeyRateLimit(context.Context, int64) error {
	return nil
}

// --- 测试 ---

func TestUpdateBalance_Success(t *testing.T) {
	repo := &mockUserRepo{}
	cache := &mockBillingCache{}
	svc := NewUserService(repo, nil, nil, cache)

	err := svc.UpdateBalance(context.Background(), 42, 100.0)
	require.NoError(t, err)

	// 等待异步 goroutine 完成
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "应异步调用 InvalidateUserBalance")

	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Equal(t, []int64{42}, cache.invalidatedUserIDs, "应对 userID=42 失效缓存")
}

func TestGetProfileIdentitySummaries_AllowsUnbindWhenAnotherLoginMethodRemains(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:    7,
			Email: "alice@example.com",
		},
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "alice@example.com",
			},
			{
				ProviderType:    "linuxdo",
				ProviderKey:     "linuxdo",
				ProviderSubject: "linuxdo-subject-123456",
				Metadata: map[string]any{
					"username": "linuxdo-handle",
				},
			},
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 7, repo.getByIDUser)

	require.NoError(t, err)
	require.True(t, summaries.LinuxDo.Bound)
	require.True(t, summaries.LinuxDo.CanUnbind)
	require.Equal(t, "linuxdo-handle", summaries.LinuxDo.DisplayName)
	require.NotEmpty(t, summaries.LinuxDo.SubjectHint)
}

func TestUnbindUserAuthProviderRejectsLastRemainingLoginMethod(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:    9,
			Email: "only-user@linuxdo-connect.invalid",
		},
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "linuxdo",
				ProviderKey:     "linuxdo",
				ProviderSubject: "linuxdo-only-subject",
			},
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	_, err := svc.UnbindUserAuthProvider(context.Background(), 9, "linuxdo")

	require.ErrorIs(t, err, ErrIdentityUnbindLastMethod)
	require.Empty(t, repo.unboundProviders)
}

func TestGetProfileIdentitySummaries_DoesNotTreatOAuthOnlyCompatEmailAsAlternativeLoginMethod(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           10,
			Email:        "oauth-only@example.com",
			SignupSource: "oidc",
		},
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "oidc",
				ProviderKey:     "https://issuer.example.com",
				ProviderSubject: "oidc-only-subject",
			},
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 10, repo.getByIDUser)

	require.NoError(t, err)
	require.False(t, summaries.OIDC.CanUnbind)

	_, err = svc.UnbindUserAuthProvider(context.Background(), 10, "oidc")
	require.ErrorIs(t, err, ErrIdentityUnbindLastMethod)
	require.Empty(t, repo.unboundProviders)
}

func TestGetProfileIdentitySummaries_DoesNotTreatCompatBackfilledEmailIdentityAsAlternativeLoginMethod(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           11,
			Email:        "oauth-only@example.com",
			SignupSource: "wechat",
		},
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "oauth-only@example.com",
				Metadata: map[string]any{
					"backfill_source": "users.email",
					"migration":       "109_auth_identity_compat_backfill",
				},
			},
			{
				ProviderType:    "wechat",
				ProviderKey:     "wechat",
				ProviderSubject: "wechat-only-subject",
			},
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 11, repo.getByIDUser)

	require.NoError(t, err)
	require.True(t, summaries.Email.Bound)
	require.False(t, summaries.WeChat.CanUnbind)

	_, err = svc.UnbindUserAuthProvider(context.Background(), 11, "wechat")
	require.ErrorIs(t, err, ErrIdentityUnbindLastMethod)
	require.Empty(t, repo.unboundProviders)
}

func TestUnbindUserAuthProviderRemovesProviderAndReturnsUpdatedProfile(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:    12,
			Email: "alice@example.com",
		},
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "alice@example.com",
			},
			{
				ProviderType:    "linuxdo",
				ProviderKey:     "linuxdo",
				ProviderSubject: "linuxdo-subject-12",
			},
		},
	}
	invalidator := &mockAuthCacheInvalidator{}
	svc := NewUserService(repo, nil, invalidator, nil)

	user, err := svc.UnbindUserAuthProvider(context.Background(), 12, "linuxdo")

	require.NoError(t, err)
	require.Equal(t, []string{"linuxdo"}, repo.unboundProviders)
	require.Equal(t, int64(12), user.ID)
	require.Equal(t, []int64{12}, invalidator.invalidatedUserIDs)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 12, user)
	require.NoError(t, err)
	require.False(t, summaries.LinuxDo.Bound)
	require.True(t, summaries.LinuxDo.CanBind)
}

func TestGetProfileIdentitySummaries_HidesBindActionWhenProviderExplicitlyDisabled(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:    15,
			Email: "alice@example.com",
		},
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "alice@example.com",
			},
		},
	}
	settingRepo := &mockUserSettingRepo{
		values: map[string]string{
			SettingKeyLinuxDoConnectEnabled: "false",
		},
	}
	svc := NewUserService(repo, settingRepo, nil, nil)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 15, repo.getByIDUser)

	require.NoError(t, err)
	require.False(t, summaries.LinuxDo.Bound)
	require.False(t, summaries.LinuxDo.CanBind)
	require.Empty(t, summaries.LinuxDo.BindStartPath)
}

func TestGetProfileIdentitySummaries_UsesBindStartRoute(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:    16,
			Email: "alice@example.com",
		},
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "alice@example.com",
			},
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 16, repo.getByIDUser)

	require.NoError(t, err)
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
}

func TestUpdateBalance_NilBillingCache_NoPanic(t *testing.T) {
	repo := &mockUserRepo{}
	svc := NewUserService(repo, nil, nil, nil) // billingCache = nil

	err := svc.UpdateBalance(context.Background(), 1, 50.0)
	require.NoError(t, err, "billingCache 为 nil 时不应 panic")
}

func TestUpdateBalance_CacheFailure_DoesNotAffectReturn(t *testing.T) {
	repo := &mockUserRepo{}
	cache := &mockBillingCache{invalidateErr: errors.New("redis connection refused")}
	svc := NewUserService(repo, nil, nil, cache)

	err := svc.UpdateBalance(context.Background(), 99, 200.0)
	require.NoError(t, err, "缓存失效失败不应影响主流程返回值")

	// 等待异步 goroutine 完成（即使失败也应调用）
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "即使失败也应调用 InvalidateUserBalance")
}

func TestTouchLastActive_UpdatesWhenStale(t *testing.T) {
	stale := time.Now().Add(-11 * time.Minute)
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           42,
			LastActiveAt: &stale,
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	svc.TouchLastActive(context.Background(), 42)

	require.Equal(t, []int64{42}, repo.updateLastActiveUserIDs)
	require.Len(t, repo.updateLastActiveAt, 1)
	require.WithinDuration(t, time.Now(), repo.updateLastActiveAt[0], 2*time.Second)
}

func TestTouchLastActive_SkipsWhenRecent(t *testing.T) {
	recent := time.Now().Add(-time.Minute)
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           42,
			LastActiveAt: &recent,
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	svc.TouchLastActive(context.Background(), 42)

	require.Empty(t, repo.updateLastActiveUserIDs)
	require.Empty(t, repo.updateLastActiveAt)
}

func TestUpdateBalance_RepoError_ReturnsError(t *testing.T) {
	repo := &mockUserRepo{updateBalanceErr: errors.New("database error")}
	cache := &mockBillingCache{}
	svc := NewUserService(repo, nil, nil, cache)

	err := svc.UpdateBalance(context.Background(), 1, 100.0)
	require.Error(t, err, "repo 失败时应返回错误")
	require.Contains(t, err.Error(), "update balance")

	// repo 失败时不应触发缓存失效
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, int64(0), cache.invalidateCallCount.Load(),
		"repo 失败时不应调用 InvalidateUserBalance")
}

func TestUpdateBalance_WithAuthCacheInvalidator(t *testing.T) {
	repo := &mockUserRepo{}
	auth := &mockAuthCacheInvalidator{}
	cache := &mockBillingCache{}
	svc := NewUserService(repo, nil, auth, cache)

	err := svc.UpdateBalance(context.Background(), 77, 300.0)
	require.NoError(t, err)

	// 验证 auth cache 同步失效
	auth.mu.Lock()
	require.Equal(t, []int64{77}, auth.invalidatedUserIDs)
	auth.mu.Unlock()

	// 验证 billing cache 异步失效
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestNewUserService_FieldsAssignment(t *testing.T) {
	repo := &mockUserRepo{}
	auth := &mockAuthCacheInvalidator{}
	cache := &mockBillingCache{}

	svc := NewUserService(repo, nil, auth, cache)
	require.NotNil(t, svc)
	require.Equal(t, repo, svc.userRepo)
	require.Equal(t, auth, svc.authCacheInvalidator)
	require.Equal(t, cache, svc.billingCache)
}

func TestUpdateProfile_StoresInlineAvatarWithinLimit(t *testing.T) {
	raw := []byte("small-avatar")
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	expectedSum := sha256.Sum256(raw)
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       7,
			Email:    "avatar@example.com",
			Username: "avatar-user",
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	updated, err := svc.UpdateProfile(context.Background(), 7, UpdateProfileRequest{
		AvatarURL: &dataURL,
	})
	require.NoError(t, err)
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
}

func TestUpdateProfile_CompressesInlineAvatarToTwentyKilobytes(t *testing.T) {
	var encoded bytes.Buffer
	for _, size := range []int{192, 224, 256, 288} {
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
			}
		}
		require.NoError(t, png.Encode(&encoded, &img))
		if encoded.Len() > 20*1024 && encoded.Len() <= maxInlineAvatarBytes {
			break
		}
	}
	require.Greater(t, encoded.Len(), 20*1024)
	require.LessOrEqual(t, encoded.Len(), maxInlineAvatarBytes)

	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       17,
			Email:    "avatar-compress@example.com",
			Username: "avatar-compress",
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	updated, err := svc.UpdateProfile(context.Background(), 17, UpdateProfileRequest{
		AvatarURL: &dataURL,
	})
	require.NoError(t, err)
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
}

func TestUpdateProfile_RejectsInlineAvatarOverLimit(t *testing.T) {
	raw := make([]byte, maxInlineAvatarBytes+1)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       8,
			Email:    "large-avatar@example.com",
			Username: "too-large",
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	_, err := svc.UpdateProfile(context.Background(), 8, UpdateProfileRequest{
		AvatarURL: &dataURL,
	})
	require.ErrorIs(t, err, ErrAvatarTooLarge)
	require.Empty(t, repo.upsertAvatarArgs)
	require.Empty(t, repo.deleteAvatarIDs)
	require.Zero(t, repo.updateCalls)
}

func TestUpdateProfile_StoresRemoteAvatarURL(t *testing.T) {
	remoteURL := "https://cdn.example.com/avatar.png"
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       9,
			Email:    "remote-avatar@example.com",
			Username: "remote-avatar",
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	updated, err := svc.UpdateProfile(context.Background(), 9, UpdateProfileRequest{
		AvatarURL: &remoteURL,
	})
	require.NoError(t, err)
	require.Len(t, repo.upsertAvatarArgs, 1)
	require.Equal(t, "remote_url", repo.upsertAvatarArgs[0].StorageProvider)
	require.Equal(t, remoteURL, repo.upsertAvatarArgs[0].URL)
	require.Equal(t, remoteURL, updated.AvatarURL)
	require.Equal(t, "remote_url", updated.AvatarSource)
	require.Zero(t, updated.AvatarByteSize)
}

func TestUpdateProfile_DeletesAvatarOnEmptyString(t *testing.T) {
	empty := ""
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           10,
			Email:        "delete-avatar@example.com",
			Username:     "delete-avatar",
			AvatarURL:    "https://cdn.example.com/old.png",
			AvatarSource: "remote_url",
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	updated, err := svc.UpdateProfile(context.Background(), 10, UpdateProfileRequest{
		AvatarURL: &empty,
	})
	require.NoError(t, err)
	require.Equal(t, []int64{10}, repo.deleteAvatarIDs)
	require.Empty(t, repo.upsertAvatarArgs)
	require.Empty(t, updated.AvatarURL)
	require.Empty(t, updated.AvatarSource)
}

func TestUpdateProfile_RollsBackAvatarMutationWhenUserUpdateFails(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           11,
			Email:        "rollback@example.com",
			AvatarURL:    "https://cdn.example.com/original.png",
			AvatarSource: "remote_url",
		},
		updateFn: func(context.Context, *User) error {
			return errors.New("write user failed")
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	remoteURL := "https://cdn.example.com/new.png"
	_, err := svc.UpdateProfile(context.Background(), 11, UpdateProfileRequest{
		AvatarURL: &remoteURL,
	})

	require.EqualError(t, err, "update user: write user failed")
	require.Equal(t, 1, repo.txCalls)
	require.Empty(t, repo.upsertAvatarArgs)
	require.Empty(t, repo.deleteAvatarIDs)
	require.Equal(t, "https://cdn.example.com/original.png", repo.getByIDUser.AvatarURL)
	require.Equal(t, "remote_url", repo.getByIDUser.AvatarSource)
}

func TestGetProfile_HydratesAvatarFromRepository(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       12,
			Email:    "profile-avatar@example.com",
			Username: "profile-avatar",
		},
		getAvatarFn: func(context.Context, int64) (*UserAvatar, error) {
			return &UserAvatar{
				StorageProvider: "remote_url",
				URL:             "https://cdn.example.com/profile.png",
			}, nil
		},
	}
	svc := NewUserService(repo, nil, nil, nil)

	user, err := svc.GetProfile(context.Background(), 12)
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example.com/profile.png", user.AvatarURL)
	require.Equal(t, "remote_url", user.AvatarSource)
}
