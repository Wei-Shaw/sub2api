package admin

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRelayCodexImportDeduplicatesOnlyWithinTheCurrentOwner(t *testing.T) {
	token := buildCodexAccessToken(t, "shared-workspace", "upstream-user", time.Now().Add(time.Hour))
	for _, currentOwner := range []int64{7, 8} {
		owner := int64(7)
		svc := newCodexImportMemoryAdminService([]service.Account{{
			ID: 10, Name: "existing", OwnerUserID: &owner, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
			Credentials: map[string]any{"chatgpt_account_id": "shared-workspace", "chatgpt_user_id": "upstream-user", "access_token": token},
		}})
		h := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		result, err := h.importCodexSessions(service.WithRelayOwner(context.Background(), currentOwner),
			CodexSessionImportRequest{SkipDefaultGroupBind: boolPtr(true)},
			[]codexImportEntry{{Index: 1, Value: map[string]any{"access_token": token}}})
		require.NoError(t, err)
		require.Zero(t, result.Failed)
		if currentOwner == owner {
			require.Equal(t, 1, result.Updated)
			require.Empty(t, svc.createdAccounts)
		} else {
			require.Equal(t, 1, result.Created)
			require.Empty(t, svc.updatedAccounts)
			require.Equal(t, token, svc.accounts[0].Credentials["access_token"])
		}
	}
}
