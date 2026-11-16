package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 生图意图的字节级预检查只能少算、不能漏判：任何能让扫描成立的 body，
// openAIBodyMayDeclareImageGeneration 都必须放行。下面用扫描函数本身作为基准，
// 逐个语料核对「带预检查」与「不带预检查」的结果完全一致。
func TestImageGenerationIntentPrefilterPreservesScanOutcomes(t *testing.T) {
	escape := string(rune(92)) + "u005f" // _
	bodies := map[string]string{
		"no image declarations": `{"model":"gpt-5.5","input":"hello"}`,
		"empty body":            ``,
		"invalid json":          `{"tools":[{"type":"image_generation"}]`,
		"native tool":           `{"model":"gpt-5.5","tools":[{"type":"image_generation"}]}`,
		"namespace tool":        `{"model":"gpt-5.5","tools":[{"type":"namespace","name":"image_gen"}]}`,
		"tool choice string":    `{"model":"gpt-5.5","tool_choice":"image_generation"}`,
		"tool choice namespace": `{"model":"gpt-5.5","tool_choice":{"type":"namespace","name":"image_gen"}}`,
		"tool choice function":  `{"model":"gpt-5.5","tool_choice":{"type":"function","function":{"name":"image_gen.imagegen"}}}`,
		"input additional":      `{"input":[{"type":"additional_tools","tools":[{"type":"image_generation"}]}]}`,
		"escaped key":           `{"tool_` + string(rune(92)) + `u0063hoice":{"type":"image_generation"}}`,
		"escaped tool type":     `{"tools":[{"type":"image` + escape + `generation"}]}`,
		"escaped namespace":     `{"tools":[{"type":"namespace","name":"image` + escape + `gen"}]}`,
		"uppercase model":       `{"model":"GPT-Image-2"}`,
		"grok model":            `{"model":"grok-imagine"}`,
		"uppercase grok model":  `{"model":"Grok-Imagine"}`,
		"duplicate model":       `{"model":"gpt-5.5","model":"gpt-image-2"}`,
		"model only":            `{"model":"gpt-image-2"}`,
		"prose mentions image":  `{"model":"gpt-5.5","input":"please imagine a cat"}`,
		"namespace value":       `{"tools":[{"type":"namespace","name":"image_gen","tools":[]}]}`,
	}
	for name, raw := range bodies {
		body := []byte(raw)
		t.Run(name, func(t *testing.T) {
			// endpoint 与 requestedModel 都取非图片值，预检查才会真正生效。
			require.Equal(t,
				referenceImageGenerationIntent(body),
				IsImageGenerationIntent("/v1/responses", "gpt-5.5", body),
				"declaration rule must not change with the prefilter")
			require.Equal(t,
				referenceExplicitImageGenerationIntent(body),
				IsExplicitImageGenerationIntent("/v1/responses", "gpt-5.5", body),
				"explicit rule must not change with the prefilter")
			require.Equal(t,
				referenceExplicitImageGenerationIntent(body),
				IsImageGenerationIntentForPlatform("/v1/responses", "gpt-5.5", body, PlatformGrok),
				"grok rule must not change with the prefilter")
			require.Equal(t,
				referenceImageGenerationIntent(body),
				IsImageGenerationIntentForPlatform("/v1/responses", "gpt-5.5", body, PlatformOpenAI),
				"platform rule must not change with the prefilter")
		})
	}
}

// 下面两个参考实现就是修复前的完整扫描：校验 + 根对象遍历，不带字节级预检查。
func referenceImageGenerationIntent(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	return imageGenerationDeclarationsFromRoot(parseRawJSONView(body))
}

func referenceExplicitImageGenerationIntent(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	return explicitImageGenerationDeclarationsFromRoot(parseRawJSONView(body))
}

// 预检查的健全性：扫描能成立的 body 一定被放行。语料之外由 fuzz 继续探索。
func TestImageGenerationIntentPrefilterIsNecessary(t *testing.T) {
	seeds := []string{
		`{"model":"gpt-image-2"}`,
		`{"tools":[{"type":"image_generation"}]}`,
		`{"tools":[{"type":"namespace","name":"image_gen"}]}`,
		`{"tool_choice":{"type":"namespace","name":"image_gen"}}`,
		`{"input":[{"type":"additional_tools","tools":[{"type":"image_generation"}]}]}`,
		`{"model":"grok-imagine"}`,
		`{"model":"gpt-5.5","input":"nothing here"}`,
	}
	for _, seed := range seeds {
		body := []byte(seed)
		if referenceImageGenerationIntent(body) {
			require.True(t, openAIBodyMayDeclareImageGeneration(body), "prefilter must not hide a detected body: %s", seed)
		}
	}
}

func FuzzImageGenerationIntentPrefilter(f *testing.F) {
	f.Add([]byte(`{"model":"gpt-image-2"}`))
	f.Add([]byte(`{"tools":[{"type":"namespace","name":"image_gen"}]}`))
	f.Add([]byte(`{"model":"gpt-5.5"}`))
	f.Add([]byte(`{"tools":[{"type":"image` + string(rune(92)) + `u005fgeneration"}]}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		if referenceImageGenerationIntent(body) && !openAIBodyMayDeclareImageGeneration(body) {
			t.Fatalf("prefilter hid a detected body: %q", body)
		}
		if referenceExplicitImageGenerationIntent(body) && !openAIBodyMayDeclareImageGeneration(body) {
			t.Fatalf("prefilter hid a detected body: %q", body)
		}
	})
}

func TestContainsASCIIFold(t *testing.T) {
	cases := []struct {
		name     string
		haystack string
		needle   string
		want     bool
	}{
		{"exact", "gpt-image-2", "imag", true},
		{"upper", "GPT-IMAGE-2", "imag", true},
		{"mixed", "Gpt-ImAgE-2", "imag", true},
		{"grok", "grok-imagine", "imag", true},
		{"absent", "gpt-5.5", "imag", false},
		{"needle longer", "img", "imag", false},
		{"empty haystack", "", "imag", false},
		{"partial at end", "xxIma", "imag", false},
		{"second candidate matches", "iaaiimag", "imag", true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, containsASCIIFold([]byte(tt.haystack), []byte(tt.needle)))
		})
	}
	require.False(t, containsASCIIFold(nil, []byte("imag")))

	// 与 strings.Contains(strings.ToLower(...)) 对拍一段混合大小写文本。
	haystack := []byte(`{"model":"Gpt-Image-2","input":"IMAGINE this, Image and IMAGE"}`)
	require.True(t, containsASCIIFold(haystack, []byte("imag")))
	require.Equal(t,
		strings.Contains(strings.ToLower(string(haystack)), "imag"),
		containsASCIIFold(haystack, []byte("imag")))
}
