package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/gin-gonic/gin"
)

type openAILegacySessionHashContextKey struct{}

var openAILegacySessionHashKey = openAILegacySessionHashContextKey{}

var (
	openAIStickyLegacyReadFallbackTotal atomic.Int64
	openAIStickyLegacyReadFallbackHit   atomic.Int64
	openAIStickyLegacyDualWriteTotal    atomic.Int64
)

const (
	openAIStickyAffinityBindingNamespace   = "sticky_affinity"
	openAIResponseAffinityBindingNamespace = "response_affinity"
)

func openAIStickyCompatStats() (legacyReadFallbackTotal, legacyReadFallbackHit, legacyDualWriteTotal int64) {
	return openAIStickyLegacyReadFallbackTotal.Load(),
		openAIStickyLegacyReadFallbackHit.Load(),
		openAIStickyLegacyDualWriteTotal.Load()
}

// DeriveSessionHashFromSeed computes the current-format sticky-session hash
// from an arbitrary seed string.
func DeriveSessionHashFromSeed(seed string) string {
	currentHash, _ := deriveOpenAISessionHashes(seed)
	return currentHash
}

func deriveOpenAISessionHashes(sessionID string) (currentHash string, legacyHash string) {
	normalized := strings.TrimSpace(sessionID)
	if normalized == "" {
		return "", ""
	}

	currentHash = fmt.Sprintf("%016x", xxhash.Sum64String(normalized))
	sum := sha256.Sum256([]byte(normalized))
	legacyHash = hex.EncodeToString(sum[:])
	return currentHash, legacyHash
}

func withOpenAILegacySessionHash(ctx context.Context, legacyHash string) context.Context {
	if ctx == nil {
		return nil
	}
	trimmed := strings.TrimSpace(legacyHash)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, openAILegacySessionHashKey, trimmed)
}

func openAILegacySessionHashFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(openAILegacySessionHashKey).(string)
	return strings.TrimSpace(value)
}

func attachOpenAILegacySessionHashToGin(c *gin.Context, legacyHash string) {
	if c == nil || c.Request == nil {
		return
	}
	c.Request = c.Request.WithContext(withOpenAILegacySessionHash(c.Request.Context(), legacyHash))
}

func (s *OpenAIGatewayService) openAISessionHashReadOldFallbackEnabled() bool {
	if s == nil || s.cfg == nil {
		return true
	}
	return s.cfg.Gateway.OpenAIWS.SessionHashReadOldFallback
}

func (s *OpenAIGatewayService) openAISessionHashDualWriteOldEnabled() bool {
	if s == nil || s.cfg == nil {
		return true
	}
	return s.cfg.Gateway.OpenAIWS.SessionHashDualWriteOld
}

func (s *OpenAIGatewayService) openAISessionCacheKey(sessionHash string) string {
	normalized := strings.TrimSpace(sessionHash)
	if normalized == "" {
		return ""
	}
	return "openai:" + normalized
}

func (s *OpenAIGatewayService) openAILegacySessionCacheKey(ctx context.Context, sessionHash string) string {
	legacyHash := openAILegacySessionHashFromContext(ctx)
	if legacyHash == "" {
		return ""
	}
	legacyKey := "openai:" + legacyHash
	if legacyKey == s.openAISessionCacheKey(sessionHash) {
		return ""
	}
	return legacyKey
}

func (s *OpenAIGatewayService) openAIStickyLegacyTTL(ttl time.Duration) time.Duration {
	legacyTTL := ttl
	if legacyTTL <= 0 {
		legacyTTL = openaiStickySessionTTL
	}
	if legacyTTL > 10*time.Minute {
		return 10 * time.Minute
	}
	return legacyTTL
}

func (s *OpenAIGatewayService) getStickySessionAccountID(ctx context.Context, groupID *int64, sessionHash string) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, nil
	}

	primaryKey := s.openAISessionCacheKey(sessionHash)
	if primaryKey == "" {
		return 0, nil
	}

	accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), primaryKey)
	if err == nil && accountID > 0 {
		return accountID, nil
	}
	if !s.openAISessionHashReadOldFallbackEnabled() {
		return accountID, err
	}

	legacyKey := s.openAILegacySessionCacheKey(ctx, sessionHash)
	if legacyKey == "" {
		return accountID, err
	}

	openAIStickyLegacyReadFallbackTotal.Add(1)
	legacyAccountID, legacyErr := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), legacyKey)
	if legacyErr == nil && legacyAccountID > 0 {
		openAIStickyLegacyReadFallbackHit.Add(1)
		return legacyAccountID, nil
	}
	return accountID, err
}

