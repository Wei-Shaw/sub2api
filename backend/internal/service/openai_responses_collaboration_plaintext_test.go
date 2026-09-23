package service

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// collaboration plaintext 适配器识别的最小请求形态：collaboration namespace
// 中 message 参数带 encrypted:true 的 send_message，外加一个 control 工具 wait
// 与一个不相关的普通 function。
const collabPlaintextToolsJSON = `[
	{"type":"namespace","name":"collaboration","tools":[
		{"type":"function","name":"send_message","description":"dm","parameters":{"type":"object","properties":{"message":{"type":"string","encrypted":true,"description":"note"}},"required":["message"]}},
		{"type":"function","name":"wait","parameters":{"type":"object"}}
	]},
	{"type":"function","name":"exec","parameters":{"type":"object"}}
]`

func collabPlaintextRequestBody(stream bool) []byte {
	streamValue := "false"
	if stream {
		streamValue = "true"
	}
	return []byte(`{"model":"gpt-5.5","stream":` + streamValue + `,"tools":` + collabPlaintextToolsJSON + `,"input":[
		{"type":"function_call","namespace":"collaboration","name":"send_message","call_id":"call_prev","arguments":"{\"message\":\"hi\"}"},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"go"}]}
	]}`)
}

func TestCollaborationPlaintextEscapedJSONFastPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	escapedNamespace := []byte(`{"tools":[{"type":"namespace","name":"collabor\u0061tion","tools":[{"type":"function","name":"send_message","parameters":{"type":"object","properties":{"message":{"type":"string","encrypted":true}}}}]}],"input":"go"}`)
	plain := []byte(`{"tools":[{"type":"function","name":"exec","parameters":{"type":"object"}}],"input":"go"}`)
	for _, tc := range []struct {
		name    string
		body    []byte
		lowered bool
	}{
		{"escaped namespace", escapedNamespace, true},
		{"ordinary no-op", plain, false},
		{"normal namespace", collabPlaintextRequestBody(false), true},
	} {
		t.Run("http request "+tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			out, err := adaptOpenAIResponsesCollabPlaintextBody(c, collabPlaintextAPIKeyAccount(), tc.body)
			require.NoError(t, err)
			require.Equal(t, tc.lowered, gjson.GetBytes(out, `tools.#(name=="collaboration__send_message")`).Exists())
			if !tc.lowered {
				require.True(t, bytes.Equal(tc.body, out))
			}
		})
		t.Run("ws request "+tc.name, func(t *testing.T) {
			session := &openAIWSCollabPlaintextSession{accountID: 1}
			out, err := session.adaptPayload(tc.body)
			require.NoError(t, err)
			require.Equal(t, tc.lowered, gjson.GetBytes(out, `tools.#(name=="collaboration__send_message")`).Exists())
			if !tc.lowered {
				require.True(t, bytes.Equal(tc.body, out))
			}
		})
	}
	t.Run("escaped namespace prefix and numeric no-op", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		body := bytes.Replace(escapedNamespace, []byte(`collabor\u0061tion`), []byte(`\u0063ollaboration`), 1)
		out, err := adaptOpenAIResponsesCollabPlaintextBody(c, collabPlaintextAPIKeyAccount(), body)
		require.NoError(t, err)
		require.True(t, gjson.GetBytes(out, `tools.#(name=="collaboration__send_message")`).Exists())
		noOp := []byte(`{"metadata":{"count":900719925474099312345,"note":"\u0061"},"input":"go"}`)
		fresh, _ := gin.CreateTestContext(httptest.NewRecorder())
		unchanged, err := adaptOpenAIResponsesCollabPlaintextBody(fresh, collabPlaintextAPIKeyAccount(), noOp)
		require.NoError(t, err)
		require.True(t, bytes.Equal(noOp, unchanged))
	})

	t.Run("ws request escaped additional tools without top-level tools", func(t *testing.T) {
		session := &openAIWSCollabPlaintextSession{accountID: 1}
		frame := []byte(`{"type":"response.create","input":[{"type":"additional_tools","tools":[{"type":"namespace","name":"collabor\u0061tion","tools":[{"type":"function","name":"send_message","parameters":{"type":"object","properties":{"message":{"type":"string","encrypted":true}}}}]}]}]}`)
		out, err := session.adaptPayload(frame)
		require.NoError(t, err)
		require.True(t, gjson.GetBytes(out, `input.0.tools.#(name=="collaboration__send_message")`).Exists())
	})

	for _, tc := range []struct {
		name     string
		payload  []byte
		restored bool
	}{
		{"escaped alias", []byte(`{"output":[{"type":"function_call","name":"collaboration\u005f\u005fsend_message","call_id":"c1","arguments":"{}"}]}`), true},
		{"normal alias", []byte(`{"output":[{"type":"function_call","name":"collaboration__send_message","call_id":"c1","arguments":"{}"}]}`), true},
		{"ordinary no-op", []byte(`{"output":[]}`), false},
	} {
		t.Run("http response "+tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			setOpenAIResponsesCollabPlaintextMapping(c, apicompat.ResponsesCollaborationPlaintextMapping{Calls: map[string]apicompat.ResponsesNamespaceName{
				"collaboration__send_message": {Namespace: "collaboration", Name: "send_message"},
			}})
			out, err := restoreOpenAIResponsesCollabPlaintextPayload(c, tc.payload)
			require.NoError(t, err)
			require.Equal(t, tc.restored, gjson.GetBytes(out, "output.0.namespace").String() == "collaboration")
			if !tc.restored {
				require.True(t, bytes.Equal(tc.payload, out))
			}
		})
		t.Run("ws response "+tc.name, func(t *testing.T) {
			session := &openAIWSCollabPlaintextSession{accountID: 1, mapping: apicompat.ResponsesCollaborationPlaintextMapping{Calls: map[string]apicompat.ResponsesNamespaceName{
				"collaboration__send_message": {Namespace: "collaboration", Name: "send_message"},
			}}}
			out, err := session.restore(tc.payload)
			require.NoError(t, err)
			require.Equal(t, tc.restored, gjson.GetBytes(out, "output.0.namespace").String() == "collaboration")
			if !tc.restored {
				require.True(t, bytes.Equal(tc.payload, out))
			}
		})
	}
	t.Run("escaped malformed upstream with active mapping fails closed", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		setOpenAIResponsesCollabPlaintextMapping(c, apicompat.ResponsesCollaborationPlaintextMapping{Calls: map[string]apicompat.ResponsesNamespaceName{
			"collaboration__send_message": {Namespace: "collaboration", Name: "send_message"},
		}})
		_, err := restoreOpenAIResponsesCollabPlaintextPayload(c, []byte(`{"name":"collaboration\u005f\u005fsend_message`))
		require.Error(t, err)
	})
}

