package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIsOpenAIImageURLUnsupportedError(t *testing.T) {
	serdeBody := []byte(`{"error":{"message":"Error from provider (Console Go): Upstream request failed: [invalid_request_error] Failed to deserialize the JSON body into the target type: messages[139]: unknown variant ` + "`image_url`" + `, expected ` + "`text`" + ` at line 1 column 310907","type":"invalid_request_error","code":"invalid_request_error"}}`)

	require.True(t, isOpenAIImageURLUnsupportedError(http.StatusBadRequest, "", serdeBody))
	require.True(t, isOpenAIImageURLUnsupportedError(http.StatusBadRequest, "unknown variant `image_url`, expected `text`", nil))
	require.False(t, isOpenAIImageURLUnsupportedError(http.StatusBadGateway, "", serdeBody))
	require.False(t, isOpenAIImageURLUnsupportedError(http.StatusBadRequest, "unknown variant `tool`, expected `text`", nil))
	require.False(t, isOpenAIImageURLUnsupportedError(http.StatusBadRequest, "invalid request body", nil))
}

func TestTransformOpenAIImageInputBodyStripsChatImages(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "看看这张图"},
				{"type": "image_url", "image_url": {"url": "https://example.com/a.png"}},
				{"type": "image_url", "image_url": {"url": "data:image/png;base64,AAA"}}
			]},
			{"role": "user", "content": [
				{"type": "image_url", "image_url": {"url": "https://example.com/b.png"}}
			]},
			{"role": "user", "content": "纯文本消息"}
		]
	}`)

	retryBody, reason, changed, err := transformOpenAIImageInputBody(body, openAIImageInputFallbackModeStrip, nil)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "strip unsupported image content", reason)

	require.Equal(t, int64(1), gjson.GetBytes(retryBody, "messages.0.content.#").Int())
	require.Equal(t, "text", gjson.GetBytes(retryBody, "messages.0.content.0.type").String())
	require.False(t, gjson.GetBytes(retryBody, "messages.0.content.1").Exists())
	// 只有图片的消息被替换为占位文本字符串。
	require.Equal(t, openAIImageInputFallbackPlaceholderText, gjson.GetBytes(retryBody, "messages.1.content").String())
	require.Equal(t, "纯文本消息", gjson.GetBytes(retryBody, "messages.2.content").String())
}

func TestTransformOpenAIImageInputBodyStripsResponsesImages(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.5",
		"input": [
			{"type": "message", "role": "user", "content": [
				{"type": "input_text", "text": "描述图片"},
				{"type": "input_image", "image_url": "https://example.com/c.png"}
			]},
			{"type": "message", "role": "user", "content": [
				{"type": "input_image", "image_url": "https://example.com/d.png"}
			]}
		]
	}`)

	retryBody, _, changed, err := transformOpenAIImageInputBody(body, openAIImageInputFallbackModeStrip, nil)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, int64(1), gjson.GetBytes(retryBody, "input.0.content.#").Int())
	require.Equal(t, "input_text", gjson.GetBytes(retryBody, "input.0.content.0.type").String())
	require.Equal(t, "input_text", gjson.GetBytes(retryBody, "input.1.content.0.type").String())
	require.Equal(t, openAIImageInputFallbackPlaceholderText, gjson.GetBytes(retryBody, "input.1.content.0.text").String())
}

func TestTransformOpenAIImageInputBodyNoImages(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`)
	retryBody, _, changed, err := transformOpenAIImageInputBody(body, openAIImageInputFallbackModeStrip, nil)
	require.NoError(t, err)
	require.False(t, changed)
	require.Nil(t, retryBody)
}

func TestTransformOpenAIImageInputBodyDescribeMode(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": [
				{"type": "image_url", "image_url": {"url": "https://example.com/a.png"}}
			]}
		]
	}`)
	describeAll := func(images []map[string]any) ([]string, error) {
		require.Len(t, images, 1)
		return []string{"一只橘猫坐在窗台上"}, nil
	}
	retryBody, reason, changed, err := transformOpenAIImageInputBody(body, openAIImageInputFallbackModeDescribe, describeAll)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "replace image content with vision model description", reason)
	require.Equal(t, "text", gjson.GetBytes(retryBody, "messages.0.content.0.type").String())
	require.Equal(t, "一只橘猫坐在窗台上", gjson.GetBytes(retryBody, "messages.0.content.0.text").String())
}

func TestTransformOpenAIImageInputBodyDescribeFailsFallsBackToStrip(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": [
				{"type": "image_url", "image_url": {"url": "https://example.com/a.png"}}
			]}
		]
	}`)
	describeAll := func(images []map[string]any) ([]string, error) {
		return nil, http.ErrServerClosed
	}
	retryBody, reason, changed, err := transformOpenAIImageInputBody(body, openAIImageInputFallbackModeDescribe, describeAll)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "strip unsupported image content", reason)
	require.Equal(t, openAIImageInputFallbackPlaceholderText, gjson.GetBytes(retryBody, "messages.0.content").String())
}

func TestNormalizeOpenAIImageVisionPart(t *testing.T) {
	chatPart := map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}}
	part, ok := normalizeOpenAIImageVisionPart(chatPart)
	require.True(t, ok)
	require.Equal(t, "https://example.com/a.png", gjson.GetBytes(mustImageInputJSON(t, part), "image_url.url").String())

	responsesPart := map[string]any{"type": "input_image", "image_url": "https://example.com/b.png"}
	part, ok = normalizeOpenAIImageVisionPart(responsesPart)
	require.True(t, ok)
	require.Equal(t, "image_url", gjson.GetBytes(mustImageInputJSON(t, part), "type").String())
	require.Equal(t, "https://example.com/b.png", gjson.GetBytes(mustImageInputJSON(t, part), "image_url.url").String())

	unknown := map[string]any{"type": "text", "text": "hello"}
	_, ok = normalizeOpenAIImageVisionPart(unknown)
	require.False(t, ok)
}

func TestCallOpenAIVisionDescribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"一张海边日落的照片"}}]}`))
	}))
	defer srv.Close()

	desc, err := callOpenAIVisionDescribe(context.Background(), srv.URL, "sk-test", "gpt-4o-mini",
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}},
		0,
	)
	require.NoError(t, err)
	require.Equal(t, "一张海边日落的照片", desc)
}

