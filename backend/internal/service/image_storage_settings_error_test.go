//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type probingImageStorage struct {
	probeCalls int
	probeErr   error
}

func (s *probingImageStorage) Save(context.Context, string, string, []byte) (string, error) {
	return "https://cdn.example.com/image.png", nil
}

func (s *probingImageStorage) TestConnection(context.Context) error {
	s.probeCalls++
	return s.probeErr
}

func TestImageStorageSettingsReadFailureDoesNotUseOrCacheFallback(t *testing.T) {
	fallback := config.ImageStorageConfig{
		Enabled: true, Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "auto",
		Bucket: "yaml-bucket", AccessKeyID: "yaml-ak", SecretAccessKey: "yaml-sk",
	}
	svc, repo, built := newImageStorageFixture(t, fallback)
	repo.getValueErr = errors.New("database unavailable")

	uploader, enabled := svc.resolve()
	require.False(t, enabled)
	require.Nil(t, uploader)
	require.Empty(t, *built, "a read failure must not activate config-file fallback")

	repo.getValueErr = nil
	uploader, enabled = svc.resolve()
	require.True(t, enabled, "a transient read failure must be retried")
	require.NotNil(t, uploader)
}

func TestImageStorageSettingsMissingValueStillUsesFallback(t *testing.T) {
	fallback := config.ImageStorageConfig{
		Enabled: true, Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "auto",
		Bucket: "yaml-bucket", AccessKeyID: "yaml-ak", SecretAccessKey: "yaml-sk",
	}
	svc, repo, _ := newImageStorageFixture(t, fallback)
	repo.getValueErr = ErrSettingNotFound

	_, enabled := svc.resolve()
	require.True(t, enabled)
}

func TestImageStorageSettingsUpdatePreservesSecretOnReadFailure(t *testing.T) {
	svc, repo, _ := newImageStorageFixture(t, config.ImageStorageConfig{})
	repo.values[settingKeyImageStorageConfig] = `{"enabled":true,"bucket":"images","endpoint":"https://s3.example.com","access_key_id":"ak","secret_access_key":"enc:old-secret"}`
	repo.getValueErr = errors.New("database unavailable")
	setCallsBefore := repo.setCalls

	_, err := svc.Update(context.Background(), ImageStorageSettings{
		Enabled: true, Bucket: "images", Endpoint: "https://s3.example.com", AccessKeyID: "ak",
	})

	require.ErrorContains(t, err, "load existing image storage settings")
	require.Equal(t, setCallsBefore, repo.setCalls, "failed reads must not be followed by a destructive write")
}

func TestImageStorageSettingsResolverRetriesFactoryFailure(t *testing.T) {
	repo := newStubSettingRepo()
	factoryCalls := 0
	factoryErr := errors.New("temporary client build failure")
	factory := func(context.Context, *config.ImageStorageConfig) (ImageStorage, error) {
		factoryCalls++
		if factoryCalls == 1 {
			return nil, factoryErr
		}
		return &recordingStorage{}, nil
	}
	fallback := config.ImageStorageConfig{
		Enabled: true, Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "auto",
		Bucket: "yaml-bucket", AccessKeyID: "yaml-ak", SecretAccessKey: "yaml-sk",
	}
	svc := NewImageStorageSettingService(repo, reversibleEncryptor{}, nil, factory, fallback)
	svc.SetDownloadClient(&http.Client{})

	_, enabled := svc.resolve()
	require.False(t, enabled)
	_, enabled = svc.resolve()
	require.True(t, enabled)
	require.Equal(t, 2, factoryCalls)
}

func TestImageStorageSettingsTestConnectionProbesStorage(t *testing.T) {
	repo := newStubSettingRepo()
	probeErr := errors.New("invalid credentials")
	storage := &probingImageStorage{probeErr: probeErr}
	factory := func(context.Context, *config.ImageStorageConfig) (ImageStorage, error) {
		return storage, nil
	}
	svc := NewImageStorageSettingService(repo, reversibleEncryptor{}, nil, factory, config.ImageStorageConfig{})
	input := ImageStorageSettings{
		Enabled: true, Bucket: "images", Endpoint: "https://s3.example.com",
		AccessKeyID: "ak", SecretAccessKey: "secret",
	}

	err := svc.TestConnection(context.Background(), input)
	require.ErrorIs(t, err, probeErr)
	require.Equal(t, 1, storage.probeCalls)

	storage.probeErr = nil
	require.NoError(t, svc.TestConnection(context.Background(), input))
	require.Equal(t, 2, storage.probeCalls)
}
