package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortedUniqueAccountIDs(t *testing.T) {
	tests := []struct {
		name  string
		input []int64
		want  []int64
REDACTED{
		{name: "unsorted duplicates", input: []int64{12, 3, 12, 8, 3REDACTED, want: []int64{3, 8, 12REDACTEDREDACTED,
		{name: "already sorted", input: []int64{3, 8, 12REDACTED, want: []int64{3, 8, 12REDACTEDREDACTED,
		{name: "single", input: []int64{3REDACTED, want: []int64{3REDACTEDREDACTED,
		{name: "empty", input: []int64{REDACTED, want: []int64{REDACTEDREDACTED,
		{name: "nil", input: nil, want: nilREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, sortedUniqueAccountIDs(tt.input))
	REDACTED)
REDACTED
REDACTED
