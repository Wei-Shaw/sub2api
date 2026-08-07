package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/qoder"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	qoderJTCachePrefix     = "qoder:jt:"
	qoderJTLockPrefix      = "qoder:jt:lock:"
	qoderJTLockTTL         = 30 * time.Second
	qoderJTLockPollWait    = 10 * time.Second
	qoderJTLockPollEvery   = 500 * time.Millisecond
	qoderJTSafetyMargin    = 5 * time.Minute
	qoderJTDefaultLifetime = 23 * time.Hour
)

type QoderTokenProvider struct {
	redis        *redis.Client
	httpUpstream HTTPUpstream
}

func NewQoderTokenProvider(redisClient *redis.Client, httpUpstream HTTPUpstream) *QoderTokenProvider {
	return &QoderTokenProvider{
		redis:        redisClient,
		httpUpstream: httpUpstream,
	}
}

func (p *QoderTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	pat := account.GetQoderApiKey()
	if pat == "" {
		return "", fmt.Errorf("qoder direct: account %d missing api_key", account.ID)
	}

	hash := qoderPATHash(pat)
	cacheKey := qoderJTCachePrefix + hash

	if p.redis != nil {
		if cached, err := p.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			return cached, nil
		}
	}

	lockKey := qoderJTLockPrefix + hash
	if p.redis != nil {
		acquired, err := p.redis.SetNX(ctx, lockKey, "1", qoderJTLockTTL).Result()
		if err == nil && !acquired {
			token, waitErr := p.waitForKey(ctx, cacheKey)
			if waitErr == nil {
				return token, nil
			}
		}
		if acquired {
			defer p.redis.Del(context.Background(), lockKey)
		}
	}

	token, ttl, err := p.exchange(ctx, account, pat)
	if err != nil {
		return "", err
	}

	if p.redis != nil {
		if setErr := p.redis.Set(ctx, cacheKey, token, ttl).Err(); setErr != nil {
			logger.L().Warn("qoder direct: failed to cache job token",
				zap.Int64("account_id", account.ID),
				zap.Error(setErr),
			)
		}
	}
	return token, nil
}

func (p *QoderTokenProvider) exchange(ctx context.Context, account *Account, pat string) (string, time.Duration, error) {
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	doer := func(req *http.Request) (*http.Response, error) {
		return p.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	}

	resp, err := qoder.ExchangeJobToken(ctx, doer, pat)
	if err != nil {
		return "", 0, fmt.Errorf("qoder direct: token exchange failed for account %d: %w", account.ID, err)
	}

	ttl := qoderJTDefaultLifetime
	if resp.ExpiresIn > 0 {
		ttl = time.Duration(resp.ExpiresIn)*time.Millisecond - qoderJTSafetyMargin
		if ttl < time.Minute {
			ttl = time.Minute
		}
	}
	return resp.Token, ttl, nil
}

func (p *QoderTokenProvider) waitForKey(ctx context.Context, cacheKey string) (string, error) {
	deadline := time.Now().Add(qoderJTLockPollWait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(qoderJTLockPollEvery):
		}
		if cached, err := p.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			return cached, nil
		}
	}
	return "", fmt.Errorf("qoder direct: timed out waiting for token cache")
}

func qoderPATHash(pat string) string {
	sum := sha256.Sum256([]byte(pat))
	return hex.EncodeToString(sum[:8])
}
