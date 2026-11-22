//go:build unit

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func threadTestPtr(s string) *string { return &s }

func (h *threadHarness) copyOfA(mutate func(*service.Account)) *service.Account {
	row := *h.accountA
	row.Credentials, row.Extra = map[string]any{}, map[string]any{}
	for k, v := range h.accountA.Credentials {
		row.Credentials[k] = v
	}
	for k, v := range h.accountA.Extra {
		row.Extra[k] = v
	}
	mutate(&row)
	return &row
}

func (h *threadHarness) credentialKey() string {
	if h.kind == "apikey" || h.kind == "passthrough" {
		return "api_key"
	}
	return "access_token"
}

func (h *threadHarness) reserved() int {
	h.funds.mu.Lock()
	defer h.funds.mu.Unlock()
	return len(h.funds.res)
}

// A thread is continued only with the credential it was created with. The
// account row cannot show whether a new token belongs to the same upstream
// principal: the recorded account and organization may be missing, or left
// over from before the row was authorized again.
func TestMessageThreadCredentialIdentity(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth-cli", "oauth", "setup-token"} {
		for _, metadata := range []string{"recorded", "missing"} {
			t.Run(kind+"/metadata_"+metadata, func(t *testing.T) {
				h := newThreadHarness(t, kind)
				if metadata == "missing" {
					for _, account := range []*service.Account{h.accountA, h.accountB} {
						delete(account.Extra, "account_uuid")
						delete(account.Extra, "org_uuid")
					}
				}
				key := h.credentialKey()
				first := threadResponseID(t, h.send(threadTurn{}))

				// A new credential on the row, with everything else as it was. The
				// upstream would accept it, as the same caller or as another one;
				// the gateway cannot know which and sends neither.
				for _, principal := range []string{"caller-a", "someone-else"} {
					h.a.mu.Lock()
					h.a.principals["new-credential"] = principal
					h.a.mu.Unlock()
					h.accountA.Credentials[key] = "new-credential"
					h.requireNotFound(threadTurn{previous: first, stream: true})
					h.accountA.Credentials[key] = "secret-a"
				}

				// Only the authoritative row changed; the scheduler still hands
				// out the old credential.
				h.repo.set(71, h.copyOfA(func(a *service.Account) { a.Credentials[key] = "new-credential" }))
				h.requireNotFound(threadTurn{previous: first})
				h.repo.set(71, h.accountA)

				second := threadResponseID(t, h.send(threadTurn{previous: first}))
				require.Equal(t, 2, h.a.count())
				require.Zero(t, h.b.count())

				// The binding holds a digest, not the credential.
				for _, redisKey := range h.db.Keys() {
					value, err := h.db.Get(redisKey)
					require.NoError(t, err)
					require.NotContains(t, redisKey+value, "secret-a")
				}

				if kind != "oauth" && kind != "oauth-cli" {
					return
				}
				// The token is rejected mid-flight and the row meanwhile holds a
				// rotated one. An ordinary request is repeated with it; a continue
				// request is not, since it would reach the upstream as whoever
				// that token belongs to.
				rotated := h.copyOfA(func(a *service.Account) { a.Credentials["access_token"] = "rotated-token" })
				h.a.mu.Lock()
				delete(h.a.principals, "secret-a")
				h.a.principals["rotated-token"] = "someone-else"
				h.a.mu.Unlock()
				h.repo.mu.Lock()
				h.repo.script = []*service.Account{h.accountA, rotated} // final check, then the reload after the 401
				h.repo.mu.Unlock()
				before := h.a.count()
				rec := h.send(threadTurn{previous: second, stream: true})
				require.NotEqual(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, before+1, h.a.count(), "the continue request was sent once, with the credential of the thread")
				require.Equal(t, "Bearer secret-a", h.a.last().header.Get("Authorization"))
				require.Zero(t, h.b.count())

				// A create request keeps that retry, and its thread belongs to the
				// token it finally went out with.
				h.accountA.Status, h.accountA.Schedulable = service.StatusActive, true
				h.accountA.TempUnschedulableUntil, h.accountA.RateLimitResetAt = nil, nil
				h.accountB.Status = service.StatusDisabled
				h.repo.mu.Lock()
				h.repo.script = nil
				h.repo.mu.Unlock()
				h.repo.set(71, rotated)
				before = h.a.count()
				created := threadResponseID(t, h.send(threadTurn{}))
				require.Equal(t, before+2, h.a.count(), "rejected once, then repeated with the rotated token")
				require.Equal(t, "Bearer rotated-token", h.a.last().header.Get("Authorization"))
				h.requireNotFound(threadTurn{previous: created}) // the scheduler still has the stale token
				h.accountA.Credentials["access_token"] = "rotated-token"
				threadResponseID(t, h.send(threadTurn{previous: created}))
			})
		}
	}
}

