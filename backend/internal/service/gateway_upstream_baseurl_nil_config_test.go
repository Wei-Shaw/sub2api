//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/stretchr/testify/require"
)

// cfg 为 nil 时，原实现 `if s.cfg != nil && !s.cfg.Security.URLAllowlist.Enabled`
// 会因条件为 false 落进白名单分支并无条件解引用 s.cfg，直接 panic。
// 修复后 nil 按最严格策略处理：强制 HTTPS + 阻断环回/私网/链路本地。
func TestValidateUpstreamBaseURLWithNilConfigFailsSafe(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "公网 HTTPS 放行", raw: "https://api.example.com/v1/", wantErr: false},
		{name: "明文 HTTP 拒绝", raw: "http://api.example.com", wantErr: true},
		{name: "环回地址拒绝", raw: "https://127.0.0.1:8080", wantErr: true},
		{name: "localhost 拒绝", raw: "https://localhost:3000", wantErr: true},
		{name: "云元数据地址拒绝", raw: "https://169.254.169.254", wantErr: true},
		{name: "RFC1918 地址拒绝", raw: "https://10.1.2.3", wantErr: true},
		{name: "空值拒绝", raw: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			normalized, err := validateUpstreamBaseURLWithConfig(nil, tc.raw)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "https://api.example.com/v1", normalized)
		})
	}
}

// 三个网关服务的方法必须共用同一实现，且在 cfg 未装配时不再 panic。
func TestGatewaySiblingsValidateUpstreamBaseURLNilConfigNoPanic(t *testing.T) {
	const metadata = "https://169.254.169.254"

	validators := map[string]func(string) (string, error){
		"GatewayService":              (&GatewayService{}).validateUpstreamBaseURL,
		"OpenAIGatewayService":        (&OpenAIGatewayService{}).validateUpstreamBaseURL,
		"GeminiMessagesCompatService": (&GeminiMessagesCompatService{}).validateUpstreamBaseURL,
	}

	for name, validate := range validators {
		t.Run(name, func(t *testing.T) {
			require.NotPanics(t, func() {
				_, _ = validate(metadata)
			})
			_, err := validate(metadata)
			require.ErrorIs(t, err, urlvalidator.ErrPrivateHostBlocked)

			normalized, err := validate("https://relay.example.com/")
			require.NoError(t, err)
			require.Equal(t, "https://relay.example.com", normalized)
		})
	}
}

// cfg 非 nil 时行为不变：白名单开启走 ValidateHTTPSURL + RequireAllowlist。
func TestValidateUpstreamBaseURLWithConfigAllowlistBranch(t *testing.T) {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = true
	cfg.Security.URLAllowlist.UpstreamHosts = []string{"relay.example.com"}

	normalized, err := validateUpstreamBaseURLWithConfig(cfg, "https://relay.example.com/")
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com", normalized)

	_, err = validateUpstreamBaseURLWithConfig(cfg, "https://evil.example.net")
	require.Error(t, err)

	// 白名单开启时强制 HTTPS，allow_insecure_http 不参与该分支。
	_, err = validateUpstreamBaseURLWithConfig(cfg, "http://relay.example.com")
	require.Error(t, err)
}

// 白名单关闭（默认部署）时退化为 ValidateURLFormat，allow_insecure_http 生效。
func TestValidateUpstreamBaseURLWithConfigAllowlistDisabledBranch(t *testing.T) {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false

	_, err := validateUpstreamBaseURLWithConfig(cfg, "http://relay.example.com")
	require.Error(t, err, "allow_insecure_http=false 时明文 HTTP 必须被拒")

	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	normalized, err := validateUpstreamBaseURLWithConfig(cfg, "http://relay.example.com/")
	require.NoError(t, err)
	require.Equal(t, "http://relay.example.com", normalized)
}
