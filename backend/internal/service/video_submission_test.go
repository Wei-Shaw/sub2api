//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type videoSubmissionStub struct {
	UsageBillingRepository
	hashes map[string]string
	tasks  map[string]string
	claims int
}

func (r *videoSubmissionStub) ClaimVideoSubmission(_ context.Context, id int64, key, hash string) (bool, string, error) {
	if prior, ok := r.hashes[key]; ok {
		if prior != hash {
			return false, "", ErrVideoSubmissionConflict
		}
		return false, r.tasks[key], nil
	}
	r.hashes[key] = hash
	r.claims++
	return true, "", nil
}
func (r *videoSubmissionStub) CompleteVideoSubmission(_ context.Context, id int64, key, task string) error {
	r.tasks[key] = task
	return nil
}

func TestVideoSubmissionDoesNotReclaimUnknownOutcome(t *testing.T) {
	r := &videoSubmissionStub{hashes: map[string]string{}, tasks: map[string]string{}}
	s := &OpenAIGatewayService{usageBillingRepo: r}
	ctx := context.Background()
	body := []byte(`{"model":"video","prompt":"waves"}`)
	id, err := s.BeginVideoSubmission(ctx, 1, 2, "same", "application/json", body)
	require.NoError(t, err)
	require.Empty(t, id)
	_, err = s.BeginVideoSubmission(ctx, 1, 2, "same", "application/json", body)
	require.ErrorIs(t, err, ErrVideoSubmissionPending)
	require.Equal(t, 1, r.claims)
	require.NoError(t, s.CompleteVideoSubmission(ctx, 1, "same", "task-1"))
	id, err = s.BeginVideoSubmission(ctx, 1, 2, "same", "application/json", body)
	require.NoError(t, err)
	require.Equal(t, "task-1", id)
	_, err = s.BeginVideoSubmission(ctx, 1, 3, "same", "application/json", body)
	require.ErrorIs(t, err, ErrVideoSubmissionConflict)
	_, err = s.BeginVideoSubmission(ctx, 1, 2, "same", "application/json", []byte(`{"model":"different"}`))
	require.ErrorIs(t, err, ErrVideoSubmissionConflict)
}

func TestVideoSubmissionOptionalKeyAndUnavailableStore(t *testing.T) {
	s := &OpenAIGatewayService{}
	_, err := s.BeginVideoSubmission(context.Background(), 1, 2, "", "application/json", nil)
	require.NoError(t, err)
	_, err = s.BeginVideoSubmission(context.Background(), 1, 2, "key", "application/json", nil)
	require.ErrorContains(t, err, "unavailable")
}
