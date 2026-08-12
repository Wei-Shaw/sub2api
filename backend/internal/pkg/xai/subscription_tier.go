package xai

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// GrokQuotaSignalMaxAge bounds how long a grok-4.5 Responses window can
// influence SuperGrok vs Heavy inference.
const GrokQuotaSignalMaxAge = 24 * time.Hour

const (
	grok45ResponsesModel       = "grok-4.5"
	grokHeavyQuotaRequestLimit int64 = 8_300
	grokHeavyQuotaTokenLimit   int64 = 53_000_000
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
	case "supergrok_heavy", "supergrokheavy":
		return "supergrok_heavy"
	case "supergrok_pro", "supergrokpro":
		return "supergrok_pro"
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

// CanonicalGrokPlan resolves SuperGrok vs Heavy when the provider label is
// ambiguous (SuperGrokPro). JWT numeric claims are applied by the caller first.
// Monthly $150/$1500 limits still win when present.
// Rate-limit windows are only used when they came from grok-4.5 Responses.
func CanonicalGrokPlan(monthlyLimitCents *float64, subscriptionTier string, quota *QuotaSnapshot) string {
	if plan := resolvePlan(monthlyLimitCents); plan != "" {
		return NormalizeSubscriptionTier(plan)
REDACTED

	normalized := NormalizeSubscriptionTier(subscriptionTier)
	switch normalized {
	case "free", "x_basic":
		return "free"
	case "supergrok_heavy":
		return "supergrok_heavy"
	case "supergrok_lite":
		return "supergrok_lite"
	case "supergrok_plus":
		return "supergrok_plus"
REDACTED

	if isAmbiguousGrokPaidPlan(normalized) {
		if hint := Grok45ResponsesPlanHint(quota, time.Time{REDACTED); hint != "" {
			return hint
	REDACTED
		return "supergrok"
REDACTED
	return ""
REDACTED

func isAmbiguousGrokPaidPlan(normalized string) bool {
	switch normalized {
	case "supergrok", "supergrok_pro", "paid", "pro":
		return true
	default:
		return false
REDACTED
REDACTED

// IsGrok45ResponsesQuotaModel reports whether model is the grok-4.5 Responses
// id (or a dated grok-4.5-* variant). Empty and other families are false.
func IsGrok45ResponsesQuotaModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(StripGrokProviderPrefix(model)))
	return m == grok45ResponsesModel || strings.HasPrefix(m, grok45ResponsesModel+"-")
REDACTED

// Grok45ResponsesPlanHint returns SuperGrok / Heavy inferred from a grok-4.5
// Responses window. Other models' limits are ignored.
func Grok45ResponsesPlanHint(quota *QuotaSnapshot, now time.Time) string {
	if quota == nil {
		return ""
REDACTED
	if plan := NormalizeSubscriptionTier(quota.PlanFrom45Responses); plan == "supergrok" || plan == "supergrok_heavy" {
		if isQuotaTimestampFresh(quota.PlanFrom45ResponsesAt, now) {
			return plan
	REDACTED
REDACTED
	if !IsGrok45ResponsesQuotaModel(quota.Model) || !IsQuotaSnapshotFresh(quota, now) {
		return ""
REDACTED
	if quotaLooksLikeGrokHeavy(quota) {
		return "supergrok_heavy"
REDACTED
	return ""
REDACTED

// ApplyGrok45ResponsesPlanSignal records a grok-4.5 Heavy/SuperGrok hint, or
// copies the previous 4.5 hint when this observation is a different model.
func (s *QuotaSnapshot) ApplyGrok45ResponsesPlanSignal(prev *QuotaSnapshot) {
	if s == nil {
		return
REDACTED
	observedAt := firstNonEmptyQuotaTime(s.LastHeadersSeenAt, s.UpdatedAt)
	if IsGrok45ResponsesQuotaModel(s.Model) && quotaHasLimitWindow(s) {
		if quotaLooksLikeGrokHeavy(s) {
			s.PlanFrom45Responses = "supergrok_heavy"
			s.PlanFrom45ResponsesAt = observedAt
			return
	REDACTED
		s.PlanFrom45Responses = "supergrok"
		s.PlanFrom45ResponsesAt = observedAt
		return
REDACTED
	if prev != nil && strings.TrimSpace(prev.PlanFrom45Responses) != "" {
		s.PlanFrom45Responses = prev.PlanFrom45Responses
		s.PlanFrom45ResponsesAt = prev.PlanFrom45ResponsesAt
REDACTED
REDACTED

// QuotaSnapshotObservedAt prefers LastHeadersSeenAt over UpdatedAt so a later
// snapshot rewrite cannot refresh a stale Heavy window.
func QuotaSnapshotObservedAt(snapshot *QuotaSnapshot) (time.Time, bool) {
	if snapshot == nil {
		return time.Time{REDACTED, false
REDACTED
	return parseQuotaTimestamp(firstNonEmptyQuotaTime(snapshot.LastHeadersSeenAt, snapshot.UpdatedAt))
REDACTED

// IsQuotaSnapshotFresh reports whether a quota signal is recent enough to
// distinguish SuperGrok from Heavy.
func IsQuotaSnapshotFresh(snapshot *QuotaSnapshot, now time.Time) bool {
	observedAt, ok := QuotaSnapshotObservedAt(snapshot)
	if !ok {
		return false
REDACTED
	return isTimeFresh(observedAt, now)
REDACTED

func isQuotaTimestampFresh(raw string, now time.Time) bool {
	parsed, ok := parseQuotaTimestamp(raw)
	if !ok {
		return false
REDACTED
	return isTimeFresh(parsed, now)
REDACTED

func parseQuotaTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{REDACTED, false
REDACTED
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{REDACTED, false
REDACTED
	return parsed, true
REDACTED

func isTimeFresh(observedAt, now time.Time) bool {
	if now.IsZero() {
		now = time.Now()
REDACTED
	age := now.Sub(observedAt)
	return age <= GrokQuotaSignalMaxAge && age >= -5*time.Minute
REDACTED

func quotaHasLimitWindow(quota *QuotaSnapshot) bool {
	if quota == nil {
		return false
REDACTED
	if quota.Requests != nil && quota.Requests.Limit != nil {
		return true
REDACTED
	return quota.Tokens != nil && quota.Tokens.Limit != nil
REDACTED

func quotaLooksLikeGrokHeavy(quota *QuotaSnapshot) bool {
	if quota == nil {
		return false
REDACTED
	var requestLimit, tokenLimit int64
	if quota.Requests != nil && quota.Requests.Limit != nil {
		requestLimit = *quota.Requests.Limit
REDACTED
	if quota.Tokens != nil && quota.Tokens.Limit != nil {
		tokenLimit = *quota.Tokens.Limit
REDACTED
	return requestLimit >= grokHeavyQuotaRequestLimit || tokenLimit >= grokHeavyQuotaTokenLimit
REDACTED

func firstNonEmptyQuotaTime(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
	REDACTED
REDACTED
	return ""
REDACTED
