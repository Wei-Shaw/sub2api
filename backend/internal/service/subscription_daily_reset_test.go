package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

type resetDailyUserSubRepo struct {
	userSubRepoNoop
	client *dbent.Client
}

type resetDailySettingRepo struct {
	values map[string]string
}

func (r resetDailySettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	value, err := r.GetValue(context.Background(), key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r resetDailySettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (r *resetDailySettingRepo) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r resetDailySettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *resetDailySettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	for key, value := range settings {
		if err := r.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (r resetDailySettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *resetDailySettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func (r resetDailyUserSubRepo) GetByID(ctx context.Context, id int64) (*UserSubscription, error) {
	sub, err := r.client.UserSubscription.Query().
		Where(usersubscription.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	notes := ""
	if sub.Notes != nil {
		notes = *sub.Notes
	}
	return &UserSubscription{
		ID:                 sub.ID,
		UserID:             sub.UserID,
		GroupID:            sub.GroupID,
		StartsAt:           sub.StartsAt,
		ExpiresAt:          sub.ExpiresAt,
		Status:             sub.Status,
		DailyWindowStart:   sub.DailyWindowStart,
		WeeklyWindowStart:  sub.WeeklyWindowStart,
		MonthlyWindowStart: sub.MonthlyWindowStart,
		DailyUsageUSD:      sub.DailyUsageUsd,
		WeeklyUsageUSD:     sub.WeeklyUsageUsd,
		MonthlyUsageUSD:    sub.MonthlyUsageUsd,
		AssignedAt:         sub.AssignedAt,
		Notes:              notes,
		CreatedAt:          sub.CreatedAt,
		UpdatedAt:          sub.UpdatedAt,
	}, nil
}

func newSubscriptionDailyResetTestClient(t *testing.T) *dbent.Client {
	t.Helper()

	db, err := sql.Open("sqlite", fmt.Sprintf("file:subscription_daily_reset_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createDailyResetFixture(t *testing.T, client *dbent.Client, expiresAt time.Time) (*dbent.User, *dbent.Group, *dbent.UserSubscription) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	user, err := client.User.Create().
		SetEmail(fmt.Sprintf("daily-reset-%d@example.com", now.UnixNano())).
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetName(fmt.Sprintf("daily-reset-group-%d", now.UnixNano())).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	windowStart := now.Add(-6 * time.Hour)
	sub, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(expiresAt).
		SetStatus(SubscriptionStatusActive).
		SetDailyWindowStart(windowStart).
		SetDailyUsageUsd(12.5).
		SetAssignedAt(now).
		SetNotes("").
		Save(ctx)
	require.NoError(t, err)

	return user, group, sub
}

func TestResetDailyUsageWithTimeDeduction_DeductsTimeAndWritesAudit(t *testing.T) {
	client := newSubscriptionDailyResetTestClient(t)
	ctx := context.Background()
	expiresAt := time.Now().UTC().Add(48 * time.Hour)
	user, _, sub := createDailyResetFixture(t, client, expiresAt)

	svc := NewSubscriptionService(groupRepoNoop{}, resetDailyUserSubRepo{client: client}, nil, client, nil)
	updated, err := svc.ResetDailyUsageWithTimeDeduction(ctx, user.ID, sub.ID)
	require.NoError(t, err)

	require.WithinDuration(t, expiresAt.Add(-24*time.Hour), updated.ExpiresAt, time.Second)
	require.Zero(t, updated.DailyUsageUSD)
	require.NotNil(t, updated.DailyWindowStart)
	require.True(t, updated.DailyWindowStart.After(*sub.DailyWindowStart))

	audits, err := client.SubscriptionResetAudit.Query().All(ctx)
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Equal(t, sub.ID, audits[0].SubscriptionID)
	require.Equal(t, user.ID, audits[0].UserID)
	require.Equal(t, user.ID, audits[0].OperatorID)
	require.Equal(t, "user", audits[0].OperatorType)
	require.Equal(t, int(subscriptionDailyResetDeduction.Seconds()), audits[0].DeductedSeconds)
	require.Equal(t, 12.5, audits[0].BeforeDailyUsageUsd)
	require.Zero(t, audits[0].AfterDailyUsageUsd)
}

func TestResetDailyUsageWithTimeDeduction_RequiresAtLeast24HoursRemaining(t *testing.T) {
	client := newSubscriptionDailyResetTestClient(t)
	ctx := context.Background()
	expiresAt := time.Now().UTC().Add(23 * time.Hour)
	user, _, sub := createDailyResetFixture(t, client, expiresAt)

	svc := NewSubscriptionService(groupRepoNoop{}, resetDailyUserSubRepo{client: client}, nil, client, nil)
	_, err := svc.ResetDailyUsageWithTimeDeduction(ctx, user.ID, sub.ID)
	require.ErrorIs(t, err, ErrSubscriptionResetInsufficientTime)

	unchanged, err := client.UserSubscription.Get(ctx, sub.ID)
	require.NoError(t, err)
	require.WithinDuration(t, expiresAt, unchanged.ExpiresAt, time.Second)
	require.Equal(t, 12.5, unchanged.DailyUsageUsd)

	count, err := client.SubscriptionResetAudit.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestResetDailyUsageWithTimeDeduction_RejectsWhenFeatureDisabled(t *testing.T) {
	client := newSubscriptionDailyResetTestClient(t)
	ctx := context.Background()
	expiresAt := time.Now().UTC().Add(48 * time.Hour)
	user, _, sub := createDailyResetFixture(t, client, expiresAt)

	svc := NewSubscriptionService(groupRepoNoop{}, resetDailyUserSubRepo{client: client}, nil, client, nil)
	svc.SetSettingRepository(&resetDailySettingRepo{values: map[string]string{
		SettingKeySubscriptionDailyResetEnabled: "false",
	}})

	_, err := svc.ResetDailyUsageWithTimeDeduction(ctx, user.ID, sub.ID)
	require.ErrorIs(t, err, ErrSubscriptionDailyResetDisabled)

	unchanged, err := client.UserSubscription.Get(ctx, sub.ID)
	require.NoError(t, err)
	require.WithinDuration(t, expiresAt, unchanged.ExpiresAt, time.Second)
	require.Equal(t, 12.5, unchanged.DailyUsageUsd)

	count, err := client.SubscriptionResetAudit.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}