func TestCollaborationPlaintextRestoreNoAliasMalformedUnicode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, payload := range [][]byte{[]byte(`\u0061`), []byte(`collaboration \u0061`)} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		mapping := apicompat.ResponsesCollaborationPlaintextMapping{Calls: map[string]apicompat.ResponsesNamespaceName{
			"collaboration__send_message": {Namespace: "collaboration", Name: "send_message"},
		}}
		setOpenAIResponsesCollabPlaintextMapping(c, mapping)
		httpRestored, err := restoreOpenAIResponsesCollabPlaintextPayload(c, payload)
		require.NoError(t, err)
		require.True(t, bytes.Equal(payload, httpRestored), "HTTP must preserve a non-JSON fragment without a known alias")
		session := &openAIWSCollabPlaintextSession{accountID: 1, mapping: mapping}
		wsRestored, err := session.restore(payload)
		require.NoError(t, err)
		require.True(t, bytes.Equal(payload, wsRestored), "WS must preserve a non-JSON fragment without a known alias")
	}
}

func TestCollaborationPlaintextMalformedAliasExactUnicodePositions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const alias = "collaboration__send_message"
	escapePositions := func(positions ...int) string {
		selected := make(map[int]bool, len(positions))
		for _, position := range positions {
			selected[position] = true
		}
		var encoded strings.Builder
		for index := range len(alias) {
			if selected[index] {
				encoded.WriteString(fmt.Sprintf(`\u%04x`, alias[index]))
			} else {
				encoded.WriteByte(alias[index])
			}
		}
		return encoded.String()
	}
	all := make([]int, len(alias))
	for index := range all {
		all[index] = index
	}
	mixed := escapePositions(0, 5, strings.IndexByte(alias, '_'), len(alias)-1)
	for _, tc := range []struct {
		name  string
		raw   []byte
		error bool
	}{
		{"first character", []byte(`{"name":"` + escapePositions(0)), true},
		{"middle character", []byte(`{"name":"` + escapePositions(5)), true},
		{"first underscore", []byte(`{"name":"` + escapePositions(strings.IndexByte(alias, '_'))), true},
		{"last character", []byte(`{"name":"` + escapePositions(len(alias)-1)), true},
		{"all characters", []byte(`{"name":"` + escapePositions(all...)), true},
		{"mixed positions without opening quote", []byte(`prefix ` + mixed), true},
		{"plain alias outside JSON string", []byte(`prefix ` + alias), true},
		{"unknown alias", []byte(`{"name":"collaboration__not_mapped`), false},
		{"unrelated escape", []byte(`collaboration \u0061`), false},
		{"partial alias", []byte(`{"name":"collaboration__send`), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			mapping := apicompat.ResponsesCollaborationPlaintextMapping{Calls: map[string]apicompat.ResponsesNamespaceName{
				alias: {Namespace: "collaboration", Name: "send_message"},
			}}
			setOpenAIResponsesCollabPlaintextMapping(c, mapping)
			httpBody, httpErr := restoreOpenAIResponsesCollabPlaintextPayload(c, tc.raw)
			session := &openAIWSCollabPlaintextSession{mapping: mapping}
			wsBody, wsErr := session.restore(tc.raw)
			if tc.error {
				require.Error(t, httpErr)
				require.Error(t, wsErr)
				require.Nil(t, httpBody)
				require.Nil(t, wsBody)
			} else {
				require.NoError(t, httpErr)
				require.NoError(t, wsErr)
				require.True(t, bytes.Equal(tc.raw, httpBody))
				require.True(t, bytes.Equal(tc.raw, wsBody))
			}
		})
	}
	for _, name := range []string{escapePositions(0), escapePositions(5), escapePositions(all...), alias} {
		t.Run("valid JSON restoration", func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			mapping := apicompat.ResponsesCollaborationPlaintextMapping{Calls: map[string]apicompat.ResponsesNamespaceName{
				alias: {Namespace: "collaboration", Name: "send_message"},
			}}
			setOpenAIResponsesCollabPlaintextMapping(c, mapping)
			raw := []byte(`{"output":[{"type":"function_call","name":"` + name + `","arguments":"{}"}]}`)
			httpBody, err := restoreOpenAIResponsesCollabPlaintextPayload(c, raw)
			require.NoError(t, err)
			require.Equal(t, "collaboration", gjson.GetBytes(httpBody, "output.0.namespace").String())
			wsBody, err := (&openAIWSCollabPlaintextSession{mapping: mapping}).restore(raw)
			require.NoError(t, err)
			require.Equal(t, "send_message", gjson.GetBytes(wsBody, "output.0.name").String())
		})
	}
	noOp := []byte(`{"metadata":{"count":900719925474099312345,"text":"\u0061"},"output":[]}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	setOpenAIResponsesCollabPlaintextMapping(c, apicompat.ResponsesCollaborationPlaintextMapping{Calls: map[string]apicompat.ResponsesNamespaceName{
		alias: {Namespace: "collaboration", Name: "send_message"},
	}})
	unchanged, err := restoreOpenAIResponsesCollabPlaintextPayload(c, noOp)
	require.NoError(t, err)
	require.True(t, bytes.Equal(noOp, unchanged))
}

func TestOpenAIWSCollabPlaintextOffDeclarationRejectsMalformedFrames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	off := collabPlaintextAPIKeyAccount()
	delete(off.Extra, "openai_responses_plaintext_collaboration")
	valid := []byte(`{"type":"response.create","to\u006fls":[],"input":"first"}`)
	observeOpenAIWSCollabPlaintextClientTools(c, off, valid)
	session := openAIWSCollabPlaintextSessionFromContext(c)
	require.NotNil(t, session)
	require.Equal(t, "[]", string(session.declaredTools()))
	for _, invalid := range [][]byte{
		[]byte(`{"type":"response.create","tools":[`),
		[]byte(`{"type":"response.create","tools":42}`),
	} {
		observeOpenAIWSCollabPlaintextClientTools(c, off, invalid)
		require.Equal(t, "[]", string(session.declaredTools()), "invalid frame must not replace the accepted client declaration")
	}
	on := collabPlaintextAPIKeyAccount()
	onSession, err := openAIWSCollabPlaintextSessionForAccount(c, on)
	require.NoError(t, err)
	adapted, err := onSession.adaptPayload([]byte(`{"type":"response.create","input":"follow-up"}`))
	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(adapted, "tools").Array(), 0)
}

// OpenAI API-key 账号 + 已探测支持 Responses + opt-in flag。
func collabPlaintextAPIKeyAccount() *Account {
	return &Account{
		ID:          7701,
		Name:        "collab-plaintext-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode:        string(openai_compat.ResponsesSupportModeAuto),
			openai_compat.ExtraKeyResponsesSupported:   true,
			"openai_responses_plaintext_collaboration": true,
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func TestIsOpenAIResponsesPlaintextCollaborationEnabled(t *testing.T) {
	openAI := func(extra map[string]any) *Account {
		return &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: extra}
	}
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{"nil account", nil, false},
		{"nil extra", openAI(nil), false},
		{"missing key", openAI(map[string]any{"other": true}), false},
		{"explicit false", openAI(map[string]any{"openai_responses_plaintext_collaboration": false}), false},
		{"string true is not bool", openAI(map[string]any{"openai_responses_plaintext_collaboration": "true"}), false},
		{"numeric one is not bool", openAI(map[string]any{"openai_responses_plaintext_collaboration": 1}), false},
		{"enabled", openAI(map[string]any{"openai_responses_plaintext_collaboration": true}), true},
		{"non-openai platform ignored", &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_responses_plaintext_collaboration": true}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.IsOpenAIResponsesPlaintextCollaborationEnabled())
		})
	}
}

func TestOpenAIResponsesCollabPlaintextSupportedOutbound(t *testing.T) {
	oauth := func() *Account {
		a := newOpenAIOAuthNamespaceTestAccount()
		a.Extra = map[string]any{"openai_responses_plaintext_collaboration": true}
		return a
	}
	forceCC := collabPlaintextAPIKeyAccount()
	forceCC.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeForceChatCompletions)

	upstreamType := collabPlaintextAPIKeyAccount()
	upstreamType.Type = AccountTypeUpstream

	flagOff := collabPlaintextAPIKeyAccount()
	delete(flagOff.Extra, "openai_responses_plaintext_collaboration")

	require.True(t, openAIResponsesCollabPlaintextSupportedOutbound(collabPlaintextAPIKeyAccount()))
	require.True(t, openAIResponsesCollabPlaintextSupportedOutbound(oauth()))
	require.False(t, openAIResponsesCollabPlaintextSupportedOutbound(flagOff), "flag off → not applicable, callers gate on the getter first")
	require.False(t, openAIResponsesCollabPlaintextSupportedOutbound(forceCC), "force_chat_completions outbound is unsupported")
	require.False(t, openAIResponsesCollabPlaintextSupportedOutbound(upstreamType), "non apikey/oauth-like account type is unsupported")
}

func TestAdaptOpenAIResponsesCollabPlaintextBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("opt-out returns body byte-identical without touching context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		account := collabPlaintextAPIKeyAccount()
		delete(account.Extra, "openai_responses_plaintext_collaboration")
		body := collabPlaintextRequestBody(false)

		adapted, err := adaptOpenAIResponsesCollabPlaintextBody(c, account, body)

		require.NoError(t, err)
		require.Equal(t, body, adapted)
		_, ok := openAIResponsesCollabPlaintextMapping(c)
		require.False(t, ok)
	})

	t.Run("opt-in lowers marked tools and records mapping", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		adapted, err := adaptOpenAIResponsesCollabPlaintextBody(c, collabPlaintextAPIKeyAccount(), collabPlaintextRequestBody(false))
		require.NoError(t, err)

		flat := gjson.GetBytes(adapted, `tools.#(name=="collaboration__send_message")`)
		require.True(t, flat.Exists())
		require.Equal(t, "function", flat.Get("type").String())
		require.False(t, flat.Get("parameters.properties.message.encrypted").Exists(), "encrypted:true marker must be removed")
		require.Equal(t, "note", flat.Get("parameters.properties.message.description").String())
		// control 工具 wait 留在原 namespace，exec 原样。
		ns := gjson.GetBytes(adapted, `tools.#(type=="namespace")`)
		require.Equal(t, "collaboration", ns.Get("name").String())
		require.Equal(t, "wait", ns.Get("tools.0.name").String())
		require.Equal(t, "exec", gjson.GetBytes(adapted, `tools.#(name=="exec")`).Get("name").String())
		// input 里的 namespaced 调用改写成别名。
		require.Equal(t, "collaboration__send_message", gjson.GetBytes(adapted, "input.0.name").String())
		require.False(t, gjson.GetBytes(adapted, "input.0.namespace").Exists())
		require.Equal(t, "go", gjson.GetBytes(adapted, "input.1.content.0.text").String())

		mapping, ok := openAIResponsesCollabPlaintextMapping(c)
		require.True(t, ok)
		require.Equal(t,
			apicompat.ResponsesNamespaceName{Namespace: "collaboration", Name: "send_message"},
			mapping.Calls["collaboration__send_message"])
	})

	t.Run("opt-in body without marker returns byte-identical", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		body := []byte(`{"model":"gpt-5.5","input":"plain"}`)
		adapted, err := adaptOpenAIResponsesCollabPlaintextBody(c, collabPlaintextAPIKeyAccount(), body)
		require.NoError(t, err)
		require.Equal(t, body, adapted)
	})

	t.Run("unsupported outbound mode is rejected explicitly", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		account := collabPlaintextAPIKeyAccount()
		account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeForceChatCompletions)

		_, err := adaptOpenAIResponsesCollabPlaintextBody(c, account, collabPlaintextRequestBody(false))

		require.ErrorIs(t, err, errOpenAIResponsesCollabPlaintextUnsupported)
	})

	// H3：同一 attempt 内对已适配 context 再次调用不得抹掉 mapping，
	// 否则上游别名会静默泄漏给客户端。
	t.Run("second adapt on same context preserves mapping", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		account := collabPlaintextAPIKeyAccount()
		_, err := adaptOpenAIResponsesCollabPlaintextBody(c, account, collabPlaintextRequestBody(false))
		require.NoError(t, err)

		// 第二个 body 含 "collaboration" 字样但无可降级声明：若重新跑适配器，
		// 空 mapping 会把第一次的结果清掉。
		second := []byte(`{"model":"gpt-5.5","metadata":{"topic":"collaboration"},"input":"x"}`)
		adapted, err := adaptOpenAIResponsesCollabPlaintextBody(c, account, second)
		require.NoError(t, err)
		require.Equal(t, second, adapted)
		mapping, ok := openAIResponsesCollabPlaintextMapping(c)
		require.True(t, ok, "first-attempt mapping must survive the double-adapt")
		require.Contains(t, mapping.Calls, "collaboration__send_message")
	})
}

