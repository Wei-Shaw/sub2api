package service

import (
	"context"
	"net/http"
	"testing"
)

// TestSeedancePollParsesRatioGoldSample is the Module C field-alignment gold sample:
// Volcengine Ark returns the aspect ratio as "ratio" (e.g. "16:9"), which the
// adapter previously dropped. A captured real response (id/status/content.video_url
// is confirmed by the form-B smoke; the exact nesting of "ratio" is not pinned by a
// saved sample) must parse "16:9" out — accepted whether ratio is nested under
// content or at the top level.
func TestSeedancePollParsesRatioGoldSample(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "ratio nested under content alongside video_url",
			body: `{"id":"t1","status":"succeeded","content":{"video_url":"https://ark-content.cn-beijing.volces.com/v/ok.mp4","ratio":"16:9"}}`,
		},
		{
			name: "ratio at the top level",
			body: `{"id":"t1","status":"succeeded","ratio":"16:9","content":{"video_url":"https://ark-content.cn-beijing.volces.com/v/ok.mp4"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			})
			task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 3, UpstreamTaskID: "t1"}

			res, err := adapter.PollTask(context.Background(), acc, task)
			if err != nil {
				t.Fatalf("PollTask: %v", err)
			}
			if res.Status != VideoStatusSucceeded {
				t.Fatalf("status = %q, want succeeded", res.Status)
			}
			if got := res.Payload["ratio"]; got != "16:9" {
				t.Fatalf("expected parsed ratio 16:9, got %v (payload=%+v)", got, res.Payload)
			}
		})
	}
}

// TestSeedancePollDropsNonRatioShapedValue: a "ratio" field that is not a plausible
// W:H aspect ratio (e.g. an unexpected/credential-shaped string) must NOT be surfaced
// into the persisted payload, so it cannot bypass redaction.
func TestSeedancePollDropsNonRatioShapedValue(t *testing.T) {
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"t1","status":"succeeded","ratio":"sk-not-a-ratio-leak-123","content":{"video_url":"https://ark-content.cn-beijing.volces.com/v/ok.mp4"}}`))
	})
	task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 3, UpstreamTaskID: "t1"}

	res, err := adapter.PollTask(context.Background(), acc, task)
	if err != nil {
		t.Fatalf("PollTask: %v", err)
	}
	if got, ok := res.Payload["ratio"]; ok {
		t.Fatalf("non-ratio-shaped value must be dropped, but payload carried ratio=%v", got)
	}
}

func TestLooksLikeAspectRatio(t *testing.T) {
	valid := []string{"16:9", "9:16", "1:1", "4:3", "21:9", "1280:720"}
	for _, s := range valid {
		if !looksLikeAspectRatio(s) {
			t.Errorf("expected %q to be a valid aspect ratio", s)
		}
	}
	invalid := []string{"", "16", "16:", ":9", "16:9:1", "16x9", " 16:9", "sk-abc123", "abc:def", "12345:9", "16: 9"}
	for _, s := range invalid {
		if looksLikeAspectRatio(s) {
			t.Errorf("expected %q to be rejected as an aspect ratio", s)
		}
	}
}
