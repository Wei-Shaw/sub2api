package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

type kimiUsageTokenStub struct {
	token string
	err   error
}

func (s *kimiUsageTokenStub) GetAccessToken(context.Context, *Account) (string, error) {
	return s.token, s.err
}

type kimiUsageHTTPStub struct {
	request  *http.Request
	status   int
	response string
}

type kimiUsageAccountRepoStub struct {
	AccountRepository
	updates chan map[string]any
}

func (s *kimiUsageAccountRepoStub) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	s.updates <- updates
	return nil
}

func (s *kimiUsageHTTPStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.request = req
	return &http.Response{
		StatusCode: s.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(s.response)),
	}, nil
}

func (s *kimiUsageHTTPStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestBuildKimiUsageInfoMapsOfficialWindows(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	payload := &kimiUsageResponse{
		Usage: map[string]any{
			"limit":     "100",
			"used":      "42",
			"resetTime": "2026-07-28T12:00:00Z",
		},
		Limits: []kimiUsageLimit{{
			Detail: map[string]any{
				"limit":     "100",
				"remaining": "75",
				"resetTime": "2026-07-21T17:00:00Z",
			},
			Window: map[string]any{
				"duration": 300.0,
				"timeUnit": "TIME_UNIT_MINUTE",
			},
		}},
	}
	payload.User.Membership.Level = "LEVEL_INTERMEDIATE"

	usage := buildKimiUsageInfo(payload, now)
	if usage.SevenDay == nil || usage.SevenDay.Utilization != 42 {
		t.Fatalf("weekly utilization = %#v, want 42", usage.SevenDay)
	}
	if usage.SevenDay.UsedRequests != 42 || usage.SevenDay.LimitRequests != 100 {
		t.Fatalf("weekly requests = %d/%d, want 42/100", usage.SevenDay.UsedRequests, usage.SevenDay.LimitRequests)
	}
	if usage.FiveHour == nil || usage.FiveHour.Utilization != 25 {
		t.Fatalf("5h utilization = %#v, want 25", usage.FiveHour)
	}
	if usage.FiveHour.RemainingSeconds != int((5 * time.Hour).Seconds()) {
		t.Fatalf("5h remaining seconds = %d, want %d", usage.FiveHour.RemainingSeconds, int((5 * time.Hour).Seconds()))
	}
	if usage.SubscriptionTier != "INTERMEDIATE" || usage.SubscriptionTierRaw != "LEVEL_INTERMEDIATE" {
		t.Fatalf("membership = %q/%q, want INTERMEDIATE/LEVEL_INTERMEDIATE", usage.SubscriptionTier, usage.SubscriptionTierRaw)
	}
}

func TestNormalizeKimiMembershipLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw        string
		normalized string
		original   string
	}{
		{raw: " LEVEL_INTERMEDIATE ", normalized: "INTERMEDIATE", original: "LEVEL_INTERMEDIATE"},
		{raw: "level_plus", normalized: "PLUS", original: "level_plus"},
		{raw: "", normalized: "", original: ""},
	}
	for _, tt := range tests {
		normalized, original := normalizeKimiMembershipLevel(tt.raw)
		if normalized != tt.normalized || original != tt.original {
			t.Fatalf("normalizeKimiMembershipLevel(%q) = %q/%q, want %q/%q", tt.raw, normalized, original, tt.normalized, tt.original)
		}
	}
}

func TestFetchKimiUsageUsesRefreshedTokenAndFingerprint(t *testing.T) {
	t.Setenv(kimi.EnvAllowUnsafeURLOverrides, "true")
	httpStub := &kimiUsageHTTPStub{
		status: http.StatusOK,
		response: `{
			"usage":{"limit":"100","used":"10","resetTime":"2026-07-28T12:00:00Z"},
			"limits":[{"detail":{"limit":"100","remaining":"90","resetTime":"2026-07-21T17:00:00Z"},"window":{"duration":300,"timeUnit":"TIME_UNIT_MINUTE"}}],
			"user":{"membership":{"level":"LEVEL_INTERMEDIATE"}}
		}`,
	}
	repo := &kimiUsageAccountRepoStub{updates: make(chan map[string]any, 1)}
	service := &AccountUsageService{
		kimiTokenProvider: &kimiUsageTokenStub{token: "fresh-token"},
		httpUpstream:      httpStub,
		accountRepo:       repo,
	}
	account := &Account{
		ID:          9,
		Platform:    PlatformKimi,
		Type:        AccountTypeOAuth,
		Concurrency: 2,
		Credentials: map[string]any{
			"base_url":  "http://127.0.0.1:9876/coding/v1",
			"device_id": "stable-device-id",
		},
	}

	usage, err := service.fetchKimiUsage(context.Background(), account)
	if err != nil {
		t.Fatalf("fetchKimiUsage() error = %v", err)
	}
	if usage.FiveHour == nil || usage.SevenDay == nil {
		t.Fatalf("fetchKimiUsage() windows = %#v", usage)
	}
	if usage.SubscriptionTier != "INTERMEDIATE" || usage.SubscriptionTierRaw != "LEVEL_INTERMEDIATE" {
		t.Fatalf("fetchKimiUsage() membership = %q/%q", usage.SubscriptionTier, usage.SubscriptionTierRaw)
	}
	if got := account.Extra["kimi_membership_level"]; got != "LEVEL_INTERMEDIATE" {
		t.Fatalf("account membership snapshot = %#v, want LEVEL_INTERMEDIATE", got)
	}
	select {
	case updates := <-repo.updates:
		if got := updates["kimi_membership_level"]; got != "LEVEL_INTERMEDIATE" {
			t.Fatalf("persisted membership = %#v, want LEVEL_INTERMEDIATE", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for membership persistence")
	}
	if httpStub.request == nil {
		t.Fatal("expected upstream request")
	}
	if got := httpStub.request.URL.String(); got != "http://127.0.0.1:9876/coding/v1/usages" {
		t.Fatalf("request URL = %q", got)
	}
	if got := httpStub.request.Header.Get("Authorization"); got != "Bearer fresh-token" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := httpStub.request.Header.Get("X-Msh-Device-Id"); got != "stable-device-id" {
		t.Fatalf("X-Msh-Device-Id = %q", got)
	}
	if got := httpStub.request.Header.Get("User-Agent"); got != kimi.UserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, kimi.UserAgent)
	}
}
