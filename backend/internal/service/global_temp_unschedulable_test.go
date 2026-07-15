//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGlobalTempUnschedulableSettingsDefaultEnabled(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})

	settings, err := svc.GetGlobalTempUnschedulableSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.True(t, svc.IsGlobalTempUnschedulableEnabled(context.Background()))
}

func TestGlobalTempUnschedulableSettingsPersistAndPublish(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	require.NoError(t, svc.SetGlobalTempUnschedulableSettings(context.Background(), &GlobalTempUnschedulableSettings{Enabled: false}))
	require.False(t, svc.IsGlobalTempUnschedulableEnabled(context.Background()))
	require.Equal(t, "false", repo.data[SettingKeyGlobalTempUnschedulableEnabled])

	reloaded := NewSettingService(repo, &config.Config{})
	require.NoError(t, reloaded.LoadGlobalTempUnschedulableSetting(context.Background()))
	require.False(t, reloaded.IsGlobalTempUnschedulableEnabled(context.Background()))
}

func TestGlobalTempUnschedulableRuntimeCacheRefreshesAcrossInstances(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})
	require.NoError(t, svc.LoadGlobalTempUnschedulableSetting(context.Background()))
	require.True(t, svc.IsGlobalTempUnschedulableEnabled(context.Background()))

	repo.data[SettingKeyGlobalTempUnschedulableEnabled] = "false"
	svc.globalTempUnschedulableLoaded.Store(time.Now().Add(-2 * globalTempUnschedulableRefreshInterval).UnixNano())
	require.False(t, svc.IsGlobalTempUnschedulableEnabled(context.Background()))
}

func TestGlobalTempUnschedulableDisabledClearsOnlyTaggedRuntimeBlock(t *testing.T) {
	repo := newMockSettingRepo()
	repo.data[SettingKeyGlobalTempUnschedulableEnabled] = "false"
	settingSvc := NewSettingService(repo, &config.Config{})
	require.NoError(t, settingSvc.LoadGlobalTempUnschedulableSetting(context.Background()))

	svc := &OpenAIGatewayService{settingService: settingSvc}
	tempAccount := &Account{ID: 501, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc.BlockAccountScheduling(tempAccount, time.Now().Add(time.Minute), "transport_error")
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(tempAccount))

	rateLimitedAccount := &Account{ID: 502, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc.BlockAccountScheduling(rateLimitedAccount, time.Now().Add(time.Minute), "429")
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(rateLimitedAccount))
}

func TestGlobalTempUnschedulableDisabledSkipsGrokCredentialTransientMutation(t *testing.T) {
	settingRepo := newMockSettingRepo()
	settingRepo.data[SettingKeyGlobalTempUnschedulableEnabled] = "false"
	settingSvc := NewSettingService(settingRepo, &config.Config{})
	require.NoError(t, settingSvc.LoadGlobalTempUnschedulableSetting(context.Background()))

	account := expiredGrokOAuthAccountForCredentialTest(503)
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	svc := &OpenAIGatewayService{accountRepo: repo, settingService: settingSvc}

	token, err := svc.applyGrokCredentialAccountFailure(context.Background(), account, grokCredentialFailureClass{
		scope:     GatewayFailureScopeAccount,
		reason:    GrokCredentialReasonRefreshTransient,
		action:    NextAccountRetry,
		transient: true,
	})

	require.NoError(t, err)
	require.Empty(t, token)
	require.Zero(t, repo.setTempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestRateLimitServiceGlobalTempUnschedulableDisabledSkipsRule(t *testing.T) {
	accountRepo := &geminiErrorPolicyRepo{}
	settingRepo := newMockSettingRepo()
	settingRepo.data[SettingKeyGlobalTempUnschedulableEnabled] = "false"
	settingSvc := NewSettingService(settingRepo, &config.Config{})
	require.NoError(t, settingSvc.LoadGlobalTempUnschedulableSetting(context.Background()))

	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)
	account := &Account{
		ID:       91,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{"error_code": 503, "duration_minutes": 5},
			},
		},
	}

	result := svc.CheckErrorPolicy(context.Background(), account, 503, []byte(`{"error":"temporary"}`))
	require.Equal(t, ErrorPolicyNone, result)
	require.Zero(t, accountRepo.setTempCalls)
}

type globalTempCleanerRepo struct {
	ids []int64
}

func (r *globalTempCleanerRepo) ClearAllTempUnschedulable(context.Context) ([]int64, error) {
	return r.ids, nil
}

type globalTempCleanerCache struct {
	cleared bool
}

func (c *globalTempCleanerCache) SetTempUnsched(context.Context, int64, *TempUnschedState) error {
	return nil
}

func (c *globalTempCleanerCache) GetTempUnsched(context.Context, int64) (*TempUnschedState, error) {
	return nil, nil
}

func (c *globalTempCleanerCache) DeleteTempUnsched(context.Context, int64) error {
	return nil
}

func (c *globalTempCleanerCache) DeleteAllTempUnsched(context.Context) error {
	c.cleared = true
	return nil
}

type globalTempCleanerBlocker struct {
	cleared []int64
}

func (b *globalTempCleanerBlocker) BlockAccountScheduling(*Account, time.Time, string) {}

func (b *globalTempCleanerBlocker) ClearAccountSchedulingBlock(accountID int64) {
	b.cleared = append(b.cleared, accountID)
}

func TestGlobalTempUnschedulableCleanerClearsEveryStateLayer(t *testing.T) {
	repo := &globalTempCleanerRepo{ids: []int64{11, 29}}
	cache := &globalTempCleanerCache{}
	blocker := &globalTempCleanerBlocker{}
	cleaner := NewGlobalTempUnschedulableCleaner(repo, cache, blocker)

	count, err := cleaner.Clear(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.True(t, cache.cleared)
	require.Equal(t, []int64{11, 29}, blocker.cleared)
}
