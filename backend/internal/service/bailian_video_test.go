//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestParseBailianVideoRequestNormalizesDefaults(t *testing.T) {
	info := ParseBailianVideoRequest([]byte(`{"model":"wan2.7-t2v","prompt":"a cat"}`))
	require.Equal(t, "wan2.7-t2v", info.Model)
	require.Equal(t, "a cat", info.Prompt)
	require.Equal(t, bailianVideoDefaultDurationSeconds, info.DurationSeconds)
	require.Equal(t, bailianVideoDefaultResolution, info.Resolution)
	require.Equal(t, 1, info.N)
	require.False(t, info.HasInputImage())
}

func TestParseBailianVideoRequestFullFields(t *testing.T) {
	body := []byte(`{
		"model":"happyhorse-1.1-i2v",
		"prompt":"run",
		"negative_prompt":"blur",
		"image":{"url":"https://img.example/first.png"},
		"ratio":"9:16",
		"resolution":"1080P",
		"duration":12,
		"seed":42,
		"watermark":false,
		"n":1
	}`)
	info := ParseBailianVideoRequest(body)
	require.Equal(t, "happyhorse-1.1-i2v", info.Model)
	require.Equal(t, "blur", info.NegativePrompt)
	require.Equal(t, "https://img.example/first.png", info.ImageURL)
	require.True(t, info.HasInputImage())
	require.Equal(t, "9:16", info.Ratio)
	require.Equal(t, VideoBillingResolution1080P, info.Resolution)
	require.Equal(t, 12, info.DurationSeconds)
	require.NotNil(t, info.Seed)
	require.EqualValues(t, 42, *info.Seed)
	require.NotNil(t, info.Watermark)
	require.False(t, *info.Watermark)
}

func TestParseBailianVideoRequestClampsDuration(t *testing.T) {
	over := ParseBailianVideoRequest([]byte(`{"model":"wan2.7-t2v","prompt":"x","duration":99}`))
	require.Equal(t, VideoBillingMaxDurationSeconds, over.DurationSeconds)
	under := ParseBailianVideoRequest([]byte(`{"model":"wan2.7-t2v","prompt":"x","duration":-3}`))
	require.Equal(t, bailianVideoDefaultDurationSeconds, under.DurationSeconds)
}

func TestParseBailianVideoRequestStringImage(t *testing.T) {
	info := ParseBailianVideoRequest([]byte(`{"model":"wan2.7-i2v","prompt":"x","image":"data:image/png;base64,AAA"}`))
	require.Equal(t, "data:image/png;base64,AAA", info.ImageURL)
}

func TestBuildDashScopeVideoSynthesisBody(t *testing.T) {
	seed := int64(7)
	watermark := false
	info := BailianVideoRequestInfo{
		Model:           "wan2.7-t2v",
		Prompt:          "a cat",
		NegativePrompt:  "blur",
		ImageURL:        "https://img.example/f.png",
		Ratio:           "16:9",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 5,
		Seed:            &seed,
		Watermark:       &watermark,
	}
	body, err := buildDashScopeVideoSynthesisBody(info, "wan2.7-t2v-mapped")
	require.NoError(t, err)
	require.Equal(t, "wan2.7-t2v-mapped", gjson.GetBytes(body, "model").String())
	require.Equal(t, "a cat", gjson.GetBytes(body, "input.prompt").String())
	require.Equal(t, "blur", gjson.GetBytes(body, "input.negative_prompt").String())
	require.Equal(t, "first_frame", gjson.GetBytes(body, "input.media.0.type").String())
	require.Equal(t, "https://img.example/f.png", gjson.GetBytes(body, "input.media.0.url").String())
	require.Equal(t, "720P", gjson.GetBytes(body, "parameters.resolution").String())
	require.EqualValues(t, 5, gjson.GetBytes(body, "parameters.duration").Int())
	require.Equal(t, "16:9", gjson.GetBytes(body, "parameters.ratio").String())
	require.EqualValues(t, 7, gjson.GetBytes(body, "parameters.seed").Int())
	require.False(t, gjson.GetBytes(body, "parameters.watermark").Bool())
	require.True(t, gjson.GetBytes(body, "parameters.watermark").Exists())
}

