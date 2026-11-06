package service

import (
	"crypto/sha256"
	"encoding/json"
)

// An account row can be reauthorized for a different Google identity without
// changing its database ID. Scope both cached snapshots and in-flight queries
// to the current credentials, not mutable last-used timestamps. Only the hash
// is retained; credentials must never appear in flight keys or diagnostics.
func antigravityUsageIdentity(account *Account) [32]byte {
	encoded, _ := json.Marshal([]string{
		account.GetCredential("access_token"),
		account.GetCredential("refresh_token"),
		account.GetCredential("project_id"),
	})
	return sha256.Sum256(encoded)
}
