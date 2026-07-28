package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/qoder"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// qoderDefaultBaseURL is the Qoder Cloud Agents API root used when an account
// does not override credentials.base_url.
const qoderDefaultBaseURL = qoder.DefaultBaseURL

// QoderGatewayService bridges the stateless OpenAI Chat Completions protocol
// onto Qoder's stateful Cloud Agents API (agent + environment + session + SSE
// events). It reuses the shared upstream transport for proxy/concurrency and
// persists auto-provisioned agent/environment ids into the account's extra.
type QoderGatewayService struct {
	accountRepo    AccountRepository
	httpUpstream   HTTPUpstream
	redis          *redis.Client
	cfg            *config.Config
	settingService *SettingService
}

// NewQoderGatewayService constructs the service. redis may be nil, in which case
// session reuse degrades gracefully to a fresh session per request.
func NewQoderGatewayService(
	accountRepo AccountRepository,
	httpUpstream HTTPUpstream,
	redisClient *redis.Client,
	cfg *config.Config,
	settingService *SettingService,
) *QoderGatewayService {
	return &QoderGatewayService{
		accountRepo:    accountRepo,
		httpUpstream:   httpUpstream,
		redis:          redisClient,
		cfg:            cfg,
		settingService: settingService,
	}
}

// qoderTurn is one role/text pair in the flattened conversation used for the
// session-stitching key and for building the fallback prompt.
type qoderTurn struct {
	Role string
	Text string
}

// qoderChatContext is the parsed request plus the derived conversation state.
type qoderChatContext struct {
	Request   apicompat.ChatCompletionsRequest
	System    string
	Turns     []qoderTurn // ordered user/assistant turns (system excluded)
	UserText  string      // the final user message to send this turn
	LookupKey string      // key for the previous transcript (messages[:-1])
}

// ForwardChatCompletions handles an OpenAI /v1/chat/completions request for a
// qoder account, returning the standard OpenAIForwardResult consumed by the
// handler's billing/usage/failover machinery.
func (s *QoderGatewayService) ForwardChatCompletions(ctx context.Context, c *gin.Context, account *Account, body []byte, promptCacheKey, defaultMappedModel string) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	sessionID := getSessionID(c)
	prefix := logPrefix(sessionID, account.Name)

	var req apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse chat completions request: %w", err)
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("missing model")
	}

	chatCtx, err := s.buildQoderChatContext(account.ID, &req)
	if err != nil {
		return nil, err
	}

	provision, err := s.EnsureAgentAndEnvironment(ctx, account)
	if err != nil {
		logger.LegacyPrintf("service.qoder_gateway", "%s provision failed: %v", prefix, err)
		return nil, &UpstreamFailoverError{
			StatusCode:             http.StatusBadGateway,
			ResponseBody:           []byte(fmt.Sprintf(`{"error":"qoder provision failed: %s"}`, err.Error())),
			RetryableOnSameAccount: true,
		}
	}

	client := s.newQoderClient(account)

	// Resolve the session: reuse a stitched one (which already carries the
	// conversation context) when available, otherwise open a fresh session.
	qoderSessionID, lastEventID, hasContext, err := s.resolveQoderSession(ctx, client, &provision, chatCtx)
	if err != nil {
		logger.LegacyPrintf("service.qoder_gateway", "%s session resolve failed: %v", prefix, err)
		return nil, &UpstreamFailoverError{
			StatusCode:             http.StatusBadGateway,
			ResponseBody:           []byte(fmt.Sprintf(`{"error":"qoder session failed: %s"}`, err.Error())),
			RetryableOnSameAccount: true,
		}
	}

	if err := s.sendQoderTurn(ctx, client, &provision, chatCtx, &qoderSessionID, &lastEventID, &hasContext); err != nil {
		logger.LegacyPrintf("service.qoder_gateway", "%s send message failed: %v", prefix, err)
		return nil, s.qoderUpstreamError(err)
	}

	resp, err := client.StreamEvents(ctx, qoderSessionID, lastEventID)
	if err != nil {
		logger.LegacyPrintf("service.qoder_gateway", "%s stream open failed: %v", prefix, err)
		return nil, s.qoderUpstreamError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var streamRes *qoderStreamResult
	if req.Stream {
		streamRes, err = s.streamQoderToClient(ctx, c, resp, startTime, req.Model)
	} else {
		streamRes, err = s.collectQoderToClient(ctx, c, resp, startTime, req.Model)
	}
	if err != nil {
		return nil, err
	}

	// Persist the session binding for the next turn using the full transcript
	// (previous turns + this turn's assistant reply).
	if streamRes.LastEventID != "" {
		replyTurns := append(append([]qoderTurn{}, chatCtx.Turns...), qoderTurn{Role: "assistant", Text: streamRes.Text})
		storeKey := qoderConversationKey(account.ID, req.Model, chatCtx.System, replyTurns)
		s.storeQoderSession(ctx, storeKey, qoderSessionState{SessionID: qoderSessionID, LastEventID: streamRes.LastEventID})
	}

	duration := time.Since(startTime)
	logger.LegacyPrintf("service.qoder_gateway", "%s status=success duration_ms=%d stream=%v", prefix, duration.Milliseconds(), req.Stream)

	return &OpenAIForwardResult{
		Usage: OpenAIUsage{
			InputTokens:  qoderEstimateTokens(chatCtx.UserText),
			OutputTokens: qoderEstimateTokens(streamRes.Text),
		},
		Model:            req.Model,
		UpstreamModel:    resolveQoderAgentModel(account),
		UpstreamEndpoint: "/sessions/{id}/events/stream",
		Stream:           req.Stream,
		Duration:         duration,
		FirstTokenMs:     streamRes.FirstTokenMs,
		ClientDisconnect: streamRes.ClientDisconnect,
	}, nil
}

