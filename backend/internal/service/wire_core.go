//go:build !ldap
// +build !ldap

package service

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/wire"
)

// ProvideExternalAuthProvider is used when LDAP is disabled
func ProvideExternalAuthProvider(
	userRepo UserRepository,
	ldapUserRepo LDAPUserRepository,
	settingService *SettingService,
	cfg *config.Config,
	refreshTokenCache RefreshTokenCache,
) (ExternalAuthProvider, func()) {
	return nil, func() {}
}

var ProviderSetExternalAuth = wire.NewSet(
	ProvideExternalAuthProvider,
)
