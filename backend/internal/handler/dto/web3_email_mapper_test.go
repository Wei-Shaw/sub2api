package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

const web3TestEmail = "web3-52908400098527886e0f7030069857d2e4169ee7@web3-connect.invalid"

func web3TestUser() *service.User {
	return &service.User{ID: 7, Email: web3TestEmail, Username: "alice"}
}

// User-facing DTOs hide the synthetic Web3 email.
func TestUserFacingMappersHideWeb3SyntheticEmail(t *testing.T) {
	require.Empty(t, UserFromServiceShallow(web3TestUser()).Email)
	require.Empty(t, UserFromService(web3TestUser()).Email)
	require.Empty(t, APIKeyFromService(&service.APIKey{User: web3TestUser()}).User.Email)
	require.Empty(t, RedeemCodeFromService(&service.RedeemCode{User: web3TestUser()}).User.Email)
	require.Empty(t, UsageLogFromService(&service.UsageLog{User: web3TestUser()}).User.Email)
	require.Empty(t, UserSubscriptionFromService(&service.UserSubscription{User: web3TestUser()}).User.Email)
}

// Admin DTOs keep it, including nested users, so operators can identify and
// search Web3 accounts from every admin surface.
func TestAdminMappersKeepWeb3SyntheticEmail(t *testing.T) {
	require.Equal(t, web3TestEmail, UserFromServiceAdmin(web3TestUser()).Email)
	require.Equal(t, web3TestEmail, APIKeyFromServiceAdmin(&service.APIKey{User: web3TestUser()}).User.Email)
	require.Equal(t, web3TestEmail, RedeemCodeFromServiceAdmin(&service.RedeemCode{User: web3TestUser()}).User.Email)
	require.Equal(t, web3TestEmail, UsageLogFromServiceAdmin(&service.UsageLog{User: web3TestUser()}).User.Email)
	require.Equal(t, web3TestEmail, PromoCodeUsageFromService(&service.PromoCodeUsage{User: web3TestUser()}).User.Email)

	sub := UserSubscriptionFromServiceAdmin(&service.UserSubscription{
		User:           web3TestUser(),
		AssignedByUser: web3TestUser(),
	})
	require.Equal(t, web3TestEmail, sub.User.Email)
	require.Equal(t, web3TestEmail, sub.AssignedByUser.Email)
}

// Regular emails must round-trip untouched on both sides.
func TestMappersLeaveRegularEmailUntouched(t *testing.T) {
	u := &service.User{ID: 8, Email: "alice@example.com"}
	require.Equal(t, "alice@example.com", UserFromServiceShallow(u).Email)
	require.Equal(t, "alice@example.com", userFromServiceShallowAdmin(u).Email)
}

func TestRestoreAdminEmailIgnoresNil(t *testing.T) {
	require.NotPanics(t, func() {
		restoreAdminEmail(nil, web3TestUser())
		restoreAdminEmail(&User{}, nil)
	})
	require.Nil(t, userFromServiceShallowAdmin(nil))
}
