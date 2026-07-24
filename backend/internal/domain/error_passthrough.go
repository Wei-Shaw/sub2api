package domain

import "time"

// ErrorPassthroughRule is a global upstream-error passthrough rule.
// It controls how upstream failures are rewritten for clients.
type ErrorPassthroughRule struct {
	ID              int64
	Name            string
	Enabled         bool
	Priority        int
	ErrorCodes      []int
	Keywords        []string
	MatchMode       string
	Platforms       []string
	PassthroughCode bool
	ResponseCode    *int
	PassthroughBody bool
	CustomMessage   *string
	SkipMonitoring  bool
	Description     *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Match-mode constants for ErrorPassthroughRule.
const (
	MatchModeAny = "any"
	MatchModeAll = "all"
)

// AllErrorPassthroughPlatforms returns the platforms supported by passthrough rules.
// Values align with domain platform constants.
func AllErrorPassthroughPlatforms() []string {
	return []string{
		PlatformAnthropic,
		PlatformOpenAI,
		PlatformGemini,
		PlatformAntigravity,
		PlatformGrok,
	}
}

// Validate reports whether the rule configuration is usable.
func (r *ErrorPassthroughRule) Validate() error {
	if r == nil || r.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if r.MatchMode != MatchModeAny && r.MatchMode != MatchModeAll {
		return &ValidationError{Field: "match_mode", Message: "match_mode must be 'any' or 'all'"}
	}
	// At least one match condition is required.
	if len(r.ErrorCodes) == 0 && len(r.Keywords) == 0 {
		return &ValidationError{Field: "conditions", Message: "at least one error_code or keyword is required"}
	}
	if !r.PassthroughCode && (r.ResponseCode == nil || *r.ResponseCode <= 0) {
		return &ValidationError{Field: "response_code", Message: "response_code is required when passthrough_code is false"}
	}
	if !r.PassthroughBody && (r.CustomMessage == nil || *r.CustomMessage == "") {
		return &ValidationError{Field: "custom_message", Message: "custom_message is required when passthrough_body is false"}
	}
	return nil
}
