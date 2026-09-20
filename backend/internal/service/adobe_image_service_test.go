//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/stretchr/testify/require"
)

// adobeFakeTransport 按调用序号返回预置响应，并记录收到的请求。
type adobeFakeTransport struct {
	mu      sync.Mutex
	handler func(req *adobe.Request, index int) (*adobe.Response, error)
	calls   []*adobe.Request
}

func (t *adobeFakeTransport) Do(_ context.Context, req *adobe.Request) (*adobe.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	index := len(t.calls)
	t.calls = append(t.calls, req)
	return t.handler(req, index)
}

func adobeJSONResponse(t *testing.T, status int, body any, headers map[string]string) *adobe.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	if headers == nil {
		headers = map[string]string{}
	}
	return &adobe.Response{StatusCode: status, Headers: headers, Body: raw}
}

// adobeSubmitPollDownload 模拟成功的「提交 → 轮询 → 下载」，同一 Transport 可服务多次 fan-out。
func adobeSubmitPollDownload(t *testing.T, api *adobeFakeTransport, imageBytes []byte) *adobe.Client {
	t.Helper()
	api.handler = func(req *adobe.Request, _ int) (*adobe.Response, error) {
		if req.URL == adobe.ImageSubmitURL {
			return adobeJSONResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		}
		return adobeJSONResponse(t, 200, map[string]any{
			"status":  "COMPLETED",
			"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": "https://cdn/img.png"}}},
		}, nil), nil
	}
	download := &adobeFakeTransport{handler: func(*adobe.Request, int) (*adobe.Response, error) {
		return &adobe.Response{StatusCode: 200, Headers: map[string]string{}, Body: imageBytes}, nil
	}}
	return adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: download})
}

func adobeSubmitBodies(t *testing.T, api *adobeFakeTransport) []map[string]any {
	t.Helper()
	api.mu.Lock()
	defer api.mu.Unlock()
	var bodies []map[string]any
	for _, req := range api.calls {
		if req.URL != adobe.ImageSubmitURL {
			continue
		}
		var submitted map[string]any
		require.NoError(t, json.Unmarshal(req.Body, &submitted))
		bodies = append(bodies, submitted)
	}
	return bodies
}

func newAdobeTestService(t *testing.T, client *adobe.Client, resolve ImageStorageResolver) *AdobeImageService {
	t.Helper()
	svc := NewAdobeImageService(resolve)
	svc.clients.newClient = func(string) *adobe.Client { return client }
	return svc
}

func adobeTestAccount() *Account {
	return &Account{ID: 7, Platform: PlatformAdobe, Type: AccountTypeOAuth}
}

func TestAdobeImageServiceGenerateFillsBillingFields(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("PNGDATA"))
	svc := newAdobeTestService(t, client, nil)

	result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model:  "gpt-image-2",
		Prompt: "a cat",
		Size:   "1024x1024",
		N:      1,
	})
	require.NoError(t, err)

	// 计费只认这两个字段：写错就是静默错账，没有任何其它信号会报警。
	require.Equal(t, 1, result.Forward.ImageCount)
	require.Equal(t, "1K", result.Forward.ImageSize)
	require.Equal(t, "1K", NormalizeImageBillingTierOrDefault(result.Forward.ImageSize))
	require.Equal(t, [][]byte{[]byte("PNGDATA")}, result.Images)

	// Model 记客户端请求名，UpstreamModel 记实际打到 Adobe 的全量 id。
	require.Equal(t, "gpt-image-2", result.Forward.Model)
	require.Equal(t, "firefly-gpt-image-2", result.Forward.UpstreamModel)
	require.NotEmpty(t, result.Forward.RequestID)
	require.Greater(t, result.Forward.Duration, time.Duration(0))
}

// 账号的默认 mapping 必须真的生效：gpt-image-2 → firefly-gpt-image-2。
func TestAdobeImageServiceAppliesAccountModelMapping(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("X"))
	svc := newAdobeTestService(t, client, nil)

	account := adobeTestAccount()
	account.Credentials = map[string]any{
		"model_mapping": map[string]any{"my-alias": "firefly-nano-banana-pro"},
	}
	result, err := svc.Generate(context.Background(), account, "tok", &OpenAIImagesRequest{
		Model: "my-alias", Prompt: "x", Size: "1024x1024", N: 1,
	})
	require.NoError(t, err)
	require.Equal(t, "firefly-nano-banana-pro-1k-1x1", result.Forward.UpstreamModel)
}

