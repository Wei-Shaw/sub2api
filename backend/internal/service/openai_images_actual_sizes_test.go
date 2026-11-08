package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// actualSizePNGBytes 返回指定尺寸的 PNG 原始字节，用于模拟上游以 url 返回的图片。
func actualSizePNGBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encodeOpenAIImageTestPNG(t, width, height))
	require.NoError(t, err)
	return data
}

func newActualSizeProbeService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
}

func TestDetectOpenAIImageBytesSize(t *testing.T) {
	png := actualSizePNGBytes(t, 1672, 941)
	require.Equal(t, "1672x941", detectOpenAIImageBytesSize(png))
	require.Equal(t, "1254x1254", detectOpenAIImageBytesSize(actualSizePNGBytes(t, 1254, 1254)))

	jpegEncoded, err := base64.StdEncoding.DecodeString(encodeOpenAIImageTestJPEG(t, 1402, 1122))
	require.NoError(t, err)
	require.Equal(t, "1402x1122", detectOpenAIImageBytesSize(jpegEncoded))

	webpEncoded, err := base64.StdEncoding.DecodeString(encodeOpenAIImageTestWebPVP8X(1920, 1080))
	require.NoError(t, err)
	require.Equal(t, "1920x1080", detectOpenAIImageBytesSize(webpEncoded))

	require.Empty(t, detectOpenAIImageBytesSize(nil))
	require.Empty(t, detectOpenAIImageBytesSize([]byte{}))
	require.Empty(t, detectOpenAIImageBytesSize([]byte("not an image")))
	// 只有魔数、长度不足时既不得 panic，也不得报出错误尺寸。
	require.Empty(t, detectOpenAIImageBytesSize(png[:8]))
}

func TestIsResolvedImageSize(t *testing.T) {
	tests := map[string]bool{
		"1672x941":    true,
		"1254x1254":   true,
		"1024X1024":   true,
		" 3840x2160 ": true,
		"":            false,
		"auto":        false,
		"AUTO":        false,
		"1k":          false,
		"1672":        false,
		"x941":        false,
		"0x941":       false,
		"abcx941":     false,
	}
	for size, want := range tests {
		require.Equal(t, want, isResolvedImageSize(size), "size=%q", size)
	}
}

