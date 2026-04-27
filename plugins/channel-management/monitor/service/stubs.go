package monitorservice

import "context"

// stubs.go provides build-time stubs for symbols that will be filled in by
// later commits as the checker / ssrf / runner files are ported. Keeping
// them in one file makes it obvious which TODOs remain — and lets earlier
// commits stay green so the V5 W6 staged migration can land incrementally.

// isSupportedProvider returns true if p is one of the recognised provider
// strings. Real implementation lives next to the provider adapter table in
// checker.go, which has not been ported yet.
func isSupportedProvider(p string) bool {
	switch p {
	case MonitorProviderOpenAI, MonitorProviderAnthropic, MonitorProviderGemini:
		return true
	}
	return false
}

// isPrivateOrLoopbackHost is the SSRF guard's host classification. The real
// version (152 lines, including DNS resolution + IP-range checks) lives in
// channel_monitor_ssrf.go and will be replaced by the W4 SafeOutboundHTTP
// SDK helper. Until that swap lands the validation step degrades to "always
// allow" so existing host-side validators still compile — the actual check
// will run inside SafeOutboundHTTP at request time.
func isPrivateOrLoopbackHost(ctx context.Context, host string) (bool, error) {
	_ = ctx
	_ = host
	return false, nil
}