func TestRestoreOpenAIResponsesCollabPlaintextPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newCtx := func(withMapping bool) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		if withMapping {
			setOpenAIResponsesCollabPlaintextMapping(c, apicompat.ResponsesCollaborationPlaintextMapping{
				Calls: map[string]apicompat.ResponsesNamespaceName{
					"collaboration__send_message": {Namespace: "collaboration", Name: "send_message"},
				},
			})
		}
		return c
	}
	upstreamCall := []byte(`{"id":"resp_1","output":[{"type":"function_call","name":"collaboration__send_message","call_id":"c1","arguments":"{}"}]}`)

	t.Run("restores alias to namespace form", func(t *testing.T) {
		restored, err := restoreOpenAIResponsesCollabPlaintextPayload(newCtx(true), upstreamCall)
		require.NoError(t, err)
		require.Equal(t, "send_message", gjson.GetBytes(restored, "output.0.name").String())
		require.Equal(t, "collaboration", gjson.GetBytes(restored, "output.0.namespace").String())
		require.True(t, gjson.GetBytes(restored, "output.0.encrypted_function_args").IsArray())
		require.Len(t, gjson.GetBytes(restored, "output.0.encrypted_function_args").Array(), 0)
	})

	t.Run("no mapping leaves payload unchanged", func(t *testing.T) {
		restored, err := restoreOpenAIResponsesCollabPlaintextPayload(newCtx(false), upstreamCall)
		require.NoError(t, err)
		require.Equal(t, upstreamCall, restored)
	})

	t.Run("mapping without alias needle is byte-identical passthrough", func(t *testing.T) {
		payload := []byte(`{"id":"resp_1","output":[]}`)
		restored, err := restoreOpenAIResponsesCollabPlaintextPayload(newCtx(true), payload)
		require.NoError(t, err)
		require.Equal(t, payload, restored)
	})

	// 含别名的载荷若不是合法 JSON，静默放行会把别名泄漏给客户端；
	// [DONE]/SSE 注释等合法非 JSON 项不含别名 needle，不受影响。
	t.Run("mapping with invalid JSON containing alias is an error", func(t *testing.T) {
		_, err := restoreOpenAIResponsesCollabPlaintextPayload(newCtx(true), []byte(`{"type":"error","detail":"collaboration__send_message`))
		require.Error(t, err)
	})

	t.Run("invalid JSON without alias needle passes through", func(t *testing.T) {
		payload := []byte(`data: [DONE]`)
		restored, err := restoreOpenAIResponsesCollabPlaintextPayload(newCtx(true), payload)
		require.NoError(t, err)
		require.Equal(t, payload, restored)
	})
}

