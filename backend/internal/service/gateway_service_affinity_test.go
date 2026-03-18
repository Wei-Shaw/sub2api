//go:build unit

// Package service 提供 API 网关核心服务。
// 本文件包含 SortAffinityClients 函数的单元测试，
// 验证 AffinityClient 切片排序逻辑在各种输入条件下的正确行为。
//
// This file contains unit tests for the SortAffinityClients function,
// verifying correct sorting behavior for AffinityClient slices under
// various input conditions including empty, single, sorted, reverse,
// and duplicate-timestamp scenarios.
package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSortAffinityClients_Empty(t *testing.T) {
	var clients []AffinityClient
	SortAffinityClients(clients)
	require.Empty(t, clients)

	clients = []AffinityClient{}
	SortAffinityClients(clients)
	require.Empty(t, clients)
}

func TestSortAffinityClients_SingleElement(t *testing.T) {
	now := time.Now()
	clients := []AffinityClient{
		{ClientID: "client-1", LastActive: now},
	}
	SortAffinityClients(clients)
	require.Len(t, clients, 1)
	require.Equal(t, "client-1", clients[0].ClientID)
	require.Equal(t, now, clients[0].LastActive)
}

func TestSortAffinityClients_AlreadySorted(t *testing.T) {
	now := time.Now()
	clients := []AffinityClient{
		{ClientID: "newest", LastActive: now},
		{ClientID: "middle", LastActive: now.Add(-1 * time.Hour)},
		{ClientID: "oldest", LastActive: now.Add(-2 * time.Hour)},
	}
	SortAffinityClients(clients)

	require.Equal(t, "newest", clients[0].ClientID)
	require.Equal(t, "middle", clients[1].ClientID)
	require.Equal(t, "oldest", clients[2].ClientID)
}

func TestSortAffinityClients_ReverseOrder(t *testing.T) {
	now := time.Now()
	clients := []AffinityClient{
		{ClientID: "oldest", LastActive: now.Add(-2 * time.Hour)},
		{ClientID: "middle", LastActive: now.Add(-1 * time.Hour)},
		{ClientID: "newest", LastActive: now},
	}
	SortAffinityClients(clients)

	require.Equal(t, "newest", clients[0].ClientID)
	require.Equal(t, "middle", clients[1].ClientID)
	require.Equal(t, "oldest", clients[2].ClientID)
}

func TestSortAffinityClients_SameTimestamps(t *testing.T) {
	now := time.Now()
	clients := []AffinityClient{
		{ClientID: "c1", LastActive: now},
		{ClientID: "c2", LastActive: now},
		{ClientID: "c3", LastActive: now},
	}
	SortAffinityClients(clients)

	// 所有时间戳相同时，排序结果应保持稳定（sort.Slice 不保证稳定性，
	// 但只要结果是某种确定的顺序即可）。
	// 验证所有元素仍然存在且时间相同。
	require.Len(t, clients, 3)
	ids := map[string]bool{}
	for _, c := range clients {
		ids[c.ClientID] = true
		require.Equal(t, now, c.LastActive)
	}
	require.True(t, ids["c1"])
	require.True(t, ids["c2"])
	require.True(t, ids["c3"])
}

func TestSortAffinityClients_MixedOrder(t *testing.T) {
	now := time.Now()
	clients := []AffinityClient{
		{ClientID: "c3", LastActive: now.Add(-30 * time.Minute)},
		{ClientID: "c1", LastActive: now},
		{ClientID: "c5", LastActive: now.Add(-2 * time.Hour)},
		{ClientID: "c2", LastActive: now.Add(-10 * time.Minute)},
		{ClientID: "c4", LastActive: now.Add(-1 * time.Hour)},
	}
	SortAffinityClients(clients)

	// 按 LastActive 降序排列
	require.Equal(t, "c1", clients[0].ClientID) // now
	require.Equal(t, "c2", clients[1].ClientID) // -10m
	require.Equal(t, "c3", clients[2].ClientID) // -30m
	require.Equal(t, "c4", clients[3].ClientID) // -1h
	require.Equal(t, "c5", clients[4].ClientID) // -2h
}

func TestSortAffinityClients_SubSecondDifferences(t *testing.T) {
	base := time.Now()
	clients := []AffinityClient{
		{ClientID: "early", LastActive: base},
		{ClientID: "late", LastActive: base.Add(500 * time.Millisecond)},
	}
	SortAffinityClients(clients)

	// 500ms 差异也应正确排序（更晚的在前）
	require.Equal(t, "late", clients[0].ClientID)
	require.Equal(t, "early", clients[1].ClientID)
}