func (s *OpenAIGatewayService) setStickySessionAccountID(ctx context.Context, groupID *int64, sessionHash string, accountID int64, ttl time.Duration) error {
	if s == nil || s.cache == nil || accountID <= 0 {
		return nil
	}
	primaryKey := s.openAISessionCacheKey(sessionHash)
	if primaryKey == "" {
		return nil
	}

	if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), primaryKey, accountID, ttl); err != nil {
		return err
	}

	if !s.openAISessionHashDualWriteOldEnabled() {
		return nil
	}
	legacyKey := s.openAILegacySessionCacheKey(ctx, sessionHash)
	if legacyKey == "" {
		return nil
	}
	if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), legacyKey, accountID, s.openAIStickyLegacyTTL(ttl)); err != nil {
		return err
	}
	openAIStickyLegacyDualWriteTotal.Add(1)
	return nil
}

func (s *OpenAIGatewayService) refreshStickySessionTTL(ctx context.Context, groupID *int64, sessionHash string, ttl time.Duration) error {
	if s == nil || s.cache == nil {
		return nil
	}
	primaryKey := s.openAISessionCacheKey(sessionHash)
	if primaryKey == "" {
		return nil
	}

	err := s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), primaryKey, ttl)
	if !s.openAISessionHashReadOldFallbackEnabled() && !s.openAISessionHashDualWriteOldEnabled() {
		return err
	}

	legacyKey := s.openAILegacySessionCacheKey(ctx, sessionHash)
	if legacyKey != "" {
		_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), legacyKey, s.openAIStickyLegacyTTL(ttl))
	}
	return err
}

func (s *OpenAIGatewayService) deleteStickySessionAccountID(ctx context.Context, groupID *int64, sessionHash string) error {
	if s == nil || s.cache == nil {
		return nil
	}
	primaryKey := s.openAISessionCacheKey(sessionHash)
	if primaryKey == "" {
		return nil
	}

	err := s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), primaryKey)
	if !s.openAISessionHashReadOldFallbackEnabled() && !s.openAISessionHashDualWriteOldEnabled() {
		return err
	}

	legacyKey := s.openAILegacySessionCacheKey(ctx, sessionHash)
	if legacyKey != "" {
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), legacyKey)
	}
	return err
}

func (s *OpenAIGatewayService) openAICompanionBindingCache() OpenAICompanionBindingCache {
	if s == nil || s.cache == nil {
		return nil
	}
	cache, _ := s.cache.(OpenAICompanionBindingCache)
	return cache
}

