package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractContentModerationInputTypeSafeSystemOne(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"string", `{"state":"plain text"}`, "plain text"},
		{"object", `{"state":{"title":"hello","nested":{"body":"world"}}}`, "title hello nested body world"},
		{"array", `{"state":["first",{"text":"second"},3]}`, "first text second"},
		{"questions", `{"state":"state text","questions":{"c":{"type":"choice","instructions":"pick one","criteria":{"label a":{"description":"desc a"},"label b":null}},"s":{"type":"score","criteria":["low",{"text":"high"}]},"n":{"type":"noul","instructions":["judge"],"criteria":{"true":"yes"},"ext":"extra"}},"top":{"k":"v"}}`, "c pick one label a description desc a label b s low text high n judge true yes ext extra top k v state text"},
		{"key only payload", `{"state":{"hidden state key":1},"questions":{"hidden question id":{"type":"noul"}}}`, "hidden question id hidden state key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := ExtractContentModerationInput(ContentModerationProtocolTypeSafeSystemOne, []byte(tc.body))
			require.Equal(t, tc.want, input.Text)
			require.Empty(t, input.Images)
		})
	}
}

func TestExtractContentModerationInputTypeSafeSystemOneKeepsReminderText(t *testing.T) {
	body := `{"state":"<system-reminder>hidden payload</system-reminder>","questions":{"q":{"type":"noul","instructions":"<system-reminder>hidden instructions</system-reminder>"}}}`
	input := ExtractContentModerationInput(ContentModerationProtocolTypeSafeSystemOne, []byte(body))
	require.Contains(t, input.Text, "hidden payload")
	require.Contains(t, input.Text, "hidden instructions")
}
