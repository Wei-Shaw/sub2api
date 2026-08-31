package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
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

func TestAPIKeyService_RejectsV20AuthSnapshotWithoutSmartRoutingMembers(t *testing.T) {
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-smart-routing", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{Version: 20},
	})

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
	}
	if ok {
		t.Fatal("expected v20 auth snapshot to be rejected after smart routing members were added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestAPIKeyService_AuthSnapshotRoundTripsSmartRoutingMembers(t *testing.T) {
	svc := &APIKeyService{}
	groupID := int64(12)
	members := []domain.SmartRoutingMember{
		{GroupID: 11, Priority: 1, Weight: 3},
		{GroupID: 9, Priority: 2, Weight: 1},
	}

	source := &APIKey{
		ID:      6,
		UserID:  1,
		Key:     "k-smart",
		Status:  StatusActive,
		GroupID: &groupID,
		User:    &User{ID: 1, Status: StatusActive, Role: RoleUser, Balance: 10},
		Group: &Group{
			ID:                  groupID,
			Name:                "smart",
			Platform:            PlatformSmartRouting,
			Status:              StatusActive,
			SubscriptionType:    SubscriptionTypeStandard,
			RateMultiplier:      1,
			SmartRoutingMembers: members,
		},
	}

	snapshot := svc.snapshotFromAPIKey(t.Context(), source)
	if snapshot == nil {
		t.Fatal("expected snapshot to be built")
	}
	if len(snapshot.Group.SmartRoutingMembers) != 2 {
		t.Fatalf("expected snapshot to carry 2 smart routing members, got %d", len(snapshot.Group.SmartRoutingMembers))
	}

	restored, ok, err := svc.applyAuthCacheEntry("k-smart", &APIKeyAuthCacheEntry{Snapshot: snapshot})
	if err != nil || !ok {
		t.Fatalf("expected current-version snapshot to be accepted, ok=%v err=%v", ok, err)
	}
	if restored == nil || restored.Group == nil {
		t.Fatal("expected restored API key with group")
	}
	if !restored.Group.IsSmartRouting() {
		t.Fatalf("expected restored group to be smart routing, members=%v", restored.Group.SmartRoutingMembers)
	}
	if len(restored.Group.SmartRoutingMembers) != 2 ||
		restored.Group.SmartRoutingMembers[0].GroupID != 11 ||
		restored.Group.SmartRoutingMembers[1].GroupID != 9 {
		t.Fatalf("smart routing members did not round-trip: %#v", restored.Group.SmartRoutingMembers)
	}
}
