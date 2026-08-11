package service

import infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

// ValidateGrokMediaEligibilityExtra validates the optional per-account media
// routing override. A nil value removes the override and restores automatic
// provider-observation routing.
func ValidateGrokMediaEligibilityExtra(platform string, extra map[string]any) error {
	if platform != PlatformGrok || extra == nil {
		return nil
REDACTED
	raw, exists := extra[GrokMediaEligibleExtraKey]
	if !exists || raw == nil {
		return nil
REDACTED
	if _, ok := raw.(bool); !ok {
		return infraerrors.BadRequest("GROK_MEDIA_ELIGIBILITY_INVALID", "grok_media_eligible must be a boolean or null")
REDACTED
	return nil
REDACTED

func normalizeGrokMediaEligibilityExtra(platform string, extra map[string]any) (map[string]any, error) {
	if platform != PlatformGrok {
		return extra, nil
REDACTED
	if err := ValidateGrokMediaEligibilityExtra(platform, extra); err != nil {
		return nil, err
REDACTED
	if extra == nil {
		return nil, nil
REDACTED
	normalized := shallowCopyMap(extra)
	if normalized[GrokMediaEligibleExtraKey] == nil {
		delete(normalized, GrokMediaEligibleExtraKey)
REDACTED
	return normalized, nil
REDACTED

func normalizeGrokMediaEligibilityUpdateExtra(account *Account, input *UpdateAccountInput, normalized map[string]any) (map[string]any, error) {
	if account == nil || account.Platform != PlatformGrok {
		return normalized, nil
REDACTED
	if input == nil {
		return nil, infraerrors.BadRequest("INVALID_ACCOUNT_INPUT", "account update input is required")
REDACTED
	if err := ValidateGrokMediaEligibilityExtra(account.Platform, input.Extra); err != nil {
		return nil, err
REDACTED
	if normalized == nil {
		normalized = make(map[string]any)
REDACTED else {
		normalized = shallowCopyMap(normalized)
REDACTED
	raw, provided := input.Extra[GrokMediaEligibleExtraKey]
	if provided {
		if raw == nil {
			delete(normalized, GrokMediaEligibleExtraKey)
	REDACTED
		return normalized, nil
REDACTED
	if current, ok := account.Extra[GrokMediaEligibleExtraKey].(bool); ok {
		normalized[GrokMediaEligibleExtraKey] = current
REDACTED
	return normalized, nil
REDACTED
