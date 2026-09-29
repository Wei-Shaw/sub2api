//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// e03UserGroupRateRow 模拟 user_group_rate_multipliers 的一行（rate 与 rpm_override 共用一行）。
type e03UserGroupRateRow struct {
	rate *float64
	rpm  *int
}

type e03UserGroupRateKey struct {
	userID  int64
	groupID int64
}

// e03UserGroupRateRepoFake 以内存行模拟仓储语义，用于校验清空倍率时 rpm_override 是否被保留。
type e03UserGroupRateRepoFake struct {
	rows map[e03UserGroupRateKey]*e03UserGroupRateRow
}

func (f *e03UserGroupRateRepoFake) GetByUserID(_ context.Context, _ int64) (map[int64]float64, error) {
	panic("unexpected GetByUserID call")
}

func (f *e03UserGroupRateRepoFake) GetByUserAndGroup(_ context.Context, _, _ int64) (*float64, error) {
	panic("unexpected GetByUserAndGroup call")
}

func (f *e03UserGroupRateRepoFake) GetRPMOverrideByUserAndGroup(_ context.Context, _, _ int64) (*int, error) {
	panic("unexpected GetRPMOverrideByUserAndGroup call")
}

func (f *e03UserGroupRateRepoFake) GetByGroupID(_ context.Context, _ int64) ([]UserGroupRateEntry, error) {
	panic("unexpected GetByGroupID call")
}

func (f *e03UserGroupRateRepoFake) SyncUserGroupRates(_ context.Context, _ int64, _ map[int64]*float64) error {
	panic("unexpected SyncUserGroupRates call")
}

// SyncGroupRateMultipliers 与仓储实现一致：未在 entries 中的行 rate 归 NULL，整行 NULL 则删除。
func (f *e03UserGroupRateRepoFake) SyncGroupRateMultipliers(_ context.Context, groupID int64, entries []GroupRateMultiplierInput) error {
	keep := make(map[int64]float64, len(entries))
	for _, e := range entries {
		keep[e.UserID] = e.RateMultiplier
	}
	for k, row := range f.rows {
		if k.groupID != groupID {
			continue
		}
		if _, ok := keep[k.userID]; !ok {
			row.rate = nil
		}
		if row.rate == nil && row.rpm == nil {
			delete(f.rows, k)
		}
	}
	for userID, rate := range keep {
		r := rate
		k := e03UserGroupRateKey{userID: userID, groupID: groupID}
		if row, ok := f.rows[k]; ok {
			row.rate = &r
		} else {
			f.rows[k] = &e03UserGroupRateRow{rate: &r}
		}
	}
	return nil
}

func (f *e03UserGroupRateRepoFake) SyncGroupRPMOverrides(_ context.Context, _ int64, _ []GroupRPMOverrideInput) error {
	panic("unexpected SyncGroupRPMOverrides call")
}

func (f *e03UserGroupRateRepoFake) ClearGroupRPMOverrides(_ context.Context, _ int64) error {
	panic("unexpected ClearGroupRPMOverrides call")
}

// DeleteByGroupID 与仓储实现一致：整组行全部删除（含 rpm_override）。
func (f *e03UserGroupRateRepoFake) DeleteByGroupID(_ context.Context, groupID int64) error {
	for k := range f.rows {
		if k.groupID == groupID {
			delete(f.rows, k)
		}
	}
	return nil
}

func (f *e03UserGroupRateRepoFake) DeleteByUserID(_ context.Context, _ int64) error {
	panic("unexpected DeleteByUserID call")
}

func TestAdminService_ClearGroupRateMultipliers_PreservesRPMOverride_E03(t *testing.T) {
	const groupID int64 = 7
	const otherGroupID int64 = 8
	rate08, rate12, rate20 := 0.8, 1.2, 2.0
	rpm600, rpm100, rpm50 := 600, 100, 50

	repo := &e03UserGroupRateRepoFake{rows: map[e03UserGroupRateKey]*e03UserGroupRateRow{
		{userID: 1, groupID: groupID}:      {rate: &rate08, rpm: &rpm600}, // 倍率 + RPM
		{userID: 2, groupID: groupID}:      {rate: &rate12},               // 仅倍率
		{userID: 3, groupID: groupID}:      {rpm: &rpm100},                // 仅 RPM
		{userID: 1, groupID: otherGroupID}: {rate: &rate20, rpm: &rpm50},  // 其他分组不受影响
	}}
	svc := &adminServiceImpl{userGroupRateRepo: repo}

	require.NoError(t, svc.ClearGroupRateMultipliers(context.Background(), groupID))

	row1, ok := repo.rows[e03UserGroupRateKey{userID: 1, groupID: groupID}]
	require.True(t, ok, "row with rpm_override must survive clearing rate multipliers")
	require.Nil(t, row1.rate)
	require.NotNil(t, row1.rpm)
	require.Equal(t, 600, *row1.rpm)

	_, ok = repo.rows[e03UserGroupRateKey{userID: 2, groupID: groupID}]
	require.False(t, ok, "row with only rate_multiplier should be removed")

	row3, ok := repo.rows[e03UserGroupRateKey{userID: 3, groupID: groupID}]
	require.True(t, ok, "rpm-only row must be kept")
	require.Nil(t, row3.rate)
	require.NotNil(t, row3.rpm)
	require.Equal(t, 100, *row3.rpm)

	other, ok := repo.rows[e03UserGroupRateKey{userID: 1, groupID: otherGroupID}]
	require.True(t, ok)
	require.NotNil(t, other.rate)
	require.Equal(t, 2.0, *other.rate)
	require.NotNil(t, other.rpm)
	require.Equal(t, 50, *other.rpm)
}
