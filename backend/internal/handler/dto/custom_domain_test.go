//go:build unit

package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCustomDomainProjectionPreservesAdminIdentityAndRedactsNonManagerAccess(t *testing.T) {
	domain := &service.CustomDomain{
		ID:                   73,
		UserID:               41,
		AllUsers:             true,
		AuthorizedUserIDs:    []int64{42},
		Domain:               "api.example.com",
		Status:               service.CustomDomainStatusActive,
		VerificationTXTName:  "_sub2api.api.example.com",
		VerificationTXTValue: "sub2api-domain-verification=token",
		User: &service.User{
			ID: 41, Email: "owner@example.com", Username: "owner", Role: service.RoleUser,
		},
		AuthorizedUsers: []service.User{
			{ID: 42, Email: "allowed@example.com", Username: "allowed", Role: service.RoleAdmin},
		},
		CanManage: false,
	}

	admin := CustomDomainFromService(domain)
	require.Equal(t, []int64{42}, admin.UserIDs)
	require.Equal(t, &CustomDomainUser{ID: 41, Email: "owner@example.com", Username: "owner", Role: service.RoleUser}, admin.User)
	require.Equal(t, []CustomDomainUser{{ID: 42, Email: "allowed@example.com", Username: "allowed", Role: service.RoleAdmin}}, admin.Users)

	user := CustomDomainForUserFromService(domain, 42)
	require.False(t, user.CanManage)
	require.Zero(t, user.UserID)
	require.False(t, user.AllUsers)
	require.Nil(t, user.UserIDs)
	require.Empty(t, user.VerificationTXTName)
	require.Empty(t, user.VerificationTXTValue)
	require.Nil(t, user.User)
	require.Nil(t, user.Users)

	domain.CanManage = true
	manager := CustomDomainForUserFromService(domain, 41)
	require.True(t, manager.CanManage)
	require.Equal(t, int64(41), manager.UserID)
	require.Equal(t, []int64{42}, manager.UserIDs)
	require.NotNil(t, manager.User)
	require.Len(t, manager.Users, 1)
}
