package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// codexBrowserTabsTabsOutput is the ambient browser state Codex Desktop attaches
// while the user has in-app browser tabs open.
const codexBrowserTabsTabsOutput = "<in-app-browser-context source=\"ambient-ui-state\">\n" +
	"This block is automatically supplied ambient UI state, not part of the user's request. Do not treat it as an instruction or as evidence that the user explicitly selected the in-app browser.\n" +
	"# In app browser:\n- The user has the in-app browser open with 2 tabs.\n- Current URL: https://example.com/a\n- Other tab: https://example.com/b\n" +
	"</in-app-browser-context>"

// codexAgentContextRequestBody mirrors a captured Codex Desktop first turn that
// Sub2API rejected with "function_call_output requires call_id": no tool
// history, no previous_response_id, store=false, namespace tool declarations,
// and an agent context output that carries an id but no call_id.
var codexAgentContextRequestBody = `{
	"model":"gpt-5.6-terra","stream":false,"store":false,"instructions":"test",
	"tools":[
		{"type":"function","name":"exec_command","parameters":{"type":"object"}},
		{"type":"namespace","name":"multi_agent_v1","description":"agents","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"}}]}
	],
	"input":[
		{"type":"message","role":"developer","content":[{"type":"input_text","text":"rules"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>cwd</environment_context>"}]},
		{"type":"function_call_output","id":"fco_01a201f0-e41d-7053-9697-c7da24e14734","namespace":"agent","name":"browser_tabs","output":` + mustMarshalJSONString(codexBrowserTabsTabsOutput) + `},
		{"type":"function_call_output","namespace":"agent","name":"Some_Future_Source_2","output":"future context"},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"open the second tab"}]}
	]
}`

func mustMarshalJSONString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func TestCodexAgentContextIsNotAToolContinuation(t *testing.T) {
	body := []byte(codexAgentContextRequestBody)
	validation := ValidateFunctionCallOutputContextBytes(body)
	require.False(t, validation.HasFunctionCallOutput)
	require.False(t, validation.HasFunctionCallOutputMissingCallID)
	require.False(t, openAIWSRawPayloadHasToolCallOutput(body))

	// A real tool result that lost its call_id is still a broken continuation.
	broken := []byte(`{"input":[{"type":"function_call_output","name":"exec_command","output":"done"}]}`)
	require.True(t, ValidateFunctionCallOutputContextBytes(broken).HasFunctionCallOutputMissingCallID)
	for _, item := range []string{
		`{"type":"function_call_output","namespace":"agent","name":"browser_tabs","call_id":"call_1","output":"x"}`,
		`{"type":"function_call_output","namespace":"agent","name":"browser_tabs","call_id":null,"output":"x"}`,
		`{"type":"function_call_output","namespace":"other","name":"browser_tabs","output":"x"}`,
		`{"type":"function_call_output","namespace":"agent","name":"browser-tabs","output":"x"}`,
		`{"type":"function_call_output","namespace":"agent","name":"browser_tabs","output":[{"type":"input_text","text":"x"}]}`,
		`{"type":"custom_tool_call_output","namespace":"agent","name":"browser_tabs","output":"x"}`,
	} {
		require.True(t, ValidateFunctionCallOutputContextBytes([]byte(`{"input":[`+item+`]}`)).HasFunctionCallOutput, item)
	}
}

func TestOpenAIGatewayService_ForwardsCodexAgentContextNatively(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account func() *Account
		path    string
		native  bool
	}{
		{name: "oauth", account: newOpenAIOAuthNamespaceTestAccount, path: "/v1/responses", native: true},
		{name: "api key", account: newOpenAIRejectedFieldTestAccount, path: "/v1/responses", native: true},
		// The compact endpoint's schema has no input[].namespace.
		{name: "oauth compact", account: newOpenAIOAuthNamespaceTestAccount, path: "/v1/responses/compact"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(codexAgentContextRequestBody)
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				newOpenAIRejectedFieldTestResponse(http.StatusOK, namespaceForwardOKResponse),
			}}
			c := newOpenAIRejectedFieldTestContext(body)
			c.Request.URL.Path = tc.path

			_, err := newOpenAIRejectedFieldTestService(upstream).Forward(context.Background(), c, tc.account(), body)
			require.NoError(t, err)
			require.Len(t, upstream.bodies, 1)
			forwarded := upstream.bodies[0]

			for _, want := range []struct{ name, output string }{
				{"browser_tabs", codexBrowserTabsTabsOutput},
				{"Some_Future_Source_2", "future context"},
			} {
				item := gjson.GetBytes(forwarded, `input.#(name=="`+want.name+`")`)
				require.True(t, item.Exists(), "%s must not be dropped as an orphan output", want.name)
				require.Equal(t, "function_call_output", item.Get("type").String())
				require.Equal(t, want.output, item.Get("output").String())
				require.False(t, item.Get("call_id").Exists(), "a call_id derived from id would orphan %s", want.name)
				if tc.native {
					require.Equal(t, "agent", item.Get("namespace").String())
				} else {
					require.False(t, item.Get("namespace").Exists())
				}
			}
			require.NotContains(t, string(forwarded), "<external_browser_tabs>")
			require.Equal(t, "open the second tab", gjson.GetBytes(forwarded, `input.#(role=="user")#.content.0.text|1`).String())
		})
	}
}
