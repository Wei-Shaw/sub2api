//go:build unit

package service

import "testing"

func TestApplyMonitorUpdate_ProviderOnlySwitchToGrokUsesDefaultModel(t *testing.T) {
	grok := MonitorProviderGrok
	existing := &ChannelMonitor{
		Provider:        MonitorProviderOpenAI,
		APIMode:         MonitorAPIModeResponses,
		PrimaryModel:    "gpt-5",
		IntervalSeconds: 60,
	}

	err := applyMonitorUpdate(existing, ChannelMonitorUpdateParams{Provider: &grok})
	if err != nil {
		t.Fatalf("provider-only switch to Grok failed: %v", err)
	}
	if existing.PrimaryModel != MonitorDefaultGrokModel {
		t.Fatalf("expected Grok default model %q, got %q", MonitorDefaultGrokModel, existing.PrimaryModel)
	}
	if existing.APIMode != MonitorAPIModeChatCompletions {
		t.Fatalf("expected Grok API mode %q, got %q", MonitorAPIModeChatCompletions, existing.APIMode)
	}
}

func TestApplyMonitorUpdate_SwitchToGrokPreservesExplicitModel(t *testing.T) {
	grok := MonitorProviderGrok
	explicitModel := "grok-4.3"
	existing := &ChannelMonitor{
		Provider:        MonitorProviderOpenAI,
		APIMode:         MonitorAPIModeChatCompletions,
		PrimaryModel:    "gpt-5",
		IntervalSeconds: 60,
	}

	err := applyMonitorUpdate(existing, ChannelMonitorUpdateParams{
		Provider:     &grok,
		PrimaryModel: &explicitModel,
	})
	if err != nil {
		t.Fatalf("switch to Grok with explicit model failed: %v", err)
	}
	if existing.PrimaryModel != explicitModel {
		t.Fatalf("expected explicit model %q, got %q", explicitModel, existing.PrimaryModel)
	}
}

func TestApplyMonitorUpdate_SameGrokProviderDoesNotResetExistingModel(t *testing.T) {
	grok := MonitorProviderGrok
	existing := &ChannelMonitor{
		Provider:        MonitorProviderGrok,
		APIMode:         MonitorAPIModeChatCompletions,
		PrimaryModel:    "grok-4.3",
		IntervalSeconds: 60,
	}

	err := applyMonitorUpdate(existing, ChannelMonitorUpdateParams{Provider: &grok})
	if err != nil {
		t.Fatalf("same-provider Grok update failed: %v", err)
	}
	if existing.PrimaryModel != "grok-4.3" {
		t.Fatalf("same-provider update reset existing model to %q", existing.PrimaryModel)
	}
}

func TestApplyMonitorUpdate_SwitchToGrokRejectsResponsesMode(t *testing.T) {
	grok := MonitorProviderGrok
	responses := MonitorAPIModeResponses
	existing := &ChannelMonitor{
		Provider:        MonitorProviderOpenAI,
		APIMode:         MonitorAPIModeChatCompletions,
		PrimaryModel:    "gpt-5",
		IntervalSeconds: 60,
	}

	err := applyMonitorUpdate(existing, ChannelMonitorUpdateParams{
		Provider: &grok,
		APIMode:  &responses,
	})
	if err == nil {
		t.Fatal("Grok responses mode should remain unsupported")
	}
}
