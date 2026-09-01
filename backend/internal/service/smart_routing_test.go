package service

import (
	mathrand "math/rand"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func srCand(id int64, priority, weight int) SmartRoutingCandidate {
	return SmartRoutingCandidate{
		Group:    &Group{ID: id, Status: StatusActive},
		Priority: priority,
		Weight:   weight,
	}
}

func TestSmartRoutingMemberIDs(t *testing.T) {
	ids := smartRoutingMemberIDs([]domain.SmartRoutingMember{
		{GroupID: 2, Priority: 1, Weight: 1},
		{GroupID: 0},
		{GroupID: 2}, // dup
		{GroupID: 5},
		{GroupID: -1},
	})
	assert.Equal(t, []int64{2, 5}, ids)
	assert.Nil(t, smartRoutingMemberIDs(nil))
}

func TestOrderSmartRoutingCandidates_TiersAscending(t *testing.T) {
	cands := []SmartRoutingCandidate{
		srCand(3, 3, 1),
		srCand(1, 1, 1),
		srCand(2, 2, 1),
	}
	out := orderSmartRoutingCandidates(cands, mathrand.New(mathrand.NewSource(1)))
	require.Len(t, out, 3)
	assert.Equal(t, int64(1), out[0].Group.ID)
	assert.Equal(t, int64(2), out[1].Group.ID)
	assert.Equal(t, int64(3), out[2].Group.ID)
}

func TestOrderSmartRoutingCandidates_PreservesAllWithinTier(t *testing.T) {
	cands := []SmartRoutingCandidate{
		srCand(1, 1, 1),
		srCand(2, 1, 2),
		srCand(3, 1, 3),
	}
	out := orderSmartRoutingCandidates(cands, mathrand.New(mathrand.NewSource(42)))
	require.Len(t, out, 3)
	seen := map[int64]bool{}
	for _, c := range out {
		seen[c.Group.ID] = true
	}
	assert.True(t, seen[1] && seen[2] && seen[3])
}

func TestOrderSmartRoutingCandidates_SingleOrEmpty(t *testing.T) {
	assert.Nil(t, orderSmartRoutingCandidates(nil, nil))
	one := []SmartRoutingCandidate{srCand(1, 1, 1)}
	assert.Equal(t, one, orderSmartRoutingCandidates(one, nil))
}

// TestWeightedShuffleSmartRouting_Distribution 用统计方式验证加权分流：
// 权重 9:1 的两个候选，首个候选约 90% 的概率被选中。
func TestWeightedShuffleSmartRouting_Distribution(t *testing.T) {
	const trials = 4000
	firstPicks := 0
	for i := 0; i < trials; i++ {
		rnd := mathrand.New(mathrand.NewSource(int64(i)))
		items := []SmartRoutingCandidate{
			srCand(1, 1, 9),
			srCand(2, 1, 1),
		}
		out := weightedShuffleSmartRouting(items, rnd)
		require.Len(t, out, 2)
		if out[0].Group.ID == 1 {
			firstPicks++
		}
	}
	ratio := float64(firstPicks) / trials
	// 期望约 0.9，放宽到 ±0.05 避免偶发抖动。
	assert.InDelta(t, 0.9, ratio, 0.05, "weighted first-pick ratio=%v", ratio)
}

func TestWeightedShuffleSmartRouting_ZeroWeightsStable(t *testing.T) {
	items := []SmartRoutingCandidate{
		srCand(1, 1, 0),
		srCand(2, 1, 0),
	}
	out := weightedShuffleSmartRouting(items, nil)
	require.Len(t, out, 2)
	assert.Equal(t, int64(1), out[0].Group.ID)
	assert.Equal(t, int64(2), out[1].Group.ID)
}

func TestSmartRoutingIntn(t *testing.T) {
	assert.Equal(t, 0, smartRoutingIntn(nil, 0))
	assert.Equal(t, 0, smartRoutingIntn(nil, -5))
	rnd := mathrand.New(mathrand.NewSource(7))
	for i := 0; i < 100; i++ {
		v := smartRoutingIntn(rnd, 10)
		assert.True(t, v >= 0 && v < 10)
	}
}
