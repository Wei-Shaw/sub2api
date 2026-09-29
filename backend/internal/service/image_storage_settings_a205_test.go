//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// a205FlakySettingRepo fails GetValue for the given key a fixed number of times
// (a transient DB error), then delegates to the embedded stub.
type a205FlakySettingRepo struct {
	*stubSettingRepo
	mu       sync.Mutex
	failKey  string
	failures int
	err      error
	calls    int
}

func (r *a205FlakySettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	r.mu.Lock()
	if key == r.failKey {
		r.calls++
		if r.failures > 0 {
			r.failures--
			r.mu.Unlock()
			return "", r.err
		}
	}
	r.mu.Unlock()
	return r.stubSettingRepo.GetValue(ctx, key)
}

func a205SeedOwnCredentials(t *testing.T, repo *stubSettingRepo) {
	t.Helper()
	data, err := json.Marshal(ImageStorageSettings{
		Enabled: true, Bucket: "b", Endpoint: "https://e", Region: "auto",
		AccessKeyID: "ak", SecretAccessKey: "enc:sk", Prefix: "images/", MaxDownloadBytes: 1024,
	})
	require.NoError(t, err)
	require.NoError(t, repo.Set(context.Background(), settingKeyImageStorageConfig, string(data)))
}

// a205ElapseRetryBackoff simulates the retry interval having passed.
func a205ElapseRetryBackoff(svc *ImageStorageSettingService) {
	svc.mu.Lock()
	svc.retryAt = time.Now().Add(-time.Second)
	svc.mu.Unlock()
}

// A transient DB error on the first resolve must not disable async image tasks
// until a restart or an admin re-save (A2-05).
func TestImageStorageSettingsTransientLoadErrorIsRetried_A205(t *testing.T) {
	svc, repo, built := newImageStorageFixture(t, config.ImageStorageConfig{})
	a205SeedOwnCredentials(t, repo)
	flaky := &a205FlakySettingRepo{
		stubSettingRepo: repo, failKey: settingKeyImageStorageConfig,
		failures: 1, err: errors.New("db down"),
	}
	svc.settingRepo = flaky

	uploader, enabled := svc.resolve()
	require.False(t, enabled, "the failing resolve cannot enable the feature")
	require.Nil(t, uploader)

	// 退避期内不重复查库。
	_, enabled = svc.resolve()
	require.False(t, enabled)
	require.Equal(t, 1, flaky.calls, "a failed resolve is retried only after the backoff")

	a205ElapseRetryBackoff(svc)
	uploader, enabled = svc.resolve()
	require.True(t, enabled, "a transient load error must not be cached permanently")
	require.NotNil(t, uploader)
	require.Len(t, *built, 1)

	// 成功后正常缓存。
	_, enabled = svc.resolve()
	require.True(t, enabled)
	require.Equal(t, 2, flaky.calls)
}

// A DB error must not be mistaken for "never configured" and fall back to
// config.yaml (which is disabled for admin-UI deployments).
func TestImageStorageSettingsLoadErrorIsNotTreatedAsUnconfigured_A205(t *testing.T) {
	svc, repo, _ := newImageStorageFixture(t, config.ImageStorageConfig{Bucket: "yaml-bucket"})
	a205SeedOwnCredentials(t, repo)
	svc.settingRepo = &a205FlakySettingRepo{
		stubSettingRepo: repo, failKey: settingKeyImageStorageConfig,
		failures: 1, err: errors.New("db down"),
	}

	_, err := svc.Get(context.Background())
	require.Error(t, err, "Get must surface the DB error instead of returning config.yaml values")

	got, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, "b", got.Bucket)
}

// A genuinely missing setting still means "never configured": use config.yaml.
func TestImageStorageSettingsNotFoundFallsBackToConfig_A205(t *testing.T) {
	svc, repo, built := newImageStorageFixture(t, config.ImageStorageConfig{
		Enabled: true, Bucket: "yaml-bucket", Endpoint: "https://e", Region: "auto",
		AccessKeyID: "ak", SecretAccessKey: "sk", Prefix: "images/",
	})
	svc.settingRepo = &a205FlakySettingRepo{
		stubSettingRepo: repo, failKey: settingKeyImageStorageConfig,
		failures: 100, err: ErrSettingNotFound,
	}

	_, enabled := svc.resolve()
	require.True(t, enabled, "setting-not-found falls back to config.yaml")
	require.Len(t, *built, 1)
	require.Equal(t, "yaml-bucket", (*built)[0].Bucket)

	got, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, "yaml-bucket", got.Bucket)
}

// The reuse-backup path must also recover once the backup S3 config is readable.
func TestImageStorageSettingsReuseBackupTransientErrorIsRetried_A205(t *testing.T) {
	svc, repo, built := newImageStorageFixture(t, config.ImageStorageConfig{})
	ctx := context.Background()
	seedBackupS3(t, repo, BackupS3Config{
		Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "auto",
		Bucket: "backup-bucket", AccessKeyID: "ak", SecretAccessKey: "sk",
	})
	_, err := svc.Update(ctx, ImageStorageSettings{Enabled: true, ReuseBackupS3: true})
	require.NoError(t, err)

	// BackupService 共用同一个 repo，把失败注入到备份配置的读取上。
	flaky := &a205FlakySettingRepo{
		stubSettingRepo: repo, failKey: settingKeyBackupS3Config,
		failures: 1, err: errors.New("db down"),
	}
	svc.backup.settingRepo = flaky

	_, enabled := svc.resolve()
	require.False(t, enabled)

	a205ElapseRetryBackoff(svc)
	_, enabled = svc.resolve()
	require.True(t, enabled, "the backup S3 lookup failure must not be cached permanently")
	require.Len(t, *built, 1)
}
