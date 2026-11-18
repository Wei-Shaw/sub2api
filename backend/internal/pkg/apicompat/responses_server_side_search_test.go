package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsServerSideSearchCall(t *testing.T) {
	tests := []struct {
		name     string
		itemType string
		callID   string
		want     bool
	}{
		{"x search", "custom_tool_call", "xs_123", true},
		{"attachment search", "custom_tool_call", "as_456", true},
		{"padded call id is not a prefix match", "custom_tool_call", " xs_123", false},
		{"client custom tool", "custom_tool_call", "call_9", false},
		{"prefix is not a substring", "custom_tool_call", "call_xs_9", false},
		{"no call id", "custom_tool_call", "", false},
		{"function call keeps its client meaning", "function_call", "xs_123", false},
		{"other server-side item types are separate items", "x_search_call", "xs_123", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsServerSideSearchCall(tt.itemType, tt.callID))
		})
	}
}