// A probe is admitted to a Claude-Code-only group without a system prompt, for
// that one request. It does not make a thread whose later, ordinary turns
// inherit a system prompt check that was never made.
func TestMessageThreadProbeDoesNotGrantClaudeCodeInheritance(t *testing.T) {
	h := newThreadHarness(t, "oauth-cli")
	h.group.ClaudeCodeOnly = true
	stored := func(n int) bool {
		t.Helper()
		keys := threadBindingKeys(h.db)
		require.Len(t, keys, n)
		all := true
		for _, key := range keys {
			value, err := h.db.Get(key)
			require.NoError(t, err)
			all = all && gjson.Get(value, "cc").Bool()
		}
		return all
	}

	// One output token, no system prompt: admitted as a probe, as before.
	probe := threadResponseID(t, h.send(threadTurn{omitDefs: true, maxTokens: 1}))
	require.False(t, stored(1), "a probe does not validate a system prompt")
	h.requireNotFound(threadTurn{previous: probe, messages: `[{"role":"user","content":"now a real turn"}]`})

	// Another probe on that thread is still a probe, and still grants nothing.
	again := threadResponseID(t, h.send(threadTurn{previous: probe, maxTokens: 1, messages: `[{"role":"user","content":"ping"}]`}))
	require.False(t, stored(2))
	h.requireNotFound(threadTurn{previous: again, messages: `[{"role":"user","content":"now a real turn"}]`})
	require.Equal(t, 2, h.a.count())

	// A create request with a validated system prompt does, for every later turn.
	h.db.FlushAll()
	first := threadResponseID(t, h.send(threadTurn{}))
	second := threadResponseID(t, h.send(threadTurn{previous: first}))
	third := threadResponseID(t, h.send(threadTurn{previous: second, maxTokens: 1}))
	threadResponseID(t, h.send(threadTurn{previous: third}))
	require.True(t, stored(4))
	require.Zero(t, h.b.count())
}

// Whatever the upstream metered before a stream failed is billed once, whether
// the stream ended in an error event or simply stopped, and the response is
// not bound.
func TestMessageThreadStreamFailureSettlesUsage(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth"} {
		for _, mode := range []string{"error-after-start", "eof-after-start"} {
			for _, turn := range []string{"create", "continue"} {
				t.Run(fmt.Sprintf("%s/%s/%s", kind, mode, turn), func(t *testing.T) {
					h := newThreadHarness(t, kind)
					previous, settled := "", 0
					if turn == "continue" {
						previous = threadResponseID(t, h.send(threadTurn{stream: true}))
						settled = 1
						require.Equal(t, 20, h.requireCommands(1)[0].OutputTokens, "a complete stream is billed by its final count")
					}
					h.a.setMode(mode)
					before := h.a.count()
					rec := h.send(threadTurn{previous: previous, stream: true})
					require.Equal(t, http.StatusOK, rec.Code, "the stream had started")
					require.Contains(t, rec.Body.String(), "message_start")
					require.NotContains(t, rec.Body.String(), "message_stop")

					command := h.requireCommands(settled + 1)[settled]
					require.Equal(t, 10000, command.InputTokens)
					require.Equal(t, 80000, command.CacheReadTokens)
					require.Equal(t, 1000, command.CacheCreationTokens)
					require.Equal(t, threadFakeStartOutputTokens, command.OutputTokens, "the output reported so far is billed too")
					require.Positive(t, command.BalanceCost)

					require.Equal(t, before+1, h.a.count())
					require.Zero(t, h.b.count(), "a request whose output has started is not moved to another account")
					require.Equal(t, settled, threadBindingCount(h.db), "no binding for the failed response")
					require.Eventually(t, func() bool { return h.reserved() == 0 }, 2*time.Second, 5*time.Millisecond, "the reservation is released")
					h.requireCommands(settled + 1)
				})
			}
		}
	}
}

// Block rules apply to the beta set that is about to be sent, including tokens
// the client never asked for: the ones this gateway adds, and the ones an
// account override adds.
func TestMessageThreadBetaPolicyOnOutboundSet(t *testing.T) {
	const injected = "interleaved-thinking-2025-05-14"
	t.Run("token added by the gateway", func(t *testing.T) {
		h := newThreadHarness(t, "oauth")
		first := threadResponseID(t, h.send(threadTurn{}))
		require.Contains(t, threadTestCanonicalBeta(h.a.last().header), injected)
		calls := h.a.count()

		// The rule is tightened mid-thread. The client's own header is unchanged.
		h.settings.policy = `{"rules":[{"beta_token":"` + injected + `","action":"block","scope":"oauth","error_message":"that beta is not allowed here"}]}`
		for _, turn := range []threadTurn{{previous: first}, {previous: first, stream: true}, {}} {
			rec := h.send(turn)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "that beta is not allowed here")
		}
		require.Equal(t, calls, h.a.count(), "nothing carrying the blocked token was sent")
		require.Zero(t, h.b.count())

		// Filtered instead: the stored set is not sent either, and a replay
		// builds a set without the token.
		h.settings.policy = `{"rules":[{"beta_token":"` + injected + `","action":"filter","scope":"oauth"}]}`
		h.requireNotFound(threadTurn{previous: first})
		replayed := threadResponseID(t, h.send(threadTurn{}))
		require.NotContains(t, threadTestCanonicalBeta(h.a.last().header), injected)
		threadResponseID(t, h.send(threadTurn{previous: replayed}))
		require.NotContains(t, threadTestCanonicalBeta(h.a.last().header), injected)
	})

	t.Run("token added by an account override", func(t *testing.T) {
		for _, kind := range []string{"apikey", "passthrough"} {
			h := newThreadHarness(t, kind)
			for _, account := range []*service.Account{h.accountA, h.accountB} {
				account.Credentials["header_override_enabled"] = true
				account.Credentials["header_overrides"] = map[string]any{"anthropic-beta": threadTestBeta + ",operator-beta-2026-03-03"}
			}
			first := threadResponseID(t, h.send(threadTurn{}))
			require.Contains(t, threadTestCanonicalBeta(h.a.last().header), "operator-beta-2026-03-03")
			calls := h.a.count()
			h.settings.policy = `{"rules":[{"beta_token":"operator-beta-2026-03-03","action":"block","scope":"apikey","error_message":"operator beta withdrawn"}]}`
			rec := h.send(threadTurn{previous: first})
			require.Equal(t, http.StatusBadRequest, rec.Code, kind+": "+rec.Body.String())
			require.Contains(t, rec.Body.String(), "operator beta withdrawn")
			require.Equal(t, calls, h.a.count())
			h.settings.policy = `{"rules":[]}`
			threadResponseID(t, h.send(threadTurn{previous: first}))
		}
	})
}

