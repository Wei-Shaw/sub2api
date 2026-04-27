package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *AuthService) completeDeviceInviteLogin(ctx context.Context, input InviteLoginInput, code *RedeemCode) (*InviteLoginResult, error) {
	if code == nil || code.Type != RedeemTypeDeviceLogin {
		return nil, ErrInvitationCodeInvalid
	}
	if s.inviteLoginDeviceRepo == nil || s.userRepo == nil {
		return nil, ErrServiceUnavailable
	}

	deviceHash := normalizeDeviceHash(input.DeviceHash)
	if deviceHash == "" {
		return nil, ErrDeviceHashRequired
	}
	if len(deviceHash) != 64 {
		return nil, ErrDeviceHashInvalid
	}
	for _, ch := range deviceHash {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return nil, ErrDeviceHashInvalid
		}
	}

	device, err := s.inviteLoginDeviceRepo.GetByLoginRedeemCodeID(ctx, code.ID)
	if err != nil {
		if errors.Is(err, ErrUserDeviceNotFound) {
			return nil, ErrInvitationCodeInvalid
		}
		return nil, ErrServiceUnavailable
	}
	if device == nil || !device.IsActive() {
		return nil, ErrDeviceRevoked
	}
	if normalizeDeviceHash(device.DeviceHash) != deviceHash {
		return nil, ErrDeviceMismatch
	}

	inputInstallID := strings.TrimSpace(input.InstallID)
	if device.InstallID != nil {
		boundInstallID := strings.TrimSpace(*device.InstallID)
		if boundInstallID != "" && inputInstallID != "" && !strings.EqualFold(boundInstallID, inputInstallID) {
			return nil, ErrDeviceMismatch
		}
	}

	user, err := s.userRepo.GetByID(ctx, device.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvitationCodeInvalid
		}
		return nil, ErrServiceUnavailable
	}

	grantPlan := s.resolveSignupGrantPlan(ctx, "invite_login")
	s.assignSubscriptions(ctx, user.ID, grantPlan.Subscriptions, "auto assigned by invite login")

	bootstrapKeys, err := s.provisionInviteBootstrapAPIKeys(ctx, user.ID, code)
	if err != nil {
		return nil, err
	}

	tokenPair, err := s.GenerateTokenPair(ctx, user, "")
	if err != nil {
		return nil, fmt.Errorf("generate token pair: %w", err)
	}

	if err := s.inviteLoginDeviceRepo.UpdateLastLoginAt(ctx, device.ID, time.Now().UTC()); err != nil {
		return nil, ErrServiceUnavailable
	}

	return &InviteLoginResult{
		TokenPair:        tokenPair,
		User:             user,
		BootstrapAPIKeys: bootstrapKeys,
	}, nil
}