func TestResolveOpenAIImagesResultSizes(t *testing.T) {
	inflight := base64.StdEncoding.EncodeToString(actualSizePNGBytes(t, 1254, 1254))

	tests := []struct {
		name          string
		body          string
		upstream      *httpUpstreamRecorder
		wantSizes     []string
		wantTopLevel  string
		wantDownloads int
	}{
		{
			name:          "内联 b64_json 时直接解码不下载",
			body:          `{"created":1,"data":[{"b64_json":"` + inflight + `"}]}`,
			upstream:      &httpUpstreamRecorder{},
			wantSizes:     []string{"1254x1254"},
			wantTopLevel:  "1254x1254",
			wantDownloads: 0,
		},
		{
			name:          "以 url 返回时只取头部探测",
			body:          `{"created":1,"data":[{"url":"https://cdn.example.com/a.png","revised_prompt":"a cat"}]}`,
			upstream:      &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusOK, "image/png", actualSizePNGBytes(t, 1672, 941))},
			wantSizes:     []string{"1672x941"},
			wantTopLevel:  "1672x941",
			wantDownloads: 1,
		},
		{
			name:          "上游已声明具体尺寸时不覆盖也不下载",
			body:          `{"created":1,"data":[{"size":"1086x1448","url":"https://cdn.example.com/a.png"}]}`,
			upstream:      &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusOK, "image/png", actualSizePNGBytes(t, 1672, 941))},
			wantSizes:     []string{"1086x1448"},
			wantTopLevel:  "",
			wantDownloads: 0,
		},
		{
			name:          "上游回显 auto 时仍探测真实尺寸",
			body:          `{"created":1,"data":[{"size":"auto","url":"https://cdn.example.com/a.png"}]}`,
			upstream:      &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusOK, "image/png", actualSizePNGBytes(t, 1672, 941))},
			wantSizes:     []string{"1672x941"},
			wantTopLevel:  "1672x941",
			wantDownloads: 1,
		},
		{
			name:          "data url 不下载",
			body:          `{"created":1,"data":[{"url":"data:image/png;base64,` + inflight + `"}]}`,
			upstream:      &httpUpstreamRecorder{},
			wantSizes:     []string{"1254x1254"},
			wantTopLevel:  "1254x1254",
			wantDownloads: 0,
		},
		{
			// b64_json 非空说明这批字节已经取过一次（上游内联或回填已下载），
			// 解不出尺寸时不得再下载同一个 url。
			name:          "b64_json 存在但无法解码时不重复下载",
			body:          `{"created":1,"data":[{"b64_json":"aW1n","url":"https://cdn.example.com/a.png"}]}`,
			upstream:      &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusOK, "image/png", actualSizePNGBytes(t, 1672, 941))},
			wantSizes:     []string{""},
			wantDownloads: 0,
		},
		{
			name:          "既无 b64 也无 url 时保持原样",
			body:          `{"created":1,"data":[{"revised_prompt":"a cat"}]}`,
			upstream:      &httpUpstreamRecorder{},
			wantSizes:     []string{""},
			wantDownloads: 0,
		},
		{
			name:          "私网 url 一律不探测",
			body:          `{"created":1,"data":[{"url":"http://127.0.0.1:8080/a.png"}]}`,
			upstream:      &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusOK, "image/png", actualSizePNGBytes(t, 1672, 941))},
			wantSizes:     []string{""},
			wantDownloads: 0,
		},
		{
			name:          "下载失败时 fail-soft",
			body:          `{"created":1,"data":[{"url":"https://cdn.example.com/a.png"}]}`,
			upstream:      &httpUpstreamRecorder{err: errors.New("boom")},
			wantSizes:     []string{""},
			wantDownloads: 1,
		},
		{
			name:          "上游返回非 2xx 时 fail-soft",
			body:          `{"created":1,"data":[{"url":"https://cdn.example.com/a.png"}]}`,
			upstream:      &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusNotFound, "text/plain", []byte("nope"))},
			wantSizes:     []string{""},
			wantDownloads: 1,
		},
		{
			name:          "响应内容不是图片时 fail-soft",
			body:          `{"created":1,"data":[{"url":"https://cdn.example.com/a.png"}]}`,
			upstream:      &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusOK, "text/html", []byte("<html></html>"))},
			wantSizes:     []string{""},
			wantDownloads: 1,
		},
		{
			name:          "多张图各自解析且互不影响",
			body:          `{"created":1,"data":[{"url":"https://cdn.example.com/a.png"},{"size":"1536x1024"}]}`,
			upstream:      &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusOK, "image/png", actualSizePNGBytes(t, 1254, 1254))},
			wantSizes:     []string{"1254x1254", "1536x1024"},
			wantTopLevel:  "1254x1254",
			wantDownloads: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newActualSizeProbeService(tt.upstream)
			got := svc.resolveOpenAIImagesResultSizes(context.Background(), b64BackfillAccount(false), []byte(tt.body))
			require.True(t, gjson.ValidBytes(got))

			items := gjson.GetBytes(got, "data").Array()
			require.Len(t, items, len(tt.wantSizes))
			for i, want := range tt.wantSizes {
				require.Equal(t, want, items[i].Get("size").String(), "data.%d.size", i)
			}
			require.Equal(t, tt.wantTopLevel, gjson.GetBytes(got, "size").String())
			require.Len(t, tt.upstream.requests, tt.wantDownloads)

			// 顶层 size 只在网关自行解析出尺寸时才写入，因此它精确对应"响应是否被改写"：
			// 没有解析出任何尺寸时必须逐字节保持原样（不改动响应形态、不影响既有客户端）。
			if tt.wantTopLevel == "" {
				require.Equal(t, tt.body, string(got))
			}
			// 除 size 外的字段必须原样保留。
			if gjson.Get(tt.body, "data.0.revised_prompt").Exists() {
				require.Equal(t, "a cat", gjson.GetBytes(got, "data.0.revised_prompt").String())
			}
			if gjson.Get(tt.body, "created").Exists() {
				require.Equal(t, int64(1), gjson.GetBytes(got, "created").Int())
			}
		})
	}
}

