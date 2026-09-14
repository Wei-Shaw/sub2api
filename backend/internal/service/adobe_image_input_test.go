//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 客户端可以把任意 URL 交给我们去请求，这是一条标准的 SSRF 入口。
// 下面几条是这条路径的安全边界，改动取图器时必须保持它们成立。
func TestIsPublicUnicastIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", // 环回
		"10.0.0.1", "172.16.0.1", "192.168.1.1", "fd00::1", // 私网
		"169.254.169.254", "fe80::1", // 链路本地（含云元数据端点）
		"0.0.0.0", "::", // 未指定
		"224.0.0.1", "ff02::1", // 组播
		"100.64.0.1", "100.127.255.255", // 运营商级 NAT
		"255.255.255.255", // 广播
	}
	for _, raw := range blocked {
		require.False(t, isPublicUnicastIP(net.ParseIP(raw)), "%s 应被拒绝", raw)
	}

	allowed := []string{"1.1.1.1", "8.8.8.8", "93.184.216.34", "2606:4700::1111", "100.63.255.255", "100.128.0.1"}
	for _, raw := range allowed {
		require.True(t, isPublicUnicastIP(net.ParseIP(raw)), "%s 应被放行", raw)
	}

	require.False(t, isPublicUnicastIP(nil))
}

func TestFetchAdobeInputImageRejectsLoopback(t *testing.T) {
	// httptest 起在 127.0.0.1 上，Dialer.Control 应在连接阶段就拦下来。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
	}))
	defer server.Close()

	_, err := fetchAdobeInputImage(context.Background(), newAdobeInputImageClient(), server.URL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not allowed")
}

func TestFetchAdobeInputImageRejectsNonHTTPScheme(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "gopher://x/", "ftp://example.com/a.png"} {
		_, err := fetchAdobeInputImage(context.Background(), newAdobeInputImageClient(), raw)
		require.ErrorIs(t, err, errAdobeInputImageBlocked, raw)
	}
}

func TestFetchAdobeInputImageRejectsEmptyURL(t *testing.T) {
	_, err := fetchAdobeInputImage(context.Background(), newAdobeInputImageClient(), "   ")
	require.ErrorContains(t, err, "empty")
}

func TestDecodeAdobeDataURL(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 32))
	encoded := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

	image, err := decodeAdobeDataURL(encoded)
	require.NoError(t, err)
	require.Equal(t, png, image.Data)
	require.Equal(t, "image/png", image.ContentType)

	// data: URL 不走网络，传 nil 客户端也应能解出来。
	viaFetch, err := fetchAdobeInputImage(context.Background(), nil, encoded)
	require.NoError(t, err)
	require.Equal(t, png, viaFetch.Data)
}

func TestDecodeAdobeDataURLRejectsBadInput(t *testing.T) {
	tests := map[string]string{
		"没有逗号":      "data:image/png;base64",
		"不是 base64": "data:image/png,rawbytes",
		"base64 非法": "data:image/png;base64,!!!!",
		"空负载":       "data:image/png;base64,",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := decodeAdobeDataURL(raw)
			require.Error(t, err)
		})
	}
}

// 声明的类型不可信时按字节嗅探；嗅不出图片才拒绝。
func TestDecodeAdobeDataURLSniffsContentType(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 32))
	image, err := decodeAdobeDataURL("data:application/octet-stream;base64," +
		base64.StdEncoding.EncodeToString(png))
	require.NoError(t, err)
	require.Equal(t, "image/png", image.ContentType)

	_, err = decodeAdobeDataURL("data:application/octet-stream;base64," +
		base64.StdEncoding.EncodeToString([]byte("not an image at all")))
	require.ErrorContains(t, err, "does not carry an image")
}
