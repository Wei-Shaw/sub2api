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
	}{
		{name: "unsorted duplicates", input: []int64{12, 3, 12, 8, 3}, want: []int64{3, 8, 12}},
		{name: "already sorted", input: []int64{3, 8, 12}, want: []int64{3, 8, 12}},
		{name: "single", input: []int64{3}, want: []int64{3}},
		{name: "empty", input: []int64{}, want: []int64{}},
		{name: "nil", input: nil, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, sortedUniqueAccountIDs(tt.input))
		})
	}
}
