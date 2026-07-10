package service

import (
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ResultURLExpirySourceURLQuery  = "url_query"
	ResultURLExpirySourceEstimated = "estimated"
	ResultURLExpirySourceUnknown   = "unknown"

	// DefaultSeedanceResultURLTTL is used when the CDN URL has no signed expiry params.
	DefaultSeedanceResultURLTTL = 24 * time.Hour
)

// ParseResultURLExpiry extracts an expiry time from a result URL when possible.
// Prefer X-Amz-Date + X-Amz-Expires (or lowercase variants). Fall back to
// completedAt + 24h with source "estimated". Empty URL → nil, "unknown".
func ParseResultURLExpiry(resultURL string, completedAt *time.Time) (expiresAt *time.Time, source string) {
	resultURL = strings.TrimSpace(resultURL)
	if resultURL == "" {
		return nil, ResultURLExpirySourceUnknown
	}
	if t, ok := parseSignedURLExpiry(resultURL); ok {
		return &t, ResultURLExpirySourceURLQuery
	}
	if completedAt != nil && !completedAt.IsZero() {
		est := completedAt.UTC().Add(DefaultSeedanceResultURLTTL)
		return &est, ResultURLExpirySourceEstimated
	}
	return nil, ResultURLExpirySourceUnknown
}

// ReliabilityRemoteAssetAvailable mirrors the admin/API delivery projection:
// empty URL is unavailable; a known/estimated expiry that is not after now is
// unavailable; unknown expiry remains an allowed remote fallback.
func ReliabilityRemoteAssetAvailable(resultURL string, completedAt *time.Time, now time.Time) bool {
	if strings.TrimSpace(resultURL) == "" {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	expiresAt, _ := ParseResultURLExpiry(resultURL, completedAt)
	return expiresAt == nil || expiresAt.After(now)
}

func parseSignedURLExpiry(raw string) (time.Time, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return time.Time{}, false
	}
	q := u.Query()
	expiresSec := firstQueryInt(q, "X-Amz-Expires", "x-amz-expires", "Expires", "expires")
	if expiresSec <= 0 {
		return time.Time{}, false
	}
	dateRaw := firstQueryValue(q, "X-Amz-Date", "x-amz-date")
	var start time.Time
	if dateRaw != "" {
		if t, err := time.Parse("20060102T150405Z", dateRaw); err == nil {
			start = t.UTC()
		}
	}
	if start.IsZero() {
		// Some CDNs use unix epoch Expires without Amz-Date.
		if epoch := firstQueryInt(q, "Expires", "expires"); epoch > 1_000_000_000 {
			return time.Unix(epoch, 0).UTC(), true
		}
		return time.Time{}, false
	}
	return start.Add(time.Duration(expiresSec) * time.Second), true
}

func firstQueryValue(q url.Values, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(q.Get(key)); v != "" {
			return v
		}
	}
	return ""
}

func firstQueryInt(q url.Values, keys ...string) int64 {
	raw := firstQueryValue(q, keys...)
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
