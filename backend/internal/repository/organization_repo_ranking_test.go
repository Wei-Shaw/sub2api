//go:build unit

package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOrganizationRankingUserLabel(t *testing.T) {
	tests := []struct {
		name         string
		username     string
		email        string
		loginName    string
		identityType string
		companyID    string
		want         string
	}{
		{name: "username wins", username: "Finance Reader", email: "reader@example.com", loginName: "reader", identityType: service.IdentityTypeIAM, companyID: "c123456789012345", want: "Finance Reader"},
		{name: "IAM principal fallback", loginName: "reader", identityType: service.IdentityTypeIAM, companyID: "c123456789012345", want: "reader@c123456789012345.opentk.ai"},
		{name: "root email fallback", email: "owner@example.com", identityType: service.IdentityTypeRoot, want: "owner@example.com"},
		{name: "legacy login fallback", loginName: "legacy", want: "legacy"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, organizationRankingUserLabel(test.username, test.email, test.loginName, test.identityType, test.companyID))
		})
	}
}