func TestClearOpenAIResponsesCollabPlaintextMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	setOpenAIResponsesCollabPlaintextMapping(c, apicompat.ResponsesCollaborationPlaintextMapping{
		Calls: map[string]apicompat.ResponsesNamespaceName{
			"collaboration__send_message": {Namespace: "collaboration", Name: "send_message"},
		},
	})

	clearOpenAIResponsesCollabPlaintextMapping(c)

	_, ok := openAIResponsesCollabPlaintextMapping(c)
	require.False(t, ok)
}

func wsCollabFrameWithTools() []byte {
	return []byte(`{"type":"response.create","model":"gpt-5.5","tools":` + collabPlaintextToolsJSON + `,"input":"first"}`)
}

// H1 回归：会话尚未观测到任何 tools 声明时，仅因用户文本含 "collaboration"
// 就不得向 payload 注入空的 toolsRaw（会产生残缺 JSON）。
func TestOpenAIWSCollabPlaintextSession_NoDeclaredToolsDoesNotInject(t *testing.T) {
	session := &openAIWSCollabPlaintextSession{accountID: 1}
	payload := []byte(`{"type":"response.create","model":"gpt-5.5","previous_response_id":"resp_1","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"collaboration"}]}]}`)

	adapted, err := session.adaptPayload(payload)

	require.NoError(t, err)
	require.JSONEq(t, string(payload), string(adapted))
	require.False(t, gjson.GetBytes(adapted, "tools").Exists(), "no tools declaration observed → nothing to inject")
}