func TestAdobeImageServiceForwardsAccountARPSessionID(t *testing.T) {
	const wantARP = "eyJzaWQiOiJzZXJ2aWNlLWFycCIsImZ0ciI6InJlYWwifQ=="
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("X"))
	svc := newAdobeTestService(t, client, nil)

	account := adobeTestAccount()
	account.Credentials = map[string]any{"arp_session_id": wantARP}
	_, err := svc.Generate(context.Background(), account, "tok", &OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "x", Size: "1024x1024", N: 1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, api.calls)
	require.Equal(t, wantARP, api.calls[0].Headers["x-arp-session-id"])
}

func TestAdobeImageServicePassesGeminiAspectRatioAndImageSize(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("X"))
	svc := newAdobeTestService(t, client, nil)

	req := &OpenAIImagesRequest{
		Model: "nano-banana-pro", Prompt: "x", Size: "2048x2048", N: 1,
	}
	call := NewAdobeImageCall(req, "")
	call.AspectRatio = "16:9"
	call.ImageSize = "2K"

	result, err := svc.GenerateCall(context.Background(), adobeTestAccount(), "tok", call)
	require.NoError(t, err)
	require.Equal(t, "2K", result.Forward.ImageSize)
	require.Equal(t, "firefly-nano-banana-pro-2k-16x9", result.Forward.UpstreamModel)

	require.NotEmpty(t, api.calls)
	var submitted map[string]any
	require.NoError(t, json.Unmarshal(api.calls[0].Body, &submitted))
	msp, ok := submitted["modelSpecificPayload"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "16:9", msp["aspectRatio"])
	size, ok := submitted["size"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(2048), size["width"])
	require.Equal(t, float64(2048), size["height"])
}

// gpt-image 不走 aspectRatio 字段，比例只能靠 Gemini 路径推导出的 WxH 带过去。
func TestAdobeImageServiceGeminiAspectRatioReachesGPTImagePixels(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("X"))
	svc := newAdobeTestService(t, client, nil)

	req, call, err := ParseAdobeGeminiImageRequest("gpt-image-2", []byte(`{
		"contents": [{"parts": [{"text": "x"}]}],
		"generationConfig": {"imageConfig": {"aspectRatio": "16:9", "imageSize": "2K"}}
	}`))
	require.NoError(t, err)
	require.Equal(t, "2048x1152", req.Size)

	result, err := svc.GenerateCall(context.Background(), adobeTestAccount(), "tok", call)
	require.NoError(t, err)
	require.Equal(t, "2K", result.Forward.ImageSize)

	bodies := adobeSubmitBodies(t, api)
	require.NotEmpty(t, bodies)
	size, ok := bodies[0]["size"].(map[string]any)
	require.True(t, ok, "gpt-image payload must carry top-level size")
	require.Equal(t, float64(2048), size["width"])
	require.Equal(t, float64(1152), size["height"])
}

// TestAdobeImageServiceSizeDrivesBillingTier 是本仓库唯一能把「出图档位」和「计费档位」
// 焊在一起的地方。
//
// internal/pkg/adobe 对 sub2api 内部零依赖，没法直接复用 ClassifyImageBillingTier，
// 于是 adobe.ResolutionFromSize 是它的并行实现。两边的长边阈值一旦错开，同一个 size
// 就会「出 4K 的图、按 2K 计费」——没有任何其它信号会报警，只能靠这条测试。
func TestAdobeImageServiceSizeDrivesBillingTier(t *testing.T) {
	sizes := []string{
		"512x512", "1024x1024", "1025x1024", "1792x1024",
		"2048x2048", "2048x1152", "2049x100", "3840x2160", "2160x3840",
	}
	for _, size := range sizes {
		t.Run(size, func(t *testing.T) {
			api := &adobeFakeTransport{}
			client := adobeSubmitPollDownload(t, api, []byte("X"))
			svc := newAdobeTestService(t, client, nil)

			result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
				Model: "gpt-image-2", Prompt: "x", Size: size, N: 1,
			})
			require.NoError(t, err)

			wantTier, ok := ClassifyImageBillingTier(size)
			require.True(t, ok, "size %s 应能被计费侧识别", size)
			require.Equal(t, wantTier, result.Forward.ImageSize,
				"出图档位与 ClassifyImageBillingTier 必须一致")
		})
	}
}

