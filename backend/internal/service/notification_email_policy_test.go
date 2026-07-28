package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotificationEmailPolicyDefaultsPreserveExistingBehavior(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)

	view, err := svc.GetPolicy(context.Background())
	require.NoError(t, err)
	require.Equal(t, NotificationEmailPolicyVersion, view.Version)
	require.False(t, view.Configured)

	channels := notificationEmailPolicyChannelsByID(view.Channels)
	require.True(t, channels[NotificationEmailChannelAuthVerification].Enabled)
	require.True(t, channels[NotificationEmailChannelBalance].Enabled)
	require.False(t, channels[NotificationEmailChannelAccountQuota].Enabled)
	require.False(t, channels[NotificationEmailChannelRefundAdmin].Enabled)
	require.False(t, channels[NotificationEmailChannelRefundUser].Enabled)
	require.Contains(t, channels[NotificationEmailChannelRefundUser].Events, NotificationEmailEventRefundRequestedUser)
	require.False(t, channels[NotificationEmailChannelOpsAlert].Enabled)
	require.False(t, channels[NotificationEmailChannelOpsReport].Enabled)
	require.Equal(t, NotificationEmailRecipientKindExplicit, channels[NotificationEmailChannelAuthVerification].RecipientKind)
	require.Equal(t, NotificationEmailRecipientKindUser, channels[NotificationEmailChannelBalance].RecipientKind)
	require.False(t, channels[NotificationEmailChannelBalance].IncludeUserPrimary)
	require.True(t, channels[NotificationEmailChannelBalance].IncludeVerifiedAdditional)
	require.Equal(t, NotificationEmailRecipientGroupAccountQuota, channels[NotificationEmailChannelAccountQuota].RecipientGroup)

	payload, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(payload), `"members":null`)
	require.Contains(t, string(payload), `"members":[]`)
}

func TestNotificationEmailPolicyKeepsNewRefundEventOffUntilPolicyIsSaved(t *testing.T) {
	ctx := context.Background()
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)

	err := svc.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventRefundRequestedUser,
		RecipientEmail: "user@example.com",
	})
	require.ErrorIs(t, err, ErrNotificationEmailChannelDisabled)
}

func TestNotificationEmailPolicyImportsLegacyRecipientsWithoutActivatingUnverifiedAddresses(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	require.NoError(t, repo.Set(ctx, SettingKeyAccountQuotaNotifyEmails, `[
		{"email":"verified@example.com","disabled":false,"verified":true},
		{"email":"legacy@example.com","disabled":false,"verified":false}
	]`))
	opsConfig := OpsEmailNotificationConfig{
		Alert:  OpsEmailAlertConfig{Enabled: true, Recipients: []string{"oncall@example.com"}},
		Report: OpsEmailReportConfig{Enabled: true, Recipients: []string{"reports@example.com"}},
	}
	raw, err := json.Marshal(opsConfig)
	require.NoError(t, err)
	require.NoError(t, repo.Set(ctx, SettingKeyOpsEmailNotificationConfig, string(raw)))

	view, err := NewNotificationEmailService(repo, nil).GetPolicy(ctx)
	require.NoError(t, err)
	channels := notificationEmailPolicyChannelsByID(view.Channels)
	require.True(t, channels[NotificationEmailChannelOpsAlert].Enabled)
	require.True(t, channels[NotificationEmailChannelOpsReport].Enabled)

	groups := notificationEmailPolicyGroupsByID(view.RecipientGroups)
	quota := groups[NotificationEmailRecipientGroupAccountQuota]
	require.Len(t, quota.Members, 2)
	require.Equal(t, NotificationEmailRecipientStatusLegacyUnverified, quota.Members[0].Status)
	require.False(t, quota.Members[0].Enabled)
	require.Equal(t, NotificationEmailRecipientStatusVerified, quota.Members[1].Status)
	require.True(t, quota.Members[1].Enabled)
	require.Equal(t, "oncall@example.com", groups[NotificationEmailRecipientGroupOpsAlert].Members[0].Email)
	require.Equal(t, NotificationEmailRecipientStatusAdminTrusted, groups[NotificationEmailRecipientGroupOpsAlert].Members[0].Status)
}