func TestOpenAIWSCollabPlaintextSession_AdaptInheritAndReset(t *testing.T) {
	session := &openAIWSCollabPlaintextSession{accountID: 1}

	// turn 1：显式声明 → 降级 + mapping。
	adapted1, err := session.adaptPayload(wsCollabFrameWithTools())
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(adapted1, `tools.#(name=="collaboration__send_message")`).Exists())
	require.False(t, gjson.GetBytes(adapted1, `tools.#(name=="collaboration__send_message").parameters.properties.message.encrypted`).Exists())

	// turn 2：省略 tools → 继承 turn 1 的客户端原始声明，重新降级；
	// input 里的 namespaced 调用也按本帧 mapping 改写。
	followup := []byte(`{"type":"response.create","model":"gpt-5.5","previous_response_id":"resp_1","input":[
		{"type":"function_call","namespace":"collaboration","name":"send_message","call_id":"c1","arguments":"{\"message\":\"again\"}"}
	]}`)
	adapted2, err := session.adaptPayload(followup)
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(adapted2, `tools.#(name=="collaboration__send_message")`).Exists(), "omitted tools must inherit the original declaration")
	require.Equal(t, "collaboration__send_message", gjson.GetBytes(adapted2, "input.0.name").String())
	require.False(t, gjson.GetBytes(adapted2, "input.0.namespace").Exists())

	// turn 3：显式空 tools → 整体替换并重算 mapping；旧别名不再改写。
	reset := []byte(`{"type":"response.create","model":"gpt-5.5","tools":[],"input":[
		{"type":"function_call","namespace":"collaboration","name":"send_message","call_id":"c2","arguments":"{}"}
	]}`)
	adapted3, err := session.adaptPayload(reset)
	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(adapted3, "tools").Array(), 0)
	require.Equal(t, "send_message", gjson.GetBytes(adapted3, "input.0.name").String(), "empty tools reset → no alias rewrite")
	require.Equal(t, "collaboration", gjson.GetBytes(adapted3, "input.0.namespace").String())

	// reset 后旧别名不再被还原。
	restored, err := session.restore([]byte(`{"output":[{"type":"function_call","name":"collaboration__send_message","call_id":"c3","arguments":"{}"}]}`))
	require.NoError(t, err)
	require.Equal(t, "collaboration__send_message", gjson.GetBytes(restored, "output.0.name").String(), "post-reset mapping must not restore the previous alias")
}

