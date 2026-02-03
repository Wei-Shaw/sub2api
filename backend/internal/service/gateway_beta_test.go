package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeAnthropicBeta(t *testing.T) {
	got := mergeAnthropicBeta(
		[]string{"oauth-2025-04-20", "REDACTED"REDACTED,
		"foo, oauth-2025-04-20,bar, foo",
	)
	require.Equal(t, "oauth-2025-04-20,REDACTED,foo,bar", got)
REDACTED

func TestMergeAnthropicBeta_EmptyIncoming(t *testing.T) {
	got := mergeAnthropicBeta(
		[]string{"oauth-2025-04-20", "REDACTED"REDACTED,
		"",
	)
	require.Equal(t, "oauth-2025-04-20,REDACTED", got)
REDACTED