// threadFixedFixture is one legal thread: a request the fake upstream accepts
// on its own terms (model, token budget, tools and betas that go together),
// kept the same over three turns, and then changed one fixed field at a time.
type threadFixedFixture struct {
	name string
	// kinds lists where the feature is supported; everywhere when empty.
	kinds     []string
	beta      string // tokens the client adds to its beta header for the feature
	fixed     string
	tools     string
	maxTokens int
	// messages continues the thread when the response was not a tool call.
	messages string
	// respelled is the same fixed fields written differently, with the parts
	// that may change between requests changed.
	respelled string
	changes   map[string]string
}

var threadFixedFixtures = []threadFixedFixture{
	{
		name:      "tool choice and cache control",
		fixed:     `"tool_choice":{"type":"auto"},"cache_control":{"type":"ephemeral"}`,
		respelled: `"cache_control":{ "type" : "ephemeral" },"tool_choice":{"type":"auto"},"stop_sequences":["END"]`,
		changes: map[string]string{
			"tool_choice":   `"tool_choice":{"type":"any"},"cache_control":{"type":"ephemeral"}`,
			"cache_control": `"tool_choice":{"type":"auto"},"cache_control":{"type":"ephemeral","ttl":"1h"}`,
			// Dropped, set to null, and one that was never there.
			"tool_choice ":   `"cache_control":{"type":"ephemeral"}`,
			"cache_control ": `"tool_choice":{"type":"auto"},"cache_control":null`,
			"output_config":  `"tool_choice":{"type":"auto"},"cache_control":{"type":"ephemeral"},"output_config":null`,
		},
	},
	{
		name:      "thinking budget",
		fixed:     `"thinking":{"type":"enabled","budget_tokens":2048}`,
		maxTokens: 4096,
		respelled: `"thinking":{"budget_tokens":2.048e3,"type":"enabled"},"stop_sequences":["END"]`,
		changes: map[string]string{
			"thinking":  `"thinking":{"type":"enabled","budget_tokens":3072}`,
			"thinking ": `"thinking":{"type":"disabled"}`,
		},
	},
	{
		name: "structured output",
		fixed: `"output_config":{"format":{"type":"json_schema","schema":{"type":"object",` +
			`"properties":{"n":{"type":"integer","enum":[9007199254740993]}},"required":["n"],"additionalProperties":false}}}`,
		respelled: `"output_config":{"format":{"schema":{"additionalProperties":false,"required":["n"],` +
			`"properties":{"n":{"enum":[9007199254740993.0],"type":"integer"}},"type":"object"},"type":"json_schema"}}`,
		changes: map[string]string{
			// The neighbouring integer: equal as a float64, not equal.
			"output_config": `"output_config":{"format":{"type":"json_schema","schema":{"type":"object",` +
				`"properties":{"n":{"type":"integer","enum":[9007199254740992]}},"required":["n"],"additionalProperties":false}}}`,
		},
	},
	{
		name:      "context management",
		beta:      "context-management-2025-06-27",
		fixed:     `"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}`,
		respelled: `"context_management":{"edits":[ {"keep":"all","type":"clear_thinking_20251015"} ]},"stop_sequences":["END"]`,
		changes: map[string]string{
			"context_management": `"context_management":{"edits":[{"type":"clear_tool_uses_20250919","keep":{"type":"tool_uses","value":2}}]}`,
		},
	},
	{
		// A native MCP server: its beta, and a toolset that refers to it.
		name:  "MCP server",
		kinds: []string{"apikey", "passthrough", "oauth-cli"},
		beta:  "mcp-client-2025-11-20",
		fixed: `"mcp_servers":[{"type":"url","url":"https://mcp.example.com/sse","name":"docs","authorization_token":"token-1"}]`,
		tools: `[{"type":"mcp_toolset","mcp_server_name":"docs"},{"name":"lookup","description":"Look something up","input_schema":{"type":"object"}}]`,
		// The authorization token may change between requests.
		respelled: `"mcp_servers":[{"authorization_token":"token-2","name":"docs","url":"https://mcp.example.com/sse","type":"url"}]`,
		changes: map[string]string{
			"mcp_servers": `"mcp_servers":[{"type":"url","url":"https://other.example.com/sse","name":"docs","authorization_token":"token-1"}]`,
		},
	},
	{
		// A skill: its betas, and the code execution tool it runs in.
		name:     "skill",
		kinds:    []string{"apikey", "passthrough", "oauth-cli"},
		beta:     "skills-2025-10-02,code-execution-2025-08-25",
		fixed:    `"container":{"skills":[{"type":"anthropic","skill_id":"pdf","version":"latest"}]}`,
		tools:    `[{"type":"code_execution_20250825","name":"code_execution"}]`,
		messages: `[{"role":"user","content":"and the next page"}]`,
		// The container id is not part of the thread.
		respelled: `"container":{"id":"container_01","skills":[{"version":"latest","skill_id":"pdf","type":"anthropic"}]}`,
		changes: map[string]string{
			"container.skills": `"container":{"skills":[{"type":"anthropic","skill_id":"xlsx","version":"latest"}]}`,
		},
	},
}

