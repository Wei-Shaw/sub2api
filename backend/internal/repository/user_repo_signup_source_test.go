package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserSignupSourceOrDefaultPreservesFeishu(t *testing.T) {
	require.Equal(t, "feishu", userSignupSourceOrDefault(" Feishu "))
}