func TestOpenAIImageInputFallbackMode(t *testing.T) {
	require.Equal(t, openAIImageInputFallbackModeOff, openAIImageInputFallbackMode(nil))
	cfg := &config.Config{}
	require.Equal(t, openAIImageInputFallbackModeOff, openAIImageInputFallbackMode(cfg))
	cfg.Gateway.ImageInputFallback = "STRIP"
	require.Equal(t, openAIImageInputFallbackModeStrip, openAIImageInputFallbackMode(cfg))
	cfg.Gateway.ImageInputFallback = "describe"
	require.Equal(t, openAIImageInputFallbackModeDescribe, openAIImageInputFallbackMode(cfg))
}

func mustImageInputJSON(t *testing.T, v any) []byte {
	t.Helper()
	out, err := marshalOpenAIUpstreamJSON(v)
	require.NoError(t, err)
	return out
}

func TestModelMatchesImageInputFallbackModels(t *testing.T) {
	require.True(t, modelMatchesImageInputFallbackModels("gpt-4", "gpt-4"))
	require.True(t, modelMatchesImageInputFallbackModels("gpt-4o", "gpt-4, gpt-3.5"))
	require.True(t, modelMatchesImageInputFallbackModels("GPT-4-Turbo", "gpt-4"))
	require.False(t, modelMatchesImageInputFallbackModels("gpt-4o", "gpt-3.5"))
	require.False(t, modelMatchesImageInputFallbackModels("", "gpt-4"))
	require.False(t, modelMatchesImageInputFallbackModels("gpt-4o", ""))
	// 通配符匹配
	require.True(t, modelMatchesImageInputFallbackModels("deepseek-chat", "deepseek-*"))
	require.True(t, modelMatchesImageInputFallbackModels("deepseek-reasoner", "deepseek-*"))
	require.True(t, modelMatchesImageInputFallbackModels("deepseek-chat", "gpt-4, deepseek-*"))
	require.False(t, modelMatchesImageInputFallbackModels("gpt-4o", "deepseek-*"))
	require.True(t, modelMatchesImageInputFallbackModels("deepseek-r1-0528", "deepseek-*r1*"))
	require.False(t, modelMatchesImageInputFallbackModels("xdeepseek-chat", "deepseek-*"))
}

func TestGlobMatch(t *testing.T) {
	require.True(t, globMatch("deepseek-chat", "deepseek-*"))
	require.True(t, globMatch("deepseek-chat", "deepseek-*chat"))
	require.True(t, globMatch("deepseek-r1-0528", "deepseek-*r1*"))
	require.True(t, globMatch("deepseek-chat", "*deepseek*"))
	require.False(t, globMatch("xdeepseek-chat", "deepseek-*"))
	require.False(t, globMatch("deepseek", "deepseek-*"))
	require.True(t, globMatch("abc", "a*c"))
	require.False(t, globMatch("ac", "a*b*c"))
}

func TestMaybeApplyImageInputFallbackBeforeUpstream(t *testing.T) {
	repo := &imageInputFallbackSettingRepo{values: map[string]string{}}
	settingSvc := NewSettingService(repo, &config.Config{})
	require.NoError(t, settingSvc.SetImageInputFallbackSettings(context.Background(), &ImageInputFallbackSettings{
		Mode:   ImageInputFallbackModeStrip,
		Models: "gpt-4",
	}))
	svc := &OpenAIGatewayService{cfg: &config.Config{}, settingService: settingSvc}

	// 命中模型 + 含图片 → 主动 strip
	body := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"https://x/y.png"}}]}]}`)
	out, reason, changed, err := svc.maybeApplyImageInputFallbackBeforeUpstream(context.Background(), svc.cfg, body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, reason, "proactively")
	require.Equal(t, int64(1), gjson.GetBytes(out, "messages.0.content.#").Int())
	require.Equal(t, "text", gjson.GetBytes(out, "messages.0.content.0.type").String())

	// 未命中模型 → 不处理
	body2 := []byte(`{"model":"claude-3-haiku","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://x/y.png"}}]}]}`)
	_, _, changed2, err := svc.maybeApplyImageInputFallbackBeforeUpstream(context.Background(), svc.cfg, body2)
	require.NoError(t, err)
	require.False(t, changed2)

	// 无图片 → 不处理
	body3 := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`)
	_, _, changed3, err := svc.maybeApplyImageInputFallbackBeforeUpstream(context.Background(), svc.cfg, body3)
	require.NoError(t, err)
	require.False(t, changed3)
}
