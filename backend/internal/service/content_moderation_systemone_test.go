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
		{"object", `{"state":{"title":"hello","nested":{"body":"world"}}}`, "hello world"},
		{"array", `{"state":["first",{"text":"second"},3]}`, "first second"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := ExtractContentModerationInput(ContentModerationProtocolTypeSafeSystemOne, []byte(tc.body))
			require.Equal(t, tc.want, input.Text)
			require.Empty(t, input.Images)
		})
	}
}
