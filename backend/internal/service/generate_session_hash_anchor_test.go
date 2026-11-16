//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func mustParseChatCompletionsSessionHashRequest(t *testing.T, body string, ctx *SessionContext) *ParsedRequest {
	t.Helper()
	parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(body)), "chat_completions")
	require.NoError(t, err)
	parsed.SessionContext = ctx
	return parsed
}

// A client without metadata.user_id keeps one sticky key for the whole
// conversation, so every round lands on the account that holds its prompt
// cache. Breakpoints placed on the latest messages move each round and must
// not decide the key.
func TestGenerateSessionHash_ConversationAnchorStaysStableAcrossRounds(t *testing.T) {
	svc := &GatewayService{}
	ctx := &SessionContext{ClientIP: "127.0.0.1", UserAgent: "test-client", APIKeyID: 11}
	marked := func(text string) map[string]any {
		return msg("user", []any{map[string]any{"type": "text", "text": text, "cache_control": map[string]any{"type": "ephemeral"}}})
	}
	hash := func(messages ...any) string {
		return svc.GenerateSessionHash(mustParseSessionHashRequest(t, anthropicSessionBody("You are a helpful assistant.", messages, ""), ctx))
	}
	// Round 1 has its breakpoint on the first message and keeps the content key.
	// From round 2 the breakpoint has moved to the latest message and the
	// anchor takes over.
	round2 := hash(msg("user", "summarize order 42"), msg("assistant", "calling tool"), marked("tool results 1"))
	round3 := hash(msg("user", "summarize order 42"), msg("assistant", "calling tool"), marked("tool results 1"), msg("assistant", "calling tool again"), marked("tool results 2"))
	require.NotEmpty(t, round2)
	require.Equal(t, round2, round3)

	other := hash(msg("user", "summarize order 43"), msg("assistant", "calling tool"), marked("tool results 1"))
	require.NotEqual(t, round2, other, "a conversation with another first message is another session")

	geminiCtx := &SessionContext{ClientIP: "10.0.0.1", UserAgent: "gemini-cli", APIKeyID: 42}
	round1 := svc.GenerateSessionHash(mustParseGeminiSessionHashRequest(t, geminiSessionBody(nil, []any{geminiMsg("user", "Write a Go function")}), geminiCtx))
	geminiRound2 := svc.GenerateSessionHash(mustParseGeminiSessionHashRequest(t, geminiSessionBody(nil, []any{geminiMsg("user", "Write a Go function"), geminiMsg("model", "func hello() {}"), geminiMsg("user", "Add tests")}), geminiCtx))
	require.NotEmpty(t, round1)
	require.Equal(t, round1, geminiRound2)
}

// On the chat completions shape the system prompt is messages[0]. The anchor
// is the leading system messages plus the first other message; the system
// prompt alone must not be the anchor, otherwise every conversation of one
// client shares one key.
func TestGenerateSessionHash_ChatCompletionsAnchorIncludesSystemAndFirstMessage(t *testing.T) {
	svc := &GatewayService{}
	ctx := &SessionContext{ClientIP: "127.0.0.1", UserAgent: "test-client", APIKeyID: 11}
	hash := func(body string) string {
		return svc.GenerateSessionHash(mustParseChatCompletionsSessionHashRequest(t, body, ctx))
	}

	round1 := hash(`{"model":"my-model","messages":[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"summarize order 42"}]}`)
	round2 := hash(`{"model":"my-model","messages":[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"summarize order 42"},{"role":"assistant","content":"order 42 shipped"},{"role":"user","content":"and order 43?"}]}`)
	require.NotEmpty(t, round1)
	require.Equal(t, round1, round2, "rounds of one conversation share a key")

	other := hash(`{"model":"my-model","messages":[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"draft a release note"}]}`)
	require.NotEqual(t, round1, other, "same system prompt with another first user message is another conversation")

	otherSystem := hash(`{"model":"my-model","messages":[{"role":"system","content":"You are a terse assistant."},{"role":"user","content":"summarize order 42"}]}`)
	require.NotEqual(t, round1, otherSystem, "same first user message with another system message is another conversation")

	withDeveloper := hash(`{"model":"my-model","messages":[{"role":"developer","content":"Answer briefly."},{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"summarize order 42"}]}`)
	require.NotEqual(t, round1, withDeveloper, "a leading developer message is part of the anchor")
}

