//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ----- fakes -----

// fakeCOSSettingRepo 仅实现 COS 转存所需的 GetValue/Set，其余方法继承 nil 接口（不会被调用）。
type fakeCOSSettingRepo struct {
	SettingRepository
	mu   sync.Mutex
	data map[string]string
}

func newFakeCOSSettingRepo() *fakeCOSSettingRepo {
	return &fakeCOSSettingRepo{data: map[string]string{}}
}

func (r *fakeCOSSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.data[key], nil
}

func (r *fakeCOSSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[key] = value
	return nil
}

// noopEncryptor 直通加解密（测试不验证加密本身）。
type noopEncryptor struct{}

func (noopEncryptor) Encrypt(s string) (string, error) { return s, nil }
func (noopEncryptor) Decrypt(s string) (string, error) { return s, nil }

// fakeObjectStore 记录上传次数，可配置失败次数模拟重试。
type fakeObjectStore struct {
	uploads     int32
	failUploads int32 // 前 N 次 Upload 返回错误
}

func (s *fakeObjectStore) Upload(_ context.Context, _ string, body io.Reader, _ string) (int64, error) {
	n := atomic.AddInt32(&s.uploads, 1)
	if n <= atomic.LoadInt32(&s.failUploads) {
		return 0, io.ErrClosedPipe
	}
	data, _ := io.ReadAll(body)
	return int64(len(data)), nil
}

func (s *fakeObjectStore) Download(context.Context, string) (io.ReadCloser, error) { return nil, nil }
func (s *fakeObjectStore) Delete(context.Context, string) error                    { return nil }
func (s *fakeObjectStore) PresignURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (s *fakeObjectStore) HeadBucket(context.Context) error { return nil }

func newCOSServiceForTest(t *testing.T, store *fakeObjectStore) (*COSImageTransferService, *fakeCOSSettingRepo) {
	t.Helper()
	repo := newFakeCOSSettingRepo()
	factory := func(context.Context, *BackupS3Config) (BackupObjectStore, error) {
		return store, nil
	}
	svc := NewCOSImageTransferService(repo, noopEncryptor{}, factory)
	return svc, repo
}

func enableCOS(t *testing.T, svc *COSImageTransferService) {
	t.Helper()
	_, err := svc.UpdateConfig(context.Background(), COSImageConfig{
		Enabled:         true,
		Endpoint:        "https://cos.ap-guangzhou.myqcloud.com",
		Region:          "ap-guangzhou",
		Bucket:          "test-bucket",
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
		Prefix:          "images",
		PublicBaseURL:   "https://cdn.example.com",
	})
	require.NoError(t, err)
}

// fakeImageServer 返回固定的图片字节，记录被请求次数。
func fakeImageServer(t *testing.T, failTimes *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failTimes != nil && atomic.AddInt32(failTimes, -1) >= 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\nfake-image-bytes"))
	}))
}

// ----- tests -----

func TestCOSTransfer_Success(t *testing.T) {
	store := &fakeObjectStore{}
	svc, _ := newCOSServiceForTest(t, store)
	enableCOS(t, svc)

	img := fakeImageServer(t, nil)
	defer img.Close()

	out, allOK := svc.TransferImages(context.Background(), []string{img.URL + "/a.png"})
	require.True(t, allOK)
	require.Len(t, out, 1)
	require.Contains(t, out[0], "https://cdn.example.com/")
	require.EqualValues(t, 1, atomic.LoadInt32(&store.uploads))
}

func TestCOSTransfer_RetryThenSucceed(t *testing.T) {
	// 前 2 次上传失败，第 3 次成功（最大 3 次尝试内成功）。
	store := &fakeObjectStore{failUploads: 2}
	svc, _ := newCOSServiceForTest(t, store)
	enableCOS(t, svc)

	img := fakeImageServer(t, nil)
	defer img.Close()

	out, allOK := svc.TransferImages(context.Background(), []string{img.URL + "/a.png"})
	require.True(t, allOK)
	require.Contains(t, out[0], "https://cdn.example.com/")
	require.EqualValues(t, 3, atomic.LoadInt32(&store.uploads))
}

func TestCOSTransfer_FallbackAfterRetriesExhausted(t *testing.T) {
	// 上传始终失败：3 次尝试耗尽后回退（cos url 为空）。
	store := &fakeObjectStore{failUploads: 100}
	svc, _ := newCOSServiceForTest(t, store)
	enableCOS(t, svc)

	img := fakeImageServer(t, nil)
	defer img.Close()

	out, allOK := svc.TransferImages(context.Background(), []string{img.URL + "/a.png"})
	require.False(t, allOK)
	require.Len(t, out, 1)
	require.Equal(t, "", out[0]) // 回退：留空，调用方用原始 fal url
	require.EqualValues(t, 3, atomic.LoadInt32(&store.uploads))
}

func TestCOSTransfer_DownloadFailureFallsBack(t *testing.T) {
	// 下载始终 500：3 次尝试均失败，回退留空。
	store := &fakeObjectStore{}
	svc, _ := newCOSServiceForTest(t, store)
	enableCOS(t, svc)

	failForever := int32(1 << 30)
	img := fakeImageServer(t, &failForever)
	defer img.Close()

	out, allOK := svc.TransferImages(context.Background(), []string{img.URL + "/a.png"})
	require.False(t, allOK)
	require.Equal(t, "", out[0])
	require.EqualValues(t, 0, atomic.LoadInt32(&store.uploads)) // 下载失败不会触发上传
}

func TestCOSTransfer_DisabledReturnsNotOK(t *testing.T) {
	store := &fakeObjectStore{}
	svc, _ := newCOSServiceForTest(t, store)
	// 未启用配置。
	require.False(t, svc.IsEnabled(context.Background()))

	out, allOK := svc.TransferImages(context.Background(), []string{"https://fal.example/a.png"})
	require.False(t, allOK)
	require.Len(t, out, 1)
	require.Equal(t, "", out[0])
}
