//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type outboxRepoFake struct {
	messages  []NotificationOutboxMessage
	delivered []int64
	retried   []int64
	terminal  bool
	next      time.Time
}

func (r *outboxRepoFake) Claim(context.Context, string, int, int, time.Duration) ([]NotificationOutboxMessage, error) {
	return r.messages, nil
}
func (r *outboxRepoFake) MarkDelivered(_ context.Context, id int64, _ string) error {
	r.delivered = append(r.delivered, id)
	return nil
}
func (r *outboxRepoFake) MarkRetry(_ context.Context, id int64, _ string, next time.Time, _ string, terminal bool) error {
	r.retried = append(r.retried, id)
	r.terminal = terminal
	r.next = next
	return nil
}
func (r *outboxRepoFake) Stats(context.Context, int) (NotificationOutboxStats, error) {
	return NotificationOutboxStats{}, nil
}

type outboxSenderFake struct {
	err    error
	inputs []NotificationEmailSendInput
}

func (s *outboxSenderFake) Send(_ context.Context, input NotificationEmailSendInput) error {
	s.inputs = append(s.inputs, input)
	return s.err
}

func TestNotificationOutboxWorkerDeliversClaimedMessage(t *testing.T) {
	repo := &outboxRepoFake{messages: []NotificationOutboxMessage{{ID: 7, Event: "company_upgrade_approved", Recipient: "user@example.com", Locale: "zh-CN", Variables: map[string]string{"company_name": "Acme"}, AttemptCount: 1}}}
	sender := &outboxSenderFake{}
	worker := &NotificationOutboxWorker{repo: repo, emailer: sender, workerID: "worker-a", poll: time.Second, maxAttempts: 3}
	require.NoError(t, worker.ProcessOnce(context.Background()))
	require.Equal(t, []int64{7}, repo.delivered)
	require.Empty(t, repo.retried)
	require.Equal(t, "user@example.com", sender.inputs[0].RecipientEmail)
}

func TestNotificationOutboxWorkerRetriesAndTerminatesAtLimit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		attempts int
		terminal bool
	}{{"retry", 1, false}, {"terminal", 3, true}} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &outboxRepoFake{messages: []NotificationOutboxMessage{{ID: 8, Event: "company_upgrade_rejected", Recipient: "user@example.com", AttemptCount: tc.attempts}}}
			worker := &NotificationOutboxWorker{repo: repo, emailer: &outboxSenderFake{err: errors.New("provider unavailable")}, workerID: "worker-b", poll: time.Second, maxAttempts: 3}
			before := time.Now()
			require.NoError(t, worker.ProcessOnce(context.Background()))
			require.Equal(t, []int64{8}, repo.retried)
			require.Equal(t, tc.terminal, repo.terminal)
			require.True(t, repo.next.After(before))
		})
	}
}
