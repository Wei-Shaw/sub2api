//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func mustCreateUser(t *testing.T, client *dbent.Client, u *service.User) *service.User {
REDACTED
	ctx := context.Background()

	if u.Email == "" {
		u.Email = "user-" + time.Now().Format(time.RFC3339Nano) + "@example.com"
REDACTED
	if u.PasswordHash == "" {
		u.PasswordHash = "test-password-hash"
REDACTED
	if u.Role == "" {
		u.Role = service.RoleUser
REDACTED
	if u.Status == "" {
		u.Status = service.StatusActive
REDACTED
	if u.Concurrency == 0 {
		u.Concurrency = 5
REDACTED

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
REDACTED
	if !u.UpdatedAt.IsZero() {
		create.SetUpdatedAt(u.UpdatedAt)
REDACTED

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
	REDACTED
REDACTED

	return u
REDACTED

func mustCreateGroup(t *testing.T, client *dbent.Client, g *service.Group) *service.Group {
REDACTED
	ctx := context.Background()

	if g.Platform == "" {
		g.Platform = service.PlatformAnthropic
REDACTED
	if g.Status == "" {
		g.Status = service.StatusActive
REDACTED
	if g.SubscriptionType == "" {
		g.SubscriptionType = service.SubscriptionTypeStandard
REDACTED

	create := client.Group.Create().
		SetName(g.Name).
		SetPlatform(g.Platform).
		SetStatus(g.Status).
		SetSubscriptionType(g.SubscriptionType).
		SetRateMultiplier(g.RateMultiplier).
		SetIsExclusive(g.IsExclusive)
	if g.Description != "" {
		create.SetDescription(g.Description)
REDACTED
	if g.DailyLimitUSD != nil {
		create.SetDailyLimitUsd(*g.DailyLimitUSD)
REDACTED
	if g.WeeklyLimitUSD != nil {
		create.SetWeeklyLimitUsd(*g.WeeklyLimitUSD)
REDACTED
	if g.MonthlyLimitUSD != nil {
		create.SetMonthlyLimitUsd(*g.MonthlyLimitUSD)
REDACTED
	if !g.CreatedAt.IsZero() {
		create.SetCreatedAt(g.CreatedAt)
REDACTED
	if !g.UpdatedAt.IsZero() {
		create.SetUpdatedAt(g.UpdatedAt)
REDACTED

	created, err := create.Save(ctx)
	require.NoError(t, err, "create group")

	g.ID = created.ID
	g.CreatedAt = created.CreatedAt
	g.UpdatedAt = created.UpdatedAt
	return g
REDACTED

func mustCreateProxy(t *testing.T, client *dbent.Client, p *service.Proxy) *service.Proxy {
REDACTED
	ctx := context.Background()

	if p.Protocol == "" {
		p.Protocol = "http"
REDACTED
	if p.Host == "" {
		p.Host = "127.0.0.1"
REDACTED
	if p.Port == 0 {
		p.Port = 8080
REDACTED
	if p.Status == "" {
		p.Status = service.StatusActive
REDACTED

	create := client.Proxy.Create().
		SetName(p.Name).
		SetProtocol(p.Protocol).
		SetHost(p.Host).
		SetPort(p.Port).
		SetStatus(p.Status)
	if p.Username != "" {
		create.SetUsername(p.Username)
REDACTED
	if p.Password != "" {
		create.SetPassword(p.Password)
REDACTED
	if !p.CreatedAt.IsZero() {
		create.SetCreatedAt(p.CreatedAt)
REDACTED
	if !p.UpdatedAt.IsZero() {
		create.SetUpdatedAt(p.UpdatedAt)
REDACTED

	created, err := create.Save(ctx)
	require.NoError(t, err, "create proxy")

	p.ID = created.ID
	p.CreatedAt = created.CreatedAt
	p.UpdatedAt = created.UpdatedAt
	return p
REDACTED

func mustCreateAccount(t *testing.T, client *dbent.Client, a *service.Account) *service.Account {
REDACTED
	ctx := context.Background()

	if a.Platform == "" {
		a.Platform = service.PlatformAnthropic
REDACTED
	if a.Type == "" {
		a.Type = service.AccountTypeOAuth
REDACTED
	if a.Status == "" {
		a.Status = service.StatusActive
REDACTED
	if a.Concurrency == 0 {
		a.Concurrency = 3
REDACTED
	if a.Priority == 0 {
		a.Priority = 50
REDACTED
	if !a.Schedulable {
		a.Schedulable = true
REDACTED
	if a.Credentials == nil {
		a.Credentials = map[string]any{REDACTED
REDACTED
	if a.Extra == nil {
		a.Extra = map[string]any{REDACTED
REDACTED

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
REDACTED
	if a.LastUsedAt != nil {
		create.SetLastUsedAt(*a.LastUsedAt)
REDACTED
	if a.RateLimitedAt != nil {
		create.SetRateLimitedAt(*a.RateLimitedAt)
REDACTED
	if a.RateLimitResetAt != nil {
		create.SetRateLimitResetAt(*a.RateLimitResetAt)
REDACTED
	if a.OverloadUntil != nil {
		create.SetOverloadUntil(*a.OverloadUntil)
REDACTED
	if a.SessionWindowStart != nil {
		create.SetSessionWindowStart(*a.SessionWindowStart)
REDACTED
	if a.SessionWindowEnd != nil {
		create.SetSessionWindowEnd(*a.SessionWindowEnd)
REDACTED
	if a.SessionWindowStatus != "" {
		create.SetSessionWindowStatus(a.SessionWindowStatus)
REDACTED
	if !a.CreatedAt.IsZero() {
		create.SetCreatedAt(a.CreatedAt)
REDACTED
	if !a.UpdatedAt.IsZero() {
		create.SetUpdatedAt(a.UpdatedAt)
REDACTED
	if a.ParentAccountID != nil {
		create.SetParentAccountID(*a.ParentAccountID)
REDACTED
	if a.QuotaDimension != "" {
		create.SetQuotaDimension(dbaccount.QuotaDimension(a.QuotaDimension))
REDACTED

	created, err := create.Save(ctx)
	require.NoError(t, err, "create account")

	a.ID = created.ID
	a.CreatedAt = created.CreatedAt
	a.UpdatedAt = created.UpdatedAt
	return a
REDACTED

func mustCreateApiKey(t *testing.T, client *dbent.Client, k *service.APIKey) *service.APIKey {
REDACTED
	ctx := context.Background()

	if k.Status == "" {
		k.Status = service.StatusActive
REDACTED
	if k.Key == "" {
		k.Key = "sk-" + time.Now().Format("150405.000000")
REDACTED
	if k.Name == "" {
		k.Name = "default"
REDACTED

	create := client.APIKey.Create().
		SetUserID(k.UserID).
		SetKey(k.Key).
		SetName(k.Name).
		SetStatus(k.Status)
	if k.Quota != 0 {
		create.SetQuota(k.Quota)
REDACTED
	if k.QuotaUsed != 0 {
		create.SetQuotaUsed(k.QuotaUsed)
REDACTED
	if k.RateLimit5h != 0 {
		create.SetRateLimit5h(k.RateLimit5h)
REDACTED
	if k.RateLimit1d != 0 {
		create.SetRateLimit1d(k.RateLimit1d)
REDACTED
	if k.RateLimit7d != 0 {
		create.SetRateLimit7d(k.RateLimit7d)
REDACTED
	if k.Usage5h != 0 {
		create.SetUsage5h(k.Usage5h)
REDACTED
	if k.Usage1d != 0 {
		create.SetUsage1d(k.Usage1d)
REDACTED
	if k.Usage7d != 0 {
		create.SetUsage7d(k.Usage7d)
REDACTED
	if k.Window5hStart != nil {
		create.SetWindow5hStart(*k.Window5hStart)
REDACTED
	if k.Window1dStart != nil {
		create.SetWindow1dStart(*k.Window1dStart)
REDACTED
	if k.Window7dStart != nil {
		create.SetWindow7dStart(*k.Window7dStart)
REDACTED
	if k.ExpiresAt != nil {
		create.SetExpiresAt(*k.ExpiresAt)
REDACTED
	if k.GroupID != nil {
		create.SetGroupID(*k.GroupID)
REDACTED
	if !k.CreatedAt.IsZero() {
		create.SetCreatedAt(k.CreatedAt)
REDACTED
	if !k.UpdatedAt.IsZero() {
		create.SetUpdatedAt(k.UpdatedAt)
REDACTED

	created, err := create.Save(ctx)
	require.NoError(t, err, "create api key")

	k.ID = created.ID
	k.CreatedAt = created.CreatedAt
	k.UpdatedAt = created.UpdatedAt
	return k
REDACTED

func mustCreateRedeemCode(t *testing.T, client *dbent.Client, c *service.RedeemCode) *service.RedeemCode {
REDACTED
	ctx := context.Background()

	if c.Status == "" {
		c.Status = service.StatusUnused
REDACTED
	if c.Type == "" {
		c.Type = service.RedeemTypeBalance
REDACTED
	if c.Code == "" {
		c.Code = "rc-" + time.Now().Format("150405.000000")
REDACTED

	create := client.RedeemCode.Create().
		SetCode(c.Code).
		SetType(c.Type).
		SetValue(c.Value).
		SetStatus(c.Status).
		SetNotes(c.Notes).
		SetValidityDays(c.ValidityDays)
	if c.UsedBy != nil {
		create.SetUsedBy(*c.UsedBy)
REDACTED
	if c.UsedAt != nil {
		create.SetUsedAt(*c.UsedAt)
REDACTED
	if c.GroupID != nil {
		create.SetGroupID(*c.GroupID)
REDACTED
	if !c.CreatedAt.IsZero() {
		create.SetCreatedAt(c.CreatedAt)
REDACTED

	created, err := create.Save(ctx)
	require.NoError(t, err, "create redeem code")

	c.ID = created.ID
	c.CreatedAt = created.CreatedAt
	return c
REDACTED

func mustCreateSubscription(t *testing.T, client *dbent.Client, s *service.UserSubscription) *service.UserSubscription {
REDACTED
	ctx := context.Background()

	if s.Status == "" {
		s.Status = service.SubscriptionStatusActive
REDACTED
	now := time.Now()
	if s.StartsAt.IsZero() {
		s.StartsAt = now.Add(-1 * time.Hour)
REDACTED
	if s.ExpiresAt.IsZero() {
		s.ExpiresAt = now.Add(24 * time.Hour)
REDACTED
	if s.AssignedAt.IsZero() {
		s.AssignedAt = now
REDACTED
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
REDACTED
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
REDACTED

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
REDACTED
	if !s.CreatedAt.IsZero() {
		create.SetCreatedAt(s.CreatedAt)
REDACTED
	if !s.UpdatedAt.IsZero() {
		create.SetUpdatedAt(s.UpdatedAt)
REDACTED

	created, err := create.Save(ctx)
	require.NoError(t, err, "create user subscription")

	s.ID = created.ID
	s.CreatedAt = created.CreatedAt
	s.UpdatedAt = created.UpdatedAt
	return s
REDACTED

func mustBindAccountToGroup(t *testing.T, client *dbent.Client, accountID, groupID int64, priority int) {
REDACTED
	ctx := context.Background()

	_, err := client.AccountGroup.Create().
		SetAccountID(accountID).
		SetGroupID(groupID).
		SetPriority(priority).
		Save(ctx)
	require.NoError(t, err, "create account_group")
REDACTED
