package xai

import (
	"context"
	"encoding/hex"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseGRPCWebBillingResponse(t *testing.T) {
	t.Parallel()

	raw, err := hex.DecodeString(
		"0a3f0df0a7a64012001a002206088097f3d0062a060880b191d2063a07080215583964403a070801150e2dd23f421208011206088097f3d0061a060880b191d206",
	)
	if err != nil {
		t.Fatalf("hex decode: %v", err)
	}
	frame := appendGRPCWebDataFrame(nil, raw)
	frame = appendGRPCWebTrailerFrame(frame, []byte("grpc-status:0\r\n"))

	now := time.Unix(1781069112, 0).UTC()
	snapshot, err := parseGRPCWebBillingResponse(frame, now)
	if err != nil {
		t.Fatalf("parseGRPCWebBillingResponse() error = %v", err)
	}
	if snapshot.UsedPercent == nil || math.Abs(*snapshot.UsedPercent-5.208000183105469) > 0.001 {
		t.Fatalf("UsedPercent = %#v, want ~5.208", snapshot.UsedPercent)
	}
	if snapshot.ResetsAt == nil || snapshot.ResetsAt.UTC().Format(time.RFC3339) != "2026-07-01T00:00:00Z" {
		t.Fatalf("ResetsAt = %#v, want 2026-07-01T00:00:00Z", snapshot.ResetsAt)
	}
	if snapshot.PeriodStart == nil || snapshot.PeriodStart.UTC().Format(time.RFC3339) != "2026-06-01T00:00:00Z" {
		t.Fatalf("PeriodStart = %#v, want 2026-06-01T00:00:00Z", snapshot.PeriodStart)
	}
}

func TestFetchBillingSummaryPrefersREST(t *testing.T) {
	restServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("REST method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"config": {
				"monthlyLimit": { "val": "15000" },
				"used": { "val": "13255" },
				"onDemandCap": { "val": "0" },
				"billingPeriodStart": "2026-06-01T00:00:00Z",
				"billingPeriodEnd": "2026-07-01T00:00:00Z"
			}
		}`))
	}))
	defer restServer.Close()

	webCalled := false
	webServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer webServer.Close()

	oldREST := BillingRESTEndpoint
	oldWeb := BillingWebGRPCEndpoint
	BillingRESTEndpoint = restServer.URL
	BillingWebGRPCEndpoint = webServer.URL
	defer func() {
		BillingRESTEndpoint = oldREST
		BillingWebGRPCEndpoint = oldWeb
	}()

	summary, err := FetchBillingSummary(context.Background(), "token", "")
	if err != nil {
		t.Fatalf("FetchBillingSummary() error = %v", err)
	}
	if webCalled {
		t.Fatal("expected REST success without calling web fallback")
	}
	if summary.Source != "rest" {
		t.Fatalf("Source = %q, want rest", summary.Source)
	}
	if summary.MonthlyLimitCents == nil || *summary.MonthlyLimitCents != 15000 {
		t.Fatalf("MonthlyLimitCents = %#v, want 15000", summary.MonthlyLimitCents)
	}
}

func TestFetchBillingSummaryFallsBackToWebGRPC(t *testing.T) {
	raw, err := hex.DecodeString(
		"0a3f0df0a7a64012001a002206088097f3d0062a060880b191d2063a07080215583964403a070801150e2dd23f421208011206088097f3d0061a060880b191d206",
	)
	if err != nil {
		t.Fatalf("hex decode: %v", err)
	}
	frame := appendGRPCWebDataFrame(nil, raw)
	frame = appendGRPCWebTrailerFrame(frame, []byte("grpc-status:0\r\n"))

	restServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"The operation was cancelled","error":"Timeout expired"}`))
	}))
	defer restServer.Close()

	webServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("web method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/grpc-web+proto")
		_, _ = w.Write(frame)
	}))
	defer webServer.Close()

	oldREST := BillingRESTEndpoint
	oldWeb := BillingWebGRPCEndpoint
	BillingRESTEndpoint = restServer.URL
	BillingWebGRPCEndpoint = webServer.URL
	defer func() {
		BillingRESTEndpoint = oldREST
		BillingWebGRPCEndpoint = oldWeb
	}()

	summary, err := FetchBillingSummary(context.Background(), "token", "")
	if err != nil {
		t.Fatalf("FetchBillingSummary() error = %v", err)
	}
	if summary.Source != "web_grpc" {
		t.Fatalf("Source = %q, want web_grpc", summary.Source)
	}
	if summary.UsedPercent == nil || math.Abs(*summary.UsedPercent-5.208000183105469) > 0.001 {
		t.Fatalf("UsedPercent = %#v, want ~5.208", summary.UsedPercent)
	}
	if summary.BillingPeriodEnd == nil || summary.BillingPeriodEnd.UTC().Format(time.RFC3339) != "2026-07-01T00:00:00Z" {
		t.Fatalf("BillingPeriodEnd = %#v, want 2026-07-01T00:00:00Z", summary.BillingPeriodEnd)
	}
}

func appendGRPCWebDataFrame(dst, payload []byte) []byte {
	dst = append(dst, 0x00)
	dst = append(dst, byte(len(payload)>>24), byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload)))
	return append(dst, payload...)
}

func appendGRPCWebTrailerFrame(dst, payload []byte) []byte {
	dst = append(dst, 0x80)
	dst = append(dst, byte(len(payload)>>24), byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload)))
	return append(dst, payload...)
}

func TestShouldRetryBillingError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "timeout body",
			err:  &BillingHTTPError{StatusCode: http.StatusBadRequest, Body: `{"error":"Timeout expired"}`},
			want: true,
		},
		{
			name: "unauthorized",
			err:  &BillingHTTPError{StatusCode: http.StatusUnauthorized, Body: `{"error":"expired token"}`},
			want: false,
		},
		{
			name: "gateway timeout",
			err:  &BillingHTTPError{StatusCode: http.StatusGatewayTimeout, Body: "upstream timeout"},
			want: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldRetryBillingError(tc.err); got != tc.want {
				t.Fatalf("shouldRetryBillingError() = %v, want %v", got, tc.want)
			}
		})
	}
}
