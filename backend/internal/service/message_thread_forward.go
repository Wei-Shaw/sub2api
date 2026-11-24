package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// relayMessageThreadUpstreamError relays an upstream thread error exactly as
// received. These errors describe the thread, not the account: they are
// returned before the model runs, the client recovers from them by error code,
// and none of them is a reason to retry, switch account or lower the account's
// health. A response without a thread error code is left to normal handling,
// so a 404 for an unknown model is never reported as a thread error.
//
// A redirect is not an answer either. The HTTP client does not follow one for
// a thread request, and the 3xx it hands back must not be taken for a
// successful response: the request fails here, unbound and unbilled.
func (s *GatewayService) relayMessageThreadUpstreamError(ctx context.Context, c *gin.Context, resp *http.Response) error {
	if threadFrom(ctx) == nil || resp == nil || resp.Body == nil {
		return nil
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		_ = resp.Body.Close()
		setOpsUpstreamError(c, resp.StatusCode, "upstream redirect not followed for a message thread", "")
		return &MessageThreadError{Status: http.StatusBadGateway, Type: "api_error",
			Message: "thread: the upstream answered with a redirect, which is not followed for a message thread. Send the full conversation without thread."}
	}
	if resp.StatusCode < 400 || resp.StatusCode >= 500 {
		return nil
	}
	raw, err := s.readUpstreamErrorBody(resp)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil || !gjson.ValidBytes(raw) {
		return nil
	}
	code := gjson.GetBytes(raw, "error.details.error_code")
	if code.Type != gjson.String || !strings.HasPrefix(code.Str, "thread_") {
		return nil
	}
	// The failure is decided by the request, so it is not attributed to the
	// upstream account in operations data either.
	errType := gjson.GetBytes(raw, "error.type").String()
	if errType == "" {
		errType = "invalid_request_error"
	}
	MarkOpsRequestScopedError(c, errType)
	MarkResponseCommitted(c)
	writeAnthropicPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, raw)
	return &MessageThreadError{
		Status:  resp.StatusCode,
		Type:    errType,
		Code:    code.Str,
		Message: "upstream message thread error: " + code.Str,
		Written: true,
	}
}

// readNativeMessageBody may return BOTH a body and an error only for a thread.
// In that case the body is a complete, metered JSON document within the size
// limit: run the normal route's accounting, but never publish or bind it.
func (s *GatewayService) readNativeMessageBody(ctx context.Context, reader io.Reader, c *gin.Context) ([]byte, error) {
	if threadFrom(ctx) == nil {
		return ReadUpstreamResponseBody(reader, s.cfg, c, anthropicTooLargeError)
	}
	body, err := readUpstreamResponseBodyLimited(reader, resolveUpstreamResponseReadLimit(s.cfg))
	if err == nil {
		return body, nil
	}
	if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
		setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
		anthropicTooLargeError(c)
		return nil, err
	}
	failure := &MessageThreadError{Status: http.StatusBadGateway, Type: "api_error", Message: "The upstream message body ended with a read error."}
	var envelope struct {
		Type  string       `json:"type"`
		ID    string       `json:"id"`
		Usage *ClaudeUsage `json:"usage"`
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.Type != "message" || !validThreadResponseID(envelope.ID) || envelope.Usage == nil {
		return nil, failure
	}
	return body, failure
}

// observeJSON checks the upstream body before writing headers or publishing an
// id. A malformed response with reported usage can still owe a settlement.
func (t *MessageThread) observeJSON(ctx context.Context, body []byte, usage *ClaudeUsage) error {
	if t == nil || t.attempt.finished {
		return nil
	}
	var facts threadResponseFacts
	payload, ambiguous := decodeThreadEventPayload(string(body))
	kind, _ := payload["type"].(string)
	id, _ := payload["id"].(string)
	if kind != "message" || !validThreadResponseID(id) {
		facts.invalid = "not_a_message"
	} else if ambiguous {
		// A member named twice: the client may read another id or stop
		// reason than this code does.
		facts.invalid = "ambiguous_json"
	}
	facts.id = id
	facts.stopReason, _ = payload["stop_reason"].(string)
	content, isArray := payload["content"].([]any)
	if !isArray {
		facts.invalid = "missing_content_array"
		t.bind(ctx, facts, usage, false) // records the rejection, not a child
		return &MessageThreadError{Status: http.StatusBadGateway, Type: "api_error", Message: "The upstream message did not contain a content array."}
	}
	if len(content) > 0 {
		last, _ := content[len(content)-1].(map[string]any)
		facts.lastBlock, _ = last["type"].(string)
	}
	reported, _ := payload["usage"].(map[string]any)
	facts.fallback = threadUsageHasFallbackMessage(reported)
	t.bind(ctx, facts, usage, false)
	return nil
}

// countThreadCreateBreakpoints counts the cache breakpoints a create request
// places on system and tools. The upstream counts them again on every continue
// request of the thread, although the request no longer carries them.
func countThreadCreateBreakpoints(body []byte) int {
	_, _, toolPaths, systemPaths := collectCacheControlPaths(body)
	return min(len(toolPaths)+len(systemPaths), maxCacheControlBlocks)
}

// enforceCacheControl keeps a thread request within the upstream's limit of
// four cache breakpoints, which for a thread also counts the top-level
// cache_control and, on a continue request, the breakpoints of the create
// request's system and tools.
func (t *MessageThread) enforceCacheControl(body []byte) []byte {
	if t == nil {
		return body
	}
	limit := maxCacheControlBlocks
	if gjson.GetBytes(body, "cache_control").Exists() {
		limit--
	}
	if t.IsContinue() {
		limit -= t.parent.Breakpoints
	}
	if limit >= maxCacheControlBlocks {
		return body
	}
	return normalizeCacheControlTTLOrder(enforceCacheControlBlockLimitN(body, max(limit, 0)))
}
