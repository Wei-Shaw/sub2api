//go:build unit

package adobe

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoFamiliesRegistered(t *testing.T) {
	names := make([]string, 0, len(VideoFamilies))
	for _, f := range VideoFamilies {
		names = append(names, f.Family)
	}
	require.Equal(t, []string{
		"sora2", "sora2-pro", "veo31", "veo31-ref", "veo31-fast", "kling-o3", "kling3",
	}, names)
}

func TestResolveVideoModelSora(t *testing.T) {
	conf, ok := ResolveVideoModel("firefly-sora2-8s-16x9")
	require.True(t, ok)
	require.Equal(t, "sora2", conf.Family)
	require.Equal(t, "openai:firefly:colligo:sora2", conf.UpstreamModel)
	require.Equal(t, "sora", conf.UpstreamModelID)
	require.Equal(t, "sora-2", conf.UpstreamModelVersion)
	require.Equal(t, EngineSora2, conf.Engine)
	require.Equal(t, 8, conf.Duration)
	require.Equal(t, "16:9", conf.AspectRatio)
	require.Equal(t, VideoResolution720P, conf.OutputResolution)
	require.False(t, conf.GenerateAudio)
	require.Empty(t, conf.ReferenceMode)

	// sora 系固定 720p，不把分辨率拼进 id。
	_, ok = ResolveVideoModel("firefly-sora2-8s-16x9-720p")
	require.False(t, ok)
}

func TestResolveVideoModelVeo(t *testing.T) {
	standard, ok := ResolveVideoModel("firefly-veo31-6s-16x9-1080p")
	require.True(t, ok)
	require.Equal(t, "veo31", standard.Family)
	require.Equal(t, "3.1-generate", standard.UpstreamModelVersion)
	require.Equal(t, EngineVeo31Standard, standard.Engine)
	require.Equal(t, 6, standard.Duration)
	require.Equal(t, VideoResolution1080P, standard.OutputResolution)

	fast, ok := ResolveVideoModel("firefly-veo31-fast-4s-9x16-720p")
	require.True(t, ok)
	require.Equal(t, "veo31-fast", fast.Family)
	require.Equal(t, "3.1-fast-generate", fast.UpstreamModelVersion)
	require.Equal(t, EngineVeo31Fast, fast.Engine)

	ref, ok := ResolveVideoModel("firefly-veo31-ref-8s-16x9-1080p")
	require.True(t, ok)
	require.Equal(t, ReferenceModeImage, ref.ReferenceMode)
	// ref 与 fast 都以 firefly-veo31- 开头，必须解析成各自的族而非 veo31。
	require.Equal(t, "veo31-ref", ref.Family)
}

func TestResolveVideoModelKling(t *testing.T) {
	kling3, ok := ResolveVideoModel("firefly-kling3-10s-16x9")
	require.True(t, ok)
	require.True(t, kling3.GenerateAudio, "kling3 默认生成音频")

	klingO3, ok := ResolveVideoModel("firefly-kling-o3-15s-9x16")
	require.True(t, ok)
	require.Equal(t, VideoResolution1080P, klingO3.OutputResolution)
	require.False(t, klingO3.GenerateAudio)
}

func TestResolveVideoModelRejectsUnknown(t *testing.T) {
	// 3s 不在 sora2 的时长枚举内。
	_, ok := ResolveVideoModel("firefly-sora2-3s-16x9")
	require.False(t, ok)
	// 图像模型不应落进视频目录。
	_, ok = ResolveVideoModel("firefly-gpt-image-2-2k-1x1")
	require.False(t, ok)

	require.True(t, IsVideoModelID("firefly-veo31-6s-16x9-1080p"))
	require.False(t, IsVideoModelID("nope"))
	require.False(t, IsVideoModelID(""))
}

func TestVideoSizeFor(t *testing.T) {
	size, ok := VideoSizeFor(VideoResolution720P, "16:9")
	require.True(t, ok)
	require.Equal(t, Size{1280, 720}, size)

	size, ok = VideoSizeFor(VideoResolution1080P, "9:16")
	require.True(t, ok)
	require.Equal(t, Size{1080, 1920}, size)

	// 视频只支持横竖两种比例。
	_, ok = VideoSizeFor(VideoResolution720P, "1:1")
	require.False(t, ok)
}

func TestVideoModelConfMaxInputImages(t *testing.T) {
	tests := map[string]int{
		"firefly-sora2-8s-16x9":            1,
		"firefly-veo31-6s-16x9-1080p":      2,
		"firefly-veo31-ref-6s-16x9-1080p":  3,
		"firefly-veo31-fast-6s-16x9-1080p": 2,
		"firefly-kling3-10s-16x9":          2,
		"firefly-kling-o3-15s-9x16":        2,
	}
	for modelID, want := range tests {
		conf, ok := ResolveVideoModel(modelID)
		require.True(t, ok, modelID)
		require.Equal(t, want, conf.MaxInputImages(), modelID)
	}
}

// 目录里每个条目都必须能取到像素尺寸并构造出 payload，否则 /v1/models 会列出不可用的模型。
func TestEveryVideoModelIsUsable(t *testing.T) {
	require.NotEmpty(t, VideoModelIDs)
	for _, modelID := range VideoModelIDs {
		conf, ok := ResolveVideoModel(modelID)
		require.True(t, ok, modelID)

		size := conf.Size()
		require.NotZero(t, size.Width, modelID)
		require.NotZero(t, size.Height, modelID)

		opts := VideoPayloadOptionsFor(conf)
		opts.Prompt = "smoke"
		payload := BuildVideoPayload(opts)
		require.Equal(t, conf.UpstreamModelID, payload["modelId"], modelID)
		require.Equal(t, conf.UpstreamModelVersion, payload["modelVersion"], modelID)
		require.Equal(t, size, payload["size"], modelID)
	}
}

// 全量 id 才是视频的对外契约：时长是 id 的一部分，无法从请求参数推导。
func TestVideoModelIDsAreSortedAndComplete(t *testing.T) {
	require.Len(t, VideoModelIDs, 58)
	for i := 1; i < len(VideoModelIDs); i++ {
		require.Less(t, VideoModelIDs[i-1], VideoModelIDs[i], "VideoModelIDs 应有序")
	}
}
