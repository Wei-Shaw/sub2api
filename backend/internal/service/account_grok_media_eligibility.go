package service

import infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

// ValidateGrokMediaEligibilityExtra validates the optional per-account media
// routing override. A nil value removes the override and restores automatic
// provider-observation routing.
func ValidateGrokMediaEligibilityExtra(platform string, extra map[string]any) error {
	if platform != PlatformGrok || extra == nil {
		return nil
	}
	raw, exists := extra[GrokMediaEligibleExtraKey]
	if !exists || raw == nil {
		return nil
	}
	if _, ok := raw.(bool); !ok {
		return infraerrors.BadRequest("GROK_MEDIA_ELIGIBILITY_INVALID", "grok_media_eligible must be a boolean or null")
	}
	return nil
}

func normalizeGrokMediaEligibilityExtra(platform string, extra map[string]any) (map[string]any, error) {
	if platform != PlatformGrok {
		return extra, nil
	}
	if err := ValidateGrokMediaEligibilityExtra(platform, extra); err != nil {
		return nil, err
	}
	if extra == nil {
		return nil, nil
	}
	normalized := shallowCopyMap(extra)
	if normalized[GrokMediaEligibleExtraKey] == nil {
		delete(normalized, GrokMediaEligibleExtraKey)
	}
	return normalized, nil
}

func normalizeGrokMediaEligibilityUpdateExtra(account *Account, input *UpdateAccountInput, normalized map[string]any) (map[string]any, error) {
	if account == nil || account.Platform != PlatformGrok {
		return normalized, nil
	}
	if input == nil {
		return nil, infraerrors.BadRequest("INVALID_ACCOUNT_INPUT", "account update input is required")
	}
	if err := ValidateGrokMediaEligibilityExtra(account.Platform, input.Extra); err != nil {
		return nil, err
	}
	if normalized == nil {
		normalized = make(map[string]any)
	} else {
		normalized = shallowCopyMap(normalized)
	}
	raw, provided := input.Extra[GrokMediaEligibleExtraKey]
	if provided {
		if raw == nil {
			delete(normalized, GrokMediaEligibleExtraKey)
		}
		return normalized, nil
	}
	if current, ok := account.Extra[GrokMediaEligibleExtraKey].(bool); ok {
		normalized[GrokMediaEligibleExtraKey] = current
	}
	return normalized, nil
}
