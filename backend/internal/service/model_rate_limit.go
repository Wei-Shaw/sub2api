package service

import (
	"context"
	"strings"
	"time"
)

const (
	modelRateLimitsKey                 = "model_rate_limits"
	antigravityGeminiModelRateLimitKey = "antigravity:gemini"
)

// isRateLimitActiveForKey 检查指定 key 的限流是否生效
func (a *Account) isRateLimitActiveForKey(key string) bool {
	resetAt := a.modelRateLimitResetAt(key)
	return resetAt != nil && time.Now().Before(*resetAt)
REDACTED

// getRateLimitRemainingForKey 获取指定 key 的限流剩余时间，0 表示未限流或已过期
func (a *Account) getRateLimitRemainingForKey(key string) time.Duration {
	resetAt := a.modelRateLimitResetAt(key)
	if resetAt == nil {
		return 0
REDACTED
	remaining := time.Until(*resetAt)
	if remaining > 0 {
		return remaining
REDACTED
	return 0
REDACTED

func (a *Account) isModelRateLimitedWithContext(ctx context.Context, requestedModel string) bool {
	if a == nil {
		return false
REDACTED

	modelKey := a.GetMappedModel(requestedModel)
	if a.Platform == PlatformAntigravity {
		modelKey = resolveFinalAntigravityModelKey(ctx, a, requestedModel)
		if isAntigravityGeminiModel(modelKey) && a.isRateLimitActiveForKey(antigravityGeminiModelRateLimitKey) {
			return true
	REDACTED
REDACTED
	modelKey = strings.TrimSpace(modelKey)
	if modelKey == "" {
		return false
REDACTED
	return a.isRateLimitActiveForKey(modelKey)
REDACTED

// GetModelRateLimitRemainingTime 获取模型限流剩余时间
// 返回 0 表示未限流或已过期
func (a *Account) GetModelRateLimitRemainingTime(requestedModel string) time.Duration {
	return a.GetModelRateLimitRemainingTimeWithContext(context.Background(), requestedModel)
REDACTED

func (a *Account) GetModelRateLimitRemainingTimeWithContext(ctx context.Context, requestedModel string) time.Duration {
	if a == nil {
		return 0
REDACTED

	modelKey := a.GetMappedModel(requestedModel)
	if a.Platform == PlatformAntigravity {
		modelKey = resolveFinalAntigravityModelKey(ctx, a, requestedModel)
REDACTED
	modelKey = strings.TrimSpace(modelKey)
	if modelKey == "" {
		return 0
REDACTED
	remaining := a.getRateLimitRemainingForKey(modelKey)
	if a.Platform == PlatformAntigravity && isAntigravityGeminiModel(modelKey) {
		if familyRemaining := a.getRateLimitRemainingForKey(antigravityGeminiModelRateLimitKey); familyRemaining > remaining {
			return familyRemaining
	REDACTED
REDACTED
	return remaining
REDACTED

func resolveFinalAntigravityModelKey(ctx context.Context, account *Account, requestedModel string) string {
	modelKey := mapAntigravityModel(account, requestedModel)
	if modelKey == "" {
		return ""
REDACTED
	// thinking 会影响 Antigravity 最终模型名（例如 claude-sonnet-4-5 -> claude-sonnet-4-5-thinking）
	if enabled, ok := ThinkingEnabledFromContext(ctx); ok {
		modelKey = applyThinkingModelSuffix(modelKey, enabled)
REDACTED
	return modelKey
REDACTED

func isAntigravityGeminiModel(model string) bool {
	return strings.HasPrefix(normalizeAntigravityModelName(model), "gemini-")
REDACTED

func antigravityModelRateLimitKeys(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
REDACTED
	keys := []string{modelREDACTED
	if isAntigravityGeminiModel(model) && model != antigravityGeminiModelRateLimitKey {
		keys = append(keys, antigravityGeminiModelRateLimitKey)
REDACTED
	return keys
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