// 显式 tools 换成另一组 collaboration 工具时，上一帧的别名不得残留。
func TestOpenAIWSCollabPlaintextSession_ToolsReplacementRecomputesMapping(t *testing.T) {
	session := &openAIWSCollabPlaintextSession{accountID: 1}
	_, err := session.adaptPayload(wsCollabFrameWithTools())
	require.NoError(t, err)

	replacement := []byte(`{"type":"response.create","model":"gpt-5.5","tools":[
		{"type":"namespace","name":"collaboration","tools":[
			{"type":"function","name":"spawn_agent","parameters":{"type":"object","properties":{"message":{"type":"string","encrypted":true}}}}
		]}
	],"input":"second"}`)
	adapted, err := session.adaptPayload(replacement)
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(adapted, `tools.#(name=="collaboration__spawn_agent")`).Exists())
	require.False(t, gjson.GetBytes(adapted, `tools.#(name=="collaboration__send_message")`).Exists())

	restored, err := session.restore([]byte(`{"output":[{"type":"function_call","name":"collaboration__send_message","call_id":"c9","arguments":"{}"}]}`))
	require.NoError(t, err)
	require.Equal(t, "collaboration__send_message", gjson.GetBytes(restored, "output.0.name").String(), "stale alias from the previous declaration must not be restored")
}