// gpt-image-2.5 必须把请求像素原样写进顶层 size，并按同一长边计费。
// 旧实现不发顶层 size、却按请求 4K 计费，会在低档套餐被上游拒绝时仍然错账。
func TestAdobeImageServiceGPTImage25SendsRequestedPixelsAndBillsThem(t *testing.T) {
	cases := []struct {
		size     string
		width    int
		height   int
		wantTier string
	}{
		{"1024x1024", 1024, 1024, "1K"},
		{"1024x1536", 1024, 1536, "2K"},
		{"2048x1152", 2048, 1152, "2K"},
		{"3840x2160", 3840, 2160, "4K"},
	}
	for _, tc := range cases {
		t.Run(tc.size, func(t *testing.T) {
			api := &adobeFakeTransport{}
			client := adobeSubmitPollDownload(t, api, []byte("X"))
			svc := newAdobeTestService(t, client, nil)

			result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
				Model: "gpt-image-2.5-flare", Prompt: "x", Size: tc.size, N: 1,
			})
			require.NoError(t, err)
			require.Equal(t, tc.wantTier, result.Forward.ImageSize)
			require.Equal(t, tc.wantTier, NormalizeImageBillingTierOrDefault(result.Forward.ImageSize))

			require.NotEmpty(t, api.calls)
			var submitted map[string]any
			require.NoError(t, json.Unmarshal(api.calls[0].Body, &submitted))
			size, ok := submitted["size"].(map[string]any)
			require.True(t, ok, "v2.5 有 WxH 时必须发顶层 size")
			require.Equal(t, float64(tc.width), size["width"])
			require.Equal(t, float64(tc.height), size["height"])
			require.NotContains(t, submitted, "outputResolution")
			msp, ok := submitted["modelSpecificPayload"].(map[string]any)
			require.True(t, ok)
			require.NotContains(t, msp, "size")
		})
	}
}

func TestAdobeImageServiceGPTImage25AutoOmitsTopLevelSize(t *testing.T) {
	for _, size := range []string{"", "auto"} {
		t.Run(size, func(t *testing.T) {
			api := &adobeFakeTransport{}
			client := adobeSubmitPollDownload(t, api, []byte("X"))
			svc := newAdobeTestService(t, client, nil)

			result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
				Model: "gpt-image-2.5-flare", Prompt: "x", Size: size, N: 1,
			})
			require.NoError(t, err)
			require.Equal(t, "2K", result.Forward.ImageSize)

			require.NotEmpty(t, api.calls)
			var submitted map[string]any
			require.NoError(t, json.Unmarshal(api.calls[0].Body, &submitted))
			require.NotContains(t, submitted, "size")
			require.NotContains(t, submitted, "outputResolution")
			msp := submitted["modelSpecificPayload"].(map[string]any)
			require.Equal(t, "auto", msp["size"])
		})
	}
}

// 省略 size 的请求不受长边推导影响，仍走默认 2K —— OpenAI 客户端不传 size 是常态，
// 这条守的是「本轮改动没有悄悄挪动默认路径」。
func TestAdobeImageServiceMissingSizeKeepsDefaultTier(t *testing.T) {
	for _, size := range []string{"", "auto"} {
		api := &adobeFakeTransport{}
		client := adobeSubmitPollDownload(t, api, []byte("X"))
		svc := newAdobeTestService(t, client, nil)

		result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
			Model: "gpt-image-2", Prompt: "x", Size: size, N: 1,
		})
		require.NoError(t, err, size)
		require.Equal(t, "2K", result.Forward.ImageSize, size)
		require.Equal(t, "firefly-gpt-image-2", result.Forward.UpstreamModel, size)

		var submitted map[string]any
		require.NoError(t, json.Unmarshal(api.calls[0].Body, &submitted))
		require.NotContains(t, submitted, "size")
		msp := submitted["modelSpecificPayload"].(map[string]any)
		require.Equal(t, "auto", msp["size"])
	}
}

// 缺口 A 的服务层回归：这些 size 在旧实现里全部静默出方图。
func TestAdobeImageServiceForwardsGPTImage2Pixels(t *testing.T) {
	tests := map[string]adobe.Size{
		"3840x2160": {3840, 2160},
		"2160x3840": {2160, 3840},
		"2048x1152": {2048, 1152},
		"1536x1024": {1536, 1024},
		"1024x1536": {1024, 1536},
		"1152x928":  {1152, 928},
	}
	for size, want := range tests {
		t.Run(size, func(t *testing.T) {
			api := &adobeFakeTransport{}
			client := adobeSubmitPollDownload(t, api, []byte("X"))
			svc := newAdobeTestService(t, client, nil)

			result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
				Model: "gpt-image-2", Prompt: "x", Size: size, N: 1,
			})
			require.NoError(t, err)
			require.Equal(t, "firefly-gpt-image-2", result.Forward.UpstreamModel)

			require.NotEmpty(t, api.calls)
			var submitted map[string]any
			require.NoError(t, json.Unmarshal(api.calls[0].Body, &submitted))
			got := submitted["size"].(map[string]any)
			require.Equal(t, float64(want.Width), got["width"])
			require.Equal(t, float64(want.Height), got["height"])
			require.NotContains(t, submitted, "outputResolution")
			msp := submitted["modelSpecificPayload"].(map[string]any)
			require.NotContains(t, msp, "size")
		})
	}
}

