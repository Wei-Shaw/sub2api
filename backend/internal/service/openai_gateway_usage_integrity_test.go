//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequiresBillableGrokChatUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		account *Account
		models  []string
		want    bool
	}{
		{
			name:    "grok platform",
			account: &Account{Platform: PlatformGrok},
			models:  []string{"alias"},
			want:    true,
		},
		{
			name:    "OpenAI-compatible requested Grok model",
			account: &Account{Platform: PlatformOpenAI},
			models:  []string{"grok-4.5"},
			want:    true,
		},
		{
			name:    "OpenAI-compatible mapped Grok model",
			account: &Account{Platform: PlatformOpenAI},
			models:  []string{"alias", "grok-4.5"},
			want:    true,
		},
		{
			name:    "xAI-qualified Grok model",
			account: &Account{Platform: PlatformOpenAI},
			models:  []string{"xai/grok-4.5"},
			want:    true,
		},
		{
			name:    "ordinary OpenAI model",
			account: &Account{Platform: PlatformOpenAI},
			models:  []string{"gpt-5.4"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, requiresBillableGrokChatUsage(tt.account, tt.models...))
		})
	}
}
