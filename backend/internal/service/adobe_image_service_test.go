//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/stretchr/testify/require"
)

// adobeFakeTransport 按调用序号返回预置响应，并记录收到的请求。
type adobeFakeTransport struct {
	handler func(req *adobe.Request, index int) (*adobe.Response, error)
	calls   []*adobe.Request
}

func (t *adobeFakeTransport) Do(_ context.Context, req *adobe.Request) (*adobe.Response, error) {
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

// adobeSubmitPollDownload 模拟一次成功的「提交 → 轮询 → 下载」。
func adobeSubmitPollDownload(t *testing.T, api *adobeFakeTransport, imageBytes []byte) *adobe.Client {
	t.Helper()
	api.handler = func(_ *adobe.Request, index int) (*adobe.Response, error) {
		if index == 0 {
			return adobeJSONResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://poll/x"}), nil
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

	// Model 记客户端请求名，UpstreamModel 记实际打到 Adobe 的全量 id。
	require.Equal(t, "gpt-image-2", result.Forward.Model)
	require.Equal(t, "firefly-gpt-image-2-1k-1x1", result.Forward.UpstreamModel)
	require.NotEmpty(t, result.Forward.RequestID)
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
			require.NotContains(t, msp, "size")
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
		require.Equal(t, "firefly-gpt-image-2-2k-1x1", result.Forward.UpstreamModel, size)
	}
}

// 缺口 A 的服务层回归：这些 size 在旧实现里全部静默出方图。
func TestAdobeImageServiceDerivesAspectRatioFromSize(t *testing.T) {
	tests := map[string]string{
		"3840x2160": "firefly-gpt-image-2-4k-16x9",
		"2160x3840": "firefly-gpt-image-2-4k-9x16",
		"2048x1152": "firefly-gpt-image-2-2k-16x9",
		"1536x1024": "firefly-gpt-image-2-2k-3x2",
		"1024x1536": "firefly-gpt-image-2-2k-2x3",
		"2016x864":  "firefly-gpt-image-2-2k-21x9",
	}
	for size, wantModel := range tests {
		t.Run(size, func(t *testing.T) {
			api := &adobeFakeTransport{}
			client := adobeSubmitPollDownload(t, api, []byte("X"))
			svc := newAdobeTestService(t, client, nil)

			result, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
				Model: "gpt-image-2", Prompt: "x", Size: size, N: 1,
			})
			require.NoError(t, err)
			require.Equal(t, wantModel, result.Forward.UpstreamModel)
		})
	}
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

func TestAdobeImageServiceRejectsMultipleImages(t *testing.T) {
	api := &adobeFakeTransport{handler: func(*adobe.Request, int) (*adobe.Response, error) {
		t.Fatal("不应发起上游请求")
		return nil, nil
	}}
	client := adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: api})
	svc := newAdobeTestService(t, client, nil)

	_, err := svc.Generate(context.Background(), adobeTestAccount(), "tok", &OpenAIImagesRequest{
		Model: "gpt-image-2", Prompt: "x", N: 2,
	})
	require.ErrorContains(t, err, "one image per request")
	// 参数错误换号也无用。
	require.Equal(t, NextAccountStop, classifyAdobeError(err).Failover.NextAccountAction)
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
				map[string]string{"x-override-status-link": "https://poll/x"}), nil
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
		Model: "gpt-image-2", Prompt: "edit", Size: "1024x1024", N: 1,
		Uploads: []OpenAIImagesUpload{{Data: []byte("SRC"), ContentType: "image/png"}},
	})
	require.NoError(t, err)

	// 提交体应带上上传拿到的 id，且 gpt-image 家族的 usage 必须是 subject。
	var submitted map[string]any
	require.NoError(t, json.Unmarshal(api.calls[1].Body, &submitted))
	require.Equal(t, []any{map[string]any{"id": "img-1", "usage": "subject"}}, submitted["referenceBlobs"])
	require.Equal(t, "image2image", submitted["generationMetadata"].(map[string]any)["module"])
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
