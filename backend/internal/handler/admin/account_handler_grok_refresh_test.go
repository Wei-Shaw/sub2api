//go:build unit

package admin

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type grokRefreshOAuthStub struct {
	account *service.Account
	info    *service.GrokTokenInfo
	calls   int
REDACTED

func (s *grokRefreshOAuthStub) RefreshAccountToken(_ context.Context, account *service.Account) (*service.GrokTokenInfo, error) {
	s.calls++
	s.account = account
	return s.info, nil
REDACTED

func (s *grokRefreshOAuthStub) BuildAccountCredentials(info *service.GrokTokenInfo) map[string]any {
	return map[string]any{
		"access_token":  info.AccessToken,
		"refresh_token": info.RefreshToken,
		"expires_at":    info.ExpiresAt,
		"base_url":      "https://api.x.ai/v1",
REDACTED
REDACTED

type grokRefreshAdminService struct {
	*stubAdminService
	updatedCredentials map[string]any
REDACTED

func (s *grokRefreshAdminService) UpdateAccount(_ context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	s.updatedCredentials = input.Credentials
	return &service.Account{
		ID:          id,
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Credentials: input.Credentials,
REDACTED, nil
REDACTED

func TestRefreshSingleAccountRoutesGrokThroughGrokOAuthService(t *testing.T) {
	t.Parallel()

	adminSvc := &grokRefreshAdminService{stubAdminService: newStubAdminService()REDACTED
	grokOAuth := &grokRefreshOAuthStub{info: &service.GrokTokenInfo{
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		ExpiresAt:    1_800_000_000,
REDACTEDREDACTED
	handler := NewAccountHandler(
		adminSvc,
		nil,
		nil,
		nil,
		nil,
		grokOAuth,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	account := &service.Account{
		ID:       4227,
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
REDACTED
			"access_token":       "old-access",
			"refresh_token":      "old-refresh",
			"base_url":           "https://example.invalid/v1",
			"subscription_tier":  "SUPER_GROK",
			"entitlement_status": "ACTIVE",
	REDACTED,
REDACTED

	updated, warning, err := handler.refreshSingleAccount(context.Background(), account)
REDACTED
	require.Empty(t, warning)
	require.Equal(t, 1, grokOAuth.calls)
	require.Same(t, account, grokOAuth.account)
	require.Equal(t, "new-access", adminSvc.updatedCredentials["access_token"])
	require.Equal(t, "new-refresh", adminSvc.updatedCredentials["refresh_token"])
	require.Equal(t, "https://example.invalid/v1", adminSvc.updatedCredentials["base_url"])
	require.Equal(t, "SUPER_GROK", adminSvc.updatedCredentials["subscription_tier"])
	require.Equal(t, "ACTIVE", adminSvc.updatedCredentials["entitlement_status"])
	require.Equal(t, adminSvc.updatedCredentials, updated.Credentials)
REDACTED
