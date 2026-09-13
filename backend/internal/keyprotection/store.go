package keyprotection

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Scope is built exclusively from authenticated server-side records. GroupID
// additionally prevents a key moved between groups reusing its previous state.
type Scope struct{ UserID, APIKeyID, GroupID int64 }

var (
	ErrStore   = errors.New("key protection mapping storage unavailable")
	ErrExpired = errors.New("key protection continuation mapping missing or expired; resend full history")
)

const storePrefix = "key-protection:v1:"

// JSON string escapes can expand mapping bytes by at most six times. Include
// bounded room for token names and object delimiters at the maximum entry cap.
const maxMappingCiphertextBytes = 6*MaxMappingBytes + (2 << 20)

// RedisStore holds only purpose-derived encryption material, never mappings.
// Mapping values are AES-256-GCM ciphertext; the Redis key is authenticated as
// additional data so copying ciphertext to another scope cannot grant access.
type RedisStore struct {
	client   *redis.Client
	aead     cipher.AEAD
	tokenKey []byte
}

func (s RedisStore) String() string   { return "key protection encrypted store" }
func (s RedisStore) GoString() string { return s.String() }

func NewRedisStore(client *redis.Client, masterHex string) *RedisStore {
	s := &RedisStore{client: client}
	master, err := hex.DecodeString(masterHex)
	if err != nil || len(master) != 32 {
		return s
	}
	derive := hmac.New(sha256.New, master)
	_, _ = derive.Write([]byte("sub2api/automatic-key-protection/mappings/aes-gcm/v1"))
	block, err := aes.NewCipher(derive.Sum(nil))
	if err != nil {
		return s
	}
	s.aead, _ = cipher.NewGCM(block)
	derive.Reset()
	_, _ = derive.Write([]byte("sub2api/automatic-key-protection/placeholders/hmac-sha256/v1"))
	s.tokenKey = derive.Sum(nil)
	return s
}

// Seed gives stable keyed identifiers across workers/restarts. Client-supplied
// session strings are subordinate to authenticated owner IDs. No-session full
// history clients share deterministic tokens within their own API key scope.
func (s *RedisStore) Seed(scope Scope, session string) ([]byte, error) {
	if s == nil || len(s.tokenKey) != 32 {
		return nil, ErrStore
	}
	prefix, err := scopePrefix(scope)
	if err != nil {
		return nil, err
	}
	derive := hmac.New(sha256.New, s.tokenKey)
	_, _ = derive.Write([]byte(prefix + "\x00" + session))
	return derive.Sum(nil), nil
}

func (s *RedisStore) Ready(ctx context.Context) error {
	if s == nil || s.client == nil || s.aead == nil {
		return ErrStore
	}
	if s.client.Ping(ctx).Err() != nil {
		return ErrStore
	}
	return nil
}

func scopePrefix(scope Scope) (string, error) {
	if scope.UserID <= 0 || scope.APIKeyID <= 0 {
		return "", ErrStore
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d", scope.UserID, scope.APIKeyID, scope.GroupID)))
	return storePrefix + hex.EncodeToString(h[:]) + ":", nil
}

func mappingKey(prefix, kind, identifier string) string {
	h := sha256.Sum256([]byte(identifier))
	return prefix + kind + ":" + hex.EncodeToString(h[:])
}

func (s *RedisStore) seal(key string, entries map[string]string) ([]byte, error) {
	data, err := json.Marshal(entries)
	if err != nil || len(data) > maxMappingCiphertextBytes-64 {
		return nil, ErrStore
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, ErrStore
	}
	return s.aead.Seal(nonce, nonce, data, []byte(key)), nil
}

func (s *RedisStore) open(key string, ciphertext []byte) (map[string]string, error) {
	n := s.aead.NonceSize()
	if len(ciphertext) < n || len(ciphertext) > maxMappingCiphertextBytes {
		return nil, ErrStore
	}
	data, err := s.aead.Open(nil, ciphertext[:n], ciphertext[n:], []byte(key))
	if err != nil {
		return nil, ErrStore
	}
	var entries map[string]string
	if json.Unmarshal(data, &entries) != nil || entries == nil {
		return nil, ErrStore
	}
	return entries, nil
}

func (s *RedisStore) load(ctx context.Context, key string) (map[string]string, error) {
	data, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrExpired
	}
	if err != nil {
		return nil, ErrStore
	}
	return s.open(key, data)
}