func TestAdobeImageServiceBananaSquareTierAndAspectRatio(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("X"))
	svc := newAdobeTestService(t, client, nil)

	result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model: "nano-banana2", Prompt: "x", Size: "1536x1024", N: 1,
	})
	require.NoError(t, err)
	require.Equal(t, "2K", result.Forward.ImageSize)
	require.Equal(t, "firefly-nano-banana2-2k-3x2", result.Forward.UpstreamModel)

	var submitted map[string]any
	require.NoError(t, json.Unmarshal(api.calls[0].Body, &submitted))
	size := submitted["size"].(map[string]any)
	require.Equal(t, float64(2048), size["width"])
	require.Equal(t, float64(2048), size["height"])
	msp := submitted["modelSpecificPayload"].(map[string]any)
	require.Equal(t, "3:2", msp["aspectRatio"])
	require.Equal(t, float64(2), submitted["caiClaimVersion"])
	require.NotContains(t, submitted, "skipCai")
}

func TestAdobeImageServiceBananaEmptySizeIs1K(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("X"))
	svc := newAdobeTestService(t, client, nil)

	result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model: "nano-banana2", Prompt: "x", N: 1,
	})
	require.NoError(t, err)
	require.Equal(t, "1K", result.Forward.ImageSize)

	var submitted map[string]any
	require.NoError(t, json.Unmarshal(api.calls[0].Body, &submitted))
	size := submitted["size"].(map[string]any)
	require.Equal(t, float64(1024), size["width"])
	require.Equal(t, float64(1024), size["height"])
	msp := submitted["modelSpecificPayload"].(map[string]any)
	require.NotContains(t, msp, "aspectRatio")
}

func TestAdobeImageServiceReturnsB64WithoutStorage(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("PNGDATA"))
	svc := newAdobeTestService(t, client, nil)

	result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "x", Size: "1024x1024", N: 1,
	})
	require.NoError(t, err)

	var payload struct {
		Created int64 `json:"created"`
		Data    []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(result.Body, &payload))
	require.Positive(t, payload.Created)
	require.Len(t, payload.Data, 1)
	require.Empty(t, payload.Data[0].URL)
	decoded, err := base64.StdEncoding.DecodeString(payload.Data[0].B64JSON)
	require.NoError(t, err)
	require.Equal(t, []byte("PNGDATA"), decoded)
}

// 假的对象存储，只记录被存了什么。
type adobeFakeStorage struct {
	saved map[string][]byte
}

func (s *adobeFakeStorage) Save(_ context.Context, key, _ string, data []byte) (string, error) {
	if s.saved == nil {
		s.saved = map[string][]byte{}
	}
	s.saved[key] = data
	return "https://cdn.example/" + key, nil
}

func TestAdobeImageServiceReturnsURLWithStorage(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("PNGDATA"))
	storage := &adobeFakeStorage{}
	uploader := NewImageResultUploader(storage, "adobe/", 0, nil)
	svc := newAdobeTestService(t, client, func() (*ImageResultUploader, bool) { return uploader, true })

	result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "x", Size: "1024x1024", N: 1,
	})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(result.Body, &payload))
	items := payload["data"].([]any)
	first := items[0].(map[string]any)
	require.Contains(t, first["url"], "https://cdn.example/adobe/")
	// 转存后必须不再带 b64，否则响应体白白翻倍。
	require.NotContains(t, first, "b64_json")
	require.Len(t, storage.saved, 1)
}

// 对象存储挂了不该让已经生成好（且已扣上游额度）的图丢掉。
func TestAdobeImageServiceFallsBackToB64WhenStorageFails(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("PNGDATA"))
	uploader := NewImageResultUploader(&adobeFailingStorage{}, "adobe/", 0, nil)
	svc := newAdobeTestService(t, client, func() (*ImageResultUploader, bool) { return uploader, true })

	result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "x", Size: "1024x1024", N: 1,
	})
	require.NoError(t, err)
	require.Contains(t, string(result.Body), "b64_json")
}

