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
