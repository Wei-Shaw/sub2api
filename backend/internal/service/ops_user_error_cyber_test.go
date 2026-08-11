package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapUserErrorCategoryCyber(t *testing.T) {
	require.Equal(t, "cyber", MapUserErrorCategory("request", "cyber_policy"))
	require.Equal(t, "cyber", MapUserErrorCategory("request", "content_policy_violation"))
	require.Equal(t, "cyber", MapUserErrorCategory("request", "moderation_blocked"))
	phases, types := CategoryToFilter("cyber")
	require.Equal(t, []string{"request"}, phases)
	require.Equal(t, []string{"cyber_policy", "content_policy_violation", "moderation_blocked"}, types)
}
