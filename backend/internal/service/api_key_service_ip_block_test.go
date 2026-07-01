//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyServiceDisableUserForIPBlockSkipsAdmin(t *testing.T) {
	repo := &userRepoStub{
		user: &User{
			ID:     1,
			Email:  "admin@example.com",
			Role:   RoleAdmin,
			Status: StatusActive,
		},
	}
	svc := NewAPIKeyService(nil, repo, nil, nil, nil, nil, &config.Config{})

	err := svc.DisableUserForIPBlock(context.Background(), 1, "1.2.3.4")

	require.NoError(t, err)
	require.Equal(t, StatusActive, repo.user.Status)
	require.Empty(t, repo.updated)
}

func TestAPIKeyServiceDisableUserForIPBlockDisablesRegularUser(t *testing.T) {
	repo := &userRepoStub{
		user: &User{
			ID:     2,
			Email:  "user@example.com",
			Role:   RoleUser,
			Status: StatusActive,
		},
	}
	svc := NewAPIKeyService(nil, repo, nil, nil, nil, nil, &config.Config{})

	err := svc.DisableUserForIPBlock(context.Background(), 2, "1.2.3.4")

	require.NoError(t, err)
	require.Equal(t, StatusDisabled, repo.user.Status)
	require.Len(t, repo.updated, 1)
}
