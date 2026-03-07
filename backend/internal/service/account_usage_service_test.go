package service

import (
	"net/http"
	"testing"
	"time"
)

func TestShouldRefreshOpenAICodexSnapshot(t *testing.T) {
	t.Parallel()

	rateLimitedUntil := time.Now().Add(5 * time.Minute)
	now := time.Now()
	usage := &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 0REDACTED,
		SevenDay: &UsageProgress{Utilization: 0REDACTED,
REDACTED

	if !shouldRefreshOpenAICodexSnapshot(&Account{RateLimitResetAt: &rateLimitedUntilREDACTED, usage, now) {
		t.Fatal("expected rate-limited account to force codex snapshot refresh")
REDACTED

	if shouldRefreshOpenAICodexSnapshot(&Account{REDACTED, usage, now) {
		t.Fatal("expected complete non-rate-limited usage to skip codex snapshot refresh")
REDACTED

	if !shouldRefreshOpenAICodexSnapshot(&Account{REDACTED, &UsageInfo{FiveHour: nil, SevenDay: &UsageProgress{REDACTEDREDACTED, now) {
		t.Fatal("expected missing 5h snapshot to require refresh")
REDACTED

	staleAt := now.Add(-(openAIProbeCacheTTL + time.Minute)).Format(time.RFC3339)
	if !shouldRefreshOpenAICodexSnapshot(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"codex_usage_updated_at":                       staleAt,
	REDACTED,
REDACTED, usage, now) {
		t.Fatal("expected stale ws snapshot to trigger refresh")
REDACTED
REDACTED

func TestExtractOpenAICodexProbeUpdatesAccepts429WithCodexHeaders(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	updates, err := extractOpenAICodexProbeUpdates(&http.Response{StatusCode: http.StatusTooManyRequests, Header: headersREDACTED)
	if err != nil {
		t.Fatalf("extractOpenAICodexProbeUpdates() error = %v", err)
REDACTED
	if len(updates) == 0 {
		t.Fatal("expected codex probe updates from 429 headers")
REDACTED
	if got := updates["codex_5h_used_percent"]; got != 100.0 {
		t.Fatalf("codex_5h_used_percent = %v, want 100", got)
REDACTED
	if got := updates["codex_7d_used_percent"]; got != 100.0 {
		t.Fatalf("codex_7d_used_percent = %v, want 100", got)
REDACTED
REDACTED
