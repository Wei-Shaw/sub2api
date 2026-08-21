package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestImageStatusFromTaskMapsTerminalFailureToFailed(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "pending", status: service.AsyncMediaStatusPending, want: fal.StatusInQueue},
		{name: "running", status: service.AsyncMediaStatusRunning, want: fal.StatusInProgress},
		{name: "succeeded", status: service.AsyncMediaStatusSucceeded, want: fal.StatusCompleted},
		{name: "failed", status: service.AsyncMediaStatusFailed, want: fal.StatusFailed},
		{name: "refunded", status: service.AsyncMediaStatusRefunded, want: fal.StatusFailed},
		{name: "expired", status: service.AsyncMediaStatusExpired, want: fal.StatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imageStatusFromTask(&service.AsyncMediaTask{Status: tt.status})
			if got != tt.want {
				t.Fatalf("imageStatusFromTask(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}
