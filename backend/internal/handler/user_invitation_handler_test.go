//go:build unit

package handler

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserInvitationCodeFromService_StatusAndMasking(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	pending := userInvitationCodeFromService(&service.RedeemCode{
		Code: "pending", Type: service.RedeemTypeInvitation, Status: service.StatusUnused, ExpiresAt: &future,
	}, now)
	require.Equal(t, userInvitationStatusUnused, pending.Status)
	require.Empty(t, pending.UsedByEmailMask)

	neverExpires := userInvitationCodeFromService(&service.RedeemCode{
		Code: "forever", Type: service.RedeemTypeInvitation, Status: service.StatusUnused,
	}, now)
	require.Equal(t, userInvitationStatusUnused, neverExpires.Status)

	expired := userInvitationCodeFromService(&service.RedeemCode{
		Code: "expired", Type: service.RedeemTypeInvitation, Status: service.StatusUnused, ExpiresAt: &past,
	}, now)
	require.Equal(t, userInvitationStatusExpired, expired.Status)

	// 已使用的码即使已过期也展示为 used，并脱敏被邀请人邮箱
	used := userInvitationCodeFromService(&service.RedeemCode{
		Code: "used", Type: service.RedeemTypeInvitation, Status: service.StatusUsed, ExpiresAt: &past,
		User: &service.User{Email: "friend@example.com"},
	}, now)
	require.Equal(t, userInvitationStatusUsed, used.Status)
	require.NotEmpty(t, used.UsedByEmailMask)
	require.NotContains(t, used.UsedByEmailMask, "friend@")
	require.Contains(t, used.UsedByEmailMask, "@example.com")
}
