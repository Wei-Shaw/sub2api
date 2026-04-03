package service

import "testing"

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
