//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminProxyUpdateCredentials(t *testing.T) {
	check := func(name string, input UpdateProxyInput, username, password string) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			repo := &updatingProxyRepoStub{proxyRepoStub: &proxyRepoStub{}, proxy: &Proxy{
				ID: 9, Username: "old-user", Password: "old-pass", FallbackMode: FallbackModeNone,
			}}
			svc := &adminServiceImpl{proxyRepo: repo}
			got, err := svc.UpdateProxy(context.Background(), 9, &input)
			require.NoError(t, err)
			require.Equal(t, username, got.Username)
			require.Equal(t, password, got.Password)
			require.Equal(t, 1, repo.updateCalls)
		})
	}
	empty, username, password := "", "new-user", "new-pass"
	check("omitted", UpdateProxyInput{Status: "inactive"}, "old-user", "old-pass")
	check("clear both", UpdateProxyInput{Username: &empty, Password: &empty}, "", "")
	check("clear username only", UpdateProxyInput{Username: &empty}, "", "old-pass")
	check("clear password only", UpdateProxyInput{Password: &empty}, "old-user", "")
	check("replace both", UpdateProxyInput{Username: &username, Password: &password}, username, password)
}
