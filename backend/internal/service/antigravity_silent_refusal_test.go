package service

import "testing"

func TestAntigravityRefusalDetector_IsSilentRefusal(t *testing.T) {
	cases := []struct {
		name             string
		enabled          bool
		blockingOverride []string
		finishReason     string
		sawContent       bool
		want             bool
	}{
		{name: "SAFETY no content -> refusal", enabled: true, finishReason: "SAFETY", sawContent: false, want: true},
		{name: "RECITATION no content -> refusal", enabled: true, finishReason: "RECITATION", sawContent: false, want: true},
		{name: "PROHIBITED_CONTENT no content -> refusal", enabled: true, finishReason: "PROHIBITED_CONTENT", sawContent: false, want: true},
		{name: "SAFETY with content -> not refusal", enabled: true, finishReason: "SAFETY", sawContent: true, want: false},
		{name: "STOP no content -> not refusal", enabled: true, finishReason: "STOP", sawContent: false, want: false},
		{name: "MAX_TOKENS no content -> not refusal", enabled: true, finishReason: "MAX_TOKENS", sawContent: false, want: false},
		{name: "empty finish no content -> not refusal", enabled: true, finishReason: "", sawContent: false, want: false},
		{name: "lowercase safety normalized -> refusal", enabled: true, finishReason: "safety", sawContent: false, want: true},
		{name: "whitespace padded -> refusal", enabled: true, finishReason: " SAFETY ", sawContent: false, want: true},
		{name: "disabled -> never refusal", enabled: false, finishReason: "SAFETY", sawContent: false, want: false},
		{name: "override set excludes SAFETY", enabled: true, blockingOverride: []string{"RECITATION"}, finishReason: "SAFETY", sawContent: false, want: false},
		{name: "override set includes custom", enabled: true, blockingOverride: []string{"CUSTOM_BLOCK"}, finishReason: "custom_block", sawContent: false, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newAntigravityRefusalDetector(tc.enabled, tc.blockingOverride)
			got := d.IsSilentRefusal(tc.finishReason, tc.sawContent)
			if got != tc.want {
				t.Fatalf("IsSilentRefusal(%q, content=%v) = %v, want %v", tc.finishReason, tc.sawContent, got, tc.want)
			}
		})
	}
}

func TestAntigravityRefusalDetector_NilSafe(t *testing.T) {
	var d *antigravityRefusalDetector
	if d.Enabled() {
		t.Fatal("nil detector should not be enabled")
	}
	if d.IsSilentRefusal("SAFETY", false) {
		t.Fatal("nil detector should never report a refusal")
	}
}