// Protect atomically scans with the latest session map. Optimistic conflicts
// rerun only the local deterministic scan, never an upstream request. Response
// references are immutable snapshots, scoped to the same authenticated owner.
func (s *RedisStore) Protect(ctx context.Context, scope Scope, session, previous string, cfg Config, body []byte, protocol string) ([]byte, *State, error) {
	cfg = cfg.Normalized()
	if err := s.Ready(ctx); err != nil {
		return nil, nil, err
	}
	seed, err := s.Seed(scope, session)
	if err != nil {
		return nil, nil, err
	}
	prefix, err := scopePrefix(scope)
	if err != nil {
		return nil, nil, err
	}
	prior := map[string]string{}
	if previous != "" {
		prior, err = s.load(ctx, mappingKey(prefix, "response", previous))
		if err != nil {
			return nil, nil, err
		}
	}
	if session == "" {
		state, err := NewStateWithSeed(cfg, prior, seed)
		if err != nil {
			return nil, nil, err
		}
		out, err := state.ProtectJSON(body, protocol)
		return out, state, err
	}
	key := mappingKey(prefix, "session", session)
	index := prefix + "index"
	var out []byte
	var state *State
	for attempt := 0; attempt < 12; attempt++ {
		err = s.client.Watch(ctx, func(tx *redis.Tx) error {
			entries := make(map[string]string, len(prior))
			for k, v := range prior {
				entries[k] = v
			}
			data, readErr := tx.Get(ctx, key).Bytes()
			if readErr == nil {
				current, e := s.open(key, data)
				if e != nil {
					return e
				}
				for k, v := range current {
					if old, ok := entries[k]; ok && old != v {
						return ErrStore
					}
					entries[k] = v
				}
			} else if !errors.Is(readErr, redis.Nil) {
				return ErrStore
			}
			var e error
			state, e = NewStateWithSeed(cfg, entries, seed)
			if e != nil {
				return e
			}
			out, e = state.ProtectJSON(body, protocol)
			if e != nil {
				return e
			}
			return s.saveTransaction(ctx, tx, index, key, state.Entries(), cfg)
		}, key, index)
		if !errors.Is(err, redis.TxFailedErr) {
			return out, state, safeStoreError(err)
		}
	}
	return nil, nil, ErrStore
}

func safeStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrCapacity) || errors.Is(err, ErrExpired) {
		return err
	}
	if errors.Is(err, ErrContent) || errors.Is(err, ErrUnsupported) {
		return err
	}
	// Core errors are already safe; all callers display a fixed failure message.
	return ErrStore
}

func (s *RedisStore) saveTransaction(ctx context.Context, tx *redis.Tx, index, key string, entries map[string]string, cfg Config) error {
	if len(entries) > cfg.MaxMappings {
		return ErrCapacity
	}
	now := time.Now().UnixMilli()
	count, err := tx.ZCount(ctx, index, strconv.FormatInt(now+1, 10), "+inf").Result()
	if err != nil {
		return ErrStore
	}
	expiry, err := tx.ZScore(ctx, index, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return ErrStore
	}
	if expiry <= float64(now) && count >= int64(cfg.MaxSessions) {
		return ErrCapacity
	}
	sealed, err := s.seal(key, entries)
	if err != nil {
		return err
	}
	ttl := time.Duration(cfg.TTLSeconds) * time.Second
	_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.ZRemRangeByScore(ctx, index, "-inf", strconv.FormatInt(now, 10))
		pipe.Set(ctx, key, sealed, ttl)
		pipe.ZAdd(ctx, index, redis.Z{Score: float64(now + ttl.Milliseconds()), Member: key})
		// The index contains only opaque IDs. Keep it at least as long as any
		// allowed mapping even when an administrator shortens the current TTL.
		pipe.Expire(ctx, index, 31*24*time.Hour)
		return nil
	})
	return err
}

// SaveResponse is called before the response ID is exposed to the client.
func (s *RedisStore) SaveResponse(ctx context.Context, scope Scope, responseID string, state *State) error {
	if responseID == "" || len(responseID) > 512 {
		return ErrStore
	}
	if s == nil || s.client == nil || s.aead == nil {
		return ErrStore
	}
	prefix, err := scopePrefix(scope)
	if err != nil {
		return err
	}
	key, index := mappingKey(prefix, "response", responseID), prefix+"index"
	for attempt := 0; attempt < 12; attempt++ {
		err = s.client.Watch(ctx, func(tx *redis.Tx) error {
			// An existing reference may only be reused with identical mappings.
			data, e := tx.Get(ctx, key).Bytes()
			if e == nil {
				old, e := s.open(key, data)
				if e != nil {
					return e
				}
				entries := state.Entries()
				if len(old) != len(entries) {
					return ErrStore
				}
				for k, v := range old {
					if entries[k] != v {
						return ErrStore
					}
				}
				return nil
			}
			if !errors.Is(e, redis.Nil) {
				return ErrStore
			}
			return s.saveTransaction(ctx, tx, index, key, state.Entries(), state.Config())
		}, key, index)
		if !errors.Is(err, redis.TxFailedErr) {
			return safeStoreError(err)
		}
	}
	return ErrStore
}

// Purge removes this feature's encrypted state only. In-flight requests retain
// their request-local snapshot; administrators should disable first to revoke
// future continuation and then purge again after active requests drain.
func (s *RedisStore) Purge(ctx context.Context) error {
	if s == nil || s.client == nil {
		return ErrStore
	}
	var cursor uint64
	for {
		keys, next, err := s.client.Scan(ctx, cursor, storePrefix+"*", 200).Result()
		if err != nil {
			return ErrStore
		}
		if len(keys) > 0 {
			if s.client.Del(ctx, keys...).Err() != nil {
				return ErrStore
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}
