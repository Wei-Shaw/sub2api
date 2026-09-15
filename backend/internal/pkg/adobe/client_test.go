//go:build unit

package adobe

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	submitRetryWait = 0
	os.Exit(m.Run())
}

// fakeTransport 按调用序号返回预置响应，并记录收到的请求。
type fakeTransport struct {
	handler func(req *Request, index int) (*Response, error)
	calls   []*Request
}

func (t *fakeTransport) Do(_ context.Context, req *Request) (*Response, error) {
	index := len(t.calls)
	// 记录请求的副本：调用方可能复用 header map。
	t.calls = append(t.calls, req)
	return t.handler(req, index)
}

func (t *fakeTransport) urls() []string {
	out := make([]string, 0, len(t.calls))
	for _, call := range t.calls {
		out = append(out, call.URL)
	}
	return out
}

func jsonResponse(t *testing.T, status int, body any, headers map[string]string) *Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	if headers == nil {
		headers = map[string]string{}
	}
	return &Response{StatusCode: status, Headers: headers, Body: raw}
}

func bytesResponse(status int, data []byte) *Response {
	return &Response{StatusCode: status, Headers: map[string]string{}, Body: data}
}

// fakeToken 造一个 claims 里带 user_id 的 token。
func fakeToken(t *testing.T) string {
	t.Helper()
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	return enc(`{"alg":"none"}`) + "." + enc(`{"user_id":"u1"}`) + ".sig"
}

func testClient(api, download Transport) *Client {
	return NewClient(ClientConfig{Transport: api, DownloadTransport: download})
}

func TestExtractResultLink(t *testing.T) {
	t.Run("优先响应头", func(t *testing.T) {
		require.Equal(t, "https://firefly-3p.ff.adobe.io/jobs/1", ExtractResultLink(
			map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/1"},
			map[string]any{"links": map[string]any{"result": "https://firefly-3p.ff.adobe.io/jobs/2"}},
		))
	})

	t.Run("回落 body.links.result", func(t *testing.T) {
		require.Equal(t, "https://firefly-3p.ff.adobe.io/jobs/2", ExtractResultLink(nil,
			map[string]any{"links": map[string]any{"result": "https://firefly-3p.ff.adobe.io/jobs/2"}}))
		require.Equal(t, "https://firefly-3p.ff.adobe.io/jobs/3", ExtractResultLink(nil,
			map[string]any{"links": map[string]any{"result": map[string]any{"href": "https://firefly-3p.ff.adobe.io/jobs/3"}}}))
	})

	t.Run("都没有返回空", func(t *testing.T) {
		require.Empty(t, ExtractResultLink(map[string]string{}, map[string]any{}))
		require.Empty(t, ExtractResultLink(nil, map[string]any{"links": "not-an-object"}))
	})
}

func TestNormalizeVideoPollURL(t *testing.T) {
	require.Equal(t,
		"https://bks-epo1234.adobe.io/v2/jobs/result/video-job-1?host=firefly-epo1234-prod.adobe.io/",
		NormalizeVideoPollURL("https://firefly-epo1234-prod.adobe.io/v2/jobs/video-job-1"))

	// 普通地址、分片号不是四位数字、非法 URL 都原样返回。
	for _, raw := range []string{
		"https://poll.example/jobs/1",
		"https://firefly-epoabcd.adobe.io/jobs/1",
		"not a url",
		"",
	} {
		require.Equal(t, raw, NormalizeVideoPollURL(raw), "raw=%q", raw)
	}
}

