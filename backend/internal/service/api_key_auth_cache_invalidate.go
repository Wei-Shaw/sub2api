package service

import "context"

// InvalidateAuthCacheByKey 清除指定 API Key 的认证缓存
func (s *APIKeyService) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	if key == "" {
		return
REDACTED
	cacheKey := s.authCacheKey(key)
	s.deleteAuthCache(ctx, cacheKey)
REDACTED

// InvalidateAuthCacheByUserID 清除用户相关的 API Key 认证缓存
func (s *APIKeyService) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	if userID <= 0 {
		return
REDACTED
	keys, err := s.apiKeyRepo.ListKeysByUserID(ctx, userID)
	if err != nil {
		return
REDACTED
	s.deleteAuthCacheByKeys(ctx, keys)
REDACTED

// InvalidateAuthCacheByGroupID 清除分组相关的 API Key 认证缓存
func (s *APIKeyService) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	if groupID <= 0 {
		return
REDACTED
	keys, err := s.apiKeyRepo.ListKeysByGroupID(ctx, groupID)
	if err != nil {
		return
REDACTED
	s.deleteAuthCacheByKeys(ctx, keys)
REDACTED

func (s *APIKeyService) deleteAuthCacheByKeys(ctx context.Context, keys []string) {
	if len(keys) == 0 {
		return
REDACTED
	for _, key := range keys {
		if key == "" {
			continue
	REDACTED
		s.deleteAuthCache(ctx, s.authCacheKey(key))
REDACTED
REDACTED
