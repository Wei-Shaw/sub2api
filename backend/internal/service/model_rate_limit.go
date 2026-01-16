package service

import (
	"strings"
	"time"
)

const modelRateLimitsKey = "model_rate_limits"
const modelRateLimitScopeClaudeSonnet = "claude_sonnet"

func resolveModelRateLimitScope(requestedModel string) (string, bool) {
	model := strings.ToLower(strings.TrimSpace(requestedModel))
	if model == "" {
		return "", false
REDACTED
	model = strings.TrimPrefix(model, "models/")
	if strings.Contains(model, "sonnet") {
		return modelRateLimitScopeClaudeSonnet, true
REDACTED
	return "", false
REDACTED

func (a *Account) isModelRateLimited(requestedModel string) bool {
	scope, ok := resolveModelRateLimitScope(requestedModel)
	if !ok {
		return false
REDACTED
	resetAt := a.modelRateLimitResetAt(scope)
	if resetAt == nil {
		return false
REDACTED
	return time.Now().Before(*resetAt)
REDACTED

func (a *Account) modelRateLimitResetAt(scope string) *time.Time {
	if a == nil || a.Extra == nil || scope == "" {
		return nil
REDACTED
	rawLimits, ok := a.Extra[modelRateLimitsKey].(map[string]any)
	if !ok {
		return nil
REDACTED
	rawLimit, ok := rawLimits[scope].(map[string]any)
	if !ok {
		return nil
REDACTED
	resetAtRaw, ok := rawLimit["rate_limit_reset_at"].(string)
	if !ok || strings.TrimSpace(resetAtRaw) == "" {
		return nil
REDACTED
	resetAt, err := time.Parse(time.RFC3339, resetAtRaw)
	if err != nil {
		return nil
REDACTED
	return &resetAt
REDACTED
