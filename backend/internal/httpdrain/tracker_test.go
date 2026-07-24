package httpdrain

import "testing"

func TestTrackerCountsRequestsAndHijackedConnections(t *testing.T) {
	baseline := Status()
	finish := BeginRequest("/api/v1/messages")
	BeginHijackedConnection()
	current := Status()
	if current.ActiveRequests != baseline.ActiveRequests+1 || current.HijackedConnections != baseline.HijackedConnections+1 || current.Blockers != baseline.Blockers+2 {
		t.Fatalf("unexpected active snapshot: %+v (baseline %+v)", current, baseline)
	}
	finish()
	finish()
	EndHijackedConnection()
	if got := Status(); got != baseline {
		t.Fatalf("tracker did not return to baseline: got %+v, want %+v", got, baseline)
	}
}

func TestHealthRequestIsNotADrainBlocker(t *testing.T) {
	baseline := Status()
	finish := BeginRequest("/health")
	defer finish()
	if got := Status(); got != baseline {
		t.Fatalf("health probe changed drain snapshot: got %+v, want %+v", got, baseline)
	}
}
