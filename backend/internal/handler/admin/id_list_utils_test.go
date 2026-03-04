//go:build unit

package admin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeInt64IDList(t *testing.T) {
	tests := []struct {
		name string
		in   []int64
		want []int64
REDACTED{
		{"nil input", nil, nilREDACTED,
		{"empty input", []int64{REDACTED, nilREDACTED,
		{"single element", []int64{5REDACTED, []int64{5REDACTEDREDACTED,
		{"already sorted unique", []int64{1, 2, 3REDACTED, []int64{1, 2, 3REDACTEDREDACTED,
		{"duplicates removed", []int64{3, 1, 3, 2, 1REDACTED, []int64{1, 2, 3REDACTEDREDACTED,
		{"zero filtered", []int64{0, 1, 2REDACTED, []int64{1, 2REDACTEDREDACTED,
		{"negative filtered", []int64{-5, -1, 3REDACTED, []int64{3REDACTEDREDACTED,
		{"all invalid", []int64{0, -1, -2REDACTED, []int64{REDACTEDREDACTED,
		{"sorted output", []int64{9, 3, 7, 1REDACTED, []int64{1, 3, 7, 9REDACTEDREDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeInt64IDList(tc.in)
			if tc.want == nil {
				require.Nil(t, got)
		REDACTED else {
				require.Equal(t, tc.want, got)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestBuildAccountTodayStatsBatchCacheKey(t *testing.T) {
	tests := []struct {
		name string
		ids  []int64
		want string
REDACTED{
		{"empty", nil, "accounts_today_stats_empty"REDACTED,
		{"single", []int64{42REDACTED, "accounts_today_stats:42"REDACTED,
		{"multiple", []int64{1, 2, 3REDACTED, "accounts_today_stats:1,2,3"REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildAccountTodayStatsBatchCacheKey(tc.ids)
			require.Equal(t, tc.want, got)
	REDACTED)
REDACTED
REDACTED
