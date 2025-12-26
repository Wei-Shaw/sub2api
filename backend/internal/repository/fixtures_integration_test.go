//go:build integration

package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func mustCreateUser(t *testing.T, db *gorm.DB, u *userModel) *userModel {
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
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
REDACTED
	if u.UpdatedAt.IsZero() {
		u.UpdatedAt = u.CreatedAt
REDACTED
	require.NoError(t, db.Create(u).Error, "create user")
	return u
REDACTED

func mustCreateGroup(t *testing.T, db *gorm.DB, g *groupModel) *groupModel {
REDACTED
	if g.Platform == "" {
		g.Platform = service.PlatformAnthropic
REDACTED
	if g.Status == "" {
		g.Status = service.StatusActive
REDACTED
	if g.SubscriptionType == "" {
		g.SubscriptionType = service.SubscriptionTypeStandard
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

func mustCreateProxy(t *testing.T, db *gorm.DB, p *proxyModel) *proxyModel {
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
		p.Status = service.StatusActive
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

func mustCreateAccount(t *testing.T, db *gorm.DB, a *accountModel) *accountModel {
REDACTED
	if a.Platform == "" {
		a.Platform = service.PlatformAnthropic
REDACTED
	if a.Type == "" {
		a.Type = service.AccountTypeOAuth
REDACTED
	if a.Status == "" {
		a.Status = service.StatusActive
REDACTED
	if !a.Schedulable {
		a.Schedulable = true
REDACTED
	if a.Credentials == nil {
		a.Credentials = datatypes.JSONMap{REDACTED
REDACTED
	if a.Extra == nil {
		a.Extra = datatypes.JSONMap{REDACTED
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

func mustCreateApiKey(t *testing.T, db *gorm.DB, k *apiKeyModel) *apiKeyModel {
REDACTED
	if k.Status == "" {
		k.Status = service.StatusActive
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

func mustCreateRedeemCode(t *testing.T, db *gorm.DB, c *redeemCodeModel) *redeemCodeModel {
REDACTED
	if c.Status == "" {
		c.Status = service.StatusUnused
REDACTED
	if c.Type == "" {
		c.Type = service.RedeemTypeBalance
REDACTED
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
REDACTED
	require.NoError(t, db.Create(c).Error, "create redeem code")
	return c
REDACTED

func mustCreateSubscription(t *testing.T, db *gorm.DB, s *userSubscriptionModel) *userSubscriptionModel {
REDACTED
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
	require.NoError(t, db.Create(s).Error, "create user subscription")
	return s
REDACTED

func mustBindAccountToGroup(t *testing.T, db *gorm.DB, accountID, groupID int64, priority int) {
REDACTED
	require.NoError(t, db.Create(&accountGroupModel{
		AccountID: accountID,
		GroupID:   groupID,
		Priority:  priority,
		CreatedAt: time.Now(),
REDACTED).Error, "create account_group")
REDACTED
