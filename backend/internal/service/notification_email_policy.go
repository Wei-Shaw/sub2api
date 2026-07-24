package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"sort"
	"strings"
)

var ErrNotificationEmailChannelDisabled = errors.New("notification email channel is disabled")

const (
	NotificationEmailPolicyVersion    = 1
	SettingKeyNotificationEmailPolicy = "notification_email_policy_v1"

	NotificationEmailChannelAuthVerification = "auth_verification"
	NotificationEmailChannelPasswordReset    = "password_reset"
	NotificationEmailChannelSubscription     = "subscription"
	NotificationEmailChannelBalance          = "balance"
	NotificationEmailChannelAccountQuota     = "account_quota"
	NotificationEmailChannelRiskControl      = "risk_control"
	NotificationEmailChannelRefundAdmin      = "refund_admin"
	NotificationEmailChannelRefundUser       = "refund_user"
	NotificationEmailChannelOpsAlert         = "ops_alert"
	NotificationEmailChannelOpsReport        = "ops_report"

	NotificationEmailRecipientKindExplicit = "explicit"
	NotificationEmailRecipientKindUser     = "user"
	NotificationEmailRecipientKindGroup    = "group"

	NotificationEmailRecipientGroupFinance      = "finance"
	NotificationEmailRecipientGroupAccountQuota = "account_quota"
	NotificationEmailRecipientGroupSecurity     = "security"
	NotificationEmailRecipientGroupOpsAlert     = "ops_alert"
	NotificationEmailRecipientGroupOpsReport    = "ops_report"

	NotificationEmailRecipientStatusVerified         = "verified"
	NotificationEmailRecipientStatusAdminTrusted     = "admin_trusted"
	NotificationEmailRecipientStatusLegacyUnverified = "legacy_unverified"

	notificationEmailMaxRecipientGroupMembers = 20
)

type NotificationEmailChannelPolicy struct {
	ID                        string   `json:"id"`
	Enabled                   bool     `json:"enabled"`
	RecipientKind             string   `json:"recipient_kind"`
	RecipientGroup            string   `json:"recipient_group,omitempty"`
	AllowUserPrimary          bool     `json:"allow_user_primary,omitempty"`
	AllowVerifiedAdditional   bool     `json:"allow_verified_additional,omitempty"`
	IncludeUserPrimary        bool     `json:"include_user_primary,omitempty"`
	IncludeVerifiedAdditional bool     `json:"include_verified_additional,omitempty"`
	Events                    []string `json:"events"`
}

type NotificationEmailRecipientMember struct {
	Email   string `json:"email"`
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
}

type NotificationEmailRecipientGroup struct {
	ID      string                             `json:"id"`
	Members []NotificationEmailRecipientMember `json:"members"`
}

type NotificationEmailPolicyView struct {
	Version         int                               `json:"version"`
	Configured      bool                              `json:"configured"`
	Channels        []NotificationEmailChannelPolicy  `json:"channels"`
	RecipientGroups []NotificationEmailRecipientGroup `json:"recipient_groups"`
}

type NotificationEmailPolicyUpdate struct {
	Channels        []NotificationEmailChannelPolicy  `json:"channels"`
	RecipientGroups []NotificationEmailRecipientGroup `json:"recipient_groups"`
}

type notificationEmailStoredPolicy struct {
	Version         int                                        `json:"version"`
	Channels        map[string]NotificationEmailChannelPolicy  `json:"channels"`
	RecipientGroups map[string]NotificationEmailRecipientGroup `json:"recipient_groups"`
}

type notificationEmailChannelDefinition struct {
	ID                       string
	RecipientKind            string
	DefaultRecipientGroup    string
	DefaultEnabled           bool
	AllowUserPrimary         bool
	AllowVerifiedAdditional  bool
	DefaultIncludePrimary    bool
	DefaultIncludeAdditional bool
	Events                   []string
}

