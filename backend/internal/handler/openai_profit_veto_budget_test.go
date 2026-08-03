package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecordOpenAIProfitVetoDoesNotCapEligibleCandidates(t *testing.T) {
	failed := make(map[int64]struct{})

	for i := int64(1); i <= 64; i++ {
		recordOpenAIProfitVeto(failed, i)
		require.Contains(t, failed, i, "否决的账号必须进入本请求排除集")
	}

	require.Len(t, failed, 64)
}

func TestProfitVetoRecordersShareExclusionSemantics(t *testing.T) {
	fs := NewFailoverState(10, false)
	failed := make(map[int64]struct{})

	for i := int64(1); i <= 32; i++ {
		fs.RecordProfitVeto(i)
		recordOpenAIProfitVeto(failed, i)
	}

	require.Equal(t, fs.FailedAccountIDs, failed)
}
