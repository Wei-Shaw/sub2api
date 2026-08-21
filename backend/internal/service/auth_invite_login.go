package service

import (
	"context"
	"strings"
	"time"
)

type InviteLoginInput struct {
	InvitationCode string
	DeviceHash     string
}

type InviteLoginResult struct {
	TokenPair *TokenPair
	User      *User
}

func (s *AuthService) SetInviteLoginDeviceResolver(repo UserDeviceRepository) {
	if s != nil {
		s.inviteLoginDeviceRepo = repo
	}
}

// InviteLogin authenticates a device using its explicit DLG code and fingerprint.
// It intentionally does not select a device by user or choose the newest binding.
func (s *AuthService) InviteLogin(ctx context.Context, input InviteLoginInput) (*InviteLoginResult, error) {
	code := strings.ToUpper(strings.TrimSpace(input.InvitationCode))
	if code == "" {
		return nil, ErrInvitationCodeRequired
	}
	if s == nil || s.inviteLoginDeviceRepo == nil || s.userRepo == nil {
		return nil, ErrServiceUnavailable
	}
	if err := validateInviteLoginDeviceInput(input); err != nil {
		return nil, err
	}

	device, err := s.inviteLoginDeviceRepo.GetByDeviceCode(ctx, code)
	if err != nil || device == nil {
		return nil, ErrInvitationCodeInvalid
	}
	if !device.IsActive() {
		return nil, ErrDeviceRevoked
	}
	if normalizeDeviceHash(device.DeviceHash) != normalizeDeviceHash(input.DeviceHash) {
		return nil, ErrDeviceMismatch
	}

	user, err := s.userRepo.GetByID(ctx, device.UserID)
	if err != nil || user == nil {
		return nil, ErrInvitationCodeInvalid
	}
	if !user.IsActive() {
		return nil, ErrUserNotActive
	}

	tokenPair, err := s.GenerateTokenPair(ctx, user, "")
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	if err := s.inviteLoginDeviceRepo.UpdateLastLoginAt(ctx, device.ID, time.Now().UTC()); err != nil {
		return nil, ErrServiceUnavailable
	}
	return &InviteLoginResult{TokenPair: tokenPair, User: user}, nil
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
