package service

import "testing"

func TestAcceptGroupProbeResult_PrefersOperationalOverDegraded(t *testing.T) {
	var best *CheckResult
	slowLatency := 8000
	fastLatency := 1200

	if acceptGroupProbeResult(&CheckResult{Status: MonitorStatusDegraded, LatencyMs: &slowLatency}, &best) {
		t.Fatal("degraded result should be kept as fallback, not accepted immediately")
	}
	if best == nil || best.LatencyMs == nil || *best.LatencyMs != slowLatency {
		t.Fatalf("degraded fallback not recorded correctly: %#v", best)
	}
	if !acceptGroupProbeResult(&CheckResult{Status: MonitorStatusOperational, LatencyMs: &fastLatency}, &best) {
		t.Fatal("operational result should be accepted immediately")
	}
}

func TestAcceptGroupProbeResult_KeepsFastestDegradedFallback(t *testing.T) {
	var best *CheckResult
	slowLatency := 9000
	fasterLatency := 7000

	acceptGroupProbeResult(&CheckResult{Status: MonitorStatusDegraded, LatencyMs: &slowLatency}, &best)
	acceptGroupProbeResult(&CheckResult{Status: MonitorStatusDegraded, LatencyMs: &fasterLatency}, &best)

	if best == nil || best.LatencyMs == nil || *best.LatencyMs != fasterLatency {
		t.Fatalf("fastest degraded fallback not retained: %#v", best)
	}
}
