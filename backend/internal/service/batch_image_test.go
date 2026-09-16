//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanTransitionBatchImageJob(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{name: "created_to_uploading", from: BatchImageJobStatusCreated, to: BatchImageJobStatusUploading, want: true},
		{name: "uploading_to_submitted", from: BatchImageJobStatusUploading, to: BatchImageJobStatusSubmitted, want: true},
		{name: "submitted_to_running", from: BatchImageJobStatusSubmitted, to: BatchImageJobStatusRunning, want: true},
		{name: "running_self_poll", from: BatchImageJobStatusRunning, to: BatchImageJobStatusRunning, want: true},
		{name: "running_to_indexing", from: BatchImageJobStatusRunning, to: BatchImageJobStatusIndexing, want: true},
		{name: "indexing_to_settling", from: BatchImageJobStatusIndexing, to: BatchImageJobStatusSettling, want: true},
		{name: "settling_to_completed", from: BatchImageJobStatusSettling, to: BatchImageJobStatusCompleted, want: true},
		{name: "submitted_to_cancelled", from: BatchImageJobStatusSubmitted, to: BatchImageJobStatusCancelled, want: true},
		{name: "non_terminal_to_failed", from: BatchImageJobStatusCreated, to: BatchImageJobStatusFailed, want: true},
		{name: "completed_to_output_deleted", from: BatchImageJobStatusCompleted, to: BatchImageJobStatusOutputDeleted, want: true},
		{name: "failed_to_output_deleted", from: BatchImageJobStatusFailed, to: BatchImageJobStatusOutputDeleted, want: true},
		{name: "cancelled_to_output_deleted", from: BatchImageJobStatusCancelled, to: BatchImageJobStatusOutputDeleted, want: true},
		{name: "created_to_running_invalid", from: BatchImageJobStatusCreated, to: BatchImageJobStatusRunning, want: false},
		{name: "completed_to_running_invalid", from: BatchImageJobStatusCompleted, to: BatchImageJobStatusRunning, want: false},
		{name: "output_deleted_to_failed_invalid", from: BatchImageJobStatusOutputDeleted, to: BatchImageJobStatusFailed, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, CanTransitionBatchImageJob(tt.from, tt.to))
		})
	}
}

func TestIsTerminalBatchImageJobStatus(t *testing.T) {
	require.True(t, IsTerminalBatchImageJobStatus(BatchImageJobStatusCompleted))
	require.True(t, IsTerminalBatchImageJobStatus(BatchImageJobStatusFailed))
	require.True(t, IsTerminalBatchImageJobStatus(BatchImageJobStatusCancelled))
	require.True(t, IsTerminalBatchImageJobStatus(BatchImageJobStatusOutputDeleted))
	require.False(t, IsTerminalBatchImageJobStatus(BatchImageJobStatusRunning))
}

func TestIsSupportedBatchImageProvider(t *testing.T) {
	require.True(t, IsSupportedBatchImageProvider(BatchImageProviderGeminiAPI))
	require.True(t, IsSupportedBatchImageProvider(BatchImageProviderVertex))
	require.False(t, IsSupportedBatchImageProvider("gemini_oauth"))
	require.False(t, IsSupportedBatchImageProvider(""))
}

func TestNewBatchImageID(t *testing.T) {
	id, err := NewBatchImageID()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(id, "imgbatch_"))
	require.Len(t, id, len("imgbatch_")+32)
}
