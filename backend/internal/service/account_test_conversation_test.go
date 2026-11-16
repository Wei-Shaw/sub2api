package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newAccountTestConversationContext(t *testing.T, conv *AccountTestConversation) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)
	bindAccountTestConversation(c, conv)
	return c, recorder
}

func sampleAccountTestConversation() *AccountTestConversation {
	temperature := 0.2
	return &AccountTestConversation{
		Messages: []AccountTestMessage{
			{Role: "user", Content: "first question"},
			{Role: "assistant", Content: "first answer"},
			{Role: "user", Content: "second question"},
		},
		System:      "  be brief  ",
		MaxTokens:   2048,
		Temperature: &temperature,
	}
}

// roundTrip normalizes a payload the way it reaches the upstream.
func roundTrip(t *testing.T, payload any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return decoded
}

func TestAccountTestConversationValidate(t *testing.T) {
	valid := func() *AccountTestConversation { return sampleAccountTestConversation() }
	tooHot := 2.5

	cases := []struct {
		name    string
		mutate  func(conv *AccountTestConversation)
		wantErr string
	}{
		{"valid", func(*AccountTestConversation) {}, ""},
		{"no messages", func(conv *AccountTestConversation) { conv.Messages = nil }, "must not be empty"},
		{"unknown role", func(conv *AccountTestConversation) { conv.Messages[1].Role = "system" }, "role must be"},
		{"blank content", func(conv *AccountTestConversation) { conv.Messages[0].Content = " \n" }, "content must not be empty"},
		{"invalid utf8", func(conv *AccountTestConversation) { conv.Messages[0].Content = "a\xffb" }, "valid UTF-8"},
		{"starts with assistant", func(conv *AccountTestConversation) { conv.Messages = conv.Messages[1:] }, "first message"},
		{"ends with assistant", func(conv *AccountTestConversation) { conv.Messages = conv.Messages[:2] }, "last message"},
		{"too many messages", func(conv *AccountTestConversation) {
			conv.Messages = make([]AccountTestMessage, accountTestConversationMaxMessages+1)
			for i := range conv.Messages {
				conv.Messages[i] = AccountTestMessage{Role: "user", Content: "x"}
			}
		}, "too many messages"},
		{"content too large", func(conv *AccountTestConversation) {
			conv.Messages[0].Content = strings.Repeat("x", accountTestConversationMaxContentBytes+1)
		}, "too large"},
		{"system too large", func(conv *AccountTestConversation) {
			conv.System = strings.Repeat("x", accountTestConversationMaxSystemBytes+1)
		}, "system prompt is too large"},
		{"negative max tokens", func(conv *AccountTestConversation) { conv.MaxTokens = -1 }, "max_tokens"},
		{"max tokens over limit", func(conv *AccountTestConversation) {
			conv.MaxTokens = accountTestConversationMaxTokensLimit + 1
		}, "max_tokens"},
		{"temperature out of range", func(conv *AccountTestConversation) { conv.Temperature = &tooHot }, "temperature"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conv := valid()
			tc.mutate(conv)
			err := conv.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}

	var none *AccountTestConversation
	require.NoError(t, none.Validate())
}

func TestApplyAnthropicTestConversation(t *testing.T) {
	c, _ := newAccountTestConversationContext(t, sampleAccountTestConversation())
	payload, err := createTestPayload("claude-sonnet-5-5")
	require.NoError(t, err)

	applyAnthropicTestConversation(c, payload)
	got := roundTrip(t, payload)

	messages := got["messages"].([]any)
	require.Len(t, messages, 3)
	require.Equal(t, "assistant", messages[1].(map[string]any)["role"])
	lastContent := messages[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	require.Equal(t, "text", lastContent["type"])
	require.Equal(t, "second question", lastContent["text"])

	system := got["system"].([]any)
	require.Len(t, system, 2, "the probe's own system block must stay in front")
	require.Equal(t, claudeCodeSystemPrompt, system[0].(map[string]any)["text"])
	require.Equal(t, "be brief", system[1].(map[string]any)["text"])

	require.EqualValues(t, 2048, got["max_tokens"])
	require.EqualValues(t, 0.2, got["temperature"])
	require.Equal(t, true, got["stream"])
	require.False(t, accountTestConversationSkipped(c))
}

func TestApplyAnthropicTestConversationKeepsProbeDefaults(t *testing.T) {
	c, _ := newAccountTestConversationContext(t, &AccountTestConversation{
		Messages: []AccountTestMessage{{Role: "user", Content: "hello"}},
	})
	payload := map[string]any{"max_tokens": 256, "temperature": 1}

	applyAnthropicTestConversation(c, payload)
	got := roundTrip(t, payload)

	require.EqualValues(t, 256, got["max_tokens"])
	require.EqualValues(t, 1, got["temperature"])
	require.NotContains(t, got, "system")
}

func TestApplyResponsesTestConversation(t *testing.T) {
	t.Run("api key account", func(t *testing.T) {
		c, _ := newAccountTestConversationContext(t, sampleAccountTestConversation())
		payload := createOpenAITestPayload("gpt-5", false)

		applyResponsesTestConversation(c, payload, "developer", true)
		got := roundTrip(t, payload)

		input := got["input"].([]any)
		require.Len(t, input, 4)
		require.Equal(t, "developer", input[0].(map[string]any)["role"])
		assistantContent := input[2].(map[string]any)["content"].([]any)[0].(map[string]any)
		require.Equal(t, "output_text", assistantContent["type"])
		userContent := input[3].(map[string]any)["content"].([]any)[0].(map[string]any)
		require.Equal(t, "input_text", userContent["type"])
		require.Equal(t, "second question", userContent["text"])
		require.EqualValues(t, 2048, got["max_output_tokens"])
		require.EqualValues(t, 0.2, got["temperature"])
		require.Contains(t, got, "instructions")
	})

	t.Run("oauth account drops sampling parameters", func(t *testing.T) {
		c, _ := newAccountTestConversationContext(t, sampleAccountTestConversation())
		payload := createOpenAITestPayload("gpt-5", true)

		applyResponsesTestConversation(c, payload, "developer", false)
		got := roundTrip(t, payload)

		require.Len(t, got["input"].([]any), 4)
		require.NotContains(t, got, "max_output_tokens")
		require.NotContains(t, got, "temperature")
		require.Equal(t, false, got["store"])
	})
}

func TestApplyChatCompletionsTestConversation(t *testing.T) {
	c, _ := newAccountTestConversationContext(t, sampleAccountTestConversation())
	payload := createOpenAIChatCompletionsTestPayload("deepseek-chat", "ignored")

	applyChatCompletionsTestConversation(c, payload)
	got := roundTrip(t, payload)

	messages := got["messages"].([]any)
	require.Len(t, messages, 4)
	require.Equal(t, map[string]any{"role": "system", "content": "be brief"}, messages[0])
	require.Equal(t, map[string]any{"role": "user", "content": "second question"}, messages[3])
	require.EqualValues(t, 2048, got["max_tokens"])
	require.EqualValues(t, 0.2, got["temperature"])
}

func TestApplyGeminiTestConversation(t *testing.T) {
	c, _ := newAccountTestConversationContext(t, sampleAccountTestConversation())

	rewritten := applyGeminiTestConversation(c, "gemini-2.5-pro", createGeminiTestPayload("gemini-2.5-pro", "ignored"))
	var got map[string]any
	require.NoError(t, json.Unmarshal(rewritten, &got))

	contents := got["contents"].([]any)
	require.Len(t, contents, 3)
	require.Equal(t, "model", contents[1].(map[string]any)["role"])
	require.Equal(t, "second question", contents[2].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"])
	systemParts := got["systemInstruction"].(map[string]any)["parts"].([]any)
	require.Equal(t, "be brief", systemParts[0].(map[string]any)["text"])
	generationConfig := got["generationConfig"].(map[string]any)
	require.EqualValues(t, 2048, generationConfig["maxOutputTokens"])
	require.EqualValues(t, 0.2, generationConfig["temperature"])
	require.False(t, accountTestConversationSkipped(c))
}

func TestApplyGeminiTestConversationLeavesImageProbeAlone(t *testing.T) {
	const imageModel = "gemini-2.5-flash-image"
	require.True(t, isImageGenerationModel(imageModel))
	c, _ := newAccountTestConversationContext(t, sampleAccountTestConversation())
	original := createGeminiTestPayload(imageModel, "a red square")

	require.Equal(t, original, applyGeminiTestConversation(c, imageModel, original))
	require.True(t, accountTestConversationSkipped(c))
}

func TestApplyTestConversationWithoutConversationIsNoop(t *testing.T) {
	c, _ := newAccountTestConversationContext(t, nil)

	anthropicPayload, err := createTestPayload("claude-sonnet-5-5")
	require.NoError(t, err)
	before := roundTrip(t, anthropicPayload)
	applyAnthropicTestConversation(c, anthropicPayload)
	after := roundTrip(t, anthropicPayload)
	// metadata.user_id is random per payload but untouched between the two reads.
	require.Equal(t, before, after)

	responsesPayload := createOpenAITestPayload("gpt-5", false)
	applyResponsesTestConversation(c, responsesPayload, "developer", true)
	require.Equal(t, roundTrip(t, createOpenAITestPayload("gpt-5", false)), roundTrip(t, responsesPayload))

	chatPayload := createOpenAIChatCompletionsTestPayload("deepseek-chat", "hi")
	applyChatCompletionsTestConversation(c, chatPayload)
	require.Equal(t, roundTrip(t, createOpenAIChatCompletionsTestPayload("deepseek-chat", "hi")), roundTrip(t, chatPayload))

	geminiPayload := createGeminiTestPayload("gemini-2.5-pro", "hi")
	require.Equal(t, geminiPayload, applyGeminiTestConversation(c, "gemini-2.5-pro", geminiPayload))
	require.False(t, accountTestConversationSkipped(c))
}

func TestSendEventReportsSkippedConversation(t *testing.T) {
	svc := &AccountTestService{}

	t.Run("conversation not consumed", func(t *testing.T) {
		c, recorder := newAccountTestConversationContext(t, sampleAccountTestConversation())
		svc.sendEvent(c, TestEvent{Type: "test_complete", Success: true})

		body := recorder.Body.String()
		notice := strings.Index(body, `"code":"conversation_not_applied"`)
		complete := strings.Index(body, `"type":"test_complete"`)
		require.GreaterOrEqual(t, notice, 0, "notice must be sent")
		require.Greater(t, complete, notice, "notice must precede completion")
	})

	t.Run("conversation consumed", func(t *testing.T) {
		c, recorder := newAccountTestConversationContext(t, sampleAccountTestConversation())
		applyChatCompletionsTestConversation(c, map[string]any{})
		svc.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
		require.NotContains(t, recorder.Body.String(), "conversation_not_applied")
	})

	t.Run("plain connectivity test", func(t *testing.T) {
		c, recorder := newAccountTestConversationContext(t, nil)
		svc.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
		require.NotContains(t, recorder.Body.String(), "conversation_not_applied")
	})
}
