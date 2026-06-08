//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func mustCreateUser(t *testing.T, client *dbent.Client, u *service.User) *service.User {
	t.Helper()
	ctx := context.Background()

	if u.Email == "" {
		u.Email = "user-" + time.Now().Format(time.RFC3339Nano) + "@example.com"
	}
	if u.PasswordHash == "" {
		u.PasswordHash = "test-password-hash"
	}
	if u.Role == "" {
		u.Role = service.RoleUser
	}
	if u.Status == "" {
		u.Status = service.StatusActive
	}
	if u.Concurrency == 0 {
		u.Concurrency = 5
	}

	create := client.User.Create().
		SetEmail(u.Email).
		SetPasswordHash(u.PasswordHash).
		SetRole(u.Role).
		SetStatus(u.Status).
		SetBalance(u.Balance).
		SetConcurrency(u.Concurrency).
		SetUsername(u.Username).
		SetNotes(u.Notes)
	if !u.CreatedAt.IsZero() {
		create.SetCreatedAt(u.CreatedAt)
	}
	if !u.UpdatedAt.IsZero() {
		create.SetUpdatedAt(u.UpdatedAt)
	}

	created, err := create.Save(ctx)
	require.NoError(t, err, "create user")

	u.ID = created.ID
	u.CreatedAt = created.CreatedAt
	u.UpdatedAt = created.UpdatedAt

	if len(u.AllowedGroups) > 0 {
		for _, groupID := range u.AllowedGroups {
			_, err := client.UserAllowedGroup.Create().
				SetUserID(u.ID).
				SetGroupID(groupID).
				Save(ctx)
			require.NoError(t, err, "create user_allowed_groups row")
		}
	}

	return u
}

func mustCreateGroup(t *testing.T, client *dbent.Client, g *service.Group) *service.Group {
	t.Helper()
	ctx := context.Background()

	if g.Platform == "" {
		g.Platform = service.PlatformAnthropic
	}
	if g.Status == "" {
		g.Status = service.StatusActive
	}
	if g.SubscriptionType == "" {
		g.SubscriptionType = service.SubscriptionTypeStandard
	}

	create := client.Group.Create().
		SetName(g.Name).
		SetPlatform(g.Platform).
		SetStatus(g.Status).
		SetSubscriptionType(g.SubscriptionType).
		SetRateMultiplier(g.RateMultiplier).
		SetIsExclusive(g.IsExclusive)
	if g.Description != "" {
		create.SetDescription(g.Description)
	}
	if g.DailyLimitUSD != nil {
		create.SetDailyLimitUsd(*g.DailyLimitUSD)
	}
	if g.WeeklyLimitUSD != nil {
		create.SetWeeklyLimitUsd(*g.WeeklyLimitUSD)
	}
	if g.MonthlyLimitUSD != nil {
		create.SetMonthlyLimitUsd(*g.MonthlyLimitUSD)
	}
	if !g.CreatedAt.IsZero() {
		create.SetCreatedAt(g.CreatedAt)
	}
	if !g.UpdatedAt.IsZero() {
		create.SetUpdatedAt(g.UpdatedAt)
	}

	created, err := create.Save(ctx)
	require.NoError(t, err, "create group")

	g.ID = created.ID
	g.CreatedAt = created.CreatedAt
	g.UpdatedAt = created.UpdatedAt
	return g
}

func mustCreateProxy(t *testing.T, client *dbent.Client, p *service.Proxy) *service.Proxy {
	t.Helper()
	ctx := context.Background()

	if p.Protocol == "" {
		p.Protocol = "http"
	}
	if p.Host == "" {
		p.Host = "127.0.0.1"
	}
	if p.Port == 0 {
		p.Port = 8080
	}
	if p.Status == "" {
		p.Status = service.StatusActive
	}

	create := client.Proxy.Create().
		SetName(p.Name).
		SetProtocol(p.Protocol).
		SetHost(p.Host).
		SetPort(p.Port).
		SetStatus(p.Status)
	if p.Username != "" {
		create.SetUsername(p.Username)
	}
	if p.Password != "" {
		create.SetPassword(p.Password)
	}
	if !p.CreatedAt.IsZero() {
		create.SetCreatedAt(p.CreatedAt)
	}
	if !p.UpdatedAt.IsZero() {
		create.SetUpdatedAt(p.UpdatedAt)
	}

	created, err := create.Save(ctx)
	require.NoError(t, err, "create proxy")

	p.ID = created.ID
	p.CreatedAt = created.CreatedAt
	p.UpdatedAt = created.UpdatedAt
	return p
}

