package service

import (
	"context"
	"testing"
)

func TestAPIKeyService_RejectsV10AuthSnapshotWithoutModelsListConfig(t *testing.T) {
	groupID := int64(9)
	svc := &APIKeyService{}

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
			},
			Group: &APIKeyAuthGroupSnapshot{
				ID:               groupID,
				Name:             "openai",
				Platform:         PlatformOpenAI,
				Status:           StatusActive,
				SubscriptionType: SubscriptionTypeStandard,
				RateMultiplier:   1,
			},
		},
	})

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
	}
	if ok {
		t.Fatalf("expected v10 auth snapshot to be rejected after models_list_config was added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestAPIKeyService_RejectsV15AuthSnapshotWithoutReasoningEffortPolicy(t *testing.T) {
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-reasoning-mappings", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{Version: 15},
	})

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
	}
	if ok {
		t.Fatal("expected v15 auth snapshot to be rejected after reasoning effort policy was added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestAPIKeyService_RejectsV18AuthSnapshotWithoutSharePool(t *testing.T) {
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-share-pool", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{Version: 18},
	})

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
	}
	if ok {
		t.Fatal("expected v18 auth snapshot to be rejected after is_share_pool was added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestAPIKeyAuthSnapshotSharePoolRoundtrip(t *testing.T) {
	svc := &APIKeyService{}
	groupID := int64(42)
	ownerKey := &APIKey{
		ID:      1,
		UserID:  2,
		GroupID: &groupID,
		Key:     "sk-share-pool",
		Status:  StatusActive,
		User: &User{
			ID:          2,
			Status:      StatusActive,
			Role:        RoleUser,
			Balance:     10,
			Concurrency: 3,
		},
		Group: &Group{
			ID:               groupID,
			Name:             "Grok",
			Platform:         PlatformGrok,
			UpstreamPlan:     "supergrok",
			IsSharePool:      true,
			Status:           StatusActive,
			SubscriptionType: SubscriptionTypeStandard,
			RateMultiplier:   1,
		},
	}

	snapshot := svc.snapshotFromAPIKey(context.Background(), ownerKey)
	if snapshot == nil || snapshot.Group == nil {
		t.Fatal("expected snapshot with group")
	}
	if snapshot.Version != apiKeyAuthSnapshotVersion {
		t.Fatalf("version=%d want %d", snapshot.Version, apiKeyAuthSnapshotVersion)
	}
	if !snapshot.Group.IsSharePool {
		t.Fatal("snapshot must preserve IsSharePool=true")
	}
	if snapshot.Group.UpstreamPlan != "supergrok" {
		t.Fatalf("UpstreamPlan=%q", snapshot.Group.UpstreamPlan)
	}

	materialized, used, err := svc.applyAuthCacheEntry(ownerKey.Key, &APIKeyAuthCacheEntry{Snapshot: snapshot})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !used || materialized == nil || materialized.Group == nil {
		t.Fatal("expected materialized group from cache")
	}
	if !materialized.Group.IsSharePool {
		t.Fatal("materialized Group.IsSharePool must stay true for share-revenue mode")
	}
	if materialized.Group.UpstreamPlan != "supergrok" {
		t.Fatalf("materialized UpstreamPlan=%q", materialized.Group.UpstreamPlan)
	}
}