var notificationEmailChannelDefinitions = []notificationEmailChannelDefinition{
	{ID: NotificationEmailChannelAuthVerification, RecipientKind: NotificationEmailRecipientKindExplicit, DefaultEnabled: true, Events: []string{NotificationEmailEventAuthVerifyCode, NotificationEmailEventNotificationEmailVerifyCode}},
	{ID: NotificationEmailChannelPasswordReset, RecipientKind: NotificationEmailRecipientKindExplicit, DefaultEnabled: true, Events: []string{NotificationEmailEventAuthPasswordReset}},
	{ID: NotificationEmailChannelSubscription, RecipientKind: NotificationEmailRecipientKindUser, DefaultEnabled: true, AllowUserPrimary: true, DefaultIncludePrimary: true, Events: []string{NotificationEmailEventSubscriptionPurchaseSuccess, NotificationEmailEventSubscriptionExpiryReminder}},
	{ID: NotificationEmailChannelBalance, RecipientKind: NotificationEmailRecipientKindUser, DefaultEnabled: true, AllowUserPrimary: true, AllowVerifiedAdditional: true, DefaultIncludeAdditional: true, Events: []string{NotificationEmailEventBalanceLow, NotificationEmailEventBalanceRechargeSuccess}},
	{ID: NotificationEmailChannelAccountQuota, RecipientKind: NotificationEmailRecipientKindGroup, DefaultRecipientGroup: NotificationEmailRecipientGroupAccountQuota, DefaultEnabled: true, Events: []string{NotificationEmailEventAccountQuotaAlert}},
	{ID: NotificationEmailChannelRiskControl, RecipientKind: NotificationEmailRecipientKindUser, DefaultEnabled: true, AllowUserPrimary: true, DefaultIncludePrimary: true, Events: []string{NotificationEmailEventContentModerationViolation, NotificationEmailEventContentModerationDisabled, NotificationEmailEventCyberPolicyNotice}},
	{ID: NotificationEmailChannelRefundAdmin, RecipientKind: NotificationEmailRecipientKindGroup, DefaultRecipientGroup: NotificationEmailRecipientGroupFinance, DefaultEnabled: false},
	{ID: NotificationEmailChannelRefundUser, RecipientKind: NotificationEmailRecipientKindUser, DefaultEnabled: false, AllowUserPrimary: true, DefaultIncludePrimary: true, Events: []string{NotificationEmailEventRefundRequestedUser}},
	{ID: NotificationEmailChannelOpsAlert, RecipientKind: NotificationEmailRecipientKindGroup, DefaultRecipientGroup: NotificationEmailRecipientGroupOpsAlert, DefaultEnabled: false, Events: []string{NotificationEmailEventOpsAlert}},
	{ID: NotificationEmailChannelOpsReport, RecipientKind: NotificationEmailRecipientKindGroup, DefaultRecipientGroup: NotificationEmailRecipientGroupOpsReport, DefaultEnabled: false, Events: []string{NotificationEmailEventOpsScheduledReport}},
}

var notificationEmailRecipientGroupOrder = []string{
	NotificationEmailRecipientGroupFinance,
	NotificationEmailRecipientGroupAccountQuota,
	NotificationEmailRecipientGroupSecurity,
	NotificationEmailRecipientGroupOpsAlert,
	NotificationEmailRecipientGroupOpsReport,
}

var notificationEmailEventChannels = map[string]string{
	NotificationEmailEventAuthVerifyCode:              NotificationEmailChannelAuthVerification,
	NotificationEmailEventNotificationEmailVerifyCode: NotificationEmailChannelAuthVerification,
	NotificationEmailEventAuthPasswordReset:           NotificationEmailChannelPasswordReset,
	NotificationEmailEventSubscriptionPurchaseSuccess: NotificationEmailChannelSubscription,
	NotificationEmailEventSubscriptionExpiryReminder:  NotificationEmailChannelSubscription,
	NotificationEmailEventBalanceLow:                  NotificationEmailChannelBalance,
	NotificationEmailEventBalanceRechargeSuccess:      NotificationEmailChannelBalance,
	NotificationEmailEventRefundRequestedUser:         NotificationEmailChannelRefundUser,
	NotificationEmailEventAccountQuotaAlert:           NotificationEmailChannelAccountQuota,
	NotificationEmailEventContentModerationViolation:  NotificationEmailChannelRiskControl,
	NotificationEmailEventContentModerationDisabled:   NotificationEmailChannelRiskControl,
	NotificationEmailEventCyberPolicyNotice:           NotificationEmailChannelRiskControl,
	NotificationEmailEventOpsAlert:                    NotificationEmailChannelOpsAlert,
	NotificationEmailEventOpsScheduledReport:          NotificationEmailChannelOpsReport,
}

