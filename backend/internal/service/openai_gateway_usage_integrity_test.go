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
REDACTED{
		{
			name:    "grok platform",
			account: &Account{Platform: PlatformGrokREDACTED,
			models:  []string{"alias"REDACTED,
			want:    true,
	REDACTED,
		{
			name:    "OpenAI-compatible requested Grok model",
			account: &Account{Platform: PlatformOpenAIREDACTED,
			models:  []string{"grok-4.5"REDACTED,
			want:    true,
	REDACTED,
		{
			name:    "OpenAI-compatible mapped Grok model",
			account: &Account{Platform: PlatformOpenAIREDACTED,
			models:  []string{"alias", "grok-4.5"REDACTED,
			want:    true,
	REDACTED,
		{
			name:    "xAI-qualified Grok model",
			account: &Account{Platform: PlatformOpenAIREDACTED,
			models:  []string{"xai/grok-4.5"REDACTED,
			want:    true,
	REDACTED,
		{
			name:    "ordinary OpenAI model",
			account: &Account{Platform: PlatformOpenAIREDACTED,
			models:  []string{"gpt-5.4"REDACTED,
			want:    false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, requiresBillableGrokChatUsage(tt.account, tt.models...))
	REDACTED)
REDACTED
REDACTED