// The fixed fields of a thread, one legal thread and one change at a time. The
// fake upstream checks each request on its own terms and compares the fixed
// fields itself; the gateway must neither alter them between turns nor let a
// client's change disappear in a rewrite.
func TestMessageThreadFixedFieldsAcrossTurns(t *testing.T) {
	for _, fixture := range threadFixedFixtures {
		kinds := fixture.kinds
		if len(kinds) == 0 {
			kinds = threadTestKinds()
		}
		for _, kind := range kinds {
			t.Run(fixture.name+"/"+kind, func(t *testing.T) {
				h := newThreadHarness(t, kind)
				h.fixed, h.betaExtra = fixture.fixed, fixture.beta
				turn := func(previous string, stream bool, fixed *string) threadTurn {
					next := threadTurn{previous: previous, stream: stream, fixed: fixed, tools: fixture.tools, maxTokens: fixture.maxTokens}
					if previous != "" {
						next.messages = fixture.messages
					}
					return next
				}
				first := threadResponseID(t, h.send(turn("", false, nil)))
				second := threadResponseID(t, h.send(turn(first, true, &fixture.respelled)))
				third := threadResponseID(t, h.send(turn(second, false, nil)))
				require.Equal(t, 3, h.a.count(), "three turns accepted by an upstream that checks each request and every fixed field")

				calls := h.a.count()
				for field, changed := range fixture.changes {
					field = strings.TrimSpace(field)
					rec := h.send(turn(third, false, &changed))
					require.Equal(t, http.StatusBadRequest, rec.Code, field+": "+rec.Body.String())
					require.Equal(t, "thread_fingerprint_mismatch", gjson.Get(rec.Body.String(), "error.details.error_code").Str, field)
					require.Contains(t, gjson.Get(rec.Body.String(), "error.message").Str, field)
				}
				require.Equal(t, calls, h.a.count(), "a changed request is not sent")

				threadResponseID(t, h.send(turn(third, true, nil)))
				require.Zero(t, h.b.count())
			})
		}
	}

	// Changes a gateway rewrite would make invisible to the upstream.
	t.Run("removed by the rewrite", func(t *testing.T) {
		// Without its beta, context_management is stripped from the request, so
		// the upstream sees the same request with or without it.
		h := newThreadHarness(t, "apikey")
		first := threadResponseID(t, h.send(threadTurn{}))
		for _, added := range []string{`"context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]}`, `"context_management":null`} {
			rec := h.send(threadTurn{previous: first, fixed: &added})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, gjson.Get(rec.Body.String(), "error.message").Str, "context_management")
		}
		require.Equal(t, 1, h.a.count())

		// A thread without tools has its tool_choice removed on every turn,
		// whatever its value, null included.
		for _, kind := range []string{"oauth", "setup-token"} {
			h = newThreadHarness(t, kind)
			first = threadResponseID(t, h.send(threadTurn{omitDefs: true}))
			next := `[{"role":"user","content":"next"}]`
			for _, added := range []string{`"tool_choice":{"type":"auto"}`, `"tool_choice":null`} {
				rec := h.send(threadTurn{previous: first, fixed: &added, messages: next})
				require.Equal(t, http.StatusBadRequest, rec.Code, added+": "+rec.Body.String())
				require.Equal(t, "thread_fingerprint_mismatch", gjson.Get(rec.Body.String(), "error.details.error_code").Str)
				require.Contains(t, gjson.Get(rec.Body.String(), "error.message").Str, "tool_choice")
			}
			require.Equal(t, 1, h.a.count())
			threadResponseID(t, h.send(threadTurn{previous: first, messages: next}))
		}
	})
}

