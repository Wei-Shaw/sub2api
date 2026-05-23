// Package gatewayutil — model mapping helpers used by gateway plugins.
package gatewayutil

import "slices"

// ResolveModelMapping returns the upstream model name to use for the given
// requestedModel, applying the credentials-encoded model_mapping table only
// when the account's type appears in enabledTypes.
//
// Behavior:
//   - currentType not in enabledTypes → requestedModel is returned unchanged
//     (account types like OAuth typically rely on host-side discovery, not
//     credential-level overrides).
//   - mapping is empty or has no entry for requestedModel → unchanged.
//   - mapping has an entry → the mapped value is returned.
//
// credentialsJSON is the raw JSON blob from GatewayAccountInfo.CredentialsJson
// or RefreshToken/ValidateAccountData credentials.
func ResolveModelMapping(
	requestedModel string,
	credentialsJSON []byte,
	enabledTypes []string,
	currentType string,
) string {
	if !slices.Contains(enabledTypes, currentType) {
		return requestedModel
	}
	mapping := ExtractModelMapping(credentialsJSON)
	if len(mapping) == 0 {
		return requestedModel
	}
	if mapped, ok := mapping[requestedModel]; ok {
		return mapped
	}
	return requestedModel
}
