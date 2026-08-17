package service

import (
	"bytes"
	"context"
	"errors"
	"log"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type subscriptionExpiryRepoStub struct {
	listCalls int
REDACTED

func (r *subscriptionExpiryRepoStub) Create(context.Context, *UserSubscription) error {
	return nil
REDACTED

func (r *subscriptionExpiryRepoStub) GetByID(context.Context, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
REDACTED

func (r *subscriptionExpiryRepoStub) GetByIDForUpdate(context.Context, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
REDACTED

func (r *subscriptionExpiryRepoStub) GetByIDIncludeDeleted(context.Context, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
REDACTED

func (r *subscriptionExpiryRepoStub) GetByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
REDACTED

func (r *subscriptionExpiryRepoStub) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
REDACTED

func (r *subscriptionExpiryRepoStub) Update(context.Context, *UserSubscription) error {
	return nil
REDACTED

func (r *subscriptionExpiryRepoStub) Delete(context.Context, int64) error {
	return nil
REDACTED

func (r *subscriptionExpiryRepoStub) Restore(context.Context, int64, string) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
REDACTED

func (r *subscriptionExpiryRepoStub) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, nil
REDACTED

func (r *subscriptionExpiryRepoStub) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, nil
REDACTED

func (r *subscriptionExpiryRepoStub) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED

func (r *subscriptionExpiryRepoStub) List(context.Context, pagination.PaginationParams, *int64, *int64, string, string, string, string) ([]UserSubscription, *pagination.PaginationResult, error) {
	r.listCalls++
	return nil, &pagination.PaginationResult{Page: 1, Pages: 1REDACTED, nil
REDACTED

func (r *subscriptionExpiryRepoStub) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return false, nil
REDACTED

func (r *subscriptionExpiryRepoStub) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return false, nil
REDACTED

func (r *subscriptionExpiryRepoStub) ExtendExpiry(context.Context, int64, time.Time) error {
	return nil
REDACTED

func (r *subscriptionExpiryRepoStub) UpdateStatus(context.Context, int64, string) error {
	return nil
REDACTED

func (r *subscriptionExpiryRepoStub) UpdateNotes(context.Context, int64, string) error {
	return nil
REDACTED

func (r *subscriptionExpiryRepoStub) ActivateWindows(context.Context, int64, time.Time, time.Time) error {
	return nil
REDACTED

func (r *subscriptionExpiryRepoStub) ResetUsageWindows(context.Context, int64, bool, bool, bool, time.Time, time.Time) error {
	return nil
REDACTED

func (r *subscriptionExpiryRepoStub) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
REDACTED

func (r *subscriptionExpiryRepoStub) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
REDACTED

func (r *subscriptionExpiryRepoStub) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
REDACTED

func (r *subscriptionExpiryRepoStub) IncrementUsage(context.Context, int64, float64) error {
	return nil
REDACTED

func (r *subscriptionExpiryRepoStub) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	return 0, nil
REDACTED

type subscriptionExpirySettingRepoStub struct {
	values   map[string]string
	err      error
	multiErr error
REDACTED

func (r *subscriptionExpirySettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
REDACTED

func (r *subscriptionExpirySettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if r.err != nil {
		return "", r.err
REDACTED
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
REDACTED
	return value, nil
REDACTED

func (r *subscriptionExpirySettingRepoStub) Set(context.Context, string, string) error {
	return nil
REDACTED

func (r *subscriptionExpirySettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	if r.multiErr != nil {
		return nil, r.multiErr
REDACTED
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			values[key] = value
	REDACTED
REDACTED
	return values, nil
REDACTED

func (r *subscriptionExpirySettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
REDACTED

func (r *subscriptionExpirySettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return nil, nil
REDACTED

func (r *subscriptionExpirySettingRepoStub) Delete(context.Context, string) error {
	return nil
REDACTED

func TestSubscriptionExpiryService_ExpiryReminderEnabledDefaultsToTrue(t *testing.T) {
	svc := NewSubscriptionExpiryService(nil, time.Minute)
	svc.SetSettingRepository(&subscriptionExpirySettingRepoStub{values: map[string]string{REDACTEDREDACTED)

	require.True(t, svc.expiryReminderEnabled(context.Background()))
REDACTED

func TestSubscriptionExpiryService_ExpiryReminderDisabledSkipsSubscriptionScan(t *testing.T) {
	repo := &subscriptionExpiryRepoStub{REDACTED
	settingRepo := &subscriptionExpirySettingRepoStub{
		values: map[string]string{SettingKeySubscriptionExpiryNotifyEnabled: "false"REDACTED,
REDACTED
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, nil))

	svc.sendExpiryReminders(context.Background())

	require.Zero(t, repo.listCalls)
REDACTED

func TestSubscriptionExpiryService_ExpiryReminderSettingReadErrorFailsClosed(t *testing.T) {
	svc := NewSubscriptionExpiryService(nil, time.Minute)
	svc.SetSettingRepository(&subscriptionExpirySettingRepoStub{err: errors.New("db down")REDACTED)

	require.False(t, svc.expiryReminderEnabled(context.Background()))
REDACTED

func TestSubscriptionExpiryService_MissingSMTPSkipsReminderScanAndLogsOncePerInterval(t *testing.T) {
	repo := &subscriptionExpiryRepoStub{REDACTED
	settingRepo := &subscriptionExpirySettingRepoStub{values: map[string]string{REDACTEDREDACTED
	emailService := NewEmailService(settingRepo, nil)
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, emailService))

	var logs bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
REDACTED)

	svc.sendExpiryReminders(context.Background())
	svc.sendExpiryReminders(context.Background())

	require.Zero(t, repo.listCalls)
	require.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("SMTP is not configured")))
REDACTED

func TestSubscriptionExpiryService_SMTPConfigReadErrorSkipsReminderScan(t *testing.T) {
	repo := &subscriptionExpiryRepoStub{REDACTED
	settingRepo := &subscriptionExpirySettingRepoStub{
		values:   map[string]string{REDACTED,
		multiErr: errors.New("db down"),
REDACTED
	emailService := NewEmailService(settingRepo, nil)
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, emailService))

	svc.sendExpiryReminders(context.Background())

	require.Zero(t, repo.listCalls)
REDACTED
