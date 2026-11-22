//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type threadStoreStub struct {
	bindings map[string]MessageThreadBinding
	readErr  error
	bindErr  error
	binds    int
}

func (s *threadStoreStub) ReadMessageThread(_ context.Context, scope, id string) (*MessageThreadBinding, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	if b, ok := s.bindings[scope+"/"+id]; ok {
		return &b, nil
	}
	return nil, nil
}

func (s *threadStoreStub) BindMessageThread(_ context.Context, scope, id string, b MessageThreadBinding) error {
	s.binds++
	if s.bindErr != nil {
		return s.bindErr
	}
	if s.bindings == nil {
		s.bindings = map[string]MessageThreadBinding{}
	}
	s.bindings[scope+"/"+id] = b
	return nil
}

type threadGatewayCacheStub struct {
	GatewayCache
	*threadStoreStub
}

func threadTestBinding() MessageThreadBinding {
	return MessageThreadBinding{
		AccountID: 7, Identity: strings.Repeat("a", 64), Model: "claude-sonnet-4-5", ClientModel: "claude-sonnet-4-5",
		Wire: threadWireDigest([]byte(`{}`)),
		Beta: "oauth-2025-04-20," + messageThreadsBeta, ClientBeta: digestThreadValue(messageThreadsBeta), ContextTokens: 1200,
		ClientFixed: map[string]string{},
	}
}

func threadTestError(t *testing.T, err error) *MessageThreadError {
	t.Helper()
	var threadErr *MessageThreadError
	require.ErrorAs(t, err, &threadErr)
	return threadErr
}

