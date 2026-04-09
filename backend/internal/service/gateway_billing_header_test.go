package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncBillingHeaderVersion(t *testing.T) {
	tests := []struct {
		name      string
		body      []byte
		userAgent string
		wantSub   string
		unchanged bool
	}{
		{
			name:      "replaces cc_version preserving message-derived suffix",
			body:      []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81.df2; cc_entrypoint=cli; cch=00000;"},{"type":"text","text":"You are Claude Code.","cache_control":{"type":"ephemeral"}}],"messages":[]}`),
			userAgent: "claude-cli/2.1.22",
			wantSub:   "cc_version=2.1.22.df2",
		},
		{
			name:      "no billing header in system",
			body:      []byte(`{"system":[{"type":"text","text":"ordinary system"}],"messages":[]}`),
			userAgent: "claude-cli/2.1.22",
			unchanged: true,
		},
		{
			name:      "no version in user-agent",
			body:      []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`),
			userAgent: "unknown-client",
			unchanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := syncBillingHeaderVersion(tt.body, tt.userAgent)
			if tt.unchanged {
				assert.JSONEq(t, string(tt.body), string(result))
				return
			}
			assert.Contains(t, string(result), tt.wantSub)
		})
	}
}

func TestSignBillingHeaderCCH(t *testing.T) {
	t.Run("replaces placeholder", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`)
		result := signBillingHeaderCCH(body)
		assert.NotEqual(t, string(body), string(result))
		assert.NotContains(t, string(result), "cch=00000;")
	})

	t.Run("no billing header unchanged", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"ordinary system"}],"messages":[]}`)
		result := signBillingHeaderCCH(body)
		assert.JSONEq(t, string(body), string(result))
	})

	t.Run("deterministic for same body", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":"hi"}]}`)
		body2 := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":"hi"}]}`)
		result1 := signBillingHeaderCCH(body)
		result2 := signBillingHeaderCCH(body2)
		require.Equal(t, string(result1), string(result2))
	})
}
