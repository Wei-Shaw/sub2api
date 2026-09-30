//go:build unit

package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCreateOpenAIPelicanTestPayloadMatchesCockpitTools(t *testing.T) {
	payload := createOpenAIPelicanTestPayload("gpt-6-astra")
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	require.Equal(t, "gpt-6-astra", gjson.GetBytes(body, "model").String())
	require.Equal(t, openAIPelicanTestPrompt, gjson.GetBytes(body, "input.0.content.0.text").String())
	require.Equal(t, openAIPelicanInstructions, gjson.GetBytes(body, "instructions").String())
	require.Equal(t, "medium", gjson.GetBytes(body, "reasoning.effort").String())
	require.Equal(t, "auto", gjson.GetBytes(body, "reasoning.summary").String())
	require.False(t, gjson.GetBytes(body, "store").Bool())
}

func TestExtractOpenAIPelicanHTML(t *testing.T) {
	require.Equal(t,
		"<!DOCTYPE html><html><body>pelican</body></html>",
		extractOpenAIPelicanHTML("notes\n<!DOCTYPE html><html><body>pelican</body></html>\ntrailing"),
	)
	require.Equal(t,
		"<html><body>pelican</body></html>",
		extractOpenAIPelicanHTML("```html\n<html><body>pelican</body></html>\n```"),
	)
	require.Empty(t, extractOpenAIPelicanHTML("I would create an inline SVG."))
}

func TestProcessOpenAIPelicanStreamEmitsPreviewArtifact(t *testing.T) {
	c, recorder := newTestContext()
	stream := strings.NewReader(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"<!DOCTYPE html><html><body>\"}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"pelican</body></html>\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-6-astra\"}}\n\n",
	)

	err := (&AccountTestService{}).processOpenAIPelicanStream(c, stream)
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), `"type":"pelican_result"`)
	require.Contains(t, recorder.Body.String(), `"has_html":true`)
	require.Contains(t, recorder.Body.String(), `"response_id":"resp_1"`)
	require.Contains(t, recorder.Body.String(), `"response_model":"gpt-6-astra"`)
	require.NotContains(t, recorder.Body.String(), `"type":"degradation_result"`)
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestProcessOpenAIPelicanStreamReportsMissingHTML(t *testing.T) {
	c, recorder := newTestContext()
	stream := strings.NewReader(
		"data: {\"type\":\"response.completed\",\"response\":{\"output\":[" +
			"{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"I cannot create that.\"}]}]}}\n\n",
	)

	err := (&AccountTestService{}).processOpenAIPelicanStream(c, stream)
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), `"has_html":false`)
	require.Contains(t, recorder.Body.String(), `"reply_preview":"I cannot create that."`)
}

func TestOpenAIAccountPelicanModeUsesChatGPTProbe(t *testing.T) {
	c, recorder := newTestContext()
	response := newJSONResponse(http.StatusOK, "")
	response.Body = io.NopCloser(strings.NewReader(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"<html></html>\"}\n\n" +
			"data: {\"type\":\"response.completed\"}\n\n",
	))
	upstream := &queuedHTTPUpstream{responses: []*http.Response{response}}
	service := &AccountTestService{httpUpstream: upstream}
	account := &Account{
		ID:          90,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := service.testOpenAIAccountConnection(c, account, "gpt-6-astra", "ignored", AccountTestModePelican)
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, chatgptCodexAPIURL, upstream.requests[0].URL.String())
	body, err := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, err)
	require.Equal(t, openAIPelicanTestPrompt, gjson.GetBytes(body, "input.0.content.0.text").String())
	require.Contains(t, recorder.Body.String(), `"has_html":true`)
}

func TestOpenAIAccountPelicanModeRejectsAPIKey(t *testing.T) {
	c, recorder := newTestContext()
	service := &AccountTestService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	err := service.testOpenAIAccountConnection(c, account, "gpt-6-astra", "", AccountTestModePelican)
	require.ErrorContains(t, err, "requires an OpenAI OAuth or setup-token account")
	require.Contains(t, recorder.Body.String(), `"type":"error"`)
}