// The rewrite for subscription accounts replaces the client's beta list with a
// fixed one. A native MCP server or a skill would go out without the beta it
// needs, so a thread request carrying one is refused there, before anything is
// sent. Tools that only have MCP in their name are ordinary tools.
func TestMessageThreadRefusesNativeFeaturesWithoutTheirBeta(t *testing.T) {
	for _, kind := range []string{"oauth", "setup-token"} {
		for _, fixture := range threadFixedFixtures {
			if len(fixture.kinds) == 0 {
				continue
			}
			t.Run(kind+"/"+fixture.name, func(t *testing.T) {
				h := newThreadHarness(t, kind)
				h.fixed, h.betaExtra = fixture.fixed, fixture.beta
				field := "mcp_servers"
				if strings.Contains(fixture.fixed, "skills") {
					field = "container.skills"
				}
				for _, stream := range []bool{false, true} {
					rec := h.send(threadTurn{stream: stream, tools: fixture.tools, maxTokens: fixture.maxTokens})
					require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
					require.Equal(t, "thread_unsupported_request", gjson.Get(rec.Body.String(), "error.details.error_code").Str)
					require.Equal(t, field, gjson.Get(rec.Body.String(), "error.details.unsupported.0").Str)
				}
				require.Zero(t, h.a.count()+h.b.count(), "nothing was sent")
				require.Zero(t, threadBindingCount(h.db))
			})
		}
		t.Run(kind+"/ordinary tools named after MCP", func(t *testing.T) {
			h := newThreadHarness(t, kind)
			tools := `[{"name":"mcp__files__read","description":"Read a file","input_schema":{"type":"object"}},` +
				`{"name":"mcp_servers","description":"List mcp_servers","input_schema":{"type":"object"}}]`
			first := threadResponseID(t, h.send(threadTurn{tools: tools, extra: `"metadata":{"user_id":"mcp_servers"}`}))
			require.Equal(t, "mcp__files__read", gjson.GetBytes(h.a.last().body, "tools.0.name").Str)
			second := threadResponseID(t, h.send(threadTurn{previous: first, stream: true}))
			threadResponseID(t, h.send(threadTurn{previous: second}))
			require.Equal(t, 3, h.a.count())
		})
	}
}

// The model a client names is compared as the client wrote it, not as the
// gateway normalizes it.
func TestMessageThreadPublicModelName(t *testing.T) {
	for _, kind := range []string{"apikey", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{model: threadTestModel + "[1m]"}))
			wireModel := gjson.GetBytes(h.a.last().body, "model").Str
			require.NotContains(t, wireModel, "[1m]", "the suffix is not part of the upstream model")

			// The other name resolves to the same upstream model, so only the
			// gateway can tell that the client changed it.
			rec := h.send(threadTurn{previous: first})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Equal(t, "thread_fingerprint_mismatch", gjson.Get(rec.Body.String(), "error.details.error_code").Str)
			require.Contains(t, gjson.Get(rec.Body.String(), "error.message").Str, "model")
			require.Equal(t, 1, h.a.count())

			threadResponseID(t, h.send(threadTurn{previous: first, model: threadTestModel + "[1m]"}))
			require.Equal(t, wireModel, gjson.GetBytes(h.a.last().body, "model").Str)
		})
	}
}

// Behind a composite route the request body already carries the upstream
// model. The client's change of the public name is its own change; a route
// that now resolves elsewhere is the gateway's, and ends the thread quietly.
func TestMessageThreadPublicModelBehindCompositeRoute(t *testing.T) {
	decision := func(public, upstream string) *service.CompositeRouteDecision {
		return &service.CompositeRouteDecision{Matched: true, TargetPlatform: service.PlatformAnthropic, PublicModel: public, UpstreamModel: upstream}
	}
	for _, kind := range []string{"apikey", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			created := h.send(threadTurn{composite: decision("team/fast", threadTestModel)})
			first := threadResponseID(t, created)
			require.Equal(t, "team/fast", gjson.Get(created.Body.String(), "model").Str, "the client is answered in its own model name")

			rec := h.send(threadTurn{previous: first, composite: decision("team/alias", threadTestModel)})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Equal(t, "thread_fingerprint_mismatch", gjson.Get(rec.Body.String(), "error.details.error_code").Str)
			require.Contains(t, gjson.Get(rec.Body.String(), "error.message").Str, "model")
			require.Equal(t, 1, h.a.count())

			// Same public name, route changed: not the client's doing.
			h.requireNotFound(threadTurn{previous: first, model: "claude-haiku-4-5", composite: decision("team/fast", "claude-haiku-4-5")})

			threadResponseID(t, h.send(threadTurn{previous: first, composite: decision("team/fast", threadTestModel)}))
			require.Equal(t, 2, h.a.count())
		})
	}
}

// An account override of anthropic-beta is filtered by the tools of a request.
// A continue request has none, and must still go out with the set of its thread.
func TestMessageThreadHeaderOverrideWithToolset(t *testing.T) {
	const fineGrained = "fine-grained-tool-streaming-2025-05-14"
	for _, kind := range []string{"apikey", "passthrough"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			h.accountA.Credentials["header_override_enabled"] = true
			h.accountA.Credentials["header_overrides"] = map[string]any{"anthropic-beta": threadTestBeta + "," + fineGrained}
			toolset := `[{"type":"computer_toolset_20260801","name":"computer"}]`
			create := threadTurn{model: "claude-sonnet-5-5", tools: toolset}
			first := threadResponseID(t, h.send(create))
			createBeta := threadTestCanonicalBeta(h.a.last().header)
			require.NotContains(t, createBeta, fineGrained, "not compatible with the toolset of this request")

			second := threadResponseID(t, h.send(threadTurn{previous: first, model: "claude-sonnet-5-5", stream: true}))
			require.Equal(t, createBeta, threadTestCanonicalBeta(h.a.last().header))
			threadResponseID(t, h.send(threadTurn{previous: second, model: "claude-sonnet-5-5"}))
			require.Equal(t, createBeta, threadTestCanonicalBeta(h.a.last().header))
			require.Equal(t, 3, h.a.count())

			// Changing the override ends the threads made under the old one.
			h.accountA.Credentials["header_overrides"] = map[string]any{"anthropic-beta": threadTestBeta}
			h.requireNotFound(threadTurn{previous: "msg_a_3", model: "claude-sonnet-5-5"})
		})
	}
}

