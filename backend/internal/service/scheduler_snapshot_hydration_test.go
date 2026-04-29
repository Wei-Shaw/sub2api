//go:build unit

package service

import (
	"context"
	"testing"
	"time"
)

type snapshotHydrationCache struct {
	snapshot []*Account
	accounts map[int64]*Account
REDACTED

func (c *snapshotHydrationCache) GetSnapshot(ctx context.Context, bucket SchedulerBucket) ([]*Account, bool, error) {
	return c.snapshot, true, nil
REDACTED

func (c *snapshotHydrationCache) SetSnapshot(ctx context.Context, bucket SchedulerBucket, accounts []Account) error {
	return nil
REDACTED

func (c *snapshotHydrationCache) GetAccount(ctx context.Context, accountID int64) (*Account, error) {
	if c.accounts == nil {
		return nil, nil
REDACTED
	return c.accounts[accountID], nil
REDACTED

func (c *snapshotHydrationCache) SetAccount(ctx context.Context, account *Account) error {
	return nil
REDACTED

func (c *snapshotHydrationCache) DeleteAccount(ctx context.Context, accountID int64) error {
	return nil
REDACTED

func (c *snapshotHydrationCache) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
REDACTED

func (c *snapshotHydrationCache) TryLockBucket(ctx context.Context, bucket SchedulerBucket, ttl time.Duration) (bool, error) {
	return true, nil
REDACTED

func (c *snapshotHydrationCache) UnlockBucket(ctx context.Context, bucket SchedulerBucket) error {
	return nil
REDACTED

func (c *snapshotHydrationCache) ListBuckets(ctx context.Context) ([]SchedulerBucket, error) {
	return nil, nil
REDACTED

func (c *snapshotHydrationCache) GetOutboxWatermark(ctx context.Context) (int64, error) {
	return 0, nil
REDACTED

func (c *snapshotHydrationCache) SetOutboxWatermark(ctx context.Context, id int64) error {
	return nil
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_HydratesSelectedAccountFromSchedulerSnapshot(t *testing.T) {
	cache := &snapshotHydrationCache{
		snapshot: []*Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
		REDACTED
					"model_mapping": map[string]any{
						"gpt-4": "gpt-4",
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		accounts: map[int64]*Account{
			1: {
				ID:          1,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
		REDACTED
					"api_key":       "sk-live",
					"model_mapping": map[string]any{"gpt-4": "gpt-4"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	schedulerSnapshot := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
	groupID := int64(2)
	svc := &OpenAIGatewayService{
		schedulerSnapshot: schedulerSnapshot,
		cache:             &stubGatewayCache{REDACTED,
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selected account")
REDACTED
	if got := selection.Account.GetOpenAIApiKey(); got != "sk-live" {
		t.Fatalf("expected hydrated api key, got %q", got)
REDACTED
REDACTED

func TestGatewaySelectAccountWithLoadAwareness_HydratesSelectedAccountFromSchedulerSnapshot(t *testing.T) {
	cache := &snapshotHydrationCache{
		snapshot: []*Account{
			{
				ID:          9,
				Platform:    PlatformAnthropic,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
		REDACTED,
	REDACTED,
		accounts: map[int64]*Account{
			9: {
				ID:          9,
				Platform:    PlatformAnthropic,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
		REDACTED
					"api_key": "anthropic-live-key",
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	schedulerSnapshot := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
	svc := &GatewayService{
		schedulerSnapshot: schedulerSnapshot,
		cache:             &mockGatewayCacheForPlatform{REDACTED,
		cfg:               testConfig(),
REDACTED

	result, err := svc.SelectAccountWithLoadAwareness(context.Background(), nil, "", "claude-3-5-sonnet-20241022", nil, "", 0)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if result == nil || result.Account == nil {
		t.Fatalf("expected selected account")
REDACTED
	if got := result.Account.GetCredential("api_key"); got != "anthropic-live-key" {
		t.Fatalf("expected hydrated api key, got %q", got)
REDACTED
REDACTED
