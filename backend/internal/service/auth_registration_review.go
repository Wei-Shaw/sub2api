package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const DefaultRegistrationReasonMaxLength = 500

func parseRegistrationReasonMaxLength(raw string) int {
	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || limit < 1 || limit > 10000 {
		return DefaultRegistrationReasonMaxLength
	}
	return limit
}

var (
	ErrRegistrationReasonRequired = infraerrors.BadRequest("REGISTRATION_REASON_REQUIRED", "registration reason is required")
	ErrRegistrationReasonTooLong  = infraerrors.BadRequest("REGISTRATION_REASON_TOO_LONG", "registration reason exceeds the configured character limit")
)

func (s *AuthService) resolveRegistrationStatus(ctx context.Context, reason string) (string, string, error) {
	reason = strings.TrimSpace(reason)
	limit := DefaultRegistrationReasonMaxLength
	if s.settingService != nil {
		raw, err := s.settingService.settingRepo.GetValue(ctx, SettingKeyRegistrationReasonMaxLength)
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			return "", "", ErrServiceUnavailable.WithCause(err)
		}
		limit = parseRegistrationReasonMaxLength(raw)
	}
	if utf8.RuneCountInString(reason) > limit {
		return "", "", ErrRegistrationReasonTooLong
	}
	if s.settingService == nil {
		return StatusActive, reason, nil
	}
	value, err := s.settingService.settingRepo.GetValue(ctx, SettingKeyRegistrationReviewEnabled)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return "", "", ErrServiceUnavailable.WithCause(err)
	}
	if value == "true" {
		if reason == "" {
			return "", "", ErrRegistrationReasonRequired
		}
		return StatusPending, reason, nil
	}
	return StatusActive, reason, nil
}

func firstRegistrationReason(reasons []string) string {
	if len(reasons) > 0 {
		return reasons[0]
	}
	return ""
}

func activeUserError(user *User) error {
	if user != nil && user.Status == StatusPending {
		return ErrRegistrationPending
	}
	if user == nil || !user.IsActive() {
		return ErrUserNotActive
	}
	return nil
}
