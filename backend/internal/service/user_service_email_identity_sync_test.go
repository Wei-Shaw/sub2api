//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateProfile_ReplacesEmailAuthIdentityWhenEmailChanges(t *testing.T) {
	repo := &emailSyncRepoStub{
		user: &User{
			ID:          19,
			Email:       "profile-before@example.com",
			Username:    "tester",
			Concurrency: 2,
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
