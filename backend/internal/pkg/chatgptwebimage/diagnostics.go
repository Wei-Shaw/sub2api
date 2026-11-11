package chatgptwebimage

// Diagnostic contains only bounded metadata suitable for logs and client
// errors. In particular it never includes a response body, cause, conversation
// identifier, signed asset URL, prompt, or account credentials.
type Diagnostic struct {
	Stage      string
	Kind       string
	StatusCode int
	Accepted   bool
}

// Diagnostic allowlists identifiers even though the provider currently sets
// them internally. Future upstream-derived fields must not become log content.
func (e *Error) Diagnostic() Diagnostic {
	d := Diagnostic{Stage: "unknown", Kind: "unknown"}
	if e == nil {
		return d
	}
	switch e.Stage {
	case "credentials", "initialize", "validate", "bootstrap", "requirements", "prepare", "conversation", "poll", "result", "download", "download_lookup":
		d.Stage = e.Stage
	}
	switch e.Kind {
	case "credentials_unavailable", "invalid_configuration", "invalid_request", "transport", "timeout_or_canceled", "upstream_http", "model_not_found", "authentication_or_challenge", "rate_limited", "invalid_response", "entropy", "proof_failed", "interactive_challenge", "missing_requirements_token", "missing_conduit", "invalid_stream", "no_image", "invalid_image", "unsupported_image", "untrusted_download_origin", "invalid_download_url", "too_many_redirects":
		d.Kind = e.Kind
	}
	if e.StatusCode >= 100 && e.StatusCode <= 599 {
		d.StatusCode = e.StatusCode
	}
	d.Accepted = e.Accepted
	return d
}