// A one-hour breakpoint may not follow a shorter one, and the breakpoints a
// thread already holds cannot be raised any more.
func TestMessageThreadCacheTTLOrder(t *testing.T) {
	hour := `[{"role":"user","content":[{"type":"text","text":"keep this for an hour","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]`
	for _, kind := range []string{"oauth", "setup-token"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{omitDefs: true}))
			require.NotContains(t, gjson.GetBytes(h.a.last().body, "system").Raw, `"1h"`, "the gateway's own breakpoints are short")

			// Sent as it is, the upstream would reject the order; the client is
			// told to replay instead, and the replay is repaired as a whole.
			h.requireNotFound(threadTurn{previous: first, messages: hour})
			replayed := threadResponseID(t, h.send(threadTurn{omitDefs: true, messages: hour}))
			for _, breakpoint := range gjson.GetBytes(h.a.last().body, "system.#.cache_control.ttl").Array() {
				require.Equal(t, "1h", breakpoint.Str)
			}
			threadResponseID(t, h.send(threadTurn{previous: replayed, messages: hour}))
			require.Equal(t, 3, h.a.count())
			require.Zero(t, h.b.count())
		})
	}
	// The same on the paths that send the client's breakpoints as they are,
	// checked on the request as it would go out.
	short := `[{"role":"user","content":[{"type":"text","text":"keep this briefly","cache_control":{"type":"ephemeral"}}]}]`
	for _, kind := range []string{"apikey", "passthrough", "oauth-cli"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{messages: short}))
			h.requireNotFound(threadTurn{previous: first, messages: hour})
			second := threadResponseID(t, h.send(threadTurn{previous: first, messages: short}))
			h.requireNotFound(threadTurn{previous: second, stream: true, messages: hour})
			require.Equal(t, 2, h.a.count())
			// A thread that holds nothing shorter takes a one-hour breakpoint.
			long := threadResponseID(t, h.send(threadTurn{omitDefs: true, messages: hour}))
			threadResponseID(t, h.send(threadTurn{previous: long, messages: hour}))
			require.Equal(t, 4, h.a.count())
		})
	}
}

// The final check before sending re-reads the account row and applies the
// model rules to it, not to the object the scheduler handed out.
func TestMessageThreadFinalCheckUsesTheCurrentRow(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{}))
			for name, mapping := range map[string]map[string]any{
				"model withdrawn": {"claude-opus-4-8": "claude-opus-4-8"},
				"model remapped":  {threadTestModel: "claude-haiku-4-5"},
			} {
				h.repo.set(71, h.copyOfA(func(a *service.Account) { a.Credentials["model_mapping"] = mapping }))
				h.requireNotFound(threadTurn{previous: first, stream: true})
				if t.Failed() {
					t.Fatal(name)
				}
			}
			h.repo.set(71, h.accountA)
			threadResponseID(t, h.send(threadTurn{previous: first}))
		})
	}
}

// In simple run mode accounts are not scoped by group, for a create request
// and therefore for its continue requests.
func TestMessageThreadSimpleModePool(t *testing.T) {
	for _, kind := range []string{"apikey", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			h.cfg.RunMode = config.RunModeSimple
			h.accountA.AccountGroups = []service.AccountGroup{{AccountID: 71, GroupID: 7}}
			first := threadResponseID(t, h.send(threadTurn{}))
			second := threadResponseID(t, h.send(threadTurn{previous: first, stream: true}))
			threadResponseID(t, h.send(threadTurn{previous: second}))
			require.Equal(t, 3, h.a.count())
			require.Zero(t, h.b.count())

			// Standard mode keeps the group rule.
			h.cfg.RunMode = config.RunModeStandard
			h.requireNotFound(threadTurn{previous: "msg_a_3"})
		})
	}
}