func TestPrepareMessageThread(t *testing.T) {
	group := int64(3)
	store := &threadStoreStub{bindings: map[string]MessageThreadBinding{"1:2:3/msg_1": threadTestBinding()}}
	svc := &GatewayService{cache: threadGatewayCacheStub{threadStoreStub: store}}
	header := http.Header{"Anthropic-Beta": {messageThreadsBeta}}
	prepare := func(body string, native bool) (*MessageThread, error) {
		return svc.PrepareMessageThread(context.Background(), []byte(body), header, "claude-sonnet-4-5", 1, 2, &group, native)
	}

	thread, err := prepare(`{"model":"m","messages":[]}`, true)
	require.NoError(t, err)
	require.Nil(t, thread, "a request without thread is untouched")
	require.False(t, thread.IsContinue())
	require.Zero(t, thread.PreviousTokens())

	thread, err = prepare(`{"thread":{"type":"create"}}`, true)
	require.NoError(t, err)
	require.False(t, thread.IsContinue())

	thread, err = prepare(`{"thread":{"type":"continue","previous_message_id":"msg_1"},"messages":[]}`, true)
	require.NoError(t, err)
	require.True(t, thread.IsContinue())
	require.Equal(t, 1200, thread.PreviousTokens())
	require.EqualValues(t, 7, thread.BoundAccountID())

	// An escaped key is still the thread field.
	thread, err = prepare(`{"\u0074hread":{"type":"continue","previous_message_id":"msg_1"}}`, true)
	require.NoError(t, err)
	require.True(t, thread.IsContinue())

	for name, tc := range map[string]struct {
		body   string
		status int
		code   string
	}{
		"not an object":       {`{"thread":"create"}`, 400, ""},
		"unknown type":        {`{"thread":{"type":"fork"}}`, 400, ""},
		"missing type":        {`{"thread":{}}`, 400, ""},
		"missing id":          {`{"thread":{"type":"continue"}}`, 400, ""},
		"empty id":            {`{"thread":{"type":"continue","previous_message_id":""}}`, 400, ""},
		"numeric id":          {`{"thread":{"type":"continue","previous_message_id":1}}`, 400, ""},
		"id on create":        {`{"thread":{"type":"create","previous_message_id":"msg_1"}}`, 400, ""},
		"duplicate field":     {`{"thread":{"type":"create"},"thread":{"type":"continue","previous_message_id":"msg_1"}}`, 400, ""},
		"escaped duplicate":   {`{"thread":{"type":"create"},"\u0074hread":{"type":"continue","previous_message_id":"msg_1"}}`, 400, ""},
		"duplicate type":      {`{"thread":{"type":"create","type":"continue","previous_message_id":"msg_1"}}`, 400, ""},
		"system on continue":  {`{"system":"x","thread":{"type":"continue","previous_message_id":"msg_1"}}`, 400, ""},
		"null system":         {`{"system":null,"thread":{"type":"continue","previous_message_id":"msg_1"}}`, 400, ""},
		"empty tools":         {`{"tools":[],"thread":{"type":"continue","previous_message_id":"msg_1"}}`, 400, ""},
		"unknown id":          {`{"thread":{"type":"continue","previous_message_id":"msg_2"}}`, 404, "thread_not_found"},
		"id of another scope": {`{"thread":{"type":"continue","previous_message_id":"1:2:3/msg_1"}}`, 404, "thread_not_found"},
	} {
		_, err := prepare(tc.body, true)
		threadErr := threadTestError(t, err)
		require.Equal(t, tc.status, threadErr.Status, name)
		require.Equal(t, tc.code, threadErr.Code, name)
		require.False(t, threadErr.Written, name)
	}

	// A converted route refuses both kinds, and never reads the store.
	for _, body := range []string{`{"thread":{"type":"create"}}`, `{"thread":{"type":"continue","previous_message_id":"msg_1"}}`} {
		_, err := prepare(body, false)
		require.Equal(t, "thread_unsupported_request", threadTestError(t, err).Code)
		_, err = (*GatewayService)(nil).PrepareMessageThread(context.Background(), []byte(body), nil, "", 0, 0, nil, false)
		require.Equal(t, "thread_unsupported_request", threadTestError(t, err).Code)
	}

	// The client changed a fixed field.
	_, err = svc.PrepareMessageThread(context.Background(), []byte(`{"thread":{"type":"continue","previous_message_id":"msg_1"}}`), header, "claude-opus-4-8", 1, 2, &group, true)
	require.Equal(t, "thread_fingerprint_mismatch", threadTestError(t, err).Code)
	require.Contains(t, err.Error(), "model")
	_, err = svc.PrepareMessageThread(context.Background(), []byte(`{"thread":{"type":"continue","previous_message_id":"msg_1"}}`), http.Header{"Anthropic-Beta": {messageThreadsBeta + ",x"}}, "claude-sonnet-4-5", 1, 2, &group, true)
	require.Contains(t, err.Error(), "anthropic-beta")
	// The same set in another order and spread over two header lines is unchanged.
	_, err = svc.PrepareMessageThread(context.Background(), []byte(`{"thread":{"type":"continue","previous_message_id":"msg_1"}}`), http.Header{"Anthropic-Beta": {" " + messageThreadsBeta + " "}}, "claude-sonnet-4-5", 1, 2, &group, true)
	require.NoError(t, err)

	// Another tenant never sees the binding.
	for _, ids := range [][2]int64{{9, 2}, {1, 9}} {
		_, err := svc.PrepareMessageThread(context.Background(), []byte(`{"thread":{"type":"continue","previous_message_id":"msg_1"}}`), header, "claude-sonnet-4-5", ids[0], ids[1], &group, true)
		require.Equal(t, "thread_not_found", threadTestError(t, err).Code)
	}
	other := int64(4)
	_, err = svc.PrepareMessageThread(context.Background(), []byte(`{"thread":{"type":"continue","previous_message_id":"msg_1"}}`), header, "claude-sonnet-4-5", 1, 2, &other, true)
	require.Equal(t, "thread_not_found", threadTestError(t, err).Code)

	// A store failure is not a miss, and a create request does not need the store.
	store.readErr = errors.New("redis down")
	_, err = prepare(`{"thread":{"type":"continue","previous_message_id":"msg_1"}}`, true)
	require.Equal(t, http.StatusServiceUnavailable, threadTestError(t, err).Status)
	require.Empty(t, threadTestError(t, err).Code)
	_, err = prepare(`{"thread":{"type":"create"}}`, true)
	require.NoError(t, err)
	store.readErr = nil

	// A binding that exists but could not have been written by this version is
	// a fault, not an expiry the client should replay after.
	broken := threadTestBinding()
	broken.Beta = "oauth-2025-04-20"
	store.bindings["1:2:3/msg_broken"] = broken
	_, err = prepare(`{"thread":{"type":"continue","previous_message_id":"msg_broken"}}`, true)
	require.Equal(t, http.StatusServiceUnavailable, threadTestError(t, err).Status)
	require.Empty(t, threadTestError(t, err).Code)

	// Fallbacks cannot be combined with a thread here; refused before the store
	// is read, for create and continue alike.
	for _, body := range []string{`{"fallbacks":"default","thread":{"type":"create"}}`, `{"fallbacks":[],"thread":{"type":"continue","previous_message_id":"msg_1"}}`} {
		_, err = prepare(body, true)
		require.Equal(t, "thread_unsupported_request", threadTestError(t, err).Code)
		require.Equal(t, []string{"fallbacks"}, threadTestError(t, err).Unsupported)
	}

	// A fixed field the client changed, among them one it only added.
	fixed := threadTestBinding()
	fixed.ClientFixed = threadFixedFieldDigests([]byte(`{"tool_choice":{"type":"auto"},"thinking":{"type":"enabled","budget_tokens":1024}}`))
	store.bindings["1:2:3/msg_fixed"] = fixed
	_, err = prepare(`{"tool_choice":{"type":"auto"},"thinking":{"budget_tokens":1024,"type":"enabled"},"max_tokens":5,"thread":{"type":"continue","previous_message_id":"msg_fixed"}}`, true)
	require.NoError(t, err)
	_, err = prepare(`{"tool_choice":{"type":"any"},"thinking":{"type":"enabled","budget_tokens":1024},"output_config":{"effort":"low"},"thread":{"type":"continue","previous_message_id":"msg_fixed"}}`, true)
	require.Equal(t, "thread_fingerprint_mismatch", threadTestError(t, err).Code)
	require.Contains(t, err.Error(), "tool_choice, output_config")
	require.NotContains(t, err.Error(), "thinking")

	// Without a shared store there is no thread support at all.
	_, err = (&GatewayService{}).PrepareMessageThread(context.Background(), []byte(`{"thread":{"type":"create"}}`), header, "m", 1, 2, &group, true)
	require.Equal(t, http.StatusServiceUnavailable, threadTestError(t, err).Status)
}

