package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newRecoveryTestService(repo SettingRepository, github GitHubReleaseClient) (*OpenAICodexVersionSyncService, *time.Time) {
	svc := newCodexVersionSyncService(repo, github)
	now := time.Date(2026, 10, 1, 14, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	svc.retryJitter = func() time.Duration { return 0 }
	return svc, &now
}

func readRecoveryState(t *testing.T, repo SettingRepository) OpenAICodexVersionSyncState {
	t.Helper()
	value, err := repo.GetValue(context.Background(), SettingKeyOpenAICodexVersionSyncState)
	require.NoError(t, err)
	state, err := decodeCodexVersionSyncState(value)
	require.NoError(t, err)
	return state
}

func TestCodexSyncPrimaryLimitWaitsWithoutFallback(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{
		SettingKeyOpenAICodexClientVersionSynced: "0.157.0",
		SettingKeyOpenAICodexClientVersion:       "0.159.0",
	})
	github := &codexVersionSyncGitHubStub{}
	svc, now := newRecoveryTestService(repo, github)
	reset := now.Add(15 * time.Minute)
	github.latestErr = &GitHubAPIError{StatusCode: 403, Code: "github_primary_rate_limit", RateLimitResetAt: &reset, RetryAt: &reset}
	svc.runOnce()
	state := readRecoveryState(t, repo)
	require.Equal(t, "failed", state.Status)
	require.Equal(t, reset.Add(5*time.Second), *state.NextCheckAt)
	require.Equal(t, reset, *state.RateLimitResetAt)
	require.Nil(t, state.LastSucceededAt)
	require.Equal(t, 1, state.RetryCount)
	require.Zero(t, github.calls, "限流后不请求共享同一额度的列表接口")
	require.Empty(t, repo.syncedWrites())
	require.Equal(t, "0.159.0", repo.values[SettingKeyOpenAICodexClientVersion])
	svc.runOnce()
	require.Equal(t, 1, github.latestCalls, "冷却结束前不能发起请求")

	// 重启保留冷却和已用重试次数，不能重新开始立即请求。
	restarted, restartedNow := newRecoveryTestService(repo, github)
	*restartedNow = now.Add(time.Minute)
	restarted.runInitial()
	require.Equal(t, 1, github.latestCalls)
	require.Equal(t, state.RetryCount, restarted.state.RetryCount)
	require.Equal(t, *state.NextCheckAt, *restarted.state.NextCheckAt)
}

func TestCodexSyncRecoveryBudgetIsBounded(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(nil)
	github := &codexVersionSyncGitHubStub{latestErr: &GitHubAPIError{StatusCode: 429, Code: "github_secondary_rate_limit"}}
	svc, now := newRecoveryTestService(repo, github)
	for attempt := 0; attempt <= openAICodexVersionMaxRetries; attempt++ {
		svc.runOnce()
		state := readRecoveryState(t, repo)
		if attempt < openAICodexVersionMaxRetries {
			require.False(t, state.RetryExhausted)
			require.Equal(t, time.Minute*time.Duration(1<<attempt)+5*time.Second, state.NextCheckAt.Sub(*now))
		} else {
			require.True(t, state.RetryExhausted)
			require.Equal(t, svc.interval, state.NextCheckAt.Sub(*now))
		}
		*now = *state.NextCheckAt
	}
	require.Equal(t, 4, github.latestCalls)
	require.Zero(t, github.calls)
	svc.runOnce()
	require.Equal(t, 1, readRecoveryState(t, repo).RetryCount, "常规周期开启新的有界恢复预算")
}

func TestCodexSyncExhaustedBudgetStillHonorsLongCooldown(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(nil)
	github := &codexVersionSyncGitHubStub{}
	svc, now := newRecoveryTestService(repo, github)
	svc.state.RetryCount = openAICodexVersionMaxRetries
	reset := now.Add(10 * time.Hour)
	github.latestErr = &GitHubAPIError{StatusCode: 403, Code: "github_primary_rate_limit", RateLimitResetAt: &reset, RetryAt: &reset}
	svc.runOnce()
	state := readRecoveryState(t, repo)
	require.True(t, state.RetryExhausted)
	require.Greater(t, state.NextCheckAt.Sub(*now), svc.interval)
	require.True(t, state.NextCheckAt.After(reset))
}

