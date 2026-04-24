package service

import (
	"context"
	"testing"
	"time"
)

func TestOpenAIRoutingSnapshot_FromSelection(t *testing.T) {
	account := &Account{ID: 66, Name: "acc-66"}
	snap := NewOpenAIRoutingSnapshot(OpenAIRoutingSnapshotInput{
		TargetGroup:    TargetGroupExhausted,
		ScheduleLayer:  string(openAIAccountScheduleLayerLoadBalance),
		Account:        account,
		RequestedModel: "gpt-5.4-Sys",
		EffectiveModel: "gpt-5.4",
	})

	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if snap.TargetGroup != "exhausted" {
		t.Fatalf("target group = %q", snap.TargetGroup)
	}
	if snap.ScheduleLayer != string(openAIAccountScheduleLayerLoadBalance) {
		t.Fatalf("schedule layer = %q", snap.ScheduleLayer)
	}
	if snap.SelectedAccountID == nil || *snap.SelectedAccountID != 66 {
		t.Fatalf("selected account id missing")
	}
	if snap.SelectedAccountName == nil || *snap.SelectedAccountName != "acc-66" {
		t.Fatalf("selected account name missing")
	}
}

func TestOpenAIRoutingSnapshot_RecordFailover(t *testing.T) {
	snap := NewOpenAIRoutingSnapshot(OpenAIRoutingSnapshotInput{
		TargetGroup:    TargetGroupActive,
		ScheduleLayer:  string(openAIAccountScheduleLayerSessionSticky),
		RequestedModel: "gpt-5.4",
		EffectiveModel: "gpt-5.4",
	})

	snap.RecordFailover("upstream_502")
	snap.RecordFailover("selected_exhausted_fallback")

	if snap.FailoverCount != 2 {
		t.Fatalf("failover count = %d", snap.FailoverCount)
	}
	if snap.FailoverFinalReason != "selected_exhausted_fallback" {
		t.Fatalf("final reason = %q", snap.FailoverFinalReason)
	}
}

func TestOpenAIRoutingSnapshot_ProjectionMetadataFromInput(t *testing.T) {
	builtAt := time.Unix(1_716_000_123, 0).UTC()
	snap := NewOpenAIRoutingSnapshot(OpenAIRoutingSnapshotInput{
		TargetGroup:        TargetGroupExhausted,
		SelectedGroup:      openAISelectedGroupReserve,
		ScheduleLayer:      string(openAIAccountScheduleLayerLoadBalance),
		ProjectionVersion:  7,
		ProjectionModelKey: "gpt-5.4-Sys",
		ProjectionBuiltAt:  builtAt,
		Account:            &Account{ID: 67, Name: "acc-67"},
		RequestedModel:     "gpt-5.4-Sys",
		EffectiveModel:     "gpt-5.4",
	})

	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if snap.ProjectionVersion != 7 {
		t.Fatalf("projection version = %d", snap.ProjectionVersion)
	}
	if snap.ProjectionModelKey != "gpt-5.4" {
		t.Fatalf("projection model key = %q", snap.ProjectionModelKey)
	}
	if !snap.ProjectionBuiltAt.Equal(builtAt) {
		t.Fatalf("projection built at = %v", snap.ProjectionBuiltAt)
	}

	binding := buildOpenAIRoutingAffinityBinding(snap)
	if binding == nil {
		t.Fatal("affinity binding is nil")
	}
	if binding.ProjectionVersion != 7 {
		t.Fatalf("binding projection version = %d", binding.ProjectionVersion)
	}
	if binding.ProjectionModelKey != "gpt-5.4" {
		t.Fatalf("binding projection model key = %q", binding.ProjectionModelKey)
	}
	if binding.ProjectionBuiltAt == nil || !binding.ProjectionBuiltAt.Equal(builtAt) {
		t.Fatalf("binding projection built at = %v", binding.ProjectionBuiltAt)
	}
}

