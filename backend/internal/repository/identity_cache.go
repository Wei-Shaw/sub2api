package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"sub2api/internal/service/ports"

	"github.com/redis/go-redis/v9"
)

const (
	fingerprintKeyPrefix = "fingerprint:"
	fingerprintTTL       = 24 * time.Hour
)

type identityCache struct {
	rdb *redis.Client
REDACTED

func NewIdentityCache(rdb *redis.Client) ports.IdentityCache {
	return &identityCache{rdb: rdbREDACTED
REDACTED

func (c *identityCache) GetFingerprint(ctx context.Context, accountID int64) (*ports.Fingerprint, error) {
	key := fmt.Sprintf("%s%d", fingerprintKeyPrefix, accountID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
REDACTED
	var fp ports.Fingerprint
	if err := json.Unmarshal([]byte(val), &fp); err != nil {
		return nil, err
REDACTED
	return &fp, nil
REDACTED

func (c *identityCache) SetFingerprint(ctx context.Context, accountID int64, fp *ports.Fingerprint) error {
	key := fmt.Sprintf("%s%d", fingerprintKeyPrefix, accountID)
	val, err := json.Marshal(fp)
	if err != nil {
		return err
REDACTED
	return c.rdb.Set(ctx, key, val, fingerprintTTL).Err()
REDACTED
