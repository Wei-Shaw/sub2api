//go:build unit

package repository

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApiKeyRateLimitKey(t *testing.T) {
	tests := []struct {
		name     string
		userID   int64
		expected string
	}{
		{
			name:     "normal_user_id",
			userID:   123,
			expected: "apikey:ratelimit:123",
		},
		{
			name:     "zero_user_id",
			userID:   0,
			expected: "apikey:ratelimit:0",
		},
		{
			name:     "negative_user_id",
			userID:   -1,
			expected: "apikey:ratelimit:-1",
		},
		{
			name:     "max_int64",
			userID:   math.MaxInt64,
			expected: "apikey:ratelimit:9223372036854775807",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := apiKeyRateLimitKey(tc.userID)
			require.Equal(t, tc.expected, got)
		})
	}
}
