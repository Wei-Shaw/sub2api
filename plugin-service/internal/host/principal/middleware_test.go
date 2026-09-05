package principal

import (
	"net/http"
	"testing"
)

func TestDefaultResolveMainSiteBaseCandidates(t *testing.T) {
	got := defaultResolveMainSiteBaseCandidates(nil)
	want := []string{"http://sub2api:8080", "http://localhost:8080", "http://127.0.0.1:8080"}

	if len(got) != len(want) {
		t.Fatalf("candidate count = %d, want %d; values=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoadCurrentPrincipalUsesForwardedPrincipalHeadersFirst(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://plugin-service/plugins/image-generation/api/me", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("X-Sub2api-User-Id", "7")
	req.Header.Set("X-Sub2api-User-Role", "admin")
	req.Header.Set("X-Sub2api-User-Email", "admin@example.com")
	req.Header.Set("X-Sub2api-User-Name", "tester")

	got, err := LoadCurrentPrincipal(req, "image-generation")
	if err != nil {
		t.Fatalf("LoadCurrentPrincipal() error = %v", err)
	}
	if got.UserID != 7 || got.Role != "admin" || got.Email != "admin@example.com" || got.Username != "tester" || got.Plugin != "image-generation" {
		t.Fatalf("LoadCurrentPrincipal() = %#v", got)
	}
}