func TestCodexSyncForbiddenIsNotRetriedAsRateLimit(t *testing.T) {
	for _, code := range []string{"github_unauthorized", "github_forbidden"} {
		t.Run(code, func(t *testing.T) {
			repo := newCodexVersionSyncSettingRepoStub(nil)
			github := &codexVersionSyncGitHubStub{latestErr: &GitHubAPIError{StatusCode: 403, Code: code}}
			svc, now := newRecoveryTestService(repo, github)
			svc.runOnce()
			state := readRecoveryState(t, repo)
			require.Equal(t, code, state.ErrorCode)
			require.Nil(t, state.RateLimitResetAt)
			require.Zero(t, state.RetryCount)
			require.Equal(t, now.Add(svc.interval), *state.NextCheckAt)
			require.Zero(t, github.calls)
		})
	}
}

func TestCodexSyncUnchangedVersionRecordsSuccessfulCheck(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{SettingKeyOpenAICodexClientVersionSynced: "0.159.0"})
	github := &codexVersionSyncGitHubStub{latest: &GitHubRelease{TagName: "rust-v0.159.0"}}
	svc, now := newRecoveryTestService(repo, github)
	repo.updatedAt = now.Add(-7 * 24 * time.Hour)
	svc.state.RetryCount = 2
	svc.state.ErrorCode = "github_primary_rate_limit"
	svc.runOnce()
	status, err := (&SettingService{settingRepo: repo}).GetOpenAICodexVersionSyncStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, *now, *status.LastCheckedAt)
	require.Equal(t, *now, *status.LastSucceededAt)
	require.Equal(t, repo.updatedAt, *status.LastVersionUpdatedAt)
	require.Equal(t, "success", status.Status)
	require.Empty(t, status.ErrorCode)
	require.Zero(t, status.RetryCount)
	require.Empty(t, repo.syncedWrites(), "版本不变时不能刷新版本行的更新时间")

	// 版本行已经很旧，但刚刚成功检查过；重启不应再次请求。
	restarted, restartedNow := newRecoveryTestService(repo, github)
	*restartedNow = now.Add(time.Minute)
	restarted.runInitial()
	require.Equal(t, 1, github.latestCalls)
}

func TestCodexSyncFailureRetainsLastSuccessAndRedactsError(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(nil)
	github := &codexVersionSyncGitHubStub{latest: &GitHubRelease{TagName: "rust-v0.159.0"}}
	svc, now := newRecoveryTestService(repo, github)
	svc.runOnce()
	succeeded := *now
	*now = *svc.state.NextCheckAt
	github.latestErr = errors.New("proxy https://private-secret@example.invalid")
	github.err = github.latestErr
	svc.runOnce()
	state := readRecoveryState(t, repo)
	require.Equal(t, succeeded, *state.LastSucceededAt)
	require.Equal(t, "network_error", state.ErrorCode)
	require.NotContains(t, repo.values[SettingKeyOpenAICodexVersionSyncState], "private-secret")
	require.Equal(t, "0.159.0", repo.values[SettingKeyOpenAICodexClientVersionSynced])
}

func TestCodexSyncPersistenceFailureDoesNotHideNewFailure(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(nil)
	github := &codexVersionSyncGitHubStub{latest: &GitHubRelease{TagName: "rust-v0.159.0"}}
	svc, now := newRecoveryTestService(repo, github)
	svc.settingService = &SettingService{settingRepo: repo}
	svc.runOnce()
	*now = *svc.state.NextCheckAt
	repo.setErr = errors.New("database read-only")
	github.latestErr = &GitHubAPIError{StatusCode: 403, Code: "github_primary_rate_limit"}
	svc.runOnce()
	require.Equal(t, "success", readRecoveryState(t, repo).Status, "模拟 DB 仍只保留旧成功结果")
	status, err := svc.settingService.GetOpenAICodexVersionSyncStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, "failed", status.Status)
	require.True(t, status.PersistenceFailed)
	require.Equal(t, "github_primary_rate_limit", status.ErrorCode)
}