// A breakpoint after the first message moves with the conversation, so such a
// request skips the cacheable content key and uses the conversation anchor,
// which includes the request context. A breakpoint on the system prompt alone
// keeps its content key.
func TestGenerateSessionHash_LaterMessageBreakpointSkipsCacheableContentKey(t *testing.T) {
	svc := &GatewayService{}
	ctxA := &SessionContext{ClientIP: "10.0.0.1", UserAgent: "test-client", APIKeyID: 1}
	ctxB := &SessionContext{ClientIP: "10.0.0.2", UserAgent: "test-client", APIKeyID: 2}
	cc := map[string]any{"type": "ephemeral"}
	system := []any{map[string]any{"type": "text", "text": "You are a helpful assistant.", "cache_control": cc}}
	marked := msg("user", []any{map[string]any{"type": "text", "text": "tool results 1", "cache_control": cc}})

	later := anthropicSessionBody(system, []any{msg("user", "summarize order 42"), msg("assistant", "calling tool"), marked}, "")
	a := svc.GenerateSessionHash(mustParseSessionHashRequest(t, later, ctxA))
	b := svc.GenerateSessionHash(mustParseSessionHashRequest(t, later, ctxB))
	require.NotEqual(t, a, b, "a later message breakpoint skips the content key, so the request context is part of the key")

	systemOnly := func(first string, ctx *SessionContext) string {
		return svc.GenerateSessionHash(mustParseSessionHashRequest(t, anthropicSessionBody(system, []any{msg("user", first)}, ""), ctx))
	}
	require.Equal(t, systemOnly("summarize order 42", ctxA), systemOnly("draft a release note", ctxB), "a system-only breakpoint keeps its content key")
}

func sessionHashOf(t *testing.T, svc *GatewayService, protocol, body string, ctx *SessionContext) string {
	t.Helper()
	parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(body)), protocol)
	require.NoError(t, err)
	parsed.SessionContext = ctx
	return svc.GenerateSessionHash(parsed)
}

// A breakpoint on the first message is fixed for the whole conversation, so the
// content key from the marked message text stays the key, whatever the client
// address or the rest of the system prompt look like.
func TestGenerateSessionHash_FirstMessageBreakpointKeepsContentKey(t *testing.T) {
	svc := &GatewayService{}
	ctxA := &SessionContext{ClientIP: "1.1.1.1", UserAgent: "my-app/1.0", APIKeyID: 7}
	ctxB := &SessionContext{ClientIP: "2.2.2.2", UserAgent: "my-app/1.0", APIKeyID: 7}
	doc := `{"type":"text","text":"<<reference document>>","cache_control":{"type":"ephemeral"}}`
	hash := func(system, messages string, ctx *SessionContext) string {
		return sessionHashOf(t, svc, "anthropic", `{"model":"my-model","system":`+system+`,"messages":`+messages+`}`, ctx)
	}

	r1 := hash(`"You are helpful."`, `[{"role":"user","content":[`+doc+`,{"type":"text","text":"q1","cache_control":{"type":"ephemeral"}}]}]`, ctxA)
	r2 := hash(`"You are helpful."`, `[{"role":"user","content":[`+doc+`,{"type":"text","text":"q1"}]},{"role":"assistant","content":"a1"},{"role":"user","content":[{"type":"text","text":"q2","cache_control":{"type":"ephemeral"}}]}]`, ctxA)
	r2Moved := hash(`"You are helpful."`, `[{"role":"user","content":[`+doc+`,{"type":"text","text":"q1"}]},{"role":"assistant","content":"a1"},{"role":"user","content":[{"type":"text","text":"q2","cache_control":{"type":"ephemeral"}}]}]`, ctxB)
	require.NotEmpty(t, r1)
	require.Equal(t, r1, r2)
	require.Equal(t, r1, r2Moved, "another client address must not change the key")

	withClock1 := hash(`[{"type":"text","text":"Now: 10:00"}]`, `[{"role":"user","content":[`+doc+`]}]`, ctxA)
	withClock2 := hash(`[{"type":"text","text":"Now: 10:05"}]`, `[{"role":"user","content":[`+doc+`]},{"role":"assistant","content":"a1"},{"role":"user","content":[{"type":"text","text":"q2","cache_control":{"type":"ephemeral"}}]}]`, ctxA)
	require.Equal(t, withClock1, withClock2, "an uncached system text that changes must not change the key")
}

// A breakpoint on a leading system message of the chat completions shape is
// the system prompt's breakpoint and keeps the content key.
func TestGenerateSessionHash_ChatSystemMessageBreakpointKeepsContentKey(t *testing.T) {
	svc := &GatewayService{}
	ctxA := &SessionContext{ClientIP: "1.1.1.1", UserAgent: "my-app/1.0", APIKeyID: 7}
	ctxB := &SessionContext{ClientIP: "2.2.2.2", UserAgent: "my-app/1.0", APIKeyID: 7}
	system := `{"role":"system","content":[{"type":"text","text":"Shared system","cache_control":{"type":"ephemeral"}}]}`
	hash := func(messages string, ctx *SessionContext) string {
		return sessionHashOf(t, svc, "chat_completions", `{"model":"my-model","messages":[`+system+messages+`]}`, ctx)
	}

	r1 := hash(`,{"role":"user","content":"task X"}`, ctxA)
	r2 := hash(`,{"role":"user","content":"task X"},{"role":"assistant","content":"ok"},{"role":"user","content":"more"}`, ctxB)
	other := hash(`,{"role":"user","content":"task Y"}`, ctxB)
	require.NotEmpty(t, r1)
	require.Equal(t, r1, r2)
	require.Equal(t, r1, other, "the system message breakpoint decides the key, as it did for a system breakpoint")
}

