package service

// isAccountFailoverStatus reports whether an upstream HTTP status should trigger
// switching to another account (auth/rate-limit/overloaded/5xx).
func isAccountFailoverStatus(statusCode int) bool {
	switch statusCode {
	case 401, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
	}
}

// isTransientUpstreamStatus reports whether status is a common transient
// upstream error (rate limit / gateway / overloaded).
func isTransientUpstreamStatus(statusCode int) bool {
	switch statusCode {
	case 429, 500, 502, 503, 504, 529:
		return true
	default:
		return false
	}
}
