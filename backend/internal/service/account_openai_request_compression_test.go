package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountGetOpenAIRequestCompressionOverride(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		wantSet bool
		want    bool
	}{
		{name: "nil account"},
		{name: "nil extra", account: &Account{}},
		{name: "missing setting", account: &Account{Extra: map[string]any{}}},
		{name: "null setting", account: &Account{Extra: map[string]any{openAIRequestCompressionExtraKey: nil}}},
		{name: "explicit opt out", account: &Account{Extra: map[string]any{openAIRequestCompressionExtraKey: false}}, wantSet: true},
		{name: "explicit true", account: &Account{Extra: map[string]any{openAIRequestCompressionExtraKey: true}}, wantSet: true, want: true},
		{name: "string is unset", account: &Account{Extra: map[string]any{openAIRequestCompressionExtraKey: "false"}}},
		{name: "number is unset", account: &Account{Extra: map[string]any{openAIRequestCompressionExtraKey: 0}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.GetOpenAIRequestCompressionOverride()
			if !tt.wantSet {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, tt.want, *got)
		})
	}
}
