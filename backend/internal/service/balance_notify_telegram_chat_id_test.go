package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeBalanceNotifyTelegramChatID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trims spaces", in: "  123456789  ", want: "123456789"},
		{name: "allows negative group id", in: " -1001234567890 ", want: "-1001234567890"},
		{name: "clears invalid letters", in: "abc123", want: ""},
		{name: "clears empty", in: "   ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeBalanceNotifyTelegramChatID(tt.in))
		})
	}
}
