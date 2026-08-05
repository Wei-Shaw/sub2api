package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestProxyPoolProxyFromServiceUsesAPIFieldsAndHidesPassword(t *testing.T) {
	mapped := ProxyPoolProxyFromService(&service.ProxyPoolProxy{
		Proxy:      service.Proxy{ID: 7, Name: "pool member", Password: "secret"},
		PoolID:     2,
		PoolHealth: service.ProxyPoolHealthHealthy,
	})
	payload, err := json.Marshal(mapped)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"id":7`)
	require.Contains(t, string(payload), `"pool_id":2`)
	require.Contains(t, string(payload), `"pool_health":"healthy"`)
	require.NotContains(t, string(payload), "secret")
	require.NotContains(t, string(payload), "Password")
}