type adobeFailingStorage struct{}

func (*adobeFailingStorage) Save(context.Context, string, string, []byte) (string, error) {
	return "", http.ErrHandlerTimeout
}

func TestAdobeImageServiceFansOutN(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("PNGDATA"))
	svc := newAdobeTestService(t, client, nil)

	result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "x", Size: "1024x1024", N: 2,
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.Forward.ImageCount)

	var payload struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(result.Body, &payload))
	require.Len(t, payload.Data, 2)
	for _, item := range payload.Data {
		decoded, err := base64.StdEncoding.DecodeString(item.B64JSON)
		require.NoError(t, err)
		require.Equal(t, []byte("PNGDATA"), decoded)
	}

	submitted := adobeSubmitBodies(t, api)
	require.Len(t, submitted, 2)
	seeds := make([]int, 0, 2)
	for _, body := range submitted {
		require.Equal(t, float64(1), body["n"])
		rawSeeds, ok := body["seeds"].([]any)
		require.True(t, ok)
		require.Len(t, rawSeeds, 1)
		seeds = append(seeds, int(rawSeeds[0].(float64)))
	}
	require.NotEqual(t, seeds[0], seeds[1])
	delta := seeds[0] - seeds[1]
	if delta < 0 {
		delta = -delta
	}
	require.Equal(t, 1, delta)
}

func TestAdobeImageServiceTreatsNonPositiveNAsOne(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("PNGDATA"))
	svc := newAdobeTestService(t, client, nil)

	result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "x", Size: "1024x1024", N: 0,
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.Forward.ImageCount)
	require.Len(t, adobeSubmitBodies(t, api), 1)
}

func TestAdobeImageServiceFansOutNSharesUploadedSource(t *testing.T) {
	var uploads int
	api := &adobeFakeTransport{}
	api.handler = func(req *adobe.Request, _ int) (*adobe.Response, error) {
		switch req.URL {
		case adobe.ImageUploadURL:
			uploads++
			require.Equal(t, []byte("SRC"), req.Body)
			return adobeJSONResponse(t, 200, map[string]any{
				"images": []any{map[string]any{"id": "img-1"}},
			}, nil), nil
		case adobe.ImageSubmitURL:
			return adobeJSONResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		default:
			return adobeJSONResponse(t, 200, map[string]any{
				"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": "https://cdn/y.png"}}},
			}, nil), nil
		}
	}
	download := &adobeFakeTransport{handler: func(*adobe.Request, int) (*adobe.Response, error) {
		return &adobe.Response{StatusCode: 200, Headers: map[string]string{}, Body: []byte("Y")}, nil
	}}
	client := adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: download})
	svc := newAdobeTestService(t, client, nil)

	result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model:    "gpt-image-2",
		Prompt:   "edit",
		Size:     "1024x1024",
		N:        2,
		Endpoint: openAIImagesEditsEndpoint,
		Uploads:  []OpenAIImagesUpload{{Data: []byte("SRC"), ContentType: "image/png"}},
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.Forward.ImageCount)
	require.Equal(t, 1, uploads)

	submitted := adobeSubmitBodies(t, api)
	require.Len(t, submitted, 2)
	for _, body := range submitted {
		require.Equal(t, []any{map[string]any{"id": "img-1", "usage": "subject"}}, body["referenceBlobs"])
	}
}

func TestAdobeImageServiceRejectsNAboveMax(t *testing.T) {
	api := &adobeFakeTransport{handler: func(*adobe.Request, int) (*adobe.Response, error) {
		t.Fatal("不应发起上游请求")
		return nil, nil
	}}
	client := adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: api})
	svc := newAdobeTestService(t, client, nil)

	_, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "x", N: 11,
	})
	require.ErrorContains(t, err, "n must be between 1 and 10")
	require.Equal(t, NextAccountStop, classifyAdobeError(err).Failover.NextAccountAction)
}

func TestAdobeImageServiceRejectsWebPOutputFormat(t *testing.T) {
	api := &adobeFakeTransport{handler: func(*adobe.Request, int) (*adobe.Response, error) {
		t.Fatal("不应发起上游请求")
		return nil, nil
	}}
	client := adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: api})
	svc := newAdobeTestService(t, client, nil)

	for _, format := range []string{"webp", "WEBP"} {
		_, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
			Model: "gpt-image-2", Prompt: "x", N: 1, OutputFormat: format,
		})
		require.ErrorContains(t, err, "output_format=")
		require.Equal(t, NextAccountStop, classifyAdobeError(err).Failover.NextAccountAction)
	}
}

