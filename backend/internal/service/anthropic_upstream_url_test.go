package service

import "testing"

func TestBuildAnthropicUpstreamURLVersionedBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		path    string
		want    string
	}{
		{
			name:    "origin base adds v1",
			baseURL: "https://relay.example.com/",
			path:    "/v1/messages",
			want:    "https://relay.example.com/v1/messages?beta=true",
		},
		{
			name:    "versioned base does not duplicate v1",
			baseURL: "https://relay.example.com/v1",
			path:    "/v1/messages",
			want:    "https://relay.example.com/v1/messages?beta=true",
		},
		{
			name:    "prefixed versioned base does not duplicate v1 for count tokens",
			baseURL: "https://relay.example.com/anthropic/v1/",
			path:    "/v1/messages/count_tokens",
			want:    "https://relay.example.com/anthropic/v1/messages/count_tokens?beta=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildAnthropicUpstreamURL(tt.baseURL, tt.path); got != tt.want {
				t.Fatalf("buildAnthropicUpstreamURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
