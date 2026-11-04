package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Opt-in local integration: real installed Codex -> actual Forward pipeline ->
// deterministic SSE upstream. No real account or external model is contacted.
func TestCodexNormalCLIFlow(t *testing.T) {
	binary := os.Getenv("SUB2API_CODEX_CLI")
	if binary == "" {
		t.Skip("set SUB2API_CODEX_CLI to the installed native Codex binary")
	}
	version, err := exec.Command(binary, "--version").CombinedOutput()
	require.NoError(t, err)
	require.Equal(t, "codex-cli "+codexCLIVersion, strings.TrimSpace(string(version)))
	emit := func(events ...map[string]any) string {
		var out strings.Builder
		for _, event := range events {
			b, e := json.Marshal(event)
			require.NoError(t, e)
			fmt.Fprintf(&out, "event: %s\ndata: %s\n\n", event["type"], b)
		}
		return out.String()
	}
	call := map[string]any{"id": "fc_normal_tool", "type": "function_call", "call_id": "call_normal_tool", "name": "exec_command", "arguments": `{"cmd":"printf normal-tool-ok","max_output_tokens":100}`, "status": "completed"}
	message := map[string]any{"id": "msg_normal_final", "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": "normal-flow-ok", "annotations": []any{}}}}
	stream := func(id string, item map[string]any) *http.Response {
		response := map[string]any{"id": id, "status": "completed", "model": "gpt-6.1-sol", "output": []any{item}, "usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15, "input_tokens_details": map[string]any{"cached_tokens": 0}, "output_tokens_details": map[string]any{"reasoning_tokens": 0}}}
		events := []map[string]any{
			{"type": "response.created", "response": map[string]any{"id": id, "status": "in_progress", "output": []any{}}},
			{"type": "response.output_item.added", "output_index": 0, "item": item},
			{"type": "response.output_item.done", "output_index": 0, "item": item},
			{"type": "response.completed", "response": response},
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(emit(events...)))}
	}
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprintf("passthrough=%t", passthrough), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{stream("resp_normal_tool", call), stream("resp_normal_final", message)}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "local-fixture", "chatgpt_account_id": "local-account"}, Extra: map[string]any{"openai_passthrough": passthrough}}
			var incomingBodies [][]byte
			var incomingHeaders []http.Header
			complete := make(chan error, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, e := io.ReadAll(r.Body)
				if e != nil {
					http.Error(w, e.Error(), 400)
					complete <- e
					return
				}
				incomingBodies = append(incomingBodies, body)
				incomingHeaders = append(incomingHeaders, r.Header.Clone())
				c, _ := gin.CreateTestContext(w)
				c.Request = r
				_, e = svc.Forward(r.Context(), c, account, body)
				complete <- e
			}))
			defer server.Close()
			home := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			args := []string{"exec", "--ignore-user-config", "--skip-git-repo-check", "--ephemeral", "--disable", "plugins", "--disable", "recommended_plugins", "-C", home, "-s", "read-only", "-m", "gpt-6.1-sol",
				"-c", `approval_policy="never"`,
				"-c", `model_provider="local_probe"`,
				"-c", `model_providers.local_probe.name="local_probe"`,
				"-c", fmt.Sprintf(`model_providers.local_probe.base_url="%s/v1"`, server.URL),
				"-c", `model_providers.local_probe.wire_api="responses"`,
				"-c", `model_providers.local_probe.request_max_retries=0`,
				"-c", `model_providers.local_probe.stream_max_retries=0`,
				"Run printf normal-tool-ok once, then reply normal-flow-ok.",
			}
			cmd := exec.CommandContext(ctx, binary, args...)
			for _, e := range os.Environ() {
				key := strings.ToUpper(strings.SplitN(e, "=", 2)[0])
				if key == "TERM" || strings.Contains(key, "CODEX") || strings.Contains(key, "TOKEN") || strings.Contains(key, "API_KEY") || strings.Contains(key, "PROXY") {
					continue
				}
				cmd.Env = append(cmd.Env, e)
			}
			cmd.Env = append(cmd.Env, "CODEX_HOME="+home)
			output, runErr := cmd.CombinedOutput()
			if dir := os.Getenv("SUB2API_FLOW_EVIDENCE"); dir != "" {
				dir = filepath.Join(dir, fmt.Sprintf("passthrough-%t", passthrough))
				require.NoError(t, os.MkdirAll(dir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "cli.log"), output, 0644))
				for i, body := range incomingBodies {
					require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("input-%d.json", i+1)), body, 0644))
				}
				for i, body := range upstream.bodies {
					require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("upstream-%d.json", i+1)), body, 0644))
				}
				headers := []http.Header{}
				for _, r := range upstream.requests {
					h := r.Header.Clone()
					h.Del("Authorization")
					headers = append(headers, h)
				}
				b, err := json.MarshalIndent(headers, "", "  ")
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, "upstream-headers.json"), b, 0644))
			}
			require.NoError(t, runErr, string(output))
			require.Contains(t, string(output), "normal-flow-ok")
			require.Len(t, upstream.bodies, 2, string(output))
			for i := 0; i < 2; i++ {
				require.NoError(t, <-complete)
				req := upstream.requests[i]
				body := upstream.bodies[i]
				require.Equal(t, codexCLIUserAgent, req.Header.Get("User-Agent"))
				require.Equal(t, codexCLIVersion, req.Header.Get("Version"))
				require.Equal(t, "codex_exec", req.Header.Get("Originator"))
				require.Equal(t, "text/event-stream", req.Header.Get("Accept"))
				require.Equal(t, "application/json", req.Header.Get("Content-Type"))
				require.Equal(t, "/backend-api/codex/responses", req.URL.Path)
				require.False(t, gjson.GetBytes(body, "instructions").Exists())
				require.Equal(t, "gpt-6.1-sol", gjson.GetBytes(body, "model").String())
				for _, key := range []string{"tools", "tool_choice", "parallel_tool_calls", "reasoning", "include", "stream", "store", "text"} {
					before, after := gjson.GetBytes(incomingBodies[i], key), gjson.GetBytes(body, key)
					require.Equal(t, before.Exists(), after.Exists(), key)
					if before.Exists() {
						require.JSONEq(t, before.Raw, after.Raw, key)
					}
				}
				// Existing call-id canonicalization is allowed; the tools, prompt,
				// arguments and output must otherwise survive unchanged.
				var beforeInput, afterInput []map[string]any
				require.NoError(t, json.Unmarshal([]byte(gjson.GetBytes(incomingBodies[i], "input").Raw), &beforeInput))
				require.NoError(t, json.Unmarshal([]byte(gjson.GetBytes(body, "input").Raw), &afterInput))
				for _, items := range [][]map[string]any{beforeInput, afterInput} {
					for _, item := range items {
						if item["type"] == "function_call" || item["type"] == "function_call_output" {
							delete(item, "call_id")
						}
					}
				}
				require.Equal(t, beforeInput, afterInput)
				if i == 1 {
					callID := gjson.GetBytes(body, `input.#(type=="function_call").call_id`).String()
					outputID := gjson.GetBytes(body, `input.#(type=="function_call_output").call_id`).String()
					require.NotEmpty(t, callID)
					require.Equal(t, callID, outputID)
				}
				for _, key := range []string{"session-id", "thread-id", "x-client-request-id", "x-codex-window-id", "x-codex-turn-metadata", "x-codex-beta-features"} {
					require.NotEmpty(t, incomingHeaders[i].Get(key), key)
					require.NotEmpty(t, req.Header.Get(key), key)
				}
				require.Equal(t, gjson.GetBytes(body, "client_metadata.session_id").String(), req.Header.Get("session-id"))
				require.Equal(t, gjson.GetBytes(body, "client_metadata.thread_id").String(), req.Header.Get("thread-id"))
				require.Equal(t, gjson.GetBytes(body, "client_metadata.x-codex-window-id").String(), req.Header.Get("x-codex-window-id"))
				require.JSONEq(t, gjson.GetBytes(body, "client_metadata.x-codex-turn-metadata").String(), req.Header.Get("x-codex-turn-metadata"))
				require.Equal(t, incomingHeaders[i].Get("x-codex-beta-features"), req.Header.Get("x-codex-beta-features"))
			}
			result := gjson.GetBytes(upstream.bodies[1], `input.#(type=="function_call_output").output`).String()
			require.Contains(t, result, "Process exited with code 0")
			require.Contains(t, result, "Output:\nnormal-tool-ok")
			t.Log("CLI 0.166.0 -> Forward -> tool exec -> follow-up -> final answer: verified")
		})
	}
}
