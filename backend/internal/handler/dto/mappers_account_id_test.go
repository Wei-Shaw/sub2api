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
