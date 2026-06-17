package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapUserErrorCategoryCyber(t *testing.T) {
	require.Equal(t, "cyber", MapUserErrorCategory("request", "cyber_policy"))
	phases, types := CategoryToFilter("cyber")
	require.Equal(t, []string{"request"}, phases)
	require.Equal(t, []string{"cyber_policy"}, types)
}
