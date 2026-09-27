package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const openAIFirstServeHTTPLimit = 4096
const openAIFirstServeResponseLimit = 4096

// Keep IDs only; never share prompts or response content between conversations.
// An evicted/unknown ID starts a new chain after rotation.
type firstServeResponses struct {
	ids   map[string]bool
	order []string
	next  int
}

func (r *firstServeResponses) add(id string) {
	if id == "" || r.ids[id] {
		return
	}
	if r.ids == nil {
		r.ids = make(map[string]bool)
	}
	if len(r.order) == openAIFirstServeResponseLimit {
		delete(r.ids, r.order[r.next])
		r.order[r.next] = id
		r.next = (r.next + 1) % openAIFirstServeResponseLimit
	} else {
		r.order = append(r.order, id)
	}
	r.ids[id] = true
}

// HTTP keeps routing identities only, never prompts or response history. Each
// entry has its own lock: neither proxy lookup nor a running SSE blocks others.
type openAIFirstServeHTTPRegistry struct {
	mu    sync.Mutex
	items map[string]*openAIFirstServeHTTPEntry
}

type openAIFirstServeHTTPEntry struct {
	mu              sync.Mutex
	state           *openAIFirstServeState
	responses       *firstServeResponses
	fingerprintSeed string    // renewed only when proxy selection succeeds; protected by mu
	refs            int       // protected by registry.mu
	lastUsed        time.Time // protected by registry.mu
}

type openAIFirstServeHTTPKey struct{}

type openAIFirstServeRouteOptions struct {
	fallbackScope string // stable for a native WS connection or a Live call
	webSocket     bool
}

type openAIFirstServeHTTPLease struct {
	registry          *openAIFirstServeHTTPRegistry
	entry             *openAIFirstServeHTTPEntry
	key               string
	id                string
	fingerprint       string // immutable device identity for the captured proxy generation
	accountID         int64
	conversationID    string
	shared            bool
	kind              string
	started           time.Time
	missing           bool
	fresh             bool
	resetContinuation bool                 // reject response IDs from before the captured generation
	responses         *firstServeResponses // protected by entry.mu
	turnID            string
	snapshot          *OpenAIFirstServeUsageSnapshot
	uses              *atomic.Uint64
	applied           bool // protected by entry.mu
	reused            bool // protected by entry.mu
	observed          bool // protected by entry.mu
}

func firstServeHTTPLease(ctx context.Context) *openAIFirstServeHTTPLease {
	l, _ := ctx.Value(openAIFirstServeHTTPKey{}).(*openAIFirstServeHTTPLease)
	return l
}

func (r *openAIFirstServeHTTPRegistry) acquire(key string, now time.Time) (*openAIFirstServeHTTPEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items == nil {
		r.items = make(map[string]*openAIFirstServeHTTPEntry)
	}
	entry := r.items[key]
	if entry == nil {
		oldestKey := ""
		oldest := now
		for k, item := range r.items {
			if item.refs == 0 && now.Sub(item.lastUsed) > 24*time.Hour {
				delete(r.items, k)
				continue
			}
			if item.refs == 0 && !item.lastUsed.After(oldest) {
				oldestKey, oldest = k, item.lastUsed
			}
		}
		if len(r.items) >= openAIFirstServeHTTPLimit {
			if oldestKey == "" {
				return nil, infraerrors.ServiceUnavailable("FIRST_SERVE_BUSY", "首服模式会话已满，请等待部分请求完成后重试。")
			}
			delete(r.items, oldestKey)
		}
		entry = &openAIFirstServeHTTPEntry{}
		r.items[key] = entry
	}
	entry.refs++
	entry.lastUsed = now
	return entry, nil
}

func (l *openAIFirstServeHTTPLease) release() {
	l.registry.mu.Lock()
	defer l.registry.mu.Unlock()
	l.entry.refs--
	l.entry.lastUsed = time.Now()
	if l.missing && l.entry.refs == 0 {
		delete(l.registry.items, l.key)
	}
}

func (s *OpenAIGatewayService) prepareFirstServeHTTP(ctx context.Context, c *gin.Context, account *Account, body []byte) (context.Context, *Account, *openAIFirstServeHTTPLease, error) {
	if c == nil || c.Request == nil || GetOpenAIClientTransport(c) == OpenAIClientTransportWS {
		return ctx, account, nil, nil
	}
	return s.prepareFirstServeRoute(ctx, c, account, body, openAIFirstServeRouteOptions{})
}

