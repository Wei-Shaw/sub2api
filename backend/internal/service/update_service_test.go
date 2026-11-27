//go:build unit

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release        *GitHubRelease
	recentReleases []*GitHubRelease
	recentErr      error
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) FetchRecentReleases(context.Context, string, int) ([]*GitHubRelease, error) {
	return s.recentReleases, s.recentErr
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v0.1.132",
				Name:    "v0.1.132",
			},
		},
		"0.1.132",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}

func newRollbackTestService(current string, releases []*GitHubRelease) *UpdateService {
	return NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"release",
	)
}

func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148", PublishedAt: "2026-07-09T00:00:00Z"},                       // newer than current: excluded
		{TagName: "v0.1.147", PublishedAt: "2026-07-08T00:00:00Z"},                       // current: excluded
		{TagName: "v0.1.146-rc1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true}, // prerelease: excluded
		{TagName: "v0.1.146", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "v0.1.145", PublishedAt: "2026-07-06T00:00:00Z", Draft: true}, // draft: excluded
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"}, // duplicate: excluded
		{TagName: "v0.1.143", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "v0.1.142", PublishedAt: "2026-07-03T00:00:00Z"}, // beyond cap of 3: excluded
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.144", versions[1].Version)
	require.Equal(t, "0.1.143", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.144"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.145", versions[1].Version)
	require.Equal(t, "0.1.144", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.148"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Empty(t, versions)
}

func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.147",
		"release",
	)

	_, err := svc.ListRollbackVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148"},
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
		{TagName: "v0.1.144"},
		{TagName: "v0.1.143"},
		{TagName: "v0.1.142"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	for _, target := range []string{
		"",         // empty
		"0.1.147",  // current version
		"v0.1.147", // current version with prefix
		"0.1.148",  // newer than current
		"0.1.142",  // older than the 3 most recent
		"9.9.9",    // nonexistent
	} {
		err := svc.RollbackToVersion(context.Background(), target)
		require.ErrorIs(t, err, ErrRollbackVersionNotAllowed, "target %q should be rejected", target)
	}
}

func TestUpdateServiceRollbackToVersionAcceptsVPrefix(t *testing.T) {
	// No platform asset in the release: the target passes the allowlist check
	// and fails later at asset lookup, proving the version itself was accepted.
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	err := svc.RollbackToVersion(context.Background(), "v0.1.146")

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	require.Contains(t, err.Error(), "no compatible release found")
}

type updateServiceChecksumClientStub struct {
	updateServiceGitHubClientStub
	checksumData []byte
	checksumErr  error
}

func (s *updateServiceChecksumClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	return s.checksumData, s.checksumErr
}

func writeChecksumTestArchive(t *testing.T) (string, string) {
	t.Helper()
	content := []byte("fake archive content")
	path := filepath.Join(t.TempDir(), "sub2api_0.2.9_linux_amd64.tar.gz")
	require.NoError(t, os.WriteFile(path, content, 0o600))
	sum := sha256.Sum256(content)
	return path, hex.EncodeToString(sum[:])
}

func TestUpdateServiceApplyReleaseAssetsRejectsMissingChecksumFile(t *testing.T) {
	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "0.1.0", "release")
	archive := svc.getArchiveName() + ".tar.gz"
	assets := []Asset{{
		Name:        archive,
		DownloadURL: "https://github.com/Wei-Shaw/sub2api/releases/download/v0.2.9/" + archive,
	}}

	err := svc.applyReleaseAssets(context.Background(), assets)

	require.Error(t, err)
	require.Contains(t, err.Error(), "checksums.txt")
}

func TestUpdateServiceVerifyChecksumRejectsUnlistedAsset(t *testing.T) {
	path, _ := writeChecksumTestArchive(t)
	client := &updateServiceChecksumClientStub{checksumData: []byte("deadbeef  other_file.tar.gz\n")}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.0", "release")

	err := svc.verifyChecksum(context.Background(), path, "https://github.com/x/checksums.txt")

	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum not found")
}

func TestUpdateServiceVerifyChecksumRejectsMismatch(t *testing.T) {
	path, _ := writeChecksumTestArchive(t)
	client := &updateServiceChecksumClientStub{checksumData: []byte("deadbeef  " + filepath.Base(path) + "\n")}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.0", "release")

	err := svc.verifyChecksum(context.Background(), path, "https://github.com/x/checksums.txt")

	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum mismatch")
}

func TestUpdateServiceVerifyChecksumRejectsFetchError(t *testing.T) {
	path, _ := writeChecksumTestArchive(t)
	client := &updateServiceChecksumClientStub{checksumErr: errors.New("network down")}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.0", "release")

	err := svc.verifyChecksum(context.Background(), path, "https://github.com/x/checksums.txt")

	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to download checksums")
}

func TestUpdateServiceVerifyChecksumAcceptsMatch(t *testing.T) {
	path, hash := writeChecksumTestArchive(t)
	client := &updateServiceChecksumClientStub{checksumData: []byte(hash + "  " + filepath.Base(path) + "\n")}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.0", "release")

	err := svc.verifyChecksum(context.Background(), path, "https://github.com/x/checksums.txt")

	require.NoError(t, err)
}
