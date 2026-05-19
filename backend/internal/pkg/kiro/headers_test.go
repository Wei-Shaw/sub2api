package kiro

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildStreamingHeaderValues_IncludesMachineID(t *testing.T) {
	v := BuildStreamingHeaderValues("abc-123", "q.us-east-1.amazonaws.com")
	if !strings.HasSuffix(v.UserAgent, "-abc-123") {
		t.Fatalf("UserAgent should end with -abc-123, got %q", v.UserAgent)
	}
	if !strings.HasSuffix(v.AmzUserAgent, "-abc-123") {
		t.Fatalf("AmzUserAgent should end with -abc-123, got %q", v.AmzUserAgent)
	}
	if v.Host != "q.us-east-1.amazonaws.com" {
		t.Fatalf("Host = %q", v.Host)
	}
	if !strings.Contains(v.UserAgent, "aws-sdk-js/") {
		t.Fatalf("UserAgent missing aws-sdk-js: %q", v.UserAgent)
	}
}

func TestBuildStreamingHeaderValues_NoMachineID(t *testing.T) {
	v := BuildStreamingHeaderValues("", "host")
	if strings.Contains(v.UserAgent, "KiroIDE--") {
		t.Fatalf("empty machine id should not produce double dash: %q", v.UserAgent)
	}
}

func TestApplyBaseHeaders_SetsExpectedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://q.example/path", strings.NewReader("{}"))
	v := BuildStreamingHeaderValues("m1", "q.example")
	ApplyBaseHeaders(req, "tok-1", v)

	if req.Header.Get("Authorization") != "Bearer tok-1" {
		t.Fatalf("Authorization = %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("x-amzn-codewhisperer-optout") != "true" {
		t.Fatalf("x-amzn-codewhisperer-optout missing")
	}
	if req.Header.Get("x-amzn-kiro-agent-mode") != "vibe" {
		t.Fatalf("x-amzn-kiro-agent-mode missing")
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type wrong")
	}
	if req.Header.Get("Amz-Sdk-Invocation-Id") == "" {
		t.Fatalf("Amz-Sdk-Invocation-Id should be set")
	}
	if req.Host != "q.example" {
		t.Fatalf("req.Host = %q", req.Host)
	}
}

func TestApplyBaseHeaders_OmitsAuthForEmptyToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://x/y", strings.NewReader(""))
	ApplyBaseHeaders(req, "", BuildStreamingHeaderValues("", ""))
	if req.Header.Get("Authorization") != "" {
		t.Fatalf("Authorization should be empty when token absent, got %q", req.Header.Get("Authorization"))
	}
}

func TestGenerateMachineID_ReturnsUUID(t *testing.T) {
	id := GenerateMachineID()
	if len(id) != 36 {
		t.Fatalf("expected 36-char UUID, got %q", id)
	}
}
