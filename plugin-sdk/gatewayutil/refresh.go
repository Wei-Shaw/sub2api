package gatewayutil

import (
	"encoding/json"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// HandleNoRefreshToken returns the existing access_token info when no
// refresh_token is available (same fallback as the host service).
// All four gateway plugins use this identical logic.
func HandleNoRefreshToken(creds map[string]any) (*pb.RefreshTokenResponse, error) {
	if CredStr(creds, "access_token") == "" {
		return &pb.RefreshTokenResponse{
			Success: false,
			Error:   "no refresh_token or access_token available",
		}, nil
	}
	// Return existing credentials unchanged -- host preserves them.
	credsJSON, _ := json.Marshal(creds)
	return &pb.RefreshTokenResponse{
		Success:                true,
		UpdatedCredentialsJson: credsJSON,
	}, nil
}
