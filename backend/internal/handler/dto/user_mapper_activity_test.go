package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserFromServiceAdmin_MapsActivityTimestamps(t *testing.T) {
	t.Parallel()

	lastLoginAt := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	lastActiveAt := lastLoginAt.Add(15 * time.Minute)

	out := UserFromServiceAdmin(&service.User{
		ID:           42,
		Email:        "admin@example.com",
		Username:     "admin",
		Role:         service.RoleAdmin,
		Status:       service.StatusActive,
		LastLoginAt:  &lastLoginAt,
		LastActiveAt: &lastActiveAt,
REDACTED)

	require.NotNil(t, out)
	require.NotNil(t, out.LastLoginAt)
	require.NotNil(t, out.LastActiveAt)
	require.WithinDuration(t, lastLoginAt, *out.LastLoginAt, time.Second)
	require.WithinDuration(t, lastActiveAt, *out.LastActiveAt, time.Second)
REDACTED
