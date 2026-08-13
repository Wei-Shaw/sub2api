//go:build unit

package service

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type adminFileStoreStub struct {
	existingKey string
	uploads     int
}

func (s *adminFileStoreStub) Upload(_ context.Context, _ string, body io.Reader, _ string) (int64, error) {
	s.uploads++
	data, err := io.ReadAll(body)
	return int64(len(data)), err
}
func (s *adminFileStoreStub) UploadFile(
	_ context.Context, _ string, filePath string, _ string,
) (int64, error) {
	s.uploads++
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
func (s *adminFileStoreStub) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}
func (s *adminFileStoreStub) Delete(context.Context, string) error { return nil }
func (s *adminFileStoreStub) PresignURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (s *adminFileStoreStub) HeadBucket(context.Context) error { return nil }
func (s *adminFileStoreStub) ListObjects(
	_ context.Context, prefix, _, _ string, _ int32,
) (*ObjectPage, error) {
	entries := []ObjectEntry{}
	if prefix == s.existingKey {
		entries = append(entries, ObjectEntry{Key: s.existingKey})
	}
	return &ObjectPage{Entries: entries}, nil
}

// sanitizeObjectKey 是管理端文件管理的第一道门：所有 key 入参都过它。
// 管理员权限很大，但仍要挡住手误与被诱导构造的畸形 key —— 尤其是 ".."，
// 很多 CDN/网关会做路径归一，可能落到预期之外的对象上。
func TestSanitizeObjectKey(t *testing.T) {
	t.Run("trims leading slashes and whitespace", func(t *testing.T) {
		for _, raw := range []string{"images/a.png", "/images/a.png", "  //images/a.png  "} {
			got, err := sanitizeObjectKey(raw)
			require.NoError(t, err, "raw=%q", raw)
			require.Equal(t, "images/a.png", got, "raw=%q", raw)
		}
	})

	t.Run("rejects empty keys", func(t *testing.T) {
		for _, raw := range []string{"", "   ", "/", "///"} {
			_, err := sanitizeObjectKey(raw)
			require.ErrorIs(t, err, ErrInvalidObjectKey, "raw=%q", raw)
		}
	})

	t.Run("rejects path traversal segments", func(t *testing.T) {
		for _, raw := range []string{
			"../secret.txt",
			"images/../../etc/passwd",
			"images/../a.png",
			"a/b/..",
		} {
			_, err := sanitizeObjectKey(raw)
			require.ErrorIs(t, err, ErrInvalidObjectKey, "raw=%q", raw)
		}
	})

	t.Run("allows dots that are not a full segment", func(t *testing.T) {
		// "..foo" / "a..b" 只是普通文件名，不该被误杀。
		for _, raw := range []string{"images/..foo.png", "images/a..b.png", "images/.hidden"} {
			got, err := sanitizeObjectKey(raw)
			require.NoError(t, err, "raw=%q", raw)
			require.NotEmpty(t, got)
		}
	})

	t.Run("rejects control characters", func(t *testing.T) {
		// \r\n 混进 key 还可能被用于日志注入。
		for _, raw := range []string{"a\nb.png", "a\rb.png", "a\x00b.png", "a\x7fb.png", "a\tb.png"} {
			_, err := sanitizeObjectKey(raw)
			require.ErrorIs(t, err, ErrInvalidObjectKey, "raw=%q", raw)
		}
	})

	t.Run("rejects keys longer than the S3 limit", func(t *testing.T) {
		long := make([]byte, adminFileMaxKeyLength+1)
		for i := range long {
			long[i] = 'a'
		}
		_, err := sanitizeObjectKey(string(long))
		require.ErrorIs(t, err, ErrInvalidObjectKey)

		exact := make([]byte, adminFileMaxKeyLength)
		for i := range exact {
			exact[i] = 'a'
		}
		got, err := sanitizeObjectKey(string(exact))
		require.NoError(t, err)
		require.Len(t, got, adminFileMaxKeyLength)
	})

	t.Run("accepts unicode names", func(t *testing.T) {
		got, err := sanitizeObjectKey("图片/参考图 01.png")
		require.NoError(t, err)
		require.Equal(t, "图片/参考图 01.png", got)
	})
}

func TestNormalizeKeyPrefix(t *testing.T) {
	require.Equal(t, "", normalizeKeyPrefix(""))
	require.Equal(t, "", normalizeKeyPrefix("   "))
	require.Equal(t, "", normalizeKeyPrefix("/"))
	require.Equal(t, "images/", normalizeKeyPrefix("/images/"))
	require.Equal(t, "images/2026/", normalizeKeyPrefix("  images/2026/  "))
	// 不强制补尾斜杠：前缀匹配语义下 "img" 可能是有意的（匹配 img*）。
	require.Equal(t, "img", normalizeKeyPrefix("img"))
}