func TestGenerateImageRoundTrip(t *testing.T) {
	api := &fakeTransport{}
	api.handler = func(req *Request, index int) (*Response, error) {
		if index == 0 {
			require.Contains(t, req.URL, "/v2/3p-images/generate-async")
			require.Equal(t, "clio-playground-web", req.Headers["x-api-key"])
			require.Equal(t, DefaultIdentity.Origin, req.Headers["origin"])
			require.Equal(t, DefaultIdentity.Referer, req.Headers["referer"])
			require.Equal(t, "cross-site", req.Headers["sec-fetch-site"])
			require.NotEmpty(t, req.Headers["x-arp-session-id"])
			token := strings.TrimPrefix(req.Headers["authorization"], "Bearer ")
			require.Equal(t, BuildSubmitNonce(token, "a cat"), req.Headers["x-nonce"])
			return jsonResponse(t, 200,
				map[string]any{"links": map[string]any{"result": "https://firefly-3p.ff.adobe.io/jobs/abc"}},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/abc"}), nil
		}
		require.Equal(t, "clio-playground-web", req.Headers["x-api-key"])
		require.Equal(t, "application/json", req.Headers["content-type"])
		require.Equal(t, DefaultIdentity.Origin, req.Headers["origin"])
		return jsonResponse(t, 200, map[string]any{
			"status":  "COMPLETED",
			"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": "https://cdn/img.png"}}},
		}, nil), nil
	}
	download := &fakeTransport{handler: func(*Request, int) (*Response, error) {
		return bytesResponse(200, []byte("PNGDATA")), nil
	}}

	out, err := testClient(api, download).GenerateImage(context.Background(), GenerateImageInput{
		Token: fakeToken(t),
		Options: ImagePayloadOptions{
			Prompt:               "a cat",
			AspectRatio:          "16:9",
			OutputResolution:     Resolution2K,
			UpstreamModelID:      "gpt-image",
			UpstreamModelVersion: "2",
		},
		PollInterval: time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, []byte("PNGDATA"), out.Bytes)
	require.Equal(t, "COMPLETED", out.Raw["status"])
	require.Equal(t, "https://cdn/img.png", download.calls[0].URL)
}

// 产物下载必须走独立的传输（不带 TLS 伪装），不能复用 API 传输。
func TestGenerateImageUsesDownloadTransport(t *testing.T) {
	api := &fakeTransport{handler: func(req *Request, index int) (*Response, error) {
		if index == 0 {
			return jsonResponse(t, 200, map[string]any{}, map[string]string{
				"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/abc",
			}), nil
		}
		return jsonResponse(t, 200, map[string]any{
			"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": "https://cdn/img.png"}}},
		}, nil), nil
	}}
	download := &fakeTransport{handler: func(*Request, int) (*Response, error) {
		return bytesResponse(200, []byte("X")), nil
	}}

	_, err := testClient(api, download).GenerateImage(context.Background(), GenerateImageInput{
		Token:        fakeToken(t),
		Options:      ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		PollInterval: time.Millisecond,
	})
	require.NoError(t, err)
	require.Len(t, download.calls, 1)
	require.NotContains(t, strings.Join(api.urls(), " "), "cdn")
}

func TestGenerateImageAuthErrors(t *testing.T) {
	generate := func(t *testing.T, resp *Response) error {
		t.Helper()
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) { return resp, nil }}
		_, err := testClient(api, nil).GenerateImage(context.Background(), GenerateImageInput{
			Token:   fakeToken(t),
			Options: ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		})
		return err
	}

	t.Run("taste_exhausted 归为配额耗尽", func(t *testing.T) {
		err := generate(t, jsonResponse(t, 401, map[string]any{},
			map[string]string{"x-access-error": "taste_exhausted"}))
		var quota *QuotaExhaustedError
		require.True(t, errors.As(err, &quota))
		// 配额耗尽应冷却账号而非刷新凭据，故不能同时是 AuthError。
		var auth *AuthError
		require.False(t, errors.As(err, &auth))
		require.True(t, IsRotatable(err))
	})

	t.Run("普通 401 归为鉴权失效", func(t *testing.T) {
		err := generate(t, jsonResponse(t, 401, map[string]any{}, nil))
		var auth *AuthError
		require.True(t, errors.As(err, &auth))
	})

	t.Run("403 空 body 仍按鉴权处理", func(t *testing.T) {
		err := generate(t, jsonResponse(t, 403, map[string]any{}, nil))
		var auth *AuthError
		require.True(t, errors.As(err, &auth))
		var entitled *NotEntitledError
		require.False(t, errors.As(err, &entitled))
	})

	t.Run("403 model_not_entitled 是权益不足", func(t *testing.T) {
		err := generate(t, jsonResponse(t, 403, map[string]any{
			"error_code": "model_not_entitled",
		}, nil))
		var entitled *NotEntitledError
		require.True(t, errors.As(err, &entitled))
		var auth *AuthError
		require.False(t, errors.As(err, &auth), "权益不足不能当成 token 失效")
		require.True(t, IsRotatable(err))
	})

	t.Run("403 user_not_entitled 同样是权益不足", func(t *testing.T) {
		err := generate(t, jsonResponse(t, 403, map[string]any{
			"error_code": "user_not_entitled",
		}, nil))
		var entitled *NotEntitledError
		require.True(t, errors.As(err, &entitled))
		var auth *AuthError
		require.False(t, errors.As(err, &auth))
	})

	t.Run("x-access-error 带 model_not_entitled 也算权益不足", func(t *testing.T) {
		err := generate(t, jsonResponse(t, 403, map[string]any{},
			map[string]string{"x-access-error": "model_not_entitled"}))
		var entitled *NotEntitledError
		require.True(t, errors.As(err, &entitled))
		var auth *AuthError
		require.False(t, errors.As(err, &auth))
	})
}

