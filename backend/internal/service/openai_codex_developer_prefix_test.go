package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_CodexOAuthPreservesDeveloperPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const developer = `{"type":"message","role":"developer","content":[{"type":"input_text","text":"Use the caller's project rules."}]}`
	const tools = `{"type":"additional_tools","role":"developer","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`
	const user = `{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}`
	for _, tc := range []struct {
		name         string
		input        string
		instructions string
		model        string
		mappedModel  string
		codex        bool
		lite         bool
		wantFallback bool
	}{
		{name: "6.2 plain Codex", input: developer + "," + user, model: "gpt-6.2-sol", codex: true},
		{name: "6.1 plain Codex", input: developer + "," + user, model: "gpt-6.1-sol", codex: true},
		{name: "Lite prefix and model mapping", input: tools + "," + developer + "," + user, model: "gpt-6.2-astra", mappedModel: "gpt-6.2-sol", codex: true, lite: true, instructions: `,"instructions":" "`},
		{name: "caller explicit instructions", input: developer + "," + user, model: "gpt-6.2-sol", codex: true, instructions: `,"instructions":"Only this turn."`},
		{name: "non-Codex retains fallback", input: developer + "," + user, model: "gpt-6.2-sol", wantFallback: true},
		{name: "Codex without developer prefix retains fallback", input: user, model: "gpt-6.2-sol", codex: true, wantFallback: true},
		{name: "Codex later developer is not a prefix", input: user + "," + developer, model: "gpt-6.2-sol", codex: true, wantFallback: true},
		{name: "Codex blank developer retains fallback", input: `{"type":"message","role":"developer","content":[{"type":"input_text","text":" "}]}` + "," + user, model: "gpt-6.2-sol", codex: true, wantFallback: true},
		{name: "invalid instructions retain existing handling", input: developer + "," + user, model: "gpt-6.2-sol", codex: true, instructions: `,"instructions":{}`, wantFallback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"model":"` + tc.model + `","stream":true,"input":[` + tc.input + `]` + tc.instructions + `}`
			account := instructionsEchoOAuthAccount(false)
			wantModel := tc.model
			if tc.mappedModel != "" {
				account.Credentials["model_mapping"] = map[string]any{tc.model: tc.mappedModel}
				wantModel = tc.mappedModel
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, openAIResponsesEndpoint, nil)
			if tc.codex {
				c.Request.Header.Set("User-Agent", "codex-tui/0.171.0")
				c.Request.Header.Set("Originator", "codex-tui")
			} else {
				c.Request.Header.Set("User-Agent", "OpenAI/Python 2.8.0")
			}
			if tc.lite {
				c.Request.Header.Set(responsesLiteHeader, "true")
			}
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			upstream := instructionsEchoUpstream("null")
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			result, err := svc.Forward(context.Background(), c, account, []byte(body))
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, wantModel, gjson.GetBytes(upstream.lastBody, "model").String())
			outgoing := gjson.GetBytes(upstream.lastBody, "instructions")
			if tc.wantFallback {
				require.Equal(t, defaultCodexSynthInstructions(wantModel), outgoing.String())
				return
			}
			incoming := gjson.Get(body, "instructions")
			require.Equal(t, incoming.Exists(), outgoing.Exists(), "must not synthesize instructions")
			require.Equal(t, incoming.String(), outgoing.String(), "must preserve explicit instructions")
			require.JSONEq(t, gjson.Get(body, "input").Raw, gjson.GetBytes(upstream.lastBody, "input").Raw)
		})
	}
}
