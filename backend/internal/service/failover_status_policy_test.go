package service

import "testing"

func TestShouldFailoverStatusCodeDefaultPolicy(t *testing.T) {
	t.Setenv(failoverStatusCodesEnv, "")
	t.Setenv(failoverExcludeStatusCodesEnv, "")

	tests := []struct {
		status int
		want   bool
	}{
		{399, false},
		{400, true},
		{404, true},
		{500, true},
		{524, false},
		{529, true},
	}

	for _, tt := range tests {
		if got := shouldFailoverStatusCode(tt.status); got != tt.want {
			t.Fatalf("status %d: got %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestShouldFailoverStatusCodeEnvOverrides(t *testing.T) {
	t.Setenv(failoverStatusCodesEnv, "401, 429, 500-599")
	t.Setenv(failoverExcludeStatusCodesEnv, "503|529")

	tests := []struct {
		status int
		want   bool
	}{
		{400, false},
		{401, true},
		{429, true},
		{500, true},
		{503, false},
		{524, true},
		{529, false},
	}

	for _, tt := range tests {
		if got := shouldFailoverStatusCode(tt.status); got != tt.want {
			t.Fatalf("status %d: got %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestShouldFailoverStatusCodeSupportsClassAndComparisons(t *testing.T) {
	t.Setenv(failoverStatusCodesEnv, "4xx >=500")
	t.Setenv(failoverExcludeStatusCodesEnv, "<=401")

	tests := []struct {
		status int
		want   bool
	}{
		{399, false},
		{400, false},
		{401, false},
		{402, true},
		{499, true},
		{500, true},
	}

	for _, tt := range tests {
		if got := shouldFailoverStatusCode(tt.status); got != tt.want {
			t.Fatalf("status %d: got %v, want %v", tt.status, got, tt.want)
		}
	}
}
