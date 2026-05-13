//go:build unit

package service

import (
	"testing"

	openaipkg "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

func TestDefaultOpenAITestModelForAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		account *Account
		want    string
	}{
		{
			name: "nil account falls back to package default",
			want: openaipkg.DefaultTestModel,
		},
		{
			name: "openai oauth uses safe default",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
			},
			want: openAIOAuthDefaultTestModel,
		},
		{
			name: "openai apikey keeps package default",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
			},
			want: openaipkg.DefaultTestModel,
		},
		{
			name: "non openai oauth keeps package default",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeOAuth,
			},
			want: openaipkg.DefaultTestModel,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := defaultOpenAITestModelForAccount(tt.account); got != tt.want {
				t.Fatalf("defaultOpenAITestModelForAccount() = %q, want %q", got, tt.want)
			}
		})
	}
}
