//go:build integration

package adobe

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 真机冒烟：打真实 Adobe 上游，验证「cookie → token → 额度 → 出图」整条链路。
//
// 需要一份有效的 Adobe cookie：
//
//	ADOBE_TEST_COOKIE='...' go test -tags=integration -run TestFireflySmoke ./internal/pkg/adobe/
//
// 可选 ADOBE_TEST_PROXY 指定出口代理。产物写到 t.TempDir()，失败时用 -v 看路径。
func TestFireflySmoke(t *testing.T) {
	cookie := os.Getenv("ADOBE_TEST_COOKIE")
	if cookie == "" {
		t.Skip("未设置 ADOBE_TEST_COOKIE，跳过真机冒烟")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := NewClient(ClientConfig{ProxyURL: os.Getenv("ADOBE_TEST_PROXY")})

	refreshed, err := client.RefreshAccessTokenFromCookie(ctx, cookie, RefreshOptions{})
	require.NoError(t, err, "cookie 换 token 失败")
	require.NotEmpty(t, refreshed.AccessToken)

	exp, ok := DecodeJWTExp(refreshed.AccessToken)
	require.True(t, ok, "token 的过期时间应可解析")
	require.Greater(t, exp, time.Now().Unix(), "token 不应已过期")
	accountID := AccountIDFromToken(refreshed.AccessToken)
	t.Logf("token 有效期至 %s；账号 %+v；account_id=%q normalized=%q",
		time.Unix(exp, 0).Format(time.RFC3339), refreshed.Account, accountID, normalizeAdobeAccountID(accountID))

	balance, err := client.FetchCreditsBalance(ctx, refreshed.AccessToken)
	require.NoError(t, err, "查额度失败")
	require.NotNil(t, balance.Available, "上游应返回可用额度")
	t.Logf("额度 total=%v used=%v available=%v", deref(balance.Total), deref(balance.Used), deref(balance.Available))

	conf, err := ResolveImage(ImageRequest{ModelID: "firefly-gpt-image-2", Ratio: "1:1", Resolution: Resolution1K})
	require.NoError(t, err)
	t.Logf("出图模型 family=%s upstream=%s/%s",
		conf.Family, conf.UpstreamModelID, conf.UpstreamModelVersion)

	out, err := client.GenerateImage(ctx, GenerateImageInput{
		Token: refreshed.AccessToken,
		Options: ImagePayloadOptions{
			Prompt:               "a single red ceramic coffee mug, isolated subject, plain background",
			AspectRatio:          conf.AspectRatio,
			OutputResolution:     conf.OutputResolution,
			UpstreamModelID:      conf.UpstreamModelID,
			UpstreamModelVersion: conf.UpstreamModelVersion,
			PayloadKind:          conf.PayloadKind,
			SizePixels:           conf.SizePixels,
			QualityLevel:         "low",
			Background:           "transparent",
		},
	})
	require.NoError(t, err, "出图失败")
	require.NotEmpty(t, out.Bytes)

	path := filepath.Join(t.TempDir(), "firefly-smoke.png")
	require.NoError(t, os.WriteFile(path, out.Bytes, 0o600))
	t.Logf("产物已写入 %s（%d 字节）", path, len(out.Bytes))

	img, format, err := image.Decode(bytes.NewReader(out.Bytes))
	require.NoError(t, err, "产物应是可解码的图片")
	bounds := img.Bounds()
	require.Equal(t, bounds.Dx(), bounds.Dy(), "1:1 请求应返回方图")
	t.Logf("格式 %s，尺寸 %dx%d", format, bounds.Dx(), bounds.Dy())

	// 关键：不能只看「有没有 alpha 通道」。上游静默降级时回来的是带 alpha 通道但
	// 整幅全不透明的 PNG，只看通道会被骗，必须数真透明像素。
	//
	// 另外 alpha 不会精确落在 0 和 255，而是集中在 1~2 与 250~252，故用视觉阈值
	// （a<=8 视为透明）而不是 a==0。
	var transparent, semi, total int
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			alpha8 := a >> 8
			switch {
			case alpha8 <= 8:
				transparent++
			case alpha8 < 247:
				semi++
			}
			total++
		}
	}
	ratio := float64(transparent) / float64(total) * 100
	t.Logf("透明像素 %.2f%%（半透明 %.2f%%）", ratio, float64(semi)/float64(total)*100)
	require.Positive(t, transparent,
		"background=transparent 应产出真透明像素；全不透明说明上游静默降级了")
}

func deref(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
