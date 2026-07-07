package service

import (
	"os"
	"strconv"
	"strings"
)

const (
	failoverStatusCodesEnv        = "SUB2API_FAILOVER_STATUS_CODES"
	failoverExcludeStatusCodesEnv = "SUB2API_FAILOVER_EXCLUDE_STATUS_CODES"
)

// shouldFailoverStatusCode applies the runtime-configurable upstream HTTP failover policy.
//
// SUB2API_FAILOVER_STATUS_CODES defaults to ">=400".
// SUB2API_FAILOVER_EXCLUDE_STATUS_CODES defaults to "524".
//
// Supported tokens: 429, 400-499, >=500, <=499, 4xx, 5xx. Tokens are separated
// by comma, semicolon, pipe, or whitespace. Invalid tokens are ignored; if no
// include token is valid, the default include policy is used.
func shouldFailoverStatusCode(statusCode int) bool {
	if !statusCodeMatchesPolicy(statusCode, os.Getenv(failoverStatusCodesEnv), ">=400") {
		return false
	}
	return !statusCodeMatchesPolicy(statusCode, os.Getenv(failoverExcludeStatusCodesEnv), "524")
}

func statusCodeMatchesPolicy(statusCode int, raw string, defaultRaw string) bool {
	if raw == "" {
		raw = defaultRaw
	}
	matched, valid := false, false
	for _, token := range splitStatusPolicy(raw) {
		tokenMatched, tokenValid := statusPolicyTokenMatches(statusCode, token)
		if !tokenValid {
			continue
		}
		valid = true
		matched = matched || tokenMatched
	}
	if !valid && defaultRaw != "" && raw != defaultRaw {
		return statusCodeMatchesPolicy(statusCode, defaultRaw, "")
	}
	return matched
}

func splitStatusPolicy(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}

func statusPolicyTokenMatches(statusCode int, token string) (bool, bool) {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return false, false
	}
	if strings.HasSuffix(token, "xx") && len(token) == 3 {
		prefix, err := strconv.Atoi(token[:1])
		if err != nil {
			return false, false
		}
		start := prefix * 100
		return statusCode >= start && statusCode <= start+99, true
	}
	if strings.HasPrefix(token, ">=") || strings.HasPrefix(token, "<=") {
		n, err := strconv.Atoi(strings.TrimSpace(token[2:]))
		if err != nil {
			return false, false
		}
		if strings.HasPrefix(token, ">=") {
			return statusCode >= n, true
		}
		return statusCode <= n, true
	}
	if strings.HasPrefix(token, ">") || strings.HasPrefix(token, "<") {
		n, err := strconv.Atoi(strings.TrimSpace(token[1:]))
		if err != nil {
			return false, false
		}
		if strings.HasPrefix(token, ">") {
			return statusCode > n, true
		}
		return statusCode < n, true
	}
	if strings.Contains(token, "-") {
		parts := strings.SplitN(token, "-", 2)
		start, errStart := strconv.Atoi(strings.TrimSpace(parts[0]))
		end, errEnd := strconv.Atoi(strings.TrimSpace(parts[1]))
		if errStart != nil || errEnd != nil {
			return false, false
		}
		if start > end {
			start, end = end, start
		}
		return statusCode >= start && statusCode <= end, true
	}
	n, err := strconv.Atoi(token)
	if err != nil {
		return false, false
	}
	return statusCode == n, true
}
