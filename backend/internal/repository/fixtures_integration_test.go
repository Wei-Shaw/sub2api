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
REDACTED
	if u.PasswordHash == "" {
		u.PasswordHash = "test-password-hash"
REDACTED
	if u.Role == "" {
		u.Role = model.RoleUser
REDACTED
	if u.Status == "" {
		u.Status = model.StatusActive
REDACTED
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
REDACTED
	if u.UpdatedAt.IsZero() {
		u.UpdatedAt = u.CreatedAt
REDACTED
	require.NoError(t, db.Create(u).Error, "create user")
	return u
REDACTED

func mustCreateGroup(t *testing.T, db *gorm.DB, g *model.Group) *model.Group {
REDACTED
	if g.Platform == "" {
		g.Platform = model.PlatformAnthropic
REDACTED
	if g.Status == "" {
		g.Status = model.StatusActive
REDACTED
	if g.SubscriptionType == "" {
		g.SubscriptionType = model.SubscriptionTypeStandard
REDACTED
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now()
REDACTED
	if g.UpdatedAt.IsZero() {
		g.UpdatedAt = g.CreatedAt
REDACTED
	require.NoError(t, db.Create(g).Error, "create group")
	return g
REDACTED

func mustCreateProxy(t *testing.T, db *gorm.DB, p *model.Proxy) *model.Proxy {
REDACTED
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
		p.Status = model.StatusActive
REDACTED
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
REDACTED
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = p.CreatedAt
REDACTED
	require.NoError(t, db.Create(p).Error, "create proxy")
	return p
REDACTED

func mustCreateAccount(t *testing.T, db *gorm.DB, a *model.Account) *model.Account {
REDACTED
	if a.Platform == "" {
		a.Platform = model.PlatformAnthropic
REDACTED
	if a.Type == "" {
		a.Type = model.AccountTypeOAuth
REDACTED
	if a.Status == "" {
		a.Status = model.StatusActive
REDACTED
	if !a.Schedulable {
		a.Schedulable = true
REDACTED
	if a.Credentials == nil {
		a.Credentials = model.JSONB{REDACTED
REDACTED
	if a.Extra == nil {
		a.Extra = model.JSONB{REDACTED
REDACTED
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
REDACTED
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = a.CreatedAt
REDACTED
	require.NoError(t, db.Create(a).Error, "create account")
	return a
REDACTED

func mustCreateApiKey(t *testing.T, db *gorm.DB, k *model.ApiKey) *model.ApiKey {
REDACTED
	if k.Status == "" {
		k.Status = model.StatusActive
REDACTED
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now()
REDACTED
	if k.UpdatedAt.IsZero() {
		k.UpdatedAt = k.CreatedAt
REDACTED
	require.NoError(t, db.Create(k).Error, "create api key")
	return k
REDACTED

func mustCreateRedeemCode(t *testing.T, db *gorm.DB, c *model.RedeemCode) *model.RedeemCode {
REDACTED
	if c.Status == "" {
		c.Status = model.StatusUnused
REDACTED
	if c.Type == "" {
		c.Type = model.RedeemTypeBalance
REDACTED
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
REDACTED
	require.NoError(t, db.Create(c).Error, "create redeem code")
	return c
REDACTED

func mustCreateSubscription(t *testing.T, db *gorm.DB, s *model.UserSubscription) *model.UserSubscription {
REDACTED
	if s.Status == "" {
		s.Status = model.SubscriptionStatusActive
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
	require.NoError(t, db.Create(s).Error, "create user subscription")
	return s
REDACTED

func mustBindAccountToGroup(t *testing.T, db *gorm.DB, accountID, groupID int64, priority int) {
REDACTED
	require.NoError(t, db.Create(&model.AccountGroup{
		AccountID: accountID,
		GroupID:   groupID,
		Priority:  priority,
REDACTED).Error, "create account_group")
REDACTED
