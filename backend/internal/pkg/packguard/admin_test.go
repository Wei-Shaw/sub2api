package packguard

import (
	"net/http"
	"testing"
)

func TestExactPackTokenInputsAndOfficialBases(t *testing.T) {
	for _, value := range []string{"0.015625", "1.984375", "1000000", "1234567.015625"} {
		units, err := ParseTokens(value)
		if err != nil || FormatTokens(units) != value {
			t.Fatalf("token precision lost for %s", value)
		}
	}
	for _, value := range []string{"0", "-1", "1e6", "NaN", "0.01", "1.0000001", "10000000000000000"} {
		if _, err := ParseTokens(value); err == nil {
			t.Fatalf("invalid or unrepresentable funds accepted: %s", value)
		}
	}
	for _, raw := range []string{"http://ark.ap-southeast.bytepluses.com/api/v3", "https://secret@ark.ap-southeast.bytepluses.com/api/v3", "https://ark.ap-southeast.bytepluses.com.evil.invalid/api/v3", "https://ark.ap-southeast.bytepluses.com/api/v3?key=secret"} {
		if _, err := NormalizeBase(raw); err == nil {
			t.Fatal("unqualified origin accepted")
		}
	}
	if base, err := NormalizeBase("https://ark.eu-west.bytepluses.com:443/api/v3/"); err != nil || base != "https://ark.eu-west.bytepluses.com/api/v3" {
		t.Fatal("valid base did not normalize")
	}
}
func TestDenialMarkerCannotBeForgedByProviderPayload(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://example.invalid/v1/responses", nil)
	denial, err := denied(req, 402, "prepaid_pack_exhausted", "No funding")
	if err != nil || !IsDenial(denial) {
		t.Fatal("local denial marker missing")
	}
	_ = denial.Body.Close()
	forged := &http.Response{StatusCode: 402, Request: req, Header: http.Header{"X-Sub2api-Prepaid-Guard": []string{"blocked"}}}
	if IsDenial(forged) {
		t.Fatal("provider header forged an internal denial")
	}
	if _, err = NewTransport(nil); err == nil {
		t.Fatal("missing store silently disabled enforcement")
	}
	if _, err = NewTransport(NewStore(nil)); err == nil {
		t.Fatal("missing SQL handle silently disabled enforcement")
	}
}