// HTTP requests and WebSocket turns share proxy selection and routing identity;
// WebSocket replay history remains private to its connection.
func (s *OpenAIGatewayService) prepareFirstServeRoute(ctx context.Context, c *gin.Context, account *Account, body []byte, options openAIFirstServeRouteOptions) (context.Context, *Account, *openAIFirstServeHTTPLease, error) {
	if !account.IsOpenAIFirstServe() {
		return ctx, account, nil, nil
	}
	if l := firstServeHTTPLease(ctx); l != nil && l.accountID == account.ID {
		return ctx, account, nil, nil // internal retry belongs to the same request
	}
	if err := validateOpenAIFirstServeRouting(account); err != nil {
		return ctx, account, nil, err
	}
	cfg, err := account.firstServeConfig()
	if err != nil {
		return ctx, account, nil, err
	}
	apiKeyID := getAPIKeyIDFromContext(c)
	scope, _ := resolveOpenAIWSExecutionScope(c, body, apiKeyID)
	if scope == "" {
		session := strings.TrimSpace(gjson.GetBytes(body, "client_metadata.session_id").String())
		if session == "" {
			session = promptCacheKeyFromAnthropicMetadataSession(&apicompat.AnthropicRequest{Metadata: []byte(gjson.GetBytes(body, "metadata").Raw)})
		}
		if session != "" {
			scope, _ = deriveOpenAISessionHashes(openAIWSExecutionScopeSeed(apiKeyID, "session", session, resolveOpenAIWSExecutionLane(c, body)))
		}
	}
	missing := scope == "" || apiKeyID == 0
	if missing {
		// A content hash is not a conversation identity. Do not merge unrelated
		// chats just because they use the same account/API key or initial prompt.
		scope = options.fallbackScope
		if scope == "" {
			scope = uuid.NewString()
		} else {
			missing = false
		}
	}
	groupID := getOpenAIGroupIDFromContext(c)
	key := fmt.Sprintf("%d:%d:%s:%s", account.ID, groupID, scope, cfg.key(*account.ProxyGroupID))
	shared := cfg.ReuseScope == "account"
	if shared {
		// Only routing affinity and proxy selection are shared. The independent
		// conversation ID below must not collapse tenants or conversation history.
		key = fmt.Sprintf("%d:account:%s", account.ID, cfg.key(*account.ProxyGroupID))
		missing = false
	}
	kind := "non_stream"
	if gjson.GetBytes(body, "stream").Bool() {
		kind = "stream"
	}
	if isExplicitOpenAICompactRequest(c, body) || isOpenAINativeCompactionV2(c) {
		kind = "compact"
	}
	if options.webSocket {
		kind = "ws" // native WS diagnostics are recorded by its connection state
	}
	now := time.Now()
	entry, err := s.openaiFirstServeHTTP.acquire(key, now)
	if err != nil {
		return ctx, account, nil, err
	}
	l := &openAIFirstServeHTTPLease{registry: &s.openaiFirstServeHTTP, entry: entry, key: key, accountID: account.ID, missing: missing,
		shared: shared, kind: kind, started: now}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.state == nil {
		entry.state = &openAIFirstServeState{status: OpenAIFirstServeStatus{
			ID: uuid.NewString(), AccountID: account.ID, Config: cfg, Transport: "http", SessionMissing: missing,
		}}
	}
	state := entry.state
	copyAccount := *account
	copyAccount.Proxy = state.proxy
	if state.proxy == nil || state.due(now) {
		// Rotation is requested only after three consecutive slow first-token
		// samples. A successful rotation starts a new upstream response chain.
		proxy, selectErr := selectOpenAIFirstServeProxy(ctx, &copyAccount, state.status.ProxyID)
		if selectErr != nil {
			state.publish("proxy_unavailable", now)
			if state.proxy == nil {
				l.release()
				return ctx, account, nil, selectErr
			}
		} else {
			if state.proxy != nil {
				state.status.Rotations++
				state.rotate(proxy, now)
			} else {
				state.bind(proxy, uuid.NewString(), now)
			}
			entry.responses = &firstServeResponses{}
			l.fresh = true
			entry.fingerprintSeed = newCodexFingerprintSeed()
		}
	}
	if kind == "stream" {
		state.status.Requests++
	}
	state.publish(state.status.Reason, now)
	l.id = state.status.ConnID
	l.snapshot = firstServeUsageSnapshot(state, now)
	l.resetContinuation = state.status.Rotations > 0
	l.responses = entry.responses
	l.turnID = uuid.NewString()
	l.conversationID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("first_serve:%d:%d:%d:%s:%s", account.ID, apiKeyID, groupID, scope, l.id))).String()
	if account.GetCodexFingerprintMode() != codexFingerprintOff {
		// Keep the stored account identity intact. Every request receives the
		// same generation seed, including an old request finishing after rotation.
		copyAccount.Extra = maps.Clone(account.Extra)
		copyAccount.Extra[codexFingerprintSeedExtraKey] = entry.fingerprintSeed
		delete(copyAccount.Extra, "openai_device_id")
		l.fingerprint = resolveConvergedInstallationID(&copyAccount, entry.fingerprintSeed)
		copyAccount.Extra["openai_device_id"] = l.fingerprint
	}
	if account.GetCodexFingerprintMode() == codexFingerprintFull {
		l.conversationID = l.id
	}
	l.uses = state.uses
	copyAccount.Proxy, copyAccount.ProxyID, copyAccount.proxyGroupResolved = state.proxy, &state.proxy.ID, true
	return context.WithValue(ctx, openAIFirstServeHTTPKey{}, l), &copyAccount, l, nil
}