// 401/403 是凭据问题，必须立刻中断而不是继续试下一个 payload 候选。
func TestGenerateImageStopsCandidatesOnAuthError(t *testing.T) {
	api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
		return jsonResponse(t, 401, map[string]any{}, nil), nil
	}}
	_, err := testClient(api, nil).GenerateImage(context.Background(), GenerateImageInput{
		Token:   fakeToken(t),
		Options: ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
	})
	require.Error(t, err)
	require.Len(t, api.calls, 1)
}

func TestGenerateImageRetriesRetryableSubmit(t *testing.T) {
	t.Run("408 之后 200 则成功", func(t *testing.T) {
		api := &fakeTransport{handler: func(_ *Request, index int) (*Response, error) {
			if index == 0 {
				return jsonResponse(t, 408, map[string]any{
					"error_code": "timeout_error", "message": "system under load",
				}, nil), nil
			}
			if index == 1 {
				return jsonResponse(t, 200, map[string]any{}, map[string]string{
					"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/abc",
				}), nil
			}
			return jsonResponse(t, 200, map[string]any{
				"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": "https://cdn/img.png"}}},
			}, nil), nil
		}}
		download := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return bytesResponse(200, []byte("X")), nil
		}}
		out, err := testClient(api, download).GenerateImage(context.Background(), GenerateImageInput{
			Token:        fakeToken(t),
			Options:      ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
			PollInterval: time.Millisecond,
		})
		require.NoError(t, err)
		require.Equal(t, []byte("X"), out.Bytes)
		require.Equal(t, ImageSubmitURL, api.calls[0].URL)
		require.Equal(t, ImageSubmitURL, api.calls[1].URL)
	})

	t.Run("连续 408 耗尽重试后仍是临时故障", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return jsonResponse(t, 408, map[string]any{
				"error_code": "timeout_error", "message": "system under load",
			}, nil), nil
		}}
		_, err := testClient(api, nil).GenerateImage(context.Background(), GenerateImageInput{
			Token:   fakeToken(t),
			Options: ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		})
		var temporary *UpstreamTemporaryError
		require.True(t, errors.As(err, &temporary))
		require.Equal(t, 408, temporary.StatusCode)
		require.Len(t, api.calls, submitAttempts)
	})

	t.Run("连续 429 同号重试到上限", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return jsonResponse(t, 429, map[string]any{"message": "too many requests"}, nil), nil
		}}
		_, err := testClient(api, nil).GenerateImage(context.Background(), GenerateImageInput{
			Token:   fakeToken(t),
			Options: ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		})
		var temporary *UpstreamTemporaryError
		require.True(t, errors.As(err, &temporary))
		require.Len(t, api.calls, submitAttempts)
	})

	// 5xx/451 可能是上游已受理、中间层回错：同号重发会叠加重复的付费任务，只提交一次交给 handler 换号。
	for _, status := range []int{451, 500, 502, 503, 504} {
		t.Run(fmt.Sprintf("%d 不在同号重试", status), func(t *testing.T) {
			api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
				return &Response{StatusCode: status, Headers: map[string]string{}, Body: []byte("upstream down")}, nil
			}}
			_, err := testClient(api, nil).GenerateImage(context.Background(), GenerateImageInput{
				Token:   fakeToken(t),
				Options: ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
			})
			var temporary *UpstreamTemporaryError
			require.True(t, errors.As(err, &temporary))
			require.Equal(t, status, temporary.StatusCode)
			require.True(t, IsRotatable(err))
			require.Len(t, api.calls, 1)
		})
	}
}

