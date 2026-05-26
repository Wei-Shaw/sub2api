//go:build unit

package repository

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepoForAdapter struct {
	upsertCalledWith   []UserPlatformQuotaRecord
	upsertCalledUserID int64
	upsertErr          error
	resetCalledWith    [4]any // userID, platform, window, newStart
	resetErr           error
REDACTED

func (f *fakeRepoForAdapter) BulkInsertInitial(_ context.Context, _ []UserPlatformQuotaRecord) error {
	return nil
REDACTED
func (f *fakeRepoForAdapter) GetByUserPlatform(_ context.Context, _ int64, _ string) (*UserPlatformQuotaRecord, error) {
	return nil, nil
REDACTED
func (f *fakeRepoForAdapter) ListByUser(_ context.Context, _ int64) ([]UserPlatformQuotaRecord, error) {
	return nil, nil
REDACTED
func (f *fakeRepoForAdapter) IncrementUsageWithReset(_ context.Context, _ int64, _ string, _ float64, _ time.Time) error {
	return nil
REDACTED
func (f *fakeRepoForAdapter) ResetExpiredWindow(_ context.Context, userID int64, platform string, window string, newStart time.Time) error {
	f.resetCalledWith = [4]any{userID, platform, window, newStartREDACTED
	return f.resetErr
REDACTED
func (f *fakeRepoForAdapter) UpsertForUser(_ context.Context, userID int64, records []UserPlatformQuotaRecord) error {
	f.upsertCalledUserID = userID
	f.upsertCalledWith = records
	return f.upsertErr
REDACTED

func TestGenericAdapter_UpsertForUser_ForwardsRecords(t *testing.T) {
	fake := &fakeRepoForAdapter{REDACTED
	adapter := NewUserPlatformQuotaServiceAdapter(fake)

	err := adapter.UpsertForUser(context.Background(), 42, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
REDACTED
	if fake.upsertCalledUserID != 42 {
		t.Errorf("forwarded userID = %d, want 42", fake.upsertCalledUserID)
REDACTED
REDACTED

func TestGenericAdapter_UpsertForUser_PropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	fake := &fakeRepoForAdapter{upsertErr: wantErrREDACTED
	adapter := NewUserPlatformQuotaServiceAdapter(fake)

	err := adapter.UpsertForUser(context.Background(), 1, nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("expected %v, got %v", wantErr, err)
REDACTED
REDACTED

func TestGenericAdapter_ResetExpiredWindow_ForwardsAllParams(t *testing.T) {
	fake := &fakeRepoForAdapter{REDACTED
	adapter := NewUserPlatformQuotaServiceAdapter(fake)

	now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	if err := adapter.ResetExpiredWindow(context.Background(), 7, "openai", "weekly", now); err != nil {
		t.Fatalf("unexpected err: %v", err)
REDACTED
	if fake.resetCalledWith[0].(int64) != 7 ||
		fake.resetCalledWith[1].(string) != "openai" ||
		fake.resetCalledWith[2].(string) != "weekly" ||
		!fake.resetCalledWith[3].(time.Time).Equal(now) {
		t.Errorf("forwarded params mismatch: %+v", fake.resetCalledWith)
REDACTED
REDACTED

func TestGenericAdapter_ResetExpiredWindow_PropagatesError(t *testing.T) {
	wantErr := errors.New("not found")
	fake := &fakeRepoForAdapter{resetErr: wantErrREDACTED
	adapter := NewUserPlatformQuotaServiceAdapter(fake)

	err := adapter.ResetExpiredWindow(context.Background(), 1, "a", "daily", time.Now())
	if !errors.Is(err, wantErr) {
		t.Errorf("expected %v, got %v", wantErr, err)
REDACTED
REDACTED
