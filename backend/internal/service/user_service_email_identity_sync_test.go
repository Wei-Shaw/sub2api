//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type emailSyncMockUserRepo struct {
	*mockUserRepo
	ensureCalls  []ensureEmailCall
	replaceCalls []replaceEmailCall
REDACTED

func (m *emailSyncMockUserRepo) EnsureEmailAuthIdentity(_ context.Context, userID int64, email string) error {
	m.ensureCalls = append(m.ensureCalls, ensureEmailCall{userID: userID, email: emailREDACTED)
	return nil
REDACTED

func (m *emailSyncMockUserRepo) ReplaceEmailAuthIdentity(_ context.Context, userID int64, oldEmail, newEmail string) error {
	m.replaceCalls = append(m.replaceCalls, replaceEmailCall{
		userID:   userID,
		oldEmail: oldEmail,
		newEmail: newEmail,
REDACTED)
	return nil
REDACTED

func (m *emailSyncMockUserRepo) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{REDACTED, nil
REDACTED

func (m *emailSyncMockUserRepo) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
REDACTED

func TestUpdateProfile_ReplacesEmailAuthIdentityWhenEmailChanges(t *testing.T) {
	repo := &emailSyncMockUserRepo{
		mockUserRepo: &mockUserRepo{
			getByIDUser: &User{
				ID:          19,
				Email:       "profile-before@example.com",
				Username:    "tester",
				Concurrency: 2,
		REDACTED,
	REDACTED,
REDACTED
	svc := NewUserService(repo, nil, nil, nil)

	newEmail := "profile-after@example.com"
	updated, err := svc.UpdateProfile(context.Background(), 19, UpdateProfileRequest{
		Email: &newEmail,
REDACTED)
REDACTED
	require.NotNil(t, updated)
	require.Equal(t, newEmail, updated.Email)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, []replaceEmailCall{{
		userID:   19,
		oldEmail: "profile-before@example.com",
		newEmail: "profile-after@example.com",
REDACTEDREDACTED, repo.replaceCalls)
	require.Empty(t, repo.ensureCalls)
REDACTED
