//go:build unit

package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// astraProRequestBody is the inbound OpenAI Responses request that must survive
// the account-switch loop unchanged: gpt-6-astra with the official top-level
// reasoning.mode/effort preserved by the service guard
// (normalizeOpenAIResponsesReasoningMode), never downgraded mode -> effort.
const astraProRequestBody = `{"model":"gpt-6-astra","stream":false,"input":"hello","reasoning":{"mode":"pro","effort":"max"}}`

// astraProCodexURL is the OAuth codex Responses exit every forwarded attempt must hit.
const astraProCodexURL = "https://chatgpt.com/backend-api/codex/responses"

// astraProSuccessResponse is the winning account's non-stream success; it echoes
// the structured reasoning.mode=pro so the client response must keep mode=pro.
const astraProSuccessResponse = `{"id":"resp_astra","model":"gpt-6-astra","status":"completed",` +
	`"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],` +
	`"usage":{"input_tokens":1,"output_tokens":2},` +
	`"reasoning":{"mode":"pro","effort":"max"}}`

// astraProForbiddenResponse: a plain upstream 403 that stays failover-eligible but
// is not reclassified as access-state/credential failure nor a silent refusal.
const astraProForbiddenResponse = `{"error":{"type":"forbidden_error","code":"forbidden","message":"Forbidden"}}`

// astraProCapturedUpstream records URL/accountID/body per forwarded attempt.
type astraProCapturedUpstream struct {
	service.HTTPUpstream
	mu         sync.Mutex
	urls       []string
	accountIDs []int64
	bodies     [][]byte
	answers    []*http.Response
}

func newAstraProCapturedUpstream(answers ...*http.Response) *astraProCapturedUpstream {
	return &astraProCapturedUpstream{answers: answers}
}

func (u *astraProCapturedUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	body, _ := io.ReadAll(req.Body)
	u.urls = append(u.urls, req.URL.String())
	u.accountIDs = append(u.accountIDs, accountID)
	u.bodies = append(u.bodies, body)
	var resp *http.Response
	if len(u.answers) > 0 {
		resp = u.answers[0]
		u.answers = u.answers[1:]
	}
	if resp == nil {
		resp = &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(bytes.NewReader(nil))}
	}
	return resp, nil
}

func (u *astraProCapturedUpstream) snapshot() ([]string, []int64, [][]byte) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.urls...), append([]int64(nil), u.accountIDs...), append([][]byte(nil), u.bodies...)
}

// newAstraProFailoverContext builds the gin context that drives handler.Responses
// through the real failover loop for the astra pro request.
func newAstraProFailoverContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	groupID := int64(3132)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      99,
		GroupID: &groupID,
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
		},
		User: &service.User{ID: 100},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100, Concurrency: 0})
	return c, rec
}

func astra403() *http.Response {
	return &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(astraProForbiddenResponse)),
	}
}

func astra200() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(astraProSuccessResponse)),
	}
}

// assertAstraProWire: every forwarded attempt must hit the codex responses URL and
// keep model=gpt-6-astra + reasoning.mode=pro + effort=max (no silent downgrade).
func assertAstraProWire(t *testing.T, urls []string, bodies [][]byte) {
	t.Helper()
	require.NotEmpty(t, urls, "failover loop must reach the upstream transport")
	for i, u := range urls {
		require.Equal(t, astraProCodexURL, u, "attempt %d must target the codex responses URL", i)
	}
	for i, body := range bodies {
		require.Equal(t, "gpt-6-astra", gjson.GetBytes(body, "model").String(), "attempt %d must keep model", i)
		require.Equal(t, "pro", gjson.GetBytes(body, "reasoning.mode").String(), "attempt %d must keep mode=pro", i)
		require.Equal(t, "max", gjson.GetBytes(body, "reasoning.effort").String(), "attempt %d must keep effort=max", i)
	}
}

// assertAstraProAccountSwitch: the failover loop must have actually touched both
// distinct OAuth fixture accounts (IDs 1 and 2), not replayed the same one.
func assertAstraProAccountSwitch(t *testing.T, accountIDs []int64) {
	t.Helper()
	require.Len(t, accountIDs, 2, "failover must attempt two accounts")
	require.ElementsMatch(t, []int64{1, 2}, accountIDs, "both OAuth accounts must be reached")
}

