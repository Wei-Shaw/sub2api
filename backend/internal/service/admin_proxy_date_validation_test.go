//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdminProxyRejectsOutOfRangeExpiry(t *testing.T) {
	for _, year := range []int{-1, 10000} {
		expiry := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		t.Run(expiry.String(), func(t *testing.T) {
			// A nil repository ensures rejection happens before any persistence or probe.
			svc := &adminServiceImpl{}
			_, err := svc.CreateProxy(context.Background(), &CreateProxyInput{ExpiresAt: &expiry})
			require.Error(t, err)
			require.Contains(t, err.Error(), "proxy expiry year")
			_, err = svc.UpdateProxy(context.Background(), 1, &UpdateProxyInput{ExpiresAt: &expiry})
			require.Error(t, err)
			require.Contains(t, err.Error(), "proxy expiry year")
		})
	}
}

func TestAdminProxyAcceptsJSONExpiryBoundaries(t *testing.T) {
	for _, year := range []int{0, 9999} {
		expiry := time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC)
		repo := &updatingProxyRepoStub{proxyRepoStub: &proxyRepoStub{}, proxy: &Proxy{ID: 9}}
		svc := &adminServiceImpl{proxyRepo: repo}
		got, err := svc.UpdateProxy(context.Background(), 9, &UpdateProxyInput{ExpiresAt: &expiry})
		require.NoError(t, err)
		_, err = json.Marshal(got)
		require.NoError(t, err)
		require.Equal(t, &expiry, repo.proxy.ExpiresAt)
	}
}