func (s *NotificationEmailService) GetPolicy(ctx context.Context) (NotificationEmailPolicyView, error) {
	policy, configured, err := s.loadPolicy(ctx)
	if err != nil {
		return NotificationEmailPolicyView{}, err
	}
	return notificationEmailPolicyView(policy, configured), nil
}

func (s *NotificationEmailService) GetChannelPolicy(ctx context.Context, channelID string) (NotificationEmailChannelPolicy, error) {
	channel, _, err := s.GetChannelPolicyState(ctx, channelID)
	return channel, err
}

func (s *NotificationEmailService) GetChannelPolicyState(ctx context.Context, channelID string) (NotificationEmailChannelPolicy, bool, error) {
	policy, configured, err := s.loadPolicy(ctx)
	if err != nil {
		return NotificationEmailChannelPolicy{}, false, err
	}
	channel, ok := policy.Channels[strings.TrimSpace(channelID)]
	if !ok {
		return NotificationEmailChannelPolicy{}, false, fmt.Errorf("unsupported notification email channel: %s", channelID)
	}
	return channel, configured, nil
}

func (s *NotificationEmailService) requireEventChannelEnabled(ctx context.Context, event string) error {
	channelID, ok := notificationEmailEventChannels[event]
	if !ok {
		return fmt.Errorf("notification email event has no channel policy: %s", event)
	}
	policy, configured, err := s.loadPolicy(ctx)
	if err != nil {
		return err
	}
	if !configured {
		// Refund mail is new and has no legacy behavior to preserve. Its
		// default-off policy remains closed until an administrator saves it.
		if channelID == NotificationEmailChannelRefundUser && !policy.Channels[channelID].Enabled {
			return fmt.Errorf("%w: %s", ErrNotificationEmailChannelDisabled, channelID)
		}
		return nil
	}
	channel := policy.Channels[channelID]
	if !channel.Enabled {
		return fmt.Errorf("%w: %s", ErrNotificationEmailChannelDisabled, channelID)
	}
	return nil
}

func (s *NotificationEmailService) ResolveGroupRecipients(ctx context.Context, channelID string) ([]string, error) {
	policy, configured, err := s.loadPolicy(ctx)
	if err != nil {
		return nil, err
	}
	if !configured {
		return nil, nil
	}
	channel, ok := policy.Channels[strings.TrimSpace(channelID)]
	if !ok {
		return nil, fmt.Errorf("unsupported notification email channel: %s", channelID)
	}
	if !channel.Enabled || channel.RecipientKind != NotificationEmailRecipientKindGroup {
		return []string{}, nil
	}
	group, ok := policy.RecipientGroups[channel.RecipientGroup]
	if !ok {
		return []string{}, nil
	}
	recipients := make([]string, 0, len(group.Members))
	for _, member := range group.Members {
		if member.Enabled && member.Status != NotificationEmailRecipientStatusLegacyUnverified {
			recipients = append(recipients, member.Email)
		}
	}
	return recipients, nil
}

