package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const proxyLatencyKeyPrefix = "proxy:latency:"

func proxyLatencyKey(proxyID int64) string {
	return fmt.Sprintf("%s%d", proxyLatencyKeyPrefix, proxyID)
REDACTED

type proxyLatencyCache struct {
	rdb *redis.Client
REDACTED

func NewProxyLatencyCache(rdb *redis.Client) service.ProxyLatencyCache {
	return &proxyLatencyCache{rdb: rdbREDACTED
REDACTED

func (c *proxyLatencyCache) GetProxyLatencies(ctx context.Context, proxyIDs []int64) (map[int64]*service.ProxyLatencyInfo, error) {
	results := make(map[int64]*service.ProxyLatencyInfo)
	if len(proxyIDs) == 0 {
		return results, nil
REDACTED

	keys := make([]string, 0, len(proxyIDs))
	for _, id := range proxyIDs {
		keys = append(keys, proxyLatencyKey(id))
REDACTED

	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return results, err
REDACTED

	for i, raw := range values {
		if raw == nil {
			continue
	REDACTED
		var payload []byte
		switch v := raw.(type) {
		case string:
			payload = []byte(v)
		case []byte:
			payload = v
		default:
			continue
	REDACTED
		var info service.ProxyLatencyInfo
		if err := json.Unmarshal(payload, &info); err != nil {
			continue
	REDACTED
		results[proxyIDs[i]] = &info
REDACTED

	return results, nil
REDACTED

func (c *proxyLatencyCache) SetProxyLatency(ctx context.Context, proxyID int64, info *service.ProxyLatencyInfo) error {
	if info == nil {
		return nil
REDACTED
	payload, err := json.Marshal(info)
	if err != nil {
		return err
REDACTED
	return c.rdb.Set(ctx, proxyLatencyKey(proxyID), payload, 0).Err()
REDACTED
