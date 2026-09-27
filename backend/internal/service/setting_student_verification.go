package service

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// IsStudentVerificationEnabled reports whether student verification is enabled.
// Fails closed: a missing or unreadable setting disables the feature.
func (s *SettingService) IsStudentVerificationEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyStudentVerificationEnabled)
	if err != nil {
		return false
	}
	return value == "true"
}

// GetStudentVerificationEmailSuffixes returns the normalized school-email
// domain whitelist. Empty list means no email qualifies (fail closed), which
// differs from the registration whitelist where empty means allow-all.
func (s *SettingService) GetStudentVerificationEmailSuffixes(ctx context.Context) []string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyStudentVerificationEmailSuffixes)
	if err != nil {
		return []string{}
	}
	return ParseRegistrationEmailSuffixWhitelist(value)
}

// IsStudentVerificationEmailAllowed checks a candidate school email against the
// configured domain whitelist. Empty whitelist denies everything.
func (s *SettingService) IsStudentVerificationEmailAllowed(ctx context.Context, email string) bool {
	suffixes := s.GetStudentVerificationEmailSuffixes(ctx)
	if len(suffixes) == 0 {
		return false
	}
	return IsRegistrationEmailSuffixAllowed(email, suffixes)
}

// GetStudentVerificationValidityDays returns the verification validity period
// in days, clamped to [Min, Max]; missing/invalid values fall back to default.
func (s *SettingService) GetStudentVerificationValidityDays(ctx context.Context) int {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyStudentVerificationValidityDays)
	if err != nil {
		return StudentVerificationValidityDaysDefault
	}
	days, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return StudentVerificationValidityDaysDefault
	}
	if days < StudentVerificationValidityDaysMin {
		return StudentVerificationValidityDaysMin
	}
	if days > StudentVerificationValidityDaysMax {
		return StudentVerificationValidityDaysMax
	}
	return days
}

// GetStudentVerificationGroupIDs returns the configured student group IDs.
// Invalid JSON or entries degrade to an empty list.
func (s *SettingService) GetStudentVerificationGroupIDs(ctx context.Context) []int64 {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyStudentVerificationGroupIDs)
	if err != nil {
		return []int64{}
	}
	var ids []int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &ids); err != nil {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// GetStudentVerificationRebateRatePercent returns the exclusive rebate rate
// granted to verified students, clamped to the affiliate rate bounds.
// A non-positive value disables the rebate grant while keeping group access.
func (s *SettingService) GetStudentVerificationRebateRatePercent(ctx context.Context) float64 {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyStudentVerificationRebateRate)
	if err != nil {
		return 0
	}
	rate, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return 0
	}
	return clampAffiliateRebateRate(rate)
}
