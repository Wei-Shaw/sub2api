package securityaudit

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestExtractResponseTextJSONProtocols(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "openai chat", body: `{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`, want: "hello"},
		{name: "openai responses", body: `{"output":[{"type":"message","content":[{"type":"output_text","text":"world"}]}]}`, want: "world"},
		{name: "anthropic", body: `{"content":[{"type":"text","text":"claude"}]}`, want: "claude"},
		{name: "gemini", body: `{"candidates":[{"content":{"parts":[{"text":"gemini"}]}}]}`, want: "gemini"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := ExtractResponseText([]byte(test.body), false)
			require.True(t, result.Recognized)
			require.Equal(t, test.want, result.Text)
			require.False(t, result.Truncated)
		})
	}
}

func TestExtractResponseTextSSEUsesDeltasWithoutDuplicatingFinalResponse(t *testing.T) {
	body := strings.Join([]string{
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"hel"}`,
		``,
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"lo"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"output":[{"content":[{"type":"output_text","text":"hello"}]}]}}`,
		``,
		`data: [DONE]`,
	}, "\n")
	result := ExtractResponseText([]byte(body), false)
	require.True(t, result.Recognized)
	require.Equal(t, "hello", result.Text)
}

func TestExtractResponseTextMarksCaptureAndStorageTruncation(t *testing.T) {
	result := ExtractResponseText([]byte(`{"choices":[{"message":{"content":"hello"}}]}`), true)
	require.True(t, result.Truncated)

	longText := strings.Repeat("你", PromptResponseTextLimit)
	payload := `{"choices":[{"message":{"content":"` + longText + `"}}]}`
	result = ExtractResponseText([]byte(payload), false)
	require.True(t, result.Truncated)
	require.True(t, len(result.Text) <= PromptResponseTextLimit)
	require.True(t, utf8.ValidString(result.Text))
}
