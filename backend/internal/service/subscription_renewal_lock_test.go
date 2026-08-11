package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type lockingRenewalRepo struct {
	userSubRepoNoop
	mu        sync.Mutex
	stale     UserSubscription
	current   UserSubscription
	lockReads int
REDACTED

func (r *lockingRenewalRepo) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return true, nil
REDACTED

func (r *lockingRenewalRepo) GetByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	copy := r.stale
	return &copy, nil
REDACTED

func (r *lockingRenewalRepo) GetByID(_ context.Context, _ int64) (*UserSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := r.current
	return &copy, nil
REDACTED

func (r *lockingRenewalRepo) GetByIDForUpdate(_ context.Context, _ int64) (*UserSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lockReads++
	copy := r.current
	return &copy, nil
REDACTED

func (r *lockingRenewalRepo) ExtendExpiry(_ context.Context, _ int64, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current.ExpiresAt = expiresAt
	return nil
REDACTED

func (r *lockingRenewalRepo) UpdateStatus(_ context.Context, _ int64, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current.Status = status
	return nil
REDACTED

func (r *lockingRenewalRepo) UpdateNotes(_ context.Context, _ int64, notes string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current.Notes = notes
	return nil
REDACTED

func (r *lockingRenewalRepo) Update(_ context.Context, sub *UserSubscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current = *sub
	return nil
REDACTED

func TestAssignOrExtendSubscriptionUsesLockedCurrentRow(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	lockedExpiry := now.AddDate(0, 0, 20)
	windowStart := now.Add(-24 * time.Hour)
	repo := &lockingRenewalRepo{
		stale: UserSubscription{ID: 7, UserID: 11, GroupID: 13, ExpiresAt: now.Add(-time.Hour), Status: SubscriptionStatusExpired, Notes: "stale"REDACTED,
		current: UserSubscription{
			ID: 7, UserID: 11, GroupID: 13, StartsAt: now.AddDate(0, 0, -10), ExpiresAt: lockedExpiry,
			Status: SubscriptionStatusSuspended, Notes: "current", DailyWindowStart: &windowStart, DailyUsageUSD: 4,
	REDACTED,
REDACTED
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{ID: 13, SubscriptionType: SubscriptionTypeSubscriptionREDACTEDREDACTED, repo, nil, nil, nil)
	svc.now = func() time.Time { return now REDACTED

	sub, extended, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
		UserID: 11, GroupID: 13, ValidityDays: 5, Notes: "renewed",
REDACTED)

REDACTED
	require.True(t, extended)
	require.Equal(t, 1, repo.lockReads)
	require.Equal(t, lockedExpiry.AddDate(0, 0, 5), sub.ExpiresAt)
	require.Equal(t, SubscriptionStatusActive, sub.Status)
	require.Equal(t, "current\nrenewed", sub.Notes)
	require.Equal(t, windowStart, *sub.DailyWindowStart)
	require.Equal(t, float64(4), sub.DailyUsageUSD)
REDACTED

func TestAssignOrExtendSubscriptionSerializedRenewalsAccumulateDays(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	initialExpiry := now.AddDate(0, 0, 10)
	stale := UserSubscription{ID: 17, UserID: 21, GroupID: 23, StartsAt: now, ExpiresAt: initialExpiry, Status: SubscriptionStatusActiveREDACTED
	repo := &lockingRenewalRepo{stale: stale, current: staleREDACTED
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{ID: 23, SubscriptionType: SubscriptionTypeSubscriptionREDACTEDREDACTED, repo, nil, nil, nil)
	svc.now = func() time.Time { return now REDACTED
	input := &AssignSubscriptionInput{UserID: 21, GroupID: 23, ValidityDays: 7REDACTED

	_, _, err := svc.AssignOrExtendSubscription(context.Background(), input)
REDACTED
	second, _, err := svc.AssignOrExtendSubscription(context.Background(), input)
REDACTED

	require.Equal(t, 2, repo.lockReads)
	require.Equal(t, initialExpiry.AddDate(0, 0, 14), second.ExpiresAt)
REDACTED

func TestAssignSubscriptionDoesNotReactivateRowSuspendedAfterStaleRead(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	windowStart := now.Add(-24 * time.Hour)
	current := UserSubscription{
		ID: 27, UserID: 31, GroupID: 33, StartsAt: now.AddDate(0, 0, -10), ExpiresAt: now.Add(-time.Hour),
		Status: SubscriptionStatusSuspended, Notes: "suspended", DailyWindowStart: &windowStart, DailyUsageUSD: 4,
REDACTED
	repo := &lockingRenewalRepo{
		stale:   UserSubscription{ID: 27, UserID: 31, GroupID: 33, ExpiresAt: now.Add(-time.Hour), Status: SubscriptionStatusExpiredREDACTED,
		current: current,
REDACTED
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{ID: 33, SubscriptionType: SubscriptionTypeSubscriptionREDACTEDREDACTED, repo, nil, nil, nil)
	svc.now = func() time.Time { return now REDACTED

	sub, reused, err := svc.assignSubscriptionWithReuse(context.Background(), &AssignSubscriptionInput{
		UserID: 31, GroupID: 33, ValidityDays: 5, Notes: "renewed",
REDACTED)

REDACTED
	require.True(t, reused)
	require.Equal(t, 1, repo.lockReads)
	require.Equal(t, current, repo.current)
	require.Equal(t, SubscriptionStatusSuspended, sub.Status)
	require.Equal(t, current.ExpiresAt, sub.ExpiresAt)
	require.Equal(t, current.Notes, sub.Notes)
REDACTED