func TestOpenAIWSCollabPlaintextSession_Restore(t *testing.T) {
	session := &openAIWSCollabPlaintextSession{accountID: 1}
	_, err := session.adaptPayload(wsCollabFrameWithTools())
	require.NoError(t, err)

	event := []byte(`{"type":"response.output_item.done","item":{"type":"function_call","name":"collaboration__send_message","call_id":"c1","arguments":"{\"message\":\"ok\"}"}}`)
	restored, err := session.restore(event)
	require.NoError(t, err)
	require.Equal(t, "send_message", gjson.GetBytes(restored, "item.name").String())
	require.Equal(t, "collaboration", gjson.GetBytes(restored, "item.namespace").String())
	require.Len(t, gjson.GetBytes(restored, "item.encrypted_function_args").Array(), 0)

	// 无别名帧原样。
	plain := []byte(`{"type":"response.output_text.delta","delta":"hi"}`)
	unchanged, err := session.restore(plain)
	require.NoError(t, err)
	require.Equal(t, plain, unchanged)

	// mapping 存在但载荷是残缺 JSON 且含别名 → 显式错误，不泄漏别名。
	_, err = session.restore([]byte(`{"item":{"name":"collaboration__send_message"`))
	require.Error(t, err)
}

// H2：会话按 accountID 绑定。A→B(opt-in) 切换保留客户端原始 tools 声明、
// 仅重置 mapping；B(off) 不返回会话也不动用旧 mapping。
func TestOpenAIWSCollabPlaintextSessionForAccount_BindsAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	accountA := collabPlaintextAPIKeyAccount()
	accountB := collabPlaintextAPIKeyAccount()
	accountB.ID = 7702
	accountOff := collabPlaintextAPIKeyAccount()
	accountOff.ID = 7703
	delete(accountOff.Extra, "openai_responses_plaintext_collaboration")

	sessionA, err := openAIWSCollabPlaintextSessionForAccount(c, accountA)
	require.NoError(t, err)
	require.NotNil(t, sessionA)
	_, err = sessionA.adaptPayload(wsCollabFrameWithTools())
	require.NoError(t, err)

	// B opt-in：同一 session 对象复用，但 mapping 按新账号重置；
	// 客户端声明的 tools 保留，follow-up 省略 tools 时仍能继承。
	sessionB, err := openAIWSCollabPlaintextSessionForAccount(c, accountB)
	require.NoError(t, err)
	require.Same(t, sessionA, sessionB)
	restored, err := sessionB.restore([]byte(`{"output":[{"type":"function_call","name":"collaboration__send_message","call_id":"c1","arguments":"{}"}]}`))
	require.NoError(t, err)
	require.Equal(t, "collaboration__send_message", gjson.GetBytes(restored, "output.0.name").String(), "account A's mapping must not restore account B's responses")

	followup := []byte(`{"type":"response.create","model":"gpt-5.5","input":"go"}`)
	adapted, err := sessionB.adaptPayload(followup)
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(adapted, `tools.#(name=="collaboration__send_message")`).Exists(), "client-declared tools must survive account failover")

	// B off：不返回会话；A 的 mapping 不会被用于还原。
	sessionOff, err := openAIWSCollabPlaintextSessionForAccount(c, accountOff)
	require.NoError(t, err)
	require.Nil(t, sessionOff)

	// unsupported：明确错误而不是静默放行。
	unsupported := collabPlaintextAPIKeyAccount()
	unsupported.ID = 7704
	unsupported.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeForceChatCompletions)
	sessionU, err := openAIWSCollabPlaintextSessionForAccount(c, unsupported)
	require.ErrorIs(t, err, errOpenAIResponsesCollabPlaintextUnsupported)
	require.Nil(t, sessionU)
}