// Thread state that exists but cannot be used is a fault, reported as one and
// never sent anywhere; it is not the ordinary expiry a client replays after.
func TestMessageThreadDamagedState(t *testing.T) {
	unavailable := func(h *threadHarness, previous string) {
		h.t.Helper()
		a, b := h.a.count(), h.b.count()
		rec := h.send(threadTurn{previous: previous, stream: true})
		require.Equal(h.t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
		require.Equal(h.t, "api_error", gjson.Get(rec.Body.String(), "error.type").Str)
		require.False(h.t, gjson.Get(rec.Body.String(), "error.details").Exists())
		require.Equal(h.t, a, h.a.count())
		require.Equal(h.t, b, h.b.count())
	}
	for _, kind := range []string{"apikey", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			h.fixed = `"tool_choice":{"type":"auto"}`
			first := threadResponseID(t, h.send(threadTurn{}))
			key := threadBindingKeys(h.db)[0]
			intact, err := h.db.Get(key)
			require.NoError(t, err)

			damagedRecords := []string{`{}`, `{"a":71}`, `not json`}
			// A record that is whole except for one digest that this version
			// never writes in that shape. Neither the client's fault (400) nor
			// an expiry (404).
			wire, identity := gjson.Get(intact, "w").Str, gjson.Get(intact, "i").Str
			field := gjson.Get(intact, "cf.tool_choice").Str
			require.Len(t, wire, 32)
			require.Len(t, identity, 64)
			require.Len(t, field, 16)
			for _, cut := range [][2]string{
				{`"w":"` + wire + `"`, `"w":"` + wire[:1] + `"`},
				{`"i":"` + identity + `"`, `"i":"` + strings.Repeat("z", 64) + `"`},
				{`"tool_choice":"` + field + `"`, `"tool_choice":"` + field[:8] + `"`},
				{`"tool_choice":"` + field + `"`, `"tool_choice":"` + field + `","max_tokens":"` + field + `"`},
			} {
				require.Contains(t, intact, cut[0])
				damagedRecords = append(damagedRecords, strings.Replace(intact, cut[0], cut[1], 1))
			}
			// Every field of a record is written on every record. One that is
			// gone, or null, is damage: the digest of the client's beta set
			// included, without which the request would look like a client
			// that changed its betas.
			var stored map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(intact), &stored))
			require.Contains(t, stored, "cb")
			require.Contains(t, stored, "cf")
			for field := range stored {
				for _, replacement := range []string{"", "null"} {
					damaged := map[string]json.RawMessage{}
					for name, value := range stored {
						if name != field {
							damaged[name] = value
						} else if replacement != "" {
							damaged[name] = json.RawMessage(replacement)
						}
					}
					encoded, err := json.Marshal(damaged)
					require.NoError(t, err)
					damagedRecords = append(damagedRecords, string(encoded))
				}
			}
			// A record of a version this code does not write.
			damagedRecords = append(damagedRecords, strings.Replace(intact, `{`, `{"future":1,`, 1))
			for _, damaged := range damagedRecords {
				require.NoError(t, h.db.Set(key, damaged))
				unavailable(h, first)
			}
			require.NoError(t, h.db.Set(key, intact))
			h.db.SetTTL(key, service.MessageThreadTTL)

			// The group cannot be resolved right now.
			h.groups.fail.Store(true)
			unavailable(h, first)
			h.groups.fail.Store(false)

			threadResponseID(t, h.send(threadTurn{previous: first}))
			h.db.FastForward(service.MessageThreadTTL + 10*time.Second)
			h.requireNotFound(threadTurn{previous: first})
		})
	}
}

// A request the gateway repeats after rewriting it can be answered with a
// thread error too. That answer is relayed like any other thread error.
func TestMessageThreadErrorAfterRectifiedRetry(t *testing.T) {
	for _, kind := range []string{"apikey", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			if kind == "apikey" {
				// The rectifier for API-key accounts is a setting of its own.
				h.settings.rectifier = `{"enabled":true,"thinking_signature_enabled":true,"thinking_budget_enabled":true,"apikey_signature_enabled":true}`
			}
			h.a.setMode("signature-then-thread-error")
			rec := h.send(threadTurn{
				messages: `[{"role":"user","content":"hi"},{"role":"assistant","content":[{"type":"thinking","thinking":"hm","signature":"bad"},{"type":"text","text":"hello"}]},{"role":"user","content":"again"}]`,
				fixed:    threadTestPtr(`"thinking":{"type":"enabled","budget_tokens":1024}`),
			})
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, threadTestUnsupported, rec.Body.String(), "status and bytes of the second answer")
			require.Equal(t, 2, h.a.count(), "the rejected request and its one rewritten repeat")
			require.Zero(t, h.b.count())
			require.True(t, h.accountA.IsSchedulable())
			h.repo.mu.Lock()
			require.Empty(t, h.repo.modelLimits)
			h.repo.mu.Unlock()
			require.Zero(t, threadBindingCount(h.db))
			require.Empty(t, h.commands())
		})
	}
}

// A terminal event the upstream did not complete, or whose name and payload
// disagree, ends the response for the client and for billing but is not a
// message_stop to continue from.
func TestMessageThreadIncompleteTerminalEvent(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth"} {
		for _, mode := range []string{"unterminated-stop", "mismatched-stop", "misnamed-stop", "untyped-stop", "unnamed-stop", "unclosed-block", "hang-unterminated"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				h := newThreadHarness(t, kind)
				if mode == "hang-unterminated" {
					h.cfg.Gateway.StreamDataIntervalTimeout = 1
				}
				first := threadResponseID(t, h.send(threadTurn{stream: true}))
				h.requireCommands(1)
				h.a.setMode(mode)
				rec := h.send(threadTurn{previous: first, stream: true})
				require.Equal(t, http.StatusOK, rec.Code)
				h.a.setMode("")

				command := h.requireCommands(2)[1]
				require.Equal(t, 10000, command.InputTokens)
				require.Equal(t, 20, command.OutputTokens, "the usage that did arrive is billed")
				require.Equal(t, 1, threadBindingCount(h.db), "nothing was bound")
				if mode == "unterminated-stop" || mode == "hang-unterminated" {
					// The upstream never finished its last event. The client is
					// not handed a finished one in its place.
					require.Contains(t, rec.Body.String(), "message_delta")
					require.NotContains(t, rec.Body.String(), "message_stop")
				}
				h.requireNotFound(threadTurn{previous: "msg_a_2"})
				require.Zero(t, h.b.count())
			})
		}
	}
}