func TestGenerateImageUpstreamErrors(t *testing.T) {
	t.Run("5xx 归为可重试的临时故障", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return &Response{StatusCode: 503, Headers: map[string]string{}, Body: []byte("upstream down")}, nil
		}}
		_, err := testClient(api, nil).GenerateImage(context.Background(), GenerateImageInput{
			Token:   fakeToken(t),
			Options: ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		})
		var temporary *UpstreamTemporaryError
		require.True(t, errors.As(err, &temporary))
		require.Equal(t, 503, temporary.StatusCode)
		require.True(t, IsRotatable(err))
	})

	t.Run("400 归为终态错误", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return &Response{StatusCode: 400, Headers: map[string]string{}, Body: []byte("bad prompt")}, nil
		}}
		_, err := testClient(api, nil).GenerateImage(context.Background(), GenerateImageInput{
			Token:   fakeToken(t),
			Options: ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "bad prompt")
		require.False(t, IsRotatable(err), "请求本身有问题，换号也无用")
	})

	t.Run("提交成功但没给轮询地址", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return jsonResponse(t, 200, map[string]any{}, nil), nil
		}}
		_, err := testClient(api, nil).GenerateImage(context.Background(), GenerateImageInput{
			Token:   fakeToken(t),
			Options: ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		})
		require.ErrorContains(t, err, "no poll url returned")
	})
}

func TestGenerateImageEditSubmitsOnce(t *testing.T) {
	api := &fakeTransport{handler: func(req *Request, _ int) (*Response, error) {
		if strings.Contains(req.URL, "generate-async") {
			return jsonResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		}
		return jsonResponse(t, 200, map[string]any{
			"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": "https://cdn/y.png"}}},
		}, nil), nil
	}}
	download := &fakeTransport{handler: func(*Request, int) (*Response, error) {
		return bytesResponse(200, []byte("Y")), nil
	}}

	out, err := testClient(api, download).GenerateImage(context.Background(), GenerateImageInput{
		Token: fakeToken(t),
		Options: ImagePayloadOptions{
			Prompt: "edit", AspectRatio: "1:1", UpstreamModelID: "gpt-image",
			SourceImageIDs: []string{"img1"},
		},
		PollInterval: time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, []byte("Y"), out.Bytes)

	// gpt-image 图生图只有一个候选，一次 submit 即可。
	submits := 0
	for _, call := range api.calls {
		if strings.Contains(call.URL, "generate-async") {
			submits++
		}
	}
	require.Equal(t, 1, submits)
}

func TestGenerateImagePollFailure(t *testing.T) {
	api := &fakeTransport{handler: func(_ *Request, index int) (*Response, error) {
		if index == 0 {
			return jsonResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		}
		return jsonResponse(t, 200, map[string]any{"status": "FAILED"}, nil), nil
	}}
	_, err := testClient(api, nil).GenerateImage(context.Background(), GenerateImageInput{
		Token:        fakeToken(t),
		Options:      ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		PollInterval: time.Millisecond,
	})
	require.ErrorContains(t, err, "image job failed")
}

// 任务状态也可能只出现在响应头里。
func TestGenerateImagePollFailureFromHeader(t *testing.T) {
	api := &fakeTransport{handler: func(_ *Request, index int) (*Response, error) {
		if index == 0 {
			return jsonResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		}
		return jsonResponse(t, 200, map[string]any{}, map[string]string{"x-task-status": "cancelled"}), nil
	}}
	_, err := testClient(api, nil).GenerateImage(context.Background(), GenerateImageInput{
		Token:        fakeToken(t),
		Options:      ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		PollInterval: time.Millisecond,
	})
	require.ErrorContains(t, err, "image job failed")
}

