//go:build unit

package service

import (
	"context"
	"time"
)

// testPtrFloat64 returns a pointer to the given float64 value.
func testPtrFloat64(v float64) *float64 { return &v }

// testPtrInt returns a pointer to the given int value.
func testPtrInt(v int) *int { return &v }

// testPtrString returns a pointer to the given string value.
func testPtrString(v string) *string { return &v }

// testPtrBool returns a pointer to the given bool value.
func testPtrBool(v bool) *bool { return &v }

// ptrFloat returns a pointer to the given float64 value.
func ptrFloat(v float64) *float64 { return &v }

// userPlatformQuotaRepoStub is a stub for UserPlatformQuotaRepository used across tests.
type userPlatformQuotaRepoStub struct {
	bulkInsertCalls [][]UserPlatformQuotaRecord
}

func (s *userPlatformQuotaRepoStub) GetByUserPlatform(context.Context, int64, string) (*UserPlatformQuotaRecord, error) {
	return nil, nil
}

func (s *userPlatformQuotaRepoStub) BulkInsertInitial(_ context.Context, records []UserPlatformQuotaRecord) error {
	s.bulkInsertCalls = append(s.bulkInsertCalls, records)
	return nil
}

func (s *userPlatformQuotaRepoStub) IncrementUsageWithReset(_ context.Context, _ int64, _ string, _ float64, _ time.Time) error {
	return nil
}

func (s *userPlatformQuotaRepoStub) ListByUser(context.Context, int64) ([]UserPlatformQuotaRecord, error) {
	return nil, nil
}

func (s *userPlatformQuotaRepoStub) UpsertForUser(context.Context, int64, []UserPlatformQuotaRecord) error {
	return nil
}

func (s *userPlatformQuotaRepoStub) ResetExpiredWindow(_ context.Context, _ int64, _ string, _ string, _ time.Time) error {
	return nil
}
