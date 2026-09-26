package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const openAIFirstServeMaxReplay = 8 << 20

// Configuration is account scoped. Conversation history remains local to each
// WebSocket connection; HTTP routing affinity can be shared explicitly.
func (a *Account) IsOpenAIFirstServe() bool {
	return a != nil && a.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeOff) == OpenAIWSIngressModeFirstServe
}

func validateOpenAIFirstServe(account *Account) error {
	if !account.IsOpenAIFirstServe() {
		return nil
	}
	_, err := account.firstServeConfig()
	return err
}

// Imports can be saved before a proxy group is configured; dispatch must never
// silently use a direct connection while first serve is enabled.
func validateOpenAIFirstServeRouting(account *Account) error {
	if !account.IsOpenAIFirstServe() {
		return nil
	}
	if account.ProxyGroupID == nil || *account.ProxyGroupID <= 0 {
		return infraerrors.BadRequest("FIRST_SERVE_PROXY_GROUP_REQUIRED", fmt.Sprintf("账号 %s：首服模式需要代理组，请在账号编辑中选择至少两个不同出口的代理。", account.Name))
	}
	return validateOpenAIFirstServe(account)
}

// A request summary contains usage and timing only, never request/response content.
type OpenAIFirstServeRequestStatus struct {
	Kind         string `json:"kind"`
	Outcome      string `json:"outcome"`
	RequestID    string `json:"request_id,omitempty"`
	InputTokens  *int   `json:"input_tokens"`
	OutputTokens *int   `json:"output_tokens"`
	DurationMs   int64  `json:"duration_ms"`
}

type OpenAIFirstServeStatus struct {
	Requests       int                            `json:"requests"`
	LastRequest    *OpenAIFirstServeRequestStatus `json:"last_request,omitempty"`
	Transport      string                         `json:"transport"`
	SessionMissing bool                           `json:"session_missing,omitempty"`
	ID             string                         `json:"id"`
	AccountID      int64                          `json:"account_id"`
	ProxyID        int64                          `json:"proxy_id"`
	ProxyName      string                         `json:"proxy_name"`
	ConnID         string                         `json:"conn_id"`
	ExpiresAt      time.Time                      `json:"expires_at"`
	UpdatedAt      time.Time                      `json:"updated_at"`
	FirstTokenMs   *int                           `json:"first_token_ms"`
	Rotations      int                            `json:"rotations"`
	Reason         string                         `json:"reason"`
	Active         bool                           `json:"active"`
	Config         OpenAIFirstServeConfig         `json:"config"`
}

// Only diagnostic snapshots are global. No credentials or conversation data
// leave the connection; the bounded registry is local to this gateway node.
var openAIFirstServeStatuses = struct {
	sync.Mutex
	items map[string]OpenAIFirstServeStatus
}{items: make(map[string]OpenAIFirstServeStatus)}