func mustCreateAccount(t *testing.T, client *dbent.Client, a *service.Account) *service.Account {
	t.Helper()
	ctx := context.Background()

	if a.Platform == "" {
		a.Platform = service.PlatformAnthropic
	}
	if a.Type == "" {
		a.Type = service.AccountTypeOAuth
	}
	if a.Status == "" {
		a.Status = service.StatusActive
	}
	if a.Concurrency == 0 {
		a.Concurrency = 3
	}
	if a.Priority == 0 {
		a.Priority = 50
	}
	if !a.Schedulable {
		a.Schedulable = true
	}
	if a.Credentials == nil {
		a.Credentials = map[string]any{}
	}
	if a.Extra == nil {
		a.Extra = map[string]any{}
	}

	create := client.Account.Create().
		SetName(a.Name).
		SetPlatform(a.Platform).
		SetType(a.Type).
		SetCredentials(a.Credentials).
		SetExtra(a.Extra).
		SetConcurrency(a.Concurrency).
		SetPriority(a.Priority).
		SetStatus(a.Status).
		SetSchedulable(a.Schedulable).
		SetErrorMessage(a.ErrorMessage)

	if a.ProxyID != nil {
		create.SetProxyID(*a.ProxyID)
	}
	if a.LastUsedAt != nil {
		create.SetLastUsedAt(*a.LastUsedAt)
	}
	if a.RateLimitedAt != nil {
		create.SetRateLimitedAt(*a.RateLimitedAt)
	}
	if a.RateLimitResetAt != nil {
		create.SetRateLimitResetAt(*a.RateLimitResetAt)
	}
	if a.OverloadUntil != nil {
		create.SetOverloadUntil(*a.OverloadUntil)
	}
	if a.SessionWindowStart != nil {
		create.SetSessionWindowStart(*a.SessionWindowStart)
	}
	if a.SessionWindowEnd != nil {
		create.SetSessionWindowEnd(*a.SessionWindowEnd)
	}
	if a.SessionWindowStatus != "" {
		create.SetSessionWindowStatus(a.SessionWindowStatus)
	}
	if !a.CreatedAt.IsZero() {
		create.SetCreatedAt(a.CreatedAt)
	}
	if !a.UpdatedAt.IsZero() {
		create.SetUpdatedAt(a.UpdatedAt)
	}

	created, err := create.Save(ctx)
	require.NoError(t, err, "create account")

	a.ID = created.ID
	a.CreatedAt = created.CreatedAt
	a.UpdatedAt = created.UpdatedAt
	return a
}

func mustCreateApiKey(t *testing.T, client *dbent.Client, k *service.APIKey) *service.APIKey {
	t.Helper()
	ctx := context.Background()

	if k.Status == "" {
		k.Status = service.StatusActive
	}
	if k.Key == "" {
		k.Key = "sk-" + time.Now().Format("150405.000000")
	}
	if k.Name == "" {
		k.Name = "default"
	}

	create := client.APIKey.Create().
		SetUserID(k.UserID).
		SetKey(k.Key).
		SetName(k.Name).
		SetStatus(k.Status)
	if k.Quota != 0 {
		create.SetQuota(k.Quota)
	}
	if k.QuotaUsed != 0 {
		create.SetQuotaUsed(k.QuotaUsed)
	}
	if k.RateLimit5h != 0 {
		create.SetRateLimit5h(k.RateLimit5h)
	}
	if k.RateLimit1d != 0 {
		create.SetRateLimit1d(k.RateLimit1d)
	}
	if k.RateLimit7d != 0 {
		create.SetRateLimit7d(k.RateLimit7d)
	}
	if k.Usage5h != 0 {
		create.SetUsage5h(k.Usage5h)
	}
	if k.Usage1d != 0 {
		create.SetUsage1d(k.Usage1d)
	}
	if k.Usage7d != 0 {
		create.SetUsage7d(k.Usage7d)
	}
	if k.Window5hStart != nil {
		create.SetWindow5hStart(*k.Window5hStart)
	}
	if k.Window1dStart != nil {
		create.SetWindow1dStart(*k.Window1dStart)
	}
	if k.Window7dStart != nil {
		create.SetWindow7dStart(*k.Window7dStart)
	}
	if k.ExpiresAt != nil {
		create.SetExpiresAt(*k.ExpiresAt)
	}
	if k.GroupID != nil {
		create.SetGroupID(*k.GroupID)
	}
	if !k.CreatedAt.IsZero() {
		create.SetCreatedAt(k.CreatedAt)
	}
	if !k.UpdatedAt.IsZero() {
		create.SetUpdatedAt(k.UpdatedAt)
	}

	created, err := create.Save(ctx)
	require.NoError(t, err, "create api key")

	k.ID = created.ID
	k.CreatedAt = created.CreatedAt
	k.UpdatedAt = created.UpdatedAt
	return k
}

