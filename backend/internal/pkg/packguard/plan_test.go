package packguard

import (
	"encoding/json"
	"testing"
	"time"
)

func TestVideoCeilingsAndLifetime(t *testing.T) {
	for _, resolution := range []string{"480p", "720p", "1080p"} {
		for _, duration := range []int{4, 15, 30} {
			body, _ := json.Marshal(map[string]any{"model": SeedanceModel, "content": []any{map[string]any{"type": "text", "text": "A studio."}}, "duration": duration, "resolution": resolution})
			p, canonical, err := BuildPlan("/contents/generations/tasks", body)
			if err != nil {
				t.Fatal(err)
			}
			if !p.Async || p.Lifetime != time.Hour || p.Units <= 0 {
				t.Fatalf("invalid plan: %+v", p)
			}
			var result map[string]any
			_ = json.Unmarshal(canonical, &result)
			if result["execution_expires_after"] != float64(3600) {
				t.Fatal("missing enforced expiry")
			}
			pixels := int64(854 * 480)
			if resolution == "720p" {
				pixels = 1280 * 720
			}
			if resolution == "1080p" {
				pixels = 1920 * 1080
			}
			expected := (pixels*24*int64(duration+1) + 1023) / 1024 * p.Ratio
			if p.Units < expected {
				t.Fatal("reservation does not cover documented dimensions and frame rounding")
			}
		}
	}
}
func TestSeedOutputLimitIncludesReasoningAndPaidFeaturesFailClosed(t *testing.T) {
	p, body, err := BuildPlan("/chat/completions", []byte(`{"model":"dola-seed-2-1-turbo-260628","messages":[{"role":"user","content":"Hello"}],"max_tokens":16,"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	_ = json.Unmarshal(body, &v)
	if v["max_tokens"] != nil || v["max_completion_tokens"] != float64(16) || p.Units < (16+1024)*64 {
		t.Fatal("unbounded or changed output limit")
	}
	for _, addition := range []string{`,"caching":{"type":"enabled"}`, `,"previous_response_id":"old"`, `,"tools":[{"type":"web_search"}]`, `,"input":[{"type":"input_image","image_url":"https://example.com/a.png"}]`, `,"background":true`} {
		_, _, err = BuildPlan("/responses", []byte(`{"model":"dola-seed-2-1-turbo-260628","input":"Hi"`+addition+`}`))
		if err == nil {
			t.Fatalf("unpriced feature was accepted: %s", addition)
		}
	}
	_, _, err = BuildPlan("/contents/generations/tasks", []byte(`{"model":"dreamina-seedance-2-5-260628","content":[{"type":"text","text":"Hi"}],"execution_expires_after":259200}`))
	if err == nil {
		t.Fatal("long queue could outlive funding")
	}
}
func TestOnlyQualifiedModelsAndEndpoints(t *testing.T) {
	if _, _, err := BuildPlan("/contents/generations/tasks", []byte(`{"model":"dreamina-seedance-2-5-260628","content":[{"type":"text","text":"Studio --resolution 1080p --duration 30"}],"duration":4,"resolution":"480p"}`)); err == nil {
		t.Fatal("legacy prompt flags could override the priced JSON settings")
	}
	for _, body := range []string{`{"model":"new-future-model","messages":[]}`, `{"model":"dola-seed-2-1-turbo-260628","n":2}`, `{"model":"dola-seed-2-1-turbo-260628","max_tokens":1000000}`} {
		if _, _, err := BuildPlan("/chat/completions", []byte(body)); err == nil {
			t.Fatal("unsupported cost was admitted")
		}
	}
	if _, _, err := BuildPlan("/embeddings", []byte(`{"model":"dola-seed-2-1-turbo-260628"}`)); err == nil {
		t.Fatal("unpriced endpoint admitted")
	}
}
func TestUsageRequiresTerminalProof(t *testing.T) {
	cases := []struct {
		body   string
		stream bool
		want   int64
	}{
		{`{"choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":7}}`, false, 10},
		{`{"status":"completed","usage":{"input_tokens":4,"output_tokens":6}}`, false, 10},
		{"data: {\"choices\":[],\"usage\":{\"total_tokens\":10}}\n\ndata: [DONE]\n\n", true, 10},
		{"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":7}}}\n\n", true, 10},
		{"data: {\"usage\":{\"total_tokens\":10}}\n\n", true, 0},
		{"data: {\"usage\":{\"total_tokens\":10}}\n\ndata: {\"type\":\"error\"}\n\ndata: [DONE]\n\n", true, 0},
		{`{"usage":{"total_tokens":10}}`, false, 0},
	}
	for _, c := range cases {
		if got := usageTokens([]byte(c.body), c.stream); got != c.want {
			t.Fatalf("got %d want %d for %s", got, c.want, c.body)
		}
	}
}
