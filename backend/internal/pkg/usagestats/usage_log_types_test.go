package usagestats

import (
	"encoding/json"
	"testing"
)

func TestIsValidModelSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{name: "requested", source: ModelSourceRequested, want: true},
		{name: "upstream", source: ModelSourceUpstream, want: true},
		{name: "mapping", source: ModelSourceMapping, want: true},
		{name: "invalid", source: "foobar", want: false},
		{name: "empty", source: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidModelSource(tc.source); got != tc.want {
				t.Fatalf("IsValidModelSource(%q)=%v want %v", tc.source, got, tc.want)
			}
		})
	}
}

func TestNormalizeModelSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "requested", source: ModelSourceRequested, want: ModelSourceRequested},
		{name: "upstream", source: ModelSourceUpstream, want: ModelSourceUpstream},
		{name: "mapping", source: ModelSourceMapping, want: ModelSourceMapping},
		{name: "invalid falls back", source: "foobar", want: ModelSourceRequested},
		{name: "empty falls back", source: "", want: ModelSourceRequested},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeModelSource(tc.source); got != tc.want {
				t.Fatalf("NormalizeModelSource(%q)=%q want %q", tc.source, got, tc.want)
			}
		})
	}
}

func TestUsageStatsJSONIncludesSplitCacheFields(t *testing.T) {
	stats := UsageStats{
		TotalRequests:            1,
		TotalInputTokens:         10,
		TotalOutputTokens:        20,
		TotalCacheTokens:         7,
		TotalCacheCreationTokens: 3,
		TotalCacheReadTokens:     4,
		TotalTokens:              37,
	}

	payload, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal UsageStats: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal UsageStats JSON: %v", err)
	}
	if got["total_cache_creation_tokens"] != float64(3) {
		t.Fatalf("total_cache_creation_tokens=%v want 3 in %s", got["total_cache_creation_tokens"], payload)
	}
	if got["total_cache_read_tokens"] != float64(4) {
		t.Fatalf("total_cache_read_tokens=%v want 4 in %s", got["total_cache_read_tokens"], payload)
	}
}
