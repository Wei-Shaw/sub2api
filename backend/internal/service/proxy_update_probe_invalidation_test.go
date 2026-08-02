//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updatingProxyRepoStub struct {
	*proxyRepoStub
	proxy       *Proxy
	updateCalls int

	healthAuditCalls []struct {
		proxyID      int64
		failCount    int
		lastHealthAt *time.Time
		isolatedBy   string
	}
}

func (s *updatingProxyRepoStub) GetByID(context.Context, int64) (*Proxy, error) {
	copy := *s.proxy
	return &copy, nil
}

func (s *updatingProxyRepoStub) Update(_ context.Context, proxy *Proxy) error {
	s.updateCalls++
	copy := *proxy
	s.proxy = &copy
	return nil
}

func (s *updatingProxyRepoStub) UpdateHealthAudit(_ context.Context, proxyID int64, failCount int, lastHealthAt *time.Time, isolatedBy string) error {
	s.healthAuditCalls = append(s.healthAuditCalls, struct {
		proxyID      int64
		failCount    int
		lastHealthAt *time.Time
		isolatedBy   string
	}{proxyID: proxyID, failCount: failCount, lastHealthAt: lastHealthAt, isolatedBy: isolatedBy})
	return nil
}

func TestBothProxyUpdateServicesUseRepositoryUpdateBoundary(t *testing.T) {
	t.Run("ProxyService", func(t *testing.T) {
		repo := &updatingProxyRepoStub{
			proxyRepoStub: &proxyRepoStub{},
			proxy:         &Proxy{ID: 9, Protocol: "http", Host: "old.example", Port: 8080, Status: StatusActive},
		}
		svc := NewProxyService(repo)
		host := "new.example"

		_, err := svc.Update(context.Background(), 9, UpdateProxyRequest{Host: &host})

		require.NoError(t, err)
		require.Equal(t, 1, repo.updateCalls)
		require.Equal(t, host, repo.proxy.Host)
	})

	t.Run("adminService", func(t *testing.T) {
		repo := &updatingProxyRepoStub{
			proxyRepoStub: &proxyRepoStub{},
			proxy: &Proxy{
				ID:             9,
				Protocol:       "http",
				Host:           "old.example",
				Port:           8080,
				Status:         StatusActive,
				FallbackMode:   FallbackModeNone,
				ExpiryWarnDays: 7,
			},
		}
		svc := &adminServiceImpl{proxyRepo: repo}

		mode := FallbackModeNone
		warnDays := 7
		_, err := svc.UpdateProxy(context.Background(), 9, &UpdateProxyInput{
			Host:           "new.example",
			FallbackMode:   &mode,
			ExpiryWarnDays: &warnDays,
		})

		require.NoError(t, err)
		require.Equal(t, 1, repo.updateCalls)
		require.Equal(t, "new.example", repo.proxy.Host)
	})
}

func TestAdminService_UpdateProxy_PartialDoesNotClearExpiry(t *testing.T) {
	exp := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	repo := &updatingProxyRepoStub{
		proxyRepoStub: &proxyRepoStub{},
		proxy: &Proxy{
			ID:             11,
			Name:           "keep-me",
			Protocol:       "http",
			Host:           "old.example",
			Port:           8080,
			Status:         StatusActive,
			ExpiresAt:      &exp,
			FallbackMode:   FallbackModeDirect,
			ExpiryWarnDays: 5,
			Username:       "u1",
			Password:       "p1",
		},
	}
	svc := &adminServiceImpl{proxyRepo: repo}

	// Only rename — expiry/fallback/creds must stay.
	_, err := svc.UpdateProxy(context.Background(), 11, &UpdateProxyInput{Name: "renamed"})
	require.NoError(t, err)
	require.Equal(t, "renamed", repo.proxy.Name)
	require.NotNil(t, repo.proxy.ExpiresAt)
	require.True(t, repo.proxy.ExpiresAt.Equal(exp))
	require.Equal(t, FallbackModeDirect, repo.proxy.FallbackMode)
	require.Equal(t, 5, repo.proxy.ExpiryWarnDays)
	require.Equal(t, "u1", repo.proxy.Username)
	require.Equal(t, "p1", repo.proxy.Password)

	// Explicit clear of expires_at.
	_, err = svc.UpdateProxy(context.Background(), 11, &UpdateProxyInput{ExpiresAtProvided: true, ExpiresAt: nil})
	require.NoError(t, err)
	require.Nil(t, repo.proxy.ExpiresAt)

	// Explicit clear of password.
	empty := ""
	_, err = svc.UpdateProxy(context.Background(), 11, &UpdateProxyInput{Password: &empty})
	require.NoError(t, err)
	require.Equal(t, "", repo.proxy.Password)
	require.Equal(t, "u1", repo.proxy.Username)
}
