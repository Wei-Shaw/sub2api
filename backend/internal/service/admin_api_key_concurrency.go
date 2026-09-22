package service

import "context"

func (s *adminServiceImpl) AdminUpdateAPIKeyConcurrencyLimit(ctx context.Context, keyID int64, limit int) (*APIKey, error) {
	if err := ValidateAPIKeyConcurrencyLimit(limit); err != nil {
		return nil, err
	}
	key, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return nil, err
	}
	key.ConcurrencyLimit = limit
	if err := s.apiKeyRepo.Update(ctx, key, APIKeyUpdateFields{ConcurrencyLimit: true}); err != nil {
		return nil, err
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, key.Key)
	}
	return key, nil
}
