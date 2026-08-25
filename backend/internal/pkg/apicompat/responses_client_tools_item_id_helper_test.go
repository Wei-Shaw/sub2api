package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetypedResponsesToolCallItemID(t *testing.T) {
	for _, tc := range []struct {
		name     string
		id       string
		itemType string
		want     string
	}{
		{"function id raised to custom", "fc_abc", "custom_tool_call", "ctc_abc"},
		{"function id raised to tool search", "fc_abc", "tool_search_call", "tsc_abc"},
		{"already correct is untouched", "ctc_abc", "custom_tool_call", "ctc_abc"},
		{"custom id lowered to function", "ctc_abc", "function_call", "fc_abc"},
		{"unknown prefix is left alone", "item_abc", "custom_tool_call", "item_abc"},
		{"unprefixed id is left alone", "abc", "custom_tool_call", "abc"},
		{"empty id stays empty", "", "custom_tool_call", ""},
		{"unconstrained item type is left alone", "fc_abc", "message", "fc_abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, retypedResponsesToolCallItemID(tc.id, tc.itemType))
		})
	}
}