func TestOpenAIRoutingSnapshot_ProjectionMetadataFallsBackToStickyBinding(t *testing.T) {
	builtAt := time.Unix(1_716_000_456, 0).UTC()
	sticky := &openAIStickyEval{
		AffinityBinding: &openAIAffinityBinding{
			BoundAccountID:     68,
			AffinityDomain:     string(TargetGroupExhausted),
			SelectedGroup:      openAISelectedGroupReserve,
			ProjectionVersion:  9,
			ProjectionModelKey: "gpt-5.4-Sys",
			ProjectionBuiltAt:  &builtAt,
		},
	}

	snap := NewOpenAIRoutingSnapshot(OpenAIRoutingSnapshotInput{
		TargetGroup:    TargetGroupExhausted,
		SelectedGroup:  openAISelectedGroupReserve,
		ScheduleLayer:  string(openAIAccountScheduleLayerPreviousResponse),
		Sticky:         sticky,
		RequestedModel: "gpt-5.4-Sys",
		EffectiveModel: "gpt-5.4",
	})

	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if snap.ProjectionVersion != 9 {
		t.Fatalf("projection version = %d", snap.ProjectionVersion)
	}
	if snap.ProjectionModelKey != "gpt-5.4" {
		t.Fatalf("projection model key = %q", snap.ProjectionModelKey)
	}
	if !snap.ProjectionBuiltAt.Equal(builtAt) {
		t.Fatalf("projection built at = %v", snap.ProjectionBuiltAt)
	}
}

func TestOpenAIRoutingSnapshot_FromLoadBalanceDecisionCarriesProjectionMetadata(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13220)
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(39410, 1),
		{ID: 39411, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1},
	}
	loadMap := map[int64]*AccountLoadInfo{
		39410: {AccountID: 39410, CurrentConcurrency: 1, LoadRate: 90},
		39411: {AccountID: 39411, CurrentConcurrency: 0, LoadRate: 10},
	}
	svc := newOpenAIReserveSelectionServiceForTest(accounts, loadMap)

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.1-Sys",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	if err != nil {
		t.Fatalf("select account: %v", err)
	}
	if selection == nil || selection.Account == nil {
		t.Fatal("selection is nil")
	}
	if selection.Account.ID != 39411 {
		t.Fatalf("selected account = %d", selection.Account.ID)
	}
	if decision.ProjectionVersion != 1 {
		t.Fatalf("decision projection version = %d", decision.ProjectionVersion)
	}
	if decision.ProjectionModelKey != "gpt-5.1" {
		t.Fatalf("decision projection model key = %q", decision.ProjectionModelKey)
	}
	wantBuiltAt := time.Unix(1_716_000_000, 0).UTC()
	if !decision.ProjectionBuiltAt.Equal(wantBuiltAt) {
		t.Fatalf("decision projection built at = %v", decision.ProjectionBuiltAt)
	}

	snap := NewOpenAIRoutingSnapshot(OpenAIRoutingSnapshotInput{
		TargetGroup:        TargetGroupAny,
		SelectedGroup:      decision.SelectedGroup,
		ScheduleLayer:      decision.Layer,
		ProjectionVersion:  decision.ProjectionVersion,
		ProjectionModelKey: decision.ProjectionModelKey,
		ProjectionBuiltAt:  decision.ProjectionBuiltAt,
		Account:            selection.Account,
		RequestedModel:     "gpt-5.1-Sys",
		EffectiveModel:     "gpt-5.1",
	})
	binding := buildOpenAIRoutingAffinityBinding(snap)
	if binding == nil {
		t.Fatal("affinity binding is nil")
	}
	if binding.ProjectionVersion != 1 {
		t.Fatalf("binding projection version = %d", binding.ProjectionVersion)
	}
	if binding.ProjectionModelKey != "gpt-5.1" {
		t.Fatalf("binding projection model key = %q", binding.ProjectionModelKey)
	}
	if binding.ProjectionBuiltAt == nil || !binding.ProjectionBuiltAt.Equal(wantBuiltAt) {
		t.Fatalf("binding projection built at = %v", binding.ProjectionBuiltAt)
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