// Record first output timing and update the consecutive slow-request policy.
func observeFirstServeHTTP(ctx context.Context, ms int) {
	l := firstServeHTTPLease(ctx)
	if l == nil || l.kind != "stream" {
		return
	}
	l.entry.mu.Lock()
	defer l.entry.mu.Unlock()
	if l.observed || l.entry.state.uses != l.uses {
		return
	}
	l.observed = true
	l.entry.state.observe(ms, time.Now())
}

// Count each dispatched request once, including internal retries. The counter
// belongs to the captured combination so concurrent rotations cannot alter it.
func (l *openAIFirstServeHTTPLease) use() {
	if l == nil {
		return
	}
	l.entry.mu.Lock()
	defer l.entry.mu.Unlock()
	if l.applied {
		return
	}
	l.applied = true
	l.reused = l.uses.Add(1) > 1
}

func (l *openAIFirstServeHTTPLease) finish(result *OpenAIForwardResult, err error) {
	if l == nil {
		return
	}
	defer l.release()
	l.entry.mu.Lock()
	defer l.entry.mu.Unlock()
	if result != nil {
		result.FirstServeActive = l.kind == "stream" && l.applied && l.reused
		result.FirstServeSnapshot = cloneFirstServeUsageSnapshot(l.snapshot)
	}
	if result != nil && err == nil {
		l.responses.add(strings.TrimSpace(result.ResponseID))
	}
	state := l.entry.state
	if state.uses != l.uses {
		return // an older concurrent response must not overwrite the new combination
	}
	if l.kind != "stream" {
		return // keep the most recent streaming diagnostics intact
	}
	now := time.Now()
	last := &OpenAIFirstServeRequestStatus{Kind: l.kind, Outcome: "ttft_unavailable", DurationMs: now.Sub(l.started).Milliseconds()}
	if result != nil {
		last.RequestID = result.RequestID
		input, output := result.Usage.InputTokens, result.Usage.OutputTokens
		last.InputTokens, last.OutputTokens = &input, &output
	}
	switch {
	case err != nil:
		last.Outcome = "request_failed"
		state.publish("connection_failed", now)
	case l.observed:
		last.Outcome = "measured"
	case result != nil && result.FirstTokenMs != nil:
		last.Outcome = "measured"
		state.observe(*result.FirstTokenMs, now)
	}
	if state.status.FirstTokenMs == nil && err == nil {
		state.status.Reason = last.Outcome
	}
	state.status.LastRequest = last
	state.publish(state.status.Reason, now)
	if l.missing {
		state.status.Active = false
		state.publish(state.status.Reason, now)
	}
}

// Apply after all protocol adapters, account overrides and fingerprint changes.
// Only the outbound copy changes; ingress and retry bodies remain intact.
func applyFirstServeHTTPRequest(req *http.Request, account *Account) error {
	l := firstServeHTTPLease(req.Context())
	if l == nil || l.accountID != account.ID {
		return nil
	}
	applyFirstServeHeaders(req.Header, l)
	if !strings.HasSuffix(strings.TrimRight(req.URL.Path, "/"), "/responses") || req.GetBody == nil {
		return nil
	}
	reader, err := req.GetBody()
	if err != nil {
		return err
	}
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		return err
	}
	body, err = applyFirstServeBody(body, l)
	if err != nil {
		return err
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	return nil
}