func marshalOpenAIAffinityBinding(binding *openAIAffinityBinding) (string, error) {
	if binding == nil {
		return "", nil
	}
	payload, err := json.Marshal(binding)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func parseOpenAIAffinityBinding(raw string) (*openAIAffinityBinding, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var binding openAIAffinityBinding
	if err := json.Unmarshal([]byte(trimmed), &binding); err != nil {
		return nil, err
	}
	return cloneOpenAIAffinityBinding(&binding), nil
}

func (s *OpenAIGatewayService) getOpenAICompanionBinding(
	ctx context.Context,
	groupID *int64,
	namespace string,
	bindingKey string,
) (*openAIAffinityBinding, error) {
	cache := s.openAICompanionBindingCache()
	if cache == nil || strings.TrimSpace(bindingKey) == "" {
		return nil, nil
	}
	raw, err := cache.GetOpenAICompanionBinding(ctx, derefGroupID(groupID), namespace, bindingKey)
	if err != nil {
		return nil, err
	}
	return parseOpenAIAffinityBinding(raw)
}

func (s *OpenAIGatewayService) setOpenAICompanionBinding(
	ctx context.Context,
	groupID *int64,
	namespace string,
	bindingKey string,
	binding *openAIAffinityBinding,
	ttl time.Duration,
) error {
	cache := s.openAICompanionBindingCache()
	if cache == nil || strings.TrimSpace(bindingKey) == "" || binding == nil || binding.BoundAccountID <= 0 {
		return nil
	}
	payload, err := marshalOpenAIAffinityBinding(binding)
	if err != nil {
		return err
	}
	return cache.SetOpenAICompanionBinding(ctx, derefGroupID(groupID), namespace, bindingKey, payload, ttl)
}

func (s *OpenAIGatewayService) refreshOpenAICompanionBinding(
	ctx context.Context,
	groupID *int64,
	namespace string,
	bindingKey string,
	ttl time.Duration,
) error {
	cache := s.openAICompanionBindingCache()
	if cache == nil || strings.TrimSpace(bindingKey) == "" {
		return nil
	}
	return cache.RefreshOpenAICompanionBindingTTL(ctx, derefGroupID(groupID), namespace, bindingKey, ttl)
}

func (s *OpenAIGatewayService) deleteOpenAICompanionBinding(
	ctx context.Context,
	groupID *int64,
	namespace string,
	bindingKey string,
) error {
	cache := s.openAICompanionBindingCache()
	if cache == nil || strings.TrimSpace(bindingKey) == "" {
		return nil
	}
	return cache.DeleteOpenAICompanionBinding(ctx, derefGroupID(groupID), namespace, bindingKey)
}

func (s *OpenAIGatewayService) getOpenAIStickyAffinityBinding(ctx context.Context, groupID *int64, sessionHash string) *openAIAffinityBinding {
	if s == nil {
		return nil
	}
	primaryKey := s.openAISessionCacheKey(sessionHash)
	if primaryKey == "" {
		return nil
	}
	binding, err := s.getOpenAICompanionBinding(ctx, groupID, openAIStickyAffinityBindingNamespace, primaryKey)
	if err == nil && binding != nil {
		return binding
	}
	if !s.openAISessionHashReadOldFallbackEnabled() {
		return nil
	}
	legacyKey := s.openAILegacySessionCacheKey(ctx, sessionHash)
	if legacyKey == "" {
		return nil
	}
	binding, _ = s.getOpenAICompanionBinding(ctx, groupID, openAIStickyAffinityBindingNamespace, legacyKey)
	return binding
}

func (s *OpenAIGatewayService) setOpenAIStickyAffinityBinding(ctx context.Context, groupID *int64, sessionHash string, binding *openAIAffinityBinding, ttl time.Duration) error {
	if s == nil || binding == nil || binding.BoundAccountID <= 0 {
		return nil
	}
	primaryKey := s.openAISessionCacheKey(sessionHash)
	if primaryKey == "" {
		return nil
	}
	if err := s.setOpenAICompanionBinding(ctx, groupID, openAIStickyAffinityBindingNamespace, primaryKey, binding, ttl); err != nil {
		return err
	}
	if !s.openAISessionHashDualWriteOldEnabled() {
		return nil
	}
	legacyKey := s.openAILegacySessionCacheKey(ctx, sessionHash)
	if legacyKey == "" {
		return nil
	}
	return s.setOpenAICompanionBinding(ctx, groupID, openAIStickyAffinityBindingNamespace, legacyKey, binding, s.openAIStickyLegacyTTL(ttl))
}

func (s *OpenAIGatewayService) refreshOpenAIStickyAffinityBinding(ctx context.Context, groupID *int64, sessionHash string, ttl time.Duration) error {
	if s == nil {
		return nil
	}
	primaryKey := s.openAISessionCacheKey(sessionHash)
	if primaryKey == "" {
		return nil
	}
	err := s.refreshOpenAICompanionBinding(ctx, groupID, openAIStickyAffinityBindingNamespace, primaryKey, ttl)
	if !s.openAISessionHashReadOldFallbackEnabled() && !s.openAISessionHashDualWriteOldEnabled() {
		return err
	}
	legacyKey := s.openAILegacySessionCacheKey(ctx, sessionHash)
	if legacyKey != "" {
		_ = s.refreshOpenAICompanionBinding(ctx, groupID, openAIStickyAffinityBindingNamespace, legacyKey, s.openAIStickyLegacyTTL(ttl))
	}
	return err
}

func (s *OpenAIGatewayService) deleteOpenAIStickyAffinityBinding(ctx context.Context, groupID *int64, sessionHash string) error {
	if s == nil {
		return nil
	}
	primaryKey := s.openAISessionCacheKey(sessionHash)
	if primaryKey == "" {
		return nil
	}
	err := s.deleteOpenAICompanionBinding(ctx, groupID, openAIStickyAffinityBindingNamespace, primaryKey)
	if !s.openAISessionHashReadOldFallbackEnabled() && !s.openAISessionHashDualWriteOldEnabled() {
		return err
	}
	legacyKey := s.openAILegacySessionCacheKey(ctx, sessionHash)
	if legacyKey != "" {
		_ = s.deleteOpenAICompanionBinding(ctx, groupID, openAIStickyAffinityBindingNamespace, legacyKey)
	}
	return err
}

func (s *OpenAIGatewayService) clearOpenAIStickySessionBindings(ctx context.Context, groupID *int64, sessionHash string) error {
	if s == nil {
		return nil
	}
	_ = s.deleteOpenAIStickyAffinityBinding(ctx, groupID, sessionHash)
	return s.deleteStickySessionAccountID(ctx, groupID, sessionHash)
}

func (s *OpenAIGatewayService) bindOpenAIStickySessionAffinity(ctx context.Context, groupID *int64, sessionHash string, binding *openAIAffinityBinding) error {
	if s == nil || strings.TrimSpace(sessionHash) == "" || binding == nil || binding.BoundAccountID <= 0 {
		return nil
	}
	ttl := s.openAIWSSessionStickyTTL()
	sharedErr := s.setStickySessionAccountID(ctx, groupID, sessionHash, binding.BoundAccountID, ttl)
	if shouldBindOpenAISharedSticky(binding.SelectedGroup) {
		_ = s.deleteOpenAIStickyAffinityBinding(ctx, groupID, sessionHash)
		return sharedErr
	}
	if err := s.setOpenAIStickyAffinityBinding(ctx, groupID, sessionHash, binding, ttl); err != nil {
		return err
	}
	return sharedErr
}

func (s *OpenAIGatewayService) BindOpenAIStickySession(ctx context.Context, groupID *int64, sessionHash string, accountID int64, selectedGroup string) error {
	binding := newOpenAIAffinityBinding(accountID, selectedGroup)
	return s.bindOpenAIStickySessionAffinity(ctx, groupID, sessionHash, binding)
}
