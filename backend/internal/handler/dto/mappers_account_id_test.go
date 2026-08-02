package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserPublicAccountIDsRemainJSONStrings(t *testing.T) {
	dto := UserFromServiceShallow(&service.User{
		AccountID: "1719905235756637", ExternalUserID: "201705485041478971", IdentityType: "iam", LoginName: "finance",
	})
	wire, err := json.Marshal(dto)
	require.NoError(t, err)
	require.Contains(t, string(wire), `"account_id":"1719905235756637"`)
	require.Contains(t, string(wire), `"external_user_id":"201705485041478971"`)
}

func TestUserFromServiceAdminIncludesIAMPrincipal(t *testing.T) {
	out := UserFromServiceAdmin(&service.User{
		IdentityType:     "iam",
		LoginName:        "finance",
		CompanyID:        "c123456789012345",
		CompanyName:      "Acme AI",
		OrganizationRole: "member",
		RecoveryEmail:    "finance@example.com",
	})

	require.NotNil(t, out)
	require.Equal(t, "finance@c123456789012345.opentk.ai", out.IAMPrincipal)
	require.Equal(t, "finance@example.com", out.RecoveryEmail)
	require.Equal(t, "c123456789012345", out.CompanyID)
	require.Equal(t, "Acme AI", out.CompanyName)
	require.Equal(t, "member", out.OrganizationRole)
}
