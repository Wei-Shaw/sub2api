package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// qoderSessionMapTTL is how long a conversation-prefix → session binding is
// retained. A turn extends the binding, so idle conversations expire naturally.
const qoderSessionMapTTL = time.Hour

// qoderSessionState is the value stored per conversation-prefix key: the Qoder
// session id plus the last SSE event id, used to replay only new events on the
// next turn via the Last-Event-ID header.
type qoderSessionState struct {
	SessionID   string `json:"session_id"`
	LastEventID string `json:"last_event_id,omitempty"`
}

// qoderConversationKey derives a stable Redis key from the account and the
// conversation transcript. Callers pass the full ordered list of (role, text)
// turns that should map to a given session. Using messages[:-1] before a turn
// looks up the session established by the previous turn; using the full
// transcript (including the new assistant reply) after a turn stores the
// binding for the next lookup — this is the "session stitching" that lets a
// stateless Chat Completions client resume a stateful Qoder session.
func qoderConversationKey(accountID int64, model, system string, turns []qoderTurn) string {
	h := sha256.New()
	h.Write([]byte(model))
	h.Write([]byte{0})
	h.Write([]byte(system))
	for _, t := range turns {
		h.Write([]byte{0})
		h.Write([]byte(t.Role))
		h.Write([]byte{1})
		h.Write([]byte(t.Text))
	}
	sum := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("qoder:conv:%d:%s", accountID, sum)
}

// lookupQoderSession returns the session state bound to the given conversation
// prefix, or (nil, nil) on a miss. A nil redis client always misses.
func (s *QoderGatewayService) lookupQoderSession(ctx context.Context, key string) (*qoderSessionState, error) {
	if s.redis == nil {
		return nil, nil
	}
	raw, err := s.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state qoderSessionState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil, nil
	}
	if strings.TrimSpace(state.SessionID) == "" {
		return nil, nil
	}
	return &state, nil
}

// storeQoderSession binds a conversation prefix to a session state with the
// standard TTL. Failures are logged and swallowed: a lost binding only forces a
// new session on the next turn, never a request failure.
func (s *QoderGatewayService) storeQoderSession(ctx context.Context, key string, state qoderSessionState) {
	if s.redis == nil {
		return
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := s.redis.Set(ctx, key, payload, qoderSessionMapTTL).Err(); err != nil {
		logger.L().With(zap.String("component", "service.qoder_gateway")).
			Warn("qoder.session_map_store_failed", zap.String("key", key), zap.Error(err))
	}
}