func (s *NotificationEmailService) ResolveUserRecipients(ctx context.Context, channelID, primary string, additional []NotifyEmailEntry) ([]string, error) {
	channel, err := s.GetChannelPolicy(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if !channel.Enabled || channel.RecipientKind != NotificationEmailRecipientKindUser {
		return []string{}, nil
	}
	seen := make(map[string]struct{})
	recipients := make([]string, 0, 1+len(additional))
	appendRecipient := func(email string) {
		email = strings.TrimSpace(email)
		key := strings.ToLower(email)
		if email == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		recipients = append(recipients, email)
	}
	if channel.IncludeUserPrimary {
		appendRecipient(primary)
	}
	if channel.IncludeVerifiedAdditional {
		for _, entry := range additional {
			if !entry.Disabled && entry.Verified {
				appendRecipient(entry.Email)
			}
		}
	}
	return recipients, nil
}

func (s *NotificationEmailService) UpdatePolicy(ctx context.Context, update NotificationEmailPolicyUpdate) (NotificationEmailPolicyView, error) {
	if s == nil || s.settingRepo == nil {
		return NotificationEmailPolicyView{}, errors.New("notification email policy storage is not configured")
	}
	current, _, err := s.loadPolicy(ctx)
	if err != nil {
		return NotificationEmailPolicyView{}, err
	}
	definitions := notificationEmailChannelDefinitionMap()
	for _, channel := range update.Channels {
		definition, ok := definitions[strings.TrimSpace(channel.ID)]
		if !ok {
			return NotificationEmailPolicyView{}, fmt.Errorf("unsupported notification email channel: %s", channel.ID)
		}
		channel.ID = definition.ID
		channel.RecipientKind = definition.RecipientKind
		channel.Events = append([]string(nil), definition.Events...)
		channel.AllowUserPrimary = definition.AllowUserPrimary
		channel.AllowVerifiedAdditional = definition.AllowVerifiedAdditional
		switch definition.RecipientKind {
		case NotificationEmailRecipientKindExplicit:
			channel.RecipientGroup = ""
			channel.IncludeUserPrimary = false
			channel.IncludeVerifiedAdditional = false
		case NotificationEmailRecipientKindUser:
			channel.RecipientGroup = ""
			channel.IncludeUserPrimary = channel.IncludeUserPrimary && definition.AllowUserPrimary
			channel.IncludeVerifiedAdditional = channel.IncludeVerifiedAdditional && definition.AllowVerifiedAdditional
			if channel.Enabled && !channel.IncludeUserPrimary && !channel.IncludeVerifiedAdditional {
				return NotificationEmailPolicyView{}, fmt.Errorf("notification email channel %s must select at least one user recipient", channel.ID)
			}
		case NotificationEmailRecipientKindGroup:
			channel.RecipientGroup = strings.TrimSpace(channel.RecipientGroup)
			if channel.RecipientGroup == "" {
				channel.RecipientGroup = definition.DefaultRecipientGroup
			}
			if !isNotificationEmailRecipientGroup(channel.RecipientGroup) {
				return NotificationEmailPolicyView{}, fmt.Errorf("unsupported recipient group for channel %s: %s", channel.ID, channel.RecipientGroup)
			}
			channel.IncludeUserPrimary = false
			channel.IncludeVerifiedAdditional = false
		}
		current.Channels[channel.ID] = channel
	}

	for _, group := range update.RecipientGroups {
		group.ID = strings.TrimSpace(group.ID)
		if !isNotificationEmailRecipientGroup(group.ID) {
			return NotificationEmailPolicyView{}, fmt.Errorf("unsupported notification email recipient group: %s", group.ID)
		}
		normalized, err := normalizeNotificationEmailRecipientGroup(group, current.RecipientGroups[group.ID])
		if err != nil {
			return NotificationEmailPolicyView{}, err
		}
		current.RecipientGroups[group.ID] = normalized
	}

	payload, err := json.Marshal(current)
	if err != nil {
		return NotificationEmailPolicyView{}, fmt.Errorf("marshal notification email policy: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyNotificationEmailPolicy, string(payload)); err != nil {
		return NotificationEmailPolicyView{}, err
	}
	return notificationEmailPolicyView(current, true), nil
}

func (s *NotificationEmailService) loadPolicy(ctx context.Context) (notificationEmailStoredPolicy, bool, error) {
	if s == nil || s.settingRepo == nil {
		return notificationEmailStoredPolicy{}, false, errors.New("notification email policy storage is not configured")
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyNotificationEmailPolicy)
	if err == nil && strings.TrimSpace(raw) != "" {
		var stored notificationEmailStoredPolicy
		if jsonErr := json.Unmarshal([]byte(raw), &stored); jsonErr != nil {
			return notificationEmailStoredPolicy{}, false, fmt.Errorf("decode notification email policy: %w", jsonErr)
		}
		if stored.Version != NotificationEmailPolicyVersion {
			return notificationEmailStoredPolicy{}, false, fmt.Errorf("unsupported notification email policy version: %d", stored.Version)
		}
		return mergeNotificationEmailPolicyDefaults(stored), true, nil
	}
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return notificationEmailStoredPolicy{}, false, err
	}
	return s.legacyNotificationEmailPolicy(ctx), false, nil
}

func (s *NotificationEmailService) legacyNotificationEmailPolicy(ctx context.Context) notificationEmailStoredPolicy {
	policy := defaultNotificationEmailPolicy()
	values, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyAccountQuotaNotifyEmails, SettingKeyOpsEmailNotificationConfig})
	if err != nil {
		return policy
	}
	if raw := strings.TrimSpace(values[SettingKeyAccountQuotaNotifyEmails]); raw != "" {
		members := make([]NotificationEmailRecipientMember, 0)
		for _, entry := range ParseNotifyEmails(raw) {
			status := NotificationEmailRecipientStatusLegacyUnverified
			if entry.Verified {
				status = NotificationEmailRecipientStatusVerified
			}
			members = append(members, NotificationEmailRecipientMember{
				Email: strings.TrimSpace(entry.Email), Enabled: !entry.Disabled && entry.Verified, Status: status,
			})
		}
		group, normalizeErr := normalizeNotificationEmailRecipientGroup(
			NotificationEmailRecipientGroup{ID: NotificationEmailRecipientGroupAccountQuota, Members: members},
			NotificationEmailRecipientGroup{ID: NotificationEmailRecipientGroupAccountQuota, Members: members},
		)
		if normalizeErr == nil {
			policy.RecipientGroups[group.ID] = group
		}
	}
	if raw := strings.TrimSpace(values[SettingKeyOpsEmailNotificationConfig]); raw != "" {
		var opsConfig OpsEmailNotificationConfig
		if json.Unmarshal([]byte(raw), &opsConfig) == nil {
			alert := policy.Channels[NotificationEmailChannelOpsAlert]
			alert.Enabled = opsConfig.Alert.Enabled
			policy.Channels[alert.ID] = alert
			report := policy.Channels[NotificationEmailChannelOpsReport]
			report.Enabled = opsConfig.Report.Enabled
			policy.Channels[report.ID] = report
			policy.RecipientGroups[NotificationEmailRecipientGroupOpsAlert] = trustedNotificationEmailRecipientGroup(NotificationEmailRecipientGroupOpsAlert, opsConfig.Alert.Recipients)
			policy.RecipientGroups[NotificationEmailRecipientGroupOpsReport] = trustedNotificationEmailRecipientGroup(NotificationEmailRecipientGroupOpsReport, opsConfig.Report.Recipients)
		}
	}
	return policy
}

