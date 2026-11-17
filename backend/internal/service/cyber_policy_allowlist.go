package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ContentModerationModeCyberLogOnly preserves upstream evidence while excluding
// these events from subsequent automatic-ban counts, even after whitelist removal.
const ContentModerationModeCyberLogOnly = "cyber_log_only"
const ContentModerationModeRiskControlLogOnly = "risk_control_log_only"

// The historical cyber_policy_user_allowlist storage/API key also governs ordinary moderation.

// ParseCyberPolicyUserAllowlist accepts positive platform user IDs separated by
// commas or whitespace. Reject the whole list on invalid input; never accept a
// partially parsed privilege configuration.
func ParseCyberPolicyUserAllowlist(raw string) (map[int64]struct{}, error) {
	if len(raw) > 16384 {
		return nil, fmt.Errorf("cyber_policy_user_allowlist must not exceed 16384 bytes")
	}
	ids := make(map[int64]struct{})
	for _, field := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		id, err := strconv.ParseInt(field, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("cyber_policy_user_allowlist must contain positive user IDs separated by commas or whitespace")
		}
		ids[id] = struct{}{}
	}
	return ids, nil
}

func (s *SettingService) IsCyberPolicyUserAllowlisted(ctx context.Context, userID int64) bool {
	if s == nil || s.settingRepo == nil || userID <= 0 {
		return false
	}
	s.GetCyberSessionBlockRuntime(ctx)
	if cached, ok := s.cyberSessionBlockRuntimeCache.Load().(*cachedCyberSessionBlockRuntime); ok && cached != nil {
		_, allowed := cached.allowlistedUsers[userID]
		return allowed
	}
	return false
}

func (s *OpenAIGatewayService) CyberPolicyLogOnly(ctx context.Context, apiKey *APIKey) bool {
	return s != nil && apiKey != nil && s.settingService.IsCyberPolicyUserAllowlisted(ctx, apiKey.UserID)
}
