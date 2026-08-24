package provider

import (
	"net/url"
	"testing"
)

func TestBuildDogPayRedirectURLsUsesPaymentResultContext(t *testing.T) {
	successURL, failureURL := buildDogPayRedirectURLs(
		"https://app.example.com/payment/result?order_id=42&out_trade_no=order-42&resume_token=resume-token#ignored",
	)

	for name, rawURL := range map[string]string{
		"success": successURL,
		"failure": failureURL,
	} {
		t.Run(name, func(t *testing.T) {
			parsed, err := url.Parse(rawURL)
			if err != nil {
				t.Fatalf("url.Parse returned error: %v", err)
			}
			if parsed.Path != "/payment/result" {
				t.Fatalf("path = %q, want /payment/result", parsed.Path)
			}
			if parsed.Fragment != "" {
				t.Fatalf("fragment = %q, want empty", parsed.Fragment)
			}
			query := parsed.Query()
			if query.Get("order_id") != "42" || query.Get("out_trade_no") != "order-42" || query.Get("resume_token") != "resume-token" {
				t.Fatalf("order context was not preserved: %s", parsed.RawQuery)
			}
			wantStatus := name
			if query.Get("status") != wantStatus {
				t.Fatalf("status = %q, want %q", query.Get("status"), wantStatus)
			}
		})
	}
}