func TestAdobeImageServiceRejectsTransparentJPEG(t *testing.T) {
	api := &adobeFakeTransport{handler: func(*adobe.Request, int) (*adobe.Response, error) {
		t.Fatal("不应发起上游请求")
		return nil, nil
	}}
	client := adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: api})
	svc := newAdobeTestService(t, client, nil)

	for _, tc := range []struct {
		background string
		format     string
	}{
		{background: "transparent", format: "jpeg"},
		{background: "TRANSPARENT", format: "jpg"},
		{background: " Transparent ", format: "JPEG"},
	} {
		_, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
			Model: "gpt-image-2", Prompt: "x", N: 1, Background: tc.background, OutputFormat: tc.format,
		})
		require.ErrorContains(t, err, "background=transparent")
		require.Equal(t, NextAccountStop, classifyAdobeError(err).Failover.NextAccountAction)
	}
}

// background 是 Step 1 里生产实测过的字段，必须真的透传到上游 payload。
func TestAdobeImageServicePassesBackgroundThrough(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("X"))
	svc := newAdobeTestService(t, client, nil)

	_, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "x", Size: "1024x1024", N: 1, Background: "transparent",
	})
	require.NoError(t, err)

	var submitted map[string]any
	require.NoError(t, json.Unmarshal(api.calls[0].Body, &submitted))
	msp := submitted["modelSpecificPayload"].(map[string]any)
	require.Equal(t, "transparent", msp["background"])
}

func TestAdobeImageServiceUploadsSourceImages(t *testing.T) {
	api := &adobeFakeTransport{}
	api.handler = func(req *adobe.Request, index int) (*adobe.Response, error) {
		if index == 0 {
			require.Equal(t, adobe.ImageUploadURL, req.URL)
			require.Equal(t, []byte("SRC"), req.Body)
			return adobeJSONResponse(t, 200, map[string]any{
				"images": []any{map[string]any{"id": "img-1"}},
			}, nil), nil
		}
		if index == 1 {
			return adobeJSONResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		}
		return adobeJSONResponse(t, 200, map[string]any{
			"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": "https://cdn/y.png"}}},
		}, nil), nil
	}
	download := &adobeFakeTransport{handler: func(*adobe.Request, int) (*adobe.Response, error) {
		return &adobe.Response{StatusCode: 200, Headers: map[string]string{}, Body: []byte("Y")}, nil
	}}
	client := adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: download})
	svc := newAdobeTestService(t, client, nil)

	_, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model:    "gpt-image-2",
		Prompt:   "edit",
		Size:     "1024x1024",
		N:        1,
		Endpoint: openAIImagesEditsEndpoint,
		Uploads:  []OpenAIImagesUpload{{Data: []byte("SRC"), ContentType: "image/png"}},
	})
	require.NoError(t, err)

	// 提交体应带上上传拿到的 id，且 gpt-image 家族的 usage 必须是 subject。
	var submitted map[string]any
	require.NoError(t, json.Unmarshal(api.calls[1].Body, &submitted))
	require.Equal(t, []any{map[string]any{"id": "img-1", "usage": "subject"}}, submitted["referenceBlobs"])
	meta := submitted["generationMetadata"].(map[string]any)
	require.Equal(t, "text2image", meta["module"])
	require.Equal(t, "ff-image-editor", meta["submodule"])
}

func TestAdobeImageServiceGenerationsWithSourceUsesGenerateSubmodule(t *testing.T) {
	api := &adobeFakeTransport{}
	api.handler = func(req *adobe.Request, index int) (*adobe.Response, error) {
		if index == 0 {
			require.Equal(t, adobe.ImageUploadURL, req.URL)
			return adobeJSONResponse(t, 200, map[string]any{
				"images": []any{map[string]any{"id": "img-1"}},
			}, nil), nil
		}
		if index == 1 {
			return adobeJSONResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		}
		return adobeJSONResponse(t, 200, map[string]any{
			"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": "https://cdn/y.png"}}},
		}, nil), nil
	}
	download := &adobeFakeTransport{handler: func(*adobe.Request, int) (*adobe.Response, error) {
		return &adobe.Response{StatusCode: 200, Headers: map[string]string{}, Body: []byte("Y")}, nil
	}}
	client := adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: download})
	svc := newAdobeTestService(t, client, nil)

	_, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model:    "gpt-image-2",
		Prompt:   "edit",
		Size:     "1024x1024",
		N:        1,
		Endpoint: openAIImagesGenerationsEndpoint,
		Uploads:  []OpenAIImagesUpload{{Data: []byte("SRC"), ContentType: "image/png"}},
	})
	require.NoError(t, err)

	var submitted map[string]any
	require.NoError(t, json.Unmarshal(api.calls[1].Body, &submitted))
	meta := submitted["generationMetadata"].(map[string]any)
	require.Equal(t, "text2image", meta["module"])
	require.Equal(t, "ff-image-generate", meta["submodule"])
}

