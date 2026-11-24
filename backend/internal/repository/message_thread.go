package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

var _ service.MessageThreadStore = (*gatewayCache)(nil)

// Message thread bindings live in the shared Redis so every replica routes a
// continue request to the account that created the thread.
//
//	gateway:claude:thread:v1:{scope}:b:sha256(response id)   binding
//
// Scope includes the user, API key and group.
const (
	messageThreadKeyPrefix = "gateway:claude:thread:v1:"

	// messageThreadConflict replaces a binding once a second, different binding
	// was offered for the same response id. The id then names no route.
	messageThreadConflict = "conflict"
)

var (
	errMessageThreadConflict = errors.New("message thread response id is bound to another route")
	errMessageThreadDamaged  = errors.New("message thread binding holds a value this version does not write")
)

// bindMessageThreadScript binds one response id atomically.
//
// A response id is bound once. Writing the same binding again is a no-op that
// does not extend its life. A different binding for the same id means the id
// can no longer be trusted to name one route: it is replaced by the conflict
// marker for a full TTL, which outlasts every binding involved, so neither the
// first holder nor a later writer can have it routed anywhere in that time.
var bindMessageThreadScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current == false then
  redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
  return 1
end
if current == ARGV[1] then
  return 1
end
redis.call('SET', KEYS[1], ARGV[3], 'PX', ARGV[2])
return 0
`)

func messageThreadBindingKey(scope, messageID string) string {
	sum := sha256.Sum256([]byte(messageID))
	return messageThreadKeyPrefix + "{" + scope + "}:b:" + hex.EncodeToString(sum[:])
}

// ReadMessageThread returns (nil, nil) when the response id has no binding or
// was invalidated by a conflict. A binding that exists but cannot be read back
// whole (a field missing, a value of the wrong shape, another version) is an error, never a miss and never a binding: the caller
// must not treat a fault as an ordinary expiry, nor route by half a record.
func (c *gatewayCache) ReadMessageThread(ctx context.Context, scope, messageID string) (*service.MessageThreadBinding, error) {
	raw, err := c.rdb.Get(ctx, messageThreadBindingKey(scope, messageID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if string(raw) == messageThreadConflict {
		return nil, nil
	}
	// The record is checked here, where it enters the process: the version
	// this code writes, with every field it writes.
	binding, err := service.DecodeMessageThreadBinding(raw)
	if err != nil {
		return nil, err
	}
	if !binding.Valid() {
		return nil, errMessageThreadDamaged
	}
	return binding, nil
}

// BindMessageThread stores the binding of one response id, see
// bindMessageThreadScript. Nothing is deleted on any error path.
func (c *gatewayCache) BindMessageThread(ctx context.Context, scope, messageID string, binding service.MessageThreadBinding) error {
	raw, err := service.EncodeMessageThreadBinding(binding)
	if err != nil {
		return err
	}
	keys := []string{messageThreadBindingKey(scope, messageID)}
	stored, err := bindMessageThreadScript.Run(ctx, c.rdb, keys, string(raw), service.MessageThreadTTL.Milliseconds(), messageThreadConflict).Int()
	if err != nil {
		return err
	}
	if stored != 1 {
		return errMessageThreadConflict
	}
	return nil
}
