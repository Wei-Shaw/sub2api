//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type privateExpiresRepoStub struct {
	updates map[string]string
	values  map[string]string
}

func (r *privateExpiresRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}
func (r *privateExpiresRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if r.values == nil {
		return "", ErrSettingNotFound
	}
	v, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return v, nil
}
func (r *privateExpiresRepoStub) Set(context.Context, string, string) error { return nil }
func (r *privateExpiresRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *privateExpiresRepoStub) SetMultiple(_ context.Context, updates map[string]string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.updates = make(map[string]string, len(updates))
	for k, v := range updates {
		r.updates[k] = v
		r.values[k] = v
	}
	return nil
}
func (r *privateExpiresRepoStub) GetAll(context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for k, v := range r.values {
		out[k] = v
	}
	return out, nil
}
func (r *privateExpiresRepoStub) Delete(context.Context, string) error { return nil }

func TestNormalizePrivateGroupExpiresDate(t *testing.T) {
	got, err := normalizePrivateGroupExpiresDate("")
	require.NoError(t, err)
	require.Equal(t, "", got)

	got, err = normalizePrivateGroupExpiresDate(" 2026-12-31 ")
	require.NoError(t, err)
	require.Equal(t, "2026-12-31", got)

	_, err = normalizePrivateGroupExpiresDate("31-12-2026")
	require.Error(t, err)

	_, err = normalizePrivateGroupExpiresDate("not-a-date")
	require.Error(t, err)
}

func TestPrivateGroupExpiresDate_WriteEmptyClears(t *testing.T) {
	repo := &privateExpiresRepoStub{values: map[string]string{
		SettingKeyPrivateGroupExpiresDate: "2026-08-01",
	}}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		PrivateGroupExpiresDate: "", // 显式清空
	})
	require.NoError(t, err)

	val, ok := repo.updates[SettingKeyPrivateGroupExpiresDate]
	require.True(t, ok, "empty string must be written to clear the setting")
	require.Equal(t, "", val)
}

func TestPrivateGroupExpiresDate_OmittedPreservesStored(t *testing.T) {
	repo := &privateExpiresRepoStub{values: map[string]string{
		SettingKeyPrivateGroupExpiresDate: "2026-08-01",
	}}
	svc := NewSettingService(repo, &config.Config{})

	// 部分更新：omitted 含该 key → 不写入，库值保留
	err := svc.UpdateSettingsOmitting(context.Background(), &SystemSettings{
		DefaultBalance: 1.0,
	}, OmittedSettingKeys{
		SettingKeyPrivateGroupExpiresDate: {},
	})
	require.NoError(t, err)

	_, written := repo.updates[SettingKeyPrivateGroupExpiresDate]
	require.False(t, written, "omitted key must not appear in updates")
	require.Equal(t, "2026-08-01", repo.values[SettingKeyPrivateGroupExpiresDate])
}

func TestPrivateGroupExpiresDate_WriteValidDate(t *testing.T) {
	repo := &privateExpiresRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		PrivateGroupExpiresDate: "2027-01-15",
	})
	require.NoError(t, err)
	require.Equal(t, "2027-01-15", repo.updates[SettingKeyPrivateGroupExpiresDate])
}

func TestGetPrivateGroupExpiresDate_AndResolve(t *testing.T) {
	repo := &privateExpiresRepoStub{values: map[string]string{
		SettingKeyPrivateGroupExpiresDate: "2026-12-31",
	}}
	svc := NewSettingService(repo, &config.Config{})

	date, ok := svc.GetPrivateGroupExpiresDate(context.Background())
	require.True(t, ok)
	require.Equal(t, "2026-12-31", date)

	expiresAt, ok := svc.ResolvePrivateGroupExpiresAt(context.Background())
	require.True(t, ok)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	require.Equal(t, 2026, expiresAt.Year())
	require.Equal(t, time.December, expiresAt.Month())
	require.Equal(t, 31, expiresAt.Day())
	require.Equal(t, 23, expiresAt.Hour())
	require.Equal(t, 59, expiresAt.Minute())
	require.Equal(t, 59, expiresAt.Second())
	require.Equal(t, shanghai.String(), expiresAt.Location().String())

	// 未配置
	repo.values[SettingKeyPrivateGroupExpiresDate] = ""
	_, ok = svc.GetPrivateGroupExpiresDate(context.Background())
	require.False(t, ok)
	_, ok = svc.ResolvePrivateGroupExpiresAt(context.Background())
	require.False(t, ok)
}

func TestPrivateGroupExpiresDate_InvalidOnWrite(t *testing.T) {
	repo := &privateExpiresRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		PrivateGroupExpiresDate: "2026/12/31",
	})
	require.Error(t, err)
}
