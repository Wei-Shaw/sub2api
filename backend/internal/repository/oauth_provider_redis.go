package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// OAuth 存储的 key 前缀与载荷编码属于存储层职责，service 层只面向语义接口。
const (
	oauthTransactionKeyPrefix = "oauth:transaction:"
	oauthCodeKeyPrefix        = "oauth:code:"
	oauthAccessKeyPrefix      = "oauth:access:"
	oauthRefreshKeyPrefix     = "oauth:refresh:"
)

// ErrOAuthStoreNotFound 表示查询的 OAuth 状态不存在（含已过期被清除）。
var ErrOAuthStoreNotFound = errors.New("oauth store: entry not found")

// oauthTransactionDocument 对应 service.OAuthAuthorizationTransaction；
// State 在域结构体的 JSON 投影之外，这里随存储文档一并持久化。
type oauthTransactionDocument struct {
	service.OAuthAuthorizationTransaction
	State string `json:"state"`
}

// OAuthProviderRedisStore 以 redis 承载 OAuth Provider 的授权事务、授权码与令牌状态。
type OAuthProviderRedisStore struct {
	rdb *redis.Client
}

// NewOAuthProviderRedisStore 返回 service.OAuthProviderStore 的 redis 实现。
func NewOAuthProviderRedisStore(rdb *redis.Client) service.OAuthProviderStore {
	return &OAuthProviderRedisStore{rdb: rdb}
}

func (s *OAuthProviderRedisStore) SaveTransaction(ctx context.Context, id string, tx service.OAuthAuthorizationTransaction, ttl time.Duration) error {
	payload, err := json.Marshal(oauthTransactionDocument{OAuthAuthorizationTransaction: tx, State: tx.State})
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, oauthTransactionKeyPrefix+id, payload, ttl).Err()
}

func (s *OAuthProviderRedisStore) GetTransaction(ctx context.Context, id string) (service.OAuthAuthorizationTransaction, error) {
	raw, err := s.rdb.Get(ctx, oauthTransactionKeyPrefix+strings.TrimSpace(id)).Bytes()
	if err != nil {
		return service.OAuthAuthorizationTransaction{}, s.wrapNotFound(err)
	}
	return decodeOAuthTransaction(raw)
}

func (s *OAuthProviderRedisStore) ConsumeTransaction(ctx context.Context, id string) (service.OAuthAuthorizationTransaction, error) {
	raw, err := s.rdb.GetDel(ctx, oauthTransactionKeyPrefix+strings.TrimSpace(id)).Bytes()
	if err != nil {
		return service.OAuthAuthorizationTransaction{}, s.wrapNotFound(err)
	}
	return decodeOAuthTransaction(raw)
}

func decodeOAuthTransaction(raw []byte) (service.OAuthAuthorizationTransaction, error) {
	var doc oauthTransactionDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return service.OAuthAuthorizationTransaction{}, err
	}
	doc.OAuthAuthorizationTransaction.State = doc.State
	return doc.OAuthAuthorizationTransaction, nil
}

func (s *OAuthProviderRedisStore) SaveAuthorizationCode(ctx context.Context, code service.OAuthStoredAuthorizationCode, ttl time.Duration) error {
	payload, err := json.Marshal(code)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, oauthCodeKeyPrefix+code.CodeHash, payload, ttl).Err()
}

func (s *OAuthProviderRedisStore) ConsumeAuthorizationCode(ctx context.Context, codeHash string) (service.OAuthStoredAuthorizationCode, error) {
	raw, err := s.rdb.GetDel(ctx, oauthCodeKeyPrefix+codeHash).Bytes()
	if err != nil {
		return service.OAuthStoredAuthorizationCode{}, s.wrapNotFound(err)
	}
	var stored service.OAuthStoredAuthorizationCode
	if err := json.Unmarshal(raw, &stored); err != nil {
		return service.OAuthStoredAuthorizationCode{}, err
	}
	return stored, nil
}

func (s *OAuthProviderRedisStore) ConsumeRefreshToken(ctx context.Context, tokenHash string) (service.OAuthStoredToken, error) {
	raw, err := s.rdb.GetDel(ctx, oauthRefreshKeyPrefix+tokenHash).Bytes()
	if err != nil {
		return service.OAuthStoredToken{}, s.wrapNotFound(err)
	}
	var stored service.OAuthStoredToken
	if err := json.Unmarshal(raw, &stored); err != nil {
		return service.OAuthStoredToken{}, err
	}
	return stored, nil
}

func (s *OAuthProviderRedisStore) SaveTokenPair(ctx context.Context, accessHash string, access service.OAuthStoredToken, accessTTL time.Duration, refreshHash string, refresh service.OAuthStoredToken, refreshTTL time.Duration) error {
	accessPayload, err := json.Marshal(access)
	if err != nil {
		return err
	}
	refreshPayload, err := json.Marshal(refresh)
	if err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, oauthAccessKeyPrefix+accessHash, accessPayload, accessTTL).Err(); err != nil {
		return err
	}
	return s.rdb.Set(ctx, oauthRefreshKeyPrefix+refreshHash, refreshPayload, refreshTTL).Err()
}

func (s *OAuthProviderRedisStore) GetAccessToken(ctx context.Context, tokenHash string) (service.OAuthStoredToken, error) {
	raw, err := s.rdb.Get(ctx, oauthAccessKeyPrefix+tokenHash).Bytes()
	if err != nil {
		return service.OAuthStoredToken{}, s.wrapNotFound(err)
	}
	var stored service.OAuthStoredToken
	if err := json.Unmarshal(raw, &stored); err != nil {
		return service.OAuthStoredToken{}, err
	}
	return stored, nil
}

func (s *OAuthProviderRedisStore) DeleteTokens(ctx context.Context, accessHash, refreshHash string) error {
	return s.rdb.Del(ctx, oauthAccessKeyPrefix+accessHash, oauthRefreshKeyPrefix+refreshHash).Err()
}

// RevokeUserTokens 级联清除指定用户在指定客户端下的所有存续令牌，
// 使"断开连接"立即生效而不必等访问令牌自然过期。
func (s *OAuthProviderRedisStore) RevokeUserTokens(ctx context.Context, clientID string, userID int64) error {
	for _, pattern := range []string{oauthAccessKeyPrefix + "*", oauthRefreshKeyPrefix + "*"} {
		keys, err := s.rdb.Keys(ctx, pattern).Result()
		if err != nil {
			return err
		}
		for _, key := range keys {
			raw, getErr := s.rdb.Get(ctx, key).Bytes()
			if getErr != nil {
				continue
			}
			var token service.OAuthStoredToken
			if json.Unmarshal(raw, &token) == nil && token.UserID == userID && token.ClientID == clientID {
				_ = s.rdb.Del(ctx, key)
			}
		}
	}
	return nil
}

func (s *OAuthProviderRedisStore) wrapNotFound(err error) error {
	if errors.Is(err, redis.Nil) {
		return fmt.Errorf("%w: %v", ErrOAuthStoreNotFound, err)
	}
	return err
}