// A continue request is routed by its binding. The sticky mapping of the same
// session key belongs to ordinary traffic: the thread neither follows it nor
// refreshes, clears or rewrites it.
func TestMessageThreadLeavesStickyMappingAlone(t *testing.T) {
	for _, loadBatch := range []bool{true, false} {
		t.Run(fmt.Sprintf("load_batch=%t", loadBatch), func(t *testing.T) {
			h := newThreadHarnessFor(t, "oauth-cli", loadBatch)
			first := threadResponseID(t, h.send(threadTurn{}))
			var sticky string
			for _, key := range h.db.Keys() {
				if strings.HasPrefix(key, "sticky_session:") {
					sticky = key
				}
			}
			require.NotEmpty(t, sticky, "the create request is an ordinary request and is sticky")
			value, err := h.db.Get(sticky)
			require.NoError(t, err)
			require.Equal(t, "71", value)

			// Ordinary traffic of the same session has since moved to B.
			require.NoError(t, h.db.Set(sticky, "72"))
			h.db.SetTTL(sticky, 7*time.Minute)
			unchanged := func() {
				t.Helper()
				value, err := h.db.Get(sticky)
				require.NoError(t, err)
				require.Equal(t, "72", value, "the mapping still belongs to B")
				require.Equal(t, 7*time.Minute, h.db.TTL(sticky), "and was not refreshed")
			}

			second := threadResponseID(t, h.send(threadTurn{previous: first, stream: true}))
			require.Equal(t, 2, h.a.count(), "the thread stays on A")
			unchanged()

			// A is unavailable: the thread ends, the mapping is not touched.
			later := time.Now().Add(time.Hour)
			h.accountA.RateLimitResetAt = &later
			h.requireNotFound(threadTurn{previous: second})
			unchanged()
			h.accountA.RateLimitResetAt = nil

			// The ordinary request of that session still goes where it was sent.
			rec := h.send(threadTurn{noThread: true})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Equal(t, 1, h.b.count())
			require.Equal(t, 2, h.a.count())

			// The thread still counts as one session on its account.
			require.Equal(t, []string{threadTestSession}, h.sessions.distinct(71))
		})
	}
}

// An account that cannot serve a thread is passed over as soon as it is
// selected, not after the request has waited in its queue.
func TestMessageThreadSkipsIncapableAccountBeforeWaiting(t *testing.T) {
	for _, kind := range []string{"apikey", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarnessFor(t, kind, false)
			if kind == "apikey" {
				h.accountA.Credentials["base_url"] = "https://relay.example.com"
			} else {
				h.accountA.Extra["custom_base_url_enabled"], h.accountA.Extra["custom_base_url"] = true, "https://relay.example.com"
			}
			h.slots.busy.Store(int64(71), true) // waiting for A would last until the wait times out

			started := time.Now()
			first := threadResponseID(t, h.send(threadTurn{stream: true}))
			require.Less(t, time.Since(started), 5*time.Second)
			require.True(t, strings.HasPrefix(first, "msg_b_"))
			require.Zero(t, h.a.count())
		})
	}
}

// A thread request is not answered with a canned response: the client would
// be told 200 for a thread that no upstream holds.
func TestMessageThreadIsNotInterceptedAsWarmup(t *testing.T) {
	warmup := `[{"role":"user","content":[{"type":"text","text":"Warmup"}]}]`
	for _, kind := range []string{"apikey", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			h.accountA.Credentials["intercept_warmup_requests"] = true

			// Control: the ordinary request is answered locally.
			rec := h.send(threadTurn{noThread: true, omitDefs: true, messages: warmup})
			require.Equal(t, http.StatusOK, rec.Code)
			require.Zero(t, h.a.count())

			first := threadResponseID(t, h.send(threadTurn{omitDefs: true, messages: warmup}))
			require.Equal(t, 1, h.a.count(), "the create request reached the upstream")
			require.Equal(t, 1, threadBindingCount(h.db))
			threadResponseID(t, h.send(threadTurn{previous: first, messages: warmup}))
			require.Equal(t, 2, h.a.count())
		})
	}
}

// Server-side fallbacks are billed per attempt, which this gateway does not
// settle; the combination is refused before anything is sent.
func TestMessageThreadRefusesFallbacks(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{}))
			for _, turn := range []threadTurn{
				{extra: `"fallbacks":"default"`},
				{previous: first, extra: `"fallbacks":[{"model":"claude-haiku-4-5"}]`},
				{previous: "msg_unknown", extra: `"fallbacks":"default"`},
			} {
				rec := h.send(turn)
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				require.Equal(t, "thread_unsupported_request", gjson.Get(rec.Body.String(), "error.details.error_code").Str)
				require.Equal(t, "fallbacks", gjson.Get(rec.Body.String(), "error.details.unsupported.0").Str)
			}
			require.Equal(t, 1, h.a.count())
			require.Zero(t, h.b.count())

			// Without a thread, fallbacks are none of this change's business.
			rec := h.send(threadTurn{noThread: true, extra: `"fallbacks":"default"`})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			threadResponseID(t, h.send(threadTurn{previous: first}))
		})
	}
}