func defaultNotificationEmailPolicy() notificationEmailStoredPolicy {
	policy := notificationEmailStoredPolicy{
		Version:         NotificationEmailPolicyVersion,
		Channels:        make(map[string]NotificationEmailChannelPolicy, len(notificationEmailChannelDefinitions)),
		RecipientGroups: make(map[string]NotificationEmailRecipientGroup, len(notificationEmailRecipientGroupOrder)),
	}
	for _, definition := range notificationEmailChannelDefinitions {
		policy.Channels[definition.ID] = NotificationEmailChannelPolicy{
			ID: definition.ID, Enabled: definition.DefaultEnabled, RecipientKind: definition.RecipientKind,
			RecipientGroup: definition.DefaultRecipientGroup, IncludeUserPrimary: definition.DefaultIncludePrimary,
			AllowUserPrimary: definition.AllowUserPrimary, AllowVerifiedAdditional: definition.AllowVerifiedAdditional,
			IncludeVerifiedAdditional: definition.DefaultIncludeAdditional, Events: append([]string(nil), definition.Events...),
		}
	}
	for _, id := range notificationEmailRecipientGroupOrder {
		policy.RecipientGroups[id] = NotificationEmailRecipientGroup{ID: id, Members: []NotificationEmailRecipientMember{}}
	}
	return policy
}

func mergeNotificationEmailPolicyDefaults(stored notificationEmailStoredPolicy) notificationEmailStoredPolicy {
	merged := defaultNotificationEmailPolicy()
	for id, channel := range stored.Channels {
		if _, ok := merged.Channels[id]; ok {
			definition := notificationEmailChannelDefinitionMap()[id]
			channel.ID = id
			channel.RecipientKind = definition.RecipientKind
			channel.Events = append([]string(nil), definition.Events...)
			channel.AllowUserPrimary = definition.AllowUserPrimary
			channel.AllowVerifiedAdditional = definition.AllowVerifiedAdditional
			if definition.RecipientKind == NotificationEmailRecipientKindGroup && strings.TrimSpace(channel.RecipientGroup) == "" {
				channel.RecipientGroup = definition.DefaultRecipientGroup
			}
			merged.Channels[id] = channel
		}
	}
	for id, group := range stored.RecipientGroups {
		if isNotificationEmailRecipientGroup(id) {
			group.ID = id
			merged.RecipientGroups[id] = group
		}
	}
	return merged
}