func TestResolveOpenAIImagesResultSizes_ProbeRequestShape(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusOK, "image/png", actualSizePNGBytes(t, 1672, 941))}
	svc := newActualSizeProbeService(upstream)
	account := b64BackfillAccount(false)
	proxyID := int64(3)
	account.ProxyID = &proxyID
	account.Proxy = &Proxy{Protocol: "http", Host: "127.0.0.1", Port: 7890}

	body := []byte(`{"created":1,"data":[{"url":"https://cdn.example.com/a.png?sig=abc"}]}`)
	got := svc.resolveOpenAIImagesResultSizes(context.Background(), account, body)
	require.Equal(t, "1672x941", gjson.GetBytes(got, "data.0.size").String())

	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, http.MethodGet, req.Method)
	require.Equal(t, "https://cdn.example.com/a.png?sig=abc", req.URL.String())
	require.Equal(t, "http://127.0.0.1:7890", upstream.lastProxyURL)
	// 只取头部，避免为读图片头而下载整图。
	require.Equal(t, "bytes=0-1048575", req.Header.Get("Range"))
	// 目的地与重定向各跳都必须解析到公网地址。
	require.True(t, HTTPUpstreamPublicHostsOnly(req.Context()))
	_, hasDeadline := req.Context().Deadline()
	require.True(t, hasDeadline)
}

// TestResolveOpenAIImagesResultSizes_DrivesBillingTierFromDeliveredSize 锁死计费口径：
// 解析出真实交付像素后，档位取真实交付（source=output），而不是请求尺寸。
func TestResolveOpenAIImagesResultSizes_DrivesBillingTierFromDeliveredSize(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: b64BackfillImageResponse(http.StatusOK, "image/png", actualSizePNGBytes(t, 1672, 941))}
	svc := newActualSizeProbeService(upstream)

	body := []byte(`{"created":1,"data":[{"url":"https://cdn.example.com/a.png","revised_prompt":"a cat"}]}`)
	got := svc.resolveOpenAIImagesResultSizes(context.Background(), b64BackfillAccount(false), body)

	sizes := collectOpenAIResponseImageOutputSizesFromJSONBytes(got)
	require.Equal(t, []string{"1672x941"}, sizes)

	after := &OpenAIForwardResult{
		ImageCount:       1,
		ImageSize:        ImageBillingSize4K,
		ImageInputSize:   "3840x2160",
		ImageOutputSizes: sizes,
	}
	ApplyOpenAIImageBillingResolution(after)
	require.Equal(t, ImageBillingSize2K, after.ImageSize)
	require.Equal(t, "1672x941", after.ImageOutputSize)
	require.Equal(t, ImageSizeSourceOutput, after.ImageSizeSource)

	// 对照组：拿不到交付尺寸时（修复前的行为）只能回落请求尺寸 —— 请求 4K、实交付 2K 级画布，
	// 却被按 4K 档计费，这正是社区 issue #4690「生图收费有bug」的成因。
	before := &OpenAIForwardResult{
		ImageCount:     1,
		ImageSize:      ImageBillingSize4K,
		ImageInputSize: "3840x2160",
	}
	ApplyOpenAIImageBillingResolution(before)
	require.Equal(t, ImageBillingSize4K, before.ImageSize)
	require.Equal(t, ImageSizeSourceInput, before.ImageSizeSource)
}