// The anchor is the first non-system message that has text. A first message
// with only an image or only tool calls must not make every conversation of
// the client share one key.
func TestGenerateSessionHash_AnchorSkipsMessagesWithoutText(t *testing.T) {
	svc := &GatewayService{}
	ctx := &SessionContext{ClientIP: "1.1.1.1", UserAgent: "my-app/1.0", APIKeyID: 7}
	image := `{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}`

	hash := func(protocol, body string) string { return sessionHashOf(t, svc, protocol, body, ctx) }
	anthropic := func(rest string) string {
		return `{"model":"my-model","system":"Sys","messages":[` + image + rest + `]}`
	}
	x2 := hash("anthropic", anthropic(`,{"role":"assistant","content":"a cat"},{"role":"user","content":"is it cute?"}`))
	x3 := hash("anthropic", anthropic(`,{"role":"assistant","content":"a cat"},{"role":"user","content":"is it cute?"},{"role":"assistant","content":"yes"},{"role":"user","content":"thanks"}`))
	y2 := hash("anthropic", anthropic(`,{"role":"assistant","content":"a car"},{"role":"user","content":"what brand?"}`))
	require.Equal(t, x2, x3, "later rounds keep the anchor")
	require.NotEqual(t, x2, y2, "two image first conversations are different conversations")

	chatImage := `{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}`
	chat := func(rest string) string {
		return `{"model":"my-model","messages":[{"role":"system","content":"Sys"},` + chatImage + rest + `]}`
	}
	cx := hash("chat_completions", chat(`,{"role":"assistant","content":"a cat"},{"role":"user","content":"cute?"}`))
	cy := hash("chat_completions", chat(`,{"role":"assistant","content":"a car"},{"role":"user","content":"brand?"}`))
	require.NotEqual(t, cx, cy)

	toolCall := `{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"f","arguments":"{}"}}]}`
	tool := func(result, task string) string {
		return hash("chat_completions", `{"model":"my-model","messages":[{"role":"system","content":"Sys"},`+toolCall+`,{"role":"tool","tool_call_id":"c1","content":"`+result+`"},{"role":"user","content":"`+task+`"}]}`)
	}
	require.NotEqual(t, tool("r1", "task X"), tool("r2", "task Y"))

	geminiImage := `{"role":"user","parts":[{"inlineData":{"mimeType":"image/png","data":"AAAA"}}]}`
	gemini := func(rest string) string {
		return `{"contents":[` + geminiImage + rest + `]}`
	}
	gx := hash("gemini", gemini(`,{"role":"model","parts":[{"text":"a cat"}]},{"role":"user","parts":[{"text":"cute?"}]}`))
	gy := hash("gemini", gemini(`,{"role":"model","parts":[{"text":"a car"}]},{"role":"user","parts":[{"text":"brand?"}]}`))
	require.NotEqual(t, gx, gy)
}

// A breakpoint that moves forward each round, or one fixed after the first
// message, takes the conversation anchor: the key may change once when the
// breakpoint leaves the first message, then stays the same.
func TestGenerateSessionHash_BreakpointAfterFirstMessageUsesAnchor(t *testing.T) {
	svc := &GatewayService{}
	ctx := &SessionContext{ClientIP: "1.1.1.1", UserAgent: "my-app/1.0", APIKeyID: 7}
	marked := func(text string) map[string]any {
		return msg("user", []any{map[string]any{"type": "text", "text": text, "cache_control": map[string]any{"type": "ephemeral"}}})
	}
	hash := func(messages ...any) string {
		return svc.GenerateSessionHash(mustParseSessionHashRequest(t, anthropicSessionBody("Sys", messages, ""), ctx))
	}

	moving2 := hash(msg("user", "first"), msg("assistant", "a1"), marked("q2"))
	moving3 := hash(msg("user", "first"), msg("assistant", "a1"), msg("user", "q2"), msg("assistant", "a2"), marked("q3"))
	require.Equal(t, moving2, moving3)

	fixed2 := hash(msg("user", "first"), msg("assistant", "a1"), marked("pinned"))
	fixed3 := hash(msg("user", "first"), msg("assistant", "a1"), marked("pinned"), msg("assistant", "a2"), msg("user", "q3"))
	require.Equal(t, fixed2, fixed3)

	other := svc.GenerateSessionHash(mustParseSessionHashRequest(t, anthropicSessionBody("Sys", []any{msg("user", "second"), msg("assistant", "a1"), marked("q2")}, ""), &SessionContext{ClientIP: "1.1.1.1", UserAgent: "my-app/1.0", APIKeyID: 7}))
	require.NotEqual(t, moving2, other)
}

