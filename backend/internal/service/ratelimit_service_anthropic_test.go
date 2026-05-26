package service

import (
	"net/http"
	"testing"
	"time"
)

func TestCalculateAnthropic429ResetTime_Only5hExceeded(t *testing.T) {
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "1.02")
	headers.Set("anthropic-ratelimit-unified-5h-reset", "1770998400")
	headers.Set("anthropic-ratelimit-unified-7d-utilization", "0.32")
	headers.Set("anthropic-ratelimit-unified-7d-reset", "1771549200")

	result := calculateAnthropic429ResetTime(headers)
	assertAnthropicResult(t, result, 1770998400)

	if result.fiveHourReset == nil || !result.fiveHourReset.Equal(time.Unix(1770998400, 0)) {
		t.Errorf("expected fiveHourReset=1770998400, got %v", result.fiveHourReset)
	}
}

func TestCalculateAnthropic429ResetTime_Only7dExceeded(t *testing.T) {
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "0.50")
	headers.Set("anthropic-ratelimit-unified-5h-reset", "1770998400")
	headers.Set("anthropic-ratelimit-unified-7d-utilization", "1.05")
	headers.Set("anthropic-ratelimit-unified-7d-reset", "1771549200")

	result := calculateAnthropic429ResetTime(headers)
	assertAnthropicResult(t, result, 1771549200)

	// fiveHourReset should still be populated for session window calculation
	if result.fiveHourReset == nil || !result.fiveHourReset.Equal(time.Unix(1770998400, 0)) {
		t.Errorf("expected fiveHourReset=1770998400, got %v", result.fiveHourReset)
	}
}

func TestCalculateAnthropic429ResetTime_BothExceeded(t *testing.T) {
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "1.10")
	headers.Set("anthropic-ratelimit-unified-5h-reset", "1770998400")
	headers.Set("anthropic-ratelimit-unified-7d-utilization", "1.02")
	headers.Set("anthropic-ratelimit-unified-7d-reset", "1771549200")

	result := calculateAnthropic429ResetTime(headers)
	assertAnthropicResult(t, result, 1771549200)
}

func TestCalculateAnthropic429ResetTime_NoPerWindowHeaders(t *testing.T) {
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-reset", "1771549200")

	result := calculateAnthropic429ResetTime(headers)
	if result != nil {
		t.Errorf("expected nil result when no per-window headers, got resetAt=%v", result.resetAt)
	}
}

func TestCalculateAnthropic429ResetTime_NoHeaders(t *testing.T) {
	result := calculateAnthropic429ResetTime(http.Header{})
	if result != nil {
		t.Errorf("expected nil result for empty headers, got resetAt=%v", result.resetAt)
	}
}

func TestCalculateAnthropic429ResetTime_SurpassedThreshold(t *testing.T) {
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-surpassed-threshold", "true")
	headers.Set("anthropic-ratelimit-unified-5h-reset", "1770998400")
	headers.Set("anthropic-ratelimit-unified-7d-surpassed-threshold", "false")
	headers.Set("anthropic-ratelimit-unified-7d-reset", "1771549200")

	result := calculateAnthropic429ResetTime(headers)
	assertAnthropicResult(t, result, 1770998400)
}

func TestCalculateAnthropic429ResetTime_UtilizationExactlyOne(t *testing.T) {
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "1.0")
	headers.Set("anthropic-ratelimit-unified-5h-reset", "1770998400")
	headers.Set("anthropic-ratelimit-unified-7d-utilization", "0.5")
	headers.Set("anthropic-ratelimit-unified-7d-reset", "1771549200")

	result := calculateAnthropic429ResetTime(headers)
	assertAnthropicResult(t, result, 1770998400)
}

func TestCalculateAnthropic429ResetTime_NeitherExceeded_UsesShorter(t *testing.T) {
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "0.95")
	headers.Set("anthropic-ratelimit-unified-5h-reset", "1770998400") // sooner
	headers.Set("anthropic-ratelimit-unified-7d-utilization", "0.80")
	headers.Set("anthropic-ratelimit-unified-7d-reset", "1771549200") // later

	result := calculateAnthropic429ResetTime(headers)
	assertAnthropicResult(t, result, 1770998400)
}

func TestCalculateAnthropic429ResetTime_Only5hResetHeader(t *testing.T) {
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "1.05")
	headers.Set("anthropic-ratelimit-unified-5h-reset", "1770998400")

	result := calculateAnthropic429ResetTime(headers)
	assertAnthropicResult(t, result, 1770998400)
}

func TestCalculateAnthropic429ResetTime_Only7dResetHeader(t *testing.T) {
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-7d-utilization", "1.03")
	headers.Set("anthropic-ratelimit-unified-7d-reset", "1771549200")

	result := calculateAnthropic429ResetTime(headers)
	assertAnthropicResult(t, result, 1771549200)

	if result.fiveHourReset != nil {
		t.Errorf("expected fiveHourReset=nil when no 5h headers, got %v", result.fiveHourReset)
	}
}

func TestIsAnthropicWindowExceeded(t *testing.T) {
	tests := []struct {
		name     string
		headers  http.Header
		window   string
		expected bool
	}{
		{
			name:     "utilization above 1.0",
			headers:  makeHeader("anthropic-ratelimit-unified-5h-utilization", "1.02"),
			window:   "5h",
			expected: true,
		},
		{
			name:     "utilization exactly 1.0",
			headers:  makeHeader("anthropic-ratelimit-unified-5h-utilization", "1.0"),
			window:   "5h",
			expected: true,
		},
		{
			name:     "utilization below 1.0",
			headers:  makeHeader("anthropic-ratelimit-unified-5h-utilization", "0.99"),
			window:   "5h",
			expected: false,
		},
		{
			name:     "surpassed-threshold true",
			headers:  makeHeader("anthropic-ratelimit-unified-7d-surpassed-threshold", "true"),
			window:   "7d",
			expected: true,
		},
		{
			name:     "surpassed-threshold True (case insensitive)",
			headers:  makeHeader("anthropic-ratelimit-unified-7d-surpassed-threshold", "True"),
			window:   "7d",
			expected: true,
		},
		{
			name:     "surpassed-threshold false",
			headers:  makeHeader("anthropic-ratelimit-unified-7d-surpassed-threshold", "false"),
			window:   "7d",
			expected: false,
		},
		{
			name:     "no headers",
			headers:  http.Header{},
			window:   "5h",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isAnthropicWindowExceeded(tc.headers, tc.window)
			if got != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

// assertAnthropicResult is a test helper that verifies the result is non-nil and
// has the expected resetAt unix timestamp.
func assertAnthropicResult(t *testing.T, result *anthropic429Result, wantUnix int64) {
	t.Helper()
	if result == nil {
		t.Fatal("expected non-nil result")
		return // unreachable, but satisfies staticcheck SA5011
	}
	want := time.Unix(wantUnix, 0)
	if !result.resetAt.Equal(want) {
		t.Errorf("expected resetAt=%v, got %v", want, result.resetAt)
	}
}

func makeHeader(key, value string) http.Header {
	h := http.Header{}
	h.Set(key, value)
	return h
}
