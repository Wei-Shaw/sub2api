package apiz

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestStatusPollLogsRequestAndResponseAtInfo(t *testing.T) {
	var logs bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(original) })

	client := &Client{apiKey: "secret-apiz-key", baseURL: "https://example.test", httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		require.JSONEq(t, `{"task_id":"task-123"}`, string(body))
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"task_id":"task-123","status":"processing"}`)), Request: req}, nil
	})}}

	_, err := client.Status(context.Background(), "https://example.test/api/v3/tasks/query?task_id=task-123")
	require.NoError(t, err)
	text := logs.String()
	require.Contains(t, text, "apiz_status_poll_request")
	require.Contains(t, text, "apiz_status_poll_response")
	require.Contains(t, text, `\"task_id\":\"task-123\"`)
	require.NotContains(t, text, "secret-apiz-key")
}

// adaptSubmitParams 是网关与 apiz 上游之间唯一的参数适配点：
// 上层（演练台 / OpenAI 兼容入口）统一按 fal 系字段名下发，这里翻译成 apiz 的
// 命名与取值。翻错一个字段，用户看到的就是上游 422 而非明确提示，所以逐条锁定。
func TestAdaptSubmitParamsReferenceURLRenames(t *testing.T) {
	t.Run("renames all three reference url arrays", func(t *testing.T) {
		got := adaptSubmitParams(map[string]any{
			"prompt":     "hi",
			"image_urls": []any{"https://a/1.png", "https://a/2.png"},
			"video_urls": []any{"https://a/1.mp4"},
			"audio_urls": []any{"https://a/1.mp3"},
		})
		m, ok := got.(map[string]any)
		require.True(t, ok)

		require.Equal(t, []any{"https://a/1.png", "https://a/2.png"}, m["reference_image_urls"])
		require.Equal(t, []any{"https://a/1.mp4"}, m["reference_video_urls"])
		require.Equal(t, []any{"https://a/1.mp3"}, m["reference_audio_urls"])
		// 原名必须被移除：同时出现两套名字时 apiz 会把未知字段判为非法。
		require.NotContains(t, m, "image_urls")
		require.NotContains(t, m, "video_urls")
		require.NotContains(t, m, "audio_urls")
		// 其余字段原样透传。
		require.Equal(t, "hi", m["prompt"])
	})

	t.Run("does not invent keys when source is absent", func(t *testing.T) {
		m, ok := adaptSubmitParams(map[string]any{"prompt": "hi"}).(map[string]any)
		require.True(t, ok)
		require.NotContains(t, m, "reference_image_urls")
		require.NotContains(t, m, "reference_video_urls")
		require.NotContains(t, m, "reference_audio_urls")
	})

	t.Run("keeps caller-specified target and drops the alias", func(t *testing.T) {
		// 调用方直接用 apiz 原生名传参时，别名不得覆盖它。
		m, ok := adaptSubmitParams(map[string]any{
			"image_urls":           []any{"https://alias/1.png"},
			"reference_image_urls": []any{"https://native/1.png"},
		}).(map[string]any)
		require.True(t, ok)
		require.Equal(t, []any{"https://native/1.png"}, m["reference_image_urls"])
		require.NotContains(t, m, "image_urls")
	})

	t.Run("preserves empty and nil values instead of dropping them", func(t *testing.T) {
		// 空数组是"显式清空"的合法表达，不该在重命名过程中丢掉。
		m, ok := adaptSubmitParams(map[string]any{
			"image_urls": []any{},
			"video_urls": nil,
		}).(map[string]any)
		require.True(t, ok)
		require.Contains(t, m, "reference_image_urls")
		require.Equal(t, []any{}, m["reference_image_urls"])
		require.Contains(t, m, "reference_video_urls")
		require.Nil(t, m["reference_video_urls"])
	})

	t.Run("does not mutate the caller's map", func(t *testing.T) {
		// 调用方可能复用同一份 payload 重试，原地改会导致第二次已被改名。
		src := map[string]any{"image_urls": []any{"https://a/1.png"}}
		_ = adaptSubmitParams(src)
		require.Contains(t, src, "image_urls")
		require.NotContains(t, src, "reference_image_urls")
	})
}

// 既有的三类转换在重构（抽出 renameKeyIfAbsent）后必须行为不变。
func TestAdaptSubmitParamsExistingConversions(t *testing.T) {
	t.Run("generate_audio to audio", func(t *testing.T) {
		m, ok := adaptSubmitParams(map[string]any{"generate_audio": true}).(map[string]any)
		require.True(t, ok)
		require.Equal(t, true, m["audio"])
		require.NotContains(t, m, "generate_audio")
	})

	t.Run("explicit audio wins over generate_audio", func(t *testing.T) {
		m, ok := adaptSubmitParams(map[string]any{
			"generate_audio": true,
			"audio":          false,
		}).(map[string]any)
		require.True(t, ok)
		require.Equal(t, false, m["audio"])
		require.NotContains(t, m, "generate_audio")
	})

	t.Run("resolution is upper-cased", func(t *testing.T) {
		for in, want := range map[string]string{
			"480p": "480P", "720p": "720P",
			"480P": "480P", // 已是大写则原样
		} {
			m, ok := adaptSubmitParams(map[string]any{"resolution": in}).(map[string]any)
			require.True(t, ok)
			require.Equal(t, want, m["resolution"], "in=%q", in)
		}
	})

	t.Run("auto duration and aspect_ratio fall back to concrete values", func(t *testing.T) {
		m, ok := adaptSubmitParams(map[string]any{
			"duration":     "auto",
			"aspect_ratio": "AUTO", // 大小写不敏感
		}).(map[string]any)
		require.True(t, ok)
		require.Equal(t, apizAutoDurationFallback, m["duration"])
		require.Equal(t, apizAutoAspectRatioFallback, m["aspect_ratio"])
	})

	t.Run("non-auto duration is untouched", func(t *testing.T) {
		m, ok := adaptSubmitParams(map[string]any{"duration": 12}).(map[string]any)
		require.True(t, ok)
		require.Equal(t, 12, m["duration"])
	})
}

func TestAdaptSubmitParamsNonMapPassthrough(t *testing.T) {
	// nil / 结构体 / 切片等非 map 输入应原样返回，不得 panic。
	require.Nil(t, adaptSubmitParams(nil))
	type payload struct{ Prompt string }
	p := payload{Prompt: "x"}
	require.Equal(t, p, adaptSubmitParams(p))
	require.Equal(t, []string{"a"}, adaptSubmitParams([]string{"a"}))
}

func TestRenameKeyIfAbsent(t *testing.T) {
	t.Run("moves value and deletes source", func(t *testing.T) {
		m := map[string]any{"a": 1}
		renameKeyIfAbsent(m, "a", "b")
		require.Equal(t, map[string]any{"b": 1}, m)
	})

	t.Run("no-op when source missing", func(t *testing.T) {
		m := map[string]any{"x": 1}
		renameKeyIfAbsent(m, "a", "b")
		require.Equal(t, map[string]any{"x": 1}, m)
	})

	t.Run("target wins, source dropped", func(t *testing.T) {
		m := map[string]any{"a": 1, "b": 2}
		renameKeyIfAbsent(m, "a", "b")
		require.Equal(t, map[string]any{"b": 2}, m)
	})
}