func TestMessageThreadAccountSupport(t *testing.T) {
	apiKey := func(creds, extra map[string]any) *Account {
		if creds == nil {
			creds = map[string]any{}
		}
		creds["api_key"] = "k"
		return &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: creds, Extra: extra}
	}
	oauth := func(kind string, extra map[string]any) *Account {
		return &Account{ID: 1, Platform: PlatformAnthropic, Type: kind, Credentials: map[string]any{"access_token": "t"}, Extra: extra}
	}
	declared := map[string]any{MessageThreadAccountFlag: true}
	for name, tc := range map[string]struct {
		account *Account
		want    bool
	}{
		"api key, official origin":       {apiKey(nil, nil), true},
		"official origin, trailing /":    {apiKey(map[string]any{"base_url": "https://API.anthropic.com/"}, nil), true},
		"custom origin":                  {apiKey(map[string]any{"base_url": "https://relay.example.com"}, nil), false},
		"lookalike origin":               {apiKey(map[string]any{"base_url": "https://api.anthropic.com.example.com"}, nil), false},
		"custom origin, declared":        {apiKey(map[string]any{"base_url": "https://relay.example.com"}, declared), true},
		"declared as a string":           {apiKey(map[string]any{"base_url": "https://relay.example.com"}, map[string]any{MessageThreadAccountFlag: "true"}), false},
		"pool mode":                      {apiKey(map[string]any{"pool_mode": true}, nil), false},
		"pool mode, declared":            {apiKey(map[string]any{"pool_mode": true}, declared), true},
		"api key without a key":          {&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{}}, false},
		"oauth":                          {oauth(AccountTypeOAuth, nil), true},
		"setup token":                    {oauth(AccountTypeSetupToken, nil), true},
		"oauth through a relay":          {oauth(AccountTypeOAuth, map[string]any{"custom_base_url_enabled": true, "custom_base_url": "https://relay.example.com"}), false},
		"oauth through a declared relay": {oauth(AccountTypeOAuth, map[string]any{"custom_base_url_enabled": true, MessageThreadAccountFlag: true}), true},
		"bedrock":                        {&Account{Platform: PlatformAnthropic, Type: AccountTypeBedrock, Extra: declared}, false},
		"vertex":                         {&Account{Platform: PlatformAnthropic, Type: AccountTypeServiceAccount, Extra: declared}, false},
		"upstream":                       {&Account{Platform: PlatformAnthropic, Type: AccountTypeUpstream, Extra: declared}, false},
		"antigravity":                    {&Account{Platform: PlatformAntigravity, Type: AccountTypeOAuth, Extra: declared}, false},
		"openai":                         {&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k"}, Extra: declared}, false},
		"nil":                            {nil, false},
	} {
		require.Equal(t, tc.want, messageThreadAccountSupported(tc.account), name)
	}
}

