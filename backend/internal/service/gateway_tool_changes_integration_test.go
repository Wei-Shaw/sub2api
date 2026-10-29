package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func TestGatewayToolChangesOutgoingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mimic := range []bool{true, false} {
		name := "api-key"
		if mimic {
			name = "oauth-mimic"
		}
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Request.Header.Set("Anthropic-Beta", "inline-tools-2026-09-15,mid-conversation-tool-changes-2026-07-01")
			account := &Account{ID: 401, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "synthetic"}, Status: StatusActive, Schedulable: true}
			authType := "oauth"
			if !mimic {
				account.Type = AccountTypeAPIKey
				account.Credentials = map[string]any{"api_key": "synthetic"}
				authType = "apikey"
			}
			body := []byte(`{"model":"claude-opus-5-5","max_tokens":8,"stream":true,"tools":[{"name":"read","input_schema":{"type":"object"}},{"name":"bash","input_schema":{"type":"object"}},{"name":"codemode","input_schema":{"type":"object"}},{"name":"subagent_supervisor","input_schema":{"type":"object"}},{"name":"cron_silent","input_schema":{"type":"object"}},{"name":"__pi_deferred_placeholder__","input_schema":{"type":"object"},"defer_loading":true}],"messages":[{"role":"user","content":"synthetic"},{"role":"assistant","content":"OK"},{"role":"user","content":"Reply OK"},{"role":"system","content":[{"type":"tool_removal","tool":{"type":"tool_reference","name":"subagent_supervisor"}},{"type":"tool_addition","tool":{"type":"tool_definition","definition":{"name":"codemode","input_schema":{"type":"object"}}}}]}]}`)
			cfg := &config.Config{}
			cfg.Gateway.InjectBetaForAPIKey = true
			svc := &GatewayService{cfg: cfg}
			if mimic {
				body = svc.applyClaudeCodeOAuthMimicryToBody(context.Background(), c, account, body, nil, "claude-opus-5-5")
			}
			req, _, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "synthetic", authType, "claude-opus-5-5", false, mimic)
			if err != nil {
				t.Fatal(err)
			}
			wire, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			wantRemoval, wantDefinition := "subagent_supervisor", "codemode"
			if mimic {
				rw := toolNameRewriteFromContext(c)
				if rw == nil {
					t.Fatal("OAuth mimic fixture did not use the production rewrite path")
				}
				wantRemoval, wantDefinition = rw.Forward[wantRemoval], rw.Forward[wantDefinition]
			}
			var removal, definition string
			var changes int
			gjson.GetBytes(wire, "messages").ForEach(func(_, message gjson.Result) bool {
				message.Get("content").ForEach(func(_, block gjson.Result) bool {
					switch block.Get("type").String() {
					case "tool_removal":
						removal = block.Get("tool.name").String()
						changes++
					case "tool_addition":
						definition = block.Get("tool.definition.name").String()
						changes++
					}
					return true
				})
				return true
			})
			if removal != wantRemoval || definition != wantDefinition || changes != 2 {
				t.Errorf("outgoing changes = (%q, %q, %d); want declared aliases (%q, %q, 2)", removal, definition, changes, wantRemoval, wantDefinition)
			}
			if !strings.Contains(getHeaderRaw(req.Header, "anthropic-beta"), "inline-tools-2026-09-15") {
				t.Error("required inline-tools beta was lost during request construction")
			}
			if gjson.GetBytes(wire, "messages.#").Int() < 4 {
				t.Error("request construction dropped historical messages")
			}
		})
	}
}