func TestDisplayNameForKey(t *testing.T) {
	t.Run("strips the current prefix", func(t *testing.T) {
		require.Equal(t, "a.png", displayNameForKey("images/2026/a.png", "images/2026/", false))
		require.Equal(t, "images/2026/a.png", displayNameForKey("images/2026/a.png", "", false))
	})

	t.Run("strips the trailing slash for directories", func(t *testing.T) {
		require.Equal(t, "2026", displayNameForKey("images/2026/", "images/", true))
		require.Equal(t, "images", displayNameForKey("images/", "", true))
	})

	t.Run("falls back to base name when key equals prefix", func(t *testing.T) {
		// 罕见但会发生（目录占位对象），不能返回空白。
		require.Equal(t, "2026", displayNameForKey("images/2026/", "images/2026/", true))
	})
}

func TestIsSafeObjectKey(t *testing.T) {
	require.True(t, isSafeObjectKey("images/a.png"))
	require.True(t, isSafeObjectKey("图片/a.png"))
	require.False(t, isSafeObjectKey("../a.png"))
	require.False(t, isSafeObjectKey("a\nb"))
	// 非法 UTF-8 会让签名、日志与前端渲染都出问题。
	require.False(t, isSafeObjectKey(string([]byte{0xff, 0xfe})))
}

// AdminFileService 在未配置 COS 时必须一律短路成 ErrCOSNotConfigured，
// 而不是把 nil store 带到下游炸出 panic 或 500。
func TestAdminFileServiceRequiresCOS(t *testing.T) {
	ctx := t.Context()

	t.Run("nil cos service", func(t *testing.T) {
		s := NewAdminFileService(nil)
		require.False(t, s.Enabled(ctx))

		_, err := s.List(ctx, "", "/", "", 10)
		require.ErrorIs(t, err, ErrCOSNotConfigured)

		_, err = s.DownloadURL(ctx, "a.png")
		require.ErrorIs(t, err, ErrCOSNotConfigured)

		_, err = s.Upload(ctx, "a.png", []byte("x"), "image/png", false)
		require.ErrorIs(t, err, ErrCOSNotConfigured)

		_, err = s.ImportFromURL(ctx, "images/", "", "https://example.com/a.png", false)
		require.ErrorIs(t, err, ErrCOSNotConfigured)

		_, err = s.Rename(ctx, "a.png", "b.png")
		require.ErrorIs(t, err, ErrCOSNotConfigured)

		require.ErrorIs(t, s.Delete(ctx, "a.png"), ErrCOSNotConfigured)

		_, _, err2 := s.StorageInfo(ctx)
		require.ErrorIs(t, err2, ErrCOSNotConfigured)
	})

	t.Run("nil receiver stays safe", func(t *testing.T) {
		var s *AdminFileService
		require.False(t, s.Enabled(ctx))
	})
}

func TestAdminImportFileName(t *testing.T) {
	require.Equal(t, "photo.jpg", adminImportFileName(
		"https://cdn.example.com/a/photo.jpg?token=1", "image/jpeg",
	))
	require.Equal(t, "video.mp4", adminImportFileName(
		"https://cdn.example.com/video.mp4#preview", "video/mp4",
	))
	require.Equal(t, "download.mp3", adminImportFileName("https://cdn.example.com/", "audio/mpeg"))
}

func TestNormalizeAdminImportTarget(t *testing.T) {
	prefix, name, err := normalizeAdminImportTarget("/images/2026", " photo.png ")
	require.NoError(t, err)
	require.Equal(t, "images/2026/", prefix)
	require.Equal(t, "photo.png", name)

	for _, tc := range []struct{ prefix, name string }{
		{"../private", "photo.png"},
		{"images/", "."},
		{"images/", "/"},
	} {
		_, _, err := normalizeAdminImportTarget(tc.prefix, tc.name)
		require.ErrorIs(t, err, ErrInvalidObjectKey)
	}
}

func TestAdminFileUploadRequiresExplicitOverwrite(t *testing.T) {
	store := &adminFileStoreStub{existingKey: "images/photo.png"}
	repo := newFakeCOSSettingRepo()
	cos := NewCOSImageTransferService(repo, noopEncryptor{}, func(
		context.Context, *BackupS3Config,
	) (BackupObjectStore, error) {
		return store, nil
	})
	enableCOS(t, cos)
	svc := NewAdminFileService(cos)

	_, err := svc.Upload(t.Context(), "images/photo.png", []byte("new"), "image/png", false)
	require.ErrorIs(t, err, ErrObjectKeyExists)
	require.Zero(t, store.uploads)

	_, err = svc.Upload(t.Context(), "images/photo.png", []byte("new"), "image/png", true)
	require.NoError(t, err)
	require.Equal(t, 1, store.uploads)
}

// DeleteMany 要做到"部分失败不影响其余"，并把逐条原因带回给调用方。
func TestAdminFileServiceDeleteManyReportsFailures(t *testing.T) {
	s := NewAdminFileService(nil) // 未配置 → 每条都失败，正好验证汇总逻辑
	deleted, failures := s.DeleteMany(t.Context(), []string{"a.png", "b.png"})
	require.Equal(t, 0, deleted)
	require.Len(t, failures, 2)
	require.Contains(t, failures, "a.png")
	require.Contains(t, failures, "b.png")
}

func TestAdminFileUploadMaxBytesExposed(t *testing.T) {
	// handler 用它设置 body 读取上限；两边必须是同一个值。
	require.Equal(t, adminFileUploadMaxBytes, AdminFileUploadMaxBytes())
	require.Positive(t, AdminFileUploadMaxBytes())
}
