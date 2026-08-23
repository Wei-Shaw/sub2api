//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type inviteLoginRefreshTokenCacheStub struct {
	tokens map[string]*RefreshTokenData
}

func newInviteLoginRefreshTokenCacheStub() *inviteLoginRefreshTokenCacheStub {
	return &inviteLoginRefreshTokenCacheStub{tokens: make(map[string]*RefreshTokenData)}
}

func (s *inviteLoginRefreshTokenCacheStub) StoreRefreshToken(_ context.Context, tokenHash string, data *RefreshTokenData, _ time.Duration) error {
	copy := *data
	s.tokens[tokenHash] = &copy
	return nil
}

func (s *inviteLoginRefreshTokenCacheStub) GetRefreshToken(_ context.Context, tokenHash string) (*RefreshTokenData, error) {
	data, ok := s.tokens[tokenHash]
	if !ok {
		return nil, ErrRefreshTokenNotFound
	}
	copy := *data
	return &copy, nil
}

func (s *inviteLoginRefreshTokenCacheStub) DeleteRefreshToken(_ context.Context, tokenHash string) error {
	delete(s.tokens, tokenHash)
	return nil
}

func (s *inviteLoginRefreshTokenCacheStub) DeleteUserRefreshTokens(context.Context, int64) error {
	return nil
}

func (s *inviteLoginRefreshTokenCacheStub) DeleteTokenFamily(context.Context, string) error {
	return nil
}

func (s *inviteLoginRefreshTokenCacheStub) AddToUserTokenSet(context.Context, int64, string, time.Duration) error {
	return nil
}

func (s *inviteLoginRefreshTokenCacheStub) AddToFamilyTokenSet(context.Context, string, string, time.Duration) error {
	return nil
}

func (s *inviteLoginRefreshTokenCacheStub) GetUserTokenHashes(context.Context, int64) ([]string, error) {
	return nil, nil
}

func (s *inviteLoginRefreshTokenCacheStub) GetFamilyTokenHashes(context.Context, string) ([]string, error) {
	return nil, nil
}

func (s *inviteLoginRefreshTokenCacheStub) IsTokenInFamily(context.Context, string, string) (bool, error) {
	return false, nil
}

func TestInviteLoginDirectDeviceCodeSuccessIssuesTokensAndUpdatesLastLogin(t *testing.T) {
	probe := &inviteLoginDeviceRepoProbe{device: &UserDevice{ID: 17, UserID: 23, Status: UserDeviceStatusActive}}
	userRepo := &userRepoStub{user: &User{ID: 23, Email: "dlg@example.com", Status: StatusActive}}
	cache := newInviteLoginRefreshTokenCacheStub()
	cfg := &config.Config{}
	cfg.JWT.Secret = "test-secret-that-is-long-enough-for-signing"
	cfg.JWT.ExpireHour = 1
	cfg.JWT.RefreshTokenExpireDays = 30

	svc := NewAuthService(nil, userRepo, nil, cache, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	svc.SetUserDeviceRepository(probe)

	result, err := svc.InviteLogin(context.Background(), InviteLoginInput{
		InvitationCode: " dlg-abcd-efgh-jklm ",
		ClientKind:     "web",
	})
	if err != nil {
		t.Fatalf("InviteLogin() error = %v", err)
	}
	if result == nil || result.TokenPair == nil {
		t.Fatalf("InviteLogin() result = %#v, want token pair", result)
	}
	if result.TokenPair.AccessToken == "" || result.TokenPair.RefreshToken == "" {
		t.Fatalf("InviteLogin() token pair = %#v, want non-empty tokens", result.TokenPair)
	}
	if len(result.BootstrapAPIKeys) != 0 {
		t.Fatalf("direct DLG bootstrap keys = %#v, want none", result.BootstrapAPIKeys)
	}
	if probe.lastLoginCalls != 1 || probe.lastLoginID != 17 {
		t.Fatalf("UpdateLastLoginAt calls = %d for device %d, want one call for device 17", probe.lastLoginCalls, probe.lastLoginID)
	}
}