func TestOpenAIWSCollabPlaintextSessionForAccount_OptOutAndSameIDReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	account := collabPlaintextAPIKeyAccount()
	session, err := openAIWSCollabPlaintextSessionForAccount(c, account)
	require.NoError(t, err)
	_, err = session.adaptPayload(wsCollabFrameWithTools())
	require.NoError(t, err)

	delete(account.Extra, "openai_responses_plaintext_collaboration")
	off, err := openAIWSCollabPlaintextSessionForAccount(c, account)
	require.NoError(t, err)
	require.Nil(t, off)
	account.Extra["openai_responses_plaintext_collaboration"] = true
	on, err := openAIWSCollabPlaintextSessionForAccount(c, account)
	require.NoError(t, err)
	restored, err := on.restore([]byte(`{"output":[{"type":"function_call","name":"collaboration__send_message","arguments":"{}"}]}`))
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(restored, "output.0.namespace").Exists(), "opt-out interval invalidates the earlier attempt mapping even for the same account ID")
	adapted, err := on.adaptPayload([]byte(`{"type":"response.create","input":"next"}`))
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(adapted, `tools.#(name=="collaboration__send_message")`).Exists(), "client declaration survives an off/on interval")
}

func TestOpenAIWSCollabPlaintextSession_ExplicitNullAndErrorState(t *testing.T) {
	session := &openAIWSCollabPlaintextSession{accountID: 1}
	_, err := session.adaptPayload(wsCollabFrameWithTools())
	require.NoError(t, err)
	_, err = session.adaptPayload([]byte(`{"type":"response.create","tools":[{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"send_message","parameters":{"type":"object","properties":{"message":{"type":"number","encrypted":true}}}}]}]}`))
	require.Error(t, err)
	adapted, err := session.adaptPayload([]byte(`{"type":"response.create","input":"next"}`))
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(adapted, `tools.#(name=="collaboration__send_message")`).Exists(), "failed frame must not replace the last valid client declaration")
	reset, err := session.adaptPayload([]byte(`{"type":"response.create","tools":null,"input":"reset"}`))
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(reset, "tools").Type == gjson.Null)
	_, err = session.adaptPayload([]byte(`{"type":"response.create","input":"after reset"}`))
	require.NoError(t, err)
	restored, err := session.restore([]byte(`{"output":[{"type":"function_call","name":"collaboration__send_message","arguments":"{}"}]}`))
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(restored, "output.0.namespace").Exists(), "explicit null clears the mapping")
}