func TestNotificationEmailPolicyUpdatePersistsSwitchesAndPromotesExplicitlyEnabledLegacyRecipient(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	require.NoError(t, repo.Set(ctx, SettingKeyAccountQuotaNotifyEmails, `[{"email":"legacy@example.com","disabled":false,"verified":false}]`))
	svc := NewNotificationEmailService(repo, nil)

	updated, err := svc.UpdatePolicy(ctx, NotificationEmailPolicyUpdate{
		Channels: []NotificationEmailChannelPolicy{
			{ID: NotificationEmailChannelRefundAdmin, Enabled: true, RecipientGroup: NotificationEmailRecipientGroupFinance},
			{ID: NotificationEmailChannelBalance, Enabled: true, IncludeUserPrimary: true},
		},
		RecipientGroups: []NotificationEmailRecipientGroup{
			{ID: NotificationEmailRecipientGroupFinance, Members: []NotificationEmailRecipientMember{{Email: "billing@example.com", Enabled: true, Status: "forged"}}},
			{ID: NotificationEmailRecipientGroupAccountQuota, Members: []NotificationEmailRecipientMember{{Email: "legacy@example.com", Enabled: true}}},
		},
	})
	require.NoError(t, err)
	require.True(t, updated.Configured)
	channels := notificationEmailPolicyChannelsByID(updated.Channels)
	require.True(t, channels[NotificationEmailChannelRefundAdmin].Enabled)
	groups := notificationEmailPolicyGroupsByID(updated.RecipientGroups)
	require.Equal(t, NotificationEmailRecipientStatusAdminTrusted, groups[NotificationEmailRecipientGroupFinance].Members[0].Status)
	require.Equal(t, NotificationEmailRecipientStatusAdminTrusted, groups[NotificationEmailRecipientGroupAccountQuota].Members[0].Status)

	reloaded, err := svc.GetPolicy(ctx)
	require.NoError(t, err)
	require.True(t, reloaded.Configured)
	require.True(t, notificationEmailPolicyChannelsByID(reloaded.Channels)[NotificationEmailChannelRefundAdmin].Enabled)
}

func TestNotificationEmailPolicyRejectsUnsafeOrInvalidRouting(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	ctx := context.Background()

	_, err := svc.UpdatePolicy(ctx, NotificationEmailPolicyUpdate{Channels: []NotificationEmailChannelPolicy{{ID: "unknown", Enabled: true}}})
	require.ErrorContains(t, err, "unsupported notification email channel")

	_, err = svc.UpdatePolicy(ctx, NotificationEmailPolicyUpdate{Channels: []NotificationEmailChannelPolicy{{ID: NotificationEmailChannelBalance, Enabled: true}}})
	require.ErrorContains(t, err, "must select at least one user recipient")

	_, err = svc.UpdatePolicy(ctx, NotificationEmailPolicyUpdate{RecipientGroups: []NotificationEmailRecipientGroup{{
		ID:      NotificationEmailRecipientGroupFinance,
		Members: []NotificationEmailRecipientMember{{Email: "Billing Team <billing@example.com>", Enabled: true}},
	}}})
	require.ErrorContains(t, err, "invalid recipient email")
}

func TestNotificationEmailPolicySystemRequiredChannelsCannotBeDisabled(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	svc := NewNotificationEmailService(repo, nil)
	updated, err := svc.UpdatePolicy(ctx, NotificationEmailPolicyUpdate{Channels: []NotificationEmailChannelPolicy{
		{ID: NotificationEmailChannelAuthVerification, Enabled: false},
		{ID: NotificationEmailChannelPasswordReset, Enabled: false},
	}})
	require.NoError(t, err)
	channels := notificationEmailPolicyChannelsByID(updated.Channels)
	require.True(t, channels[NotificationEmailChannelAuthVerification].Enabled)
	require.True(t, channels[NotificationEmailChannelPasswordReset].Enabled)
	require.NoError(t, svc.requireEventChannelEnabled(ctx, NotificationEmailEventAuthVerifyCode))
	require.NoError(t, svc.requireEventChannelEnabled(ctx, NotificationEmailEventAuthPasswordReset))

	var stored notificationEmailStoredPolicy
	raw, err := repo.GetValue(ctx, SettingKeyNotificationEmailPolicy)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	require.True(t, stored.Channels[NotificationEmailChannelAuthVerification].Enabled)
	require.True(t, stored.Channels[NotificationEmailChannelPasswordReset].Enabled)
}

