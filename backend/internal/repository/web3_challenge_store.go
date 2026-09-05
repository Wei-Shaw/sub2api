package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const web3ChallengeKeyPrefix = "web3:challenge:"

type web3ChallengeStore struct {
	redis *redis.Client
}

func NewWeb3ChallengeStore(redisClient *redis.Client) service.Web3ChallengeStore {
	return &web3ChallengeStore{redis: redisClient}
}

func (s *web3ChallengeStore) Save(ctx context.Context, tokenHash string, challenge service.Web3Challenge, ttl time.Duration) error {
	payload, err := json.Marshal(challenge)
	if err != nil {
		return fmt.Errorf("encode web3 challenge: %w", err)
	}
	created, err := s.redis.SetNX(ctx, web3ChallengeKeyPrefix+tokenHash, payload, ttl).Result()
	if err != nil {
		return fmt.Errorf("store web3 challenge: %w", err)
	}
	if !created {
		return fmt.Errorf("web3 challenge token collision")
	}
	return nil
}

func (s *web3ChallengeStore) Get(ctx context.Context, tokenHash string) (*service.Web3Challenge, error) {
	payload, err := s.redis.Get(ctx, web3ChallengeKeyPrefix+tokenHash).Bytes()
	if err == redis.Nil {
		return nil, service.ErrWeb3ChallengeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get web3 challenge: %w", err)
	}
	var challenge service.Web3Challenge
	if err := json.Unmarshal(payload, &challenge); err != nil {
		return nil, fmt.Errorf("decode web3 challenge: %w", err)
	}
	return &challenge, nil
}

func (s *web3ChallengeStore) Consume(ctx context.Context, tokenHash, expectedDigest string) (bool, error) {
	result, err := redis.NewScript(`
		local value = redis.call('GET', KEYS[1])
		if not value then
			return 0
		end
		local decoded = cjson.decode(value)
		if decoded.message_digest ~= ARGV[1] then
			return -1
		end
		redis.call('DEL', KEYS[1])
		return 1
	`).Run(ctx, s.redis, []string{web3ChallengeKeyPrefix + tokenHash}, expectedDigest).Int()
	if err != nil {
		return false, fmt.Errorf("consume web3 challenge: %w", err)
	}
	return result == 1, nil
}

func (s *web3ChallengeStore) RecordFailure(ctx context.Context, tokenHash string) error {
	_, err := redis.NewScript(`
		local value = redis.call('GET', KEYS[1])
		if not value then
			return 0
		end
		local ttl = redis.call('PTTL', KEYS[1])
		local decoded = cjson.decode(value)
		decoded.failed_attempts = (decoded.failed_attempts or 0) + 1
		if decoded.failed_attempts >= 3 then
			redis.call('DEL', KEYS[1])
			return decoded.failed_attempts
		end
		redis.call('SET', KEYS[1], cjson.encode(decoded), 'PX', ttl)
		return decoded.failed_attempts
	`).Run(ctx, s.redis, []string{web3ChallengeKeyPrefix + tokenHash}).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("record web3 challenge failure: %w", err)
	}
	return nil
}
