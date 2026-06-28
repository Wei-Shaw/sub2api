package service

import "testing"

func TestUpscaleFactorForTarget(t *testing.T) {
	cases := []struct {
		realSize, target string
		want             int
	}{
		{"1024x768", ImageBillingSize2K, 2},
		{"1024x1024", ImageBillingSize4K, 4},
		{"2048x1152", ImageBillingSize4K, 2},
		{"1280x720", ImageBillingSize4K, 3},
		{"2560x1440", ImageBillingSize4K, 2},
		{"3840x2160", ImageBillingSize4K, 0}, // 已达目标
		{"2160x3840", ImageBillingSize2K, 0}, // 真实更高
		{"1024x768", ImageBillingSize1K, 0},  // 目标 1K 不放大
		{"100x100", ImageBillingSize4K, 0},   // scale > 10 不放行
		{"invalid", ImageBillingSize4K, 0},
		{"1024x768", "unknown", 0},
	}
	for _, c := range cases {
		if got := upscaleFactorForTarget(c.realSize, c.target); got != c.want {
			t.Fatalf("upscaleFactorForTarget(%s,%s)=%d want %d", c.realSize, c.target, got, c.want)
		}
	}
}

func TestRewriteOpenAIImageBase64InBody(t *testing.T) {
	body := []byte(`{"data":[{"b64_json":"AAAA"},{"b64_json":"BBBB"}]}`)
	out := rewriteOpenAIImageBase64InBody(body, []string{"AAAA", "BBBB"}, []string{"ZZZZ", "BBBB"})
	want := `{"data":[{"b64_json":"ZZZZ"},{"b64_json":"BBBB"}]}`
	if string(out) != want {
		t.Fatalf("rewrite got %s want %s", out, want)
	}
	// 无改动时原样返回。
	out2 := rewriteOpenAIImageBase64InBody(body, []string{"AAAA"}, []string{"AAAA"})
	if string(out2) != string(body) {
		t.Fatalf("no-op rewrite changed body: %s", out2)
	}
}
