//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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

		_, err = s.Upload(ctx, "a.png", []byte("x"), "image/png")
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
