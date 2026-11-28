package service

import (
	"encoding/json"
	"fmt"
	"math"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const MuseOwnerUserIDKey = "muse_owner_user_id"

// ValidateMuseAccount validates the local session envelope, not Meta's wire
// credentials. Actual validity is determined only by the provider verifier.
func ValidateMuseAccount(platform, accountType string, credentials, extra map[string]any) error {
	if platform != PlatformMuse {
		if accountType == AccountTypeSession {
			return infraerrors.BadRequest("SESSION_PLATFORM_INVALID", "session accounts are only supported for Meta Muse")
		}
		return nil
	}
	if accountType != AccountTypeSession {
		return infraerrors.BadRequest("MUSE_ACCOUNT_TYPE_INVALID", "Meta Muse requires a consumer app session account")
	}
	bundle, ok := credentials["muse_session"].(map[string]any)
	if !ok || len(bundle) == 0 {
		return infraerrors.BadRequest("MUSE_SESSION_REQUIRED", "a nonempty session document is required")
	}
	encoded, err := json.Marshal(bundle)
	if err != nil || len(encoded) > 64<<10 {
		return infraerrors.BadRequest("MUSE_SESSION_INVALID", "session document must be valid JSON no larger than 64 KiB")
	}
	if MuseOwnerUserID(extra) <= 0 {
		return infraerrors.BadRequest("MUSE_OWNER_REQUIRED", "assign the Muse workspace to a Sub2API user")
	}
	for key := range credentials {
		if key != "muse_session" && key != "model_mapping" {
			return infraerrors.BadRequest("MUSE_CREDENTIAL_FIELD_INVALID", fmt.Sprintf("unsupported Muse credential field: %s", key))
		}
	}
	return nil
}

func MuseOwnerUserID(extra map[string]any) int64 {
	switch n := extra[MuseOwnerUserIDKey].(type) {
	case int64:
		if n > 0 {
			return n
		}
	case int:
		if n > 0 {
			return int64(n)
		}
	case float64:
		if n > 0 && n <= 1<<53 && math.Trunc(n) == n {
			return int64(n)
		}
	case json.Number:
		value, err := n.Int64()
		if err == nil && value > 0 {
			return value
		}
	}
	return 0
}
