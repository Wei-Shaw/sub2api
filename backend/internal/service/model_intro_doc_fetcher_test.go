package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNormalizeModelIntroDocURL 覆盖 admin"从文档链接生成"入口的 URL 归一化：
// 补 scheme、拒绝非 http(s)、去 fragment、缺 host 直接报错。
func TestNormalizeModelIntroDocURL(t *testing.T) {
	t.Run("adds https scheme when missing", func(t *testing.T) {
		u, err := normalizeModelIntroDocURL("  fal.ai/models/fal-ai/veo3  ")
		require.NoError(t, err)
		require.Equal(t, "https://fal.ai/models/fal-ai/veo3", u.String())
	})

	t.Run("keeps http scheme and drops fragment", func(t *testing.T) {
		u, err := normalizeModelIntroDocURL("HTTP://example.com/docs?a=1#params")
		require.NoError(t, err)
		require.Equal(t, "http://example.com/docs?a=1", u.String())
	})

	t.Run("rejects empty and non-http schemes", func(t *testing.T) {
		for _, raw := range []string{"", "   ", "ftp://example.com/x", "file:///etc/passwd", "javascript://alert(1)"} {
			_, err := normalizeModelIntroDocURL(raw)
			require.ErrorIs(t, err, ErrModelIntroDocInvalidURL, "raw=%q", raw)
		}
	})
}

// TestExtractHTMLTitle 覆盖 <title> 粗提：大小写/换行/实体转义/缺失。
func TestExtractHTMLTitle(t *testing.T) {
	t.Run("unescapes entities and collapses whitespace", func(t *testing.T) {
		got := extractHTMLTitle([]byte("<html><HEAD><Title>\n  Veo 3 &amp;\tAudio  \n</Title></HEAD></html>"))
		require.Equal(t, "Veo 3 & Audio", got)
	})

	t.Run("returns empty when absent", func(t *testing.T) {
		require.Equal(t, "", extractHTMLTitle([]byte("<html><body>no title here</body></html>")))
	})

	t.Run("caps length at 300 runes", func(t *testing.T) {
		long := make([]rune, 0, 400)
		for i := 0; i < 400; i++ {
			long = append(long, '中')
		}
		got := extractHTMLTitle([]byte("<title>" + string(long) + "</title>"))
		require.Len(t, []rune(got), 300)
	})
}

// TestModelIntroDocFetcherRejectsPrivateHost 确认内网/回环目标在发请求前就被拒绝，
// 避免该接口被当成 SSRF 跳板（handler 会把它映射成 400）。
func TestModelIntroDocFetcherRejectsPrivateHost(t *testing.T) {
	f := NewModelIntroDocFetcher()
	for _, raw := range []string{
		"http://127.0.0.1:8080/docs",
		"http://localhost/docs",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.5/internal",
	} {
		_, err := f.Fetch(t.Context(), raw, 0)
		require.ErrorIs(t, err, ErrModelIntroDocBlockedURL, "raw=%q", raw)
	}
}