func TestBuildDashScopeVideoSynthesisBodyOmitsOptionalFields(t *testing.T) {
	info := BailianVideoRequestInfo{
		Model:           "wan2.7-t2v",
		Prompt:          "a cat",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 5,
	}
	body, err := buildDashScopeVideoSynthesisBody(info, "wan2.7-t2v")
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "input.media").Exists())
	require.False(t, gjson.GetBytes(body, "input.negative_prompt").Exists())
	require.False(t, gjson.GetBytes(body, "parameters.ratio").Exists())
	require.False(t, gjson.GetBytes(body, "parameters.seed").Exists())
	require.False(t, gjson.GetBytes(body, "parameters.watermark").Exists())
}

func TestBailianVideoTaskSessionHash(t *testing.T) {
	hash := BailianVideoTaskSessionHash("task-1", 10, 20)
	require.NotEmpty(t, hash)
	require.Contains(t, hash, "bailian-video:")
	require.NotEqual(t, hash, BailianVideoTaskSessionHash("task-1", 11, 20))
	require.Empty(t, BailianVideoTaskSessionHash("", 10, 20))
	require.Empty(t, BailianVideoTaskSessionHash("task-1", 0, 20))
	require.Empty(t, BailianVideoTaskSessionHash("task-1", 10, 0))
}

func TestIsBailianVideoBillingModel(t *testing.T) {
	require.True(t, isBailianVideoBillingModel("wan2.7-t2v"))
	require.True(t, isBailianVideoBillingModel("wanx2.1-t2v-turbo"))
	require.True(t, isBailianVideoBillingModel("happyhorse-1.1-i2v"))
	require.True(t, isBailianVideoBillingModel(" HappyHorse-1.1-T2V "))
	require.False(t, isBailianVideoBillingModel("qwen3-max"))
	require.False(t, isBailianVideoBillingModel("grok-imagine-video"))
	require.False(t, isBailianVideoBillingModel(""))
}

func TestIsVideoUsageResultCoversBothPlatforms(t *testing.T) {
	grokResult := &OpenAIForwardResult{VideoCount: 1, Model: "grok-imagine-video"}
	require.True(t, isVideoUsageResult(grokResult, nil))
	bailianResult := &OpenAIForwardResult{VideoCount: 1, Model: "happyhorse-1.1-t2v"}
	require.True(t, isVideoUsageResult(bailianResult, nil))
	textResult := &OpenAIForwardResult{VideoCount: 0, Model: "happyhorse-1.1-t2v"}
	require.False(t, isVideoUsageResult(textResult, nil))
	otherResult := &OpenAIForwardResult{VideoCount: 1, Model: "gpt-5.4"}
	require.False(t, isVideoUsageResult(otherResult, nil))
}

func TestGetDefaultBailianVideoPrice(t *testing.T) {
	price, ok := getDefaultBailianVideoPrice("happyhorse-1.1-t2v", "720p")
	require.True(t, ok)
	require.Equal(t, defaultBailianHappyHorseVideoPrice720P, price)

	price, ok = getDefaultBailianVideoPrice("wan2.7-t2v", "1080P")
	require.True(t, ok)
	require.Equal(t, defaultBailianWanVideoPrice1080P, price)

	price, ok = getDefaultBailianVideoPrice("wanx2.1-t2v-turbo", "480p")
	require.True(t, ok)
	require.Equal(t, defaultBailianWanVideoPrice480P, price)

	_, ok = getDefaultBailianVideoPrice("qwen3-max", "720p")
	require.False(t, ok)
}

func TestShouldFailoverBailianUpstreamError(t *testing.T) {
	require.True(t, shouldFailoverBailianUpstreamError(401))
	require.True(t, shouldFailoverBailianUpstreamError(403))
	require.True(t, shouldFailoverBailianUpstreamError(429))
	require.True(t, shouldFailoverBailianUpstreamError(500))
	require.True(t, shouldFailoverBailianUpstreamError(503))
	require.False(t, shouldFailoverBailianUpstreamError(400))
	require.False(t, shouldFailoverBailianUpstreamError(404))
}

func TestExtractBailianUpstreamErrorMessage(t *testing.T) {
	require.Equal(t,
		"InvalidParameter: duration is out of range",
		extractBailianUpstreamErrorMessage([]byte(`{"output":{"code":"InvalidParameter","message":"duration is out of range"}}`)),
	)
	require.Equal(t,
		"Throttling.RateQuota: rate limited",
		extractBailianUpstreamErrorMessage([]byte(`{"code":"Throttling.RateQuota","message":"rate limited","request_id":"r1"}`)),
	)
	require.Empty(t, extractBailianUpstreamErrorMessage([]byte(`{}`)))
}