func TestNotificationEmailPolicyRepairsStoredDisabledSystemRequiredChannels(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	stored := defaultNotificationEmailPolicy()
	auth := stored.Channels[NotificationEmailChannelAuthVerification]
	auth.Enabled = false
	stored.Channels[auth.ID] = auth
	password := stored.Channels[NotificationEmailChannelPasswordReset]
	password.Enabled = false
	stored.Channels[password.ID] = password
	raw, err := json.Marshal(stored)
	require.NoError(t, err)
	require.NoError(t, repo.Set(ctx, SettingKeyNotificationEmailPolicy, string(raw)))

	view, err := NewNotificationEmailService(repo, nil).GetPolicy(ctx)
	require.NoError(t, err)
	channels := notificationEmailPolicyChannelsByID(view.Channels)
	require.True(t, channels[NotificationEmailChannelAuthVerification].Enabled)
	require.True(t, channels[NotificationEmailChannelPasswordReset].Enabled)
}

func TestNotificationEmailPolicySavesFeatureSettingsAndLegacyRoutingTogether(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	svc := NewNotificationEmailService(repo, nil)

	updated, err := svc.UpdatePolicy(ctx, NotificationEmailPolicyUpdate{
		Channels: []NotificationEmailChannelPolicy{
			{
				ID:             NotificationEmailChannelAccountQuota,
				Enabled:        true,
				RecipientGroup: NotificationEmailRecipientGroupAccountQuota,
			},
		},
		RecipientGroups: []NotificationEmailRecipientGroup{
			{
				ID: NotificationEmailRecipientGroupAccountQuota,
				Members: []NotificationEmailRecipientMember{
					{Email: "quota@example.com", Enabled: true},
				},
			},
		},
		FeatureSettings: &NotificationEmailFeatureSettings{
			SubscriptionExpiryEnabled: false,
			BalanceLowEnabled:         true,
			BalanceLowThreshold:       12.5,
			BalanceLowRechargeURL:     "https://example.com/recharge",
		},
	})
	require.NoError(t, err)
	require.False(t, updated.FeatureSettings.SubscriptionExpiryEnabled)
	require.True(t, updated.FeatureSettings.BalanceLowEnabled)
	require.Equal(t, 12.5, updated.FeatureSettings.BalanceLowThreshold)

	values, err := repo.GetMultiple(ctx, []string{
		SettingKeyNotificationEmailPolicy,
		SettingKeySubscriptionExpiryNotifyEnabled,
		SettingKeyBalanceLowNotifyEnabled,
		SettingKeyBalanceLowNotifyThreshold,
		SettingKeyBalanceLowNotifyRechargeURL,
		SettingKeyAccountQuotaNotifyEnabled,
		SettingKeyAccountQuotaNotifyEmails,
	})
	require.NoError(t, err)
	require.NotEmpty(t, values[SettingKeyNotificationEmailPolicy])
	require.Equal(t, "false", values[SettingKeySubscriptionExpiryNotifyEnabled])
	require.Equal(t, "true", values[SettingKeyBalanceLowNotifyEnabled])
	require.Equal(t, "12.50000000", values[SettingKeyBalanceLowNotifyThreshold])
	require.Equal(t, "https://example.com/recharge", values[SettingKeyBalanceLowNotifyRechargeURL])
	require.Equal(t, "true", values[SettingKeyAccountQuotaNotifyEnabled])
	require.Contains(t, values[SettingKeyAccountQuotaNotifyEmails], "quota@example.com")
}

func TestNotificationEmailPolicyRejectsInvalidFineGrainedSettings(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	ctx := context.Background()

	_, err := svc.UpdatePolicy(ctx, NotificationEmailPolicyUpdate{
		FeatureSettings: &NotificationEmailFeatureSettings{BalanceLowEnabled: true},
	})
	require.ErrorContains(t, err, "balance_low_threshold")

	_, err = svc.UpdatePolicy(ctx, NotificationEmailPolicyUpdate{
		FeatureSettings: &NotificationEmailFeatureSettings{
			BalanceLowThreshold:   10,
			BalanceLowRechargeURL: "javascript:alert(1)",
		},
	})
	require.ErrorContains(t, err, "balance_low_recharge_url")
}

func notificationEmailPolicyChannelsByID(channels []NotificationEmailChannelPolicy) map[string]NotificationEmailChannelPolicy {
	result := make(map[string]NotificationEmailChannelPolicy, len(channels))
	for _, channel := range channels {
		result[channel.ID] = channel
	}
	return result
}

func notificationEmailPolicyGroupsByID(groups []NotificationEmailRecipientGroup) map[string]NotificationEmailRecipientGroup {
	result := make(map[string]NotificationEmailRecipientGroup, len(groups))
	for _, group := range groups {
		result[group.ID] = group
	}
	return result
}
