package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func testPromptCompressionConfig(enabled bool) *config.Config {
	return &config.Config{Gateway: config.GatewayConfig{PromptCompression: config.PromptCompressionConfig{
		Enabled: enabled, MaxBodyBytes: 10 * 1024 * 1024, MaxResultBytes: 1024 * 1024,
		MaxDurationMS: 20, MinCandidateTokens: 256, MinSavingsTokens: 64,
		AllowedProtocols: []string{"anthropic", "responses"},
	}}}
}

func TestPromptCompressionDisabledByDefault(t *testing.T) {
	svc := NewPromptCompressionService(testPromptCompressionConfig(false))
	decision := svc.ResolvePolicy(context.Background(), RTKPolicyRequest{Protocol: "anthropic"})
	if decision.Mode != RTKModeOff || decision.SkipReason != "deployment_disabled" {
		t.Fatalf("expected deployment-disabled off decision, got mode=%q reason=%q", decision.Mode, decision.SkipReason)
	}
}

func TestPromptCompressionEmergencyStopAndResume(t *testing.T) {
	svc := NewPromptCompressionService(testPromptCompressionConfig(true))
	svc.EmergencyStop("test", "incident")
	decision := svc.ResolvePolicy(context.Background(), RTKPolicyRequest{Protocol: "anthropic"})
	if decision.SkipReason != "emergency_stopped" {
		t.Fatalf("expected emergency stop, got %q", decision.SkipReason)
	}
	svc.Resume("test", "cleared")
	decision = svc.ResolvePolicy(context.Background(), RTKPolicyRequest{Protocol: "anthropic"})
	if decision.Mode != RTKModeObserve || decision.SkipReason != "" {
		t.Fatalf("expected observe after resume, got mode=%q reason=%q", decision.Mode, decision.SkipReason)
	}
}

func TestPromptCompressionGroupPolicyCannotBypassDeploymentGuard(t *testing.T) {
	svc := NewPromptCompressionService(testPromptCompressionConfig(false))
	force := RTKModeEnforce
	decision := svc.ResolvePolicy(context.Background(), RTKPolicyRequest{
		Protocol: "anthropic", GroupPolicy: &PromptCompressionGroupPolicy{Mode: force},
	})
	if decision.Mode != RTKModeOff || decision.SkipReason != "deployment_disabled" {
		t.Fatalf("group override bypassed deployment guard: mode=%q reason=%q", decision.Mode, decision.SkipReason)
	}
}

func TestPromptCompressionTelemetryIsNonBlocking(t *testing.T) {
	svc := NewPromptCompressionService(testPromptCompressionConfig(false))
	for i := 0; i < 1100; i++ {
		svc.RecordTelemetry(PromptCompressionTelemetry{Outcome: "skipped"})
	}
	status := svc.Status()
	if status.Telemetry.Recorded == 0 || status.Telemetry.Dropped == 0 {
		t.Fatalf("expected bounded telemetry queue, got recorded=%d dropped=%d", status.Telemetry.Recorded, status.Telemetry.Dropped)
	}
	if drained := len(svc.DrainTelemetry(2000)); drained > 1024 {
		t.Fatalf("drained more than bounded queue capacity: %d", drained)
	}
}

func TestPromptCompressionGroupPolicyResolution(t *testing.T) {
	svc := NewPromptCompressionService(testPromptCompressionConfig(true))
	if err := svc.UpdateGroupPolicy(42, PromptCompressionGroupPolicy{Mode: RTKModeEnforce, Intensity: "safe"}); err != nil {
		t.Fatal(err)
	}
	decision := svc.ResolvePolicy(context.Background(), RTKPolicyRequest{Protocol: "anthropic", GroupID: 42})
	if decision.Mode != RTKModeEnforce || decision.Policy.Intensity != "safe" {
		t.Fatalf("unexpected group policy decision: %+v", decision)
	}
}
