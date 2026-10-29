//go:build unit

package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageDeltaUsageExplicitZero(t *testing.T) {
	svc := &GatewayService{}
	parsers := map[string]func(string, *ClaudeUsage){
		"standard":    svc.parseSSEUsage,
		"passthrough": parseSSEUsagePassthrough,
	}
	cases := []struct {
		name string
		data string
		want ClaudeUsage
	}{
		{
			name: "final cached input replaces the initial estimate",
			data: `{"input_tokens":0,"output_tokens":33,"cache_creation_input_tokens":0,"cache_read_input_tokens":4004}`,
			want: ClaudeUsage{OutputTokens: 33, CacheReadInputTokens: 4004},
		},
		{
			name: "omitted input fields preserve the initial values",
			data: `{"output_tokens":33}`,
			want: ClaudeUsage{InputTokens: 8535, OutputTokens: 33, CacheCreationInputTokens: 20, CacheReadInputTokens: 30},
		},
		{
			name: "explicit zero clears cached input",
			data: `{"cache_read_input_tokens":0}`,
			want: ClaudeUsage{InputTokens: 8535, CacheCreationInputTokens: 20},
		},
		{
			name: "explicit zero clears cache creation",
			data: `{"cache_creation_input_tokens":0}`,
			want: ClaudeUsage{InputTokens: 8535, CacheReadInputTokens: 30},
		},
		{
			name: "string zero is accepted consistently",
			data: `{"input_tokens":"0","cache_creation_input_tokens":"0","cache_read_input_tokens":"0"}`,
			want: ClaudeUsage{},
		},
		{
			name: "negative counters do not overwrite usage",
			data: `{"input_tokens":-1,"cache_creation_input_tokens":-1,"cache_read_input_tokens":-1}`,
			want: ClaudeUsage{InputTokens: 8535, CacheCreationInputTokens: 20, CacheReadInputTokens: 30},
		},
		{
			name: "negative fractional counters do not become zero",
			data: `{"input_tokens":-0.5,"cache_creation_input_tokens":-0.5,"cache_read_input_tokens":-0.5}`,
			want: ClaudeUsage{InputTokens: 8535, CacheCreationInputTokens: 20, CacheReadInputTokens: 30},
		},
		{
			name: "invalid counters do not become zero",
			data: `{"input_tokens":null,"cache_creation_input_tokens":true,"cache_read_input_tokens":"invalid"}`,
			want: ClaudeUsage{InputTokens: 8535, CacheCreationInputTokens: 20, CacheReadInputTokens: 30},
		},
	}
	for name, parse := range parsers {
		t.Run(name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					usage := &ClaudeUsage{}
					parse(`{"type":"message_start","message":{"usage":{"input_tokens":8535,"cache_creation_input_tokens":20,"cache_read_input_tokens":30}}}`, usage)
					parse(fmt.Sprintf(`{"type":"message_delta","usage":%s}`, tc.data), usage)
					require.Equal(t, tc.want, *usage)
				})
			}
		})
	}
}