// buildQoderChatContext parses the request into system text, ordered turns and
// the final user message, and derives the session-stitching keys.
func (s *QoderGatewayService) buildQoderChatContext(accountID int64, req *apicompat.ChatCompletionsRequest) (*qoderChatContext, error) {
	var systemParts []string
	var turns []qoderTurn
	for _, msg := range req.Messages {
		text := qoderMessageText(msg.Content)
		switch msg.Role {
		case "system":
			if strings.TrimSpace(text) != "" {
				systemParts = append(systemParts, text)
			}
		case "user", "assistant":
			turns = append(turns, qoderTurn{Role: msg.Role, Text: text})
		}
	}
	if len(turns) == 0 {
		return nil, fmt.Errorf("no user message in request")
	}
	last := turns[len(turns)-1]
	if last.Role != "user" || strings.TrimSpace(last.Text) == "" {
		return nil, fmt.Errorf("last message must be a non-empty user message")
	}

	system := strings.Join(systemParts, "\n\n")
	lookupKey := qoderConversationKey(accountID, req.Model, system, turns[:len(turns)-1])
	return &qoderChatContext{
		Request:   *req,
		System:    system,
		Turns:     turns,
		UserText:  last.Text,
		LookupKey: lookupKey,
	}, nil
}

// resolveQoderSession returns the session id to use for this turn, the
// Last-Event-ID for incremental streaming, and whether the session already
// carries the conversation context. A reused stitched session has context; a
// freshly created session does not and must receive the flattened history inline
// in the user message (see qoderUserMessageText) rather than via a separate
// seeding turn, which would leave a second turn in flight and conflict.
func (s *QoderGatewayService) resolveQoderSession(ctx context.Context, client *qoder.Client, provision *qoderProvisionResult, chatCtx *qoderChatContext) (string, string, bool, error) {
	if state, err := s.lookupQoderSession(ctx, chatCtx.LookupKey); err == nil && state != nil {
		return state.SessionID, state.LastEventID, true, nil
	}

	sessionID, err := client.CreateSession(ctx, provision.AgentID, provision.EnvID)
	if err != nil {
		return "", "", false, fmt.Errorf("create session: %w", err)
	}
	return sessionID, "", false, nil
}

// sendQoderTurn posts the current user turn to the resolved session. When the
// upstream reports the session is busy (409 conflict) — typically a concurrent
// request sharing the same stitched session is mid-turn — the reused session is
// abandoned in favor of a fresh one and the send retried once. The session id,
// last-event-id and hasContext values are updated to reflect the session that
// was actually used.
func (s *QoderGatewayService) sendQoderTurn(ctx context.Context, client *qoder.Client, provision *qoderProvisionResult, chatCtx *qoderChatContext, sessionID *string, lastEventID *string, hasContext *bool) error {
	err := client.SendUserMessage(ctx, *sessionID, qoderUserMessageText(chatCtx, *hasContext))
	if !isQoderConflictError(err) {
		return err
	}
	freshID, createErr := client.CreateSession(ctx, provision.AgentID, provision.EnvID)
	if createErr != nil {
		return fmt.Errorf("create fallback session: %w", createErr)
	}
	*sessionID, *lastEventID, *hasContext = freshID, "", false
	return client.SendUserMessage(ctx, *sessionID, qoderUserMessageText(chatCtx, *hasContext))
}

