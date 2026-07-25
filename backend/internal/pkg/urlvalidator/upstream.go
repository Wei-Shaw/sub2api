package urlvalidator

// UpstreamBaseURLPolicy is the allowlist policy for validating an upstream base URL.
//
// Present mirrors the caller's cfg==nil case: when false, validation takes the
// strict allowlist path (ValidateHTTPSURL) regardless of the other fields, which
// preserves the cfg-nil semantics of the original service-level helpers.
type UpstreamBaseURLPolicy struct {
	Present           bool
	Enabled           bool
	AllowInsecureHTTP bool
	UpstreamHosts     []string
	AllowPrivateHosts bool
}

// ValidateUpstreamBaseURL validates and normalizes an upstream base URL under the
// given policy.
//
// When the policy is present and the allowlist is disabled, only minimal format
// validation is performed (ValidateURLFormat). Otherwise the strict HTTPS allowlist
// path (ValidateHTTPSURL) is used. The returned error is the raw urlvalidator error;
// callers wrap it with whatever context they need.
func ValidateUpstreamBaseURL(raw string, p UpstreamBaseURLPolicy) (string, error) {
	if p.Present && !p.Enabled {
		return ValidateURLFormat(raw, p.AllowInsecureHTTP)
	}
	return ValidateHTTPSURL(raw, ValidationOptions{
		AllowedHosts:     p.UpstreamHosts,
		RequireAllowlist: true,
		AllowPrivate:     p.AllowPrivateHosts,
	})
}
