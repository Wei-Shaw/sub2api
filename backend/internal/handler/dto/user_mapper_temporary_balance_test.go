package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserFromServiceMapsTemporaryBalanceFields(t *testing.T) {
	expiresAt := time.Date(2026, 9, 6, 23, 59, 59, 0, time.UTC)
	user := UserFromService(&service.User{
		ID:                        42,
		TemporaryBalance:          12.5,
		ActiveTemporaryBalance:    7.25,
		TemporaryBalanceExpiresAt: &expiresAt,
	})
	require.NotNil(t, user)
	require.InDelta(t, 12.5, user.TemporaryBalance, 0.000001)
	require.InDelta(t, 7.25, user.ActiveTemporaryBalance, 0.000001)
	require.Equal(t, expiresAt, user.TemporaryBalanceExpiresAt.UTC())
}