// Rolling a conversation back, or editing a message after the first one, keeps
// the key: the anchor is the first message that has text and nothing after it.
func TestGenerateSessionHash_RollbackOrEditAfterFirstMessageKeepsKey(t *testing.T) {
	svc := &GatewayService{}
	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "test", APIKeyID: 1}
	hash := func(messages ...any) string {
		return svc.GenerateSessionHash(mustParseSessionHashRequest(t, anthropicSessionBody("System prompt", messages, ""), ctx))
	}

	original := hash(msg("user", "msg1"), msg("assistant", "reply1"), msg("user", "msg2"), msg("assistant", "reply2"), msg("user", "msg3"))
	require.NotEmpty(t, original)
	require.Equal(t, original, hash(msg("user", "msg1"), msg("assistant", "reply1"), msg("user", "msg2"), msg("assistant", "reply2"), msg("user", "different msg3")), "editing the last message keeps the key")
	require.Equal(t, original, hash(msg("user", "msg1"), msg("assistant", "reply1"), msg("user", "msg2")), "rolling back to an earlier round keeps the key")
	require.Equal(t, original, hash(msg("user", "msg1"), msg("assistant", "edited reply1"), msg("user", "msg2"), msg("assistant", "reply2"), msg("user", "msg3")), "editing a reply keeps the key")
	require.NotEqual(t, original, hash(msg("user", "edited msg1"), msg("assistant", "reply1"), msg("user", "msg2"), msg("assistant", "reply2"), msg("user", "msg3")), "editing the first message is another conversation")

	chat := func(rest string) string {
		return svc.GenerateSessionHash(mustParseChatCompletionsSessionHashRequest(t, `{"model":"my-model","messages":[{"role":"system","content":"Sys"},{"role":"user","content":"msg1"}`+rest+`]}`, ctx))
	}
	chatOriginal := chat(`,{"role":"assistant","content":"reply1"},{"role":"user","content":"msg2"}`)
	require.Equal(t, chatOriginal, chat(`,{"role":"assistant","content":"reply1"},{"role":"user","content":"different msg2"}`))
	require.Equal(t, chatOriginal, chat(""))

	gemini := func(contents ...any) string {
		return svc.GenerateSessionHash(mustParseGeminiSessionHashRequest(t, geminiSessionBody(nil, contents), ctx))
	}
	geminiOriginal := gemini(geminiMsg("user", "msg1"), geminiMsg("model", "reply1"), geminiMsg("user", "msg2"))
	require.Equal(t, geminiOriginal, gemini(geminiMsg("user", "msg1"), geminiMsg("model", "reply1"), geminiMsg("user", "different msg2")))
	require.Equal(t, geminiOriginal, gemini(geminiMsg("user", "msg1")))
	require.NotEqual(t, geminiOriginal, gemini(geminiMsg("user", "edited msg1"), geminiMsg("model", "reply1"), geminiMsg("user", "msg2")))
}

// Adding user messages after the first, or changing them, keeps the key, so a
// client that sends several user messages in a row stays on one account.
func TestGenerateSessionHash_AddingOrChangingLaterUserMessagesKeepsKey(t *testing.T) {
	svc := &GatewayService{}
	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "test", APIKeyID: 1}
	hash := func(messages ...any) string {
		return svc.GenerateSessionHash(mustParseSessionHashRequest(t, anthropicSessionBody(nil, messages, ""), ctx))
	}

	base := hash(msg("user", "first"), msg("user", "second"), msg("user", "third"), msg("user", "fourth"), msg("user", "fifth"))
	require.NotEmpty(t, base)
	require.Equal(t, base, hash(msg("user", "first"), msg("user", "CHANGED"), msg("user", "third"), msg("user", "fourth"), msg("user", "fifth")), "changing a later user message keeps the key")
	require.Equal(t, base, hash(msg("user", "first"), msg("user", "second")), "fewer user messages keep the key")
	require.Equal(t, base, hash(msg("user", "first"), msg("user", "second"), msg("user", "third"), msg("user", "fourth"), msg("user", "fifth"), msg("assistant", "ok"), msg("user", "one more")), "more messages keep the key")
	require.NotEqual(t, base, hash(msg("user", "CHANGED first"), msg("user", "second"), msg("user", "third"), msg("user", "fourth"), msg("user", "fifth")), "changing the first user message is another conversation")
}
