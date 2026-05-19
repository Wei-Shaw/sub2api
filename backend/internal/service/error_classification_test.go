package service

import "testing"

func TestIsQuotaExhaustedMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{
			name: "worldrouter credit exceeded",
			msg:  "Credit has been exceeded! Current cost: 3161.571126500001, Max credit: 3100.0",
			want: true,
		},
		{
			name: "anthropic credit balance",
			msg:  "Your credit balance is too low to access the Anthropic API",
			want: true,
		},
		{
			name: "insufficient credits",
			msg:  "Insufficient credits to complete this request",
			want: true,
		},
		{
			name: "quota exhausted",
			msg:  "quota_exhausted: account has no remaining quota",
			want: true,
		},
		{
			name: "max credit",
			msg:  "Max credit limit reached for this account",
			want: true,
		},
		{
			name: "resource exhausted",
			msg:  "Resource has been exhausted (e.g. check quota)",
			want: true,
		},
		{
			name: "normal 400 error - not quota",
			msg:  "messages.17.content.101: Invalid `data` in `redacted_thinking` block",
			want: false,
		},
		{
			name: "thinking error - not quota",
			msg:  "Expected `thinking` or `redacted_thinking`, but found `text`",
			want: false,
		},
		{
			name: "empty message",
			msg:  "",
			want: false,
		},
		{
			name: "generic error",
			msg:  "invalid request body",
			want: false,
		},
		{
			name: "case insensitive - CREDIT BALANCE",
			msg:  "CREDIT BALANCE is too low",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsQuotaExhaustedMessage(tt.msg)
			if got != tt.want {
				t.Errorf("IsQuotaExhaustedMessage(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}
