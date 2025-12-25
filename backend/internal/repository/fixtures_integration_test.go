//go:build integration

package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func mustCreateUser(t *testing.T, db *gorm.DB, u *model.User) *model.User {
	t.Helper()
	if u.PasswordHash == "" {
		u.PasswordHash = "test-password-hash"
	}
	if u.Role == "" {
		u.Role = model.RoleUser
	}
	if u.Status == "" {
		u.Status = model.StatusActive
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	if u.UpdatedAt.IsZero() {
		u.UpdatedAt = u.CreatedAt
	}
	require.NoError(t, db.Create(u).Error, "create user")
	return u
}

func mustCreateGroup(t *testing.T, db *gorm.DB, g *model.Group) *model.Group {
	t.Helper()
	if g.Platform == "" {
		g.Platform = model.PlatformAnthropic
	}
	if g.Status == "" {
		g.Status = model.StatusActive
	}
	if g.SubscriptionType == "" {
		g.SubscriptionType = model.SubscriptionTypeStandard
	}
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now()
	}
	if g.UpdatedAt.IsZero() {
		g.UpdatedAt = g.CreatedAt
	}
	require.NoError(t, db.Create(g).Error, "create group")
	return g
}

func mustCreateProxy(t *testing.T, db *gorm.DB, p *model.Proxy) *model.Proxy {
	t.Helper()
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
		p.Status = model.StatusActive
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = p.CreatedAt
	}
	require.NoError(t, db.Create(p).Error, "create proxy")
	return p
}

func mustCreateAccount(t *testing.T, db *gorm.DB, a *model.Account) *model.Account {
	t.Helper()
	if a.Platform == "" {
		a.Platform = model.PlatformAnthropic
	}
	if a.Type == "" {
		a.Type = model.AccountTypeOAuth
	}
	if a.Status == "" {
		a.Status = model.StatusActive
	}
	if !a.Schedulable {
		a.Schedulable = true
	}
	if a.Credentials == nil {
		a.Credentials = model.JSONB{}
	}
	if a.Extra == nil {
		a.Extra = model.JSONB{}
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = a.CreatedAt
	}
	require.NoError(t, db.Create(a).Error, "create account")
	return a
}

func mustCreateApiKey(t *testing.T, db *gorm.DB, k *model.ApiKey) *model.ApiKey {
	t.Helper()
	if k.Status == "" {
		k.Status = model.StatusActive
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now()
	}
	if k.UpdatedAt.IsZero() {
		k.UpdatedAt = k.CreatedAt
	}
	require.NoError(t, db.Create(k).Error, "create api key")
	return k
}

func mustCreateRedeemCode(t *testing.T, db *gorm.DB, c *model.RedeemCode) *model.RedeemCode {
	t.Helper()
	if c.Status == "" {
		c.Status = model.StatusUnused
	}
	if c.Type == "" {
		c.Type = model.RedeemTypeBalance
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	require.NoError(t, db.Create(c).Error, "create redeem code")
	return c
}

func mustCreateSubscription(t *testing.T, db *gorm.DB, s *model.UserSubscription) *model.UserSubscription {
	t.Helper()
	if s.Status == "" {
		s.Status = model.SubscriptionStatusActive
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
	require.NoError(t, db.Create(s).Error, "create user subscription")
	return s
}

func mustBindAccountToGroup(t *testing.T, db *gorm.DB, accountID, groupID int64, priority int) {
	t.Helper()
	require.NoError(t, db.Create(&model.AccountGroup{
		AccountID: accountID,
		GroupID:   groupID,
		Priority:  priority,
	}).Error, "create account_group")
}
