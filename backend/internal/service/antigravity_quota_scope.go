package service

import (
	"strings"
	"time"
)

const antigravityQuotaScopesKey = "antigravity_quota_scopes"

// AntigravityQuotaScope 表示 Antigravity 的配额域
type AntigravityQuotaScope string

const (
	AntigravityQuotaScopeClaude      AntigravityQuotaScope = "claude"
	AntigravityQuotaScopeGeminiText  AntigravityQuotaScope = "gemini_text"
	AntigravityQuotaScopeGeminiImage AntigravityQuotaScope = "gemini_image"
)

// resolveAntigravityQuotaScope 根据模型名称解析配额域
func resolveAntigravityQuotaScope(requestedModel string) (AntigravityQuotaScope, bool) {
	model := normalizeAntigravityModelName(requestedModel)
	if model == "" {
		return "", false
REDACTED
	switch {
	case strings.HasPrefix(model, "claude-"):
		return AntigravityQuotaScopeClaude, true
	case strings.HasPrefix(model, "gemini-"):
		if isImageGenerationModel(model) {
			return AntigravityQuotaScopeGeminiImage, true
	REDACTED
		return AntigravityQuotaScopeGeminiText, true
	default:
		return "", false
REDACTED
REDACTED

func normalizeAntigravityModelName(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	normalized = strings.TrimPrefix(normalized, "models/")
	return normalized
REDACTED

// IsSchedulableForModel 结合 Antigravity 配额域限流判断是否可调度
func (a *Account) IsSchedulableForModel(requestedModel string) bool {
	if a == nil {
		return false
REDACTED
	if !a.IsSchedulable() {
		return false
REDACTED
	if a.isModelRateLimited(requestedModel) {
		return false
REDACTED
	if a.Platform != PlatformAntigravity {
		return true
REDACTED
	scope, ok := resolveAntigravityQuotaScope(requestedModel)
	if !ok {
		return true
REDACTED
	resetAt := a.antigravityQuotaScopeResetAt(scope)
	if resetAt == nil {
		return true
REDACTED
	now := time.Now()
	return !now.Before(*resetAt)
REDACTED

func (a *Account) antigravityQuotaScopeResetAt(scope AntigravityQuotaScope) *time.Time {
	if a == nil || a.Extra == nil || scope == "" {
		return nil
REDACTED
	rawScopes, ok := a.Extra[antigravityQuotaScopesKey].(map[string]any)
	if !ok {
		return nil
REDACTED
	rawScope, ok := rawScopes[string(scope)].(map[string]any)
	if !ok {
		return nil
REDACTED
	resetAtRaw, ok := rawScope["rate_limit_reset_at"].(string)
	if !ok || strings.TrimSpace(resetAtRaw) == "" {
		return nil
REDACTED
	resetAt, err := time.Parse(time.RFC3339, resetAtRaw)
	if err != nil {
		return nil
REDACTED
	return &resetAt
REDACTED

var antigravityAllScopes = []AntigravityQuotaScope{
	AntigravityQuotaScopeClaude,
	AntigravityQuotaScopeGeminiText,
	AntigravityQuotaScopeGeminiImage,
REDACTED

func (a *Account) GetAntigravityScopeRateLimits() map[string]int64 {
	if a == nil || a.Platform != PlatformAntigravity {
		return nil
REDACTED
	now := time.Now()
	result := make(map[string]int64)
	for _, scope := range antigravityAllScopes {
		resetAt := a.antigravityQuotaScopeResetAt(scope)
		if resetAt != nil && now.Before(*resetAt) {
			remainingSec := int64(time.Until(*resetAt).Seconds())
			if remainingSec > 0 {
				result[string(scope)] = remainingSec
		REDACTED
	REDACTED
REDACTED
	if len(result) == 0 {
		return nil
REDACTED
	return result
REDACTED
