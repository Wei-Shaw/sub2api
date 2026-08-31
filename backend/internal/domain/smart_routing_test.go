package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSmartRoutingMembers_Empty(t *testing.T) {
	assert.Nil(t, NormalizeSmartRoutingMembers(nil))
	assert.Nil(t, NormalizeSmartRoutingMembers([]SmartRoutingMember{}))
}

func TestNormalizeSmartRoutingMembers_DedupesAndDropsInvalid(t *testing.T) {
	in := []SmartRoutingMember{
		{GroupID: 2, Priority: 2, Weight: 3},
		{GroupID: 1, Priority: 1, Weight: 1},
		{GroupID: 2, Priority: 9, Weight: 9}, // duplicate of 2: keep first
		{GroupID: 0, Priority: 1, Weight: 1}, // invalid id
		{GroupID: -5, Priority: 1, Weight: 1},
	}
	out := NormalizeSmartRoutingMembers(in)
	require.Len(t, out, 2)
	// Sorted by priority ascending, then group id.
	assert.Equal(t, SmartRoutingMember{GroupID: 1, Priority: 1, Weight: 1}, out[0])
	assert.Equal(t, SmartRoutingMember{GroupID: 2, Priority: 2, Weight: 3}, out[1])
}

func TestNormalizeSmartRoutingMembers_ClampsPriorityAndWeight(t *testing.T) {
	out := NormalizeSmartRoutingMembers([]SmartRoutingMember{
		{GroupID: 1, Priority: 0, Weight: -3},
		{GroupID: 2, Priority: -1, Weight: 0},
	})
	require.Len(t, out, 2)
	for _, m := range out {
		assert.GreaterOrEqual(t, m.Priority, 1)
		assert.GreaterOrEqual(t, m.Weight, 1)
	}
}

func TestNormalizeSmartRoutingMembers_SortsByPriorityThenID(t *testing.T) {
	out := NormalizeSmartRoutingMembers([]SmartRoutingMember{
		{GroupID: 30, Priority: 2, Weight: 1},
		{GroupID: 10, Priority: 2, Weight: 1},
		{GroupID: 20, Priority: 1, Weight: 1},
	})
	require.Len(t, out, 3)
	assert.Equal(t, int64(20), out[0].GroupID) // priority 1 first
	assert.Equal(t, int64(10), out[1].GroupID) // priority 2, lower id
	assert.Equal(t, int64(30), out[2].GroupID)
}

func TestNormalizeSmartRoutingMembers_CapsAtMaxMembers(t *testing.T) {
	in := make([]SmartRoutingMember, 0, SmartRoutingMaxMembers+5)
	for i := 1; i <= SmartRoutingMaxMembers+5; i++ {
		in = append(in, SmartRoutingMember{GroupID: int64(i), Priority: 1, Weight: 1})
	}
	out := NormalizeSmartRoutingMembers(in)
	assert.Len(t, out, SmartRoutingMaxMembers)
}
