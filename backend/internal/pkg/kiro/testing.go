package kiro

import "testing"

// OverrideEndpointURLForTest replaces endpoints[i].URL for the duration
// of the test. Used by service-layer tests that need to redirect the
// gateway's outbound call to a local httptest server.
//
// Only exposed for tests — production code uses EndpointPreference()
// to read the immutable list.
func OverrideEndpointURLForTest(t *testing.T, i int, url string) {
	t.Helper()
	if i < 0 || i >= len(endpoints) {
		t.Fatalf("OverrideEndpointURLForTest: index %d out of range", i)
	}
	prev := endpoints[i].URL
	endpoints[i].URL = url
	t.Cleanup(func() { endpoints[i].URL = prev })
}

// profileURLForTest is the test-overridable copy of profileURL used by
// FetchProfile and FetchUsageLimits. Defaults to the package-level
// constant; OverrideProfileURLForTest swaps it temporarily.
var profileURLForTest = ""

// OverrideProfileURLForTest redirects FetchProfile / FetchUsageLimits to
// the supplied URL for the duration of the test. Production callers are
// unaffected (they always use the real profileURL constant).
func OverrideProfileURLForTest(t *testing.T, url string) {
	t.Helper()
	prev := profileURLForTest
	profileURLForTest = url
	t.Cleanup(func() { profileURLForTest = prev })
}

// resolvedProfileURL returns the URL the production helpers should hit.
// Returns the test override when one is active, the real URL otherwise.
func resolvedProfileURL() string {
	if profileURLForTest != "" {
		return profileURLForTest
	}
	return profileURL
}
