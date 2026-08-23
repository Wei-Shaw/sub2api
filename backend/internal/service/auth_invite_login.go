package service

import (
	"context"
	"errors"
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
	if isDeviceLoginCode(code) {
		code = strings.ToUpper(code)
		binding, err := s.resolveDirectInviteDeviceCode(ctx, code)
		if err != nil {
			return nil, err
		}
		return s.inviteLoginWithDirectDeviceCode(ctx, binding, code, input)
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
	deviceHash := normalizeDeviceHash(input.DeviceHash)
	if normalizeDeviceHash(binding.DeviceHash) != deviceHash {
		return nil, ErrDeviceMismatch
	}
	if err := validateInviteLoginInstallID(binding, input.InstallID); err != nil {
		return nil, err
	}
	return s.completeInviteDeviceLogin(ctx, binding, redeemCode, true)
}

func (s *AuthService) resolveDirectInviteDeviceCode(ctx context.Context, code string) (*UserDevice, error) {
	if s == nil || s.inviteLoginDeviceRepo == nil {
		return nil, ErrServiceUnavailable
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if !isDeviceLoginCode(code) {
		return nil, ErrInvitationCodeInvalid
	}
	binding, err := s.inviteLoginDeviceRepo.GetByDeviceCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrUserDeviceNotFound) {
			return nil, ErrInvitationCodeInvalid
		}
		return nil, ErrServiceUnavailable
	}
	if binding == nil {
		return nil, ErrInvitationCodeInvalid
	}
	return binding, nil
}

func (s *AuthService) inviteLoginWithDirectDeviceCode(ctx context.Context, binding *UserDevice, code string, input InviteLoginInput) (*InviteLoginResult, error) {
	if binding == nil || !binding.IsActive() {
		return nil, ErrDeviceRevoked
	}
	deviceHash, err := validateInviteLoginDeviceInputForClient(input)
	if err != nil {
		return nil, err
	}
	if deviceHash != "" && normalizeDeviceHash(binding.DeviceHash) != deviceHash {
		return nil, ErrDeviceMismatch
	}
	if err := validateInviteLoginInstallID(binding, input.InstallID); err != nil {
		return nil, err
	}
	return s.completeInviteDeviceLogin(ctx, binding, &RedeemCode{
		Code:   code,
		Type:   RedeemTypeDeviceLogin,
		Status: StatusUnused,
	}, false)
}

func (s *AuthService) completeInviteDeviceLogin(ctx context.Context, binding *UserDevice, redeemCode *RedeemCode, provisionBootstrap bool) (*InviteLoginResult, error) {
	if s == nil || s.userRepo == nil || binding == nil {
		return nil, ErrServiceUnavailable
	}
	if !binding.IsActive() {
		return nil, ErrDeviceRevoked
	}
	user, err := s.userRepo.GetByID(ctx, binding.UserID)
	if err != nil || user == nil {
		return nil, ErrServiceUnavailable
	}
	if err := ensureInviteLoginUserActive(user); err != nil {
		return nil, err
	}

	var bootstrapKeys []InviteBootstrapAPIKey
	if provisionBootstrap {
		bootstrapKeys, err = s.provisionInviteBootstrapAPIKeys(ctx, user.ID, redeemCode)
		if err != nil {
			return nil, err
		}
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
	_, err := validateInviteLoginDeviceHash(input.DeviceHash, false)
	return err
}

func validateInviteLoginDeviceInputForClient(input InviteLoginInput) (string, error) {
	return validateInviteLoginDeviceHash(input.DeviceHash, isWebInviteLogin(input))
}

func validateInviteLoginDeviceHash(value string, allowMissing bool) (string, error) {
	deviceHash := normalizeDeviceHash(value)
	if deviceHash == "" {
		if allowMissing {
			return "", nil
		}
		return "", ErrDeviceHashRequired
	}
	if len(deviceHash) != 64 {
		return "", ErrDeviceHashInvalid
	}
	for _, ch := range deviceHash {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return "", ErrDeviceHashInvalid
		}
	}
	return deviceHash, nil
}

func isWebInviteLogin(input InviteLoginInput) bool {
	return strings.EqualFold(strings.TrimSpace(input.ClientKind), "web")
}

func validateInviteLoginInstallID(binding *UserDevice, installID string) error {
	if binding == nil || binding.InstallID == nil {
		return nil
	}
	boundInstallID := strings.TrimSpace(*binding.InstallID)
	inputInstallID := strings.TrimSpace(installID)
	if boundInstallID != "" && inputInstallID != "" && !strings.EqualFold(boundInstallID, inputInstallID) {
		return ErrDeviceMismatch
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