func TestMessageThreadIdentity(t *testing.T) {
	oauth := func() *Account {
		return &Account{ID: 5, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
			Credentials: map[string]any{"access_token": "t1", "refresh_token": "r1", "expires_at": 1},
			Extra:       map[string]any{"account_uuid": "u", "org_uuid": "o"}}
	}
	base := messageThreadIdentity(oauth(), "t1")
	require.Len(t, base, 64)
	require.NotContains(t, base, "t1")
	require.Equal(t, base, messageThreadIdentity(oauth(), " t1 "))
	require.Empty(t, messageThreadIdentity(oauth(), ""), "no credential, no identity")
	require.Empty(t, messageThreadIdentity(nil, "t1"))

	// What is recorded about the account is not the identity: it can be
	// missing, or left over from before the row was authorized again.
	same := oauth()
	same.Extra = map[string]any{"session_window_utilization": 0.4}
	same.Credentials["refresh_token"] = "r2"
	proxy := int64(9)
	same.ProxyID = &proxy
	require.Equal(t, base, messageThreadIdentity(same, "t1"))

	// The credential that is sent is. A refreshed token and a token from a new
	// authorization look the same from here, and both end the thread.
	require.NotEqual(t, base, messageThreadIdentity(oauth(), "t2"))
	for name, change := range map[string]func(*Account){
		"row":   func(a *Account) { a.ID = 6 },
		"type":  func(a *Account) { a.Type = AccountTypeSetupToken },
		"relay": func(a *Account) { a.Extra["custom_base_url_enabled"] = true; a.Extra["custom_base_url"] = "https://r" },
	} {
		a := oauth()
		change(a)
		require.NotEqual(t, base, messageThreadIdentity(a, "t1"), name)
	}

	key := func() *Account {
		return &Account{ID: 5, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k1"}, Extra: map[string]any{}}
	}
	keyBase := messageThreadIdentity(key(), "k1")
	require.NotContains(t, keyBase, "k1")
	sameKey := key()
	sameKey.Credentials["base_url"] = "https://api.anthropic.com/"
	sameKey.Extra["quota_used"] = 3.0
	require.Equal(t, keyBase, messageThreadIdentity(sameKey, "k1"))
	require.NotEqual(t, keyBase, messageThreadIdentity(key(), "k2"))
	for name, change := range map[string]func(*Account){
		"origin":      func(a *Account) { a.Credentials["base_url"] = "https://relay.example.com" },
		"passthrough": func(a *Account) { a.Extra["anthropic_passthrough"] = true },
		"override": func(a *Account) {
			a.Credentials["header_override_enabled"] = true
			a.Credentials["header_overrides"] = map[string]any{"anthropic-beta": "x"}
		},
	} {
		a := key()
		change(a)
		require.NotEqual(t, keyBase, messageThreadIdentity(a, "k1"), name)
	}

	require.Equal(t, "k1", messageThreadRowCredential(key()))
	require.Equal(t, "t1", messageThreadRowCredential(oauth()))
	require.Empty(t, messageThreadRowCredential(nil))
}

func TestMessageThreadWireModel(t *testing.T) {
	apiKey := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"public": "claude-opus-4-8"}}}
	require.Equal(t, "claude-opus-4-8", messageThreadWireModel(apiKey, "public"))
	require.Equal(t, "other", messageThreadWireModel(apiKey, "other"))
	oauth := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{}}
	require.Equal(t, claude.NormalizeModelID("claude-sonnet-4-5"), messageThreadWireModel(oauth, "claude-sonnet-4-5"))
	require.Equal(t, "m", messageThreadWireModel(nil, "m"))
}

