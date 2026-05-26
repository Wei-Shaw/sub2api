package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLatencyHistogramBuckets_AreConsistent(t *testing.T) {
	require.Equal(t, len(latencyHistogramBuckets), len(latencyHistogramOrderedRanges))
	for i, b := range latencyHistogramBuckets {
		require.Equal(t, b.label, latencyHistogramOrderedRanges[i])
	}
}