func TestAdobeImageServiceDoesNotUploadMask(t *testing.T) {
	api := &adobeFakeTransport{}
	api.handler = func(req *adobe.Request, index int) (*adobe.Response, error) {
		if index == 0 {
			require.Equal(t, adobe.ImageUploadURL, req.URL)
			require.Equal(t, []byte("SRC"), req.Body)
			return adobeJSONResponse(t, 200, map[string]any{
				"images": []any{map[string]any{"id": "img-1"}},
			}, nil), nil
		}
		if index == 1 {
			return adobeJSONResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		}
		return adobeJSONResponse(t, 200, map[string]any{
			"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": "https://cdn/y.png"}}},
		}, nil), nil
	}
	download := &adobeFakeTransport{handler: func(*adobe.Request, int) (*adobe.Response, error) {
		return &adobe.Response{StatusCode: 200, Headers: map[string]string{}, Body: []byte("Y")}, nil
	}}
	client := adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: download})
	svc := newAdobeTestService(t, client, nil)

	_, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model:    "gpt-image-2",
		Prompt:   "edit",
		Size:     "1024x1024",
		N:        1,
		Endpoint: openAIImagesEditsEndpoint,
		HasMask:  true,
		Uploads:  []OpenAIImagesUpload{{Data: []byte("SRC"), ContentType: "image/png"}},
		MaskUpload: &OpenAIImagesUpload{
			FieldName:   "mask",
			FileName:    "mask.png",
			ContentType: "image/png",
			Data:        []byte("MASK"),
		},
	})
	require.NoError(t, err)

	uploadCount := 0
	for _, call := range api.calls {
		if call.URL == adobe.ImageUploadURL {
			uploadCount++
			require.NotEqual(t, []byte("MASK"), call.Body)
		}
	}
	require.Equal(t, 1, uploadCount)

	var submitted map[string]any
	require.NoError(t, json.Unmarshal(api.calls[1].Body, &submitted))
	require.Equal(t, []any{map[string]any{"id": "img-1", "usage": "subject"}}, submitted["referenceBlobs"])
}

func TestAdobeImageServiceUnknownModel(t *testing.T) {
	api := &adobeFakeTransport{handler: func(*adobe.Request, int) (*adobe.Response, error) {
		t.Fatal("不应发起上游请求")
		return nil, nil
	}}
	client := adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: api})
	svc := newAdobeTestService(t, client, nil)

	account := adobeTestAccount()
	// 显式清空 mapping 让未知模型直达解析器。
	account.Credentials = map[string]any{"model_mapping": map[string]any{"weird": "weird"}}
	_, err := svc.Generate(context.Background(), account, "tok", &OpenAIImagesRequest{
		Model: "weird", Prompt: "x", N: 1,
	})
	require.ErrorContains(t, err, "unknown firefly image model")
}

func TestAdobeImageServiceEmptyToken(t *testing.T) {
	svc := newAdobeTestService(t, adobe.NewClient(adobe.ClientConfig{}), nil)
	_, err := svc.Generate(context.Background(), adobeTestAccount(), "  ", &OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "x", N: 1,
	})
	require.ErrorContains(t, err, "access token is empty")
	// token 缺失应触发换号 + 刷新，而不是直接失败。
	require.Equal(t, NextAccountRetry, classifyAdobeError(err).Failover.NextAccountAction)
}

func TestAdobeImageServiceEmptyDownloadIsRotatable(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, nil)
	svc := newAdobeTestService(t, client, nil)

	result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model:  "gpt-image-2",
		Prompt: "a cat",
		Size:   "1024x1024",
		N:      1,
	})
	require.Nil(t, result)
	require.Error(t, err)
	require.True(t, adobe.IsRotatable(err), "空结果应换号重试")
}

// adobeCtxRecordingTransport 在 adobeFakeTransport 基础上记录每次调用时 ctx 是否已取消。
type adobeCtxRecordingTransport struct {
	adobeFakeTransport
	ctxErrs []error
}

func (t *adobeCtxRecordingTransport) Do(ctx context.Context, req *adobe.Request) (*adobe.Response, error) {
	t.ctxErrs = append(t.ctxErrs, ctx.Err())
	return t.adobeFakeTransport.Do(ctx, req)
}

