//go:build unit

package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func a111NewTLSCacheService(t *testing.T, isolation string) *httpUpstreamService {
	t.Helper()
	cfg := &config.Config{
		Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{AllowPrivateHosts: true}},
	}
	cfg.Gateway.ConnectionPoolIsolation = isolation
	svc, ok := NewHTTPUpstream(cfg).(*httpUpstreamService)
	require.True(t, ok)
	return svc
}

func a111ProfileA() *tlsfingerprint.Profile {
	return &tlsfingerprint.Profile{Name: "A", CipherSuites: []uint16{0x1301}}
}

func TestTLSProfileCache_A111_ChangedProfileGetsNewClient(t *testing.T) {
	svc := a111NewTLSCacheService(t, config.ConnectionPoolIsolationAccountProxy)

	eA, err := svc.getClientEntryWithTLS("", 1, 1, a111ProfileA(), service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(t, err)

	// 同一账号切换到不同 profile，必须得到新的客户端
	eB, err := svc.getClientEntryWithTLS("", 1, 1, &tlsfingerprint.Profile{Name: "B", CipherSuites: []uint16{0x1302}}, service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(t, err)
	require.NotSame(t, eA, eB, "changed TLS profile must not reuse the client built with the old profile")

	// 同名但参数被修改的 profile 也必须重建
	eAEdited, err := svc.getClientEntryWithTLS("", 1, 1, &tlsfingerprint.Profile{Name: "A", CipherSuites: []uint16{0x1303}}, service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(t, err)
	require.NotSame(t, eA, eAEdited, "edited TLS profile fields must not reuse the old client")

	// 内容相同的新指针应复用已有客户端，避免每次请求都重建
	eA2, err := svc.getClientEntryWithTLS("", 1, 1, a111ProfileA(), service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(t, err)
	require.Same(t, eA, eA2, "identical profile content should reuse the cached client")
}

func TestTLSProfileCache_A111_ProxyIsolationSeparatesProfiles(t *testing.T) {
	svc := a111NewTLSCacheService(t, config.ConnectionPoolIsolationProxy)

	e1, err := svc.getClientEntryWithTLS("", 1, 1, a111ProfileA(), service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(t, err)
	e2, err := svc.getClientEntryWithTLS("", 2, 1, &tlsfingerprint.Profile{Name: "B", CipherSuites: []uint16{0x1302}}, service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(t, err)
	require.NotSame(t, e1, e2, "accounts with different TLS profiles on the same proxy must not share a fingerprint client")

	// 同一代理、相同 profile 的账号仍共享客户端
	e3, err := svc.getClientEntryWithTLS("", 3, 1, a111ProfileA(), service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(t, err)
	require.Same(t, e1, e3)
}