func TestMessageThreadFixedFieldDigests(t *testing.T) {
	body := []byte(`{"model":"m","tool_choice":{"type":"auto"},"thinking":{"type":"enabled","budget_tokens":1024},` +
		`"mcp_servers":[{"type":"url","url":"https://mcp","name":"docs","authorization_token":"secret-token"}],` +
		`"container":{"id":"c1","skills":[{"type":"anthropic","skill_id":"pdf"}]},"cache_control":{"type":"ephemeral"},` +
		`"max_tokens":10,"metadata":{"user_id":"u"},"stop_sequences":["x"]}`)
	digests := threadFixedFieldDigests(body)
	require.ElementsMatch(t, []string{"tool_choice", "thinking", "mcp_servers", "container.skills", "cache_control"}, mapKeys(digests), "free fields are not kept")
	for _, digest := range digests {
		require.True(t, isThreadHexDigest(digest, threadFieldDigestLen))
	}
	require.NotContains(t, fmt.Sprint(digests), "secret-token")

	// The same values spelled differently, and the parts that may change.
	respelled := []byte(`{"cache_control":{"type":"ephemeral"},"container":{"skills":[{"skill_id":"pdf","type":"anthropic"}],"id":"c2"},` +
		`"mcp_servers":[{"authorization_token":"another","name":"docs","url":"https://mcp","type":"url"}],` +
		`"thinking":{"budget_tokens":1.024e3,"type":"enabled"},"tool_choice":{ "type" : "auto" },"model":"other","max_tokens":99}`)
	require.Equal(t, digests, threadFixedFieldDigests(respelled))
	require.Empty(t, changedThreadFixedFields(digests, threadFixedFieldDigests(respelled)))
	require.Equal(t, threadWireDigest(body), threadWireDigest(respelled))

	for field, changed := range map[string]string{
		"tool_choice":        `{"tool_choice":{"type":"any"}}`,
		"thinking":           `{"thinking":{"type":"enabled","budget_tokens":2048}}`,
		"mcp_servers":        `{"mcp_servers":[{"type":"url","url":"https://other","name":"docs"}]}`,
		"context_management": `{"context_management":{"edits":[]}}`,
		"output_config":      `{"output_config":{"effort":"low"}}`,
		"container.skills":   `{"container":{"skills":[{"type":"anthropic","skill_id":"xlsx"}]}}`,
		"cache_control":      `{"cache_control":{"type":"ephemeral","ttl":"1h"}}`,
	} {
		require.Contains(t, changedThreadFixedFields(digests, threadFixedFieldDigests([]byte(changed))), field)
		require.NotEqual(t, threadWireDigest([]byte(`{}`)), threadWireDigest([]byte(changed)), field)
	}
	require.Equal(t, []string{"tool_choice", "thinking", "mcp_servers", "container.skills", "cache_control"},
		changedThreadFixedFields(digests, nil), "every dropped field is named, in a stable order")

	// An explicit null is something the client sent; an absent field is not.
	for _, field := range []string{"tool_choice", "thinking", "mcp_servers", "context_management", "output_config", "cache_control"} {
		null := threadFixedFieldDigests([]byte(`{"` + field + `":null}`))
		require.Contains(t, null, field)
		require.Equal(t, []string{field}, changedThreadFixedFields(nil, null), field)
		require.NotEqual(t, threadWireDigest([]byte(`{}`)), threadWireDigest([]byte(`{"`+field+`":null}`)), field)
	}
	require.Equal(t, []string{"container.skills"}, changedThreadFixedFields(nil, threadFixedFieldDigests([]byte(`{"container":{"skills":null}}`))))
	require.NotEqual(t, threadFixedFieldDigests([]byte(`{"tool_choice":null}`)), threadFixedFieldDigests([]byte(`{"tool_choice":"null"}`)))

	// Numbers are compared as numbers, exactly: neighbours beyond 2^53 differ,
	// and no spelling of one number differs from another spelling of it.
	schema := func(n string) []byte {
		return []byte(`{"output_config":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"n":{"enum":[` + n + `]}}}}}}`)
	}
	exact := threadFixedFieldDigests(schema("9007199254740993"))
	for _, other := range []string{"9007199254740992", "9007199254740994", "9007199254740993.5", "-9007199254740993", `"9007199254740993"`, "0.1", "0.10000000000000001"} {
		require.NotEqual(t, exact, threadFixedFieldDigests(schema(other)), other)
		require.NotEqual(t, threadWireDigest(schema("9007199254740993")), threadWireDigest(schema(other)), other)
	}
	for _, same := range []string{"9007199254740993.0", "9.007199254740993e15", "9007199254740993E0", "90071992547409930e-1"} {
		require.Equal(t, exact, threadFixedFieldDigests(schema(same)), same)
	}
	require.Equal(t, threadFixedFieldDigests(schema("0.1")), threadFixedFieldDigests(schema("1e-1")))
	require.Equal(t, threadFixedFieldDigests(schema("-0")), threadFixedFieldDigests(schema("0")))
	// A literal too large to expand is still compared, by how it is written.
	require.NotEqual(t, threadFixedFieldDigests(schema("1e99999")), threadFixedFieldDigests(schema("2e99999")))
	require.Equal(t, threadFixedFieldDigests(schema("1e99999")), threadFixedFieldDigests(schema("1e99999")))
}

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestMessageThreadCacheTTLs(t *testing.T) {
	for body, want := range map[string][2]bool{
		`{}`:                                     {false, false},
		`{"cache_control":{"type":"ephemeral"}}`: {true, false},
		`{"cache_control":{"type":"ephemeral","ttl":"1h"}}`:                                       {false, true},
		`{"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral","ttl":"5m"}}]}`: {true, false},
		`{"tools":[{"name":"a","cache_control":{"type":"ephemeral","ttl":"1h"}}]}`:                {false, true},
		`{"tools":[{"name":"a","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":[{"type":"text","text":"x","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`: {true, true},
	} {
		short, long := threadCacheTTLs([]byte(body))
		require.Equal(t, want, [2]bool{short, long}, body)
	}
}

