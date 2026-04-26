//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateProfile_DoesNotReturnPartialSuccessFromEmailIdentityResync(t *testing.T) {
	repo := &emailSyncRepoStub{
		user: &User{
			ID:          19,
			Email:       "profile-before@example.com",
			Username:    "tester",
			Concurrency: 2,
		},
		replaceErr: context.DeadlineExceeded,
	}
	svc := NewUserService(repo, nil, nil, nil)

	newEmail := "profile-after@example.com"
	updated, err := svc.UpdateProfile(context.Background(), 19, UpdateProfileRequest{
		Email: &newEmail,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, newEmail, updated.Email)
	require.Equal(t, 1, repo.updateCalls)
	require.Empty(t, repo.replaceCalls)
	require.Empty(t, repo.ensureCalls)
}
