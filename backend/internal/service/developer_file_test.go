//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestDeveloperFileResponseUsesPublicSizeField(t *testing.T) {
	payload, err := json.Marshal(&DeveloperFile{
		URL:         "https://cdn.example.com/file.txt",
		SizeBytes:   12,
		ContentType: "text/plain",
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"url":"https://cdn.example.com/file.txt","size":12,"content_type":"text/plain"}`, string(payload))
}

type developerFileStore struct {
	uploadKey  string
	uploadBody []byte
	uploadSize int64
	deleted    []string
	copies     [][2]string
}

func (s *developerFileStore) Upload(context.Context, string, io.Reader, string) (int64, error) {
	return 0, nil
}
func (s *developerFileStore) UploadFile(context.Context, string, string, string) (int64, error) {
	return 0, nil
}
func (s *developerFileStore) UploadSized(_ context.Context, key string, body io.Reader, size int64, _ string) error {
	s.uploadKey = key
	s.uploadSize = size
	data, err := io.ReadAll(body)
	s.uploadBody = data
	return err
}
func (s *developerFileStore) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}
func (s *developerFileStore) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}
func (s *developerFileStore) CopyObject(_ context.Context, srcKey, dstKey string) error {
	s.copies = append(s.copies, [2]string{srcKey, dstKey})
	return nil
}
func (s *developerFileStore) PresignURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (s *developerFileStore) HeadBucket(context.Context) error { return nil }

func newDeveloperFileServiceTest(t *testing.T) (*DeveloperFileService, *COSImageTransferService, *developerFileStore) {
	t.Helper()
	store := &developerFileStore{}
	settings := newFakeCOSSettingRepo()
	cos := NewCOSImageTransferService(settings, noopEncryptor{}, func(context.Context, *BackupS3Config) (BackupObjectStore, error) {
		return store, nil
	})
	_, err := cos.UpdateConfig(t.Context(), COSImageConfig{
		Enabled:         true,
		Endpoint:        "https://cos.ap-guangzhou.myqcloud.com",
		Region:          "ap-guangzhou",
		Bucket:          "test-bucket",
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
		Prefix:          "assets/",
		PublicBaseURL:   "https://cdn.example.com/storage",
	})
	require.NoError(t, err)
	cfg := &config.Config{}
	cfg.JWT.Secret = "developer-file-test-secret"
	users := &materialPathUserRepo{user: &User{ID: 7, AccountID: "1000000000000001", Status: StatusActive}}
	materials := NewUserMaterialService(nil, cos, users, cfg)
	return NewDeveloperFileService(cos, materials), cos, store
}

func TestDeveloperFileUploadUsesIsolatedPrefixAndRandomName(t *testing.T) {
	svc, _, store := newDeveloperFileServiceTest(t)
	payload := []byte("file contents")
	result, err := svc.Upload(t.Context(), 7, "../../private/evil.PHP", "application/octet-stream", int64(len(payload)), bytes.NewReader(payload))
	require.NoError(t, err)
	require.Equal(t, payload, store.uploadBody)
	require.Equal(t, int64(len(payload)), store.uploadSize)
	require.True(t, strings.HasPrefix(store.uploadKey, "assets/file_uploads/u_"))
	require.NotContains(t, store.uploadKey, "evil")
	require.NotContains(t, store.uploadKey, "..")
	require.True(t, strings.HasSuffix(store.uploadKey, ".php"))
	require.Equal(t, "https://cdn.example.com/storage/"+store.uploadKey, result.URL)
}

func TestDeveloperFileUploadRejectsSizeBeforeStorage(t *testing.T) {
	svc, _, store := newDeveloperFileServiceTest(t)
	for _, size := range []int64{0, DeveloperFileMaxBytes + 1} {
		_, err := svc.Upload(t.Context(), 7, "a.bin", "", size, strings.NewReader("x"))
		require.Error(t, err)
		require.Empty(t, store.uploadKey)
	}
}

func TestDeveloperFileDeleteValidatesOriginAndOwner(t *testing.T) {
	svc, _, store := newDeveloperFileServiceTest(t)
	payload := []byte("x")
	own, err := svc.Upload(t.Context(), 7, "a.png", "image/png", 1, bytes.NewReader(payload))
	require.NoError(t, err)
	ownKey := store.uploadKey
	require.NoError(t, svc.Delete(t.Context(), 7, own.URL))
	require.Equal(t, []string{ownKey}, store.deleted)

	for _, rawURL := range []string{
		"https://evil.example.com/storage/" + ownKey,
		own.URL + "?signature=x",
		"https://cdn.example.com/storage/assets/file_uploads/u_other/2026/01/a.png",
		"https://cdn.example.com/storage/users/u_other/materials/a.png",
	} {
		before := len(store.deleted)
		err := svc.Delete(t.Context(), 7, rawURL)
		require.Error(t, err, "url=%s", rawURL)
		require.Len(t, store.deleted, before, "url=%s", rawURL)
	}
}

func TestCOSPublicURLToKeyAndMoveFile(t *testing.T) {
	_, cos, store := newDeveloperFileServiceTest(t)
	key, err := cos.PublicURLToKey(t.Context(), "https://cdn.example.com/storage/assets/file_uploads/u_x/a.png")
	require.NoError(t, err)
	require.Equal(t, "assets/file_uploads/u_x/a.png", key)

	destinationURL, err := cos.MoveFile(t.Context(), key, "users/u_x/materials/a.png")
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example.com/storage/users/u_x/materials/a.png", destinationURL)
	require.Equal(t, [][2]string{{key, "users/u_x/materials/a.png"}}, store.copies)
	require.Equal(t, []string{key}, store.deleted)

	_, err = cos.PublicURLToKey(t.Context(), "https://cdn.example.com/other/a.png")
	require.Error(t, err)
	require.Equal(t, "INVALID_FILE_URL", infraerrors.Reason(err))
}
