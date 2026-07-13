package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserFromServiceShallow_MapsDeletedAt(t *testing.T) {
	ts := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)

	deleted := UserFromServiceShallow(&service.User{ID: 1, Email: "d@test.com", DeletedAt: &ts})
	require.NotNil(t, deleted.DeletedAt)
	require.Equal(t, ts, *deleted.DeletedAt)

	active := UserFromServiceShallow(&service.User{ID: 2, Email: "a@test.com"})
	require.Nil(t, active.DeletedAt, "active user must have nil DeletedAt")
}

func TestUserFromServiceShallow_MapsExtraConcurrency(t *testing.T) {
	user := UserFromServiceShallow(&service.User{
		ID:               3,
		Email:            "extra@test.com",
		Concurrency:      5,
		ExtraConcurrency: 2,
	})

	require.Equal(t, 5, user.Concurrency)
	require.Equal(t, 2, user.ExtraConcurrency)
}
