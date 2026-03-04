package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

type snapshotCacheEntry struct {
	ETag      string
	Payload   any
	ExpiresAt time.Time
REDACTED

type snapshotCache struct {
	mu    sync.RWMutex
	ttl   time.Duration
	items map[string]snapshotCacheEntry
REDACTED

func newSnapshotCache(ttl time.Duration) *snapshotCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
REDACTED
	return &snapshotCache{
		ttl:   ttl,
		items: make(map[string]snapshotCacheEntry),
REDACTED
REDACTED

func (c *snapshotCache) Get(key string) (snapshotCacheEntry, bool) {
	if c == nil || key == "" {
		return snapshotCacheEntry{REDACTED, false
REDACTED
	now := time.Now()

	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return snapshotCacheEntry{REDACTED, false
REDACTED
	if now.After(entry.ExpiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return snapshotCacheEntry{REDACTED, false
REDACTED
	return entry, true
REDACTED

func (c *snapshotCache) Set(key string, payload any) snapshotCacheEntry {
	if c == nil {
		return snapshotCacheEntry{REDACTED
REDACTED
	entry := snapshotCacheEntry{
		ETag:      buildETagFromAny(payload),
		Payload:   payload,
		ExpiresAt: time.Now().Add(c.ttl),
REDACTED
	if key == "" {
		return entry
REDACTED
	c.mu.Lock()
	c.items[key] = entry
	c.mu.Unlock()
	return entry
REDACTED

func buildETagFromAny(payload any) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
REDACTED
	sum := sha256.Sum256(raw)
	return "\"" + hex.EncodeToString(sum[:]) + "\""
REDACTED

func parseBoolQueryWithDefault(raw string, def bool) bool {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return def
REDACTED
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
REDACTED
REDACTED