// 提交之后客户端断开：轮询与下载必须继续，产物交回 handler 记账。
func TestAdobeImageServiceDetachesAfterSubmit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api := &adobeCtxRecordingTransport{}
	api.handler = func(_ *adobe.Request, index int) (*adobe.Response, error) {
		if index == 0 {
			cancel() // 提交已被上游受理后客户端离开
			return adobeJSONResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		}
		return adobeJSONResponse(t, 200, map[string]any{
			"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": "https://cdn.example.com/img.png"}}},
		}, nil), nil
	}
	download := &adobeCtxRecordingTransport{}
	download.handler = func(*adobe.Request, int) (*adobe.Response, error) {
		return &adobe.Response{StatusCode: 200, Headers: map[string]string{}, Body: []byte("PNG")}, nil
	}
	client := adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: download})
	svc := newAdobeTestService(t, client, nil)

	result, err := svc.Generate(ctx, adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "a cat", Size: "1024x1024", N: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.Forward.ImageCount)
	require.Len(t, api.ctxErrs, 2)
	require.NoError(t, api.ctxErrs[1], "poll must not inherit the client's cancellation")
	require.Len(t, download.ctxErrs, 1)
	require.NoError(t, download.ctxErrs[0], "download must not inherit the client's cancellation")
}

// 提交前客户端已离开：不打上游，不花 credits。
func TestAdobeImageServiceSkipsSubmitWhenClientGone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	api := &adobeFakeTransport{handler: func(*adobe.Request, int) (*adobe.Response, error) {
		t.Error("submit must not be sent after the client left")
		return nil, context.Canceled
	}}
	svc := newAdobeTestService(t, adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: api}), nil)

	_, err := svc.Generate(ctx, adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "a cat", Size: "1024x1024", N: 1,
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, api.calls)
}

// adobeCountingRoundTripper 统计输入图抓取次数，并返回固定 PNG。
type adobeCountingRoundTripper struct {
	calls int
}

func (rt *adobeCountingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.calls++
	png := []byte("\x89PNG\r\n\x1a\n0000")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"image/png"}},
		Body:       io.NopCloser(bytes.NewReader(png)),
		Request:    req,
	}, nil
}

// 同一请求换号重试时只抓一次输入图 URL，每个账号各自重新上传。
func TestAdobeImageCallFetchesInputImagesOnce(t *testing.T) {
	uploads := 0
	api := &adobeFakeTransport{}
	api.handler = func(req *adobe.Request, _ int) (*adobe.Response, error) {
		switch req.URL {
		case adobe.ImageUploadURL:
			uploads++
			return adobeJSONResponse(t, 200, map[string]any{
				"images": []any{map[string]any{"id": "img"}},
			}, nil), nil
		case adobe.ImageSubmitURL:
			// 400 不在同号内重试，避免测试等待提交退避。
			return adobeJSONResponse(t, 400, map[string]any{}, nil), nil
		}
		t.Fatalf("unexpected url %s", req.URL)
		return nil, nil
	}
	client := adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: api})
	svc := newAdobeTestService(t, client, nil)
	rt := &adobeCountingRoundTripper{}
	svc.inputClient = &http.Client{Transport: rt}
	call := NewAdobeImageCall(&OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "edit", Size: "1024x1024", N: 1,
		Endpoint:       openAIImagesEditsEndpoint,
		InputImageURLs: []string{"https://images.example.com/a.png"},
	}, "")

	for i := 0; i < 2; i++ {
		_, err := svc.GenerateCall(context.Background(), adobeTestAccount(), "tok", call)
		require.Error(t, err)
	}
	require.Equal(t, 1, rt.calls, "input image url must be fetched once per client request")
	require.Equal(t, 2, uploads, "each attempt uploads the cached bytes again")
}

// 渠道映射后的模型参与账号映射与计费模型。
func TestAdobeImageCallUsesChannelMappedModel(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("PNG"))
	svc := newAdobeTestService(t, client, nil)

	result, err := svc.GenerateCall(context.Background(), adobeTestAccount(), "tok", NewAdobeImageCall(&OpenAIImagesRequest{
		Model: "my-alias", Prompt: "a cat", Size: "1024x1024", N: 1,
	}, "nano-banana-pro"))
	require.NoError(t, err)
	require.Equal(t, "nano-banana-pro", result.Forward.Model)
	require.Contains(t, result.Forward.UpstreamModel, "nano-banana-pro")
}
