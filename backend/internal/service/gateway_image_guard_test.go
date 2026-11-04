//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func guardTestImage(t *testing.T, w, h int) map[string]any {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h))))
	return map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": base64.StdEncoding.EncodeToString(b.Bytes())}}
}

func guardTestBody(t *testing.T, blocks ...any) []byte {
	t.Helper()
	b, e := json.Marshal(map[string]any{"model": "claude-sonnet-4-6", "max_tokens": 1024, "messages": []any{map[string]any{"role": "user", "content": blocks}}})
	require.NoError(t, e)
	return b
}

func guardAssertDimensions(t *testing.T, body []byte, path string, w, h int) {
	t.Helper()
	d, e := base64.StdEncoding.DecodeString(gjson.GetBytes(body, path+".source.data").String())
	require.NoError(t, e)
	im, _, e := image.Decode(bytes.NewReader(d))
	require.NoError(t, e)
	require.Equal(t, w, im.Bounds().Dx())
	require.Equal(t, h, im.Bounds().Dy())
}

func TestAnthropicImageGuardDimensions(t *testing.T) {
	for _, tc := range []struct {
		name             string
		w, h, outW, outH int
	}{
		{"boundary", 3000, 100, 3000, 100}, {"one_over", 3001, 100, 3000, 99},
		{"incident", 4130, 1348, 3000, 979}, {"portrait", 100, 4130, 72, 3000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := guardTestBody(t, guardTestImage(t, tc.w, tc.h))
			orig := bytes.Clone(b)
			out, e := normalizeAnthropicImages(context.Background(), b)
			require.NoError(t, e)
			require.Equal(t, orig, b, "input is immutable")
			guardAssertDimensions(t, out, "messages.0.content.0", tc.outW, tc.outH)
			if tc.w <= 3000 && tc.h <= 3000 {
				require.Equal(t, b, out)
			}
		})
	}
}

func TestAnthropicImageGuardHistoricalToolResult(t *testing.T) {
	small := guardTestImage(t, 1, 1)
	blocks := make([]any, 20)
	for i := range blocks {
		blocks[i] = small
	}
	b := guardTestBody(t, blocks...)
	big := guardTestImage(t, 3104, 100)
	signed := json.RawMessage(`{"role":"assistant","content":[{"type":"thinking","thinking":"preserve","signature":"opaque-signature"},{"type":"tool_use","id":"tool1","name":"read","input":{"type":"image","source":{"type":"url","url":"do-not-touch"}}}]}`)
	messages := gjson.GetBytes(b, "messages").Array()
	b, e := json.Marshal(map[string]any{"model": "claude-sonnet-4-6", "messages": []any{json.RawMessage(messages[0].Raw), signed, map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "tool1", "content": []any{big}}}}}})
	require.NoError(t, e)
	oldSigned := gjson.GetBytes(b, "messages.1").Raw
	out, e := normalizeAnthropicImages(context.Background(), b)
	require.NoError(t, e)
	require.Len(t, anthropicImagePaths(out), 21)
	guardAssertDimensions(t, out, "messages.2.content.0.content.0", 3000, 96)
	require.Equal(t, oldSigned, gjson.GetBytes(out, "messages.1").Raw)
	require.Equal(t, "tool1", gjson.GetBytes(out, "messages.2.content.0.tool_use_id").String())
}

func TestAnthropicImageGuardRejectsUnsafeSources(t *testing.T) {
	b := guardTestBody(t, guardTestImage(t, 1, 1))
	for _, tc := range []struct {
		name, path string
		value      any
		want       string
	}{
		{"base64", "data", "not-base64!", "invalid base64"}, {"mime", "media_type", "image/jpeg", "media_type"},
		{"remote", "type", "url", "remote/file"},
		{"bytes", "data", strings.Repeat("A", base64.StdEncoding.EncodedLen(anthropicImageMaxBytes)+1), "decode budget"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid, e := sjson.SetBytes(b, "messages.0.content.0.source."+tc.path, tc.value)
			require.NoError(t, e)
			out, e := normalizeAnthropicImages(context.Background(), invalid)
			require.ErrorContains(t, e, tc.want)
			require.Nil(t, out)
			require.Contains(t, e.Error(), "messages.0.content.0.source")
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := normalizeAnthropicImages(ctx, b)
	require.ErrorIs(t, e, context.Canceled)
}

func TestAnthropicImageGuardPixelBudget(t *testing.T) {
	b := guardTestBody(t, guardTestImage(t, 4001, 4000))
	out, e := normalizeAnthropicImages(context.Background(), b)
	require.ErrorContains(t, e, "pixel budget")
	require.Nil(t, out)
}

func TestAnthropicImageGuardDisabledAndTextUnchanged(t *testing.T) {
	body := []byte(`{ "model":"claude-sonnet-4-6", "messages":[{"role":"user","content":"hello"}], "metadata":{"id":9007199254740993} }`)
	out, e := normalizeAnthropicImages(context.Background(), body)
	require.NoError(t, e)
	require.Equal(t, body, out)
	invalid := []byte(`{"messages":[{"content":[{"type":"image","source":{"type":"url","url":"https://example.invalid"}}]}]}`)
	for _, cfg := range []*config.Config{nil, {}} {
		out, e := guardAnthropicImages(context.Background(), nil, cfg, invalid)
		require.NoError(t, e)
		require.Equal(t, invalid, out)
	}
}

func TestAnthropicImageGuardWireBuilders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{AnthropicImageGuardEnabled: true}}
	gateway := &GatewayService{cfg: cfg}
	native := &OpenAIGatewayService{cfg: cfg}
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}}
	builders := map[string]func(context.Context, *gin.Context, []byte) (*http.Request, []byte, error){
		"native_and_converted": func(ctx context.Context, c *gin.Context, b []byte) (*http.Request, []byte, error) {
			return gateway.buildUpstreamRequest(ctx, c, account, b, "test", "apikey", "claude-sonnet-4-6", false, false)
		},
		"passthrough": func(ctx context.Context, c *gin.Context, b []byte) (*http.Request, []byte, error) {
			return gateway.buildUpstreamRequestAnthropicAPIKeyPassthrough(ctx, c, account, b, "test")
		},
		"openai_native": func(ctx context.Context, c *gin.Context, b []byte) (*http.Request, []byte, error) {
			return native.buildNativeAnthropicUpstreamRequest(ctx, c, account, b, "test", "https://api.anthropic.com/v1/messages")
		},
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			b := guardTestBody(t, guardTestImage(t, 4130, 100))
			req, wire, e := build(context.Background(), c, b)
			require.NoError(t, e)
			require.NotNil(t, req)
			sent, e := io.ReadAll(req.Body)
			require.NoError(t, e)
			require.Equal(t, wire, sent)
			require.Equal(t, int64(len(sent)), req.ContentLength)
			guardAssertDimensions(t, sent, "messages.0.content.0", 3000, 72)
			invalid := guardTestBody(t, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "invalid"}})
			req, _, e = build(context.Background(), c, invalid)
			require.Error(t, e)
			require.Nil(t, req)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
		})
	}
}

func TestAnthropicImageGuardForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, invalid := range []bool{false, true} {
		t.Run(strconv.FormatBool(invalid), func(t *testing.T) {
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"msg_test","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":1}}`))}}
			svc := newForwardPartialUsageServiceForTest(upstream)
			svc.cfg.Gateway.AnthropicImageGuardEnabled = true
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			b := guardTestBody(t, guardTestImage(t, 4130, 100))
			if invalid {
				var e error
				b, e = sjson.SetBytes(b, "messages.0.content.0.source.data", "bad!")
				require.NoError(t, e)
			}
			parsed, e := ParseGatewayRequest(NewRequestBodyRef(b), PlatformAnthropic)
			require.NoError(t, e)
			result, e := svc.Forward(context.Background(), c, newAnthropicOAuthAccountForPartialUsageTest(), parsed)
			if invalid {
				require.Error(t, e)
				require.Nil(t, result)
				require.Nil(t, upstream.lastReq)
				require.Equal(t, 400, rec.Code)
				return
			}
			require.NoError(t, e)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			guardAssertDimensions(t, upstream.lastBody, "messages.0.content.0", 3000, 72)
			require.Equal(t, 200, rec.Code)
			require.Contains(t, rec.Body.String(), "ok")
		})
	}
}

func TestAnthropicImageGuardTransparencyAndOutputBudget(t *testing.T) {
	b := guardTestBody(t, guardTestImage(t, 3001, 10))
	out, e := normalizeAnthropicImages(context.Background(), b)
	require.NoError(t, e)
	d, e := base64.StdEncoding.DecodeString(gjson.GetBytes(out, "messages.0.content.0.source.data").String())
	require.NoError(t, e)
	im, e := png.Decode(bytes.NewReader(d))
	require.NoError(t, e)
	_, _, _, a := im.At(0, 0).RGBA()
	require.Zero(t, a)
	buf := &imageGuardBuffer{remaining: 3}
	_, e = buf.Write([]byte("four"))
	require.ErrorIs(t, e, io.ErrShortWrite)
	require.Zero(t, buf.Len())
}

func TestAnthropicImageGuardJPEGAndGIF(t *testing.T) {
	for _, format := range []string{"jpeg", "gif", "jpeg_exif"} {
		t.Run(format, func(t *testing.T) {
			var buf bytes.Buffer
			im := image.NewRGBA(image.Rect(0, 0, 3001, 10))
			mime := "image/jpeg"
			if format == "gif" {
				require.NoError(t, gif.Encode(&buf, im, nil))
				mime = "image/gif"
			} else {
				require.NoError(t, jpeg.Encode(&buf, im, nil))
			}
			data := buf.Bytes()
			if format == "jpeg_exif" {
				data = append(append(append([]byte{}, data[:2]...), []byte{0xff, 0xe1, 0, 8, 'E', 'x', 'i', 'f', 0, 0}...), data[2:]...)
			}
			b := guardTestBody(t, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": mime, "data": base64.StdEncoding.EncodeToString(data)}})
			out, e := normalizeAnthropicImages(context.Background(), b)
			if format == "jpeg_exif" {
				require.ErrorContains(t, e, "EXIF")
				return
			}
			require.NoError(t, e)
			guardAssertDimensions(t, out, "messages.0.content.0", 3000, 9)
			if format == "gif" {
				mime = "image/png"
			}
			require.Equal(t, mime, gjson.GetBytes(out, "messages.0.content.0.source.media_type").String())
		})
	}
}

func TestAnthropicImageGuardRequestPixelBudgetIsAtomic(t *testing.T) {
	large := guardTestImage(t, 4000, 2700)
	body := guardTestBody(t, large, large, large)
	original := bytes.Clone(body)
	out, err := normalizeAnthropicImages(context.Background(), body)
	require.ErrorContains(t, err, "messages.0.content.2.source: resize pixel budget")
	require.Nil(t, out, "do not return a partially repaired request")
	require.Equal(t, original, body)
}