func GetOpenAIFirstServeStatuses(accountID int64) []OpenAIFirstServeStatus {
	openAIFirstServeStatuses.Lock()
	defer openAIFirstServeStatuses.Unlock()
	result := make([]OpenAIFirstServeStatus, 0)
	for id, status := range openAIFirstServeStatuses.items {
		if time.Since(status.UpdatedAt) > time.Hour && (status.Transport != "http" || time.Now().After(status.ExpiresAt)) {
			delete(openAIFirstServeStatuses.items, id)
			continue
		}
		if status.AccountID == accountID {
			result = append(result, status)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

type openAIFirstServeState struct {
	scope       string
	status      OpenAIFirstServeStatus
	proxy       *Proxy
	fingerprint string         // physical WS handshake identity; shared routing owns its lifetime
	uses        *atomic.Uint64 // per combination; leases retain it across rotations
	lastID      string
	history     []json.RawMessage
	complete    bool
	current     []json.RawMessage
	currentFull bool
}

func newOpenAIFirstServeState(account *Account, connID string, now time.Time) *openAIFirstServeState {
	cfg, err := account.firstServeConfig()
	if err != nil {
		cfg = defaultOpenAIFirstServeConfig()
	}
	s := &openAIFirstServeState{status: OpenAIFirstServeStatus{ID: uuid.NewString(), AccountID: account.ID, Config: cfg, Transport: "ws"}}
	s.bind(account.Proxy, connID, now)
	return s
}

func (s *openAIFirstServeState) publish(reason string, now time.Time) {
	s.status.Reason = reason
	s.status.UpdatedAt = now
	openAIFirstServeStatuses.Lock()
	defer openAIFirstServeStatuses.Unlock()
	if s.status.Requests == 0 {
		delete(openAIFirstServeStatuses.items, s.status.ID)
		return
	}
	if len(openAIFirstServeStatuses.items) >= 4096 {
		oldestID := ""
		oldest := now
		for id, item := range openAIFirstServeStatuses.items {
			if item.UpdatedAt.Before(oldest) {
				oldestID, oldest = id, item.UpdatedAt
			}
		}
		delete(openAIFirstServeStatuses.items, oldestID)
	}
	openAIFirstServeStatuses.items[s.status.ID] = s.status
}

func (s *openAIFirstServeState) bind(proxy *Proxy, connID string, now time.Time) {
	s.proxy = proxy
	s.uses = &atomic.Uint64{}
	s.status.ConnID = connID
	s.status.Active = true
	s.status.ExpiresAt = now.Add(s.status.Config.ttl())
	s.status.FirstTokenMs = nil
	s.status.Requests = 0
	s.status.LastRequest = nil
	if proxy != nil {
		s.status.ProxyID, s.status.ProxyName = proxy.ID, proxy.Name
	}
	s.publish("observing", now)
}

// rotate changes only the selected proxy. The routing/session ID remains
// stable so upstream requests continue using the same first-serve identity.
func (s *openAIFirstServeState) rotate(proxy *Proxy, now time.Time) {
	s.bind(proxy, s.status.ConnID, now)
}

func (s *openAIFirstServeState) observe(ms int, now time.Time) {
	s.status.FirstTokenMs = &ms
	s.publish("ready", now)
}

func (s *openAIFirstServeState) due(now time.Time) bool {
	return !now.Before(s.status.ExpiresAt)
}

func (s *openAIFirstServeState) input(payload []byte) {
	prev := openAIWSPayloadStringFromRaw(payload, "previous_response_id")
	items, exists, err := openAIWSExtractNormalizedInputSequence(payload)
	s.currentFull = err == nil && exists && (prev == "" || (prev == s.lastID && s.complete))
	for i, item := range items {
		if gjson.ParseBytes(item).Type == gjson.String {
			items[i], _ = json.Marshal(map[string]any{"role": "user", "content": gjson.ParseBytes(item).String()})
		}
	}
	// A partial overlap may be a rewritten full history rather than a delta.
	// Do not silently duplicate it when rebuilding a chain.
	if prev != "" && prev == s.lastID && s.complete && len(s.history) > 0 &&
		openAIWSRawItemsHasPrefix(items, s.history[:1]) && !openAIWSRawItemsHasPrefix(items, s.history) {
		s.currentFull = false
	}
	s.current, _ = buildOpenAIWSReplayInputSequenceFromItems(s.history, s.complete && prev == s.lastID, items, exists, prev != "")
	if replaySize(s.current) > openAIFirstServeMaxReplay {
		s.current, s.currentFull = nil, false
	}
}

func replaySize(items []json.RawMessage) int {
	size := 0
	for _, item := range items {
		size += len(item)
	}
	return size
}

func (s *openAIFirstServeState) finish(id string, output []json.RawMessage, complete bool) {
	s.lastID = id
	s.complete = s.currentFull && complete
	s.history = combineOpenAIWSReplayItems(s.current, output)
	if !s.complete || replaySize(s.history) > openAIFirstServeMaxReplay {
		s.history, s.complete = nil, false
	}
	s.current = nil
}

func (s *openAIFirstServeState) replay(payload []byte) ([]byte, bool) {
	s.input(payload)
	if !s.currentFull {
		return nil, false
	}
	for _, item := range s.current {
		itemType := gjson.GetBytes(item, "type").String()
		if itemType == "item_reference" || (itemType == "reasoning" && gjson.GetBytes(item, "encrypted_content").String() == "") {
			return nil, false
		}
	}
	if openAIWSRawItemsHasFunctionCallOutput(s.current) && !openAIWSRawItemsHaveToolCallContextForOutputs(s.current) {
		return nil, false
	}
	updated, _, err := dropPreviousResponseIDFromRawPayload(payload)
	if err != nil {
		return nil, false
	}
	updated, err = setOpenAIWSPayloadInputSequence(updated, s.current, true)
	return updated, err == nil
}

// Selecting another proxy must exclude the old node at the database query,
// rather than repeatedly drawing the same node from a small random pool.
type firstServeProxySelector interface {
	SelectNextProxy(context.Context, int64, int64, []int64) (*Proxy, error)
}

func selectOpenAIFirstServeProxy(ctx context.Context, account *Account, previousID int64) (*Proxy, error) {
	defaultProxyGroupResolver.RLock()
	selector, ok := defaultProxyGroupResolver.resolver.(firstServeProxySelector)
	defaultProxyGroupResolver.RUnlock()
	if !ok || account.ProxyGroupID == nil {
		return nil, ErrProxyGroupNoProxy
	}
	cfg, err := account.firstServeConfig()
	if err != nil {
		return nil, err
	}
	var allowedIDs []int64
	if cfg.ProxyMode == "selected" {
		allowedIDs = cfg.ProxyIDs
	}
	proxy, err := selector.SelectNextProxy(ctx, *account.ProxyGroupID, previousID, allowedIDs)
	if err != nil {
		return nil, err
	}
	if proxy == nil || !cfg.allows(proxy.ID) || proxy.ID == previousID || (previousID != 0 && account.Proxy != nil && proxy.URL() == account.Proxy.URL()) {
		return nil, ErrProxyGroupNoProxy
	}
	return proxy, nil
}