// qoderUserMessageText renders the text posted to the session for this turn. A
// session that already carries the conversation context receives only the new
// user message; a fresh session receives the flattened prior history plus the
// current request in a single message so the agent has context in one turn.
func qoderUserMessageText(chatCtx *qoderChatContext, hasContext bool) string {
	if hasContext {
		return chatCtx.UserText
	}
	history := qoderFlattenHistory(chatCtx)
	if history == "" {
		return chatCtx.UserText
	}
	return history + "\n\nCurrent request:\n" + chatCtx.UserText
}

// isQoderConflictError reports whether err is a 409 session-busy conflict.
func isQoderConflictError(err error) bool {
	var apiErr *qoder.APIError
	return errors.As(err, &apiErr) && apiErr.IsConflict()
}

// qoderFlattenHistory renders the prior turns (excluding the final user message)
// into a single transcript the agent can read as context.
func qoderFlattenHistory(chatCtx *qoderChatContext) string {
	prior := chatCtx.Turns[:len(chatCtx.Turns)-1]
	if len(prior) == 0 && strings.TrimSpace(chatCtx.System) == "" {
		return ""
	}
	var b strings.Builder
	if strings.TrimSpace(chatCtx.System) != "" {
		b.WriteString("System instructions:\n")
		b.WriteString(chatCtx.System)
		b.WriteString("\n\n")
	}
	if len(prior) > 0 {
		b.WriteString("Previous conversation:\n")
		for _, t := range prior {
			b.WriteString(t.Role)
			b.WriteString(": ")
			b.WriteString(t.Text)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// qoderMessageText extracts plain text from a Chat Completions message content
// field, which may be a JSON string or an array of typed parts.
func qoderMessageText(raw json.RawMessage) string {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var parts []apicompat.ChatContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var texts []string
		for _, part := range parts {
			if part.Type == "text" && part.Text != "" {
				texts = append(texts, part.Text)
			}
		}
		return strings.Join(texts, "\n\n")
	}
	return ""
}

// qoderEstimateTokens approximates token counts for billing, since the Qoder
// stream carries no usage data. ~4 characters per token is a coarse heuristic.
func qoderEstimateTokens(text string) int {
	n := len([]rune(text)) / 4
	if n < 1 {
		return 1
	}
	return n
}

// qoderUpstreamError maps a qoder client error to a failover error so the
// handler can retry on the same account or switch accounts.
func (s *QoderGatewayService) qoderUpstreamError(err error) error {
	var apiErr *qoder.APIError
	status := http.StatusBadGateway
	retryable := true
	if errors.As(err, &apiErr) {
		status = apiErr.StatusCode
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			retryable = false
		case http.StatusTooManyRequests:
			retryable = true
		}
	}
	return &UpstreamFailoverError{
		StatusCode:             status,
		ResponseBody:           []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())),
		RetryableOnSameAccount: retryable,
	}
}

// qoderStreamResult captures the outcome of reading a Qoder SSE turn.
type qoderStreamResult struct {
	Text             string
	LastEventID      string
	FirstTokenMs     *int
	ClientDisconnect bool
}

