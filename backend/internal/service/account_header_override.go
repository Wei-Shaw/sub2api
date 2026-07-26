package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// Header-override Account methods + NormalizeHeaderOverrideCredentials moved to
// domain in Phase 3 (Account BC hybrid). Re-export the save-path helper and the
// credential-key consts so admin_account.go keeps compiling under the original
// names. The ~20 ApplyHeaderOverrides call sites need no change (method on
// domain.Account via the type alias).

const (
	credKeyHeaderOverrideEnabled = domain.CredKeyHeaderOverrideEnabled
	credKeyHeaderOverrides       = domain.CredKeyHeaderOverrides

	maxHeaderOverrideEntries     = domain.MaxHeaderOverrideEntries
	maxHeaderOverrideNameLength  = domain.MaxHeaderOverrideNameLength
	maxHeaderOverrideValueLength = domain.MaxHeaderOverrideValueLength
)

// NormalizeHeaderOverrideCredentials re-exports the domain helper so the admin
// save path keeps calling it under the original package-local name.
func NormalizeHeaderOverrideCredentials(credentials map[string]any) error {
	return domain.NormalizeHeaderOverrideCredentials(credentials)
}
