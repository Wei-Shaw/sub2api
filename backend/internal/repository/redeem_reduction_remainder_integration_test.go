//go:build integration

package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRedeemReductionPreservesPartialDay(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	user, err := client.User.Create().SetEmail("reduction-remainder@example.com").SetPasswordHash("test").Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().SetName("reduction-remainder").Save(ctx)
	require.NoError(t, err)
	// Redemption commits its own transaction; remove fixtures explicitly so
	// repository list/count tests cannot observe this test's subscriptions.
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(context.Background(), `DELETE FROM redeem_codes WHERE group_id=$1`, group.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(context.Background(), `DELETE FROM user_subscriptions WHERE user_id=$1`, user.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(context.Background(), `DELETE FROM groups WHERE id=$1`, group.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, user.ID)
		require.NoError(t, err)
	})
	now := time.Now().UTC().Truncate(time.Microsecond)
	expiry := now.Add(36 * time.Hour)
	sub, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(group.ID).SetStartsAt(now.Add(-time.Hour)).SetExpiresAt(expiry).SetStatus(service.SubscriptionStatusActive).Save(ctx)
	require.NoError(t, err)
	repo := NewUserSubscriptionRepository(client)
	codes := NewRedeemCodeRepository(client)
	code := &service.RedeemCode{Code: "REDUCE-ONE-DAY", Type: service.RedeemTypeSubscription, Status: service.StatusUnused, ValidityDays: -1, GroupID: &group.ID}
	require.NoError(t, codes.Create(ctx, code))
	subscriptions := service.NewSubscriptionService(nil, repo, nil, client, nil)
	redeem := service.NewRedeemService(codes, NewUserRepository(client, integrationDB), subscriptions, nil, nil, client, nil, nil)
	_, err = redeem.RedeemForAdminFulfillment(ctx, user.ID, code.Code)
	require.NoError(t, err)
	got, err := repo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, service.SubscriptionStatusActive, got.Status)
	require.True(t, expiry.AddDate(0, 0, -1).Equal(got.ExpiresAt))
	used, err := codes.GetByID(ctx, code.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusUsed, used.Status)
}
