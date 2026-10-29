package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIWSIngressWebSearchActivation(t *testing.T) {
	const search = `,"tools":[{"type":"web_search","external_web_access":false}]`
	const userInput = `[{"type":"message","role":"user","content":"continue"}]`
	const toolOutput = `[{"type":"function_call_output","call_id":"fc_lookup","output":"42"}]`
	const firstOutput = `[{"type":"reasoning","id":"rs_first","summary":[{"type":"summary_text","text":"Keep this reasoning"}]},{"type":"message","id":"msg_first","role":"assistant","content":[{"type":"output_text","text":"Keep this answer"}]},{"type":"function_call","id":"fc_lookup","call_id":"fc_lookup","name":"lookup","arguments":"{}"}]`
	for _, tc := range []struct {
		name          string
		accountType   string
		firstFields   string
		firstInput    string
		firstOutput   string
		nextFields    string
		nextInput     string
		nextPrevious  string
		wantReconnect bool
		wantFailClose bool
		wantTypes     []string
	}{
		{name: "empty OAuth prewarm", accountType: AccountTypeOAuth, firstFields: `,"generate":false`, nextFields: search, nextInput: userInput, wantReconnect: true, wantTypes: []string{"message"}},
		{name: "Lite search history injection", accountType: AccountTypeOAuth, firstFields: `,"generate":false`, nextFields: `,"client_metadata":{"ws_request_header_x_openai_internal_codex_responses_lite":"true"}`, nextInput: `[{"type":"web_search_call","id":"ws_history","status":"completed","action":{"type":"search","query":"example"}},{"type":"message","role":"user","content":"continue"}]`, wantReconnect: true, wantTypes: []string{"web_search_call", "message", "additional_tools"}},
		{name: "tool continuation preserves all output", accountType: AccountTypeOAuth, firstFields: `,"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]`, firstInput: `[{"type":"message","role":"user","content":"Run lookup for my original question"}]`, firstOutput: firstOutput, nextFields: search, nextInput: toolOutput, wantReconnect: true, wantTypes: []string{"message", "reasoning", "message", "function_call", "function_call_output"}},
		{name: "search initialized socket", accountType: AccountTypeOAuth, firstFields: search, nextFields: search, nextInput: userInput},
		{name: "API key provider", accountType: AccountTypeAPIKey, firstFields: `,"generate":false`, nextFields: search, nextInput: userInput},
		{name: "function called web_search", accountType: AccountTypeOAuth, nextFields: `,"tools":[{"type":"function","name":"web_search","parameters":{"type":"object"}}]`, nextInput: userInput},
		{name: "missing tool context fails closed", accountType: AccountTypeOAuth, nextFields: search, nextInput: toolOutput, wantFailClose: true},
		{name: "unknown response history fails closed", accountType: AccountTypeOAuth, nextFields: search, nextInput: userInput, nextPrevious: "resp_unknown", wantFailClose: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
			cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
			cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
			output := tc.firstOutput
			if output == "" {
				output = "[]"
			}
			events := [][]byte{
				[]byte(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_first","model":"gpt-6.1-sol","output":%s,"usage":{"input_tokens":1,"output_tokens":1}}}`, output)),
				[]byte(`{"type":"response.completed","response":{"id":"resp_second","model":"gpt-6.1-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`),
				[]byte(`{"type":"response.completed","response":{"id":"resp_third","model":"gpt-6.1-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`),
				[]byte(`{"type":"response.completed","response":{"id":"resp_fourth","model":"gpt-6.1-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`),
			}
			first := &openAIWSCaptureConn{events: append([][]byte(nil), events...)}
			second := &openAIWSCaptureConn{events: append([][]byte(nil), events[1:]...)}
			dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{first, second}}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(dialer)
			t.Cleanup(pool.Close)
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{}, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool}
			account := &Account{ID: 71, Platform: PlatformOpenAI, Type: tc.accountType, Status: StatusActive, Schedulable: true, Concurrency: 1,
				Credentials: map[string]any{"access_token": "test-token", "api_key": "test-token"},
				Extra:       map[string]any{"responses_websockets_v2_enabled": true}}
			serverErr := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					serverErr <- err
					return
				}
				defer func() { _ = conn.CloseNow() }()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				_, payload, err := conn.Read(ctx)
				if err == nil {
					err = svc.ProxyResponsesWebSocketFromClient(ctx, c, conn, account, "test-token", payload, nil)
				}
				serverErr <- err
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer func() { _ = client.CloseNow() }()
			write := func(fields, input string) {
				payload := fmt.Sprintf(`{"type":"response.create","model":"gpt-6.1-sol","store":false,"input":%s%s}`, input, fields)
				require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(payload)))
			}
			read := func(id string) {
				_, event, err := client.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, id, gjson.GetBytes(event, "response.id").String())
			}
			input := tc.firstInput
			if input == "" {
				input = "[]"
			}
			write(tc.firstFields, input)
			read("resp_first")
			previous := tc.nextPrevious
			if previous == "" {
				previous = "resp_first"
			}
			write(fmt.Sprintf(`,"previous_response_id":%q`, previous)+tc.nextFields, tc.nextInput)
			if tc.wantFailClose {
				_, _, err := client.Read(ctx)
				require.Error(t, err)
				require.Error(t, <-serverErr)
				require.Len(t, first.writes, 1, "unsafe continuation must not reach upstream")
				return
			}
			read("resp_second")
			// Removing and restoring search must keep the search-initialized socket.
			write(`,"previous_response_id":"resp_second"`, userInput)
			read("resp_third")
			write(`,"previous_response_id":"resp_third"`+tc.nextFields, userInput)
			read("resp_fourth")
			require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
			require.NoError(t, <-serverErr)
			if !tc.wantReconnect {
				require.Equal(t, 1, dialer.DialCount())
				require.Len(t, first.writes, 4)
				return
			}
			require.Equal(t, 2, dialer.DialCount())
			require.Len(t, first.writes, 1, "search must never be sent on the incompatible socket")
			require.Len(t, second.writes, 3)
			forwarded, err := json.Marshal(second.writes[0])
			require.NoError(t, err)
			require.False(t, gjson.GetBytes(forwarded, "previous_response_id").Exists())
			var types []string
			for _, item := range gjson.GetBytes(forwarded, "input").Array() {
				types = append(types, item.Get("type").String())
			}
			require.Equal(t, tc.wantTypes, types)
			if tc.firstOutput != "" {
				require.Contains(t, string(forwarded), "Run lookup for my original question")
				require.Contains(t, string(forwarded), "Keep this reasoning")
				require.Contains(t, string(forwarded), "Keep this answer")
				require.Contains(t, string(forwarded), `"output":"42"`)
			}
		})
	}
}