// 403 then a second OAuth account succeeding: wire bodies keep pro+max/model and the
// winning account's structured reasoning.mode=pro reaches the client unchanged.
func TestOpenAIGatewayHandlerResponses_AstraProFirst403SecondSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := newAstraProCapturedUpstream(astra403(), astra200())
	handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
	c, rec := newAstraProFailoverContext(t, astraProRequestBody)

	handler.Responses(c)

	urls, accountIDs, bodies := upstream.snapshot()
	assertAstraProWire(t, urls, bodies)
	assertAstraProAccountSwitch(t, accountIDs)
	require.Equal(t, http.StatusOK, rec.Code, "winning account must yield a 200 to the client")
	require.Equal(t, "completed", gjson.GetBytes(rec.Body.Bytes(), "status").String())
	require.Equal(t, "pro", gjson.GetBytes(rec.Body.Bytes(), "reasoning.mode").String(), "client must keep the winning response's mode=pro")
	require.Equal(t, "max", gjson.GetBytes(rec.Body.Bytes(), "reasoning.effort").String())
}

// both OAuth accounts 403: exhausted keeps every wire body pro+max/model, is not 200,
// and surfaces the existing-policy 502 upstream_error (unchanged 403-masking policy).
func TestOpenAIGatewayHandlerResponses_AstraProBoth403NoDowngrade(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := newAstraProCapturedUpstream(astra403(), astra403())
	handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
	c, rec := newAstraProFailoverContext(t, astraProRequestBody)

	handler.Responses(c)

	urls, accountIDs, bodies := upstream.snapshot()
	assertAstraProWire(t, urls, bodies)
	assertAstraProAccountSwitch(t, accountIDs)
	require.NotEqual(t, http.StatusOK, rec.Code, "all-403 must never surface a 200 to the client")
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Equal(t, "upstream_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
}

// passthrough -> non-passthrough drops only the encrypted *input* reasoning item;
// the official top-level reasoning.mode/effort survive verbatim.
func TestDeriveOpenAIForwardAttemptBody_AstraCrossModeKeepsTopLevelReasoningMode(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	canonical := []byte(`{"model":"gpt-6-astra","stream":false,"reasoning":{"mode":"pro","effort":"max"},"input":[` +
		`{"type":"message","role":"user","content":"hello"},` +
		`{"type":"reasoning","id":"rs_kiro_abc","encrypted_content":"ENC_BLOB","summary":[{"type":"summary_text","text":"thinking"}]}` +
		`]}`)
	canonicalSnapshot := append([]byte(nil), canonical...)
	state := &openAIPassthroughFailoverState{}

	kiro := newOpenAIPassthroughAccount(1, true)     // passthrough first
	bedrock := newOpenAIPassthroughAccount(2, false) // non-passthrough after

	firstBody := h.deriveOpenAIForwardAttemptBody(nil, canonical, kiro, state)
	require.Equal(t, 1, reasoningItemCount(t, firstBody), "first passthrough attempt keeps the encrypted input reasoning item")
	require.Equal(t, "pro", gjson.GetBytes(firstBody, "reasoning.mode").String())
	require.Equal(t, "max", gjson.GetBytes(firstBody, "reasoning.effort").String())

	secondBody := h.deriveOpenAIForwardAttemptBody(nil, canonical, bedrock, state)
	require.Equal(t, 0, reasoningItemCount(t, secondBody), "cross-mode attempt drops the encrypted input reasoning item")
	require.Equal(t, "pro", gjson.GetBytes(secondBody, "reasoning.mode").String(), "mode=pro must survive cross-mode sanitization")
	require.Equal(t, "max", gjson.GetBytes(secondBody, "reasoning.effort").String(), "effort=max must survive cross-mode sanitization")
	require.Equal(t, "gpt-6-astra", gjson.GetBytes(secondBody, "model").String())
	require.JSONEq(t, string(canonicalSnapshot), string(canonical), "canonical forwardBody must never be mutated")
	require.True(t, gjson.GetBytes(canonical, "input.1.encrypted_content").Exists())
}
