package service

import "testing"

func TestAPIKeyService_RejectsV10AuthSnapshotWithoutModelsListConfig(t *testing.T) {
	groupID := int64(9)
	svc := &APIKeyService{REDACTED

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-models-list", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{
			Version:  10,
			APIKeyID: 1,
			UserID:   2,
			GroupID:  &groupID,
			Status:   StatusActive,
			User: APIKeyAuthUserSnapshot{
				ID:          2,
				Status:      StatusActive,
				Role:        RoleUser,
				Balance:     10,
				Concurrency: 3,
		REDACTED,
			Group: &APIKeyAuthGroupSnapshot{
				ID:               groupID,
				Name:             "openai",
				Platform:         PlatformOpenAI,
				Status:           StatusActive,
				SubscriptionType: SubscriptionTypeStandard,
				RateMultiplier:   1,
		REDACTED,
	REDACTED,
REDACTED)

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
REDACTED
	if ok {
		t.Fatalf("expected v10 auth snapshot to be rejected after models_list_config was added")
REDACTED
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
REDACTED
REDACTED

func TestAPIKeyService_RejectsV17AuthSnapshotWithoutReasoningEffortMappings(t *testing.T) {
	svc := &APIKeyService{REDACTED

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-reasoning-mappings", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{Version: 17REDACTED,
REDACTED)

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
REDACTED
	if ok {
		t.Fatal("expected v17 auth snapshot to be rejected after reasoning effort mappings were added")
REDACTED
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
REDACTED
REDACTED