func mustCreateRedeemCode(t *testing.T, client *dbent.Client, c *service.RedeemCode) *service.RedeemCode {
	t.Helper()
	ctx := context.Background()

	if c.Status == "" {
		c.Status = service.StatusUnused
	}
	if c.Type == "" {
		c.Type = service.RedeemTypeBalance
	}
	if c.Code == "" {
		c.Code = "rc-" + time.Now().Format("150405.000000")
	}

	create := client.RedeemCode.Create().
		SetCode(c.Code).
		SetType(c.Type).
		SetValue(c.Value).
		SetStatus(c.Status).
		SetNotes(c.Notes).
		SetValidityDays(c.ValidityDays)
	if c.UsedBy != nil {
		create.SetUsedBy(*c.UsedBy)
	}
	if c.UsedAt != nil {
		create.SetUsedAt(*c.UsedAt)
	}
	if c.GroupID != nil {
		create.SetGroupID(*c.GroupID)
	}
	if !c.CreatedAt.IsZero() {
		create.SetCreatedAt(c.CreatedAt)
	}

	created, err := create.Save(ctx)
	require.NoError(t, err, "create redeem code")

	c.ID = created.ID
	c.CreatedAt = created.CreatedAt
	return c
}

func mustCreateSubscription(t *testing.T, client *dbent.Client, s *service.UserSubscription) *service.UserSubscription {
	t.Helper()
	ctx := context.Background()

	if s.Status == "" {
		s.Status = service.SubscriptionStatusActive
	}
	now := time.Now()
	if s.StartsAt.IsZero() {
		s.StartsAt = now.Add(-1 * time.Hour)
	}
	if s.ExpiresAt.IsZero() {
		s.ExpiresAt = now.Add(24 * time.Hour)
	}
	if s.AssignedAt.IsZero() {
		s.AssignedAt = now
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}

	create := client.UserSubscription.Create().
		SetUserID(s.UserID).
		SetGroupID(s.GroupID).
		SetStartsAt(s.StartsAt).
		SetExpiresAt(s.ExpiresAt).
		SetStatus(s.Status).
		SetAssignedAt(s.AssignedAt).
		SetNotes(s.Notes).
		SetDailyUsageUsd(s.DailyUsageUSD).
		SetWeeklyUsageUsd(s.WeeklyUsageUSD).
		SetMonthlyUsageUsd(s.MonthlyUsageUSD)

	if s.AssignedBy != nil {
		create.SetAssignedBy(*s.AssignedBy)
	}
	if !s.CreatedAt.IsZero() {
		create.SetCreatedAt(s.CreatedAt)
	}
	if !s.UpdatedAt.IsZero() {
		create.SetUpdatedAt(s.UpdatedAt)
	}

	created, err := create.Save(ctx)
	require.NoError(t, err, "create user subscription")

	s.ID = created.ID
	s.CreatedAt = created.CreatedAt
	s.UpdatedAt = created.UpdatedAt
	return s
}

func mustBindAccountToGroup(t *testing.T, client *dbent.Client, accountID, groupID int64, priority int) {
	t.Helper()
	ctx := context.Background()

	_, err := client.AccountGroup.Create().
		SetAccountID(accountID).
		SetGroupID(groupID).
		SetPriority(priority).
		Save(ctx)
	require.NoError(t, err, "create account_group")
}
