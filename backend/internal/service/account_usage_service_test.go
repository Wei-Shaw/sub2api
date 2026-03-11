package service

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type accountUsageCodexProbeRepo struct {
	stubOpenAIAccountRepo
	updateExtraCh chan map[string]any
	rateLimitCh   chan time.Time
REDACTED

func (r *accountUsageCodexProbeRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.updateExtraCh != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
	REDACTED
		r.updateExtraCh <- copied
REDACTED
	return nil
REDACTED

func (r *accountUsageCodexProbeRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	if r.rateLimitCh != nil {
		r.rateLimitCh <- resetAt
REDACTED
	return nil
REDACTED

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

func TestExtractOpenAICodexProbeSnapshotAccepts429WithResetAt(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	updates, resetAt, err := extractOpenAICodexProbeSnapshot(&http.Response{StatusCode: http.StatusTooManyRequests, Header: headersREDACTED)
	if err != nil {
		t.Fatalf("extractOpenAICodexProbeSnapshot() error = %v", err)
REDACTED
	if len(updates) == 0 {
		t.Fatal("expected codex probe updates from 429 headers")
REDACTED
	if resetAt == nil {
		t.Fatal("expected resetAt from exhausted codex headers")
REDACTED
REDACTED

func TestAccountUsageService_PersistOpenAICodexProbeSnapshotSetsRateLimit(t *testing.T) {
	t.Parallel()

	repo := &accountUsageCodexProbeRepo{
		updateExtraCh: make(chan map[string]any, 1),
		rateLimitCh:   make(chan time.Time, 1),
REDACTED
	svc := &AccountUsageService{accountRepo: repoREDACTED
	resetAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)

	svc.persistOpenAICodexProbeSnapshot(321, map[string]any{
		"codex_7d_used_percent": 100.0,
		"codex_7d_reset_at":     resetAt.Format(time.RFC3339),
REDACTED, &resetAt)

	select {
	case updates := <-repo.updateExtraCh:
		if got := updates["codex_7d_used_percent"]; got != 100.0 {
			t.Fatalf("codex_7d_used_percent = %v, want 100", got)
	REDACTED
	case <-time.After(2 * time.Second):
		t.Fatal("waiting for codex probe extra persistence timed out")
REDACTED

	select {
	case got := <-repo.rateLimitCh:
		if got.Before(resetAt.Add(-time.Second)) || got.After(resetAt.Add(time.Second)) {
			t.Fatalf("rate limit resetAt = %v, want around %v", got, resetAt)
	REDACTED
	case <-time.After(2 * time.Second):
		t.Fatal("waiting for codex probe rate limit persistence timed out")
REDACTED
REDACTED
