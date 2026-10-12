//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChannelVersionRejectsStaleSnapshot(t *testing.T) {
	current := time.Date(2026, 9, 14, 7, 0, 0, 123456000, time.UTC)
	stale := current.Add(-time.Microsecond)
	repo := &mockChannelRepository{getByIDFn: func(context.Context, int64) (*Channel, error) {
		return &Channel{ID: 3, UpdatedAt: current}, nil
	}}
	_, err := newTestChannelService(repo).Update(context.Background(), 3, &UpdateChannelInput{ExpectedUpdatedAt: &stale})
	require.ErrorIs(t, err, ErrChannelModified)
}