func TestOpenAIWSConnWebSearchInitialization(t *testing.T) {
	const search = `{"type":"response.create","tools":[{"type":"web_search"}]}`
	for _, first := range []struct {
		name          string
		body          string
		wantReconnect bool
	}{
		{"empty prewarm", `{"type":"response.create","generate":false,"input":[]}`, true},
		{"hosted search", search, false},
		{"Lite hosted search", `{"type":"response.create","input":[{"type":"additional_tools","tools":[{"type":"web_search"}]}]}`, false},
		{"function lookalike", `{"type":"response.create","tools":[{"type":"function","name":"web_search"}]}`, true},
	} {
		for _, format := range []string{"map", "raw", "bytes"} {
			t.Run(first.name+"/"+format, func(t *testing.T) {
				conn := newOpenAIWSConn("test", 1, &openAIWSCaptureConn{}, nil)
				defer conn.close()
				lease := &openAIWSConnLease{conn: conn}
				require.False(t, lease.initializedWithoutWebSearch(), "an unused socket is compatible")
				var payload any
				switch format {
				case "map":
					require.NoError(t, json.Unmarshal([]byte(first.body), &payload))
				case "raw":
					payload = json.RawMessage(first.body)
				case "bytes":
					payload = []byte(first.body)
				}
				require.NoError(t, lease.WriteJSON(payload, time.Second))
				// Capability belongs to the socket, even after it is leased again.
				lease = &openAIWSConnLease{conn: conn}
				require.Equal(t, first.wantReconnect, lease.initializedWithoutWebSearch())
				require.NoError(t, lease.WriteJSON(json.RawMessage(search), time.Second))
				require.Equal(t, first.wantReconnect, lease.initializedWithoutWebSearch(), "later writes cannot change initial capability")
			})
		}
	}
}