func TestCodexSyncCorruptStateDoesNotCauseRetryBurst(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{
		SettingKeyOpenAICodexVersionSyncState: `{"status":"failed","retry_count":-1}`,
	})
	github := &codexVersionSyncGitHubStub{latestErr: &GitHubAPIError{StatusCode: 429, Code: "github_rate_limit"}}
	svc, now := newRecoveryTestService(repo, github)
	svc.runInitial()
	require.Zero(t, github.latestCalls)
	require.Equal(t, svc.interval, svc.nextDelay())
	*now = *svc.state.NextCheckAt
	require.NotPanics(t, svc.runOnce)
	require.Equal(t, 1, github.latestCalls)
}

func TestCodexSyncSettingsReadErrorDoesNotOverwriteVersion(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{SettingKeyOpenAICodexClientVersionSynced: "0.160.0"})
	repo.getErr = errors.New("database unavailable")
	github := &codexVersionSyncGitHubStub{latest: &GitHubRelease{TagName: "rust-v0.159.0"}}
	svc, _ := newRecoveryTestService(repo, github)
	svc.runOnce()
	require.Zero(t, github.latestCalls)
	require.Empty(t, repo.syncedWrites())
	require.Equal(t, "0.160.0", repo.values[SettingKeyOpenAICodexClientVersionSynced])
	require.Equal(t, "settings_read_failed", svc.state.ErrorCode)
}

func TestCodexSyncStatusSourceAndDisabledProjection(t *testing.T) {
	for _, tt := range []struct{ manual, synced, want, source string }{
		{"0.159.0", "0.160.0", "0.159.0", "manual"},
		{"", "0.160.0", "0.160.0", "synced"},
		{"invalid", "invalid", codexCLIVersion, "builtin"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			next := time.Now().Add(time.Hour)
			state, err := json.Marshal(OpenAICodexVersionSyncState{Status: "failed", NextCheckAt: &next})
			require.NoError(t, err)
			repo := newCodexVersionSyncSettingRepoStub(map[string]string{
				SettingKeyOpenAICodexClientVersion: tt.manual, SettingKeyOpenAICodexClientVersionSynced: tt.synced,
				SettingKeyOpenAICodexVersionAutoSyncEnabled: "false", SettingKeyOpenAICodexVersionSyncState: string(state),
			})
			svc := &SettingService{settingRepo: repo}
			status, err := svc.GetOpenAICodexVersionSyncStatus(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.want, status.EffectiveVersion)
			require.Equal(t, svc.GetOpenAICodexClientVersion(context.Background()), status.EffectiveVersion)
			require.Equal(t, tt.source, status.VersionSource)
			require.False(t, status.AutoSyncEnabled)
			require.Nil(t, status.NextCheckAt)
		})
	}
}

type blockingCodexGitHub struct {
	GitHubReleaseClient
	started chan struct{}
}

func (c *blockingCodexGitHub) FetchLatestRelease(ctx context.Context, _ string) (*GitHubRelease, error) {
	close(c.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestCodexSyncStopCancelsInFlightRequest(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(nil)
	github := &blockingCodexGitHub{started: make(chan struct{})}
	svc := newCodexVersionSyncService(repo, github)
	svc.Start()
	svc.Start()
	select {
	case <-github.started:
	case <-time.After(time.Second):
		t.Fatal("同步任务未启动")
	}
	done := make(chan struct{})
	go func() { svc.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("停止时没有取消在途请求")
	}
	require.Empty(t, repo.syncedWrites())
}