// streamQoderToClient converts the Qoder SSE stream into OpenAI
// chat.completion.chunk frames written to the client, terminating with the
// standard [DONE] sentinel once the session returns to idle.
func (s *QoderGatewayService) streamQoderToClient(ctx context.Context, c *gin.Context, resp *http.Response, startTime time.Time, model string) (*qoderStreamResult, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}
	cw := newAntigravityClientWriter(c.Writer, flusher, "qoder")

	completionID := fmt.Sprintf("chatcmpl-%d", startTime.UnixNano())
	created := startTime.Unix()
	writeChunk := func(delta map[string]any, finishReason any) bool {
		chunk := map[string]any{
			"id":      completionID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []map[string]any{{
				"index":         0,
				"delta":         delta,
				"finish_reason": finishReason,
			}},
		}
		payload, _ := json.Marshal(chunk)
		return cw.Fprintf("data: %s\n\n", payload)
	}

	// OpenAI clients expect a leading role chunk.
	writeChunk(map[string]any{"role": "assistant", "content": ""}, nil)

	res := &qoderStreamResult{}
	var acc strings.Builder

	frames := make(chan *qoder.Frame, 16)
	scanErr := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		defer close(frames)
		scanner := bufio.NewScanner(resp.Body)
		maxLineSize := defaultMaxLineSize
		if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
			maxLineSize = s.cfg.Gateway.MaxLineSize
		}
		scanner.Buffer(make([]byte, 64*1024), maxLineSize)
		var accum qoder.FrameAccumulator
		for scanner.Scan() {
			if frame := accum.Line(scanner.Text()); frame != nil {
				select {
				case frames <- frame:
				case <-done:
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case scanErr <- err:
			case <-done:
			}
		}
	}()

	// Upstream data interval timeout guards against a hung session.
	var intervalCh <-chan time.Time
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		ticker := time.NewTicker(time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second)
		defer ticker.Stop()
		intervalCh = ticker.C
	}
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())

	for {
		select {
		case frame, ok := <-frames:
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !ok {
				// Upstream closed without an idle frame; finalize anyway.
				writeChunk(map[string]any{}, "stop")
				cw.Fprintf("data: [DONE]\n\n")
				res.Text = acc.String()
				res.ClientDisconnect = cw.Disconnected()
				return res, nil
			}
			if frame.ID != "" {
				res.LastEventID = frame.ID
			}
			if text := frame.DeltaText(); text != "" {
				if res.FirstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					res.FirstTokenMs = &ms
				}
				acc.WriteString(text)
				writeChunk(map[string]any{"content": text}, nil)
			}
			if frame.IsTurnEnd() {
				writeChunk(map[string]any{}, "stop")
				cw.Fprintf("data: [DONE]\n\n")
				res.Text = acc.String()
				res.ClientDisconnect = cw.Disconnected()
				return res, nil
			}

		case err := <-scanErr:
			if disconnect, handled := handleStreamReadError(err, cw.Disconnected(), "qoder"); handled {
				res.Text = acc.String()
				res.ClientDisconnect = disconnect
				return res, nil
			}
			return nil, fmt.Errorf("qoder stream read error: %w", err)

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout)*time.Second {
				continue
			}
			if cw.Disconnected() {
				res.Text = acc.String()
				res.ClientDisconnect = true
				return res, nil
			}
			return nil, fmt.Errorf("qoder stream data interval timeout")

		case <-ctx.Done():
			res.Text = acc.String()
			res.ClientDisconnect = true
			return res, nil
		}
	}
}

// collectQoderToClient reads the full Qoder SSE turn and returns a single
// non-streaming OpenAI chat.completion response.
func (s *QoderGatewayService) collectQoderToClient(ctx context.Context, c *gin.Context, resp *http.Response, startTime time.Time, model string) (*qoderStreamResult, error) {
	res := &qoderStreamResult{}
	var acc strings.Builder

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)
	var accum qoder.FrameAccumulator

	for scanner.Scan() {
		frame := accum.Line(scanner.Text())
		if frame == nil {
			continue
		}
		if frame.ID != "" {
			res.LastEventID = frame.ID
		}
		if text := frame.DeltaText(); text != "" {
			if res.FirstTokenMs == nil {
				ms := int(time.Since(startTime).Milliseconds())
				res.FirstTokenMs = &ms
			}
			acc.WriteString(text)
		}
		if frame.IsTurnEnd() {
			break
		}
	}
	if err := scanner.Err(); err != nil && acc.Len() == 0 {
		return nil, fmt.Errorf("qoder stream read error: %w", err)
	}

	res.Text = acc.String()
	if strings.TrimSpace(res.Text) == "" {
		return nil, &UpstreamFailoverError{
			StatusCode:             http.StatusBadGateway,
			ResponseBody:           []byte(`{"error":"empty response from qoder upstream"}`),
			RetryableOnSameAccount: true,
		}
	}

	completion := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", startTime.UnixNano()),
		"object":  "chat.completion",
		"created": startTime.Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]any{
				"role":    "assistant",
				"content": res.Text,
			},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}
	payload, err := json.Marshal(completion)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completion: %w", err)
	}
	c.Data(http.StatusOK, "application/json", payload)
	return res, nil
}
