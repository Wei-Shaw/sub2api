package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type checkInRepoStub struct {
	status     *CheckInStatus
	result     *CheckInResult
	lastPicker CheckInRewardPicker
	enabled    bool
	resetCount int64
}

type checkInAuthCacheStub struct {
	userIDs []int64
}

func (s *checkInAuthCacheStub) InvalidateAuthCacheByKey(context.Context, string) {}

func (s *checkInAuthCacheStub) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}

func (s *checkInAuthCacheStub) InvalidateAuthCacheByGroupID(context.Context, int64) {}

func (s *checkInRepoStub) GetUserStatus(context.Context, int64, time.Time, int) (*CheckInStatus, error) {
	return s.status, nil
}

func (s *checkInRepoStub) CheckIn(_ context.Context, _ int64, _ time.Time, picker CheckInRewardPicker) (*CheckInResult, error) {
	s.lastPicker = picker
	return s.result, nil
}

func (s *checkInRepoStub) GetAdminStats(context.Context, time.Time, int) (*CheckInAdminStats, error) {
	return &CheckInAdminStats{}, nil
}

func (s *checkInRepoStub) SetEnabled(_ context.Context, enabled bool) (*CheckInConfig, error) {
	s.enabled = enabled
	return &CheckInConfig{Enabled: enabled}, nil
}

func (s *checkInRepoStub) ResetAllCycles(context.Context, time.Time) (int64, error) {
	return s.resetCount, nil
}

func TestCheckInConfigRewardRange(t *testing.T) {
	cfg := CheckInConfig{
		StandardMin:      3,
		StandardMax:      10,
		ReducedThreshold: 30,
		ReducedMin:       1,
		ReducedMax:       3,
	}

	mode, minReward, maxReward := cfg.RewardRange(29.99)
	require.Equal(t, CheckInModeStandard, mode)
	require.Equal(t, 3, minReward)
	require.Equal(t, 10, maxReward)

	mode, minReward, maxReward = cfg.RewardRange(30)
	require.Equal(t, CheckInModeReduced, mode)
	require.Equal(t, 1, minReward)
	require.Equal(t, 3, maxReward)
}

func TestCheckInServiceDecoratesUserStatus(t *testing.T) {
	repo := &checkInRepoStub{status: &CheckInStatus{
		Config: CheckInConfig{
			StandardMin:      3,
			StandardMax:      10,
			ReducedThreshold: 30,
			ReducedMin:       1,
			ReducedMax:       3,
		},
		CycleReward: 30,
	}}
	svc := NewCheckInService(repo, nil, nil)

	status, err := svc.GetUserStatus(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, CheckInModeReduced, status.NextRewardMode)
	require.Equal(t, 1, status.NextRewardMin)
	require.Equal(t, 3, status.NextRewardMax)
	require.NotNil(t, status.RecentCheckIns)
}

func TestCheckInServiceAlreadyCheckedDoesNotInvalidateCaches(t *testing.T) {
	repo := &checkInRepoStub{result: &CheckInResult{AlreadyChecked: true}}
	invalidator := &checkInAuthCacheStub{}
	svc := NewCheckInService(repo, nil, invalidator)

	result, err := svc.CheckIn(context.Background(), 9)
	require.NoError(t, err)
	require.True(t, result.AlreadyChecked)
	require.Empty(t, invalidator.userIDs)
}

func TestRandomCheckInRewardStaysInsideInclusiveRange(t *testing.T) {
	for range 100 {
		reward, err := randomCheckInReward(3, 10)
		require.NoError(t, err)
		require.GreaterOrEqual(t, reward, 3)
		require.LessOrEqual(t, reward, 10)
	}
}
