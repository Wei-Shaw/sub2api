package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/redis/go-redis/v9"
)

const (
	legacyOAuthTokenKeyPrefix    = "oauth:token:"
	encryptedOAuthTokenKeyPrefix = "oauth:token:v1:"
	oauthRefreshLockKeyPrefix    = "oauth:refresh_lock:"
)

type geminiTokenCache struct {
	rdb              *redis.Client
	credentialCipher *CredentialCipher
}

func NewGeminiTokenCache(rdb *redis.Client) service.GeminiTokenCache {
	return newGeminiTokenCache(rdb, nil)
}

func newGeminiTokenCache(rdb *redis.Client, credentialCipher *CredentialCipher) *geminiTokenCache {
	return &geminiTokenCache{rdb: rdb, credentialCipher: credentialCipher}
}

// ProvideGeminiTokenCache constructs the shared provider access-token cache.
// Release deployments use a versioned encrypted keyspace and purge only the
// pre-encryption oauth:token:* keys; distributed refresh locks are preserved.
func ProvideGeminiTokenCache(rdb *redis.Client, credentialCipher *CredentialCipher) (service.GeminiTokenCache, error) {
	if credentialCipher != nil {
		purgeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := purgeLegacyOAuthTokenCache(purgeCtx, rdb); err != nil {
			return nil, err
		}
	}
	return newGeminiTokenCache(rdb, credentialCipher), nil
}

func (c *geminiTokenCache) GetAccessToken(ctx context.Context, cacheKey string) (string, error) {
	key := c.accessTokenKey(cacheKey)
	storage, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) && c.credentialCipher != nil {
			// A rolling upgrade may leave the old plaintext key behind after the
			// startup scan. Never read it; remove the exact legacy key and miss.
			if deleteErr := c.rdb.Del(ctx, legacyOAuthTokenKeyPrefix+cacheKey).Err(); deleteErr != nil {
				return "", deleteErr
			}
		}
		return "", err
	}
	if c.credentialCipher == nil {
		return storage, nil
	}
	token, legacy, err := c.credentialCipher.DecryptOAuthTokenCache(cacheKey, storage)
	if err != nil {
		return "", fmt.Errorf("decrypt OAuth access-token cache: %w", err)
	}
	if legacy {
		if err := c.rdb.Del(ctx, key).Err(); err != nil {
			return "", err
		}
		return "", redis.Nil
	}
	return token, nil
}

func (c *geminiTokenCache) SetAccessToken(ctx context.Context, cacheKey string, token string, ttl time.Duration) error {
	key := c.accessTokenKey(cacheKey)
	storage := token
	if c.credentialCipher != nil {
		var err error
		storage, err = c.credentialCipher.EncryptOAuthTokenCache(cacheKey, token)
		if err != nil {
			return fmt.Errorf("encrypt OAuth access-token cache: %w", err)
		}
	}
	return c.rdb.Set(ctx, key, storage, ttl).Err()
}

func (c *geminiTokenCache) DeleteAccessToken(ctx context.Context, cacheKey string) error {
	return c.rdb.Del(ctx, encryptedOAuthTokenKeyPrefix+cacheKey, legacyOAuthTokenKeyPrefix+cacheKey).Err()
}

func (c *geminiTokenCache) AcquireRefreshLock(ctx context.Context, cacheKey string, ttl time.Duration) (bool, error) {
	key := fmt.Sprintf("%s%s", oauthRefreshLockKeyPrefix, cacheKey)
	return c.rdb.SetNX(ctx, key, 1, ttl).Result()
}

func (c *geminiTokenCache) ReleaseRefreshLock(ctx context.Context, cacheKey string) error {
	key := fmt.Sprintf("%s%s", oauthRefreshLockKeyPrefix, cacheKey)
	return c.rdb.Del(ctx, key).Err()
}

func (c *geminiTokenCache) accessTokenKey(cacheKey string) string {
	if c != nil && c.credentialCipher != nil {
		return encryptedOAuthTokenKeyPrefix + cacheKey
	}
	return legacyOAuthTokenKeyPrefix + cacheKey
}

func purgeLegacyOAuthTokenCache(ctx context.Context, rdb *redis.Client) error {
	if rdb == nil {
		return errors.New("redis client is required to purge legacy OAuth token cache")
	}
	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, legacyOAuthTokenKeyPrefix+"*", 512).Result()
		if err != nil {
			return fmt.Errorf("scan legacy OAuth token cache: %w", err)
		}
		legacyKeys := make([]string, 0, len(keys))
		for _, key := range keys {
			if strings.HasPrefix(key, legacyOAuthTokenKeyPrefix) && !strings.HasPrefix(key, encryptedOAuthTokenKeyPrefix) {
				legacyKeys = append(legacyKeys, key)
			}
		}
		if len(legacyKeys) > 0 {
			if err := rdb.Del(ctx, legacyKeys...).Err(); err != nil {
				return fmt.Errorf("purge legacy OAuth token cache: %w", err)
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}
