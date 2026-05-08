//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminService_UpdateUser_AllowsPromoteToAdmin(t *testing.T) {
	repo := &userRepoStub{
		user: &User{
			ID:          21,
			Email:       "user@test.com",
			Role:        RoleUser,
			Status:      StatusActive,
			Concurrency: 1,
		},
	}
	svc := &adminServiceImpl{userRepo: repo}
	role := RoleAdmin

	user, err := svc.UpdateUser(context.Background(), 21, &UpdateUserInput{Role: &role})
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, RoleAdmin, user.Role)
	require.Len(t, repo.updated, 1)
	require.Equal(t, RoleAdmin, repo.updated[0].Role)
}

func TestAdminService_UpdateUser_RejectsInvalidRole(t *testing.T) {
	repo := &userRepoStub{
		user: &User{
			ID:          22,
			Email:       "user@test.com",
			Role:        RoleUser,
			Status:      StatusActive,
			Concurrency: 1,
		},
	}
	svc := &adminServiceImpl{userRepo: repo}
	role := "owner"

	_, err := svc.UpdateUser(context.Background(), 22, &UpdateUserInput{Role: &role})
	require.Error(t, err)
	require.Empty(t, repo.updated)
}

func TestAdminService_UpdateUser_RejectsDemotingLastActiveAdmin(t *testing.T) {
	repo := &userRepoStub{
		user: &User{
			ID:          23,
			Email:       "admin@test.com",
			Role:        RoleAdmin,
			Status:      StatusActive,
			Concurrency: 1,
		},
		listUsers: []User{
			{
				ID:     23,
				Email:  "admin@test.com",
				Role:   RoleAdmin,
				Status: StatusActive,
			},
		},
	}
	svc := &adminServiceImpl{userRepo: repo}
	role := RoleUser

	_, err := svc.UpdateUser(context.Background(), 23, &UpdateUserInput{Role: &role})
	require.Error(t, err)
	require.Empty(t, repo.updated)
}

func TestAdminService_UpdateUser_AllowsDemotingAdminWhenAnotherActiveAdminExists(t *testing.T) {
	repo := &userRepoStub{
		user: &User{
			ID:          24,
			Email:       "admin-a@test.com",
			Role:        RoleAdmin,
			Status:      StatusActive,
			Concurrency: 1,
		},
		listUsers: []User{
			{
				ID:     24,
				Email:  "admin-a@test.com",
				Role:   RoleAdmin,
				Status: StatusActive,
			},
			{
				ID:     25,
				Email:  "admin-b@test.com",
				Role:   RoleAdmin,
				Status: StatusActive,
			},
		},
	}
	svc := &adminServiceImpl{userRepo: repo}
	role := RoleUser

	user, err := svc.UpdateUser(context.Background(), 24, &UpdateUserInput{Role: &role})
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, RoleUser, user.Role)
	require.Len(t, repo.updated, 1)
	require.Equal(t, RoleUser, repo.updated[0].Role)
}
