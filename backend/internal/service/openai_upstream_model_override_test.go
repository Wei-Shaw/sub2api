package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRewriteOpenAIFinalUpstreamBody(t *testing.T) {
	t.Parallel()

	openAIAccount := &Account{Platform: PlatformOpenAI}
	otherAccount := &Account{Platform: PlatformGrok}

	tests := []struct {
		name    string
		account *Account
		body    string
		path    string
		want    string
	}{
		{name: "root model", account: openAIAccount, body: `{"model":"gpt-5.6-sol","input":"hi"}`, path: "model", want: openAITerraModel},
		{name: "session model", account: openAIAccount, body: `{"type":"session.update","session":{"model":"gpt-5.6-sol"}}`, path: "session.model", want: openAITerraModel},
		{name: "terra unchanged", account: openAIAccount, body: `{"model":"gpt-5.6-terra"}`, path: "model", want: openAITerraModel},
		{name: "similar model unchanged", account: openAIAccount, body: `{"model":"gpt-5.6-sol-max"}`, path: "model", want: "gpt-5.6-sol-max"},
		{name: "other platform unchanged", account: otherAccount, body: `{"model":"gpt-5.6-sol"}`, path: "model", want: openAISolModel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := []byte(tt.body)
			got := rewriteOpenAIFinalUpstreamBody(tt.account, body)
			require.Equal(t, tt.want, gjson.GetBytes(got, tt.path).String())
			require.Equal(t, tt.body, string(body), "wire rewrite must not mutate the logical request body")
		})
	}
}

func TestBuildUpstreamRequest_RewritesSolOnlyOnWire(t *testing.T) {
	t.Parallel()

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
	}
	body := []byte(`{"model":"gpt-5.6-sol","input":"hello"}`)

	req, err := svc.buildUpstreamRequest(t.Context(), c, account, body, "sk-test", false, "", false)
	require.NoError(t, err)
	require.Equal(t, openAITerraModel, gjson.GetBytes(readRequestBodyForTest(t, req), "model").String())
	require.Equal(t, openAISolModel, gjson.GetBytes(body, "model").String())

	result := &OpenAIForwardResult{Model: openAISolModel, UpstreamModel: openAISolModel}
	require.Empty(t, (ChannelMappingResult{}).BuildModelMappingChain(result.Model, result.UpstreamModel))
}

func TestRewriteOpenAIFinalUpstreamPayload_DoesNotMutateNestedSession(t *testing.T) {
	t.Parallel()

	session := map[string]any{"model": openAISolModel, "voice": "alloy"}
	payload := map[string]any{"type": "session.update", "session": session}

	rewriteOpenAIFinalUpstreamPayload(&Account{Platform: PlatformOpenAI}, payload)

	require.Equal(t, openAISolModel, session["model"])
	require.Equal(t, openAITerraModel, payload["session"].(map[string]any)["model"])
}

func TestRestoreOpenAIFinalOpenAIResponseBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		logicalModel string
		body         string
		path         string
		want         string
	}{
		{name: "root", logicalModel: openAISolModel, body: `{"model":"gpt-5.6-terra"}`, path: "model", want: openAISolModel},
		{name: "response", logicalModel: openAISolModel, body: `{"response":{"model":"gpt-5.6-terra"}}`, path: "response.model", want: openAISolModel},
		{name: "session", logicalModel: openAISolModel, body: `{"session":{"model":"gpt-5.6-terra"}}`, path: "session.model", want: openAISolModel},
		{name: "explicit terra", logicalModel: openAITerraModel, body: `{"model":"gpt-5.6-terra"}`, path: "model", want: openAITerraModel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := restoreOpenAIFinalOpenAIResponseBody(tt.logicalModel, []byte(tt.body))
			require.Equal(t, tt.want, gjson.GetBytes(got, tt.path).String())
		})
	}
}

func TestUsesOpenAISolToTerraOverride_UsesFinalLogicalModel(t *testing.T) {
	t.Parallel()

	account := &Account{Platform: PlatformOpenAI}
	tests := []struct {
		name  string
		input *OpenAIRecordUsageInput
		want  bool
	}{
		{name: "result upstream sol", input: &OpenAIRecordUsageInput{Result: &OpenAIForwardResult{Model: "alias", UpstreamModel: openAISolModel}}, want: true},
		{name: "missing upstream fails closed", input: &OpenAIRecordUsageInput{Result: &OpenAIForwardResult{Model: openAISolModel}, ChannelUsageFields: ChannelUsageFields{ChannelMappedModel: openAISolModel}}},
		{name: "requested sol mapped elsewhere", input: &OpenAIRecordUsageInput{Result: &OpenAIForwardResult{Model: openAISolModel}, ChannelUsageFields: ChannelUsageFields{OriginalModel: openAISolModel, ChannelMappedModel: openAITerraModel}}},
		{name: "explicit upstream wins", input: &OpenAIRecordUsageInput{Result: &OpenAIForwardResult{Model: openAISolModel, UpstreamModel: openAITerraModel}, ChannelUsageFields: ChannelUsageFields{ChannelMappedModel: openAISolModel}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, usesOpenAISolToTerraOverride(account, tt.input))
		})
	}
}

func TestOpenAIWSPassthroughKeepsDirectSolForBilling(t *testing.T) {
	t.Parallel()

	result := &OpenAIForwardResult{
		Model:         openAISolModel,
		UpstreamModel: openAIWSDifferentModel(openAISolModel, openAISolModel),
	}
	require.Equal(t, openAISolModel, result.UpstreamModel)
	require.True(t, usesOpenAISolToTerraOverride(&Account{Platform: PlatformOpenAI}, &OpenAIRecordUsageInput{Result: result}))
	require.Empty(t, openAIWSDifferentModel("gpt-5.4", "gpt-5.4"), "unchanged models keep existing sparse metadata")
}

func TestHandleNonStreamingResponse_HidesSolWireOverride(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_sol","model":"gpt-5.6-terra","usage":{"input_tokens":1,"output_tokens":1}}`)),
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	result, err := svc.handleNonStreamingResponse(t.Context(), resp, c, account, openAISolModel, openAISolModel)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, openAISolModel, gjson.Get(recorder.Body.String(), "model").String())
	require.NotContains(t, recorder.Body.String(), openAITerraModel)
	require.Equal(t, openAITerraModel, observedUpstreamResponseModel(c))
	require.False(t, observedUpstreamResponseModelConflict(c))
}
