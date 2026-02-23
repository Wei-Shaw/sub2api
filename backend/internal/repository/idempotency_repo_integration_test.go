//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestIdempotencyRepo_CreateProcessing_CompeteSameKey(t *testing.T) {
	tx := testTx(t)
	repo := &idempotencyRepository{sql: txREDACTED
	ctx := context.Background()

	now := time.Now().UTC()
	record := &service.IdempotencyRecord{
		Scope:              uniqueTestValue(t, "idem-scope-create"),
		IdempotencyKeyHash: uniqueTestValue(t, "idem-hash"),
		RequestFingerprint: uniqueTestValue(t, "idem-fp"),
		Status:             service.IdempotencyStatusProcessing,
		LockedUntil:        ptrTime(now.Add(30 * time.Second)),
		ExpiresAt:          now.Add(24 * time.Hour),
REDACTED
	owner, err := repo.CreateProcessing(ctx, record)
REDACTED
	require.True(t, owner)
	require.NotZero(t, record.ID)

	duplicate := &service.IdempotencyRecord{
		Scope:              record.Scope,
		IdempotencyKeyHash: record.IdempotencyKeyHash,
		RequestFingerprint: uniqueTestValue(t, "idem-fp-other"),
		Status:             service.IdempotencyStatusProcessing,
		LockedUntil:        ptrTime(now.Add(30 * time.Second)),
		ExpiresAt:          now.Add(24 * time.Hour),
REDACTED
	owner, err = repo.CreateProcessing(ctx, duplicate)
REDACTED
	require.False(t, owner, "same scope+key hash should be de-duplicated")
REDACTED

func TestIdempotencyRepo_TryReclaim_StatusAndLockWindow(t *testing.T) {
	tx := testTx(t)
	repo := &idempotencyRepository{sql: txREDACTED
	ctx := context.Background()

	now := time.Now().UTC()
	record := &service.IdempotencyRecord{
		Scope:              uniqueTestValue(t, "idem-scope-reclaim"),
		IdempotencyKeyHash: uniqueTestValue(t, "idem-hash-reclaim"),
		RequestFingerprint: uniqueTestValue(t, "idem-fp-reclaim"),
		Status:             service.IdempotencyStatusProcessing,
		LockedUntil:        ptrTime(now.Add(10 * time.Second)),
		ExpiresAt:          now.Add(24 * time.Hour),
REDACTED
	owner, err := repo.CreateProcessing(ctx, record)
REDACTED
	require.True(t, owner)

	require.NoError(t, repo.MarkFailedRetryable(
		ctx,
		record.ID,
		"RETRYABLE_FAILURE",
		now.Add(-2*time.Second),
		now.Add(24*time.Hour),
	))

	newLockedUntil := now.Add(20 * time.Second)
	reclaimed, err := repo.TryReclaim(
		ctx,
		record.ID,
		service.IdempotencyStatusFailedRetryable,
		now,
		newLockedUntil,
		now.Add(24*time.Hour),
	)
REDACTED
	require.True(t, reclaimed, "failed_retryable + expired lock should allow reclaim")

	got, err := repo.GetByScopeAndKeyHash(ctx, record.Scope, record.IdempotencyKeyHash)
REDACTED
	require.NotNil(t, got)
	require.Equal(t, service.IdempotencyStatusProcessing, got.Status)
	require.NotNil(t, got.LockedUntil)
	require.True(t, got.LockedUntil.After(now))

	require.NoError(t, repo.MarkFailedRetryable(
		ctx,
		record.ID,
		"RETRYABLE_FAILURE",
		now.Add(20*time.Second),
		now.Add(24*time.Hour),
	))

	reclaimed, err = repo.TryReclaim(
		ctx,
		record.ID,
		service.IdempotencyStatusFailedRetryable,
		now,
		now.Add(40*time.Second),
		now.Add(24*time.Hour),
	)
REDACTED
	require.False(t, reclaimed, "within lock window should not reclaim")
REDACTED

func TestIdempotencyRepo_StatusTransition_ToSucceeded(t *testing.T) {
	tx := testTx(t)
	repo := &idempotencyRepository{sql: txREDACTED
	ctx := context.Background()

	now := time.Now().UTC()
	record := &service.IdempotencyRecord{
		Scope:              uniqueTestValue(t, "idem-scope-success"),
		IdempotencyKeyHash: uniqueTestValue(t, "idem-hash-success"),
		RequestFingerprint: uniqueTestValue(t, "idem-fp-success"),
		Status:             service.IdempotencyStatusProcessing,
		LockedUntil:        ptrTime(now.Add(10 * time.Second)),
		ExpiresAt:          now.Add(24 * time.Hour),
REDACTED
	owner, err := repo.CreateProcessing(ctx, record)
REDACTED
	require.True(t, owner)

	require.NoError(t, repo.MarkSucceeded(ctx, record.ID, 200, `{"ok":trueREDACTED`, now.Add(24*time.Hour)))

	got, err := repo.GetByScopeAndKeyHash(ctx, record.Scope, record.IdempotencyKeyHash)
REDACTED
	require.NotNil(t, got)
	require.Equal(t, service.IdempotencyStatusSucceeded, got.Status)
	require.NotNil(t, got.ResponseStatus)
	require.Equal(t, 200, *got.ResponseStatus)
	require.NotNil(t, got.ResponseBody)
	require.Equal(t, `{"ok":trueREDACTED`, *got.ResponseBody)
	require.Nil(t, got.LockedUntil)
REDACTED

func ptrTime(v time.Time) *time.Time {
	return &v
REDACTED
