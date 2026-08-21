package service

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type InviteLoginInput struct {
	InvitationCode string
	DeviceHash     string
	InstallID      string
	ClientKind     string
}

type InviteLoginResult struct {
	TokenPair        *TokenPair
	User             *User
	BootstrapAPIKeys []InviteBootstrapAPIKey
}

func (s *AuthService) SetInviteLoginDeviceResolver(repo UserDeviceRepository) {
	if s != nil {
		s.inviteLoginDeviceRepo = repo
	}
}

func (s *AuthService) SetUserDeviceRepository(repo UserDeviceRepository) {
	s.SetInviteLoginDeviceResolver(repo)
}

// InviteLogin handles both device-bound login codes and regular invite bootstrap login.
func (s *AuthService) InviteLogin(ctx context.Context, input InviteLoginInput) (*InviteLoginResult, error) {
	code := strings.TrimSpace(input.InvitationCode)
	if code == "" {
		return nil, ErrInvitationCodeRequired
	}
	if s == nil || s.redeemRepo == nil {
		return nil, ErrServiceUnavailable
	}

	redeemCode, err := s.redeemRepo.GetByCode(ctx, code)
	if err != nil || redeemCode == nil {
		return nil, ErrInvitationCodeInvalid
	}
	if redeemCode.Type == RedeemTypeDeviceLogin {
		return s.inviteLoginWithDeviceCode(ctx, redeemCode, input)
	}
	if redeemCode.Status != StatusUnused || !isInviteLoginBootstrapRedeemType(redeemCode.Type) {
		return nil, ErrInvitationCodeInvalid
	}

	return s.completeInviteBootstrapLogin(ctx, redeemCode)
}

// RedeemLogin handles the web redeem login flow. Device login codes are intentionally
// rejected here because they require a device fingerprint.
func (s *AuthService) RedeemLogin(ctx context.Context, invitationCode string) (*InviteLoginResult, error) {
	code := strings.TrimSpace(invitationCode)
	if code == "" {
		return nil, ErrInvitationCodeRequired
	}
	if s == nil || s.redeemRepo == nil {
		return nil, ErrServiceUnavailable
	}

	redeemCode, err := s.redeemRepo.GetByCode(ctx, code)
	if err != nil || redeemCode == nil || redeemCode.Type == RedeemTypeDeviceLogin || redeemCode.Status != StatusUnused || !isInviteLoginBootstrapRedeemType(redeemCode.Type) {
		return nil, ErrInvitationCodeInvalid
	}

	return s.completeInviteBootstrapLogin(ctx, redeemCode)
}

func (s *AuthService) createInviteBootstrapUser(ctx context.Context, invitationRedeemCode *RedeemCode) (*User, error) {
	return createInviteBootstrapUserWithRedeem(ctx, s.entClient, s.userRepo, s.redeemRepo, s.cfg, s.settingService, invitationRedeemCode)
}

func (s *AuthService) inviteLoginWithDeviceCode(ctx context.Context, redeemCode *RedeemCode, input InviteLoginInput) (*InviteLoginResult, error) {
	if redeemCode == nil || redeemCode.Type != RedeemTypeDeviceLogin {
		return nil, ErrInvitationCodeInvalid
	}
	if s.inviteLoginDeviceRepo == nil {
		return nil, ErrServiceUnavailable
	}
	if err := validateInviteLoginDeviceInput(input); err != nil {
		return nil, err
	}

	binding, err := s.inviteLoginDeviceRepo.GetByLoginRedeemCodeID(ctx, redeemCode.ID)
	if err != nil || binding == nil {
		return nil, ErrInvitationCodeInvalid
	}
	if !binding.IsActive() {
		return nil, ErrDeviceRevoked
	}
	if normalizeDeviceHash(binding.DeviceHash) != normalizeDeviceHash(input.DeviceHash) {
		return nil, ErrDeviceMismatch
	}
	user, err := s.userRepo.GetByID(ctx, binding.UserID)
	if err != nil || user == nil {
		return nil, ErrServiceUnavailable
	}
	if err := ensureInviteLoginUserActive(user); err != nil {
		return nil, err
	}

	bootstrapKeys, err := s.provisionInviteBootstrapAPIKeys(ctx, user.ID, redeemCode)
	if err != nil {
		return nil, err
	}
	if err := s.inviteLoginDeviceRepo.UpdateLastLoginAt(ctx, binding.ID, time.Now().UTC()); err != nil {
		return nil, ErrServiceUnavailable
	}
	tokenPair, err := s.GenerateTokenPair(ctx, user, "")
	if err != nil {
		return nil, fmt.Errorf("generate token pair: %w", err)
	}
	return &InviteLoginResult{TokenPair: tokenPair, User: user, BootstrapAPIKeys: bootstrapKeys}, nil
}

func (s *AuthService) completeInviteBootstrapLogin(ctx context.Context, redeemCode *RedeemCode) (*InviteLoginResult, error) {
	user, err := s.createInviteBootstrapUser(ctx, redeemCode)
	if err != nil {
		return nil, err
	}
	if err := s.applyInviteLoginEntitlement(ctx, user.ID, redeemCode); err != nil {
		return nil, err
	}
	if updatedUser, err := s.userRepo.GetByID(ctx, user.ID); err == nil {
		user = updatedUser
	}

	bootstrapKeys, err := s.provisionInviteBootstrapAPIKeys(ctx, user.ID, redeemCode)
	if err != nil {
		return nil, err
	}
	tokenPair, err := s.GenerateTokenPair(ctx, user, "")
	if err != nil {
		return nil, fmt.Errorf("generate token pair: %w", err)
	}
	return &InviteLoginResult{TokenPair: tokenPair, User: user, BootstrapAPIKeys: bootstrapKeys}, nil
}

func validateInviteLoginDeviceInput(input InviteLoginInput) error {
	deviceHash := normalizeDeviceHash(input.DeviceHash)
	if deviceHash == "" {
		return ErrDeviceHashRequired
	}
	if len(deviceHash) != 64 {
		return ErrDeviceHashInvalid
	}
	for _, ch := range deviceHash {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return ErrDeviceHashInvalid
		}
	}
	return nil
}

func ensureInviteLoginUserActive(user *User) error {
	if user == nil {
		return ErrInvitationCodeInvalid
	}
	if !user.IsActive() {
		return ErrUserNotActive
	}
	return nil
}