// TestResolveOpenAIImagesResultSizes_ToleratesNilUpstreamResponse 锁死探测的 fail-soft 边界：
// 上游客户端返回 (nil, nil) 属契约违例，探测必须当作失败，绝不能让生图请求 panic。
func TestResolveOpenAIImagesResultSizes_ToleratesNilUpstreamResponse(t *testing.T) {
	upstream := &httpUpstreamRecorder{}
	svc := newActualSizeProbeService(upstream)

	body := []byte(`{"created":1,"data":[{"url":"https://cdn.example.com/a.png"}]}`)
	got := svc.resolveOpenAIImagesResultSizes(context.Background(), b64BackfillAccount(false), body)
	require.Equal(t, string(body), string(got))
	require.Len(t, upstream.requests, 1)
}

// TestOpenAIGatewayServiceForwardImages_APIKeyResolvesDeliveredSizeFromURL 是端到端闭环：
// 上游以 url 返回图片（不回显 size）且请求的是 4K 时，网关探测出真实交付像素 1672x941，
// 客户端响应（同时是异步任务落库的那份 body）拿到真实尺寸，计费档位按真实交付（2K）
// 而不是请求尺寸（4K）。这对应生产环境"输出尺寸未知 + 按请求尺寸计费"的修复验收。
func TestOpenAIGatewayServiceForwardImages_APIKeyResolvesDeliveredSizeFromURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat","size":"3840x2160","quality":"low"}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("api_key", &APIKey{ID: 42})

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	upstreamBody := `{"created":1710000000,"data":[{"url":"https://cdn.example.com/cat.png"}],"usage":{"input_tokens":27,"output_tokens":2168}}`
	upstream := &httpUpstreamRecorder{
		responses: []*http.Response{
			b64BackfillImageResponse(http.StatusOK, "application/json", []byte(upstreamBody)),
			b64BackfillImageResponse(http.StatusOK, "image/png", actualSizePNGBytes(t, 1672, 941)),
		},
	}
	svc.httpUpstream = upstream

	result, err := svc.ForwardImages(context.Background(), c, b64BackfillAccount(false), body, parsed, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, []string{"1672x941"}, result.ImageOutputSizes)

	// 客户端可见性：真实交付尺寸随响应一起返回，url 与其它字段原样保留。
	require.Equal(t, "1672x941", gjson.Get(rec.Body.String(), "data.0.size").String())
	require.Equal(t, "1672x941", gjson.Get(rec.Body.String(), "size").String())
	require.Equal(t, "https://cdn.example.com/cat.png", gjson.Get(rec.Body.String(), "data.0.url").String())
	require.Equal(t, int64(27), gjson.Get(rec.Body.String(), "usage.input_tokens").Int())

	// 计费口径：请求 4K、实交付 2K 级画布 → 按真实交付定 2K 档。
	require.Equal(t, "3840x2160", result.ImageInputSize)
	ApplyOpenAIImageBillingResolution(result)
	require.Equal(t, ImageBillingSize2K, result.ImageSize)
	require.Equal(t, "1672x941", result.ImageOutputSize)
	require.Equal(t, ImageSizeSourceOutput, result.ImageSizeSource)
}

// TestResolveOpenAIImagesResultSizes_AutoSizeWouldFallBackToRequestTier 说明上游回显
// size=auto 时的风险：auto 无法定档，输出尺寸会被整条跳过，档位只能回落请求尺寸。
func TestResolveOpenAIImagesResultSizes_AutoSizeWouldFallBackToRequestTier(t *testing.T) {
	require.Equal(t, []string{"auto"}, collectOpenAIResponseImageOutputSizesFromJSONBytes(
		[]byte(`{"data":[{"size":"auto","url":"https://cdn.example.com/a.png"}]}`),
	))

	result := &OpenAIForwardResult{
		ImageCount:       1,
		ImageSize:        ImageBillingSize4K,
		ImageInputSize:   "3840x2160",
		ImageOutputSizes: []string{"auto"},
	}
	ApplyOpenAIImageBillingResolution(result)
	require.Equal(t, ImageBillingSize4K, result.ImageSize)
	require.Equal(t, ImageSizeSourceInput, result.ImageSizeSource)
}