func TestGenerateImagePollTimeout(t *testing.T) {
	api := &fakeTransport{handler: func(_ *Request, index int) (*Response, error) {
		if index == 0 {
			return jsonResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		}
		return jsonResponse(t, 200, map[string]any{"status": "RUNNING"}, nil), nil
	}}
	_, err := testClient(api, nil).GenerateImage(context.Background(), GenerateImageInput{
		Token:        fakeToken(t),
		Options:      ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		Timeout:      time.Nanosecond,
		PollInterval: time.Millisecond,
	})
	require.ErrorContains(t, err, "image generation timed out")
}

// 轮询可能先回 202（仍在运行），再回 201 携带结果；两者都不能被当成错误或空结果。
func TestGenerateImagePollAcceptsAcceptedThenCreated(t *testing.T) {
	api := &fakeTransport{handler: func(_ *Request, index int) (*Response, error) {
		switch index {
		case 0:
			return jsonResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		case 1:
			return jsonResponse(t, 202, map[string]any{}, nil), nil
		default:
			return jsonResponse(t, 201, map[string]any{
				"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": "https://cdn/z.png"}}},
			}, nil), nil
		}
	}}
	download := &fakeTransport{handler: func(*Request, int) (*Response, error) {
		return bytesResponse(200, []byte("Z")), nil
	}}
	out, err := testClient(api, download).GenerateImage(context.Background(), GenerateImageInput{
		Token:        fakeToken(t),
		Options:      ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		PollInterval: time.Millisecond,
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, []byte("Z"), out.Bytes)
}

// 201 但没有结果时继续轮询直到超时，而不是返回 (nil, nil)。
func TestGenerateImagePollCreatedWithoutResultTimesOut(t *testing.T) {
	api := &fakeTransport{handler: func(_ *Request, index int) (*Response, error) {
		if index == 0 {
			return jsonResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		}
		return jsonResponse(t, 201, map[string]any{}, nil), nil
	}}
	out, err := testClient(api, nil).GenerateImage(context.Background(), GenerateImageInput{
		Token:        fakeToken(t),
		Options:      ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		Timeout:      time.Nanosecond,
		PollInterval: time.Millisecond,
	})
	require.Nil(t, out)
	require.ErrorContains(t, err, "image generation timed out")
}

// ctx 取消要能中断轮询循环，否则调用方断开后请求还在后台空转。
func TestGenerateImagePollRespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	api := &fakeTransport{handler: func(_ *Request, index int) (*Response, error) {
		if index == 0 {
			return jsonResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://firefly-3p.ff.adobe.io/jobs/x"}), nil
		}
		cancel()
		return jsonResponse(t, 200, map[string]any{"status": "RUNNING"}, nil), nil
	}}
	_, err := testClient(api, nil).GenerateImage(ctx, GenerateImageInput{
		Token:        fakeToken(t),
		Options:      ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		PollInterval: time.Hour,
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestGenerateVideoNormalizesPollURL(t *testing.T) {
	api := &fakeTransport{handler: func(req *Request, index int) (*Response, error) {
		if index == 0 {
			return jsonResponse(t, 200, map[string]any{}, map[string]string{
				"x-override-status-link": "https://firefly-epo5678-prod.adobe.io/jobs/video-job-2",
			}), nil
		}
		require.Equal(t,
			"https://bks-epo5678.adobe.io/v2/jobs/result/video-job-2?host=firefly-epo5678-prod.adobe.io/",
			req.URL)
		return jsonResponse(t, 200, map[string]any{
			"status":  "COMPLETED",
			"outputs": []any{map[string]any{"video": map[string]any{"presignedUrl": "https://cdn/video.mp4"}}},
		}, nil), nil
	}}
	download := &fakeTransport{handler: func(*Request, int) (*Response, error) {
		return bytesResponse(200, []byte("MP4DATA")), nil
	}}

	conf, ok := ResolveVideoModel("firefly-sora2-4s-16x9")
	require.True(t, ok)
	opts := VideoPayloadOptionsFor(conf)
	opts.Prompt = "test video"

	out, err := testClient(api, download).GenerateVideo(context.Background(), GenerateVideoInput{
		Token:        fakeToken(t),
		Options:      opts,
		PollInterval: time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, []byte("MP4DATA"), out.Bytes)
	require.Equal(t, "https://cdn/video.mp4", download.calls[0].URL)
}

func TestUploadImage(t *testing.T) {
	t.Run("返回 image id", func(t *testing.T) {
		api := &fakeTransport{handler: func(req *Request, _ int) (*Response, error) {
			require.Equal(t, ImageUploadURL, req.URL)
			require.Equal(t, "image/png", req.Headers["content-type"])
			require.Equal(t, "clio-playground-web", req.Headers["x-api-key"])
			require.Equal(t, DefaultIdentity.Origin, req.Headers["origin"])
			require.Equal(t, []byte("RAW"), req.Body)
			return jsonResponse(t, 200, map[string]any{
				"images": []any{map[string]any{"id": "img-123"}},
			}, nil), nil
		}}
		id, err := testClient(api, nil).UploadImage(context.Background(), fakeToken(t), []byte("RAW"), "image/png")
		require.NoError(t, err)
		require.Equal(t, "img-123", id)
	})

	t.Run("空图直接报错，不打上游", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			t.Fatal("不应发起请求")
			return nil, nil
		}}
		_, err := testClient(api, nil).UploadImage(context.Background(), fakeToken(t), nil, "image/png")
		require.ErrorContains(t, err, "image is empty")
	})

	t.Run("上游没返回 id", func(t *testing.T) {
		api := &fakeTransport{handler: func(*Request, int) (*Response, error) {
			return jsonResponse(t, 200, map[string]any{"images": []any{}}, nil), nil
		}}
		_, err := testClient(api, nil).UploadImage(context.Background(), fakeToken(t), []byte("R"), "")
		require.ErrorContains(t, err, "no image id returned")
	})
}

