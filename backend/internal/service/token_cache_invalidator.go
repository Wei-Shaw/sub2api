package service

import "context"

type TokenCacheInvalidator interface {
	InvalidateToken(ctx context.Context, account *Account) error
REDACTED

type CompositeTokenCacheInvalidator struct {
	geminiCache GeminiTokenCache
REDACTED

func NewCompositeTokenCacheInvalidator(geminiCache GeminiTokenCache) *CompositeTokenCacheInvalidator {
	return &CompositeTokenCacheInvalidator{
		geminiCache: geminiCache,
REDACTED
REDACTED

func (c *CompositeTokenCacheInvalidator) InvalidateToken(ctx context.Context, account *Account) error {
	if c == nil || c.geminiCache == nil || account == nil {
		return nil
REDACTED
	if account.Type != AccountTypeOAuth {
		return nil
REDACTED

	switch account.Platform {
	case PlatformGemini:
		return c.geminiCache.DeleteAccessToken(ctx, GeminiTokenCacheKey(account))
	case PlatformAntigravity:
		return c.geminiCache.DeleteAccessToken(ctx, AntigravityTokenCacheKey(account))
	default:
		return nil
REDACTED
REDACTED
