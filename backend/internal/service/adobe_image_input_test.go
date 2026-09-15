//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		"0.0.0.0", "::", "0.1.2.3", // 未指定 / 本网络
		"224.0.0.1", "ff02::1", // 组播
		"100.64.0.1", "100.127.255.255", // 运营商级 NAT
		"255.255.255.255", "240.0.0.1", // 广播 / 保留
		"192.0.0.1", "192.0.2.1", "198.18.0.1", "198.19.255.255", "198.51.100.1", "203.0.113.1", // 特殊用途
		"64:ff9b::a00:1", "64:ff9b:1::1", // NAT64
		"2002:7f00:1::", "2001::1", "2001:db8::1", "100::1", // 6to4 / Teredo / 文档 / 丢弃
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", // IPv4-mapped
	}
	for _, raw := range blocked {
		require.False(t, isPublicUnicastIP(net.ParseIP(raw)), "%s 应被拒绝", raw)
	}

	allowed := []string{"1.1.1.1", "8.8.8.8", "93.184.216.34", "2606:4700::1111", "100.63.255.255", "100.128.0.1", "::ffff:1.1.1.1"}
	for _, raw := range allowed {
		require.True(t, isPublicUnicastIP(net.ParseIP(raw)), "%s 应被放行", raw)
	}

	require.False(t, isPublicUnicastIP(nil))
}

// 输入图 URL 只接受 https + 域名（禁止 IP 字面量）+ 443 端口。
func TestValidateAdobeInputImageURL(t *testing.T) {
	allowed := []string{
		"https://example.com/a.png",
		"https://cdn.example.co.uk:443/x?y=1",
		"https://EXAMPLE.com./a",
		"HTTPS://img.xn--fiqs8s/a.png",
		"https://a-b.example123.io/a",
	}
	for _, raw := range allowed {
		u, err := validateAdobeInputImageURL(raw)
		require.NoError(t, err, raw)
		require.Equal(t, "https", u.Scheme, raw)
	}

	blocked := []string{
		// scheme
		"http://example.com/a.png", "ftp://example.com/a.png", "file:///etc/passwd", "gopher://x/", "//example.com/a", "example.com/a",
		// IP 字面量
		"https://1.1.1.1/a", "https://127.0.0.1/", "https://[2606:4700::1111]/a", "https://[::1]/", "https://[::ffff:127.0.0.1]/", "https://[fe80::1%25en0]/",
		// 会被当作 IP 解析的数字写法
		"https://127.1/", "https://2130706433/", "https://0x7f.1/", "https://0177.0.0.1/", "https://0x7f000001/", "https://1.2.3.4./",
		// localhost 与单 label
		"https://localhost/", "https://a.localhost/", "https://intranet/",
		// 端口、userinfo、非法主机名
		"https://example.com:8443/", "https://example.com:80/", "https://user:pw@example.com/", "https://user@example.com/",
		"https://exämple.com/", "https://-bad.example.com/", "https://bad-.example.com/", "https://a..example.com/", "https://under_score.example.com/",
		"https:///nohost",
	}
	for _, raw := range blocked {
		_, err := validateAdobeInputImageURL(raw)
		require.ErrorIs(t, err, errAdobeInputImageBlocked, raw)
	}
}

// Dialer.Control 是 DNS rebinding 的最后一道防线：解析出的 IP 与端口在连接时再校验一次。
func TestAdobeInputImageDialControl(t *testing.T) {
	for _, address := range []string{"127.0.0.1:443", "10.0.0.1:443", "[64:ff9b::a00:1]:443", "[::1]:443", "1.1.1.1:80", "1.1.1.1:8443", "garbage"} {
		require.ErrorIs(t, adobeInputImageDialControl("tcp", address, nil), errAdobeInputImageBlocked, address)
	}
	require.NoError(t, adobeInputImageDialControl("tcp", "1.1.1.1:443", nil))
	require.NoError(t, adobeInputImageDialControl("tcp6", "[2606:4700::1111]:443", nil))
}

func TestFetchAdobeInputImageRejectsIPLiteralServer(t *testing.T) {
	// httptest 的 URL 是 IP 字面量（且可能是 http），应在校验层就被拦下，不发请求。
	var hits int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
	}))
	defer server.Close()

	_, err := fetchAdobeInputImage(context.Background(), server.Client(), server.URL)
	require.ErrorIs(t, err, errAdobeInputImageBlocked)
	require.Zero(t, hits)
}

func TestFetchAdobeInputImageRejectsNonHTTPSScheme(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "gopher://x/", "ftp://example.com/a.png", "http://example.com/a.png"} {
		_, err := fetchAdobeInputImage(context.Background(), newAdobeInputImageClient(), raw)
		require.ErrorIs(t, err, errAdobeInputImageBlocked, raw)
	}
}

// 任何 30x 都不跟随，并按策略拒绝处理（而非普通的状态码错误）。
func TestDoFetchAdobeInputImageRejectsRedirect(t *testing.T) {
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		var followed bool
		mux := http.NewServeMux()
		mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/final", status)
		})
		mux.HandleFunc("/final", func(w http.ResponseWriter, _ *http.Request) {
			followed = true
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
		})
		server := httptest.NewTLSServer(mux)

		client := server.Client()
		client.CheckRedirect = newAdobeInputImageClient().CheckRedirect
		target, err := url.Parse(server.URL + "/start")
		require.NoError(t, err)

		_, err = doFetchAdobeInputImage(context.Background(), client, target)
		require.ErrorIs(t, err, errAdobeInputImageBlocked, status)
		require.False(t, followed, status)
		server.Close()
	}
}

func TestAdobeInputImageRequestErrorHidesDetails(t *testing.T) {
	blocked := adobeInputImageRequestError(fmt.Errorf("%w: %w", errAdobeInputImageFetchFailed,
		fmt.Errorf("dial tcp 10.1.2.3:443: %w: 10.1.2.3", errAdobeInputImageBlocked)))
	require.Equal(t, adobeInputImageBlockedUserMessage, blocked.User())
	require.Contains(t, blocked.Error(), "10.1.2.3")

	fetchFailed := adobeInputImageRequestError(fmt.Errorf("%w: connection refused", errAdobeInputImageFetchFailed))
	require.Equal(t, adobeInputImageFetchUserMessage, fetchFailed.User())

	local := adobeInputImageRequestError(errors.New("input image exceeds 20971520 bytes"))
	require.Equal(t, "input image exceeds 20971520 bytes", local.User())
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

// 超大 data URL 在解码前就被拒绝。
func TestDecodeAdobeDataURLRejectsOversizedBeforeDecode(t *testing.T) {
	payload := strings.Repeat("A", base64.StdEncoding.EncodedLen(adobeInputImageMaxBytes+64))
	_, err := decodeAdobeDataURL("data:image/png;base64," + payload)
	require.ErrorContains(t, err, "exceeds")

	// 恰好等于上限的图仍可接受。
	exact := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x89}, adobeInputImageMaxBytes))
	image, err := decodeAdobeDataURL("data:image/png;base64," + exact)
	require.NoError(t, err)
	require.Len(t, image.Data, adobeInputImageMaxBytes)
}
