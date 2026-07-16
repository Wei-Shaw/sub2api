package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type subscriptionExpiryRepoStub struct {
	listCalls int
}

func (r *subscriptionExpiryRepoStub) Create(context.Context, *UserSubscription) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) GetByID(context.Context, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}

func (r *subscriptionExpiryRepoStub) GetByIDIncludeDeleted(context.Context, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}

func (r *subscriptionExpiryRepoStub) GetByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}

func (r *subscriptionExpiryRepoStub) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}

func (r *subscriptionExpiryRepoStub) Update(context.Context, *UserSubscription) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) Delete(context.Context, int64) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) Restore(context.Context, int64, string) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}

func (r *subscriptionExpiryRepoStub) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, nil
}

func (r *subscriptionExpiryRepoStub) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, nil
}

func (r *subscriptionExpiryRepoStub) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *subscriptionExpiryRepoStub) List(context.Context, pagination.PaginationParams, *int64, *int64, string, string, string, string) ([]UserSubscription, *pagination.PaginationResult, error) {
	r.listCalls++
	return nil, &pagination.PaginationResult{Page: 1, Pages: 1}, nil
}

func (r *subscriptionExpiryRepoStub) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return false, nil
}

func (r *subscriptionExpiryRepoStub) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return false, nil
}

func (r *subscriptionExpiryRepoStub) ExtendExpiry(context.Context, int64, time.Time) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) UpdateStatus(context.Context, int64, string) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) UpdateNotes(context.Context, int64, string) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) ActivateWindows(context.Context, int64, time.Time) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) ResetUsageWindows(context.Context, int64, bool, bool, bool, time.Time) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) IncrementUsage(context.Context, int64, float64) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	return 0, nil
}

type subscriptionExpirySettingRepoStub struct {
	values map[string]string
	err    error
}

func (r *subscriptionExpirySettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *subscriptionExpirySettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *subscriptionExpirySettingRepoStub) Set(context.Context, string, string) error {
	return nil
}

func (r *subscriptionExpirySettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}

func (r *subscriptionExpirySettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
}

func (r *subscriptionExpirySettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return nil, nil
}

func (r *subscriptionExpirySettingRepoStub) Delete(context.Context, string) error {
	return nil
}

func TestSubscriptionExpiryService_ExpiryReminderEnabledDefaultsToTrue(t *testing.T) {
	svc := NewSubscriptionExpiryService(nil, time.Minute)
	svc.SetSettingRepository(&subscriptionExpirySettingRepoStub{values: map[string]string{}})

	require.True(t, svc.expiryReminderEnabled(context.Background()))
}

func TestSubscriptionExpiryService_ExpiryReminderDisabledSkipsSubscriptionScan(t *testing.T) {
	repo := &subscriptionExpiryRepoStub{}
	settingRepo := &subscriptionExpirySettingRepoStub{
		values: map[string]string{SettingKeySubscriptionExpiryNotifyEnabled: "false"},
	}
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, nil))

	svc.sendExpiryReminders(context.Background())

	require.Zero(t, repo.listCalls)
}

func TestSubscriptionExpiryService_ExpiryReminderSettingReadErrorFailsClosed(t *testing.T) {
	svc := NewSubscriptionExpiryService(nil, time.Minute)
	svc.SetSettingRepository(&subscriptionExpirySettingRepoStub{err: errors.New("db down")})

	require.False(t, svc.expiryReminderEnabled(context.Background()))
}
