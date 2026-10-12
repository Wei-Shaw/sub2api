//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type channelMonitorSortRepoStub struct {
	ChannelMonitorRepository
	updates []ChannelMonitorSortOrderUpdate
	err     error
}

func (r *channelMonitorSortRepoStub) UpdateSortOrders(_ context.Context, updates []ChannelMonitorSortOrderUpdate) error {
	r.updates = append([]ChannelMonitorSortOrderUpdate(nil), updates...)
	return r.err
}

func TestUpdateChannelMonitorSortOrdersValidatesRequest(t *testing.T) {
	tests := []struct {
		name    string
		updates []ChannelMonitorSortOrderUpdate
	}{
		{name: "empty"},
		{name: "invalid id", updates: []ChannelMonitorSortOrderUpdate{{ID: 0, SortOrder: 0}}},
		{name: "negative order", updates: []ChannelMonitorSortOrderUpdate{{ID: 1, SortOrder: -1}}},
		{name: "duplicate id", updates: []ChannelMonitorSortOrderUpdate{{ID: 1}, {ID: 1, SortOrder: 10}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &channelMonitorSortRepoStub{}
			service := NewChannelMonitorService(repo, nil)

			err := service.UpdateSortOrders(context.Background(), tt.updates)

			require.Error(t, err)
			require.Empty(t, repo.updates)
		})
	}
}

func TestUpdateChannelMonitorSortOrdersPersistsValidatedOrder(t *testing.T) {
	repo := &channelMonitorSortRepoStub{}
	service := NewChannelMonitorService(repo, nil)
	updates := []ChannelMonitorSortOrderUpdate{{ID: 8, SortOrder: 0}, {ID: 3, SortOrder: 10}}

	err := service.UpdateSortOrders(context.Background(), updates)

	require.NoError(t, err)
	require.Equal(t, updates, repo.updates)
}

func TestUpdateChannelMonitorSortOrdersWrapsRepositoryError(t *testing.T) {
	repo := &channelMonitorSortRepoStub{err: errors.New("write failed")}
	service := NewChannelMonitorService(repo, nil)

	err := service.UpdateSortOrders(context.Background(), []ChannelMonitorSortOrderUpdate{{ID: 1}})

	require.ErrorContains(t, err, "update channel monitor sort order")
}
