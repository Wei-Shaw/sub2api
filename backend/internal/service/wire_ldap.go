//go:build ldap
// +build ldap

package service

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/wire"
)

// ProvideExternalAuthProvider is used when LDAP is enabled
func ProvideExternalAuthProvider(
	userRepo UserRepository,
	ldapUserRepo LDAPUserRepository,
	settingService *SettingService,
	cfg *config.Config,
	refreshTokenCache RefreshTokenCache,
) (ExternalAuthProvider, func()) {
	provider := NewLDAPProvider(userRepo, ldapUserRepo, settingService, cfg, refreshTokenCache)
	provider.Start()

	cleanup := func() {
		provider.Stop()
	}

	return provider, cleanup
}

var ProviderSetExternalAuth = wire.NewSet(
	ProvideExternalAuthProvider,
)