// pollClient 造一个「提交成功后按 pollResponses 顺序应答轮询」的客户端。
func pollClient(t *testing.T, pollURL string, poll func(index int) (*Response, error)) (*Client, *fakeTransport, *fakeTransport) {
	t.Helper()
	api := &fakeTransport{handler: func(_ *Request, index int) (*Response, error) {
		if index == 0 {
			return jsonResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": pollURL}), nil
		}
		return poll(index - 1)
	}}
	download := &fakeTransport{handler: func(*Request, int) (*Response, error) {
		return bytesResponse(200, []byte("IMG")), nil
	}}
	return testClient(api, download), api, download
}

func completedWith(t *testing.T, presigned string) *Response {
	t.Helper()
	return jsonResponse(t, 200, map[string]any{
		"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": presigned}}},
	}, nil)
}

func generateTestImage(client *Client, t *testing.T) (*GenerateResult, error) {
	t.Helper()
	return client.GenerateImage(context.Background(), GenerateImageInput{
		Token:        fakeToken(t),
		Options:      ImagePayloadOptions{Prompt: "x", AspectRatio: "1:1", UpstreamModelID: "gpt-image"},
		PollInterval: time.Millisecond,
	})
}

// 任务已提交、credits 已在消耗：轮询偶发的 5xx / 网络错误不能让整个任务作废。
func TestGenerateImagePollToleratesTransientFailures(t *testing.T) {
	client, api, _ := pollClient(t, "https://firefly-3p.ff.adobe.io/jobs/x", func(index int) (*Response, error) {
		switch index {
		case 0:
			return jsonResponse(t, 503, map[string]any{}, nil), nil
		case 1:
			return nil, NewUpstreamTemporaryError("connection reset", 0, ErrorTypeConnection)
		default:
			return completedWith(t, "https://cdn.example.com/img.png"), nil
		}
	})

	result, err := generateTestImage(client, t)
	require.NoError(t, err)
	require.Equal(t, []byte("IMG"), result.Bytes)
	require.Len(t, api.calls, 4, "submit + 2 failed polls + 1 successful poll")
}

func TestGenerateImagePollGivesUpAfterConsecutiveFailures(t *testing.T) {
	client, api, _ := pollClient(t, "https://firefly-3p.ff.adobe.io/jobs/x", func(int) (*Response, error) {
		return jsonResponse(t, 503, map[string]any{}, nil), nil
	})

	_, err := generateTestImage(client, t)
	var temporary *UpstreamTemporaryError
	require.True(t, errors.As(err, &temporary))
	require.Len(t, api.calls, 1+maxConsecutivePollFailures+1)
}