func (l *openAIFirstServeHTTPLease) resetsResponse(id string) bool {
	if l == nil || !l.resetContinuation || strings.TrimSpace(id) == "" {
		return false
	}
	l.entry.mu.Lock()
	defer l.entry.mu.Unlock()
	return !l.responses.ids[strings.TrimSpace(id)]
}

func (l *openAIFirstServeHTTPLease) recordResponse(id string) {
	l.entry.mu.Lock()
	defer l.entry.mu.Unlock()
	l.responses.add(strings.TrimSpace(id))
}

func firstServeMetadata(l *openAIFirstServeHTTPLease) map[string]any {
	metadata := map[string]any{"session_id": l.id, "thread_id": l.conversationID, "window_id": l.conversationID, "conversation_id": l.conversationID, "turn_id": l.turnID}
	if l.fingerprint != "" {
		metadata["installation_id"] = l.fingerprint
	}
	return metadata
}

func applyFirstServeHeaders(headers http.Header, l *openAIFirstServeHTTPLease) {
	if l.fingerprint != "" {
		headers.Set("x-codex-installation-id", l.fingerprint)
	}
	for _, key := range []string{"session_id", "session-id", "conversation_id"} {
		headers.Set(key, l.id)
	}
	metadata := firstServeMetadata(l)
	headers.Set("conversation_id", l.conversationID)
	for _, key := range []string{"thread-id", "x-client-request-id", "x-codex-window-id"} {
		if headers.Get(key) != "" {
			headers.Set(key, l.conversationID)
		}
	}
	rewriteCodexTurnMetadataFields(headers, metadata)
	if l.fresh || l.shared || l.resetContinuation {
		// A client echo must not pin an account-wide combination to its old route.
		headers.Del("x-codex-turn-state")
	}
}

func applyFirstServeBody(body []byte, l *openAIFirstServeHTTPLease) ([]byte, error) {
	if l.resetsResponse(gjson.GetBytes(body, "previous_response_id").String()) {
		// A rotated proxy must not inherit the old upstream response chain.
		var err error
		body, _, err = dropPreviousResponseIDFromRawPayload(body)
		if err != nil {
			return nil, err
		}
	}
	body, err := sjson.SetBytes(body, "prompt_cache_key", l.id)
	if err != nil {
		return nil, err
	}
	if l.fingerprint != "" {
		body, err = sjson.SetBytes(body, "client_metadata.x-codex-installation-id", l.fingerprint)
		if err != nil {
			return nil, err
		}
		if gjson.GetBytes(body, "client_metadata.installation_id").Exists() {
			body, err = sjson.SetBytes(body, "client_metadata.installation_id", l.fingerprint)
			if err != nil {
				return nil, err
			}
		}
	}
	if gjson.GetBytes(body, "client_metadata.session_id").Exists() {
		body, err = sjson.SetBytes(body, "client_metadata.session_id", l.id)
		if err != nil {
			return nil, err
		}
	}
	for _, field := range []string{"client_metadata.thread_id", "client_metadata.x-codex-window-id", "client_metadata.window_id", "client_metadata.conversation_id"} {
		if gjson.GetBytes(body, field).Exists() {
			body, err = sjson.SetBytes(body, field, l.conversationID)
			if err != nil {
				return nil, err
			}
		}
	}
	if gjson.GetBytes(body, "client_metadata.turn_id").Exists() {
		body, err = sjson.SetBytes(body, "client_metadata.turn_id", l.turnID)
		if err != nil {
			return nil, err
		}
	}
	if l.fresh || l.shared || l.resetContinuation {
		body, err = sjson.DeleteBytes(body, "client_metadata.x-codex-turn-state")
		if err != nil {
			return nil, err
		}
	}
	if embedded := gjson.GetBytes(body, "client_metadata.x-codex-turn-metadata"); embedded.Type == gjson.String {
		headers := http.Header{}
		headers.Set("x-codex-turn-metadata", embedded.String())
		rewriteCodexTurnMetadataFields(headers, firstServeMetadata(l))
		body, err = sjson.SetBytes(body, "client_metadata.x-codex-turn-metadata", headers.Get("x-codex-turn-metadata"))
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}