// A request that would be answered by web search emulation never reaches an
// upstream, so it cannot create or continue a thread there.
func TestMessageThreadIsNotEmulated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	SetWebSearchManager(websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"}}, nil))
	defer SetWebSearchManager(nil)
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{Enabled: true, Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"}}})
	defer clearGlobalWebSearchConfig()

	upstream := &anthropicHTTPUpstreamRecorder{}
	svc := newForwardPartialUsageServiceForTest(upstream)
	svc.settingService = newSettingServiceForWebSearchTest(true)
	account := newAnthropicAPIKeyAccount(WebSearchModeEnabled)
	account.Credentials = map[string]any{"api_key": "k"}
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"tools":[{"type":"web_search_20250305","name":"web_search"}],"messages":[{"role":"user","content":"search"}],"thread":{"type":"create"}}`)
	require.True(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, body), "the request would be emulated")

	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	thread := &MessageThread{store: &threadStoreStub{}, scope: "1:2:3"}
	_, err = svc.Forward(thread.Context(context.Background()), c, account, parsed)
	threadErr := threadTestError(t, err)
	require.Equal(t, http.StatusBadRequest, threadErr.Status)
	require.Equal(t, "thread_unsupported_request", threadErr.Code)
	require.Nil(t, upstream.lastReq, "nothing was sent")
	require.Zero(t, rec.Body.Len(), "and nothing was answered locally")
}

func TestMessageThreadResolveBeta(t *testing.T) {
	create := &MessageThread{}
	filter := map[string]struct{}{"filtered-beta": {}}

	// A path that replaces the client's list keeps the thread beta.
	beta, err := create.resolveBeta("claude-code-20250219,oauth-2025-04-20", nil)
	require.NoError(t, err)
	require.Equal(t, "claude-code-20250219,oauth-2025-04-20,"+messageThreadsBeta, beta)
	beta, err = create.resolveBeta("", nil)
	require.NoError(t, err)
	require.Equal(t, messageThreadsBeta, beta)

	// A value that already has it is left exactly as computed.
	beta, err = create.resolveBeta("custom,"+messageThreadsBeta, filter)
	require.NoError(t, err)
	require.Equal(t, "custom,"+messageThreadsBeta, beta)

	// What the policy filters is dropped whatever its source, an account
	// override included, so a create request never freezes a token that its
	// own continue request would be refused for.
	beta, err = create.resolveBeta(messageThreadsBeta+",filtered-beta,operator-beta", filter)
	require.NoError(t, err)
	require.Equal(t, messageThreadsBeta+",operator-beta", beta)
	beta, err = create.resolveBeta("filtered-beta", filter)
	require.NoError(t, err)
	require.Equal(t, messageThreadsBeta, beta)

	// The current policy filters the thread beta itself: nothing restores it.
	_, err = create.resolveBeta("custom,"+messageThreadsBeta, map[string]struct{}{messageThreadsBeta: {}})
	require.Equal(t, "thread_unsupported_request", threadTestError(t, err).Code)

	parent := threadTestBinding()
	parent.Beta = "b-beta,a-beta," + messageThreadsBeta
	cont := &MessageThread{parent: &parent}
	// The builder now computes something else, because the request has no tools.
	beta, err = cont.resolveBeta("a-beta,c-beta", filter)
	require.NoError(t, err)
	require.Equal(t, parent.Beta, beta, "the set and order of the create request are reused")

	// A token of the thread is filtered now: the thread cannot go on as it was.
	_, err = cont.resolveBeta("a-beta", map[string]struct{}{"b-beta": {}})
	require.Equal(t, "thread_not_found", threadTestError(t, err).Code)
}

func TestMessageThreadMimicGuardedFields(t *testing.T) {
	mcp := `{"mcp_servers":[{"type":"url","url":"https://mcp.example.com","name":"docs"}]}`
	require.Equal(t, "mcp_servers", threadMimicUnsupportedField([]byte(mcp), "oauth-2025-04-20,"+messageThreadsBeta))
	require.Empty(t, threadMimicUnsupportedField([]byte(mcp), "mcp-client-2025-11-20,"+messageThreadsBeta))
	require.Equal(t, "container.skills", threadMimicUnsupportedField([]byte(`{"container":{"skills":[{"type":"anthropic","skill_id":"pdf"}]}}`), messageThreadsBeta))
	require.Empty(t, threadMimicUnsupportedField([]byte(`{"container":{"id":"c","skills":[{"type":"anthropic","skill_id":"pdf"}]}}`), "skills-2025-10-02"))
	// Only the top-level field is an MCP server. Tools that merely carry MCP in
	// their name, in a description or in the conversation are ordinary tools.
	for _, body := range []string{
		`{}`, `{"mcp_servers":[]}`, `{"mcp_servers":null}`, `{"container":{"id":"c"}}`, `{"container":{"skills":[]}}`,
		`{"tools":[{"name":"mcp__files__read","input_schema":{"type":"object"}},{"name":"mcp_servers","description":"mcp_servers"}]}`,
		`{"messages":[{"role":"user","content":"set mcp_servers to [1]"}],"metadata":{"mcp_servers":"x"}}`,
	} {
		require.Empty(t, threadMimicUnsupportedField([]byte(body), messageThreadsBeta), body)
	}
}

func TestMessageThreadBindingShape(t *testing.T) {
	valid := func() MessageThreadBinding {
		b := threadTestBinding()
		b.ClientFixed = threadFixedFieldDigests([]byte(`{"tool_choice":{"type":"auto"},"cache_control":{"type":"ephemeral"}}`))
		return b
	}
	ok := valid()
	require.True(t, ok.Valid())
	// Each of these is a digest of a shape this version never writes.
	for name, damage := range map[string]func(*MessageThreadBinding){
		"wire cut short":           func(b *MessageThreadBinding) { b.Wire = b.Wire[:1] },
		"wire not hex":             func(b *MessageThreadBinding) { b.Wire = strings.Repeat("z", 32) },
		"wire upper case":          func(b *MessageThreadBinding) { b.Wire = strings.ToUpper(b.Wire[:31]) + "A" },
		"identity cut short":       func(b *MessageThreadBinding) { b.Identity = b.Identity[:63] },
		"identity not hex":         func(b *MessageThreadBinding) { b.Identity = strings.Repeat("g", 64) },
		"client beta cut short":    func(b *MessageThreadBinding) { b.ClientBeta = b.ClientBeta[:10] },
		"client field cut short":   func(b *MessageThreadBinding) { b.ClientFixed["tool_choice"] = b.ClientFixed["tool_choice"][:15] },
		"client field not hex":     func(b *MessageThreadBinding) { b.ClientFixed["cache_control"] = "not-a-hex-digest" },
		"client field unknown":     func(b *MessageThreadBinding) { b.ClientFixed["max_tokens"] = strings.Repeat("a", 16) },
		"missing wire":             func(b *MessageThreadBinding) { b.Wire = "" },
		"missing account":          func(b *MessageThreadBinding) { b.AccountID = 0 },
		"context out of range":     func(b *MessageThreadBinding) { b.ContextTokens = messageThreadMaxTokens + 1 },
		"breakpoints out of range": func(b *MessageThreadBinding) { b.Breakpoints = 5 },
	} {
		b := valid()
		damage(&b)
		require.False(t, b.Valid(), name)
	}
	// Every request of a thread carries the thread beta itself, so the digest
	// of the client's beta set is part of every binding, like the (possibly
	// empty) set of the client's fixed fields.
	plain := threadTestBinding()
	require.True(t, plain.Valid())
	plain.ClientBeta = ""
	require.False(t, plain.Valid())
	plain = threadTestBinding()
	plain.ClientFixed = nil
	require.False(t, plain.Valid())
}

func TestMessageThreadEnforceCacheControl(t *testing.T) {
	count := func(body []byte) int { return strings.Count(string(body), `"cache_control"`) }
	messages := `"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral"}},{"type":"text","text":"b","cache_control":{"type":"ephemeral"}},{"type":"text","text":"c","cache_control":{"type":"ephemeral"}}]}]`

	parent := threadTestBinding()
	parent.Breakpoints = 2
	cont := &MessageThread{parent: &parent}
	out := cont.enforceCacheControl([]byte(`{` + messages + `}`))
	require.Equal(t, 2, count(out), "two slots are taken by the create request's system and tools")
	require.Contains(t, string(out), `"text":"c","cache_control"`, "the newest breakpoint is the one kept")

	out = cont.enforceCacheControl([]byte(`{"cache_control":{"type":"ephemeral"},` + messages + `}`))
	require.Equal(t, 2, count(out), "the top-level cache_control takes another slot and is itself kept")
	require.True(t, gjson.GetBytes(out, "cache_control").Exists())

	parent.Breakpoints = 0
	body := []byte(`{` + messages + `}`)
	require.Equal(t, string(body), string(cont.enforceCacheControl(body)), "within the limit nothing changes")

	create := &MessageThread{}
	body = []byte(`{"cache_control":{"type":"ephemeral"},"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral"}}],` + messages + `}`)
	out = create.enforceCacheControl(body)
	require.Equal(t, 4, count(out))
	require.True(t, gjson.GetBytes(out, "system.0.cache_control").Exists())

	require.Equal(t, string(body), string((*MessageThread)(nil).enforceCacheControl(body)))
	require.Equal(t, 3, countThreadCreateBreakpoints([]byte(`{"tools":[{"name":"a","cache_control":{}},{"name":"b"}],"system":[{"type":"text","text":"s","cache_control":{}},{"type":"text","text":"t","cache_control":{}}],`+messages+`}`)))
}

func TestNormalizeClaudeOAuthRequestBodyThreadContinue(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":10,"temperature":1,"tool_choice":{"type":"tool","name":"lookup"},"messages":[],"thread":{"type":"continue","previous_message_id":"msg_1"}}`)

	// Unchanged behaviour for every other request: tools is added, and a
	// tool_choice without tools is dropped.
	out, _ := normalizeClaudeOAuthRequestBody(body, "claude-sonnet-4-5", claudeOAuthNormalizeOptions{})
	require.Equal(t, `[]`, gjson.GetBytes(out, "tools").Raw)
	require.False(t, gjson.GetBytes(out, "tool_choice").Exists())

	// A continue request never gains tools, and keeps the tool_choice the
	// create request was sent with.
	out, _ = normalizeClaudeOAuthRequestBody(body, "claude-sonnet-4-5", claudeOAuthNormalizeOptions{threadContinue: true, threadHadTools: true})
	require.False(t, gjson.GetBytes(out, "tools").Exists())
	require.False(t, gjson.GetBytes(out, "system").Exists())
	require.Equal(t, "lookup", gjson.GetBytes(out, "tool_choice.name").Str)

	// The create request had no tools, so its tool_choice was dropped there too.
	out, _ = normalizeClaudeOAuthRequestBody(body, "claude-sonnet-4-5", claudeOAuthNormalizeOptions{threadContinue: true})
	require.False(t, gjson.GetBytes(out, "tools").Exists())
	require.False(t, gjson.GetBytes(out, "tool_choice").Exists())
}