func notificationEmailPolicyView(policy notificationEmailStoredPolicy, configured bool) NotificationEmailPolicyView {
	view := NotificationEmailPolicyView{Version: NotificationEmailPolicyVersion, Configured: configured}
	for _, definition := range notificationEmailChannelDefinitions {
		channel := policy.Channels[definition.ID]
		channel.Events = append([]string(nil), channel.Events...)
		view.Channels = append(view.Channels, channel)
	}
	for _, id := range notificationEmailRecipientGroupOrder {
		group := policy.RecipientGroups[id]
		group.Members = append([]NotificationEmailRecipientMember(nil), group.Members...)
		view.RecipientGroups = append(view.RecipientGroups, group)
	}
	return view
}

func normalizeNotificationEmailRecipientGroup(group, previous NotificationEmailRecipientGroup) (NotificationEmailRecipientGroup, error) {
	if len(group.Members) > notificationEmailMaxRecipientGroupMembers {
		return NotificationEmailRecipientGroup{}, fmt.Errorf("recipient group %s cannot exceed %d members", group.ID, notificationEmailMaxRecipientGroupMembers)
	}
	previousByEmail := make(map[string]NotificationEmailRecipientMember, len(previous.Members))
	for _, member := range previous.Members {
		previousByEmail[strings.ToLower(strings.TrimSpace(member.Email))] = member
	}
	seen := make(map[string]struct{}, len(group.Members))
	normalized := NotificationEmailRecipientGroup{ID: group.ID, Members: make([]NotificationEmailRecipientMember, 0, len(group.Members))}
	for _, member := range group.Members {
		email := strings.TrimSpace(member.Email)
		if email == "" {
			continue
		}
		address, err := mail.ParseAddress(email)
		if err != nil || !strings.EqualFold(address.Address, email) {
			return NotificationEmailRecipientGroup{}, fmt.Errorf("invalid recipient email in group %s: %s", group.ID, email)
		}
		key := strings.ToLower(email)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		status := NotificationEmailRecipientStatusAdminTrusted
		if existing, ok := previousByEmail[key]; ok {
			status = existing.Status
			if status == NotificationEmailRecipientStatusLegacyUnverified && member.Enabled {
				status = NotificationEmailRecipientStatusAdminTrusted
			}
		}
		if status == "" {
			status = NotificationEmailRecipientStatusAdminTrusted
		}
		normalized.Members = append(normalized.Members, NotificationEmailRecipientMember{Email: address.Address, Enabled: member.Enabled, Status: status})
	}
	sort.SliceStable(normalized.Members, func(i, j int) bool {
		return strings.ToLower(normalized.Members[i].Email) < strings.ToLower(normalized.Members[j].Email)
	})
	return normalized, nil
}

func trustedNotificationEmailRecipientGroup(id string, emails []string) NotificationEmailRecipientGroup {
	group := NotificationEmailRecipientGroup{ID: id, Members: make([]NotificationEmailRecipientMember, 0, len(emails))}
	for _, email := range emails {
		group.Members = append(group.Members, NotificationEmailRecipientMember{Email: email, Enabled: true, Status: NotificationEmailRecipientStatusAdminTrusted})
	}
	normalized, err := normalizeNotificationEmailRecipientGroup(group, group)
	if err != nil {
		return NotificationEmailRecipientGroup{ID: id, Members: []NotificationEmailRecipientMember{}}
	}
	return normalized
}

func notificationEmailChannelDefinitionMap() map[string]notificationEmailChannelDefinition {
	definitions := make(map[string]notificationEmailChannelDefinition, len(notificationEmailChannelDefinitions))
	for _, definition := range notificationEmailChannelDefinitions {
		definitions[definition.ID] = definition
	}
	return definitions
}

func isNotificationEmailRecipientGroup(id string) bool {
	for _, candidate := range notificationEmailRecipientGroupOrder {
		if candidate == id {
			return true
		}
	}
	return false
}
