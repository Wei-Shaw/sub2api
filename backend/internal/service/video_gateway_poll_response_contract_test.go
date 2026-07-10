package service

import (
	"context"
	"net/http"
	"os"
	"testing"
)

func TestSeedancePollExtractsUsageActualsAndLastFrame(t *testing.T) {
	fixture, err := os.ReadFile("testdata/ark_poll_succeeded.json")
	if err != nil {
		t.Fatalf("read Ark poll fixture: %v", err)
	}
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	})
	task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 5, UpstreamTaskID: "t1"}

	res, err := adapter.PollTask(context.Background(), acc, task)
	if err != nil {
		t.Fatalf("PollTask: %v", err)
	}
	if res.Status != VideoStatusSucceeded {
		t.Fatalf("status = %q, want succeeded", res.Status)
	}
	if res.UsageTotalTokens == nil || *res.UsageTotalTokens != 654321 {
		t.Fatalf("usage_total_tokens = %+v, want 654321", res.UsageTotalTokens)
	}
	if res.ActualResolution != "1080p" {
		t.Fatalf("actual_resolution = %q, want 1080p", res.ActualResolution)
	}
	if res.ActualDuration == nil || *res.ActualDuration != 12 {
		t.Fatalf("actual_duration = %+v, want 12", res.ActualDuration)
	}
	if res.LastFrameURL != "https://ark-content.cn-beijing.volces.com/i/last.png" {
		t.Fatalf("last_frame_url = %q", res.LastFrameURL)
	}
}
