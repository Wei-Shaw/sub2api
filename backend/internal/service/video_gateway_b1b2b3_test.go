package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// --- B1: request-side `ratio` field name + orientation mapping ---------------

// TestNormalizeSeedanceRatio exhaustively covers the pure mapping helper: logical
// orientation keywords (EN + 中文) and already-valid "W:H" ratios → Ark `ratio` value.
func TestNormalizeSeedanceRatio(t *testing.T) {
	cases := map[string]string{
		"":          "",
		"9:16":      "9:16",
		"16:9":      "16:9",
		"1:1":       "1:1",
		"竖屏":        "9:16",
		"横屏":        "16:9",
		"方形":        "1:1",
		"portrait":  "9:16",
		"Landscape": "16:9",
		"SQUARE":    "1:1",
		" 9:16 ":    "9:16", // trimmed
		"4:3":       "4:3",  // valid-but-unmapped → passthrough verbatim
		"21:9":      "21:9", // shape-valid passthrough
		"banana":    "",     // arbitrary text → dropped (field omitted)
		"16x9":      "",     // not a W:H shape → dropped
	}
	for in, want := range cases {
		if got := normalizeSeedanceRatio(in); got != want {
			t.Fatalf("normalizeSeedanceRatio(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSeedanceCreateSendsRatioWithOrientationMapping is the B1 contract test: it
// captures the ACTUAL request body the adapter sends (against a localhost mock-Ark,
// never the real Ark) and asserts the aspect is sent as Ark's `ratio` field with the
// correct mapped value — and that the legacy `aspect_ratio` field is NOT sent.
func TestSeedanceCreateSendsRatioWithOrientationMapping(t *testing.T) {
	cases := []struct {
		name, aspect, wantRatio string
	}{
		{"explicit_portrait", "9:16", "9:16"},
		{"keyword_portrait_cn", "竖屏", "9:16"},
		{"keyword_portrait_en", "portrait", "9:16"},
		{"explicit_landscape", "16:9", "16:9"},
		{"keyword_landscape_cn", "横屏", "16:9"},
		{"square_cn", "方形", "1:1"},
		{"square_ratio", "1:1", "1:1"},
		{"passthrough_4_3", "4:3", "4:3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured map[string]any
			adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&captured)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"id":"t","status":"queued"}`))
			})
			task := &VideoTask{
				Model:       "doubao-seedance-2-0-260128",
				Prompt:      "orientation contract clip",
				Duration:    5,
				AspectRatio: tc.aspect,
			}
			if _, err := adapter.CreateTask(context.Background(), acc, task); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			if captured["ratio"] != tc.wantRatio {
				t.Fatalf("aspect %q → ratio=%v, want %q (full payload=%+v)", tc.aspect, captured["ratio"], tc.wantRatio, captured)
			}
			if _, leaked := captured["aspect_ratio"]; leaked {
				t.Fatalf("request must NOT send the legacy aspect_ratio field (Ark ignores it): %+v", captured)
			}
		})
	}
}

// --- B2: resolution-scaled poll budget + wall-clock backstop -----------------

// TestVideoPollBudgetScalesWithResolution proves the poll cap scales low→high and
// that 1080p covers the observed ~19min real generation, while 720p stays at the
// legacy default and unknown resolutions fall back to the safe middle tier.
func TestVideoPollBudgetScalesWithResolution(t *testing.T) {
	base := videoPollAttempts720p
	p480 := scalePollBudgetForResolution(base, "480p")
	p720 := scalePollBudgetForResolution(base, "720p")
	p1080 := scalePollBudgetForResolution(base, "1080p")
	if p480 >= p720 || p720 >= p1080 {
		t.Fatalf("poll budgets must scale 480p<720p<1080p, got %d/%d/%d", p480, p720, p1080)
	}
	// at the default 72 baseline the tiers equal the tier constants.
	if p480 != videoPollAttempts480p || p720 != videoPollAttempts720p || p1080 != videoPollAttempts1080p {
		t.Fatalf("default-baseline tiers = %d/%d/%d, want %d/%d/%d", p480, p720, p1080, videoPollAttempts480p, videoPollAttempts720p, videoPollAttempts1080p)
	}
	if p720 != videoDefaultMaxPollAttempts {
		t.Fatalf("720p tier must equal the legacy default %d (back-compat), got %d", videoDefaultMaxPollAttempts, p720)
	}
	if scalePollBudgetForResolution(base, "") != p720 || scalePollBudgetForResolution(base, "weird") != p720 {
		t.Fatalf("unknown/empty resolution must fall back to the 720p baseline")
	}
	if scalePollBudgetForResolution(base, "4k") != p1080 || scalePollBudgetForResolution(base, "1080P") != p1080 {
		t.Fatalf("≥1080p (incl. 4k, case-insensitive) must map to the long tier")
	}
	if window := time.Duration(p1080) * videoDefaultPollInterval; window < 19*time.Minute {
		t.Fatalf("1080p poll window %s must cover the observed ~19min 1080p generation", window)
	}
	// scaling is baseline-relative: a doubled baseline doubles each tier.
	if got := scalePollBudgetForResolution(2*base, "1080p"); got != 2*p1080 {
		t.Fatalf("doubling the baseline must double the 1080p budget: got %d want %d", got, 2*p1080)
	}
}

// TestMaxPollAttemptsForTaskResolutionAndOverride proves the service picks the
// resolution-scaled cap when config is unset, and that an explicit config
// max_poll_attempts pins the cap for ALL resolutions (back-compat).
func TestMaxPollAttemptsForTaskResolutionAndOverride(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil) // nil cfg => resolution scaling
	if got := svc.maxPollAttemptsForTask(&VideoTask{Resolution: "480p"}); got != videoPollAttempts480p {
		t.Fatalf("480p task cap = %d, want %d", got, videoPollAttempts480p)
	}
	if got := svc.maxPollAttemptsForTask(&VideoTask{Resolution: "1080p"}); got != videoPollAttempts1080p {
		t.Fatalf("1080p task cap = %d, want %d", got, videoPollAttempts1080p)
	}
	if got := svc.maxPollAttemptsForTask(&VideoTask{}); got != videoPollAttempts720p {
		t.Fatalf("no-resolution task cap = %d, want 720p default %d", got, videoPollAttempts720p)
	}
	// a configured max_poll_attempts is the 720p BASELINE and STILL scales by resolution
	// (it does NOT flat-pin every resolution — that was the dead-code defect).
	baselined := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithMaxPolls(7))
	for _, res := range []string{"480p", "720p", "1080p", ""} {
		want := scalePollBudgetForResolution(7, res)
		if got := baselined.maxPollAttemptsForTask(&VideoTask{Resolution: res}); got != want {
			t.Fatalf("configured baseline 7 at %q: got %d want %d (must scale, not pin)", res, got, want)
		}
	}
	if got := baselined.maxPollAttemptsForTask(&VideoTask{Resolution: "720p"}); got != 7 {
		t.Fatalf("720p must equal the configured baseline 7, got %d", got)
	}
	if got := baselined.maxPollAttemptsForTask(&VideoTask{Resolution: "1080p"}); got <= 7 {
		t.Fatalf("1080p must scale ABOVE the baseline 7 (not flat-pinned), got %d", got)
	}
}

// TestMaxPollAttemptsScalesUnderProductionConfig mirrors the SHIPPED config
// (max_poll_attempts=72 — the viper default that validation forces > 0), the exact
// path the adversarial review flagged as making resolution scaling dead code. It
// proves 1080p STILL gets its long window in production, not the flat 72/6min.
func TestMaxPollAttemptsScalesUnderProductionConfig(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithMaxPolls(72)) // production default
	if got := svc.maxPollAttemptsForTask(&VideoTask{Resolution: "1080p"}); got != videoPollAttempts1080p {
		t.Fatalf("under shipped config (max_poll_attempts=72), 1080p must STILL scale to %d, got %d (resolution scaling is dead!)", videoPollAttempts1080p, got)
	}
	if got := svc.maxPollAttemptsForTask(&VideoTask{Resolution: "480p"}); got != videoPollAttempts480p {
		t.Fatalf("under shipped config, 480p must scale to %d, got %d", videoPollAttempts480p, got)
	}
	if got := svc.maxPollAttemptsForTask(&VideoTask{Resolution: "720p"}); got != videoPollAttempts720p {
		t.Fatalf("under shipped config, 720p must equal the 72 baseline, got %d", got)
	}
}

// TestEffectiveTaskTimeoutWrapsPollWindow proves the wall-clock backstop sits
// strictly OUTSIDE the resolution-scaled poll window (so a slow 1080p render is not
// killed before its longer window completes) while keeping the configured base as a
// floor for resolutions whose window already fits.
func TestEffectiveTaskTimeoutWrapsPollWindow(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil) // 5s interval, resolution scaling
	base := 15 * time.Minute

	eff1080 := svc.effectiveTaskTimeout(&VideoTask{Resolution: "1080p"}, base)
	window1080 := time.Duration(videoPollAttempts1080p) * videoDefaultPollInterval
	if eff1080 <= window1080 {
		t.Fatalf("1080p wall-clock %s must exceed its poll window %s (outer backstop)", eff1080, window1080)
	}
	if eff1080 <= base {
		t.Fatalf("1080p wall-clock %s must exceed the 15min base when the window needs more", eff1080)
	}
	// 720p: base 15min already exceeds window(6min)+margin(5min)=11min → base kept.
	if got := svc.effectiveTaskTimeout(&VideoTask{Resolution: "720p"}, base); got != base {
		t.Fatalf("720p wall-clock should keep the 15min base, got %s", got)
	}
}

// --- B3: video-to-video reference attached to the content array --------------

// TestSeedanceCreateAttachesReferenceVideoForV2V is the B3 contract test: a task
// with a reference video URL must surface a `video_url` content item (alongside the
// text item) in the ACTUAL request body, gated by the same SSRF/allowlist check as
// images. Asserts OUR construction only (no real call); the exact Ark field name is
// UNVERIFIED and tracked on the 待授权 list.
func TestSeedanceCreateAttachesReferenceVideoForV2V(t *testing.T) {
	const refVideo = "https://ref.cn-beijing.volces.com/clip.mp4" // under the volces.com allowlist
	var captured map[string]any
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"t","status":"queued"}`))
	})
	task := &VideoTask{
		Model:             "doubao-seedance-2-0-260128",
		Prompt:            "v2v 换皮 transform",
		Duration:          5,
		ReferenceVideoURL: refVideo,
	}
	if _, err := adapter.CreateTask(context.Background(), acc, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	content, ok := captured["content"].([]any)
	if !ok || len(content) < 2 {
		t.Fatalf("expected content array with text + video items, got %#v", captured["content"])
	}
	var sawText, sawVideo bool
	for _, item := range content {
		m, _ := item.(map[string]any)
		switch m["type"] {
		case "text":
			sawText = true
		case "video_url":
			vu, _ := m["video_url"].(map[string]any)
			if vu["url"] != refVideo {
				t.Fatalf("video_url.url = %v, want %q", vu["url"], refVideo)
			}
			sawVideo = true
		}
	}
	if !sawText || !sawVideo {
		t.Fatalf("expected both a text and a video_url content item; sawText=%v sawVideo=%v (content=%#v)", sawText, sawVideo, content)
	}
}

// TestSeedanceCreateRejectsUnsafeReferenceVideoURL proves the v2v reference video
// is gated by the SSRF guard — an internal/loopback URL is rejected before any send.
func TestSeedanceCreateRejectsUnsafeReferenceVideoURL(t *testing.T) {
	called := false
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"t","status":"queued"}`))
	})
	task := &VideoTask{
		Model:             "doubao-seedance-2-0-260128",
		Prompt:            "v2v unsafe ref",
		Duration:          5,
		ReferenceVideoURL: "http://169.254.169.254/latest/meta-data/",
	}
	if _, err := adapter.CreateTask(context.Background(), acc, task); err == nil {
		t.Fatal("expected an SSRF rejection for an internal reference_video_url")
	}
	if called {
		t.Fatal("an unsafe reference_video_url must be rejected BEFORE any provider call")
	}
}
