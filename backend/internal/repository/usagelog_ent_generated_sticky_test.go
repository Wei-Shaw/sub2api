package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/usagelog"
	"github.com/stretchr/testify/require"
)

func TestUsageLogEntGeneratedStickySessionFieldsCompile(t *testing.T) {
	entity := ent.UsageLog{}
	entity.StickySessionSource = strPtr("header_x_session_affinity")
	entity.StickySessionHashPresent = boolPtr(true)
	entity.StickyEvalResult = strPtr("hit")
	entity.StickySelectedAccountChanged = boolPtr(false)
	entity.StickyParentSessionPresent = boolPtr(true)
	entity.StickyParentSessionKey = strPtr("resp_parent_123")

	require.Equal(t, "sticky_session_source", usagelog.FieldStickySessionSource)
	require.Equal(t, "sticky_session_hash_present", usagelog.FieldStickySessionHashPresent)
	require.Equal(t, "sticky_eval_result", usagelog.FieldStickyEvalResult)
	require.Equal(t, "sticky_selected_account_changed", usagelog.FieldStickySelectedAccountChanged)
	require.Equal(t, "sticky_parent_session_present", usagelog.FieldStickyParentSessionPresent)
	require.Equal(t, "sticky_parent_session_key", usagelog.FieldStickyParentSessionKey)
	require.Contains(t, usagelog.Columns, usagelog.FieldStickySessionSource)
	require.Contains(t, usagelog.Columns, usagelog.FieldStickyParentSessionKey)
	require.NotNil(t, entity.StickySessionSource)
	require.NotNil(t, entity.StickySessionHashPresent)
	require.NotNil(t, entity.StickyEvalResult)
	require.NotNil(t, entity.StickySelectedAccountChanged)
	require.NotNil(t, entity.StickyParentSessionPresent)
	require.NotNil(t, entity.StickyParentSessionKey)
}

func boolPtr(v bool) *bool {
	vv := v
	return &vv
}

func strPtr(v string) *string {
	vv := v
	return &vv
}
