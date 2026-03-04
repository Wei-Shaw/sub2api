//go:build unit

package service

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShouldFallbackOpsPreagg(t *testing.T) {
	preaggErr := ErrOpsPreaggregatedNotPopulated
	otherErr := errors.New("some other error")

	autoFilter := &OpsDashboardFilter{QueryMode: OpsQueryModeAutoREDACTED
	rawFilter := &OpsDashboardFilter{QueryMode: OpsQueryModeRawREDACTED
	preaggFilter := &OpsDashboardFilter{QueryMode: OpsQueryModePreaggREDACTED

	tests := []struct {
		name   string
		filter *OpsDashboardFilter
		err    error
		want   bool
REDACTED{
		{"auto mode + preagg error => fallback", autoFilter, preaggErr, trueREDACTED,
		{"auto mode + other error => no fallback", autoFilter, otherErr, falseREDACTED,
		{"auto mode + nil error => no fallback", autoFilter, nil, falseREDACTED,
		{"raw mode + preagg error => no fallback", rawFilter, preaggErr, falseREDACTED,
		{"preagg mode + preagg error => no fallback", preaggFilter, preaggErr, falseREDACTED,
		{"nil filter => no fallback", nil, preaggErr, falseREDACTED,
		{"wrapped preagg error => fallback", autoFilter, errors.Join(preaggErr, otherErr), trueREDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldFallbackOpsPreagg(tc.filter, tc.err)
			require.Equal(t, tc.want, got)
	REDACTED)
REDACTED
REDACTED

func TestCloneOpsFilterWithMode(t *testing.T) {
	t.Run("nil filter returns nil", func(t *testing.T) {
		require.Nil(t, cloneOpsFilterWithMode(nil, OpsQueryModeRaw))
REDACTED)

	t.Run("cloned filter has new mode", func(t *testing.T) {
		groupID := int64(42)
		original := &OpsDashboardFilter{
			StartTime: time.Now(),
			EndTime:   time.Now().Add(time.Hour),
			Platform:  "anthropic",
			GroupID:   &groupID,
			QueryMode: OpsQueryModeAuto,
	REDACTED

		cloned := cloneOpsFilterWithMode(original, OpsQueryModeRaw)
		require.Equal(t, OpsQueryModeRaw, cloned.QueryMode)
		require.Equal(t, OpsQueryModeAuto, original.QueryMode, "original should not be modified")
		require.Equal(t, original.Platform, cloned.Platform)
		require.Equal(t, original.StartTime, cloned.StartTime)
		require.Equal(t, original.GroupID, cloned.GroupID)
REDACTED)
REDACTED
