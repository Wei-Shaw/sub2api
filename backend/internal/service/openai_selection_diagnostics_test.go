package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAISelectionError_UnwrapsBaseError(t *testing.T) {
	t.Parallel()

	err := newOpenAINoAvailableAccountsError(
		"candidate_filtering",
		"no available accounts remain after candidate filtering",
		"group_id=6 model=gpt-5.5 total=3 eligible=0 model_unsupported=2 channel_restricted=1",
		false,
	)

	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Contains(t, err.Error(), "openai account selection failed at candidate_filtering")
	require.Contains(t, err.Error(), "no available accounts remain after candidate filtering")
	require.Contains(t, err.Error(), "group_id=6 model=gpt-5.5")
}

func TestOpenAISelectionError_AbortDetection(t *testing.T) {
	t.Parallel()

	require.True(t, isOpenAISelectionAbortError(context.Canceled))
	require.True(t, isOpenAISelectionAbortError(context.DeadlineExceeded))
	require.False(t, isOpenAISelectionAbortError(errors.New("redis unavailable")))
}