// 轮询鉴权失败不是临时故障，立即返回。
func TestGenerateImagePollDoesNotRetryAuthFailure(t *testing.T) {
	client, api, _ := pollClient(t, "https://firefly-3p.ff.adobe.io/jobs/x", func(int) (*Response, error) {
		return jsonResponse(t, 401, map[string]any{}, nil), nil
	})

	_, err := generateTestImage(client, t)
	var auth *AuthError
	require.True(t, errors.As(err, &auth))
	require.Len(t, api.calls, 2)
}

// 轮询链接来自上游响应且会带 Bearer token：非 adobe.io 主机一律拒绝，不发请求。
func TestGenerateImageRejectsNonAdobePollURL(t *testing.T) {
	for _, link := range []string{
		"https://attacker.example.com/jobs/x",
		"http://firefly-3p.ff.adobe.io/jobs/x",
		"https://adobe.io.attacker.example/jobs/x",
		"https://evil-adobe.io/jobs/x",
	} {
		t.Run(link, func(t *testing.T) {
			client, api, _ := pollClient(t, link, func(int) (*Response, error) {
				t.Error("poll must not be sent to a non-adobe host")
				return nil, errors.New("unexpected")
			})
			_, err := generateTestImage(client, t)
			require.ErrorContains(t, err, "adobe api url")
			require.Len(t, api.calls, 1, "only the submit request is sent")
		})
	}
}

// 产物直链只接受 https 域名：IP 字面量、localhost、http 都拒绝，不发下载请求。
func TestGenerateImageRejectsUnsafeMediaURL(t *testing.T) {
	for _, link := range []string{
		"http://cdn.example.com/img.png",
		"https://169.254.169.254/latest/meta-data",
		"https://[::1]/img.png",
		"https://localhost/img.png",
	} {
		t.Run(link, func(t *testing.T) {
			client, _, download := pollClient(t, "https://firefly-3p.ff.adobe.io/jobs/x", func(int) (*Response, error) {
				return completedWith(t, link), nil
			})
			_, err := generateTestImage(client, t)
			require.ErrorContains(t, err, "media url")
			require.Empty(t, download.calls)
		})
	}
}

func TestGenerateImageCapsImageDownloadSize(t *testing.T) {
	client, _, download := pollClient(t, "https://firefly-3p.ff.adobe.io/jobs/x", func(int) (*Response, error) {
		return completedWith(t, "https://cdn.example.com/img.png"), nil
	})

	_, err := generateTestImage(client, t)
	require.NoError(t, err)
	require.Len(t, download.calls, 1)
	require.Equal(t, MaxImageDownloadBytes, download.calls[0].MaxBodyBytes)
}

func TestNormalizeVideoPollURLRequiresAdobeHost(t *testing.T) {
	raw := "https://firefly-epo1234.attacker.example/v2/jobs/job-1"
	require.Equal(t, raw, NormalizeVideoPollURL(raw), "non adobe.io host must not be rewritten into an adobe host")
}

// 上游 4xx 与 401 的原始 body 只进日志，对外文案固定。
func TestUpstreamErrorBodiesStayOutOfUserMessages(t *testing.T) {
	requestErr := classifyAdobeHTTPError(400, `{"internal_trace":"abc"}`, `submit failed: 400 {"internal_trace":"abc"}`)
	var reqErr *RequestError
	require.True(t, errors.As(requestErr, &reqErr))
	require.Contains(t, reqErr.Error(), "internal_trace", "log message keeps the upstream body")
	require.NotContains(t, reqErr.User(), "internal_trace")
	require.Contains(t, reqErr.User(), "HTTP 400")

	authErr := authOrQuotaError(jsonResponse(t, 401, map[string]any{"trace": "secret-trace"}, nil))
	var auth *AuthError
	require.True(t, errors.As(authErr, &auth))
	require.Contains(t, auth.Error(), "secret-trace")
	require.NotContains(t, auth.User(), "secret-trace")
}
