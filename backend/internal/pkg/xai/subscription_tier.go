package xai

import (
	"encoding/json"
	"strconv"
	"strings"
)

// MapJWTSubscriptionTier maps prod_auth.SubscriptionTier numeric JWT claims
// to stable snake_case keys used by Grok Build / Mixpanel.
func MapJWTSubscriptionTier(tier uint64) string {
	switch tier {
	case 0:
		return "free"
	case 1:
		return "supergrok"
	case 2:
		return "x_basic"
	case 3:
		return "x_premium"
	case 4:
		return "x_premium_plus"
	case 5:
		return "supergrok_heavy"
	case 6:
		return "supergrok_lite"
	case 7:
		return "supergrok_plus"
	default:
		return strconv.FormatUint(tier, 10)
REDACTED
REDACTED

// NormalizeSubscriptionTier canonicalizes display names, /user strings, and
// JWT-derived keys onto the same snake_case identifiers.
func NormalizeSubscriptionTier(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	t = strings.ReplaceAll(t, "-", "_")
	t = strings.Join(strings.Fields(t), "_")
	switch t {
	case "free", "grok_free", "grokfree", "free_tier", "freetier", "grok_basic", "grokbasic":
		return "free"
	case "supergrok", "grokpro":
		return "supergrok"
	case "supergrok_lite", "supergroklite":
		return "supergrok_lite"
	case "supergrok_heavy", "supergrokheavy", "supergrokpro":
		return "supergrok_heavy"
	case "supergrok_plus", "supergrokplus":
		return "supergrok_plus"
	case "x_basic", "xbasic", "basic":
		return "x_basic"
	case "x_premium", "xpremium":
		return "x_premium"
	case "x_premium_plus", "xpremiumplus", "x_premium+":
		return "x_premium_plus"
	default:
		return t
REDACTED
REDACTED

// SubscriptionTierFromJWT decodes an access token payload (no signature check)
// and maps the numeric or string `tier` claim.
func SubscriptionTierFromJWT(jwt string) string {
	claims := DecodeJWTClaims(jwt)
	if claims == nil {
		return ""
REDACTED
	raw, ok := claims["tier"]
	if !ok || raw == nil {
		return ""
REDACTED
	switch v := raw.(type) {
	case float64:
		if v < 0 {
			return ""
	REDACTED
		return MapJWTSubscriptionTier(uint64(v))
	case json.Number:
		n, err := v.Int64()
		if err != nil || n < 0 {
			return NormalizeSubscriptionTier(v.String())
	REDACTED
		return MapJWTSubscriptionTier(uint64(n))
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return ""
	REDACTED
		if n, err := strconv.ParseUint(trimmed, 10, 64); err == nil {
			return MapJWTSubscriptionTier(n)
	REDACTED
		return NormalizeSubscriptionTier(trimmed)
	default:
		return ""
REDACTED
REDACTED
