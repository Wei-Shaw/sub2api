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
REDACTED{
		{name: "created_to_uploading", from: BatchImageJobStatusCreated, to: BatchImageJobStatusUploading, want: trueREDACTED,
		{name: "uploading_to_submitted", from: BatchImageJobStatusUploading, to: BatchImageJobStatusSubmitted, want: trueREDACTED,
		{name: "submitted_to_running", from: BatchImageJobStatusSubmitted, to: BatchImageJobStatusRunning, want: trueREDACTED,
		{name: "running_self_poll", from: BatchImageJobStatusRunning, to: BatchImageJobStatusRunning, want: trueREDACTED,
		{name: "running_to_indexing", from: BatchImageJobStatusRunning, to: BatchImageJobStatusIndexing, want: trueREDACTED,
		{name: "indexing_to_settling", from: BatchImageJobStatusIndexing, to: BatchImageJobStatusSettling, want: trueREDACTED,
		{name: "settling_to_completed", from: BatchImageJobStatusSettling, to: BatchImageJobStatusCompleted, want: trueREDACTED,
		{name: "submitted_to_cancelled", from: BatchImageJobStatusSubmitted, to: BatchImageJobStatusCancelled, want: trueREDACTED,
		{name: "non_terminal_to_failed", from: BatchImageJobStatusCreated, to: BatchImageJobStatusFailed, want: trueREDACTED,
		{name: "completed_to_output_deleted", from: BatchImageJobStatusCompleted, to: BatchImageJobStatusOutputDeleted, want: trueREDACTED,
		{name: "failed_to_output_deleted", from: BatchImageJobStatusFailed, to: BatchImageJobStatusOutputDeleted, want: trueREDACTED,
		{name: "cancelled_to_output_deleted", from: BatchImageJobStatusCancelled, to: BatchImageJobStatusOutputDeleted, want: trueREDACTED,
		{name: "created_to_running_invalid", from: BatchImageJobStatusCreated, to: BatchImageJobStatusRunning, want: falseREDACTED,
		{name: "completed_to_running_invalid", from: BatchImageJobStatusCompleted, to: BatchImageJobStatusRunning, want: falseREDACTED,
		{name: "output_deleted_to_failed_invalid", from: BatchImageJobStatusOutputDeleted, to: BatchImageJobStatusFailed, want: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, CanTransitionBatchImageJob(tt.from, tt.to))
	REDACTED)
REDACTED
REDACTED

func TestIsTerminalBatchImageJobStatus(t *testing.T) {
	require.True(t, IsTerminalBatchImageJobStatus(BatchImageJobStatusCompleted))
	require.True(t, IsTerminalBatchImageJobStatus(BatchImageJobStatusFailed))
	require.True(t, IsTerminalBatchImageJobStatus(BatchImageJobStatusCancelled))
	require.True(t, IsTerminalBatchImageJobStatus(BatchImageJobStatusOutputDeleted))
	require.False(t, IsTerminalBatchImageJobStatus(BatchImageJobStatusRunning))
REDACTED

func TestIsSupportedBatchImageProvider(t *testing.T) {
	require.True(t, IsSupportedBatchImageProvider(BatchImageProviderGeminiAPI))
	require.True(t, IsSupportedBatchImageProvider(BatchImageProviderVertex))
	require.False(t, IsSupportedBatchImageProvider("gemini_oauth"))
	require.False(t, IsSupportedBatchImageProvider(""))
REDACTED

func TestNewBatchImageID(t *testing.T) {
	id, err := NewBatchImageID()
REDACTED
	require.True(t, strings.HasPrefix(id, "imgbatch_"))
	require.Len(t, id, len("imgbatch_")+32)
REDACTED
